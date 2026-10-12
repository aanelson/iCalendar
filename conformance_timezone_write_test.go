package ical_test

// Conformance tests for writing VTIMEZONE (RFC 5545 §3.6.5) for a
// *time.Location: tz.DefinitionOf, ical.NewTimezone and
// Calendar.AddTimezones. What is written is encoded, read back strictly,
// and checked as text against the RFC's MUSTs. Its offsets are worked out
// by §3.6.5's own rule, without tz.Compile: each observance's onsets come
// from its DTSTART, RRULE and RDATE on its TZOFFSETFROM clock, and "the
// offset to apply at any given time is found by locating the observance
// that has the last onset date and time before the time in question, and
// using the offset value from that observance".

import (
	"bytes"
	"errors"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	ical "github.com/aanelson/iCalendar"
	"github.com/aanelson/iCalendar/civil"
	"github.com/aanelson/iCalendar/recur"
	"github.com/aanelson/iCalendar/tz"
)

// cwZones have the shapes a writer has to get right: plain US and EU
// rules, a 30-minute DST, no DST since 1945, Ramadan breaks, weekdays on
// or after a day (inside a month and across its end), DST made permanent
// and then abolished, and no transitions at all.
var cwZones = []string{
	"America/Chicago", "America/New_York", "Europe/London", "Australia/Lord_Howe", "Asia/Kolkata",
	"Africa/Casablanca", "Asia/Jerusalem", "Africa/Cairo", "America/Santiago", "Europe/Moscow", "UTC",
}

var cwFroms = []time.Time{
	time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC),
	time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC),
}

// cwStrict encodes cal and reads it back in strict mode, failing on any
// recovery.
func cwStrict(t *testing.T, cal *ical.Calendar) *ical.Calendar {
	t.Helper()
	b, err := cal.Bytes()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	dec := ical.NewDecoder(bytes.NewReader(b))
	dec.Strict = true
	dec.Warn = func(err error) { t.Errorf("strict read: %v", err) }
	back, err := dec.Decode()
	if err != nil {
		t.Fatalf("strict read: %v\n%s", err, b)
	}
	return back
}

// cwWritten returns the VTIMEZONE written for loc from `from`, as read
// back from its text.
func cwWritten(t *testing.T, loc *time.Location, from time.Time) *ical.Component {
	t.Helper()
	def, err := tz.DefinitionOf(loc, from)
	if err != nil {
		t.Fatalf("DefinitionOf: %v", err)
	}
	z, err := ical.NewTimezone(def)
	if err != nil {
		t.Fatalf("NewTimezone: %v", err)
	}
	cal := ical.NewCalendar("-//conformance//EN")
	cal.Add(z.Component)
	back := cwStrict(t, cal)
	for c := range back.Components("VTIMEZONE") {
		return c
	}
	t.Fatal("no VTIMEZONE read back")
	return nil
}

func cwCount(c *ical.Component, name string) int {
	n := 0
	for range c.Props.All(name) {
		n++
	}
	return n
}

