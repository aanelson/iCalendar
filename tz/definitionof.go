package tz

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/aanelson/iCalendar/civil"
	"github.com/aanelson/iCalendar/recur"
)

// DefinitionOf describes loc as a VTIMEZONE (RFC 5545 §3.6.5), for a
// calendar that writes times in loc: every TZID a calendar uses needs a
// VTIMEZONE, and ical.NewTimezone writes one from the result.
//
// Compiled back with Compile, the definition shows loc's offset and
// abbreviation at every instant from `from` on: transition by transition
// through the year Horizon, and after it by the zone's yearly rules, when
// it ends in rules an RRULE can give that it has followed for at least 28
// years (as every zone with daylight saving time in the IANA database
// does). Any other zone keeps its last offset.
//
// Transitions that recur on one yearly rule (the second Sunday in March at
// 02:00) make one observance with an RRULE, ended by a UTC UNTIL at its
// last onset when the zone stopped following it. Other transitions are
// listed by DTSTART and RDATE. The rules in force at `from` are traced
// back to where they began, so the definition only changes with `from`
// when `from` crosses a change of rules: from any time since 2007,
// America/New_York is the two rules calendar programs write for it.
//
// DAYLIGHT is the higher offset of a daylight saving change, as calendar
// programs expect, including for zones the IANA database gives negative
// daylight saving time (Europe/Dublin's winter, in some builds of it).
//
// The definition's TZID is loc's name. A calendar that uses another name
// for the zone sets TZID to it.
//
// Results are cached by loc and where its period at `from` starts, so a
// feed that describes the same zone on every request walks it once.
func DefinitionOf(loc *time.Location, from time.Time) (Definition, error) {
	if loc == nil {
		return Definition{}, errors.New("tz: DefinitionOf a nil location")
	}
	if y := from.UTC().Year(); y < 1 || y > 9999 {
		return Definition{}, fmt.Errorf("tz: DefinitionOf %s from %v: outside the years iCalendar can write", loc, from)
	}
	from = time.Unix(from.Unix(), 0)
	end := time.Date(min(max(Horizon, from.UTC().Year()+30), 9998)+1, 1, 1, 0, 0, 0, 0, time.UTC)

	// The definition depends only on loc, its first onset and end. The
	// entry holds loc, so its address can't be reused while it's cached.
	first := lastChange(loc, from)
	start := first.Unix()
	if first.IsZero() {
		start = initialOnset(loc, from).at
	}
	key := fmt.Sprintf("%p %d %d", loc, start, end.Unix())
	if d, ok := described.get(key); ok && d.loc == loc {
		return d.def.clone(), nil
	}
	def, err := describe(loc, from, first, end)
	if err != nil {
		return Definition{}, err
	}
	described.put(key, describedZone{loc, def})
	return def.clone(), nil
}

// A describedZone is a cached result of DefinitionOf.
type describedZone struct {
	loc *time.Location
	def Definition
}

// clone returns a copy of def that shares no memory with it.
func (def Definition) clone() Definition {
	def.Observances = slices.Clone(def.Observances)
	for i := range def.Observances {
		o := &def.Observances[i]
		o.RDates = slices.Clone(o.RDates)
		o.Rules = slices.Clone(o.Rules)
		for j := range o.Rules {
			r := &o.Rules[j]
			r.BySecond, r.ByMinute, r.ByHour = slices.Clone(r.BySecond), slices.Clone(r.ByMinute), slices.Clone(r.ByHour)
			r.ByDay, r.ByMonthDay, r.ByYearDay = slices.Clone(r.ByDay), slices.Clone(r.ByMonthDay), slices.Clone(r.ByYearDay)
			r.ByWeekNo, r.ByMonth, r.BySetPos = slices.Clone(r.ByWeekNo), slices.Clone(r.ByMonth), slices.Clone(r.BySetPos)
		}
	}
	return def
}

