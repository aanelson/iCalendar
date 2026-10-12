package ical

import (
	"errors"
	"fmt"
	"iter"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/aanelson/iCalendar/civil"
	"github.com/aanelson/iCalendar/recur"
	"github.com/aanelson/iCalendar/tz"
)

// ErrNoProperty is wrapped by errors from accessors whose property is
// missing and has no default.
var ErrNoProperty = errors.New("ical: property not present")

// A Calendar is a VCALENDAR: the root of the tree.
//
// It resolves the TZIDs its components use (it is a tz.Resolver): its own
// VTIMEZONE with that TZID first, then tz.Load. Resolutions are cached on
// the Calendar, so edits to its VTIMEZONEs after the first lookup aren't
// seen; wrap the component in a new Calendar to start afresh. A Calendar
// may be read from several goroutines at once.
type Calendar struct {
	*Component

	mu    sync.Mutex
	zones map[string]zoneResult
}

type zoneResult struct {
	loc *time.Location
	err error
}

// NewCalendar returns a calendar with the two properties RFC 5545
// requires: PRODID, set to prodID, and VERSION:2.0.
func NewCalendar(prodID string) *Calendar {
	c := &Calendar{Component: NewComponent("VCALENDAR")}
	p := NewProperty("PRODID", "")
	p.SetText(prodID)
	c.Props.Add(p)
	c.Props.Add(NewProperty("VERSION", "2.0"))
	return c
}

// ProdID returns PRODID.
func (c *Calendar) ProdID() string { return textOf(c.Component, "PRODID") }

// Version returns VERSION, normally "2.0".
func (c *Calendar) Version() string { return textOf(c.Component, "VERSION") }

// Method returns METHOD (RFC 5546), or "" for none.
func (c *Calendar) Method() string { return textOf(c.Component, "METHOD") }

// Events returns the calendar's VEVENTs.
func (c *Calendar) Events() iter.Seq[Event] {
	return func(yield func(Event) bool) {
		for child := range c.Components("VEVENT") {
			if !yield(Event{Item{child, c}}) {
				return
			}
		}
	}
}

// Todos returns the calendar's VTODOs.
func (c *Calendar) Todos() iter.Seq[Todo] {
	return func(yield func(Todo) bool) {
		for child := range c.Components("VTODO") {
			if !yield(Todo{Item{child, c}}) {
				return
			}
		}
	}
}

// Journals returns the calendar's VJOURNALs.
func (c *Calendar) Journals() iter.Seq[Journal] {
	return func(yield func(Journal) bool) {
		for child := range c.Components("VJOURNAL") {
			if !yield(Journal{Item{child, c}}) {
				return
			}
		}
	}
}

// Timezones returns the calendar's VTIMEZONEs.
func (c *Calendar) Timezones() iter.Seq[Timezone] {
	return func(yield func(Timezone) bool) {
		for child := range c.Components("VTIMEZONE") {
			if !yield(Timezone{child}) {
				return
			}
		}
	}
}

// Resolve returns the location for tzid, implementing tz.Resolver.
func (c *Calendar) Resolve(tzid string) (*time.Location, error) {
	c.mu.Lock()
	if r, ok := c.zones[tzid]; ok {
		c.mu.Unlock()
		return r.loc, r.err
	}
	c.mu.Unlock()
	loc, err := c.resolve(tzid)
	c.mu.Lock()
	if c.zones == nil {
		c.zones = map[string]zoneResult{}
	}
	c.zones[tzid] = zoneResult{loc, err}
	c.mu.Unlock()
	return loc, err
}

func (c *Calendar) resolve(tzid string) (*time.Location, error) {
	var compileErr error
	for z := range c.Timezones() {
		if z.TZID() != tzid {
			continue
		}
		loc, err := z.Location()
		if err == nil {
			return loc, nil
		}
		compileErr = err
	}
	loc, err := tz.Load(tzid)
	if err != nil && compileErr != nil {
		// The calendar's own definition is the better explanation.
		return nil, compileErr
	}
	return loc, err
}

