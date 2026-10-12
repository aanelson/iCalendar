package recur_test

// An independent check of the recurrence engine against RFC 5545 §3.3.10.
//
// ref below is a reference expander written only from the text of
// §3.3.10, §3.8.5.3 and the Verified errata (docs/rfc/rfc5545-errata.md).
// It is deliberately naive: it walks every FREQ × INTERVAL period, looks at
// every day of the period and every time of day the rule allows, and keeps
// the candidates that pass every BYxxx part. It shares no code with the
// engine and does its calendar arithmetic with package time, not civil.
//
// TestConformanceDifferential compares it with Set.Between and Rule.Between
// on pseudo-random rules. Where the RFC leaves a choice open the reference
// follows docs/architecture.md ("Semantics settled by the vectors"), and
// each such place is marked CHOICE.

import (
	"flag"
	"fmt"
	"math/rand/v2"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aanelson/iCalendar/civil"
	"github.com/aanelson/iCalendar/recur"
)

var conformanceRules = flag.Int("recur.conformance", 0,
	"random rules TestConformanceDifferential compares, and three times the sets TestConformanceSetDifferential does "+
		"(0: 3000 rules and 1000 sets, a tenth with -short)")

// ---- Calendar arithmetic, on package time in UTC ----

func mkDay(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func daysInMonth(y int, m time.Month) int { return mkDay(y, m+1, 0).Day() }

func daysInYear(y int) int { return mkDay(y, 12, 31).YearDay() }

// dayDiff returns b - a in days, for two midnights.
func dayDiff(a, b time.Time) int { return int((b.Unix() - a.Unix()) / 86400) }

func toCivil(t time.Time) civil.DateTime {
	y, m, d := t.Date()
	h, mi, s := t.Clock()
	return civil.DateTime{Date: civil.Date{Year: y, Month: m, Day: d}, Time: civil.Time{Hour: h, Minute: mi, Second: s}}
}

func fromCivil(c civil.DateTime) time.Time {
	return time.Date(c.Year, c.Month, c.Day, c.Hour, c.Minute, c.Second, 0, time.UTC)
}

// stdWeekday converts a recur.Weekday (Monday = 0, as documented) to a
// time.Weekday, without the library's own conversion.
func stdWeekday(d recur.Weekday) time.Weekday { return time.Weekday((int(d) + 1) % 7) }

// matchSigned reports whether v, the n-th of count (a month day of the
// month, a year day of the year, ...), is in list, where -1 is the last.
// §3.3.10: "-10 represents the tenth to the last day of the month".
func matchSigned(list []int, v, count int) bool {
	for _, x := range list {
		if x > 0 && x == v || x < 0 && count+1+x == v {
			return true
		}
	}
	return false
}

// ---- The reference expander ----

type ref struct {
	r      recur.Rule
	start  time.Time // DTSTART's wall clock, as UTC
	allDay bool      // DTSTART is a DATE
	wkst   time.Weekday

	hours, minutes, seconds []int // the times of day the rule allows
	w1                      map[int]time.Time
}

func newRef(r recur.Rule, start civil.DateTime, allDay bool) *ref {
	x := &ref{r: r, start: fromCivil(start), allDay: allDay, wkst: stdWeekday(r.WeekStart), w1: map[int]time.Time{}}
	all := func(n int) []int {
		s := make([]int, n)
		for i := range s {
			s[i] = i
		}
		return s
	}
	set := func(l []int) []int {
		s := slices.Clone(l)
		slices.Sort(s)
		return slices.Compact(s)
	}
	f := r.Freq
	switch {
	case allDay:
		// §3.3.10: BYSECOND, BYMINUTE and BYHOUR "MUST be ignored" with a
		// DATE DTSTART; every instance is a date, i.e. midnight.
		x.hours, x.minutes, x.seconds = []int{0}, []int{0}, []int{0}
	default:
		// A BYxxx time part limits (shorter FREQ) or expands (longer FREQ);
		// either way the instance's field is in the list. Without it the
		// field comes from the period for a FREQ at most that long, and
		// otherwise "from the Start Time" (§3.3.10).
		x.hours, x.minutes, x.seconds = set(r.ByHour), set(r.ByMinute), set(r.BySecond)
		if len(r.ByHour) == 0 {
			x.hours = all(24)
			if f >= recur.Daily {
				x.hours = []int{x.start.Hour()}
			}
		}
		if len(r.ByMinute) == 0 {
			x.minutes = all(60)
			if f >= recur.Hourly {
				x.minutes = []int{x.start.Minute()}
			}
		}
		if len(r.BySecond) == 0 {
			x.seconds = all(60)
			if f >= recur.Minutely {
				x.seconds = []int{x.start.Second()}
			}
		}
	}
	return x
}

// weekStart returns the first day of the week holding d: "A week is
// defined as a seven day period, starting on the day of the week defined
// to be the week start (see WKST)".
func (x *ref) weekStart(d time.Time) time.Time {
	for d.Weekday() != x.wkst {
		d = d.AddDate(0, 0, -1)
	}
	return d
}

// week1 returns the first day of week 1 of year y: "Week number one of the
// calendar year is the first week that contains at least four (4) days in
// that calendar year."
func (x *ref) week1(y int) time.Time {
	if s, ok := x.w1[y]; ok {
		return s
	}
	s := x.weekStart(mkDay(y-1, 12, 25)) // a week with no day of y
	for {
		n := 0
		for i := range 7 {
			if s.AddDate(0, 0, i).Year() == y {
				n++
			}
		}
		if n >= 4 {
			break
		}
		s = s.AddDate(0, 0, 7)
	}
	x.w1[y] = s
	return s
}

// weekNo returns the number of the week holding d, the year that week is
// numbered in, and how many weeks that year has. Weeks are numbered on from
// week 1, so the days before week 1 are in the last week of the year before.
func (x *ref) weekNo(d time.Time) (no, weeks, year int) {
	s := x.weekStart(d)
	year = s.Year()
	if s.Before(x.week1(year)) {
		year--
	} else if !s.Before(x.week1(year + 1)) {
		year++
	}
	w1 := x.week1(year)
	return dayDiff(w1, s)/7 + 1, dayDiff(w1, x.week1(year+1)) / 7, year
}

// nth returns d's position among the days of its weekday in [from, to),
// counted from the start (1, 2, ...) and from the end (-1, -2, ...).
func nth(d, from, to time.Time) (pos, neg int) {
	return dayDiff(from, d)/7 + 1, -(dayDiff(d, to.AddDate(0, 0, -1))/7 + 1)
}

func (x *ref) matchByDay(d time.Time) bool {
	r := x.r
	for _, e := range r.ByDay {
		if stdWeekday(e.Day) != d.Weekday() {
			continue
		}
		n := e.N
		if r.Freq != recur.Monthly && r.Freq != recur.Yearly {
			// MUST NOT: "BYDAY ... with a numeric value when the FREQ rule
			// part is not set to MONTHLY or YEARLY". CHOICE (architecture
			// doc): the number is dropped.
			n = 0
		}
		if n == 0 {
			// "If an integer modifier is not present, it means all days of
			// this type within the specified frequency."
			return true
		}
		// "+1MO ... represents the first Monday within the month, whereas
		// -1MO represents the last Monday of the month." With YEARLY it is
		// "an offset within the month when the BYMONTH rule part is
		// present", and within the year otherwise (errata 1913, 3779).
		y, m, _ := d.Date()
		from, to := mkDay(y, m, 1), mkDay(y, m+1, 1)
		if r.Freq == recur.Yearly && len(r.ByMonth) == 0 {
			from, to = mkDay(y, 1, 1), mkDay(y+1, 1, 1)
		}
		if pos, neg := nth(d, from, to); n == pos || n == neg {
			return true
		}
	}
	return false
}

// dayOK reports whether the rule allows day d. Every BYxxx day part, whether
// the table calls it Expand or Limit, keeps exactly the days in its list,
// so each is a membership test; the order the RFC applies them in doesn't
// change an intersection. What the rule doesn't say comes from DTSTART.
func (x *ref) dayOK(d time.Time) bool {
	r := x.r
	y, m, md := d.Date()
	if len(r.ByMonth) > 0 && !slices.Contains(r.ByMonth, int(m)) {
		return false
	}
	if len(r.ByWeekNo) > 0 {
		// In a YEARLY rule. For any other FREQ it MUST NOT be used; CHOICE
		// (architecture doc): it then acts as a limit.
		no, weeks, _ := x.weekNo(d)
		if !matchSigned(r.ByWeekNo, no, weeks) {
			return false
		}
	}
	if len(r.ByYearDay) > 0 && !matchSigned(r.ByYearDay, d.YearDay(), daysInYear(y)) {
		return false
	}
	if len(r.ByMonthDay) > 0 && !matchSigned(r.ByMonthDay, md, daysInMonth(y, m)) {
		return false
	}
	if len(r.ByDay) > 0 && !x.matchByDay(d) {
		return false
	}
	// "Information, not contained in the rule, necessary to determine the
	// various recurrence instance start time and dates are derived from the
	// Start Time". A day part the table marks N/A for FREQ (CHOICE,
	// architecture doc: it "acts as a limit") limits the days of the period
	// and, like any day part, leaves nothing to take from DTSTART: so
	// FREQ=WEEKLY;BYMONTHDAY=23 is every 23rd, not only Sundays that are.
	_, sm, sd := x.start.Date()
	noDayParts := len(r.ByWeekNo) == 0 && len(r.ByYearDay) == 0 && len(r.ByMonthDay) == 0 && len(r.ByDay) == 0
	switch r.Freq {
	case recur.Weekly:
		if noDayParts && d.Weekday() != x.start.Weekday() {
			return false
		}
	case recur.Monthly:
		if noDayParts && md != sd {
			return false
		}
	case recur.Yearly:
		if noDayParts {
			// "FREQ=YEARLY;BYMONTH=1 doesn't specify a specific day within
			// the month ... This information would be the same as what is
			// specified for DTSTART." Without BYMONTH, the month too.
			if md != sd || len(r.ByMonth) == 0 && m != sm {
				return false
			}
		}
		if len(r.ByMonthDay) > 0 && len(r.ByMonth) == 0 && len(r.ByWeekNo) == 0 && len(r.ByYearDay) == 0 && m != sm {
			// CHOICE (architecture doc): YEARLY with BYMONTHDAY but no
			// BYMONTH takes DTSTART's month. Not when BYWEEKNO or BYYEARDAY
			// already place the day in the year: then the month isn't
			// "necessary to determine" it.
			return false
		}
		if len(r.ByWeekNo) > 0 && len(r.ByDay) == 0 && len(r.ByMonthDay) == 0 && len(r.ByYearDay) == 0 && d.Weekday() != x.start.Weekday() {
			// CHOICE (architecture doc): YEARLY with BYWEEKNO but no BYDAY
			// takes DTSTART's weekday; not when BYMONTHDAY or BYYEARDAY
			// pick the day within the week.
			return false
		}
	}
	return true
}

// periodDays returns the days [from, to) of the period k periods after
// DTSTART's, for FREQ=DAILY and longer. The period holding DTSTART is
// period 0: periods "start at the beginning of the interval defined by the
// FREQ rule part", and WEEKLY periods start on WKST.
func (x *ref) periodDays(k int) (from, to time.Time) {
	sy, sm, sd := x.start.Date()
	day := mkDay(sy, sm, sd)
	switch x.r.Freq {
	case recur.Daily:
		from = day.AddDate(0, 0, k)
		return from, from.AddDate(0, 0, 1)
	case recur.Weekly:
		from = x.weekStart(day).AddDate(0, 0, 7*k)
		return from, from.AddDate(0, 0, 7)
	case recur.Monthly:
		return mkDay(sy, sm+time.Month(k), 1), mkDay(sy, sm+time.Month(k+1), 1)
	}
	if len(x.r.ByWeekNo) > 0 {
		// CHOICE (architecture doc): with BYWEEKNO, YEARLY periods are the
		// years weeks are numbered in, so a week is in one period.
		_, _, wy := x.weekNo(day)
		return x.week1(wy + k), x.week1(wy + k + 1)
	}
	return mkDay(sy+k, 1, 1), mkDay(sy+k+1, 1, 1)
}

// setPos applies BYSETPOS to the sorted instances of one period: "the nth
// occurrence within the set of recurrence instances specified by the rule
// ... in one interval of the recurrence rule".
func (x *ref) setPos(c []time.Time) []time.Time {
	if len(x.r.BySetPos) == 0 {
		return c
	}
	var out []time.Time
	for i, t := range c {
		for _, p := range x.r.BySetPos {
			if p > 0 && p-1 == i || p < 0 && len(c)+p == i {
				out = append(out, t)
				break
			}
		}
	}
	return out
}

// expand returns what the rule generates at or after DTSTART and before
// end, period by period, stopping after the first period that takes it
// past max instances. complete is false when it stopped early: then every
// instance before raw[max] is known.
func (x *ref) expand(end time.Time, max int) (raw []time.Time, complete bool) {
	iv := max1(x.r.Interval)
	emit := func(c []time.Time) bool {
		for _, t := range x.setPos(c) {
			// Instances before DTSTART aren't in the set: "The DTSTART
			// property value always counts as the first occurrence."
			if !t.Before(x.start) && t.Before(end) {
				raw = append(raw, t)
			}
		}
		return len(raw) <= max
	}
	clock := func(d time.Time, h, m, s int) time.Time {
		return d.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second)
	}
	if x.r.Freq >= recur.Daily {
		for k := 0; ; k += iv {
			from, to := x.periodDays(k)
			if !from.Before(end) {
				return raw, true
			}
			var c []time.Time
			for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
				if !x.dayOK(d) {
					continue
				}
				for _, h := range x.hours {
					for _, m := range x.minutes {
						for _, s := range x.seconds {
							c = append(c, clock(d, h, m, s))
						}
					}
				}
			}
			if !emit(c) {
				return raw, false
			}
		}
	}
	// SECONDLY, MINUTELY, HOURLY: a period is one second, minute or hour of
	// the wall clock, so it never spans two days. A day the day parts
	// reject has no instance at all.
	unit := map[recur.Frequency]time.Duration{recur.Hourly: time.Hour, recur.Minutely: time.Minute, recur.Secondly: time.Second}[x.r.Freq]
	p0 := x.start.Truncate(unit)
	sy, sm, sd := x.start.Date()
	for d := mkDay(sy, sm, sd); d.Before(end); d = d.AddDate(0, 0, 1) {
		if !x.dayOK(d) {
			continue
		}
		var c []time.Time
		var p time.Time
		flush := func() bool {
			if len(c) == 0 {
				return true
			}
			k := int64(p.Sub(p0) / unit)
			ok := true
			if k >= 0 && k%int64(iv) == 0 {
				ok = emit(c)
			}
			c = c[:0]
			return ok
		}
		for _, h := range x.hours {
			for _, m := range x.minutes {
				for _, s := range x.seconds {
					t := clock(d, h, m, s)
					if tp := t.Truncate(unit); !tp.Equal(p) {
						if !flush() {
							return raw, false
						}
						p = tp
					}
					c = append(c, t)
				}
			}
		}
		if !flush() {
			return raw, false
		}
	}
	return raw, true
}