// describe is DefinitionOf, given the change that starts from's period
// (zero if there is none) and where to stop walking.
func describe(loc *time.Location, from, first, end time.Time) (Definition, error) {
	var onsets []onset
	traced := !first.IsZero()
	if traced {
		onsets = append(onsets, onsetAt(loc, first))
	} else {
		onsets = append(onsets, initialOnset(loc, from))
		first = from
	}
	for t := first; ; {
		if t = nextChange(loc, t, end); t.IsZero() {
			break
		}
		if len(onsets) == maxOnsets {
			return Definition{}, fmt.Errorf("tz: %s changes more than %d times after %v", loc, maxOnsets, from)
		}
		onsets = append(onsets, onsetAt(loc, t))
	}
	runs := groupRuns(onsets)

	// Trace the runs in force at `from` back to where they began, while
	// each earlier transition continues one of them.
	for t, n := first, len(onsets); traced && n < maxOnsets; n++ {
		prev := lastChange(loc, t.Add(-time.Second))
		if prev.IsZero() {
			break
		}
		o := onsetAt(loc, prev)
		if o.wall.Year < 1 || !extendBack(runs, o) {
			break
		}
		t = prev
	}
	slices.SortFunc(runs, func(a, b *run) int { return cmp.Compare(a.onsets[0].at, b.onsets[0].at) })

	def := Definition{TZID: loc.String()}
	oneOffs := map[observanceKey]int{} // index in def.Observances
	for _, r := range runs {
		o := r.onsets[0]
		if abs(o.from.offset) >= 86400 || abs(o.to.offset) >= 86400 {
			return Definition{}, fmt.Errorf("tz: %s has a UTC offset of a day or more, which iCalendar can't write", loc)
		}
		ob := Observance{Daylight: o.to.dst, Start: o.wall, OffsetFrom: o.from.offset, OffsetTo: o.to.offset, Name: o.to.name}
		if len(r.onsets) > 1 {
			ob.Rules = []recur.Rule{r.rule(end)}
			def.Observances = append(def.Observances, ob)
			continue
		}
		// Onsets that recur on no rule share an observance when they change
		// the same way, listed by RDATE.
		k := observanceKey{o.from.offset, o.to.offset, o.to.name, o.to.dst}
		if i, ok := oneOffs[k]; ok {
			def.Observances[i].RDates = append(def.Observances[i].RDates, o.wall)
			continue
		}
		oneOffs[k] = len(def.Observances)
		def.Observances = append(def.Observances, ob)
	}
	return def, nil
}

// A zoneState is what a zone shows at an instant.
type zoneState struct {
	offset int
	name   string
	dst    bool
}

func stateAt(loc *time.Location, t time.Time) zoneState {
	t = t.In(loc)
	name, offset := t.Zone()
	return zoneState{offset, name, t.IsDST()}
}

// An onset is a transition: at the Unix second at, the zone goes from
// showing one state to showing another. wall is that instant on the wall
// clock in force before it, as an observance's DTSTART and RDATEs give it.
type onset struct {
	at       int64
	from, to zoneState
	wall     civil.DateTime
}

func onsetAt(loc *time.Location, t time.Time) onset {
	from, to := stateAt(loc, t.Add(-time.Second)), stateAt(loc, t)
	// The IANA database's negative daylight saving time (Europe/Dublin's
	// winter GMT, Africa/Casablanca's +00 in Ramadan) flags the lower offset
	// of a change as DST. Calendar programs expect DAYLIGHT to be the higher
	// one, so such a change is read the other way round when the zone then
	// changes back, as daylight saving time does. A change for good, such
	// as out of DST to a new and higher standard time, keeps its flags.
	if to.dst != from.dst && to.offset != from.offset && to.dst == (to.offset < from.offset) {
		if next := nextChange(loc, t, t.AddDate(2, 0, 0)); !next.IsZero() && stateAt(loc, next) == from {
			to.dst = !to.dst
		}
	}
	return onset{at: t.Unix(), from: from, to: to, wall: civil.DateTimeOfEpochSecond(t.Unix() + int64(from.offset))}
}