// A Timezone is a VTIMEZONE.
type Timezone struct {
	*Component
}

// TZID returns the zone's TZID.
func (z Timezone) TZID() string { return textOf(z.Component, "TZID") }

// Location compiles the zone into a *time.Location; see tz.Compile.
func (z Timezone) Location() (*time.Location, error) {
	def, err := z.Definition()
	if err != nil {
		return nil, err
	}
	return tz.Compile(def)
}

// Definition returns the zone's STANDARD and DAYLIGHT observances in the
// form tz.Compile takes.
func (z Timezone) Definition() (tz.Definition, error) {
	def := tz.Definition{TZID: z.TZID()}
	for child := range z.Components("") {
		var o tz.Observance
		switch child.Name {
		case "STANDARD":
		case "DAYLIGHT":
			o.Daylight = true
		default:
			continue
		}
		to := child.Get("TZOFFSETTO")
		if to == nil {
			return def, fmt.Errorf("%w %q: %s has no TZOFFSETTO", tz.ErrInvalidDefinition, def.TZID, child.Name)
		}
		var err error
		if o.OffsetTo, err = to.UTCOffset(); err != nil {
			return def, err
		}
		o.OffsetFrom = o.OffsetTo
		if from := child.Get("TZOFFSETFROM"); from != nil {
			if o.OffsetFrom, err = from.UTCOffset(); err != nil {
				return def, err
			}
		}
		// An observance's times are on its own TZOFFSETFROM wall clock: a
		// TZID on them means nothing, and a UTC time (written by some
		// producers) is converted to that clock.
		onClock := func(d DateTime) civil.DateTime {
			if d.UTC {
				return d.DateTime.AddSeconds(int64(o.OffsetFrom))
			}
			return d.DateTime
		}
		p := child.Get("DTSTART")
		if p == nil {
			return def, fmt.Errorf("%w %q: %s has no DTSTART", tz.ErrInvalidDefinition, def.TZID, child.Name)
		}
		start, err := withoutTZID(p).DateTime(nil)
		if err != nil {
			return def, err
		}
		o.Start = onClock(start)
		if n := child.Get("TZNAME"); n != nil {
			o.Name = n.Text()
		}
		for p := range child.Props.All("RRULE") {
			r, err := p.Recur()
			if err != nil {
				return def, err
			}
			o.Rules = append(o.Rules, r)
		}
		for p := range child.Props.All("RDATE") {
			ds, err := withoutTZID(p).DateTimes(nil)
			if err != nil {
				return def, err
			}
			for _, d := range ds {
				o.RDates = append(o.RDates, onClock(d))
			}
		}
		def.Observances = append(def.Observances, o)
	}
	return def, nil
}

// maxOnsets is the most onsets NewTimezone expands one observance's rule
// to, as many as tz.Compile takes from a whole VTIMEZONE.
const maxOnsets = 20_000

