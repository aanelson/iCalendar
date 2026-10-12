package tz

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aanelson/iCalendar/civil"
	"github.com/aanelson/iCalendar/recur"
)

// beyondHorizon is how far the round trips compare: past Horizon, where a
// compiled zone runs on its TZif footer, as the source zone does on its
// own.
var beyondHorizon = time.Date(2400, 1, 1, 0, 0, 0, 0, time.UTC)

// bound returns the next bound ZoneBounds reports after t, with the time
// package's own quirks covered: nothing past the last transition is the
// far future, and an end that doesn't advance (on the last day of a leap
// year past a zone's explicit transitions) becomes a day's step.
func bound(loc *time.Location, t time.Time) time.Time {
	_, end := t.In(loc).ZoneBounds()
	switch {
	case end.IsZero():
		return time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
	case !end.After(t):
		return t.Add(24 * time.Hour)
	}
	return end
}

func earlier(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// sameZone checks that got shows want's offset and abbreviation at every
// instant in [from, to). Both change only at bounds ZoneBounds reports, so
// it checks at from, at every bound of either zone, and a second before
// each, and returns the instants where want changes.
func sameZone(t *testing.T, want, got *time.Location, from, to time.Time) (changes []time.Time) {
	t.Helper()
	for at := from; at.Before(to); at = earlier(bound(want, at), bound(got, at)) {
		for _, x := range []time.Time{at.Add(-time.Second), at} {
			if x.Before(from) {
				continue
			}
			if !sameAt(t, want, got, x) {
				return nil
			}
		}
		if at.After(from) && stateAt(want, at) != stateAt(want, at.Add(-time.Second)) {
			changes = append(changes, at)
		}
	}
	return changes
}

func sameAt(t *testing.T, want, got *time.Location, x time.Time) bool {
	t.Helper()
	wantName, wantOff := x.In(want).Zone()
	gotName, gotOff := x.In(got).Zone()
	// An unnamed zone has no TZNAME, and Compile names it by its offset.
	if gotOff != wantOff || gotName != wantName && wantName != "" {
		t.Errorf("%s at %v: compiled definition shows %s %+d, the zone %s %+d", want, x.UTC(), gotName, gotOff, wantName, wantOff)
		return false
	}
	return true
}

// roundTrip compiles DefinitionOf(loc, from) and checks it against loc
// from `from` to `to`.
func roundTrip(t *testing.T, loc *time.Location, from, to time.Time) (Definition, *time.Location, []time.Time) {
	t.Helper()
	def, err := DefinitionOf(loc, from)
	if err != nil {
		t.Fatalf("DefinitionOf(%s, %v): %v", loc, from, err)
	}
	checkDefinition(t, def, from)
	got, err := Compile(def)
	if err != nil {
		t.Fatalf("Compile(DefinitionOf(%s, %v)): %v", loc, from, err)
	}
	return def, got, sameZone(t, loc, got, from, to)
}

// checkDefinition checks what DefinitionOf promises about the definition
// itself: observances in order, starting no later than `from`, each rule
// one a VTIMEZONE may hold, ending (if it ends) with a UTC UNTIL equal to
// its last onset (RFC 5545 §3.6.5).
func checkDefinition(t *testing.T, def Definition, from time.Time) {
	t.Helper()
	instant := func(o Observance, wall civil.DateTime) int64 { return wall.EpochSecond() - int64(o.OffsetFrom) }
	if len(def.Observances) == 0 || instant(def.Observances[0], def.Observances[0].Start) > from.Unix() {
		t.Errorf("%s: no observance starts by %v", def.TZID, from)
		return
	}
	for i, o := range def.Observances {
		if i > 0 && instant(o, o.Start) <= instant(def.Observances[i-1], def.Observances[i-1].Start) {
			t.Errorf("%s: observance %d starts before the one before it", def.TZID, i)
		}
		if len(o.Rules) > 1 {
			t.Errorf("%s: observance %d has %d RRULEs (§3.6.5: SHOULD NOT occur more than once)", def.TZID, i, len(o.Rules))
		}
		for _, r := range o.Rules {
			if err := r.Validate(); err != nil {
				t.Errorf("%s: %v", def.TZID, err)
			}
			if r.Count != 0 || r.Until.IsZero() {
				continue
			}
			if !r.Until.UTC {
				t.Errorf("%s: UNTIL %s is not UTC", def.TZID, r.Until)
			}
			clock := time.FixedZone("", o.OffsetFrom)
			var last civil.DateTime
			for on := range (&recur.Set{Start: o.Start, Location: clock, Rules: []recur.Rule{r}}).All() {
				last = on
			}
			if got := last.In(clock).UTC(); !got.Equal(r.Until.DateTime.In(time.UTC)) {
				t.Errorf("%s: RRULE %s: last onset %v, not UNTIL", def.TZID, r, got)
			}
		}
	}
}

// RFC 5545 §3.6.5 asks for a VTIMEZONE that gives the right offset for
// every time a calendar writes in the zone and every recurrence instance
// after it. The zones here have been chosen for their history: no DST
// since 1945 (Kolkata), a 30-minute DST (Lord_Howe), 45-minute offsets
// (Chatham), DST abolished (Sao_Paulo, Tehran) or made permanent and
// undone (Moscow), Ramadan breaks (Casablanca, Gaza), rules POSIX writes
// past midnight or before it (Santiago, Nuuk, Jerusalem, Cairo), a day
// skipped at the date line (Apia) and a 2-hour DST (Troll). Each is
// compared everywhere it changes, and densely around each change, from
// several starting points, including ones before any transition, exactly
// at one, and past the explicit transitions of a TZif file.
func TestDefinitionOfRoundTrip(t *testing.T) {
	zones := []string{
		"America/New_York", "America/Chicago", "Europe/London", "Australia/Lord_Howe", "Asia/Kolkata",
		"Pacific/Chatham", "America/Sao_Paulo", "Asia/Tehran", "Europe/Moscow", "Africa/Casablanca",
		"Asia/Gaza", "America/Santiago", "America/Nuuk", "Asia/Jerusalem", "Africa/Cairo",
		"Europe/Dublin", "Pacific/Apia", "Antarctica/Troll", "UTC",
	}
	froms := []time.Time{
		time.Date(1800, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2007, 3, 11, 7, 0, 0, 0, time.UTC), // New York's first second of DST in 2007
		time.Date(2026, 10, 12, 15, 4, 5, 999, time.UTC),
		time.Date(2150, 6, 1, 0, 0, 0, 0, time.UTC),
	}
	near := []time.Duration{-24 * time.Hour, -time.Hour, -time.Minute, -time.Second, 0, time.Second, time.Minute, time.Hour, 24 * time.Hour}
	for _, name := range zones {
		for _, from := range froms {
			t.Run(fmt.Sprintf("%s/%d", name, from.Year()), func(t *testing.T) {
				loc := loadOrSkip(t, name)
				_, got, changes := roundTrip(t, loc, from, beyondHorizon)
				for _, at := range changes {
					for _, d := range near {
						if x := at.Add(d); !x.Before(from) && !sameAt(t, loc, got, x) {
							return
						}
					}
				}
				// And a week at a time, which doesn't rely on ZoneBounds.
				for x := from; x.Before(beyondHorizon); x = x.Add(7 * 24 * time.Hour) {
					if !sameAt(t, loc, got, x) {
						return
					}
				}
			})
		}
	}
}

// zoneinfoNames returns the zones in the host's zoneinfo directory.
func zoneinfoNames(t *testing.T) []string {
	for _, dir := range []string{"/usr/share/zoneinfo", "/usr/share/lib/zoneinfo", "/usr/lib/locale/TZ"} {
		root, err := filepath.EvalSymlinks(dir)
		if err != nil {
			continue
		}
		var names []string
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			switch {
			case d.IsDir() && (rel == "posix" || rel == "right"):
				return filepath.SkipDir
			case d.IsDir() || rel == "localtime" || rel == "posixrules":
				return nil
			}
			if b, err := os.ReadFile(path); err == nil && bytes.HasPrefix(b, []byte("TZif")) {
				names = append(names, rel)
			}
			return nil
		})
		if len(names) > 0 {
			slices.Sort(names)
			return names
		}
	}
	t.Skip("no zoneinfo directory")
	return nil
}

