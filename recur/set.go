package recur

import (
	"cmp"
	"iter"
	"slices"
	"time"

	"github.com/aanelson/iCalendar/civil"
)

// A Set is a recurrence set (RFC 5545 §3.8.5): DTSTART, every instance of
// the RRULEs, and the RDATEs, less the EXDATEs and the instances of any
// EXRULE (an RFC 2445 property RFC 5545 deprecates but which still appears
// in the wild).
//
// Start is always the first instance, whether or not the rules generate it
// (§3.8.5.3), and counts toward each rule's COUNT. The rules are evaluated
// on Start's wall clock. RDATEs and EXDATEs come in two forms: wall-clock
// times on Start's clock (RDates, ExDates) and instants (RTimes, ExTimes),
// for values written in UTC or another zone. Instances are matched against
// EXDATEs, and de-duplicated, by instant, so an EXDATE excludes an instance
// even when a DST change moves it, and an RTime can name the second of two
// identical wall-clock readings when clocks go back.
//
// A Set's fields may be read concurrently by any number of iterations, but
// must not change while one is running.
type Set struct {
	Start civil.DateTime
	// Location is the zone of Start, used to compare a UTC UNTIL and to
	// resolve instances to instants. Nil means floating: wall-clock times
	// read as UTC.
	Location *time.Location
	Rules    []Rule
	ExRules  []Rule
	RDates   []civil.DateTime
	ExDates  []civil.DateTime
	RTimes   []time.Time
	ExTimes  []time.Time
	// ExDays excludes every instance on these dates, Start's wall clock.
	// It holds DATE EXDATEs on a DATE-TIME series, which RFC 5545 doesn't
	// allow but some producers (Google among them) write.
	ExDays []civil.Date
	// AllDay marks Start as a DATE (VALUE=DATE). The rules' BYHOUR,
	// BYMINUTE and BYSECOND are then ignored, as RFC 5545 §3.3.10
	// requires, and every instance falls at midnight.
	AllDay bool
	// Cache, if not nil, is shared by the set's rules; see Cache.
	Cache *Cache

	// omitStart leaves Start out unless a rule or RDATE produces it, the
	// way python-dateutil's rruleset reads a set. Tests use it.
	omitStart bool
}

// All returns every instance of the set in order.
func (s *Set) All() iter.Seq[civil.DateTime] {
	return s.Between(civil.DateTimeOfEpochSecond(-lastSecond), civil.DateTimeOfEpochSecond(lastSecond+1))
}

// Between returns the set's instances whose wall-clock times are in
// [from, to), in order, seeking past everything before from as
// Rule.Between does.
func (s *Set) Between(from, to civil.DateTime) iter.Seq[civil.DateTime] {
	return func(yield func(civil.DateTime) bool) {
		it := s.Iterator(from, to)
		defer it.Close()
		for {
			wall, _, ok := it.Next()
			if !ok || !yield(wall) {
				return
			}
		}
	}
}

// Times returns the instants of the set's instances in [from, to), in
// order, in Location (UTC when it is nil). Wall-clock instances are
// resolved as civil.DateTime.In does.
func (s *Set) Times(from, to time.Time) iter.Seq[time.Time] {
	return func(yield func(time.Time) bool) {
		it := s.TimeIterator(from, to)
		defer it.Close()
		for {
			_, at, ok := it.Next()
			if !ok || !yield(at) {
				return
			}
		}
	}
}

// An Iterator walks a Set's instances one at a time, for callers that
// merge several sets or interleave them with other work. Ranging over
// Set.Between or Set.Times is simpler when that isn't needed.
//
// Instances come in the order of their instants. That is the order of
// their wall-clock times too, except near a DST change, where a time in
// the gap resolves to after the times that follow it.
//
// Call Close when done, so what the walk learned reaches the Set's Cache.
type Iterator struct {
	set     *Set
	loc     *time.Location // nil: floating, instants are wall clock as UTC
	sources []*source
	exRules []*source
	exAt    map[int64]bool // excluded instants
	exDays  map[int64]bool // excluded epoch days, on the wall clock
	iters   []*ruleIter

	// The window: by wall clock (Iterator) or by instant (TimeIterator).
	byInstant        bool
	wallFrom, wallTo int64
	instFrom, instTo int64

	pending itemHeap // pulled, not yet released, by instant
	bound   int64    // every item not yet pulled is at or after this instant
	drained bool
	lastAt  int64
	hasLast bool
	closed  bool
}

// An item is one instance: its wall-clock time and its instant, both as
// epoch seconds.
type item struct {
	wall, at int64
}

// Iterator returns an Iterator over the set's instances with wall-clock
// times in [from, to).
func (s *Set) Iterator(from, to civil.DateTime) *Iterator {
	return s.iterator(from.EpochSecond(), to.EpochSecond(), false, 0, 0)
}