// NewTimezone returns a VTIMEZONE holding def, the inverse of Definition:
// its TZID, then a STANDARD or DAYLIGHT for each observance, with DTSTART
// and RDATEs as local times, RRULEs, TZOFFSETFROM, TZOFFSETTO and TZNAME.
// tz.DefinitionOf describes a *time.Location this way, and
// Calendar.AddTimezones does both for every zone a calendar uses.
//
// What it writes is valid RFC 5545 for any definition it accepts. A rule
// that ends is written with an UNTIL in UTC equal to its last onset, as
// §3.6.5 requires ("the UNTIL DATE-TIME will be equal to the last instance
// generated by the recurrence pattern. It MUST be specified in UTC time"),
// however the definition ends it: a COUNT, or an UNTIL that is floating
// or a DATE (read, like DTSTART, on the TZOFFSETFROM clock). It refuses a
// definition no VTIMEZONE can hold: one with no TZID or no observances, a
// DTSTART or RDATE that isn't a valid date and time, an offset of a day
// or more, or a rule that breaks §3.3.10.
func NewTimezone(def tz.Definition) (Timezone, error) {
	if def.TZID == "" || len(def.Observances) == 0 {
		return Timezone{}, fmt.Errorf("%w %q: a VTIMEZONE needs a TZID and a STANDARD or DAYLIGHT", tz.ErrInvalidDefinition, def.TZID)
	}
	z := NewComponent("VTIMEZONE")
	setText(z, "TZID", def.TZID)
	for _, o := range def.Observances {
		c, err := newObservance(o)
		if err != nil {
			return Timezone{}, fmt.Errorf("%w %q: %w", tz.ErrInvalidDefinition, def.TZID, err)
		}
		z.Add(c)
	}
	return Timezone{z}, nil
}

func newObservance(o tz.Observance) (*Component, error) {
	writable := func(dt civil.DateTime) bool { return dt.IsValid() && dt.Year >= 0 && dt.Year <= 9999 }
	if !writable(o.Start) {
		return nil, fmt.Errorf("DTSTART %v is not a date and time", o.Start)
	}
	if max(o.OffsetFrom, -o.OffsetFrom, o.OffsetTo, -o.OffsetTo) >= 86400 {
		return nil, errors.New("an offset of a day or more")
	}
	c := NewComponent("STANDARD")
	if o.Daylight {
		c.Name = "DAYLIGHT"
	}
	// §3.6.5: "DTSTART in this usage MUST be specified as a date with a
	// local time value", as must RDATE: floating, on the TZOFFSETFROM
	// clock.
	p := NewProperty("DTSTART", "")
	p.SetDateTime(NewFloating(o.Start))
	c.Props.Add(p)
	for _, r := range o.Rules {
		if err := r.Validate(); err != nil {
			return nil, err
		}
		r, err := lastOnsetUntil(o, r)
		if err != nil {
			return nil, err
		}
		p := NewProperty("RRULE", "")
		p.SetRecur(r)
		c.Props.Add(p)
	}
	if len(o.RDates) > 0 {
		vs := make([]DateTime, len(o.RDates))
		for i, d := range o.RDates {
			if !writable(d) {
				return nil, fmt.Errorf("RDATE %v is not a date and time", d)
			}
			vs[i] = NewFloating(d)
		}
		p := NewProperty("RDATE", "")
		p.SetDateTimes(vs...)
		c.Props.Add(p)
	}
	from := NewProperty("TZOFFSETFROM", "")
	from.SetUTCOffset(o.OffsetFrom)
	to := NewProperty("TZOFFSETTO", "")
	to.SetUTCOffset(o.OffsetTo)
	c.Props.Add(from)
	c.Props.Add(to)
	setText(c, "TZNAME", o.Name)
	return c, nil
}

// lastOnsetUntil returns r, if it ends, ending instead with a UTC UNTIL
// at its last onset: r expanded from o's DTSTART on its TZOFFSETFROM
// clock, as tz.Compile expands it.
func lastOnsetUntil(o tz.Observance, r recur.Rule) (recur.Rule, error) {
	if r.Count == 0 && r.Until.IsZero() {
		return r, nil
	}
	clock := time.FixedZone("", o.OffsetFrom)
	set := recur.Set{Start: o.Start, Location: clock, Rules: []recur.Rule{r}}
	if r.Count == 0 && r.Until.UTC {
		// A UTC UNTIL at an onset, as tz.DefinitionOf writes it, is already
		// the last onset; the set seeks straight to it.
		at := civil.DateTimeOf(r.Until.DateTime.In(time.UTC).In(clock))
		for t := range set.Between(at, at.AddSeconds(1)) {
			if t == at {
				return r, nil
			}
		}
	}
	last, n := o.Start, 0
	for t := range set.All() {
		if n++; n > maxOnsets {
			return r, fmt.Errorf("RRULE %s has more than %d onsets", r, maxOnsets)
		}
		last = t
	}
	r.Count, r.Until = 0, recur.UntilTime(last.In(clock))
	return r, nil
}

