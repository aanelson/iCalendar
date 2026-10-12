package recur

import (
	"cmp"
	"iter"
	"slices"
	"time"

	"github.com/aanelson/iCalendar/civil"
)

// lastSecond is 9999-12-31T23:59:59, the last wall-clock second iCalendar
// can write. No iteration goes past it.
var lastSecond = civil.DateTime{Date: civil.Date{Year: 9999, Month: 12, Day: 31}, Time: civil.Time{Hour: 23, Minute: 59, Second: 59}}.EpochSecond()

// All returns the instances r generates from start, in order: what the
// rule alone produces, at or after start. Unlike a Set, it doesn't add
// start itself when the rule doesn't generate it, which is how
// python-dateutil and rrule-go read a rule; COUNT counts only what the rule
// generates.
//
// The rule is evaluated on start's wall clock. A UTC UNTIL is compared as
// though start were in UTC too; use a Set with a Location for a start in a
// zone. An invalid start yields nothing.
//
// Iteration always ends. Besides COUNT, UNTIL and the year 9999, it stops
// once the rule provably has no further instances: rules that can never
// match (BYMONTH=2;BYMONTHDAY=30) are detected up front, and any other rule
// that goes a whole 400-year calendar cycle without a candidate is known to
// have none left.
func (r Rule) All(start civil.DateTime) iter.Seq[civil.DateTime] {
	return func(yield func(civil.DateTime) bool) {
		it := newRuleIter(r, start, iterOpts{})
		for {
			t, ok := it.next()
			if !ok || !yield(civil.DateTimeOfEpochSecond(t)) {
				it.close()
				return
			}
		}
	}
}

// Between returns the instances of r.All(start) in [from, to): at or after
// from and before to. It starts at the first period that can contain from
// instead of walking from start, so its cost doesn't grow with the distance
// from start to from, except for a COUNT rule, which must count what came
// before (see Cache).
func (r Rule) Between(start, from, to civil.DateTime) iter.Seq[civil.DateTime] {
	return r.between(start, from, to, nil)
}

func (r Rule) between(start, from, to civil.DateTime, cache *Cache) iter.Seq[civil.DateTime] {
	return func(yield func(civil.DateTime) bool) {
		it := newRuleIter(r, start, iterOpts{cache: cache})
		it.seek(from.EpochSecond())
		it.setEnd(to.EpochSecond())
		for {
			t, ok := it.next()
			if !ok || !yield(civil.DateTimeOfEpochSecond(t)) {
				it.close()
				return
			}
		}
	}
}

// A ruleIter walks one rule's instances as wall-clock epoch seconds.
type ruleIter struct {
	p        *plan
	key      string // the cache key, or "" without a cache
	cache    *Cache
	loc      *time.Location
	untilSet bool
	until    Until
	untilAt  int64 // wall-clock second past which nothing is in UNTIL, approximately for UTC
	untilUTC time.Time
	count    int64 // COUNT, or 0

	period   int64  // the next period to expand
	cur      cursor // the instances of the last expanded period
	fresh    bool   // cur hasn't been trimmed to the lower bounds yet
	emitted  int64  // instances at or after DTSTART so far, yielded or not
	from     int64  // yield nothing before this wall-clock second
	end      int64  // yield nothing at or after this one
	lastProd int64  // the last period with a candidate
	lastAt   int64  // the last instance yielded, counted or skipped
	walkFrom int64  // the walk has seen every instance at or after this second
	known    int64  // a cache says nothing is after this wall-clock second
	done     bool
	years    [2]*yearInfo
	found    []checkpoint // checkpoints met this walk, for the cache
	finished bool         // the walk reached the rule's true end
}

// iterOpts are how a rule is being expanded.
type iterOpts struct {
	loc   *time.Location // DTSTART's zone; nil for floating
	cache *Cache
	// countStart counts a DTSTART the rule doesn't generate as its first
	// instance, as RFC 5545 §3.8.5.3 reads COUNT ("The "DTSTART" property
	// value always counts as the first occurrence") and as libical,
	// ical.js and sabre/vobject do.
	countStart bool
	allDay     bool // DTSTART is a DATE
}