// initialOnset stands for the state loc has shown since before any
// transition, for a definition that starts before its first one. An
// observance needs a DTSTART, so it changes nothing at the start of 1970,
// or of the day `from` falls on when that is earlier.
func initialOnset(loc *time.Location, from time.Time) onset {
	st := stateAt(loc, from)
	day := civil.DateOf(from.In(loc))
	if epoch := (civil.Date{Year: 1970, Month: time.January, Day: 1}); epoch.Before(day) {
		day = epoch
	}
	wall := civil.DateTime{Date: day}
	return onset{at: wall.EpochSecond() - int64(st.offset), from: st, to: st, wall: wall}
}

// nextChange returns the first instant after t, and before end, at which
// loc shows a different offset, abbreviation or DST flag, or the zero Time
// if there is none.
//
// It steps by ZoneBounds, which is exact over a zone's explicit
// transitions. Past them, where the time package computes transitions from
// the zone's POSIX TZ rule, it also gives the start of each UTC year as a
// bound, and on the last day of a leap year gives an end that doesn't
// advance (it takes a year to be 365 days). Bounds where nothing changes
// are stepped over, and one that doesn't advance is replaced by a day's
// step, searched for a change.
func nextChange(loc *time.Location, t, end time.Time) time.Time {
	cur := stateAt(loc, t)
	for t.Before(end) {
		_, next := t.In(loc).ZoneBounds()
		switch {
		case next.IsZero():
			return time.Time{}
		case !next.After(t):
			next = t.Add(24 * time.Hour)
			if stateAt(loc, next) != cur {
				next = firstChange(loc, t, next, cur)
			}
		}
		if stateAt(loc, next) != cur {
			if next.Before(end) {
				return next
			}
			return time.Time{}
		}
		t = next
	}
	return time.Time{}
}

// lastChange returns the latest instant at or before t at which loc's
// offset, abbreviation or DST flag changed, or the zero Time if it never
// did. Like nextChange, it steps over bounds where nothing changes.
func lastChange(loc *time.Location, t time.Time) time.Time {
	cur := stateAt(loc, t)
	for {
		start, _ := t.In(loc).ZoneBounds()
		if start.IsZero() {
			return time.Time{}
		}
		if start.After(t) {
			// Not something the time package does; step back a day instead.
			start = t.Add(-24 * time.Hour)
			if was := stateAt(loc, start); was != cur {
				return firstChange(loc, start, t, was)
			}
		}
		before := start.Add(-time.Second)
		if stateAt(loc, before) != cur {
			return start
		}
		t = before
	}
}

// firstChange returns the first second in (lo, hi] at which loc doesn't
// show cur, given that it shows cur at lo and something else at hi.
func firstChange(loc *time.Location, lo, hi time.Time, cur zoneState) time.Time {
	a, b := lo.Unix(), hi.Unix()
	for b-a > 1 {
		m := a + (b-a)/2
		if stateAt(loc, time.Unix(m, 0)) == cur {
			a = m
		} else {
			b = m
		}
	}
	return time.Unix(b, 0)
}

// A run is a series of onsets of one kind in consecutive years that one
// yearly rule gives: an observance with an RRULE. A run of one onset is a
// one-off.
type run struct {
	key    runKey
	onsets []onset
	rules  []yearly // the rules that give every onset so far, best first
}

// A runKey is what the onsets an RRULE generates have in common.
type runKey struct {
	offFrom, offTo int
	name           string
	dst            bool
	clock          civil.Time // on the wall clock before the onset
}

// An observanceKey is what the onsets one STANDARD or DAYLIGHT lists have
// in common.
type observanceKey struct {
	offFrom, offTo int
	name           string
	dst            bool
}

func (o onset) key() runKey {
	return runKey{o.from.offset, o.to.offset, o.to.name, o.to.dst, o.wall.Time}
}