// AddTimezones adds a VTIMEZONE for each TZID that the calendar's
// properties use and none of its VTIMEZONEs defines, as RFC 5545 requires:
// "An individual "VTIMEZONE" calendar component MUST be specified for each
// unique "TZID" parameter value specified in the iCalendar object"
// (§3.6.5). Each comes from tz.DefinitionOf, for the zone r resolves the
// TZID to, from the earliest time the calendar writes with it, so it gives
// the right offset for every value written with the TZID and for every
// later recurrence instance (see tz.DefinitionOf for how far ahead). They
// go before the calendar's other components, in TZID order, where readers
// that resolve TZIDs as they go need them.
//
// When r is nil, TZIDs resolve with tz.Load. A calendar whose times were
// computed in locations tz.Load might not give back for their names (ones
// from time/tzdata, or built with time.LoadLocationFromTZData) passes a
// resolver that returns those locations, so its VTIMEZONEs agree with
// them.
//
// A TZID that doesn't resolve, or none of whose values can be read, gets
// no VTIMEZONE and is reported in the error; the others are still added.
// The calendar's cached resolutions of the TZIDs it adds are dropped, so
// they resolve to the new VTIMEZONEs from then on.
func (c *Calendar) AddTimezones(r tz.Resolver) error {
	if r == nil {
		r = tz.ResolverFunc(tz.Load)
	}
	defined := map[string]bool{}
	for z := range c.Timezones() {
		defined[z.TZID()] = true
	}
	earliest := map[string]time.Time{}
	failed := map[string]error{}
	var visit func(*Component)
	visit = func(comp *Component) {
		for _, p := range comp.Props {
			if p == nil || !p.Params.Has("TZID") {
				continue
			}
			tzid := p.Params.Get("TZID")
			if defined[tzid] {
				continue
			}
			at, err := earliestWith(p, tzid, r)
			if err != nil {
				failed[tzid] = err
				continue
			}
			if e, ok := earliest[tzid]; !ok || at.Before(e) {
				earliest[tzid] = at
			}
		}
		for _, child := range comp.Children {
			if child != nil && !strings.EqualFold(child.Name, "VTIMEZONE") {
				visit(child)
			}
		}
	}
	visit(c.Component)

	var added []*Component
	var errs []error
	for _, tzid := range slices.Sorted(maps.Keys(failed)) {
		if _, ok := earliest[tzid]; !ok {
			errs = append(errs, fmt.Errorf("ical: no VTIMEZONE for TZID %q: %w", tzid, failed[tzid]))
		}
	}
	for _, tzid := range slices.Sorted(maps.Keys(earliest)) {
		z, err := timezoneFor(tzid, earliest[tzid], r)
		if err != nil {
			errs = append(errs, fmt.Errorf("ical: no VTIMEZONE for TZID %q: %w", tzid, err))
			continue
		}
		added = append(added, z.Component)
	}
	i := 0
	for i < len(c.Children) && c.Children[i] != nil && strings.EqualFold(c.Children[i].Name, "VTIMEZONE") {
		i++
	}
	c.Children = slices.Insert(c.Children, i, added...)
	c.mu.Lock()
	for _, z := range added {
		delete(c.zones, Timezone{z}.TZID())
	}
	c.mu.Unlock()
	return errors.Join(errs...)
}