func newRuleIter(r Rule, start civil.DateTime, o iterOpts) *ruleIter {
	loc, cache := o.loc, o.cache
	it := &ruleIter{
		loc:      loc,
		count:    int64(r.Count),
		from:     -1 << 62,
		end:      lastSecond + 1,
		lastProd: -1,
		lastAt:   -1 << 62,
		known:    lastSecond,
	}
	if !start.IsValid() {
		it.done = true
		it.p = &plan{empty: true}
		return it
	}
	it.p = compile(r, start, o.allDay)
	it.from = it.p.start
	it.walkFrom = it.p.start
	if !r.Until.IsZero() {
		it.untilSet = true
		it.until = r.Until
		switch {
		case r.Until.IsDate:
			it.untilAt = r.Until.Date.EpochDay()*86400 + 86399
		case r.Until.UTC && loc != nil:
			it.untilUTC = r.Until.DateTime.In(time.UTC)
			// The wall clock of the instant, plus a day for any zone
			// transition in between.
			it.untilAt = civil.DateTimeOf(it.untilUTC.In(loc)).EpochSecond() + 86400
		default:
			it.untilAt = r.Until.EpochSecond()
		}
	}
	if it.p.empty {
		it.done = true
	}
	if o.countStart && it.count > 0 && !it.done && !it.generates(it.p.start) {
		it.emitted = 1
		it.lastAt = it.p.start
		if it.count == 1 {
			it.finish()
		}
	}
	if cache != nil && !it.done {
		it.cache = cache
		it.key = cacheKey(r, start, o)
	}
	return it
}

// generates reports whether the rule's first period produces t.
func (it *ruleIter) generates(t int64) bool {
	it.expand(0)
	probe := it.cur
	probe.skipBelow(t)
	got, ok := probe.pop()
	it.cur.reset()
	return ok && got == t
}

// seek makes the iterator yield nothing before the wall-clock second t,
// starting at the first period that can hold t. A COUNT rule still counts
// everything from DTSTART; with a cache it resumes from a checkpoint.
func (it *ruleIter) seek(t int64) {
	if it.done || t <= it.p.start {
		return
	}
	it.from = t
	target := it.p.periodAt(t)
	if it.count == 0 {
		it.period = target
		it.lastProd = target - 1
		it.walkFrom = max(it.p.start, it.p.periodStart(target))
		if it.cache != nil {
			if end, ok := it.cache.end(it.key); ok {
				it.known = end
				it.done = end < t
			}
		}
		return
	}
	if it.cache == nil {
		return // walk from the start, counting
	}
	cp, end, ok := it.cache.checkpoint(it.key, target)
	if ok {
		// The rule's last instance is known, so COUNT needs no counting:
		// the rule is the same rule ending there.
		it.count = 0
		it.known = end
		it.done = end < t
		it.period = target
		it.lastProd = target - 1
		return
	}
	if cp.period > 0 {
		it.period, it.emitted = cp.period, cp.before
		it.lastProd = cp.period - 1
		it.walkFrom = it.p.periodStart(cp.period)
	}
}

// skipTo moves an iterator already under way past everything before the
// wall-clock second t. A rule without COUNT jumps to t's period; a COUNT
// rule still counts what it passes.
func (it *ruleIter) skipTo(t int64) {
	if it.done || t <= it.from {
		return
	}
	it.from = t
	if it.count == 0 {
		if target := it.p.periodAt(t); target > it.period {
			it.period = target
			it.lastProd = target - 1
			it.walkFrom = max(it.walkFrom, it.p.periodStart(target))
			it.cur.reset()
			it.fresh = false
			return
		}
	}
	// The period already expanded may hold instances before t.
	probe := it.cur
	n := probe.skipBelow(t)
	if it.count > 0 && it.emitted+n >= it.count {
		it.lastAt = it.cur.peekAt(it.count - it.emitted - 1)
		it.emitted = it.count
		it.finish()
		return
	}
	if n > 0 {
		it.lastAt = it.cur.peekAt(n - 1)
	}
	it.cur = probe
	it.emitted += n
}