// groupRuns sorts onsets, in time order, into runs. Each onset joins a run
// of its kind that had an onset the year before, if one of the run's rules
// also gives this one, or else starts a run.
func groupRuns(onsets []onset) []*run {
	var runs, open []*run
	for _, o := range onsets {
		k, year := o.key(), o.wall.Year
		var into *run
		for _, r := range open {
			if r.key == k && r.onsets[len(r.onsets)-1].wall.Year == year-1 {
				if fit := fitting(r.rules, o.wall.Date); len(fit) > 0 {
					into, r.rules = r, fit
					break
				}
			}
		}
		if into == nil {
			into = &run{key: k, rules: candidates(o.wall.Date)}
			runs = append(runs, into)
			open = append(open, into)
		}
		into.onsets = append(into.onsets, o)
		// Only a run with an onset this year or last can take another.
		open = slices.DeleteFunc(open, func(r *run) bool { return r.onsets[len(r.onsets)-1].wall.Year < year-1 })
	}
	return runs
}

// extendBack adds o, the transition just before every onset in runs, to
// the start of the earliest run of its kind, if it falls the year before
// that run's first onset and one of the run's rules gives it too.
func extendBack(runs []*run, o onset) bool {
	k := o.key()
	var r *run
	for _, c := range runs {
		if c.key == k && (r == nil || c.onsets[0].at < r.onsets[0].at) {
			r = c
		}
	}
	if r == nil || r.onsets[0].wall.Year != o.wall.Year+1 {
		return false
	}
	fit := fitting(r.rules, o.wall.Date)
	if len(fit) == 0 {
		return false
	}
	r.rules = fit
	r.onsets = slices.Insert(r.onsets, 0, o)
	return true
}

// minOpenYears is how many years a run must cover before its rule is
// trusted to go on past the end of the walk: enough for its dates to fall
// on every day of the week, so the rules that fit it all give the same
// dates.
const minOpenYears = 28

// rule returns the RRULE that gives a run of two or more onsets: without
// an end when the zone has followed it for minOpenYears and still does at
// end, where the walk stopped, and otherwise ending with a UTC UNTIL equal
// to its last onset, as §3.6.5 requires.
func (r *run) rule(end time.Time) recur.Rule {
	first, last := r.onsets[0], r.onsets[len(r.onsets)-1]
	rule := r.rules[0].recur()
	if d, ok := r.rules[0].dateIn(last.wall.Year + 1); ok && last.wall.Year-first.wall.Year+1 >= minOpenYears {
		next := civil.DateTime{Date: d, Time: last.wall.Time}.EpochSecond() - int64(r.key.offFrom)
		if next >= end.Unix() {
			return rule
		}
	}
	rule.Until = recur.UntilTime(time.Unix(last.at, 0))
	return rule
}

// A yearly rule gives one date a year.
type yearly struct {
	kind yearlyKind
	// month and day are where the rule's 7-day window starts, or its
	// date; for a yearDay rule, day is the day of the year.
	month time.Month
	day   int
	wday  time.Weekday // for the weekday kinds
}

type yearlyKind int

// The kinds of yearly rule, in the order they're preferred when more than
// one gives a run's onsets.
const (
	nthWeekday     yearlyKind = iota // BYMONTH=3;BYDAY=2SU: the window starts on the 1st, 8th, 15th or 22nd
	lastWeekday                      // BYMONTH=10;BYDAY=-1SU
	weekdayInMonth                   // BYMONTH=3;BYDAY=FR;BYMONTHDAY=23,24,25,26,27,28,29
	weekdayAcross                    // BYDAY=FR;BYYEARDAY=-67,-66,-65,-64,-63,-62,-61: October 26 to November 1
	monthDay                         // BYMONTH=3;BYMONTHDAY=21
	yearDay                          // BYYEARDAY=80
)