// earliestWith returns the earliest instant among p's values, read with
// the zone r resolves tzid to. A DATE, which a TZID doesn't apply to,
// counts from the start of its day there.
func earliestWith(p *Property, tzid string, r tz.Resolver) (time.Time, error) {
	loc, err := r.Resolve(tzid)
	if err != nil {
		return time.Time{}, err
	}
	vs, err := p.DateTimes(r)
	var first time.Time
	found := false
	for _, v := range vs {
		at := v.Time()
		if v.IsDate {
			at = civil.DateTime{Date: v.Date}.In(loc)
		}
		if !found || at.Before(first) {
			first, found = at, true
		}
	}
	if !found {
		if err == nil {
			err = fmt.Errorf("%s has no value", p.Name)
		}
		return first, err
	}
	return first, nil
}

// timezoneFor returns the VTIMEZONE for tzid, as r resolves it, that holds
// from `from` on.
func timezoneFor(tzid string, from time.Time, r tz.Resolver) (Timezone, error) {
	loc, err := r.Resolve(tzid)
	if err != nil {
		return Timezone{}, err
	}
	def, err := tz.DefinitionOf(loc, from)
	if err != nil {
		return Timezone{}, err
	}
	def.TZID = tzid
	return NewTimezone(def)
}

// withoutTZID returns a copy of p with no TZID parameter.
func withoutTZID(p *Property) *Property {
	q := *p
	q.Params = slices.Clone(p.Params)
	q.Params.Del("TZID")
	return &q
}

// textOf returns the TEXT of the first property named name, or "".
func textOf(c *Component, name string) string {
	if p := c.Get(name); p != nil {
		return p.Text()
	}
	return ""
}

// setText replaces the property named name with one holding s as TEXT,
// or removes it when s is "".
func setText(c *Component, name, s string) {
	if s == "" {
		c.Props.Del(name)
		return
	}
	p := NewProperty(name, "")
	p.SetText(s)
	c.Props.Set(p)
}

// An Item is what VEVENT, VTODO and VJOURNAL share: identity,
// descriptive properties, a start, and recurrence. Event, Todo and Journal
// embed it.
type Item struct {
	*Component
	// Calendar resolves the item's TZIDs. When nil they resolve with
	// tz.Load alone.
	Calendar *Calendar
}

func (it Item) resolver() tz.Resolver {
	if it.Calendar != nil {
		return it.Calendar
	}
	return nil
}

// UID returns UID.
func (it Item) UID() string { return textOf(it.Component, "UID") }

// Summary returns SUMMARY.
func (it Item) Summary() string { return textOf(it.Component, "SUMMARY") }

// Description returns DESCRIPTION.
func (it Item) Description() string { return textOf(it.Component, "DESCRIPTION") }

// Status returns STATUS, like "CONFIRMED" or "CANCELLED".
func (it Item) Status() string { return strings.ToUpper(textOf(it.Component, "STATUS")) }

// Class returns CLASS, like "PUBLIC" or "PRIVATE", or "PUBLIC", the
// default (§3.8.1.3), when there is none.
func (it Item) Class() string {
	if it.Get("CLASS") == nil {
		return "PUBLIC"
	}
	return strings.ToUpper(textOf(it.Component, "CLASS"))
}

// URL returns URL.
func (it Item) URL() string {
	if p := it.Get("URL"); p != nil {
		return strings.TrimSpace(p.Value)
	}
	return ""
}

// Categories returns every value of every CATEGORIES property.
func (it Item) Categories() []string {
	var out []string
	for p := range it.Props.All("CATEGORIES") {
		out = append(out, p.Texts()...)
	}
	return out
}

// Sequence returns SEQUENCE, or 0 when it is absent or malformed.
func (it Item) Sequence() int {
	if p := it.Get("SEQUENCE"); p != nil {
		if n, err := p.Int(); err == nil {
			return n
		}
	}
	return 0
}

// Organizer returns the ORGANIZER property, or nil.
func (it Item) Organizer() *Property { return it.Get("ORGANIZER") }

// Attendees returns the ATTENDEE properties.
func (it Item) Attendees() iter.Seq[*Property] { return it.Props.All("ATTENDEE") }