func max1(n int) int {
	if n < 1 {
		return 1
	}
	return n
}

// untilOK reports whether t is within UNTIL, which "bounds the recurrence
// rule in an inclusive manner", compared in its own form. A DATE UNTIL on
// a DATE-TIME DTSTART breaks a MUST; CHOICE (architecture doc): it covers
// the whole day. A UTC UNTIL on a floating DTSTART also breaks one (erratum
// 4414); the Set reads floating times as UTC, so it compares wall clocks.
func untilOK(u recur.Until, t time.Time) bool {
	if u.IsZero() {
		return true
	}
	ut := fromCivil(u.DateTime)
	if u.IsDate {
		y, m, d := t.Date()
		return !mkDay(y, m, d).After(mkDay(u.Year, u.Month, u.Day))
	}
	return !t.After(ut)
}

// finish applies COUNT and UNTIL, after DTSTART is added as the first
// instance for a Set ("The "DTSTART" property value always counts as the
// first occurrence", §3.3.10; §3.8.5.3: it "defines the first instance").
// A Rule alone (Rule.All, Rule.Between) doesn't add it.
func (x *ref) finish(raw []time.Time, withStart bool) []time.Time {
	var out []time.Time
	if withStart {
		// CHOICE: DTSTART stays even when UNTIL is before it, as the Set
		// documents ("Start is always the first instance").
		out = append(out, x.start)
		if len(raw) > 0 && raw[0].Equal(x.start) {
			raw = raw[1:]
		}
	}
	for _, t := range raw {
		if !untilOK(x.r.Until, t) {
			break
		}
		out = append(out, t)
	}
	if x.r.Count > 0 && len(out) > x.r.Count {
		out = out[:x.r.Count]
	}
	return out
}