// setEnd makes the iterator stop before the wall-clock second t.
func (it *ruleIter) setEnd(t int64) {
	it.end = min(it.end, t)
}

// next returns the next instance, or false when there are no more.
func (it *ruleIter) next() (int64, bool) {
	for !it.done {
		if it.fresh {
			it.fresh = false
			it.cur.skipBelow(it.p.start) // before DTSTART: not instances at all
			if it.from > it.p.start {
				probe := it.cur
				n := probe.skipBelow(it.from)
				if it.count > 0 && it.emitted+n >= it.count {
					// The COUNT-th instance comes before from: the rule
					// ends without yielding anything more.
					it.lastAt = it.cur.peekAt(it.count - it.emitted - 1)
					it.emitted = it.count
					it.finish()
					return 0, false
				}
				if n > 0 {
					it.lastAt = it.cur.peekAt(n - 1)
				}
				it.cur = probe
				it.emitted += n
			}
		}
		t, ok := it.cur.pop()
		if !ok {
			it.advance()
			continue
		}
		if it.untilSet && it.pastUntil(t) || t > it.known {
			it.finish()
			return 0, false
		}
		if t >= it.end {
			it.done = true
			return 0, false
		}
		it.emitted++
		it.lastAt = t
		if it.count > 0 && it.emitted == it.count {
			it.finish() // this is the last one
		}
		return t, true
	}
	return 0, false
}

// pastUntil reports whether the instance t is after UNTIL.
func (it *ruleIter) pastUntil(t int64) bool {
	u := it.until
	switch {
	case u.IsDate:
		return floorDiv(t, 86400) > u.Date.EpochDay()
	case u.UTC && it.loc != nil:
		if t < it.untilAt-2*86400 {
			return false
		}
		return civil.DateTimeOfEpochSecond(t).In(it.loc).After(it.untilUTC)
	default:
		return t > it.untilAt
	}
}

// finish records that the walk reached the rule's end.
func (it *ruleIter) finish() {
	it.done = true
	it.finished = true
}

// advance expands the next period with candidates into cur, or marks the
// iterator done.
func (it *ruleIter) advance() {
	p := it.p
	for {
		start := p.periodStart(it.period)
		if start > it.known || it.untilSet && start > it.untilAt {
			it.finish()
			return
		}
		if start >= it.end {
			it.done = true
			return
		}
		if it.period-it.lastProd > p.cycle {
			// A whole calendar cycle without a candidate: none will come.
			it.finish()
			return
		}
		if it.count > 0 && p.freq <= Hourly && it.countDays() {
			continue
		}
		if it.count > 0 && it.cache != nil {
			it.noteCheckpoint()
		}
		next := it.expand(it.period)
		if !it.cur.empty() {
			it.lastProd = it.period
			it.period = next
			it.fresh = true
			return
		}
		it.period = next
	}
}