// Alarms returns the item's VALARMs.
func (it Item) Alarms() iter.Seq[Alarm] {
	return func(yield func(Alarm) bool) {
		for c := range it.Components("VALARM") {
			if !yield(Alarm{c}) {
				return
			}
		}
	}
}

// Start returns DTSTART, with its TZID resolved.
func (it Item) Start() (DateTime, error) {
	return it.dateTime("DTSTART")
}

func (it Item) dateTime(name string) (DateTime, error) {
	p := it.Get(name)
	if p == nil {
		return DateTime{}, fmt.Errorf("%w: %s", ErrNoProperty, name)
	}
	return p.DateTime(it.resolver())
}

// RecurrenceID returns RECURRENCE-ID, and false when the item is not an
// override of one instance of a recurring item.
func (it Item) RecurrenceID() (DateTime, bool, error) {
	if it.Get("RECURRENCE-ID") == nil {
		return DateTime{}, false, nil
	}
	v, err := it.dateTime("RECURRENCE-ID")
	return v, true, err
}

// IsRecurring reports whether the item has a non-empty RRULE or RDATE.
func (it Item) IsRecurring() bool {
	for _, p := range it.Props {
		if (strings.EqualFold(p.Name, "RRULE") || strings.EqualFold(p.Name, "RDATE")) && strings.TrimSpace(p.Value) != "" {
			return true
		}
	}
	return false
}

// Recurrence returns the item's recurrence set: DTSTART, its RRULEs and
// RDATEs, less its EXDATEs and EXRULEs. RDATEs and EXDATEs on DTSTART's
// clock (in its zone, or floating like it) become wall-clock RDates and
// ExDates; those written in UTC or another zone become instants (RTimes,
// ExTimes), so they match instances exactly across DST changes. For a
// floating DTSTART, instants are read on the UTC wall clock; Expander
// reads them in its Floating zone instead. EXDATEs that are DATEs on a
// DATE-TIME item, which RFC 5545 doesn't allow but some producers write,
// become ExDays and exclude every instance on that day. Empty RRULEs are
// ignored.
//
// On error the set holds every part that could be read, and is nil when
// DTSTART can't be: it is missing, or its TZID can't be resolved.
func (it Item) Recurrence() (*recur.Set, error) {
	return it.recurrence(nil)
}

func (it Item) recurrence(floating *time.Location) (*recur.Set, error) {
	start, err := it.Start()
	if unusable(start, err) {
		return nil, err
	}
	errs := []error{err}
	set := &recur.Set{Start: start.DateTime, Location: start.Location(), AllDay: start.IsDate}
	if set.Location == nil && floating != nil {
		set.Location = floating
	}
	for p := range it.Props.All("RRULE") {
		if strings.TrimSpace(p.Value) == "" {
			continue // an empty RRULE, which some feeds write, is no rule
		}
		r, err := p.Recur()
		errs = append(errs, err)
		if err == nil {
			set.Rules = append(set.Rules, r)
		}
	}
	for p := range it.Props.All("EXRULE") {
		if strings.TrimSpace(p.Value) == "" {
			continue
		}
		r, err := p.Recur()
		errs = append(errs, err)
		if err == nil {
			set.ExRules = append(set.ExRules, r)
		}
	}
	sameClock := func(v DateTime) bool {
		return v.Floating() || v.Location() == start.Location() && !start.Floating()
	}
	for p := range it.Props.All("RDATE") {
		vs, err := p.DateTimes(it.resolver())
		errs = append(errs, err)
		for _, v := range vs {
			if v.unresolved() {
				continue // no instant to add; err says why
			}
			if sameClock(v) {
				set.RDates = append(set.RDates, v.DateTime)
			} else {
				set.RTimes = append(set.RTimes, v.Time())
			}
		}
	}
	for p := range it.Props.All("EXDATE") {
		vs, err := p.DateTimes(it.resolver())
		errs = append(errs, err)
		for _, v := range vs {
			switch {
			case v.unresolved():
				// No instant to exclude; err says why.
			case v.IsDate && !start.IsDate:
				set.ExDays = append(set.ExDays, v.Date)
			case sameClock(v):
				set.ExDates = append(set.ExDates, v.DateTime)
			default:
				set.ExTimes = append(set.ExTimes, v.Time())
			}
		}
	}
	return set, errors.Join(errs...)
}