// Every zone the host knows round-trips, from before the Unix epoch and
// from now, to past the horizon. -short checks every seventh zone.
func TestDefinitionOfAllZones(t *testing.T) {
	names := zoneinfoNames(t)
	t.Logf("%d zones", len(names))
	for i, name := range names {
		if testing.Short() && i%7 != 0 {
			continue
		}
		loc, err := time.LoadLocation(name)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		for _, from := range []time.Time{time.Date(1960, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)} {
			roundTrip(t, loc, from, beyondHorizon)
		}
	}
}

// observances writes a definition's observances one per line, for
// comparing with what's expected.
func observances(def Definition) string {
	var b strings.Builder
	for _, o := range def.Observances {
		kind := "STANDARD"
		if o.Daylight {
			kind = "DAYLIGHT"
		}
		fmt.Fprintf(&b, "%s %v %s %s %q", kind, o.Start, zoneName("", o.OffsetFrom), zoneName("", o.OffsetTo), o.Name)
		for _, r := range o.Rules {
			fmt.Fprintf(&b, " %s", r)
		}
		for _, d := range o.RDates {
			fmt.Fprintf(&b, " +%v", d)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// The shapes DefinitionOf writes, on zones built here so they don't change
// with the host's zone data: yearly rules traced back to where they began,
// UNTIL at the last onset when a rule stops, one-offs as RDATEs, and
// rules for weekdays on or after a day, inside a month and across its end.
func TestDefinitionOfShapes(t *testing.T) {
	obs := func(daylight bool, start string, from, to int, name string, rules ...string) Observance {
		o := Observance{Daylight: daylight, Start: civilDateTime(t, start), OffsetFrom: from, OffsetTo: to, Name: name}
		for _, r := range rules {
			o.Rules = append(o.Rules, mustRule(t, r))
		}
		return o
	}
	const h = 3600
	for _, tt := range []struct {
		name string
		def  Definition
		from time.Time
		want string
	}{{
		name: "US Eastern since 2007, from now",
		def:  usEastern(t, "US", 2007),
		from: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC),
		want: "DAYLIGHT 2007-03-11T02:00:00 -0500 -0400 \"EDT\" FREQ=YEARLY;BYDAY=2SU;BYMONTH=3\n" +
			"STANDARD 2007-11-04T02:00:00 -0400 -0500 \"EST\" FREQ=YEARLY;BYDAY=1SU;BYMONTH=11\n",
	}, {
		name: "US Eastern since 2007, from before it",
		def:  usEastern(t, "US", 2007),
		from: time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC),
		// Before 2007 the compiled zone is EST (the earliest observance's
		// TZOFFSETFROM) with no transition to start an observance at, so
		// the first one changes nothing.
		want: "STANDARD 1970-01-01T00:00:00 -0500 -0500 \"EST\"\n" +
			"DAYLIGHT 2007-03-11T02:00:00 -0500 -0400 \"EDT\" FREQ=YEARLY;BYDAY=2SU;BYMONTH=3\n" +
			"STANDARD 2007-11-04T02:00:00 -0400 -0500 \"EST\" FREQ=YEARLY;BYDAY=1SU;BYMONTH=11\n",
	}, {
		name: "a rule change, from before it",
		def: Definition{TZID: "Changed", Observances: []Observance{
			obs(false, "1990-10-28T02:00:00", -4*h, -5*h, "EST", "FREQ=YEARLY;BYMONTH=10;BYDAY=-1SU;UNTIL=20061029T060000Z"),
			obs(true, "1991-04-07T02:00:00", -5*h, -4*h, "EDT", "FREQ=YEARLY;BYMONTH=4;BYDAY=1SU;UNTIL=20060402T070000Z"),
			obs(false, "2007-11-04T02:00:00", -4*h, -5*h, "EST", "FREQ=YEARLY;BYMONTH=11;BYDAY=1SU"),
			obs(true, "2007-03-11T02:00:00", -5*h, -4*h, "EDT", "FREQ=YEARLY;BYMONTH=3;BYDAY=2SU"),
		}},
		from: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		want: "STANDARD 1990-10-28T02:00:00 -0400 -0500 \"EST\" FREQ=YEARLY;UNTIL=20061029T060000Z;BYDAY=-1SU;BYMONTH=10\n" +
			"DAYLIGHT 1991-04-07T02:00:00 -0500 -0400 \"EDT\" FREQ=YEARLY;UNTIL=20060402T070000Z;BYDAY=1SU;BYMONTH=4\n" +
			"DAYLIGHT 2007-03-11T02:00:00 -0500 -0400 \"EDT\" FREQ=YEARLY;BYDAY=2SU;BYMONTH=3\n" +
			"STANDARD 2007-11-04T02:00:00 -0400 -0500 \"EST\" FREQ=YEARLY;BYDAY=1SU;BYMONTH=11\n",
	}, {
		name: "DST abolished, with one-offs",
		def: Definition{TZID: "Abolished", Observances: []Observance{
			obs(false, "2010-10-31T03:00:00", 4*h, 3*h, "STD", "FREQ=YEARLY;BYMONTH=10;BYDAY=-1SU;UNTIL=20141026T000000Z"),
			obs(true, "2011-03-27T02:00:00", 3*h, 4*h, "DST", "FREQ=YEARLY;BYMONTH=3;BYDAY=-1SU;UNTIL=20150329T000000Z"),
			{Start: civilDateTime(t, "2016-10-30T03:00:00"), OffsetFrom: 4 * h, OffsetTo: 3 * h, Name: "STD",
				RDates: []civil.DateTime{civilDateTime(t, "2019-01-01T00:00:00")}},
			{Daylight: true, Start: civilDateTime(t, "2018-06-01T00:00:00"), OffsetFrom: 3 * h, OffsetTo: 4 * h, Name: "DST"},
		}},
		from: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		// From 2026 only the last change, standard time since 2019, is in
		// force; 2018's change to DST goes the other way, so nothing
		// earlier is traced.
		want: "STANDARD 2019-01-01T00:00:00 +0400 +0300 \"STD\"\n",
	}, {
		name: "the same, from 2012",
		def: Definition{TZID: "Abolished", Observances: []Observance{
			obs(false, "2010-10-31T03:00:00", 4*h, 3*h, "STD", "FREQ=YEARLY;BYMONTH=10;BYDAY=-1SU;UNTIL=20141026T000000Z"),
			obs(true, "2011-03-27T02:00:00", 3*h, 4*h, "DST", "FREQ=YEARLY;BYMONTH=3;BYDAY=-1SU;UNTIL=20150329T000000Z"),
			{Start: civilDateTime(t, "2016-10-30T03:00:00"), OffsetFrom: 4 * h, OffsetTo: 3 * h, Name: "STD",
				RDates: []civil.DateTime{civilDateTime(t, "2019-01-01T00:00:00")}},
			{Daylight: true, Start: civilDateTime(t, "2018-06-01T00:00:00"), OffsetFrom: 3 * h, OffsetTo: 4 * h, Name: "DST"},
		}},
		from: time.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
		want: "STANDARD 2010-10-31T03:00:00 +0400 +0300 \"STD\" FREQ=YEARLY;UNTIL=20141025T230000Z;BYDAY=-1SU;BYMONTH=10\n" +
			"DAYLIGHT 2011-03-27T02:00:00 +0300 +0400 \"DST\" FREQ=YEARLY;UNTIL=20150328T230000Z;BYDAY=-1SU;BYMONTH=3\n" +
			"STANDARD 2016-10-30T03:00:00 +0400 +0300 \"STD\" +2019-01-01T00:00:00\n" +
			"DAYLIGHT 2018-06-01T00:00:00 +0300 +0400 \"DST\"\n",
	}, {
		name: "weekdays on or after a day",
		def: Definition{TZID: "Windows", Observances: []Observance{
			// Friday on or after March 23 (Israel), and Friday on or after
			// October 26, which can be November 1 (Egypt).
			obs(true, "2010-03-26T02:00:00", 2*h, 3*h, "SUM", "FREQ=YEARLY;BYMONTH=3;BYDAY=FR;BYMONTHDAY=23,24,25,26,27,28,29"),
			obs(false, "2010-10-29T00:00:00", 3*h, 2*h, "WIN", "FREQ=YEARLY;BYDAY=FR;BYYEARDAY=-67,-66,-65,-64,-63,-62,-61"),
		}},
		from: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		want: "DAYLIGHT 2010-03-26T02:00:00 +0200 +0300 \"SUM\" FREQ=YEARLY;BYDAY=FR;BYMONTHDAY=23,24,25,26,27,28,29;BYMONTH=3\n" +
			"STANDARD 2010-10-29T00:00:00 +0300 +0200 \"WIN\" FREQ=YEARLY;BYDAY=FR;BYYEARDAY=-67,-66,-65,-64,-63,-62,-61\n",
	}, {
		// Outlook's names, which aren't POSIX TZ abbreviations, survive
		// Compile's footer, so the rules still run past the horizon.
		name: "names with spaces",
		def: Definition{TZID: "Outlook", Observances: []Observance{
			obs(false, "2007-11-04T02:00:00", -4*h, -5*h, "Eastern Standard Time", "FREQ=YEARLY;BYMONTH=11;BYDAY=1SU"),
			obs(true, "2007-03-11T02:00:00", -5*h, -4*h, "Eastern Daylight Time", "FREQ=YEARLY;BYMONTH=3;BYDAY=2SU"),
		}},
		from: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		want: "DAYLIGHT 2007-03-11T02:00:00 -0500 -0400 \"Eastern Daylight\" FREQ=YEARLY;BYDAY=2SU;BYMONTH=3\n" +
			"STANDARD 2007-11-04T02:00:00 -0400 -0500 \"Eastern Standard\" FREQ=YEARLY;BYDAY=1SU;BYMONTH=11\n",
	}, {
		name: "no transitions",
		def:  Definition{TZID: "Fixed", Observances: []Observance{obs(false, "1970-01-01T00:00:00", 5*h+1800, 5*h+1800, "XST")}},
		from: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		want: "STANDARD 1970-01-01T00:00:00 +0530 +0530 \"XST\"\n",
	}} {
		t.Run(tt.name, func(t *testing.T) {
			loc, err := Compile(tt.def)
			if err != nil {
				t.Fatal(err)
			}
			def, got, _ := roundTrip(t, loc, tt.from, beyondHorizon)
			if s := observances(def); s != tt.want {
				t.Errorf("got\n%swant\n%s", s, tt.want)
			}
			if got.String() != tt.def.TZID {
				t.Errorf("TZID %q", got)
			}
		})
	}
}

func civilDateTime(t *testing.T, s string) civil.DateTime {
	t.Helper()
	dt, err := civil.ParseDateTime(s)
	if err != nil {
		t.Fatal(err)
	}
	return dt
}

// From any time the same rules are in force, DefinitionOf gives the same
// definition, so a calendar's VTIMEZONE doesn't change as its earliest
// event moves on.
func TestDefinitionOfStable(t *testing.T) {
	loc := loadOrSkip(t, "America/New_York")
	want, err := DefinitionOf(loc, time.Date(2008, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	for y := 2009; y <= 2030; y++ {
		got, err := DefinitionOf(loc, time.Date(y, time.Month(y%12+1), 15, 0, 0, 0, 0, time.UTC))
		if err != nil {
			t.Fatal(err)
		}
		if observances(got) != observances(want) {
			t.Errorf("from %d:\n%sfrom 2008:\n%s", y, observances(got), observances(want))
		}
	}
}

// Zones without transitions, including the offsets with seconds that
// UTC-OFFSET can write (RFC 5545 §3.3.14) and the time package's own UTC.
func TestDefinitionOfFixedZones(t *testing.T) {
	for _, tt := range []struct {
		loc  *time.Location
		want string
	}{
		{time.UTC, "STANDARD 1970-01-01T00:00:00 +0000 +0000 \"UTC\"\n"},
		{time.FixedZone("LMT", -(4*3600 + 56*60 + 2)), "STANDARD 1970-01-01T00:00:00 -045602 -045602 \"LMT\"\n"},
		{time.FixedZone("", 3600), "STANDARD 1970-01-01T00:00:00 +0100 +0100 \"\"\n"},
		// Compiled, a name shorter than POSIX TZ allows stays as it is.
		{time.FixedZone("X", 3600), "STANDARD 1970-01-01T00:00:00 +0100 +0100 \"X\"\n"},
	} {
		def, _, _ := roundTrip(t, tt.loc, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), beyondHorizon)
		if s := observances(def); s != tt.want {
			t.Errorf("%s: got\n%swant\n%s", tt.loc, s, tt.want)
		}
	}
	// Before 1970, the first observance starts on the day asked for.
	def, err := DefinitionOf(time.UTC, time.Date(1901, 5, 6, 7, 8, 9, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if s, want := observances(def), "STANDARD 1901-05-06T00:00:00 +0000 +0000 \"UTC\"\n"; s != want {
		t.Errorf("got %swant %s", s, want)
	}
}

// The IANA database's negative DST, which some builds of it keep (main
// rather than rearguard data), flags Irish winter time as DST. DAYLIGHT is
// still the summer offset, as calendar programs expect.
func TestDefinitionOfNegativeDST(t *testing.T) {
	var txs []transition
	for y := 2000; y < 2010; y++ {
		summer := time.Date(y, 3, lastSunday(y, 3), 1, 0, 0, 0, time.UTC).Unix()
		winter := time.Date(y, 10, lastSunday(y, 10), 1, 0, 0, 0, time.UTC).Unix()
		txs = append(txs, transition{at: summer, offset: 3600, dst: false, name: "IST"}, transition{at: winter, offset: 0, dst: true, name: "GMT"})
	}
	data, err := encodeTZif(transition{offset: 0, dst: true, name: "GMT"}, txs, "IST-1GMT0,M10.5.0,M3.5.0/1")
	if err != nil {
		t.Fatal(err)
	}
	loc, err := time.LoadLocationFromTZData("Dublin", data)
	if err != nil {
		t.Fatal(err)
	}
	def, _, _ := roundTrip(t, loc, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), beyondHorizon)
	want := "DAYLIGHT 2000-03-26T01:00:00 +0000 +0100 \"IST\" FREQ=YEARLY;BYDAY=-1SU;BYMONTH=3\n" +
		"STANDARD 2000-10-29T02:00:00 +0100 +0000 \"GMT\" FREQ=YEARLY;BYDAY=-1SU;BYMONTH=10\n"
	if s := observances(def); s != want {
		t.Errorf("got\n%swant\n%s", s, want)
	}
}

// A change out of DST to a new, higher standard time, for good, keeps its
// flags: only a change the zone undoes is negative DST.
func TestDefinitionOfLeavingDSTForGood(t *testing.T) {
	var txs []transition
	for y := 2000; y < 2005; y++ {
		txs = append(txs,
			transition{at: time.Date(y, 3, lastSunday(y, 3), 1, 0, 0, 0, time.UTC).Unix(), offset: 3600, dst: true, name: "DST"},
			transition{at: time.Date(y, 10, lastSunday(y, 10), 1, 0, 0, 0, time.UTC).Unix(), offset: 0, name: "STD"})
	}
	txs = append(txs,
		transition{at: time.Date(2005, 3, 27, 1, 0, 0, 0, time.UTC).Unix(), offset: 3600, dst: true, name: "DST"},
		transition{at: time.Date(2005, 6, 1, 0, 0, 0, 0, time.UTC).Unix(), offset: 7200, name: "NEW"})
	data, err := encodeTZif(transition{name: "STD"}, txs, "")
	if err != nil {
		t.Fatal(err)
	}
	loc, err := time.LoadLocationFromTZData("Moved", data)
	if err != nil {
		t.Fatal(err)
	}
	def, _, _ := roundTrip(t, loc, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), beyondHorizon)
	if s, want := observances(def), "STANDARD 2005-06-01T01:00:00 +0100 +0200 \"NEW\"\n"; s != want {
		t.Errorf("got\n%swant\n%s", s, want)
	}
}

// Rules that begin just before the horizon have too few years behind them
// to be trusted past it, so they end there, with UNTIL.
func TestDefinitionOfLateRulesEnd(t *testing.T) {
	loc, err := Compile(usEastern(t, "Late", 2195))
	if err != nil {
		t.Fatal(err)
	}
	def, _, _ := roundTrip(t, loc, time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(Horizon+1, 1, 1, 0, 0, 0, 0, time.UTC))
	if len(def.Observances) < 3 {
		t.Fatalf("%d observances, want the rules from 2195 after the first", len(def.Observances))
	}
	for _, o := range def.Observances {
		for _, r := range o.Rules {
			if r.Until.IsZero() {
				t.Errorf("%s from %v has no end", r, o.Start)
			}
		}
	}
}

// DefinitionOf's cache hands out copies: changing one doesn't change the
// next.
func TestDefinitionOfCopies(t *testing.T) {
	loc, err := Compile(usEastern(t, "Copied", 2007))
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a, err := DefinitionOf(loc, from)
	if err != nil {
		t.Fatal(err)
	}
	want := observances(a)
	a.Observances[0].Rules[0].ByMonth[0] = 12
	a.Observances[1].Name = "changed"
	b, err := DefinitionOf(loc, from.AddDate(0, 1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if got := observances(b); got != want {
		t.Errorf("after changing a result:\n%swant\n%s", got, want)
	}
}

func lastSunday(y int, m time.Month) int {
	last := time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC)
	return last.Day() - int(last.Weekday())
}

// Past a zone's explicit transitions, the time package's ZoneBounds gives
// the start of each year as a bound and, on December 31 of a leap year,
// an end that doesn't advance; walking by it naively never finishes.
// The zone here has one explicit transition, in 2000, so 2040 is always
// past it, on any host.
func TestNextChangePastExplicitTransitions(t *testing.T) {
	data, err := encodeTZif(transition{offset: -5 * 3600, name: "EST"},
		[]transition{{at: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC).Unix(), offset: -5 * 3600, name: "EST"}},
		"EST5EDT,M3.2.0,M11.1.0")
	if err != nil {
		t.Fatal(err)
	}
	loc, err := time.LoadLocationFromTZData("Eastern", data)
	if err != nil {
		t.Fatal(err)
	}
	dec31 := time.Date(2040, 12, 31, 6, 0, 0, 0, time.UTC)
	if _, end := dec31.In(loc).ZoneBounds(); end.After(dec31) {
		t.Log("ZoneBounds advances on December 31 of a leap year: the time package no longer has the bug")
	}
	end := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	got := nextChange(loc, dec31, end)
	if want := time.Date(2041, 3, 10, 7, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("next change after %v: %v, want %v", dec31, got, want)
	}
	if got := lastChange(loc, dec31); !got.Equal(time.Date(2040, 11, 4, 6, 0, 0, 0, time.UTC)) {
		t.Errorf("last change before %v: %v", dec31, got)
	}
}

func TestDefinitionOfRefuses(t *testing.T) {
	if _, err := DefinitionOf(nil, time.Now()); err == nil {
		t.Error("nil location")
	}
	if _, err := DefinitionOf(time.UTC, time.Date(0, 6, 1, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Error("year 0")
	}
	if _, err := DefinitionOf(time.FixedZone("Far", 25*3600), time.Now()); err == nil {
		t.Error("an offset of 25 hours")
	}
}

// The POSIX TZ rules Compile writes for the weekday windows DefinitionOf
// uses agree with the IANA database's own spelling where it has one.
func TestPosixFooterWindows(t *testing.T) {
	for _, tt := range []struct {
		start string
		rule  string
		want  string
	}{
		{"2013-03-29T02:00:00", "FREQ=YEARLY;BYMONTH=3;BYDAY=FR;BYMONTHDAY=23,24,25,26,27,28,29", "M3.4.4/26"},     // Jerusalem
		{"2023-09-03T00:00:00", "FREQ=YEARLY;BYMONTH=9;BYDAY=SU;BYMONTHDAY=2,3,4,5,6,7,8", "M9.1.6/24"},            // Santiago
		{"2024-03-30T23:00:00", "FREQ=YEARLY;BYMONTH=3;BYDAY=SA;BYMONTHDAY=24,25,26,27,28,29,30", "M3.4.4/71"},     // Nuuk: M3.5.0/-1
		{"2023-10-27T00:00:00", "FREQ=YEARLY;BYDAY=FR;BYYEARDAY=-67,-66,-65,-64,-63,-62,-61", "M10.4.1/96"},        // Cairo: M10.5.4/24
		{"2023-01-27T02:00:00", "FREQ=YEARLY;BYDAY=FR;BYYEARDAY=26,27,28,29,30,31,32", "M1.4.1/98"},                // Jan 26 to Feb 1
		{"2023-04-29T02:00:00", "FREQ=YEARLY;BYDAY=SA;BYYEARDAY=-247,-246,-245,-244,-243,-242,-241", "M4.5.1/122"}, // Apr 29 to May 5
		{"2023-02-24T02:00:00", "FREQ=YEARLY;BYMONTH=2;BYDAY=FR;BYMONTHDAY=23,24,25,26,27,28,29", ""},              // not every February has the 29th
		{"2023-02-24T02:00:00", "FREQ=YEARLY;BYDAY=FR;BYYEARDAY=54,55,56,57,58,59,60", ""},                         // across February 29
		{"2023-03-24T02:00:00", "FREQ=YEARLY;BYMONTH=3;BYDAY=FR;BYMONTHDAY=23,24,25,26,27,28", ""},                 // six days
	} {
		o := Observance{Start: civilDateTime(t, tt.start), Rules: []recur.Rule{mustRule(t, tt.rule)}}
		got, ok := posixRule(&o)
		if !ok {
			got = ""
		}
		if got != tt.want {
			t.Errorf("%s: %q, want %q", tt.rule, got, tt.want)
		}
		if tt.want == "" {
			continue
		}
		// The POSIX rule gives the RRULE's dates, for a run of years.
		loc, err := time.LoadLocationFromTZData("W", mustTZif(t, "<A>0<B>-1,"+tt.want+",M12.5.0/23"))
		if err != nil {
			t.Fatal(err)
		}
		set := recur.Set{Start: o.Start, Location: time.UTC, Rules: o.Rules}
		n := 0
		for on := range set.Between(o.Start, civilDateTime(t, "2080-01-01T00:00:00")) {
			at := on.In(time.UTC)
			if _, off := at.Add(-time.Second).In(loc).Zone(); off != 0 {
				t.Errorf("%s: %v: already changed", tt.want, at)
			}
			if _, off := at.In(loc).Zone(); off != 3600 {
				t.Errorf("%s: %v: no change", tt.want, at)
			}
			n++
		}
		if n != 2080-o.Start.Year {
			t.Errorf("%s: %d onsets", tt.rule, n)
		}
	}
}

// mustTZif returns a TZif file with one transition in 1970 and footer,
// so the time package follows footer from then on.
func mustTZif(t *testing.T, footer string) []byte {
	t.Helper()
	b, err := encodeTZif(transition{name: "A"}, []transition{{at: 0, name: "A"}}, footer)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func BenchmarkDefinitionOf(b *testing.B) {
	loc, err := time.LoadLocation("America/Chicago")
	if err != nil {
		b.Skip(err)
	}
	from := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	for b.Loop() {
		if _, err := DefinitionOf(loc, from); err != nil {
			b.Fatal(err)
		}
	}
}