// countDays moves a sub-daily COUNT rule past whole days that lie before
// from, counting their instances arithmetically rather than expanding each
// hour, minute or second: a rule with a COUNT must count everything before
// the window, and walking a SECONDLY rule second by second would cost time
// proportional to how long ago DTSTART was. It stops before the day the
// COUNT-th instance falls on, and before UNTIL, so the normal walk handles
// both. It reports whether it moved.
func (it *ruleIter) countDays() bool {
	p := it.p
	day := floorDiv(p.periodStart(it.period), 86400)
	if day <= floorDiv(p.start, 86400) || it.period != p.firstPeriodFrom(p.unitOf(day*86400)) {
		return false // DTSTART's own day, or the middle of a day
	}
	limit := floorDiv(it.from, 86400)
	limit = min(limit, floorDiv(it.known, 86400), floorDiv(it.end, 86400))
	if it.untilSet {
		limit = min(limit, floorDiv(it.untilAt, 86400)-2)
	}
	d, lastDay := day, int64(-1<<62)
	for ; d < limit; d++ {
		n := it.dayCount(d)
		if it.emitted+n >= it.count {
			break // the rule ends on this day: walk it
		}
		it.emitted += n
		if n > 0 {
			lastDay = d
		}
	}
	if d == day {
		return false
	}
	it.period = p.firstPeriodFrom(p.unitOf(d * 86400))
	if lastDay > -1<<62 {
		it.lastProd = it.period - 1
		// An upper bound on the last instance counted, for the cache.
		it.lastAt = max(it.lastAt, (lastDay+1)*86400-1)
	}
	return true
}

// dayCount returns how many instances a sub-daily rule has on day d, which
// must be after DTSTART's day: the periods aligned to INTERVAL that start
// on d and pass the time-of-day limits, times each period's instances.
func (it *ruleIter) dayCount(d int64) int64 {
	p := it.p
	if !it.dayOK(d) {
		return 0
	}
	n := p.interval
	r := floorMod(p.u0, n)              // aligned units are those ≡ r (mod n)
	aligned := func(a, b int64) int64 { // aligned units in [a, b)
		return floorDiv(b-1-r, n) - floorDiv(a-1-r, n)
	}
	var periods int64
	for h := int64(0); h < 24; h++ {
		if p.hourMask != 0 && p.hourMask&(1<<h) == 0 {
			continue
		}
		switch p.freq {
		case Hourly:
			periods += aligned(d*24+h, d*24+h+1)
		case Minutely:
			base := (d*24 + h) * 60
			if p.minuteMask == 0 {
				periods += aligned(base, base+60)
				continue
			}
			for m := int64(0); m < 60; m++ {
				if p.minuteMask&(1<<m) != 0 {
					periods += aligned(base+m, base+m+1)
				}
			}
		default: // Secondly
			for m := int64(0); m < 60; m++ {
				if p.minuteMask != 0 && p.minuteMask&(1<<m) == 0 {
					continue
				}
				base := ((d*24+h)*60 + m) * 60
				if p.secondMask == 0 {
					periods += aligned(base, base+60)
					continue
				}
				for s := floorMod(r-base, n); s < 60; s += n {
					if p.secondMask&(1<<s) != 0 {
						periods++
					}
				}
			}
		}
	}
	return periods * p.perPeriod
}

// noteCheckpoint remembers how many instances came before the period about
// to be expanded, every so often.
func (it *ruleIter) noteCheckpoint() {
	if it.period%checkpointEvery == 0 && it.period > 0 {
		if n := len(it.found); n == 0 || it.found[n-1].period < it.period {
			it.found = append(it.found, checkpoint{period: it.period, before: it.emitted})
		}
	}
}

// close hands what the walk learned to the cache.
func (it *ruleIter) close() {
	if it.cache == nil {
		return
	}
	// Finished, the walk proved there is nothing after end: the last
	// instance it met, or the second before it started looking.
	end := max(it.lastAt, it.walkFrom-1)
	it.cache.store(it.key, it.loc, it.found, it.finished, end)
	it.found = nil
}

// year returns the yearInfo for year, building it when needed.
func (it *ruleIter) year(year int) *yearInfo {
	for _, y := range it.years {
		if y != nil && y.year == year {
			return y
		}
	}
	y := newYearInfo(it.p, year)
	it.years[1] = it.years[0]
	it.years[0] = y
	return y
}

// dayOK reports whether the epoch day d passes the day-level parts.
func (it *ruleIter) dayOK(d int64) bool {
	date := civil.DateOfEpochDay(d)
	y := it.year(date.Year)
	return it.p.dayOK(y, int(d-y.firstDay))
}