// onClockOf returns v on start's wall clock, reading floating values in
// floating.
func onClockOf(start, v DateTime, floating *time.Location) civil.DateTime {
	loc := start.Location()
	if loc == nil {
		loc = floating
	}
	if loc == nil || v.Floating() {
		return v.DateTime
	}
	return civil.DateTimeOf(v.In(loc))
}

// An Event is a VEVENT.
type Event struct{ Item }

// NewEvent returns a VEVENT with the two properties RFC 5545 requires:
// UID and DTSTAMP.
func NewEvent(uid string, stamp time.Time) Event {
	c := NewComponent("VEVENT")
	setText(c, "UID", uid)
	p := NewProperty("DTSTAMP", "")
	p.SetDateTime(NewDateTime(stamp.UTC()))
	c.Props.Add(p)
	return Event{Item{Component: c}}
}

// Location returns LOCATION.
func (e Event) Location() string { return textOf(e.Component, "LOCATION") }

// Transparent reports whether TRANSP is TRANSPARENT: the event doesn't
// take up time.
func (e Event) Transparent() bool {
	return strings.EqualFold(textOf(e.Component, "TRANSP"), "TRANSPARENT")
}

// End returns when the event ends, by RFC 5545 §3.6.1: DTEND if present,
// else DTSTART plus DURATION, else the end of DTSTART's day for a DATE
// start and DTSTART itself otherwise.
func (e Event) End() (DateTime, error) {
	if e.Get("DTEND") != nil {
		return e.dateTime("DTEND")
	}
	start, err := e.Start()
	if err != nil {
		return start, err
	}
	d, err := e.Duration()
	return start.Add(d), err
}

// Duration returns how long the event lasts: DURATION if present, else
// the time from DTSTART to DTEND, else one day for a DATE start and zero
// otherwise.
func (e Event) Duration() (Duration, error) {
	if p := e.Get("DURATION"); p != nil {
		return p.Duration()
	}
	start, err := e.Start()
	if err != nil {
		return Duration{}, err
	}
	if e.Get("DTEND") != nil {
		end, err := e.dateTime("DTEND")
		if err != nil {
			return Duration{}, err
		}
		return between(start, end), nil
	}
	if start.IsDate {
		return Duration{Days: 1}, nil
	}
	return Duration{}, nil
}

// span returns how long the item lasts and when it ends, by its kind: a
// VTODO's DUE, or for anything else what Event.End and Event.Duration
// give.
func (it Item) span(start DateTime) (Duration, DateTime, error) {
	if strings.EqualFold(it.Name, "VTODO") && it.Get("DUE") != nil {
		due, err := it.dateTime("DUE")
		if err != nil {
			return Duration{}, start, err
		}
		return between(start, due), due, nil
	}
	e := Event{it}
	d, err := e.Duration()
	if err != nil {
		return d, start.Add(d), err
	}
	end, err := e.End()
	return d, end, err
}

// between returns the duration from start to end: whole days between two
// DATEs, and the exact time between instants otherwise.
func between(start, end DateTime) Duration {
	if start.IsDate && end.IsDate {
		n := end.Date.DaysSince(start.Date)
		if n < 0 {
			return Duration{Negative: true, Days: -n}
		}
		return Duration{Days: n}
	}
	if start.Floating() && end.Floating() {
		return DurationOf(time.Duration(end.EpochSecond()-start.EpochSecond()) * time.Second)
	}
	return DurationOf(end.Time().Sub(start.Time()))
}