// TimeIterator returns an Iterator over the set's instances whose instants
// are in [from, to).
func (s *Set) TimeIterator(from, to time.Time) *Iterator {
	loc := s.Location
	if loc == nil {
		loc = time.UTC
	}
	// Instants and wall-clock times are out of step by a zone's offset and
	// its transitions; a day of slack covers any.
	lo := civil.DateTimeOf(from.In(loc)).EpochSecond() - 86400
	hi := civil.DateTimeOf(to.In(loc)).EpochSecond() + 86400
	return s.iterator(lo, hi, true, from.Unix(), to.Unix())
}

// Next returns the next instance as a wall-clock time on Start's clock and
// as an instant in the set's Location, or false when there are none left.
func (it *Iterator) Next() (civil.DateTime, time.Time, bool) {
	x, ok := it.next()
	if !ok {
		return civil.DateTime{}, time.Time{}, false
	}
	loc := it.loc
	if loc == nil {
		loc = time.UTC
	}
	return civil.DateTimeOfEpochSecond(x.wall), time.Unix(x.at, 0).In(loc), true
}

// Close ends the walk. It is safe to call more than once.
func (it *Iterator) Close() {
	if it.closed {
		return
	}
	it.closed = true
	for _, r := range it.iters {
		r.close()
	}
}

func (s *Set) iterator(from, to int64, byInstant bool, instFrom, instTo int64) *Iterator {
	it := &Iterator{
		set: s, loc: s.Location, exAt: map[int64]bool{},
		byInstant: byInstant, wallFrom: from, wallTo: to, instFrom: instFrom, instTo: instTo,
		bound: -1 << 62,
	}
	if !s.Start.IsValid() || from >= to {
		it.drained = true
		return it
	}
	start := s.Start
	start.Nanosecond = 0
	if s.AllDay {
		start.Time = civil.Time{}
		// The rules may produce several times on one day, all of which
		// mean that day: walk whole days and filter after flooring.
		from = floorDiv(from, 86400) * 86400
		to = ceilDiv(to, 86400) * 86400
	}
	first := start.EpochSecond()
	var listed []item
	if first >= from && first < to && !s.omitStart {
		listed = append(listed, it.item(first))
	}
	for _, d := range s.RDates {
		if d.IsValid() {
			if w := d.EpochSecond(); w >= from && w < to {
				listed = append(listed, it.item(w))
			}
		}
	}
	for _, t := range s.RTimes {
		w := it.wallOf(t)
		if w >= from && w < to {
			listed = append(listed, item{wall: w, at: t.Unix()})
		}
	}
	slices.SortFunc(listed, func(a, b item) int {
		return cmp.Or(cmp.Compare(a.wall, b.wall), cmp.Compare(a.at, b.at))
	})
	it.sources = append(it.sources, &source{list: listed})
	ruleFrom := max(from, first)
	for _, r := range s.Rules {
		ri := s.ruleIter(r, start, ruleFrom, to, !s.omitStart)
		it.iters = append(it.iters, ri)
		it.sources = append(it.sources, &source{rule: ri})
	}
	for _, r := range s.ExRules {
		// An EXRULE is read as the rule alone: Start is excluded only when
		// the EXRULE itself generates it.
		ri := s.ruleIter(r, start, ruleFrom, to, false)
		it.iters = append(it.iters, ri)
		it.exRules = append(it.exRules, &source{rule: ri})
	}
	for _, d := range s.ExDates {
		if d.IsValid() {
			it.exAt[it.resolve(d.EpochSecond())] = true
		}
	}
	for _, t := range s.ExTimes {
		it.exAt[t.Unix()] = true
	}
	for _, d := range s.ExDays {
		if it.exDays == nil {
			it.exDays = map[int64]bool{}
		}
		it.exDays[d.EpochDay()] = true
	}
	return it
}

func (s *Set) ruleIter(r Rule, start civil.DateTime, from, to int64, countStart bool) *ruleIter {
	it := newRuleIter(r, start, iterOpts{
		loc:        s.Location,
		cache:      s.Cache,
		countStart: countStart,
		allDay:     s.AllDay,
	})
	it.seek(from)
	it.setEnd(to)
	return it
}

// resolve returns the instant of a wall-clock time on the set's clock.
func (it *Iterator) resolve(wall int64) int64 {
	if it.loc == nil {
		return wall
	}
	return civil.DateTimeOfEpochSecond(wall).In(it.loc).Unix()
}

func (it *Iterator) item(wall int64) item {
	return item{wall: wall, at: it.resolve(wall)}
}

// wallOf returns the wall-clock time of an instant on the set's clock.
func (it *Iterator) wallOf(t time.Time) int64 {
	loc := it.loc
	if loc == nil {
		loc = time.UTC
	}
	return civil.DateTimeOf(t.In(loc)).EpochSecond()
}

// shift returns how far resolving x moved it forward, past a DST gap: zero
// for every time a zone doesn't skip.
func (it *Iterator) shift(x item) int64 {
	if it.loc == nil {
		return 0
	}
	_, off := time.Unix(x.at, 0).In(it.loc).Zone()
	return x.at - (x.wall - int64(off))
}