var (
	cwLocalTime = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}$`)
	cwUTCOffset = regexp.MustCompile(`^[+-]([01][0-9]|2[0-3])[0-5][0-9]([0-5][0-9])?$`)
)

// §3.6.5: a VTIMEZONE "MUST include the "TZID" property and at least one
// definition of a "STANDARD" or "DAYLIGHT" sub-component". In each, by
// the tzprop grammar, "dtstart / tzoffsetto / tzoffsetfrom" are "REQUIRED,
// but MUST NOT occur more than once" and "rrule" is "OPTIONAL, but SHOULD
// NOT occur more than once". "DTSTART in this usage MUST be specified as a
// date with a local time value", and "RDATE in this usage MUST be
// specified as a date with local time value". An observance RRULE's UNTIL
// "MUST be specified in UTC time". §3.3.14: a UTC-OFFSET is
// ("+" / "-") time-hour time-minute [time-second], and "The value of
// "-0000" and "-000000" are not allowed".
func TestConformanceWrittenVTimezoneGrammar(t *testing.T) {
	for _, name := range cwZones {
		for _, from := range cwFroms {
			t.Run(name+"/"+from.Format("2006"), func(t *testing.T) {
				z := cwWritten(t, cvLoad(t, name), from)
				if n := cwCount(z, "TZID"); n != 1 {
					t.Errorf("%d TZIDs", n)
				}
				if len(z.Children) == 0 {
					t.Error("no STANDARD or DAYLIGHT")
				}
				for _, o := range z.Children {
					if o.Name != "STANDARD" && o.Name != "DAYLIGHT" {
						t.Errorf("sub-component %s", o.Name)
					}
					for _, p := range []string{"DTSTART", "TZOFFSETFROM", "TZOFFSETTO"} {
						if n := cwCount(o, p); n != 1 {
							t.Errorf("%s has %d %s", o.Name, n, p)
						}
					}
					if n := cwCount(o, "RRULE"); n > 1 {
						t.Errorf("%s has %d RRULEs", o.Name, n)
					}
					if p := o.Get("DTSTART"); p == nil || len(p.Params) > 0 || !cwLocalTime.MatchString(p.Value) {
						t.Errorf("%s DTSTART %+v is not a local DATE-TIME", o.Name, p)
					}
					for p := range o.Props.All("RDATE") {
						for _, v := range strings.Split(p.Value, ",") {
							if len(p.Params) > 0 || !cwLocalTime.MatchString(v) {
								t.Errorf("%s RDATE %+v is not a local DATE-TIME", o.Name, p)
							}
						}
					}
					for p := range o.Props.All("RRULE") {
						r, err := p.Recur()
						if err != nil {
							t.Fatal(err)
						}
						if !r.Until.IsZero() && (!r.Until.UTC || !strings.Contains(p.Value, "UNTIL="+r.Until.String())) {
							t.Errorf("%s RRULE %s: UNTIL not in UTC", o.Name, p.Value)
						}
						if r.Count != 0 {
							t.Errorf("%s RRULE %s: COUNT, not UNTIL", o.Name, p.Value)
						}
					}
					for _, p := range []string{"TZOFFSETFROM", "TZOFFSETTO"} {
						if v := o.Get(p).Value; !cwUTCOffset.MatchString(v) || strings.Trim(v, "-0") == "" {
							t.Errorf("%s %s:%s", o.Name, p, v)
						}
					}
				}
			})
		}
	}
}

// A cwOnset is when an observance takes effect and what it changes to.
type cwOnset struct {
	at     time.Time
	offset int
	name   string
}

// cwOnsets lists every onset of a VTIMEZONE's observances before end, by
// §3.6.5: "The onset DATE-TIME values for the observance defined by the
// time zone sub-component is defined by the "DTSTART", "RRULE", and
// "RDATE" properties", and "The "DTSTART" and the "TZOFFSETFROM"
// properties MUST be used when generating the onset DATE-TIME values
// (instances) from the "RRULE"". It also checks "the UNTIL DATE-TIME will
// be equal to the last instance generated by the recurrence pattern".
func cwOnsets(t *testing.T, z *ical.Component, end civil.DateTime) []cwOnset {
	t.Helper()
	var out []cwOnset
	for _, o := range z.Children {
		from, err := o.Get("TZOFFSETFROM").UTCOffset()
		if err != nil {
			t.Fatal(err)
		}
		to, err := o.Get("TZOFFSETTO").UTCOffset()
		if err != nil {
			t.Fatal(err)
		}
		dtstart, err := o.Get("DTSTART").DateTime(nil)
		if err != nil {
			t.Fatal(err)
		}
		start := dtstart.DateTime
		clock := time.FixedZone("", from)
		name := ""
		if p := o.Get("TZNAME"); p != nil {
			name = p.Text()
		}
		add := func(wall civil.DateTime) { out = append(out, cwOnset{wall.In(clock), to, name}) }
		add(start)
		for p := range o.Props.All("RRULE") {
			r, err := p.Recur()
			if err != nil {
				t.Fatal(err)
			}
			var last civil.DateTime
			set := recur.Set{Start: start, Location: clock, Rules: []recur.Rule{r}}
			for on := range set.Between(start, end) {
				if on != start {
					add(on)
				}
				last = on
			}
			if !r.Until.IsZero() && !last.In(clock).Equal(r.Until.DateTime.In(time.UTC)) {
				t.Errorf("%s RRULE %s: last onset %v, not UNTIL", o.Name, p.Value, last.In(clock).UTC())
			}
		}
		for p := range o.Props.All("RDATE") {
			ds, err := p.DateTimes(nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range ds {
				add(d.DateTime)
			}
		}
	}
	slices.SortFunc(out, func(a, b cwOnset) int { return a.at.Compare(b.at) })
	return out
}

// cwOffsetAt applies §3.6.5's rule: the offset of the observance with the
// last onset at or before at.
func cwOffsetAt(onsets []cwOnset, at time.Time) (cwOnset, bool) {
	i, found := slices.BinarySearchFunc(onsets, at, func(o cwOnset, at time.Time) int { return o.at.Compare(at) })
	if found {
		return onsets[i], true
	}
	if i == 0 {
		return cwOnset{}, false
	}
	return onsets[i-1], true
}

// The written VTIMEZONE, read by §3.6.5's rule, gives the zone's offset
// and abbreviation at every instant from `from` to 2100: one second
// before and at each of the zone's changes, and every week between.
func TestConformanceWrittenVTimezoneOffsets(t *testing.T) {
	end := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, name := range cwZones {
		for _, from := range cwFroms {
			t.Run(name+"/"+from.Format("2006"), func(t *testing.T) {
				loc := cvLoad(t, name)
				onsets := cwOnsets(t, cwWritten(t, loc, from), civil.DateTimeOf(end.AddDate(1, 0, 0)))
				check := func(at time.Time) bool {
					got, ok := cwOffsetAt(onsets, at)
					wantName, want := at.In(loc).Zone()
					if !ok || got.offset != want || got.name != wantName {
						t.Errorf("at %v: VTIMEZONE gives %s %+d (onset %v), the zone %s %+d", at, got.name, got.offset, got.at, wantName, want)
						return false
					}
					return true
				}
				for at := from; at.Before(end); at = at.Add(7 * 24 * time.Hour) {
					if !check(at) {
						return
					}
				}
				for at := from; at.Before(end); {
					_, next := at.In(loc).ZoneBounds()
					if next.IsZero() {
						break
					}
					if !next.After(at) { // past the zone's explicit transitions, on a leap year's last day
						next = at.Add(24 * time.Hour)
					}
					if !check(next.Add(-time.Second)) || !check(next) {
						return
					}
					at = next
				}
			})
		}
	}
}

// §3.6.5: "If observance is known to have an effective end date, the
// "UNTIL" recurrence rule parameter MUST be used to specify the last valid
// onset of this observance (i.e., the UNTIL DATE-TIME will be equal to the
// last instance generated by the recurrence pattern). It MUST be specified
// in UTC time." NewTimezone writes that for a definition that ends a rule
// any other way: a COUNT, a floating UNTIL or a DATE UNTIL (both read on
// the TZOFFSETFROM clock, like DTSTART), or a UTC UNTIL past the last
// onset.
func TestConformanceNewTimezoneEndsRulesAtTheirLastOnset(t *testing.T) {
	rule := func(s string) recur.Rule {
		r, err := recur.ParseRule(s)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	start := civil.DateTime{Date: civil.Date{Year: 1967, Month: 10, Day: 29}, Time: civil.Time{Hour: 2}}
	// The last Sunday in October 1973 is the 28th; 02:00 EDT is 06:00Z.
	const want = "FREQ=YEARLY;UNTIL=19731028T060000Z;BYDAY=-1SU;BYMONTH=10"
	for _, r := range []string{
		"FREQ=YEARLY;BYMONTH=10;BYDAY=-1SU;COUNT=7",
		"FREQ=YEARLY;BYMONTH=10;BYDAY=-1SU;UNTIL=19731028T020000",
		"FREQ=YEARLY;BYMONTH=10;BYDAY=-1SU;UNTIL=19731028",
		"FREQ=YEARLY;BYMONTH=10;BYDAY=-1SU;UNTIL=19731231T235959Z",
		"FREQ=YEARLY;BYMONTH=10;BYDAY=-1SU;UNTIL=19731028T060000Z",
	} {
		z, err := ical.NewTimezone(tz.Definition{TZID: "America/New_York", Observances: []tz.Observance{{
			Start: start, OffsetFrom: -4 * 3600, OffsetTo: -5 * 3600, Name: "EST", Rules: []recur.Rule{rule(r)},
		}}})
		if err != nil {
			t.Fatalf("%s: %v", r, err)
		}
		if got := z.Children[0].Get("RRULE").Value; got != want {
			t.Errorf("%s: written as %s, want %s", r, got, want)
		}
	}
}

// §3.6.5: a VTIMEZONE "MUST include the "TZID" property and at least one
// definition of a "STANDARD" or "DAYLIGHT" sub-component"; §3.3.14 gives
// a UTC-OFFSET's hours as time-hour, 00 to 23; and an RRULE must follow
// §3.3.10. NewTimezone refuses a definition that can't be written as one.
func TestConformanceNewTimezoneRefuses(t *testing.T) {
	ok := tz.Observance{Start: civil.DateTime{Date: civil.Date{Year: 1970, Month: 1, Day: 1}}, OffsetFrom: 3600, OffsetTo: 3600}
	weekNo, err := recur.ParseRule("FREQ=MONTHLY;BYWEEKNO=20")
	if err != nil {
		t.Fatal(err)
	}
	for name, def := range map[string]tz.Definition{
		"no TZID":         {Observances: []tz.Observance{ok}},
		"no observance":   {TZID: "X"},
		"offset of 24h":   {TZID: "X", Observances: []tz.Observance{{Start: ok.Start, OffsetFrom: 0, OffsetTo: 86400}}},
		"invalid DTSTART": {TZID: "X", Observances: []tz.Observance{{OffsetFrom: 0, OffsetTo: 0}}},
		"invalid RDATE":   {TZID: "X", Observances: []tz.Observance{{Start: ok.Start, RDates: []civil.DateTime{{}}}}},
		"invalid RRULE":   {TZID: "X", Observances: []tz.Observance{{Start: ok.Start, Rules: []recur.Rule{weekNo}}}},
	} {
		if _, err := ical.NewTimezone(def); !errors.Is(err, tz.ErrInvalidDefinition) {
			t.Errorf("%s: %v, want ErrInvalidDefinition", name, err)
		}
	}
}

// §3.6.5: "An individual "VTIMEZONE" calendar component MUST be specified
// for each unique "TZID" parameter value specified in the iCalendar
// object." AddTimezones adds one for every TZID with none, wherever the
// parameter is (DTSTART, DTEND, EXDATE, a PERIOD RDATE, a VTODO's DUE),
// named exactly as the parameter is, Windows names included; leaves the
// calendar's own VTIMEZONEs alone; and puts the new ones before the
// components that use them. A TZID it can't resolve is an error, and
// doesn't stop the others.
func TestConformanceAddTimezonesForEveryTZID(t *testing.T) {
	cvLoad(t, "America/Chicago")
	cal := ccCal(t, `BEGIN:VTIMEZONE