// SetSummary sets SUMMARY, or removes it when s is "".
func (it Item) SetSummary(s string) { setText(it.Component, "SUMMARY", s) }

// SetDescription sets DESCRIPTION, or removes it when s is "".
func (it Item) SetDescription(s string) { setText(it.Component, "DESCRIPTION", s) }

// SetLocation sets LOCATION, or removes it when s is "".
func (e Event) SetLocation(s string) { setText(e.Component, "LOCATION", s) }

// SetStart sets DTSTART.
func (it Item) SetStart(v DateTime) { it.setDateTime("DTSTART", v) }

// SetEnd sets DTEND and removes DURATION, which can't be combined with it.
func (e Event) SetEnd(v DateTime) {
	e.Props.Del("DURATION")
	e.setDateTime("DTEND", v)
}

// SetDuration sets DURATION and removes DTEND.
func (e Event) SetDuration(d Duration) {
	e.Props.Del("DTEND")
	p := NewProperty("DURATION", "")
	p.SetDuration(d)
	e.Props.Set(p)
}

// SetRule sets the item's only RRULE.
func (it Item) SetRule(r recur.Rule) {
	p := NewProperty("RRULE", "")
	p.SetRecur(r)
	it.Props.Set(p)
}

func (it Item) setDateTime(name string, v DateTime) {
	p := NewProperty(name, "")
	p.SetDateTime(v)
	it.Props.Set(p)
}

// A Todo is a VTODO.
type Todo struct{ Item }

// Due returns DUE, or DTSTART plus DURATION when there is no DUE.
func (t Todo) Due() (DateTime, error) {
	if t.Get("DUE") != nil {
		return t.dateTime("DUE")
	}
	start, err := t.Start()
	if err != nil {
		return start, err
	}
	p := t.Get("DURATION")
	if p == nil {
		return DateTime{}, fmt.Errorf("%w: DUE", ErrNoProperty)
	}
	d, err := p.Duration()
	return start.Add(d), err
}

// Completed returns COMPLETED.
func (t Todo) Completed() (DateTime, error) { return t.dateTime("COMPLETED") }

// PercentComplete returns PERCENT-COMPLETE, or 0.
func (t Todo) PercentComplete() int {
	if p := t.Get("PERCENT-COMPLETE"); p != nil {
		if n, err := p.Int(); err == nil {
			return n
		}
	}
	return 0
}

// Priority returns PRIORITY: 1 is highest, 9 lowest, 0 undefined.
func (it Item) Priority() int {
	if p := it.Get("PRIORITY"); p != nil {
		if n, err := p.Int(); err == nil {
			return n
		}
	}
	return 0
}

// A Journal is a VJOURNAL.
type Journal struct{ Item }

// An Alarm is a VALARM.
type Alarm struct {
	*Component
}

// Action returns ACTION: "AUDIO", "DISPLAY" or "EMAIL".
func (a Alarm) Action() string { return strings.ToUpper(textOf(a.Component, "ACTION")) }

// Description returns DESCRIPTION.
func (a Alarm) Description() string { return textOf(a.Component, "DESCRIPTION") }

// Trigger returns TRIGGER: either a duration relative to the start (or,
// when relatedToEnd is set, the end) of the parent, or an absolute time.
func (a Alarm) Trigger() (rel Duration, abs DateTime, relatedToEnd bool, err error) {
	p := a.Get("TRIGGER")
	if p == nil {
		return Duration{}, DateTime{}, false, fmt.Errorf("%w: TRIGGER", ErrNoProperty)
	}
	if strings.EqualFold(p.Params.Get("VALUE"), "DATE-TIME") {
		abs, err = p.DateTime(nil)
		return Duration{}, abs, false, err
	}
	rel, err = p.Duration()
	return rel, DateTime{}, strings.EqualFold(p.Params.Get("RELATED"), "END"), err
}