// pull returns the next instance in wall-clock order, before
// de-duplication and EXDATEs: the earliest head of any source that no
// EXRULE generates.
func (it *Iterator) pull() (item, bool) {
	for {
		best := -1
		for i, src := range it.sources {
			if x, ok := src.peek(); ok && (best < 0 || x.wall < it.sources[best].head.wall ||
				x.wall == it.sources[best].head.wall && x.at < it.sources[best].head.at) {
				best = i
			}
		}
		if best < 0 {
			return item{}, false
		}
		x := it.sources[best].head
		it.sources[best].ok = false
		// Sources end at the window themselves: rule iterators stop there
		// and listed dates were filtered when the iterator was built.
		if excluded(it.exRules, x.wall) {
			continue
		}
		if it.sources[best].rule != nil {
			x.at = it.resolve(x.wall)
		}
		if it.set.AllDay {
			// A sub-daily rule on a DATE start: its instances are the days
			// it touches.
			x.wall = floorDiv(x.wall, 86400) * 86400
			x.at = it.resolve(x.wall)
		}
		return x, true
	}
}

// next returns the next instance in instant order. Instances are held
// until no later pull can produce an earlier instant: resolving moves a
// time forward only past a DST gap, by the gap's length, so every
// instance after x is at or after x.at - shift(x).
func (it *Iterator) next() (item, bool) {
	if it.closed {
		return item{}, false
	}
	for {
		if len(it.pending) > 0 && (it.drained || it.pending[0].at < it.bound) {
			x := it.pending.pop()
			if it.hasLast && x.at == it.lastAt {
				continue // the same instance from another source
			}
			it.lastAt, it.hasLast = x.at, true
			if it.exAt[x.at] || it.exDays[floorDiv(x.wall, 86400)] {
				continue
			}
			if it.byInstant {
				if x.at < it.instFrom {
					continue
				}
				if x.at >= it.instTo {
					it.Close()
					return item{}, false
				}
			} else if x.wall < it.wallFrom || x.wall >= it.wallTo {
				continue // a DATE start's day floored outside the window
			}
			return x, true
		}
		if it.drained {
			it.Close()
			return item{}, false
		}
		x, ok := it.pull()
		if !ok {
			it.drained = true
			continue
		}
		it.pending.push(x)
		// The bound uses the wall clock's first-occurrence resolution, not
		// x.at: an RTime can name the second of two repeated readings, and
		// the readings after it resolve to before it.
		first := item{wall: x.wall, at: it.resolve(x.wall)}
		it.bound = max(it.bound, first.at-it.shift(first))
	}
}

// excluded reports whether an EXRULE generates the wall-clock time t,
// advancing each EXRULE to t.
func excluded(exRules []*source, t int64) bool {
	hit := false
	for _, src := range exRules {
		if x, ok := src.peek(); ok && x.wall < t {
			// Behind: seek rather than step through every instance, which
			// for a dense EXRULE under a sparse RRULE is millions.
			src.ok = false
			src.rule.skipTo(t)
		}
		for {
			x, ok := src.peek()
			if !ok || x.wall > t {
				break
			}
			src.ok = false
			if x.wall == t {
				hit = true
				break
			}
		}
	}
	return hit
}

// A source is one stream of instances in wall-clock order: a list or a
// rule.
type source struct {
	list []item
	rule *ruleIter
	head item
	ok   bool // head holds the next instance
	done bool
}

func (s *source) peek() (item, bool) {
	if s.ok {
		return s.head, true
	}
	if s.done {
		return item{}, false
	}
	if s.rule != nil {
		var t int64
		t, s.ok = s.rule.next()
		s.head = item{wall: t, at: t}
	} else if len(s.list) > 0 {
		s.head, s.ok = s.list[0], true
		s.list = s.list[1:]
	}
	s.done = !s.ok
	return s.head, s.ok
}

// itemHeap is a min-heap of items by instant, then wall clock. It is typed
// rather than built on container/heap so items aren't boxed: one push and
// pop per instance would otherwise allocate.
type itemHeap []item

func (h itemHeap) less(i, j int) bool {
	if h[i].at != h[j].at {
		return h[i].at < h[j].at
	}
	return h[i].wall < h[j].wall
}

func (h *itemHeap) push(x item) {
	*h = append(*h, x)
	for i := len(*h) - 1; i > 0; {
		parent := (i - 1) / 2
		if !h.less(i, parent) {
			break
		}
		(*h)[i], (*h)[parent] = (*h)[parent], (*h)[i]
		i = parent
	}
}

func (h *itemHeap) pop() item {
	old := *h
	x := old[0]
	n := len(old) - 1
	old[0] = old[n]
	*h = old[:n]
	for i := 0; ; {
		l, r, least := 2*i+1, 2*i+2, i
		if l < n && h.less(l, least) {
			least = l
		}
		if r < n && h.less(r, least) {
			least = r
		}
		if least == i {
			break
		}
		(*h)[i], (*h)[least] = (*h)[least], (*h)[i]
		i = least
	}
	return x
}
