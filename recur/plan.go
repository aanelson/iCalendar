package recur

import (
	"slices"

	"github.com/aanelson/iCalendar/civil"
)

// A plan is a Rule compiled against a DTSTART: its BYxxx parts turned into
// bit masks, with the defaults RFC 5545 takes from DTSTART filled in.
//
// The engine gives every part a meaning for every frequency, so that rules
// breaking a MUST NOT still expand predictably. A day-level part that the
// §3.3.10 table marks N/A for a frequency (BYWEEKNO outside YEARLY,
// BYYEARDAY with MONTHLY) acts as a limit. A numbered BYDAY outside MONTHLY
// and YEARLY loses its number, as in python-dateutil.
type plan struct {
	freq     Frequency
	interval int64
	wkst     Weekday
	start    int64 // DTSTART as wall-clock epoch seconds
	u0       int64 // the unit (year, month, week, …) DTSTART's period starts at

	// times are the instance times of day, sorted, for DAILY and longer
	// periods; for HOURLY and MINUTELY they are the offsets within the
	// period (minute*60+second, or second). SECONDLY has the single offset 0.
	times []int32

	// Limits on the period start for sub-daily frequencies. Zero is no limit.
	hourMask   uint32 // bit h
	minuteMask uint64 // bit m
	secondMask uint64 // bit s

	// Day-level parts. A zero mask, or a false has-flag, is no restriction.
	monthMask     uint16    // bit m, 1-12
	hasMonthDay   bool      //
	monthDayPos   uint32    // bit d for d, 1-31
	monthDayNeg   uint32    // bit d for -d
	hasYearDay    bool      //
	yearDayPos    [6]uint64 // bit d for d, 1-366
	yearDayNeg    [6]uint64 // bit d for -d
	hasWeekNo     bool      //
	weekNoPos     uint64    // bit w for w, 1-53
	weekNoNeg     uint64    // bit w for -w
	hasByDay      bool      //
	weekdayMask   uint8     // bit d for every d (Monday is bit 0)
	nth           []WeekdayNum
	nthWithinYear bool // nth weekdays count within the year, not each month

	setPos []int
	// perPeriod is how many instances a sub-daily period that passes every
	// limit produces: its times, or the BYSETPOS picks among them.
	perPeriod int64

	// weekYears makes YEARLY periods week-numbering years.
	weekYears bool

	// empty records that the rule is statically known to produce nothing.
	empty bool
	// cycle is how many consecutive periods without a candidate prove that
	// no later period will have one: see unitsPerCycle.
	cycle int64
}

// unitsPerCycle is the length, in each frequency's unit, of the 400-year
// Gregorian cycle: 146,097 days, which is exactly 20,871 weeks. Calendar
// patterns repeat with this period, so a rule stepping INTERVAL units visits
// every position it ever will within lcm(INTERVAL, cycle) units.
var unitsPerCycle = [...]int64{
	Secondly: 146097 * 86400,
	Minutely: 146097 * 1440,
	Hourly:   146097 * 24,
	Daily:    146097,
	Weekly:   20871,
	Monthly:  4800,
	Yearly:   400,
}

