package recur_test

// A differential check of recurrence sets in a time zone (RFC 5545 §3.8.5
// with §3.3.5 and erratum 4271): the reference's wall-clock instances,
// resolved to instants by a naive resolver written from §3.3.5, joined with
// RDATEs and less EXDATEs as instants, against Set.Times. Starts are put
// next to real offset changes, including Lord Howe's half hour, São Paulo's
// midnight gaps and Apia's skipped day.

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/aanelson/iCalendar/civil"
	"github.com/aanelson/iCalendar/recur"
)

// wallOf returns t's wall clock in its location, as a UTC time.
func wallOf(t time.Time) time.Time {
	y, m, d := t.Date()
	h, mi, s := t.Clock()
	return time.Date(y, m, d, h, mi, s, 0, time.UTC)
}

// offsetsNear returns the UTC offsets, in seconds, loc uses within two
// days of the instant u.
func offsetsNear(u time.Time, loc *time.Location) []int {
	var out []int
	for h := -48; h <= 48; h++ {
		if _, off := u.Add(time.Duration(h) * time.Hour).In(loc).Zone(); !slices.Contains(out, off) {
			out = append(out, off)
		}
	}
	return out
}

// resolve returns the instant wall-clock time w names in loc, by §3.3.5:
// "If ... the local time described occurs more than once ..., the
// DATE-TIME value refers to the first occurrence of the referenced time"
// and "If the local time described does not occur ..., the DATE-TIME value
// is interpreted using the UTC offset before the gap in local times". A nil
// loc is floating, read as UTC.
func resolve(w time.Time, loc *time.Location) time.Time {
	if loc == nil {
		return w
	}
	var first time.Time
	for _, off := range offsetsNear(w, loc) {
		u := w.Add(-time.Duration(off) * time.Second)
		if wallOf(u.In(loc)).Equal(w) && (first.IsZero() || u.Before(first)) {
			first = u
		}
	}
	if !first.IsZero() {
		return first
	}
	// In a gap: find where the wall clock jumps over w, minute by minute.
	for u := w.Add(-16 * time.Hour); u.Before(w.Add(16 * time.Hour)); u = u.Add(time.Minute) {
		a, b := wallOf(u.In(loc)), wallOf(u.Add(time.Minute).In(loc))
		if a.Before(w) && b.After(w) && b.Sub(a) > time.Minute {
			_, before := u.In(loc).Zone()
			return w.Add(-time.Duration(before) * time.Second)
		}
	}
	panic(fmt.Sprintf("no instant for %v in %v", w, loc))
}

var setZones = []string{"America/New_York", "Europe/London", "Australia/Lord_Howe", "America/Sao_Paulo", "Pacific/Apia", "Asia/Kolkata", ""}

// changeDays returns the dates in year y whose local noon has a different
// offset from the next day's.
func changeDays(loc *time.Location, y int) []time.Time {
	var out []time.Time
	for d := mkDay(y, 1, 1); d.Year() == y; d = d.AddDate(0, 0, 1) {
		_, a := time.Date(d.Year(), d.Month(), d.Day(), 12, 0, 0, 0, loc).Zone()
		_, b := time.Date(d.Year(), d.Month(), d.Day()+1, 12, 0, 0, 0, loc).Zone()
		if a != b {
			out = append(out, d.AddDate(0, 0, 1))
		}
	}
	return out
}

type setCase struct {
	zone            string
	loc             *time.Location
	start           civil.DateTime
	allDay          bool
	rule            recur.Rule
	rdates, exdates []civil.DateTime
	rtimes, extimes []time.Time
}

func (c setCase) String() string {
	return fmt.Sprintf("TZID=%s DTSTART:%s (all day %v) RRULE:%v RDATE:%v EXDATE:%v RTIMES:%v EXTIMES:%v",
		c.zone, c.start, c.allDay, c.rule, c.rdates, c.exdates, c.rtimes, c.extimes)
}

var setSpan = map[recur.Frequency]time.Duration{
	recur.Minutely: 3 * 24 * time.Hour, recur.Hourly: 6 * 24 * time.Hour, recur.Daily: 60 * 24 * time.Hour,
	recur.Weekly: 365 * 24 * time.Hour, recur.Monthly: 3 * 365 * 24 * time.Hour, recur.Yearly: 20 * 365 * 24 * time.Hour,
}