// candidates returns the yearly rules that give d, best first.
func candidates(d civil.Date) []yearly {
	var c []yearly
	w := d.Weekday()
	if d.Day+7 > civil.DaysIn(d.Year, d.Month) {
		c = append(c, yearly{kind: lastWeekday, month: d.Month, wday: w})
	}
	// The weekday on or after each of the 7 days up to d.
	for back := range 7 {
		s := d.AddDays(-back)
		y := yearly{month: s.Month, day: s.Day, wday: w}
		switch {
		case s.Year != d.Year || s.Month == time.February && s.Day == 29:
			continue
		case s.Day+6 <= shortest(s.Month):
			if s.Month != time.February && s.Day+6 == civil.DaysIn(s.Year, s.Month) {
				continue // the last week, which lastWeekday says better
			}
			y.kind = weekdayInMonth
			if (s.Day-1)%7 == 0 {
				y.kind = nthWeekday
			}
		case s.Month == time.February || s.Month == time.December:
			continue // the window crosses February 29 or the year's end, which no BYYEARDAY can follow
		default:
			y.kind = weekdayAcross
		}
		c = append(c, y)
	}
	if d.Month != time.February || d.Day != 29 {
		c = append(c, yearly{kind: monthDay, month: d.Month, day: d.Day})
	}
	if yd := d.YearDay(); yd <= 365 {
		c = append(c, yearly{kind: yearDay, day: yd})
	}
	slices.SortStableFunc(c, func(a, b yearly) int { return cmp.Compare(a.kind, b.kind) })
	return c
}

// fitting returns the rules that give d.
func fitting(rules []yearly, d civil.Date) []yearly {
	var out []yearly
	for _, y := range rules {
		if got, ok := y.dateIn(d.Year); ok && got == d {
			out = append(out, y)
		}
	}
	return out
}

// shortest returns the fewest days month m has.
func shortest(m time.Month) int { return civil.DaysIn(2001, m) }

// dateIn returns the date y gives in year, and false when it gives none.
func (y yearly) dateIn(year int) (civil.Date, bool) {
	switch y.kind {
	case lastWeekday:
		last := civil.Date{Year: year, Month: y.month, Day: civil.DaysIn(year, y.month)}
		return last.AddDays(-((int(last.Weekday()) - int(y.wday) + 7) % 7)), true
	case monthDay:
		d := civil.Date{Year: year, Month: y.month, Day: y.day}
		return d, d.IsValid()
	case yearDay:
		if y.day > civil.DaysInYear(year) {
			return civil.Date{}, false
		}
		return civil.Date{Year: year, Month: time.January, Day: 1}.AddDays(y.day - 1), true
	}
	start := civil.Date{Year: year, Month: y.month, Day: y.day}
	if !start.IsValid() {
		return civil.Date{}, false
	}
	return start.AddDays((int(y.wday) - int(start.Weekday()) + 7) % 7), true
}

// recur returns y as an RRULE, for an observance whose DTSTART is a date
// y gives.
func (y yearly) recur() recur.Rule {
	r := recur.Rule{Freq: recur.Yearly}
	day := recur.WeekdayOf(y.wday)
	switch y.kind {
	case nthWeekday:
		r.ByMonth, r.ByDay = []int{int(y.month)}, []recur.WeekdayNum{{N: (y.day-1)/7 + 1, Day: day}}
	case lastWeekday:
		r.ByMonth, r.ByDay = []int{int(y.month)}, []recur.WeekdayNum{{N: -1, Day: day}}
	case weekdayInMonth:
		r.ByMonth, r.ByDay, r.ByMonthDay = []int{int(y.month)}, []recur.WeekdayNum{{Day: day}}, week(y.day)
	case weekdayAcross:
		// A day of the year names the same date every year when counted
		// from the start up to February 28, and from the end after it.
		first := civil.Date{Year: 2001, Month: y.month, Day: y.day}.YearDay()
		if y.month > time.February {
			first -= 366 // December 31 is -1
		}
		r.ByDay, r.ByYearDay = []recur.WeekdayNum{{Day: day}}, week(first)
	case monthDay:
		r.ByMonth, r.ByMonthDay = []int{int(y.month)}, []int{y.day}
	case yearDay:
		r.ByYearDay = []int{y.day}
	}
	return r
}

// week returns the 7 numbers from first.
func week(first int) []int {
	days := make([]int, 7)
	for i := range days {
		days[i] = first + i
	}
	return days
}