// expand loads period n's candidates into cur and returns the next period
// worth expanding: usually n+1, further when n showed that a whole day,
// hour or minute can't match.
func (it *ruleIter) expand(n int64) int64 {
	p := it.p
	c := &it.cur
	c.reset()
	u := p.u0 + n*p.interval
	next := n + 1
	switch p.freq {
	case Yearly:
		if p.weekYears {
			for d, end := weekYearStart(int(u), p.wkst), weekYearStart(int(u)+1, p.wkst); d < end; d++ {
				if it.dayOK(d) {
					c.days = append(c.days, d)
				}
			}
			c.times = p.times
			break
		}
		y := it.year(int(u))
		l := y.leapIndex()
		for m := 1; m <= 12; m++ {
			if p.monthMask != 0 && p.monthMask&(1<<m) == 0 {
				continue
			}
			for i := monthStart[l][m-1]; i < monthStart[l][m]; i++ {
				if p.dayOK(y, i) {
					c.days = append(c.days, y.firstDay+int64(i))
				}
			}
		}
		c.times = p.times
	case Monthly:
		year, month := int(floorDiv(u, 12)), int(floorMod(u, 12))+1
		if p.monthMask == 0 || p.monthMask&(1<<month) != 0 {
			y := it.year(year)
			l := y.leapIndex()
			for i := monthStart[l][month-1]; i < monthStart[l][month]; i++ {
				if p.dayOK(y, i) {
					c.days = append(c.days, y.firstDay+int64(i))
				}
			}
		}
		c.times = p.times
	case Weekly:
		first := u*7 - 3 + int64(p.wkst)
		for d := first; d < first+7; d++ {
			if it.dayOK(d) {
				c.days = append(c.days, d)
			}
		}
		c.times = p.times
	case Daily:
		if it.dayOK(u) {
			c.days = append(c.days, u)
			c.times = p.times
		} else {
			next = it.skipMonth(u, next)
		}
	default:
		next = it.expandSubDaily(u, next)
	}
	if len(p.setPos) > 0 {
		c.selectPositions(p.setPos)
	}
	return next
}

// skipMonth returns the period to try after day u failed, jumping to the
// next month when the month itself is excluded.
func (it *ruleIter) skipMonth(day, next int64) int64 {
	p := it.p
	if p.monthMask == 0 {
		return next
	}
	date := civil.DateOfEpochDay(day)
	if p.monthMask&(1<<date.Month) != 0 {
		return next
	}
	first := civil.Date{Year: date.Year, Month: date.Month, Day: 1}.AddDate(0, 1, 0).EpochDay()
	switch p.freq {
	case Daily:
		return max(next, p.firstPeriodFrom(first))
	case Hourly:
		return max(next, p.firstPeriodFrom(first*24))
	case Minutely:
		return max(next, p.firstPeriodFrom(first*1440))
	default:
		return max(next, p.firstPeriodFrom(first*86400))
	}
}

// expandSubDaily loads one HOURLY, MINUTELY or SECONDLY period starting at
// unit u, and returns the next period to try.
func (it *ruleIter) expandSubDaily(u, next int64) int64 {
	p := it.p
	c := &it.cur
	var t int64 // the period's start, wall-clock seconds
	switch p.freq {
	case Hourly:
		t = u * 3600
	case Minutely:
		t = u * 60
	default:
		t = u
	}
	day := floorDiv(t, 86400)
	if !it.dayOK(day) {
		if n := it.skipMonth(day, next); n > next {
			return n
		}
		return max(next, p.firstPeriodFrom(p.unitOf((day+1)*86400)))
	}
	sec := t - day*86400
	h, m, s := sec/3600, sec/60%60, sec%60
	if p.hourMask != 0 && p.hourMask&(1<<h) == 0 {
		return max(next, p.firstPeriodFrom(p.unitOf(day*86400+(h+1)*3600)))
	}
	if p.minuteMask != 0 && p.minuteMask&(1<<m) == 0 {
		return max(next, p.firstPeriodFrom(p.unitOf(day*86400+h*3600+(m+1)*60)))
	}
	if p.secondMask != 0 && p.secondMask&(1<<s) == 0 {
		return next
	}
	c.days = append(c.days, day)
	c.times = p.times
	c.base = int32(sec)
	return next
}

