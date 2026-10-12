package ical

import (
	"container/heap"
	"iter"
	"slices"
	"strings"
	"time"

	"github.com/aanelson/iCalendar/civil"
	"github.com/aanelson/iCalendar/recur"
)

// An Occurrence is one instance of an event, to-do or journal entry.
type Occurrence struct {
	// Component is the component the instance comes from: the recurring
	// master, or the override (the component with the same UID and a
	// RECURRENCE-ID) that replaced it.
	Component *Component
	// Start and End bound the instance. End equals Start for an instance
	// with no duration.
	Start, End DateTime
	// RecurrenceID is the start the recurrence rules gave this instance,
	// which identifies it (RFC 5545 §3.8.4.4) even after an override moves
	// it. For a component that doesn't recur it is Start.
	RecurrenceID DateTime
}

// An Expander expands a calendar's recurring components into occurrences.
// The zero value is ready to use.
type Expander struct {
	// Cache, if not nil, is shared by every expansion; see recur.Cache.
	Cache *recur.Cache
	// Floating is the zone floating times and dates are read in when
	// comparing them with the window and ordering occurrences. Nil means
	// UTC. Occurrences keep their floating form.
	Floating *time.Location
	// Components names the component kinds to expand. Nil means VEVENT
	// only.
	Components []string
}

// Occurrences returns cal's occurrences that overlap [from, to), sorted by
// start, with c.Occurrences's semantics and the zero Expander.
func (cal *Calendar) Occurrences(from, to time.Time) iter.Seq2[Occurrence, error] {
	var x Expander
	return x.Occurrences(cal, from, to)
}

// Occurrences returns the occurrences of cal's components that overlap
// [from, to), sorted by start.
//
// An occurrence overlaps the window when it starts before to and ends
// after from; one with no duration overlaps when it starts in the window
// (the CalDAV time-range test, RFC 4791 §9.9).
//
// For each recurring component it expands the recurrence set (see
// Item.Recurrence), gives each instance the master's duration (exact for
// DTEND, nominal for DURATION, RFC 5545 §3.8.5.3, or its own for an RDATE
// PERIOD), and replaces instances that an override's RECURRENCE-ID names
// with the override, which is yielded where its own DTSTART puts it. When
// several overrides name one instance, the highest SEQUENCE wins.
//
// A component that can't be expanded (an unparseable RRULE, an unknown
// TZID) yields one Occurrence naming it with the error, and expansion
// carries on with the rest.
func (x *Expander) Occurrences(cal *Calendar, from, to time.Time) iter.Seq2[Occurrence, error] {
	return func(yield func(Occurrence, error) bool) {
		floating := x.Floating
		if floating == nil {
			floating = time.UTC
		}
		kinds := x.Components
		if kinds == nil {
			kinds = []string{"VEVENT"}
		}
		var streams streamHeap
		defer func() {
			for _, s := range streams {
				s.close()
			}
		}()
		for _, g := range groupByUID(cal, kinds) {
			s, err := x.newStream(cal, g, from, to, floating)
			if err != nil && !yield(Occurrence{Component: g.master()}, err) {
				return
			}
			if s != nil && s.advance() {
				streams = append(streams, s)
			}
		}
		heap.Init(&streams)
		for len(streams) > 0 {
			s := streams[0]
			if !yield(s.head, nil) {
				return
			}
			if s.advance() {
				heap.Fix(&streams, 0)
			} else {
				heap.Pop(&streams).(*stream).close()
			}
		}
	}
}

// A group is the components sharing one UID: at most one master and any
// number of overrides. Components with no UID each form their own group.
type group struct {
	masters   []*Component
	overrides []*Component
}

func (g *group) master() *Component {
	if len(g.masters) > 0 {
		return g.masters[0]
	}
	return g.overrides[0]
}

func groupByUID(cal *Calendar, kinds []string) []*group {
	var order []*group
	byUID := map[string]*group{}
	for _, c := range cal.Children {
		if !slices.ContainsFunc(kinds, func(k string) bool { return strings.EqualFold(k, c.Name) }) {
			continue
		}
		uid := textOf(c, "UID")
		g := byUID[c.Name+"\x00"+uid]
		if g == nil || uid == "" {
			g = &group{}
			order = append(order, g)
			if uid != "" {
				byUID[c.Name+"\x00"+uid] = g
			}
		}
		if c.Get("RECURRENCE-ID") != nil {
			g.overrides = append(g.overrides, c)
		} else {
			g.masters = append(g.masters, c)
		}
	}
	return order
}