TZID:Custom
BEGIN:STANDARD
DTSTART:19700101T000000
TZOFFSETFROM:+0300
TZOFFSETTO:+0300
END:STANDARD
END:VTIMEZONE
BEGIN:VEVENT
UID:a@example.com
DTSTAMP:20260101T000000Z
DTSTART;TZID=America/Chicago:20260302T090000
DTEND;TZID=America/New_York:20260302T110000
RRULE:FREQ=WEEKLY
EXDATE;TZID=Europe/London:20260309T150000
RDATE;VALUE=PERIOD;TZID=Asia/Kolkata:20260310T193000/PT1H
END:VEVENT
BEGIN:VTODO
UID:b@example.com
DTSTAMP:20260101T000000Z
DTSTART;TZID=Custom:20260101T090000
DUE;TZID=Eastern Standard Time:20260105T170000
END:VTODO
BEGIN:VEVENT
UID:c@example.com
DTSTAMP:20260101T000000Z
DTSTART;TZID=Nowhere/Special:20260101T090000
END:VEVENT`)
	err := cal.AddTimezones(nil)
	if !errors.Is(err, tz.ErrUnknownZone) || !strings.Contains(err.Error(), "Nowhere/Special") {
		t.Errorf("AddTimezones: %v, want an error for Nowhere/Special", err)
	}
	back := cwStrict(t, cal)
	var tzids []string
	for _, c := range back.Children {
		if c.Name != "VTIMEZONE" {
			break
		}
		tzids = append(tzids, ical.Timezone{Component: c}.TZID())
	}
	want := []string{"Custom", "America/Chicago", "America/New_York", "Asia/Kolkata", "Eastern Standard Time", "Europe/London"}
	if !slices.Equal(tzids, want) {
		t.Errorf("VTIMEZONEs %q, want %q, ahead of the other components", tzids, want)
	}
	if n := slices.IndexFunc(back.Children, func(c *ical.Component) bool { return c.Name == "VTIMEZONE" && c.Get("TZID").Value == "Custom" }); n != 0 ||
		len(back.Children[0].Children) != 1 {
		t.Error("the calendar's own VTIMEZONE changed")
	}
	// The Windows name gets New York's zone, under its own name: the
	// observances DefinitionOf gives New York from the DUE (17:00 EST).
	ny, err := tz.DefinitionOf(cvLoad(t, "America/New_York"), time.Date(2026, 1, 5, 22, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	var nyNames []string
	for _, o := range ny.Observances {
		nyNames = append(nyNames, o.Name)
	}
	for z := range back.Timezones() {
		if z.TZID() == "Eastern Standard Time" {
			var names []string
			for _, o := range z.Children {
				names = append(names, o.Get("TZNAME").Text())
			}
			if !slices.Equal(names, nyNames) {
				t.Errorf("Eastern Standard Time: TZNAMEs %q, want New York's %q", names, nyNames)
			}
		}
	}
	// Nothing more to add the second time.
	before := len(cal.Children)
	if err := cal.AddTimezones(nil); !errors.Is(err, tz.ErrUnknownZone) || len(cal.Children) != before {
		t.Errorf("again: %v, %d components, want %d", err, len(cal.Children), before)
	}
}

// §3.6.5: "a "VTIMEZONE" calendar component, referred to by a recurring
// calendar component, MUST provide valid time zone information for all
// recurrence instances of the recurring calendar component." A weekly
// 09:00 meeting in Chicago with no end, written with the VTIMEZONE
// AddTimezones adds and read back, stays at 09:00 Chicago time through
// every DST change for decades, as the IANA zone has it, with the
// calendar's own VTIMEZONE doing the resolving.
func TestConformanceAddTimezonesCoversRecurrence(t *testing.T) {
	chicago := cvLoad(t, "America/Chicago")
	cal := ical.NewCalendar("-//conformance//EN")
	ev := ical.NewEvent("standup@example.com", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev.SetStart(ical.NewDateTime(time.Date(2026, 3, 2, 9, 0, 0, 0, chicago)))
	ev.SetDuration(ical.Duration{Minutes: 15})
	rule, err := recur.ParseRule("FREQ=WEEKLY")
	if err != nil {
		t.Fatal(err)
	}
	ev.SetRule(rule)
	cal.Add(ev.Component)
	if err := cal.AddTimezones(nil); err != nil {
		t.Fatal(err)
	}
	back := cwStrict(t, cal)
	var vtz *time.Location
	for z := range back.Timezones() {
		if vtz, err = z.Location(); err != nil {
			t.Fatal(err)
		}
	}
	resolved, err := back.Resolve("America/Chicago")
	if err != nil || resolved != vtz {
		t.Fatalf("America/Chicago resolves to %p (%v), not the calendar's VTIMEZONE %p", resolved, err, vtz)
	}
	from, to := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2060, 1, 1, 0, 0, 0, 0, time.UTC)
	want := time.Date(2026, 3, 2, 9, 0, 0, 0, chicago)
	n := 0
	for occ, err := range back.Occurrences(from, to) {
		if err != nil {
			t.Fatal(err)
		}
		if got := occ.Start.Time(); !got.Equal(want) {
			t.Fatalf("occurrence %d at %v, want %v", n, got, want)
		}
		want = time.Date(want.Year(), want.Month(), want.Day()+7, 9, 0, 0, 0, chicago)
		n++
	}
	if n < 52*33 {
		t.Errorf("%d occurrences", n)
	}
}

// AddTimezones drops the calendar's cached resolution of a TZID it adds a
// VTIMEZONE for, so the TZID resolves to that VTIMEZONE, as it would in
// any reader of the calendar.
func TestAddTimezonesForgetsResolutions(t *testing.T) {
	cvLoad(t, "Europe/Paris")
	cal := ical.NewCalendar("-//test//EN")
	ev := ical.NewEvent("x@example.com", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev.SetStart(ical.NewDateTime(time.Date(2026, 6, 1, 9, 0, 0, 0, cvLoad(t, "Europe/Paris"))))
	cal.Add(ev.Component)
	iana, err := cal.Resolve("Europe/Paris")
	if err != nil {
		t.Fatal(err)
	}
	if err := cal.AddTimezones(nil); err != nil {
		t.Fatal(err)
	}
	got, err := cal.Resolve("Europe/Paris")
	if err != nil || got == iana {
		t.Errorf("Europe/Paris still resolves to the IANA zone (%v)", err)
	}
}

// What NewTimezone writes reads back, through Timezone.Definition, as the
// definition it was given.
func TestNewTimezoneRoundTrip(t *testing.T) {
	for _, name := range cwZones {
		t.Run(name, func(t *testing.T) {
			def, err := tz.DefinitionOf(cvLoad(t, name), time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC))
			if err != nil {
				t.Fatal(err)
			}
			z, err := ical.NewTimezone(def)
			if err != nil {
				t.Fatal(err)
			}
			cal := ical.NewCalendar("-//test//EN")
			cal.Add(z.Component)
			var got tz.Definition
			for z := range cwStrict(t, cal).Timezones() {
				if got, err = z.Definition(); err != nil {
					t.Fatal(err)
				}
			}
			if !slices.EqualFunc(got.Observances, def.Observances, func(a, b tz.Observance) bool {
				return a.Daylight == b.Daylight && a.Start == b.Start && a.OffsetFrom == b.OffsetFrom && a.OffsetTo == b.OffsetTo &&
					a.Name == b.Name && slices.Equal(a.RDates, b.RDates) &&
					slices.EqualFunc(a.Rules, b.Rules, func(x, y recur.Rule) bool { return x.String() == y.String() })
			}) || got.TZID != def.TZID {
				t.Error("read back differently")
			}
		})
	}
}

// AddTimezones writes the zone the calendar's times were computed in, from
// the resolver it is given, not whatever tz.Load finds by the same name:
// here a location named America/Chicago that is always at -06:00, as one
// built with time.LoadLocationFromTZData from other data could be.
func TestAddTimezonesUsesTheResolver(t *testing.T) {
	cvLoad(t, "America/Chicago")
	custom := time.FixedZone("America/Chicago", -6*3600)
	cal := ical.NewCalendar("-//test//EN")
	ev := ical.NewEvent("x@example.com", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev.SetStart(ical.NewDateTime(time.Date(2026, 7, 1, 9, 0, 0, 0, custom)))
	cal.Add(ev.Component)
	err := cal.AddTimezones(tz.ResolverFunc(func(tzid string) (*time.Location, error) {
		if tzid == custom.String() {
			return custom, nil
		}
		return tz.Load(tzid)
	}))
	if err != nil {
		t.Fatal(err)
	}
	back := cwStrict(t, cal)
	for occ, err := range back.Occurrences(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)) {
		if err != nil {
			t.Fatal(err)
		}
		if got, want := occ.Start.Time(), time.Date(2026, 7, 1, 15, 0, 0, 0, time.UTC); !got.Equal(want) {
			t.Errorf("start %v, want %v as written", got, want)
		}
	}
}