func genSetCase(rng *rand.Rand, locs map[string]*time.Location) setCase {
	c := setCase{zone: setZones[rng.IntN(len(setZones))]}
	c.loc = locs[c.zone]
	y := 2000 + rng.IntN(26)
	day := mkDay(y, time.Month(1+rng.IntN(12)), 1+rng.IntN(28))
	if c.loc != nil {
		if ds := changeDays(c.loc, y); len(ds) > 0 {
			day = ds[rng.IntN(len(ds))]
		}
		if c.zone == "Pacific/Apia" && rng.IntN(2) == 0 {
			day = mkDay(2011, 12, 30) // a day Samoa skipped
		}
	}
	day = day.AddDate(0, 0, -rng.IntN(3))
	start := day.Add(time.Duration(pick(rng, 0, 1, 2, 3, rng.IntN(24)))*time.Hour + time.Duration(pick(rng, 0, 15, 30, 45, rng.IntN(60)))*time.Minute)
	c.start = toCivil(start)
	r := &c.rule
	r.Freq = pick(rng, recur.Minutely, recur.Minutely, recur.Hourly, recur.Hourly, recur.Hourly, recur.Daily, recur.Daily, recur.Daily, recur.Weekly, recur.Monthly, recur.Yearly)
	switch r.Freq {
	case recur.Minutely:
		r.Interval = pick(rng, 15, 20, 25, 30, 45, 90)
	case recur.Hourly:
		r.Interval = 1 + rng.IntN(3)
	default:
		r.Interval = 1 + rng.IntN(2)
	}
	if r.Freq >= recur.Daily && rng.IntN(8) == 0 {
		c.allDay = true
		c.start.Time = civil.Time{}
	}
	if !c.allDay && rng.IntN(3) == 0 {
		r.ByHour = someInts(rng, 3, func() int { return rng.IntN(5) })
	}
	if !c.allDay && r.Freq >= recur.Hourly && rng.IntN(3) == 0 {
		r.ByMinute = someInts(rng, 2, func() int { return pick(rng, 0, 15, 30, 45) })
	}
	if r.Freq == recur.Monthly && rng.IntN(2) == 0 {
		r.ByDay = []recur.WeekdayNum{{N: pick(rng, 1, 2, -1), Day: recur.Sunday}}
	}
	if r.Freq == recur.Yearly && rng.IntN(2) == 0 {
		r.ByMonth = []int{int(c.start.Month)}
		r.ByDay = []recur.WeekdayNum{{N: pick(rng, 1, 2, -1), Day: recur.Sunday}}
	}
	if r.Freq >= recur.Daily && rng.IntN(4) == 0 {
		r.BySetPos = []int{pick(rng, 1, -1)}
		if len(r.ByHour) == 0 {
			r.ByHour = []int{0, 1, 2, 3}
		}
	}

	span := setSpan[r.Freq]
	walls, _ := newRef(*r, c.start, c.allDay).expand(start.Add(span), 60)
	at := func(w time.Time) time.Time {
		if c.allDay {
			w = mkDay(w.Year(), w.Month(), w.Day())
		}
		return resolve(w, c.loc)
	}
	switch rng.IntN(5) {
	case 0, 1:
		r.Count = 1 + rng.IntN(30)
	case 2:
		// UNTIL in DTSTART's form: a DATE for a DATE, UTC with a zone,
		// floating without one (§3.3.10, erratum 4414). Half the time
		// exactly at an instance.
		w := start.Add(time.Duration(rng.Int64N(int64(span)))).Truncate(time.Second)
		if len(walls) > 0 && rng.IntN(2) == 0 {
			w = walls[rng.IntN(len(walls))]
		}
		switch {
		case c.allDay:
			r.Until = recur.Until{DateTime: civil.DateTime{Date: toCivil(w).Date}, IsDate: true}
		case c.loc != nil:
			r.Until = recur.Until{DateTime: toCivil(at(w).UTC()), UTC: true}
		default:
			r.Until = recur.Until{DateTime: toCivil(w)}
		}
	}
	near := func() time.Time {
		if len(walls) > 0 && rng.IntN(2) == 0 {
			return walls[rng.IntN(len(walls))]
		}
		return start.Add(time.Duration(rng.Int64N(int64(span)/4)) - 2*time.Hour).Truncate(15 * time.Minute)
	}
	for range rng.IntN(3) {
		c.rdates = append(c.rdates, toCivil(near().Add(time.Duration(rng.IntN(3))*time.Hour)))
	}
	for range rng.IntN(2) {
		// A DATE series has DATE RDATEs, so its RTimes are midnights (the
		// Set puts any other RTime on its day, as AllDay documents).
		u := at(near())
		if !c.allDay {
			u = u.Add(time.Duration(rng.IntN(3)) * time.Hour)
		}
		c.rtimes = append(c.rtimes, u)
	}
	for range rng.IntN(4) {
		c.exdates = append(c.exdates, toCivil(near()))
	}
	for range rng.IntN(3) {
		c.extimes = append(c.extimes, at(near()))
	}
	if c.allDay {
		// DATE values for a DATE series.
		for i := range c.rdates {
			c.rdates[i].Time = civil.Time{}
		}
		for i := range c.exdates {
			c.exdates[i].Time = civil.Time{}
		}
	}
	return c
}