// A stream yields one group's occurrences in the window, in order: the
// master's instances not replaced by an override, merged with the
// overrides.
type stream struct {
	floating *time.Location
	from, to time.Time

	master   *Component
	start    DateTime           // the master's DTSTART
	duration Duration           // the master's duration
	single   *Occurrence        // a master that doesn't recur
	periods  map[int64]DateTime // RDATE PERIOD ends, by start instant
	replaced map[int64]bool     // RECURRENCE-IDs (Unix seconds) an override replaces
	it       *recur.Iterator

	overrides  []Occurrence // sorted by start; yielded from here
	pending    *Occurrence  // the master's next instance, not yet yielded
	masterDone bool
	head       Occurrence
	key        time.Time // head's start as an instant, for ordering
}

func (x *Expander) newStream(cal *Calendar, g *group, from, to time.Time, floating *time.Location) (*stream, error) {
	s := &stream{floating: floating, from: from, to: to, replaced: map[int64]bool{}}
	var firstErr error
	keep := func(err error) {
		if firstErr == nil && err != nil {
			firstErr = err
		}
	}

	// Two masters with one UID is invalid, but feeds that keep every
	// revision of an event do it: the highest SEQUENCE is the current one.
	var master Event
	for _, c := range g.masters {
		if m := (Event{Item{c, cal}}); master.Component == nil || m.Sequence() >= master.Sequence() {
			master = m
		}
	}
	// The master's recurrence set, read once. Its EXDATEs, by instant and
	// by day on the master's clock, also drop overrides that a newer
	// master cancelled. A floating series expands on the wall clock the
	// window is read in.
	exAt, exDay := map[int64]bool{}, map[civil.Date]bool{}
	var mstart DateTime
	var set *recur.Set
	if master.Component != nil {
		mstart, _ = master.Start()
		var err error
		set, err = master.recurrence(floating)
		keep(err)
	}
	if set != nil {
		loc := set.Location
		if loc == nil {
			loc = floating
		}
		for _, d := range set.ExDates {
			exAt[d.In(loc).Unix()] = true
			if set.AllDay {
				exDay[d.Date] = true
			}
		}
		for _, t := range set.ExTimes {
			exAt[t.Unix()] = true
		}
		for _, d := range set.ExDays {
			exDay[d] = true
		}
	}

	// Overrides: the latest SEQUENCE for each RECURRENCE-ID.
	best := map[int64]Event{}
	for _, c := range g.overrides {
		ov := Event{Item{c, cal}}
		rid, _, err := ov.RecurrenceID()
		if err != nil && (rid.IsZero() || rid.unresolved()) {
			keep(err)
			continue
		}
		k := rid.In(floating).Unix()
		day := onClockOf(mstart, rid, floating).Date
		if master.Component != nil && (exAt[k] || exDay[day]) && ov.Sequence() < master.Sequence() {
			// The master was revised after this override and now
			// excludes the instance (as python-recurring-ical-events
			// reads such feeds).
			continue
		}
		if prev, ok := best[k]; !ok || ov.Sequence() >= prev.Sequence() {
			best[k] = ov
		}
	}
	for k, ov := range best {
		s.replaced[k] = true
		start, err := ov.Start()
		if err != nil && (start.IsZero() || start.unresolved()) {
			keep(err)
			continue
		}
		_, end, err := ov.span(start)
		keep(err)
		rid, _, _ := ov.RecurrenceID()
		occ := Occurrence{Component: ov.Component, Start: start, End: end, RecurrenceID: rid}
		if s.overlaps(occ) {
			s.overrides = append(s.overrides, occ)
		}
	}
	slices.SortFunc(s.overrides, func(a, b Occurrence) int { return a.Start.In(floating).Compare(b.Start.In(floating)) })

	if master.Component == nil {
		return s, firstErr
	}
	s.master = master.Component
	start, err := master.Start()
	if err != nil && (start.IsZero() || start.unresolved()) {
		keep(err)
		return s, firstErr
	}
	s.start = start
	// A DTEND or DUE gives an exact duration and a DURATION a nominal one
	// (§3.8.5.3); either way Add applies it as it is.
	d, end, err := master.span(start)
	keep(err)
	s.duration = d
	if !master.IsRecurring() {
		occ := Occurrence{Component: s.master, Start: start, End: end, RecurrenceID: start}
		if !s.replaced[start.In(floating).Unix()] && s.overlaps(occ) {
			s.single = &occ
		}
		return s, firstErr
	}
	if set == nil {
		return s, firstErr
	}
	set.Cache = x.Cache
	if set.Location == nil {
		set.Location = floating // a DATE series
	}
	// RDATE periods carry their own ends.
	for p := range master.Props.All("RDATE") {
		if !strings.EqualFold(p.Params.Get("VALUE"), "PERIOD") {
			continue
		}
		ps, err := p.Periods(master.resolver())
		keep(err)
		for _, per := range ps {
			if !per.EndTime().Time().After(per.Start.Time()) {
				continue // err says why; the instance keeps the series' length
			}
			if s.periods == nil {
				s.periods = map[int64]DateTime{}
			}
			s.periods[per.Start.In(floating).Unix()] = per.EndTime()
		}
	}
	// Instances that start before the window can still reach into it.
	s.it = set.TimeIterator(from.Add(-s.longest()), to)
	return s, firstErr
}