// A cursor walks one period's candidates: every combination of a day in
// days and a time in times (offset by base), in order. With BYSETPOS it
// walks sel instead. The product is never materialized, so a dense period
// (every second of a year) costs no memory.
type cursor struct {
	days   []int64
	times  []int32
	base   int32
	sel    []int64
	useSel bool
	di, ti int
	si     int
}

func (c *cursor) reset() {
	c.days = c.days[:0]
	c.times = nil
	c.base = 0
	c.sel = c.sel[:0]
	c.useSel = false
	c.di, c.ti, c.si = 0, 0, 0
}

func (c *cursor) empty() bool {
	if c.useSel {
		return len(c.sel) == 0
	}
	return len(c.days) == 0 || len(c.times) == 0
}

func (c *cursor) at(di, ti int) int64 {
	return c.days[di]*86400 + int64(c.base) + int64(c.times[ti])
}

func (c *cursor) pop() (int64, bool) {
	if c.useSel {
		if c.si >= len(c.sel) {
			return 0, false
		}
		c.si++
		return c.sel[c.si-1], true
	}
	if c.di >= len(c.days) || len(c.times) == 0 {
		return 0, false
	}
	t := c.at(c.di, c.ti)
	if c.ti++; c.ti == len(c.times) {
		c.ti = 0
		c.di++
	}
	return t, true
}

// skipBelow drops the candidates before t and returns how many it dropped.
func (c *cursor) skipBelow(t int64) int64 {
	if c.useSel {
		i, _ := slices.BinarySearch(c.sel[c.si:], t)
		c.si += i
		return int64(i)
	}
	nt := len(c.times)
	if nt == 0 || c.di >= len(c.days) {
		return 0
	}
	before := c.di*nt + c.ti
	// The first day whose last candidate is at or after t.
	last := int64(c.base) + int64(c.times[nt-1])
	k, _ := slices.BinarySearchFunc(c.days[c.di:], t, func(d, t int64) int {
		return cmp.Compare(d*86400+last, t)
	})
	di, ti := c.di+k, 0
	if k == 0 {
		ti = c.ti
	}
	if di < len(c.days) {
		base := c.days[di]*86400 + int64(c.base)
		j, _ := slices.BinarySearchFunc(c.times[ti:], t, func(x int32, t int64) int {
			return cmp.Compare(base+int64(x), t)
		})
		ti += j
	}
	c.di, c.ti = di, ti
	return int64(c.di*nt+c.ti) - int64(before)
}

// peekAt returns the candidate k places after the next one.
func (c *cursor) peekAt(k int64) int64 {
	if c.useSel {
		return c.sel[c.si+int(k)]
	}
	nt := int64(len(c.times))
	i := int64(c.di)*nt + int64(c.ti) + k
	return c.at(int(i/nt), int(i%nt))
}

// selectPositions replaces the period's candidates with the BYSETPOS picks
// among them, in order and without duplicates.
func (c *cursor) selectPositions(pos []int) {
	total := len(c.days) * len(c.times)
	c.sel = c.sel[:0]
	for _, n := range pos {
		i := n - 1
		if n < 0 {
			i = total + n
		}
		if i >= 0 && i < total {
			c.sel = append(c.sel, c.at(i/len(c.times), i%len(c.times)))
		}
	}
	slices.Sort(c.sel)
	c.sel = slices.Compact(c.sel)
	c.useSel = true
	c.si = 0
}