// compile builds the plan of r anchored at start, a wall-clock DTSTART.
// allDay marks a DATE DTSTART.
func compile(r Rule, start civil.DateTime, allDay bool) *plan {
	start.Nanosecond = 0
	if allDay {
		// RFC 5545 §3.3.10: BYSECOND, BYMINUTE and BYHOUR "MUST be
		// ignored" with a DATE DTSTART.
		r.BySecond, r.ByMinute, r.ByHour = nil, nil, nil
		start.Time = civil.Time{}
	}
	p := &plan{
		freq:     r.Freq,
		interval: int64(max(r.Interval, 1)),
		wkst:     r.WeekStart,
		start:    start.EpochSecond(),
	}
	if p.freq < Secondly || p.freq > Yearly || p.wkst < Monday || p.wkst > Sunday {
		p.empty = true
		return p
	}
	// With BYWEEKNO a YEARLY period is a week-numbering year, so that
	// INTERVAL counts the years the weeks belong to, as libical and
	// ical.net read it: week 53 of 2009 plus six years is week 53 of 2015,
	// though the first falls in January 2010.
	p.weekYears = p.freq == Yearly && len(r.ByWeekNo) > 0
	p.u0 = p.unitOf(p.start)
	c := unitsPerCycle[p.freq]
	p.cycle = c / gcd(p.interval, c)

	// Day-level defaults. RFC 5545 §3.3.10: "Information, not contained in
	// the rule, necessary to determine the various recurrence instance
	// start time and dates are derived from the Start Time". Which parts
	// are filled in follows libical, ical.js and ical.net, which agree:
	//
	//   - With no day-level part, YEARLY takes DTSTART's month (unless
	//     BYMONTH is given) and day of month; MONTHLY its day of month;
	//     WEEKLY its weekday.
	//   - YEARLY with BYWEEKNO but no BYDAY takes DTSTART's weekday in
	//     each listed week, as BYMONTH takes DTSTART's day in each month
	//     (unless BYMONTHDAY or BYYEARDAY already picks the day).
	//   - YEARLY with BYMONTHDAY but no BYMONTH takes DTSTART's month
	//     (unless BYWEEKNO or BYYEARDAY already places the day in the year).
	//
	// python-dateutil and its ports differ on the last two, expanding to
	// every day of the week and every month.
	byMonth, byMonthDay, byDay := r.ByMonth, r.ByMonthDay, r.ByDay
	weekday := WeekdayOf(start.Weekday())
	noDayParts := len(r.ByWeekNo) == 0 && len(r.ByYearDay) == 0 && len(r.ByMonthDay) == 0 && len(r.ByDay) == 0
	switch {
	case p.freq == Yearly && noDayParts:
		if len(byMonth) == 0 {
			byMonth = []int{int(start.Month)}
		}
		byMonthDay = []int{start.Day}
	case p.freq == Yearly && len(r.ByWeekNo) > 0 && len(r.ByDay) == 0 && len(r.ByYearDay) == 0 && len(r.ByMonthDay) == 0:
		byDay = []WeekdayNum{{Day: weekday}}
	case p.freq == Yearly && len(r.ByMonthDay) > 0 && len(r.ByMonth) == 0 && len(r.ByWeekNo) == 0 && len(r.ByYearDay) == 0:
		byMonth = []int{int(start.Month)}
	case p.freq == Monthly && noDayParts:
		byMonthDay = []int{start.Day}
	case p.freq == Weekly && noDayParts:
		byDay = []WeekdayNum{{Day: weekday}}
	}
	for _, m := range byMonth {
		if m >= 1 && m <= 12 {
			p.monthMask |= 1 << m
		}
	}
	for _, d := range byMonthDay {
		p.hasMonthDay = true
		switch {
		case d >= 1 && d <= 31:
			p.monthDayPos |= 1 << d
		case d <= -1 && d >= -31:
			p.monthDayNeg |= 1 << -d
		}
	}
	for _, d := range r.ByYearDay {
		p.hasYearDay = true
		switch {
		case d >= 1 && d <= 366:
			p.yearDayPos[d/64] |= 1 << (d % 64)
		case d <= -1 && d >= -366:
			p.yearDayNeg[-d/64] |= 1 << (-d % 64)
		}
	}
	for _, w := range r.ByWeekNo {
		p.hasWeekNo = true
		switch {
		case w >= 1 && w <= 53:
			p.weekNoPos |= 1 << w
		case w <= -1 && w >= -53:
			p.weekNoNeg |= 1 << -w
		}
	}
	for _, w := range byDay {
		if w.Day < Monday || w.Day > Sunday {
			continue
		}
		p.hasByDay = true
		if w.N == 0 || (p.freq != Monthly && p.freq != Yearly) {
			p.weekdayMask |= 1 << w.Day
		} else if !slices.Contains(p.nth, w) {
			p.nth = append(p.nth, w)
		}
	}
	// Errata 1913 and 3779: in a YEARLY rule a numbered BYDAY counts within
	// each month when BYMONTH is given, and within the year otherwise.
	p.nthWithinYear = p.freq == Yearly && len(r.ByMonth) == 0
	for _, n := range r.BySetPos {
		if n != 0 && !slices.Contains(p.setPos, n) {
			p.setPos = append(p.setPos, n)
		}
	}

	// Time-of-day parts. A part finer than the frequency expands the period
	// (defaulting to DTSTART's value); one at or coarser than it limits.
	hours := timeList(r.ByHour, start.Hour, 23, p.freq > Hourly)
	minutes := timeList(r.ByMinute, start.Minute, 59, p.freq > Minutely)
	// A leap second (60) can't be expressed on a wall clock that has none,
	// so it is dropped, like an invalid date.
	seconds := timeList(r.BySecond, start.Second, 59, p.freq > Secondly)
	switch p.freq {
	case Secondly:
		p.hourMask, p.minuteMask, p.secondMask = mask32(hours), mask64(minutes), mask64(seconds)
		if len(r.BySecond) > 0 && len(seconds) == 0 {
			p.empty = true // only leap seconds
		}
		p.times = []int32{0}
	case Minutely:
		p.hourMask, p.minuteMask = mask32(hours), mask64(minutes)
		for _, s := range seconds {
			p.times = append(p.times, int32(s))
		}
	case Hourly:
		p.hourMask = mask32(hours)
		for _, m := range minutes {
			for _, s := range seconds {
				p.times = append(p.times, int32(m*60+s))
			}
		}
	default:
		for _, h := range hours {
			for _, m := range minutes {
				for _, s := range seconds {
					p.times = append(p.times, int32(h*3600+m*60+s))
				}
			}
		}
	}
	if len(p.times) == 0 {
		p.empty = true
	}
	p.perPeriod = int64(len(p.times))
	if len(p.setPos) > 0 {
		picked := map[int]bool{}
		for _, n := range p.setPos {
			i := n - 1
			if n < 0 {
				i = len(p.times) + n
			}
			if i >= 0 && i < len(p.times) {
				picked[i] = true
			}
		}
		p.perPeriod = int64(len(picked))
	}
	if !p.empty {
		p.empty = p.unsatisfiable()
	}
	return p
}