// longest returns the longest an instance of the master can last: its
// duration or a longer RDATE period, and a DST change.
func (s *stream) longest() time.Duration {
	n := s.duration.Approx()
	for st, end := range s.periods {
		n = max(n, end.In(s.floating).Sub(time.Unix(st, 0)))
	}
	return max(n, 0) + 2*time.Hour
}

func (s *stream) overlaps(o Occurrence) bool {
	start, end := o.Start.In(s.floating), o.End.In(s.floating)
	if !end.After(start) {
		return !start.Before(s.from) && start.Before(s.to)
	}
	return start.Before(s.to) && end.After(s.from)
}

// nextInstance returns the master's next instance in the window that no
// override replaces.
func (s *stream) nextInstance() (Occurrence, bool) {
	if s.single != nil {
		o := *s.single
		s.single = nil
		return o, true
	}
	if s.it == nil {
		return Occurrence{}, false
	}
	for {
		wall, at, ok := s.it.Next()
		if !ok {
			return Occurrence{}, false
		}
		start := s.start
		start.DateTime = wall
		if start.IsDate {
			start.DateTime.Time = civil.Time{}
		} else if start.Location() != nil {
			start = start.withInstant(at)
		}
		key := start.In(s.floating).Unix()
		if s.replaced[key] {
			continue
		}
		end := start.Add(s.duration)
		if e, ok := s.periods[key]; ok {
			end = e
		}
		o := Occurrence{Component: s.master, Start: start, End: end, RecurrenceID: start}
		if s.overlaps(o) {
			return o, true
		}
		if !start.In(s.floating).Before(s.to) {
			return Occurrence{}, false
		}
	}
}

// advance loads the stream's next occurrence into head.
func (s *stream) advance() bool {
	if s.pending == nil && !s.masterDone {
		if inst, ok := s.nextInstance(); ok {
			s.pending = &inst
		} else {
			s.masterDone = true
		}
	}
	switch {
	case s.pending != nil && (len(s.overrides) == 0 || !s.overrides[0].Start.In(s.floating).Before(s.pending.Start.In(s.floating))):
		s.head = *s.pending
		s.pending = nil
	case len(s.overrides) > 0:
		s.head = s.overrides[0]
		s.overrides = s.overrides[1:]
	default:
		return false
	}
	s.key = s.head.Start.In(s.floating)
	return true
}

func (s *stream) close() {
	if s.it != nil {
		s.it.Close()
	}
}

// streamHeap orders streams by their head occurrence's start.
type streamHeap []*stream

func (h streamHeap) Len() int           { return len(h) }
func (h streamHeap) Less(i, j int) bool { return h[i].key.Before(h[j].key) }
func (h streamHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *streamHeap) Push(x any)        { *h = append(*h, x.(*stream)) }
func (h *streamHeap) Pop() any {
	old := *h
	s := old[len(old)-1]
	*h = old[:len(old)-1]
	return s
}