// compareSetCase returns a description of the first disagreement, or "".
func compareSetCase(c setCase, cache *recur.Cache) string {
	x := newRef(c.rule, c.start, c.allDay)
	start := fromCivil(c.start)
	end := start.Add(setSpan[c.rule.Freq])
	raw, complete := x.expand(end, diffMax)
	if !complete {
		end = raw[diffMax]
		raw = raw[:diffMax]
	}
	at := func(w time.Time) time.Time { return resolve(w, c.loc) }

	// The rule's instances on the wall clock, DTSTART first and counted;
	// UNTIL bounds each instance in its own form, a UTC UNTIL as an instant
	// (erratum 4414: with a TZID, the instances are converted to UTC).
	var walls []time.Time
	walls = append(walls, start)
	for _, w := range raw {
		if w.Equal(start) {
			continue
		}
		if u := c.rule.Until; u.UTC && c.loc != nil {
			if at(w).After(fromCivil(u.DateTime)) {
				continue
			}
		} else if !untilOK(u, w) {
			continue
		}
		walls = append(walls, w)
	}
	if n := c.rule.Count; n > 0 && len(walls) > n {
		walls = walls[:n]
	}
	// "The final recurrence set is generated by gathering all of the start
	// DATE-TIME values generated by any of the specified "RRULE" and
	// "RDATE" properties, and then excluding any start DATE-TIME values
	// specified by "EXDATE" properties." Values are compared as the
	// instants they name; duplicates count once.
	var want []time.Time
	for _, w := range walls {
		want = append(want, at(w))
	}
	for _, r := range c.rdates {
		want = append(want, at(fromCivil(r)))
	}
	want = append(want, c.rtimes...)
	want = slices.DeleteFunc(want, func(u time.Time) bool {
		for _, e := range c.exdates {
			if at(fromCivil(e)).Equal(u) {
				return true
			}
		}
		return slices.ContainsFunc(c.extimes, u.Equal)
	})
	slices.SortFunc(want, time.Time.Compare)
	want = slices.CompactFunc(want, time.Time.Equal)

	// Every wall clock at or after end is at or after end less the
	// largest offset in use, so before that the model is complete.
	to := end
	if c.loc != nil {
		to = end.Add(-time.Duration(slices.Max(offsetsNear(end, c.loc))) * time.Second)
	}
	from := at(start).Add(-30 * time.Hour)
	set := &recur.Set{Start: c.start, Location: c.loc, Rules: []recur.Rule{c.rule}, AllDay: c.allDay,
		RDates: c.rdates, ExDates: c.exdates, RTimes: c.rtimes, ExTimes: c.extimes, Cache: cache}
	rng := rand.New(rand.NewPCG(uint64(start.Unix()), 1))
	for i := range 3 {
		a, b := from, to
		if i > 0 { // a window inside, so the iterator seeks
			span := int64(to.Sub(from)/time.Minute) + 1
			p, q := rng.Int64N(span), rng.Int64N(span)
			a, b = from.Add(time.Duration(min(p, q))*time.Minute), from.Add(time.Duration(max(p, q))*time.Minute)
		}
		var got, exp []string
		for tm := range set.Times(a, b) {
			if got = append(got, tm.UTC().Format(time.RFC3339)); len(got) > 3*diffMax {
				break
			}
		}
		for _, u := range want {
			if !u.Before(a) && u.Before(b) {
				exp = append(exp, u.UTC().Format(time.RFC3339))
			}
		}
		if !slices.Equal(got, exp) {
			return fmt.Sprintf("Times(%v, %v):\n\t got %v\n\twant %v", a.UTC().Format(time.RFC3339), b.UTC().Format(time.RFC3339), got, exp)
		}
	}
	return ""
}

func TestConformanceSetDifferential(t *testing.T) {
	locs := map[string]*time.Location{}
	for _, z := range setZones {
		if z == "" {
			continue
		}
		loc, err := time.LoadLocation(z)
		if err != nil {
			t.Skipf("no zone data: %v", err)
		}
		locs[z] = loc
	}
	n := 1000
	if testing.Short() {
		n = 100
	}
	if *conformanceRules > 0 {
		n = *conformanceRules / 3
	}
	bad := 0
	cache := recur.NewCache(0)
	for i := range n {
		c := genSetCase(rand.New(rand.NewPCG(4271, uint64(i))), locs)
		ca := cache
		if i%2 == 0 {
			ca = nil
		}
		if d := compareSetCase(c, ca); d != "" {
			if bad++; bad <= 10 {
				t.Errorf("case %d: %v\n%s", i, c, d)
			}
		}
	}
	t.Logf("%d sets, %d mismatches", n, bad)
}