// breaksMustNot returns the §3.3.10 MUST NOT a rule breaks, if any, of those
// a rule can break without DTSTART (the UNTIL forms need DTSTART) and
// without an out-of-range value (which ParseRule refuses).
func breaksMustNot(r recur.Rule) string {
	numbered := slices.ContainsFunc(r.ByDay, func(e recur.WeekdayNum) bool { return e.N != 0 })
	switch {
	case r.Freq == 0:
		return "The FREQ rule part is REQUIRED"
	case r.Count > 0 && !r.Until.IsZero():
		return "The UNTIL or COUNT rule parts are OPTIONAL, but they MUST NOT occur in the same 'recur'"
	case numbered && r.Freq != recur.Monthly && r.Freq != recur.Yearly:
		return "The BYDAY rule part MUST NOT be specified with a numeric value when the FREQ rule part is not set to MONTHLY or YEARLY"
	case numbered && r.Freq == recur.Yearly && len(r.ByWeekNo) > 0:
		return "the BYDAY rule part MUST NOT be specified with a numeric value with the FREQ rule part set to YEARLY when the BYWEEKNO rule part is specified"
	case len(r.ByMonthDay) > 0 && r.Freq == recur.Weekly:
		return "The BYMONTHDAY rule part MUST NOT be specified when the FREQ rule part is set to WEEKLY"
	case len(r.ByYearDay) > 0 && (r.Freq == recur.Daily || r.Freq == recur.Weekly || r.Freq == recur.Monthly):
		return "The BYYEARDAY rule part MUST NOT be specified when the FREQ rule part is set to DAILY, WEEKLY, or MONTHLY"
	case len(r.ByWeekNo) > 0 && r.Freq != recur.Yearly:
		return "[BYWEEKNO] MUST NOT be used when the FREQ rule part is set to anything other than YEARLY"
	case len(r.BySetPos) > 0 && len(r.BySecond)+len(r.ByMinute)+len(r.ByHour)+len(r.ByDay)+len(r.ByMonthDay)+
		len(r.ByYearDay)+len(r.ByWeekNo)+len(r.ByMonth) == 0:
		return "[BYSETPOS] MUST only be used in conjunction with another BYxxx rule part"
	}
	return ""
}