// timeList returns the sorted, de-duplicated values of a BYHOUR, BYMINUTE
// or BYSECOND part, keeping those in [0, hi]. An absent part that expands
// takes DTSTART's value; one that limits stays empty, which means any.
func timeList(by []int, def, hi int, expands bool) []int {
	if len(by) == 0 {
		if expands {
			return []int{def}
		}
		return nil
	}
	var out []int
	for _, v := range by {
		if v >= 0 && v <= hi && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	slices.Sort(out)
	return out
}

func mask32(vs []int) uint32 {
	var m uint32
	for _, v := range vs {
		m |= 1 << v
	}
	return m
}

func mask64(vs []int) uint64 {
	var m uint64
	for _, v := range vs {
		m |= 1 << v
	}
	return m
}

// unsatisfiable reports combinations that can never produce an instance,
// so they cost nothing to iterate. Anything it misses still terminates,
// by the cycle bound.
func (p *plan) unsatisfiable() bool {
	// A month-day no allowed month has, like BYMONTH=2;BYMONTHDAY=30.
	if p.hasMonthDay {
		ok := false
		for m := 1; m <= 12 && !ok; m++ {
			if p.monthMask != 0 && p.monthMask&(1<<m) == 0 {
				continue
			}
			longest := civil.DaysIn(2000, monthOf(m)) // 2000 is a leap year
			for d := 1; d <= longest; d++ {
				if p.monthDayPos&(1<<d) != 0 || p.monthDayNeg&(1<<(longest-d+1)) != 0 {
					ok = true
					break
				}
				// A negative day in a short February: -29 exists only
				// in leap years, which the leap length above covers.
			}
		}
		if !ok {
			return true
		}
	}
	if p.hasMonthDay && p.monthDayPos == 0 && p.monthDayNeg == 0 ||
		p.hasYearDay && p.yearDayPos == [6]uint64{} && p.yearDayNeg == [6]uint64{} ||
		p.hasWeekNo && p.weekNoPos == 0 && p.weekNoNeg == 0 {
		return true
	}
	// A sub-daily rule's period starts land on the same few times of day
	// forever when INTERVAL shares a factor with the day: INTERVAL=24 from
	// 09:00 is always 09:00, so BYHOUR=11 never matches.
	if p.freq <= Hourly && (p.hourMask != 0 || p.minuteMask != 0 || p.secondMask != 0) {
		perDay := map[Frequency]int64{Hourly: 24, Minutely: 1440, Secondly: 86400}[p.freq]
		g := gcd(p.interval, perDay)
		phase := floorMod(p.u0, perDay) % g
		reachable := false
		for r := phase; r < perDay && !reachable; r += g {
			var h, m, s int64
			switch p.freq {
			case Hourly:
				h = r
			case Minutely:
				h, m = r/60, r%60
			case Secondly:
				h, m, s = r/3600, r/60%60, r%60
			}
			reachable = (p.hourMask == 0 || p.hourMask&(1<<h) != 0) &&
				(p.minuteMask == 0 || p.minuteMask&(1<<m) != 0) &&
				(p.secondMask == 0 || p.secondMask&(1<<s) != 0)
		}
		if !reachable {
			return true
		}
	}
	// BYSETPOS positions beyond the most candidates a period can have,
	// like FREQ=MINUTELY;BYSETPOS=2 (one candidate a minute). For a
	// sub-daily or daily rule the count is exact, which matters: their
	// cycle bound is long.
	if len(p.setPos) > 0 {
		days := map[Frequency]int{Yearly: 371, Monthly: 31, Weekly: 7}[p.freq]
		most := max(days, 1) * len(p.times)
		possible := false
		for _, n := range p.setPos {
			possible = possible || n >= -most && n <= most
		}
		if !possible {
			return true
		}
	}
	// Numbered weekdays that can't exist: a month has at most five of each
	// weekday and a year at most 53.
	if p.hasByDay && p.weekdayMask == 0 {
		most := 5
		if p.nthWithinYear {
			most = 53
		}
		possible := false
		for _, w := range p.nth {
			possible = possible || w.N >= -most && w.N <= most
		}
		if !possible {
			return true
		}
	}
	// A DAILY rule with an INTERVAL of whole weeks only ever sees one
	// weekday.
	if p.freq == Daily && p.hasByDay && len(p.nth) == 0 && p.interval%7 == 0 {
		if p.weekdayMask&(1<<weekdayOfDay(floorDiv(p.start, 86400))) == 0 {
			return true
		}
	}
	return false
}

// unitOf returns the unit of p's frequency that the wall-clock second t
// falls in: its year, month (year*12 + month - 1), week (starting WKST),
// day, hour, minute or second, counted from the epoch.
func (p *plan) unitOf(t int64) int64 {
	switch p.freq {
	case Yearly:
		if p.weekYears {
			return int64(weekYearOf(floorDiv(t, 86400), p.wkst))
		}
		return int64(civil.DateOfEpochDay(floorDiv(t, 86400)).Year)
	case Monthly:
		d := civil.DateOfEpochDay(floorDiv(t, 86400))
		return int64(d.Year)*12 + int64(d.Month) - 1
	case Weekly:
		return floorDiv(floorDiv(t, 86400)+3-int64(p.wkst), 7)
	case Daily:
		return floorDiv(t, 86400)
	case Hourly:
		return floorDiv(t, 3600)
	case Minutely:
		return floorDiv(t, 60)
	default:
		return t
	}
}

// unitStart returns the wall-clock second unit u starts at.
func (p *plan) unitStart(u int64) int64 {
	switch p.freq {
	case Yearly:
		if p.weekYears {
			return weekYearStart(int(u), p.wkst) * 86400
		}
		return civil.Date{Year: int(u), Month: 1, Day: 1}.EpochDay() * 86400
	case Monthly:
		return civil.Date{Year: int(floorDiv(u, 12)), Month: monthOf(int(floorMod(u, 12)) + 1), Day: 1}.EpochDay() * 86400
	case Weekly:
		return (u*7 - 3 + int64(p.wkst)) * 86400
	case Daily:
		return u * 86400
	case Hourly:
		return u * 3600
	case Minutely:
		return u * 60
	default:
		return u
	}
}

// periodStart returns the wall-clock second period n starts at.
func (p *plan) periodStart(n int64) int64 {
	return p.unitStart(p.u0 + n*p.interval)
}

// periodAt returns the first period that ends after the wall-clock second
// t, never before period 0.
func (p *plan) periodAt(t int64) int64 {
	return max(0, floorDiv(p.unitOf(t)-p.u0, p.interval))
}

// firstPeriodFrom returns the first period whose unit is at or after u.
func (p *plan) firstPeriodFrom(u int64) int64 {
	return max(0, ceilDiv(u-p.u0, p.interval))
}

// weekdayOfDay returns the weekday of an epoch day, Monday 0.
func weekdayOfDay(day int64) Weekday {
	return Weekday(floorMod(day+3, 7)) // 1970-01-01 was a Thursday
}

func gcd(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	if a < 0 {
		return -a
	}
	return a
}

func floorDiv(a, b int64) int64 {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

func ceilDiv(a, b int64) int64 {
	return -floorDiv(-a, b)
}

func floorMod(a, b int64) int64 {
	return a - floorDiv(a, b)*b
}