// ---- Random rules ----

type diffCase struct {
	rule    recur.Rule
	start   civil.DateTime
	allDay  bool
	invalid bool // the rule breaks a §3.3.10 MUST NOT (breaksMustNot)
	seed    uint64
}

func (c diffCase) String() string {
	s := fmt.Sprintf("DTSTART:%s RRULE:%s", c.start, c.rule)
	if c.allDay {
		s = fmt.Sprintf("DTSTART;VALUE=DATE:%s RRULE:%s", c.start.Date, c.rule)
	}
	return s
}

// windowYears is how far after DTSTART each FREQ is compared.
var windowYears = map[recur.Frequency]int{
	recur.Secondly: 1, recur.Minutely: 1, recur.Hourly: 2,
	recur.Daily: 4, recur.Weekly: 6, recur.Monthly: 12, recur.Yearly: 30,
}

func pick[T any](rng *rand.Rand, s ...T) T { return s[rng.IntN(len(s))] }

// someInts returns 1 to n distinct values, drawn from gen.
func someInts(rng *rand.Rand, n int, gen func() int) []int {
	var out []int
	for range 1 + rng.IntN(n) {
		if v := gen(); !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

func signed(rng *rand.Rand, max int, edges ...int) func() int {
	return func() int {
		if rng.IntN(2) == 0 {
			return pick(rng, edges...)
		}
		v := 1 + rng.IntN(max)
		if rng.IntN(3) == 0 {
			v = -v
		}
		return v
	}
}

func genStart(rng *rand.Rand) civil.DateTime {
	y := 1996 + rng.IntN(35)
	var d civil.Date
	switch rng.IntN(7) {
	case 0: // Feb 29
		y -= y % 4
		d = civil.Date{Year: y, Month: 2, Day: 29}
	case 1: // a 31st
		d = civil.Date{Year: y, Month: pick(rng, time.January, time.March, time.May, time.July, time.August, time.October, time.December), Day: 31}
	case 2: // the turn of the year, where week numbering is interesting
		if rng.IntN(2) == 0 {
			d = civil.Date{Year: y, Month: 12, Day: 25 + rng.IntN(7)}
		} else {
			d = civil.Date{Year: y, Month: 1, Day: 1 + rng.IntN(4)}
		}
	default:
		m := time.Month(1 + rng.IntN(12))
		d = civil.Date{Year: y, Month: m, Day: 1 + rng.IntN(daysInMonth(y, m))}
	}
	var t civil.Time
	if rng.IntN(3) > 0 {
		t = civil.Time{Hour: rng.IntN(24), Minute: pick(rng, 0, 0, 30, rng.IntN(60)), Second: pick(rng, 0, 0, rng.IntN(60))}
	}
	return civil.DateTime{Date: d, Time: t}
}

func genCase(seed uint64, i int) diffCase {
	rng := rand.New(rand.NewPCG(seed, uint64(i)))
	c := diffCase{seed: uint64(i), start: genStart(rng)}
	r := &c.rule
	r.Freq = recur.Frequency(1 + rng.IntN(7))
	if rng.IntN(2) == 0 {
		r.Interval = 2 + rng.IntN(4)
	}
	if rng.IntN(3) == 0 {
		r.WeekStart = recur.Weekday(rng.IntN(7))
	}
	f := r.Freq
	wild := rng.IntN(7) == 0 // allow parts that break a MUST NOT
	if f >= recur.Daily && rng.IntN(6) == 0 {
		c.allDay = true
		c.start.Time = civil.Time{}
	}
	if rng.IntN(4) == 0 {
		r.ByMonth = someInts(rng, 3, func() int { return 1 + rng.IntN(12) })
	}
	if (f == recur.Yearly || wild) && rng.IntN(4) == 0 {
		r.ByWeekNo = someInts(rng, 3, signed(rng, 53, 1, 2, 52, 53, -1, -2, -52, -53))
	}
	if (f == recur.Yearly || f < recur.Daily || wild) && rng.IntN(5) == 0 {
		r.ByYearDay = someInts(rng, 3, signed(rng, 366, 1, 59, 60, 365, 366, -1, -306, -365, -366))
	}
	if (f != recur.Weekly || wild) && rng.IntN(4) == 0 {
		r.ByMonthDay = someInts(rng, 3, signed(rng, 31, 1, 28, 29, 30, 31, -1, -2, -30, -31))
	}
	if rng.IntN(3) == 0 {
		numbered := (f == recur.Monthly || f == recur.Yearly && len(r.ByWeekNo) == 0 || wild && f != recur.Yearly) && rng.IntN(2) == 0
		for range 1 + rng.IntN(3) {
			e := recur.WeekdayNum{Day: recur.Weekday(rng.IntN(7))}
			if numbered && rng.IntN(4) > 0 {
				e.N = 1 + rng.IntN(5)
				if f == recur.Yearly && rng.IntN(3) == 0 {
					e.N = pick(rng, 1+rng.IntN(53), 52, 53)
				}
				if rng.IntN(2) == 0 {
					e.N = -e.N
				}
			}
			if !slices.Contains(r.ByDay, e) {
				r.ByDay = append(r.ByDay, e)
			}
		}
	}
	if rng.IntN(5) == 0 {
		r.ByHour = someInts(rng, 3, func() int { return rng.IntN(24) })
	}
	if rng.IntN(5) == 0 {
		r.ByMinute = someInts(rng, 3, func() int { return pick(rng, 0, 15, 30, 45, rng.IntN(60)) })
	}
	if rng.IntN(6) == 0 {
		r.BySecond = someInts(rng, 3, func() int { return pick(rng, 0, 30, rng.IntN(60)) })
	}
	other := len(r.ByMonth)+len(r.ByWeekNo)+len(r.ByYearDay)+len(r.ByMonthDay)+len(r.ByDay)+len(r.ByHour)+len(r.ByMinute)+len(r.BySecond) > 0
	if (other || wild) && rng.IntN(4) == 0 {
		r.BySetPos = someInts(rng, 2, signed(rng, 6, 1, 2, 3, -1, -2, 366, -366, 60))
	}
	end := c.start.AddDate(windowYears[f], 0, 0)
	switch rng.IntN(8) {
	case 0, 1, 2:
		r.Count = 1 + rng.IntN(pick(rng, 3, 10, 60))
	case 3, 4:
		span := fromCivil(end).Sub(fromCivil(c.start))
		u := fromCivil(c.start).Add(time.Duration(rng.Int64N(int64(span)/pick[int64](rng, 1, 4, 40)+1)) - time.Duration(rng.IntN(2))*time.Hour)
		if raw, _ := newRef(*r, c.start, c.allDay).expand(fromCivil(end), 20); len(raw) > 0 && rng.IntN(2) == 0 {
			// "If the value specified by UNTIL is synchronized with the
			// specified recurrence, this DATE or DATE-TIME becomes the last
			// instance of the recurrence."
			u = raw[rng.IntN(len(raw))]
		}
		r.Until = recur.Until{DateTime: toCivil(u)}
		switch {
		case c.allDay || rng.IntN(15) == 0:
			// A DATE UNTIL; on a DATE-TIME DTSTART that breaks a MUST.
			r.Until = recur.Until{DateTime: civil.DateTime{Date: r.Until.Date}, IsDate: true}
		case rng.IntN(10) == 0:
			r.Until.UTC = true // floating series read as UTC (erratum 4414)
		}
	}
	c.invalid = breaksMustNot(c.rule) != ""
	return c
}

// ---- The comparison ----

func collectN(seq func(func(civil.DateTime) bool), max int) []civil.DateTime {
	var out []civil.DateTime
	for t := range seq {
		out = append(out, t)
		if len(out) > max {
			break
		}
	}
	return out
}

func window(ts []time.Time, from, to time.Time) []civil.DateTime {
	var out []civil.DateTime
	for _, t := range ts {
		if !t.Before(from) && t.Before(to) {
			out = append(out, toCivil(t))
		}
	}
	return out
}

// diffMismatch describes one disagreement between the library and ref.
type diffMismatch struct {
	what      string // which call
	got, want []civil.DateTime
}

func (m *diffMismatch) String() string {
	i := 0
	for i < len(m.got) && i < len(m.want) && m.got[i] == m.want[i] {
		i++
	}
	at := func(s []civil.DateTime) string {
		if i < len(s) {
			return s[i].String()
		}
		return "(end)"
	}
	return fmt.Sprintf("%s: %d instances, want %d; first difference at #%d: got %s, want %s",
		m.what, len(m.got), len(m.want), i, at(m.got), at(m.want))
}

const diffMax = 100 // instances compared per rule, at most

// compareCase runs one case through the library and ref. A non-nil result
// is the first disagreement; n is how many instances of the Set it compared.
func compareCase(c diffCase, cache *recur.Cache) (bad *diffMismatch, n int) {
	x := newRef(c.rule, c.start, c.allDay)
	end := fromCivil(c.start.AddDate(windowYears[c.rule.Freq], 0, 0))
	raw, complete := x.expand(end, diffMax)
	if !complete {
		end = raw[diffMax]
		raw = raw[:diffMax]
	}
	wantSet, wantRule := x.finish(raw, true), x.finish(raw, false)
	start := fromCivil(c.start)

	set := &recur.Set{Start: c.start, Rules: []recur.Rule{c.rule}, AllDay: c.allDay, Cache: cache}
	n = len(wantSet)
	if got, want := collectN(set.Between(c.start, toCivil(end)), 2*diffMax), window(wantSet, start, end); !slices.Equal(got, want) {
		return &diffMismatch{what: fmt.Sprintf("Set.Between(DTSTART, %s)", toCivil(end)), got: got, want: want}, n
	}

	// Windows that start after DTSTART, so the engine seeks.
	rule := c.rule
	if c.allDay {
		// A Rule has no DATE form; the Set ignores these parts for a DATE
		// DTSTART, so the Rule alone must not have them.
		rule.ByHour, rule.ByMinute, rule.BySecond = nil, nil, nil
	}
	rng := rand.New(rand.NewPCG(c.seed, 7))
	span := int64(end.Sub(start) / time.Second)
	for range 3 {
		a, b := rng.Int64N(span+1), rng.Int64N(span+1)
		if a > b {
			a, b = b, a
		}
		if rng.IntN(3) == 0 { // whole days, where all-day instances are
			a, b = a/86400*86400, b/86400*86400+86400
		}
		from, to := start.Add(time.Duration(a)*time.Second), start.Add(time.Duration(b)*time.Second)
		if to.After(end) {
			to = end
		}
		if got, want := collectN(rule.Between(c.start, toCivil(from), toCivil(to)), 2*diffMax), window(wantRule, from, to); !slices.Equal(got, want) {
			return &diffMismatch{what: fmt.Sprintf("Rule.Between(DTSTART, %s, %s)", toCivil(from), toCivil(to)), got: got, want: want}, n
		}
		if got, want := collectN(set.Between(toCivil(from), toCivil(to)), 2*diffMax), window(wantSet, from, to); !slices.Equal(got, want) {
			return &diffMismatch{what: fmt.Sprintf("Set.Between(%s, %s)", toCivil(from), toCivil(to)), got: got, want: want}, n
		}
	}
	return nil, n
}

// minimize shrinks a failing case to one that still fails and can't be
// shrunk further by dropping a part, a list value or a number.
func minimize(c diffCase) diffCase {
	fails := func(c diffCase) bool { bad, _ := compareCase(c, nil); return bad != nil }
	for changed := true; changed; {
		changed = false
		try := func(mod func(r *recur.Rule)) {
			d := c
			d.rule = cloneRule(c.rule)
			mod(&d.rule)
			if !d.rule.Equal(c.rule) && fails(d) {
				c, changed = d, true
			}
		}
		try(func(r *recur.Rule) { r.Count = 0 })
		try(func(r *recur.Rule) { r.Until = recur.Until{} })
		try(func(r *recur.Rule) { r.Interval = 0 })
		try(func(r *recur.Rule) { r.WeekStart = recur.Monday })
		for _, l := range []func(r *recur.Rule) *[]int{
			func(r *recur.Rule) *[]int { return &r.BySetPos },
			func(r *recur.Rule) *[]int { return &r.BySecond },
			func(r *recur.Rule) *[]int { return &r.ByMinute },
			func(r *recur.Rule) *[]int { return &r.ByHour },
			func(r *recur.Rule) *[]int { return &r.ByMonthDay },
			func(r *recur.Rule) *[]int { return &r.ByYearDay },
			func(r *recur.Rule) *[]int { return &r.ByWeekNo },
			func(r *recur.Rule) *[]int { return &r.ByMonth },
		} {
			try(func(r *recur.Rule) { *l(r) = nil })
			for i := range len(*l(&c.rule)) {
				try(func(r *recur.Rule) {
					if i < len(*l(r)) {
						*l(r) = slices.Delete(*l(r), i, i+1)
					}
				})
			}
		}
		try(func(r *recur.Rule) { r.ByDay = nil })
		for i := range c.rule.ByDay {
			try(func(r *recur.Rule) {
				if i < len(r.ByDay) {
					r.ByDay = slices.Delete(r.ByDay, i, i+1)
				}
			})
			try(func(r *recur.Rule) {
				if i < len(r.ByDay) {
					r.ByDay[i].N = 0
				}
			})
		}
		if !c.allDay && c.start.Time != (civil.Time{}) {
			d := c
			d.start.Time = civil.Time{}
			if fails(d) {
				c, changed = d, true
			}
		}
	}
	return c
}

func cloneRule(r recur.Rule) recur.Rule {
	r.BySecond, r.ByMinute, r.ByHour = slices.Clone(r.BySecond), slices.Clone(r.ByMinute), slices.Clone(r.ByHour)
	r.ByDay, r.ByMonthDay, r.ByYearDay = slices.Clone(r.ByDay), slices.Clone(r.ByMonthDay), slices.Clone(r.ByYearDay)
	r.ByWeekNo, r.ByMonth, r.BySetPos = slices.Clone(r.ByWeekNo), slices.Clone(r.ByMonth), slices.Clone(r.BySetPos)
	return r
}

// TestConformanceDifferential compares the engine with the reference on
// random rules across all seven frequencies. -recur.conformance=N sets how
// many.
func TestConformanceDifferential(t *testing.T) {
	n := *conformanceRules
	if n == 0 {
		n = 3000
		if testing.Short() {
			n = 300
		}
	}
	const seed = 5545
	cache := recur.NewCache(0)
	type result struct {
		c        diffCase
		bad      *diffMismatch
		validate string // Validate disagreeing with breaksMustNot
		n        int    // instances compared
	}
	results := make([]result, n)
	var wg sync.WaitGroup
	next := make(chan int)
	for range runtime.GOMAXPROCS(0) {
		wg.Go(func() {
			for i := range next {
				c := genCase(seed, i)
				var ca *recur.Cache
				if i%2 == 1 {
					ca = cache
				}
				results[i].c = c
				results[i].bad, results[i].n = compareCase(c, ca)
				if (c.rule.Validate() == nil) == c.invalid {
					results[i].validate = fmt.Sprintf("Validate() = %v; RFC: %q", c.rule.Validate(), breaksMustNot(c.rule))
				}
			}
		})
	}
	for i := range n {
		next <- i
	}
	close(next)
	wg.Wait()

	var bad, badInvalid, perFreq [8]int
	shown, validateBad, invalid, several, instances := 0, 0, 0, 0, 0
	for _, r := range results {
		perFreq[r.c.rule.Freq]++
		instances += r.n
		if r.n > 1 {
			several++
		}
		if r.c.invalid {
			invalid++
		}
		if r.validate != "" {
			if validateBad++; validateBad <= 5 {
				t.Errorf("case %d: RRULE:%v: %s", r.c.seed, r.c.rule, r.validate)
			}
		}
		if r.bad == nil {
			continue
		}
		if r.c.invalid {
			badInvalid[r.c.rule.Freq]++
		} else {
			bad[r.c.rule.Freq]++
		}
		if shown++; shown <= 15 {
			m := minimize(r.c)
			mbad, _ := compareCase(m, nil)
			t.Errorf("case %d (invalid=%v): %v\n\t%s\n\tminimized: %v\n\t%s",
				r.c.seed, r.c.invalid, r.c, r.bad, m, mbad)
		}
	}
	var sb strings.Builder
	for f := recur.Secondly; f <= recur.Yearly; f++ {
		fmt.Fprintf(&sb, " %v %d/%d/%d", f, perFreq[f], bad[f], badInvalid[f])
	}
	t.Logf("%d rules, %d breaking a MUST NOT, %d with more than DTSTART, %d instances compared", n, invalid, several, instances)
	t.Logf("FREQ rules/mismatches/mismatches breaking a MUST NOT:%s", sb.String())
}
