package ical_test

// Conformance tests for VTIMEZONE (RFC 5545 §3.6.5) and TZID resolution
// (§3.2.19, §3.3.5). Every expected offset and abbreviation is derived by
// hand from the RFC's text and examples: the onsets come from each
// observance's DTSTART, RRULE and RDATE read on its TZOFFSETFROM clock,
// computed below with the time package's weekday arithmetic only.

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	ical "github.com/aanelson/iCalendar"
	"github.com/aanelson/iCalendar/tz"
)

const (
	ccEST = -5 * 3600
	ccEDT = -4 * 3600
)

// ccCal parses a calendar written as "\n"-separated lines, wrapping them
// in a VCALENDAR with the PRODID and VERSION §3.6 requires.
func ccCal(t *testing.T, body string) *ical.Calendar {
	t.Helper()
	return ccRaw(t, "BEGIN:VCALENDAR\nPRODID:-//conformance//EN\nVERSION:2.0\n"+
		strings.Trim(body, "\n")+"\nEND:VCALENDAR\n")
}

// ccRaw parses a whole iCalendar object written with "\n" line ends.
func ccRaw(t *testing.T, text string) *ical.Calendar {
	t.Helper()
	cal, err := ical.Parse(strings.NewReader(ccCRLF(text)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return cal
}

func ccCRLF(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\n", "\r\n")
}

// ccFixture parses testdata/rfc5545/ics/<name>.ics, the RFC's own examples.
func ccFixture(t *testing.T, name string) *ical.Calendar {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "rfc5545", "ics", name+".ics"))
	if err != nil {
		t.Fatal(err)
	}
	return ccRaw(t, string(b))
}

// ccZone compiles the calendar's VTIMEZONE whose TZID is tzid.
func ccZone(t *testing.T, cal *ical.Calendar, tzid string) *time.Location {
	t.Helper()
	for z := range cal.Timezones() {
		if z.TZID() == tzid {
			loc, err := z.Location()
			if err != nil {
				t.Fatalf("VTIMEZONE %s: %v", tzid, err)
			}
			return loc
		}
	}
	t.Fatalf("no VTIMEZONE %s", tzid)
	return nil
}

// ccUTC reads a UTC instant written as 2006-01-02T15:04:05Z.
func ccUTC(s string) time.Time {
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return v.UTC()
}

// ccSunday returns the day of the month of the nth Sunday of month in
// year, counting from the end when n is negative (-1 is the last).
func ccSunday(year int, month time.Month, n int) int {
	if n > 0 {
		first := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC).Weekday()
		return 1 + (7-int(first))%7 + 7*(n-1)
	}
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC)
	return last.Day() - int(last.Weekday()) + 7*(n+1)
}

// A ccTransition is an onset: at the instant At the offset changes from
// FromOff (abbreviation FromName) to ToOff (ToName). An empty name isn't
// checked.
type ccTransition struct {
	At               time.Time
	FromOff, ToOff   int
	FromName, ToName string
}

// ccOnset returns the instant of an onset written as local wall-clock
// time on the observance's TZOFFSETFROM clock, as §3.6.5 says: "TZOFFSETFROM
// is combined with DTSTART to define the effective onset".
func ccOnset(year int, month time.Month, day, hour int, from int) time.Time {
	return time.Date(year, month, day, hour, 0, 0, 0, time.UTC).Add(-time.Duration(from) * time.Second)
}

func ccCheckTransitions(t *testing.T, loc *time.Location, trs []ccTransition) {
	t.Helper()
	for _, tr := range trs {
		ccCheckZone(t, loc, tr.At.Add(-time.Second), tr.FromOff, tr.FromName)
		ccCheckZone(t, loc, tr.At, tr.ToOff, tr.ToName)
	}
}

func ccCheckZone(t *testing.T, loc *time.Location, at time.Time, off int, name string) {
	t.Helper()
	gotName, gotOff := at.In(loc).Zone()
	if gotOff != off || (name != "" && gotName != name) {
		want := name
		if want == "" {
			want = "(any name)"
		}
		t.Errorf("%s at %s: got %s %+d, want %s %+d", loc, at.UTC().Format(time.RFC3339), gotName, gotOff, want, off)
	}
}

// §3.6.5, first example: "all the time zone rules for New York City since
// April 30, 1967". The onsets are those of the table in §3.6.5, read from
// the observances: DAYLIGHT last Sunday in April 1967-1973 (UNTIL
// 19730429T070000Z), STANDARD last Sunday in October 1967-2006 (UNTIL
// 20061029T060000Z), DAYLIGHT on 1974-01-06 (DTSTART) and 1975-02-23
// (RDATE), DAYLIGHT last Sunday in April 1976-1986, DAYLIGHT first Sunday
// in April 1987-2006, and from 2007 DAYLIGHT on the second Sunday in March
// and STANDARD on the first Sunday in November, with no end. Every onset
// is at 02:00 on the TZOFFSETFROM clock (§3.6.5: "The DTSTART and the
// TZOFFSETFROM properties MUST be used when generating the onset
// DATE-TIME values"; RDATE "MUST be specified as a date with local time
// value, relative to the UTC offset specified in the TZOFFSETFROM").
func TestConformanceVTimezoneNewYorkHistory(t *testing.T) {
	cal := ccFixture(t, "rfc5545-3.6.5-c")
	loc := ccZone(t, cal, "America/New_York")

	var trs []ccTransition
	dst := func(y int, m time.Month, d int) {
		trs = append(trs, ccTransition{ccOnset(y, m, d, 2, ccEST), ccEST, ccEDT, "EST", "EDT"})
	}
	std := func(y int, m time.Month, d int) {
		trs = append(trs, ccTransition{ccOnset(y, m, d, 2, ccEDT), ccEDT, ccEST, "EDT", "EST"})
	}
	for y := 1967; y <= 1973; y++ {
		dst(y, time.April, ccSunday(y, time.April, -1))
	}
	for y := 1967; y <= 2006; y++ {
		std(y, time.October, ccSunday(y, time.October, -1))
	}
	dst(1974, time.January, 6)
	dst(1975, time.February, 23)
	for y := 1976; y <= 1986; y++ {
		dst(y, time.April, ccSunday(y, time.April, -1))
	}
	for y := 1987; y <= 2006; y++ {
		dst(y, time.April, ccSunday(y, time.April, 1))
	}
	// The open-ended rules: checked well past any precomputed horizon,
	// since nothing in the VTIMEZONE ends them.
	for _, y := range []int{2007, 2008, 2009, 2010, 2020, 2024, 2025, 2037, 2038, 2050, 2100, 2199, 2200, 2201, 2210, 2400} {
		dst(y, time.March, ccSunday(y, time.March, 2))
		std(y, time.November, ccSunday(y, time.November, 1))
	}
	slices.SortFunc(trs, func(a, b ccTransition) int { return a.At.Compare(b.At) })

	// Before the first onset (1967-04-30) the RFC is silent. The
	// architecture doc's choice: the zone keeps the first onset's
	// TZOFFSETFROM (-0500 here); its abbreviation isn't checked.
	trs[0].FromName = ""
	ccCheckTransitions(t, loc, trs)

	// The UNTIL of the first DAYLIGHT rule equals its last onset, which
	// is therefore included (§3.6.5: "the UNTIL DATE-TIME will be equal to
	// the last instance generated by the recurrence pattern").
	ccCheckZone(t, loc, ccUTC("1973-04-29T07:00:00Z"), ccEDT, "EDT")
	// The STANDARD rule's UNTIL (20061029T060000Z) ends it: no onset on
	// the last Sunday of October 2007 (Oct 28), so EDT lasts to Nov 4.
	ccCheckZone(t, loc, ccUTC("2007-10-30T12:00:00Z"), ccEDT, "EDT")
	// 1974 and 1975 had daylight time from January and February.
	ccCheckZone(t, loc, ccUTC("1974-03-01T12:00:00Z"), ccEDT, "EDT")
	ccCheckZone(t, loc, ccUTC("1975-03-01T12:00:00Z"), ccEDT, "EDT")
	ccCheckZone(t, loc, ccUTC("1973-12-01T12:00:00Z"), ccEST, "EST")
}

// §3.6.5, second example: New York "using only the DTSTART property",
// onsets 2007-03-11 02:00 EST and 2007-11-04 02:00 EDT. By §3.6.5 ("the
// observance that has the last onset date and time before the time in
// question") the zone is EST from 2007-11-04 on, however far.
func TestConformanceVTimezoneDTStartOnly(t *testing.T) {
	cal := ccFixture(t, "rfc5545-3.6.5-d")
	loc := ccZone(t, cal, "America/New_York")
	ccCheckTransitions(t, loc, []ccTransition{
		// Before the first onset: RFC silent; the documented choice is
		// that onset's TZOFFSETFROM (-0500), name unchecked.
		{ccUTC("2007-03-11T07:00:00Z"), ccEST, ccEDT, "", "EDT"},
		{ccUTC("2007-11-04T06:00:00Z"), ccEDT, ccEST, "EDT", "EST"},
		// No rule, so no onset in 2008.
		{ccUTC("2008-03-09T07:00:00Z"), ccEST, ccEST, "EST", "EST"},
	})
	ccCheckZone(t, loc, ccUTC("2008-07-01T12:00:00Z"), ccEST, "EST")
	ccCheckZone(t, loc, ccUTC("2030-07-01T12:00:00Z"), ccEST, "EST")
}

// §3.6.5, third example: the current New York rules as open-ended RRULEs
// (second Sunday in March, first Sunday in November, from 2007), with a
// TZURL, which is informational (§3.8.3.5) and must not change anything.
func TestConformanceVTimezoneOpenEndedRules(t *testing.T) {
	cal := ccFixture(t, "rfc5545-3.6.5-e")
	loc := ccZone(t, cal, "America/New_York")
	var trs []ccTransition
	for _, y := range []int{2007, 2008, 2011, 2015, 2024, 2025, 2026, 2037, 2038, 2039, 2099, 2100, 2199, 2200, 2201, 2250, 2399, 2400} {
		trs = append(trs,
			ccTransition{ccOnset(y, time.March, ccSunday(y, time.March, 2), 2, ccEST), ccEST, ccEDT, "EST", "EDT"},
			ccTransition{ccOnset(y, time.November, ccSunday(y, time.November, 1), 2, ccEDT), ccEDT, ccEST, "EDT", "EST"})
	}
	trs[0].FromName = "" // before the first onset: RFC silent
	ccCheckTransitions(t, loc, trs)
}

// §3.6.5, fourth example: "a fictitious time zone where the Daylight Time
// rule has an effective end date (i.e., after that date, Daylight Time is
// no longer observed)". STANDARD: last Sunday in October from 1967, no
// end. DAYLIGHT: first Sunday in April from 1987, UNTIL 19980404T070000Z,
// so its last onset is 1997-04-06 (the first Sunday in April 1998 is
// April 5, after UNTIL).
func TestConformanceVTimezoneFictitiousDaylightEnds(t *testing.T) {
	cal := ccFixture(t, "rfc5545-3.6.5-f")
	loc := ccZone(t, cal, "Fictitious")
	var trs []ccTransition
	for y := 1987; y <= 1997; y++ {
		trs = append(trs,
			ccTransition{ccOnset(y, time.April, ccSunday(y, time.April, 1), 2, ccEST), ccEST, ccEDT, "EST", "EDT"},
			ccTransition{ccOnset(y, time.October, ccSunday(y, time.October, -1), 2, ccEDT), ccEDT, ccEST, "EDT", "EST"})
	}
	ccCheckTransitions(t, loc, trs)
	// From 1967 to 1987 only STANDARD onsets: EST all year.
	ccCheckZone(t, loc, ccUTC("1980-07-01T12:00:00Z"), ccEST, "EST")
	// Daylight time is no longer observed after 1997.
	for _, s := range []string{"1998-04-05T07:00:00Z", "1998-07-01T12:00:00Z", "2005-07-01T12:00:00Z", "2150-07-01T12:00:00Z", "2300-07-01T12:00:00Z"} {
		ccCheckZone(t, loc, ccUTC(s), ccEST, "EST")
	}
}

// §3.6.5, fifth example: as the fourth, plus a second DAYLIGHT rule that
// "picks up where the other left off": DTSTART 19990424T020000, then the
// last Sunday in April. DTSTART is the first onset (§3.6.5: "the first
// onset DATE-TIME for the observance"), so daylight time starts on
// 1999-04-24 at 02:00 EST.
func TestConformanceVTimezoneFictitiousDaylightResumes(t *testing.T) {
	cal := ccFixture(t, "rfc5545-3.6.5-g")
	loc := ccZone(t, cal, "Fictitious")
	trs := []ccTransition{
		{ccOnset(1997, time.April, 6, 2, ccEST), ccEST, ccEDT, "EST", "EDT"},
		{ccOnset(1997, time.October, 26, 2, ccEDT), ccEDT, ccEST, "EDT", "EST"},
		{ccOnset(1999, time.April, 24, 2, ccEST), ccEST, ccEDT, "EST", "EDT"},
		{ccOnset(1999, time.October, 31, 2, ccEDT), ccEDT, ccEST, "EDT", "EST"},
	}
	for _, y := range []int{2000, 2001, 2010, 2024, 2100, 2199, 2200, 2201, 2300} {
		trs = append(trs,
			ccTransition{ccOnset(y, time.April, ccSunday(y, time.April, -1), 2, ccEST), ccEST, ccEDT, "EST", "EDT"},
			ccTransition{ccOnset(y, time.October, ccSunday(y, time.October, -1), 2, ccEDT), ccEDT, ccEST, "EDT", "EST"})
	}
	ccCheckTransitions(t, loc, trs)
	ccCheckZone(t, loc, ccUTC("1998-07-01T12:00:00Z"), ccEST, "EST")
	ccCheckZone(t, loc, ccUTC("1999-04-25T12:00:00Z"), ccEDT, "EDT")
}

// A zone exported by Outlook, which names custom zones "Customized Time
// Zone" and starts every observance in 1601 with an annual BYDAY/BYMONTH
// rule. Here: +0100 standard, +0200 daylight from the last Sunday in March
// at 02:00 (+0100) to the last Sunday in October at 03:00 (+0200), both
// 01:00 UTC. No TZNAME, so abbreviations aren't checked (TZNAME is
// OPTIONAL, §3.6.5).
func TestConformanceVTimezoneOutlookCustom(t *testing.T) {
	cal := ccCal(t, `BEGIN:VTIMEZONE
TZID:Customized Time Zone
BEGIN:STANDARD
DTSTART:16010101T030000
TZOFFSETFROM:+0200
TZOFFSETTO:+0100
RRULE:FREQ=YEARLY;INTERVAL=1;BYDAY=-1SU;BYMONTH=10
END:STANDARD
BEGIN:DAYLIGHT
DTSTART:16010101T020000
TZOFFSETFROM:+0100
TZOFFSETTO:+0200
RRULE:FREQ=YEARLY;INTERVAL=1;BYDAY=-1SU;BYMONTH=3
END:DAYLIGHT
END:VTIMEZONE
BEGIN:VEVENT
UID:outlook@example.com
DTSTAMP:20240101T000000Z
DTSTART;TZID=Customized Time Zone:20240701T090000
DTEND;TZID=Customized Time Zone:20240701T100000
END:VEVENT`)
	loc := ccZone(t, cal, "Customized Time Zone")
	const cet, cest = 3600, 7200
	var trs []ccTransition
	for _, y := range []int{1990, 2023, 2024, 2025, 2026, 2100, 2250} {
		trs = append(trs,
			ccTransition{ccOnset(y, time.March, ccSunday(y, time.March, -1), 2, cet), cet, cest, "", ""},
			ccTransition{ccOnset(y, time.October, ccSunday(y, time.October, -1), 3, cest), cest, cet, "", ""})
	}
	ccCheckTransitions(t, loc, trs)

	// The event's TZID resolves to this VTIMEZONE (§3.2.19).
	for ev := range cal.Events() {
		start, err := ev.Start()
		if err != nil {
			t.Fatal(err)
		}
		if got, want := start.Time(), ccUTC("2024-07-01T07:00:00Z"); !got.Equal(want) {
			t.Errorf("Start %s, want %s", got, want)
		}
	}
}

// The southern-hemisphere shape of the same Outlook zone: daylight time
// spans the new year (+1000 standard from the first Sunday in April at
// 03:00, +1100 from the first Sunday in October at 02:00).
func TestConformanceVTimezoneSouthernHemisphere(t *testing.T) {
	cal := ccCal(t, `BEGIN:VTIMEZONE
TZID:Customized Time Zone 1
BEGIN:STANDARD
DTSTART:16010101T030000
TZOFFSETFROM:+1100
TZOFFSETTO:+1000
RRULE:FREQ=YEARLY;INTERVAL=1;BYDAY=1SU;BYMONTH=4
END:STANDARD
BEGIN:DAYLIGHT
DTSTART:16010101T020000
TZOFFSETFROM:+1000
TZOFFSETTO:+1100
RRULE:FREQ=YEARLY;INTERVAL=1;BYDAY=1SU;BYMONTH=10
END:DAYLIGHT
END:VTIMEZONE`)
	loc := ccZone(t, cal, "Customized Time Zone 1")
	const aest, aedt = 10 * 3600, 11 * 3600
	var trs []ccTransition
	for _, y := range []int{2023, 2024, 2025, 2100, 2250} {
		trs = append(trs,
			ccTransition{ccOnset(y, time.April, ccSunday(y, time.April, 1), 3, aedt), aedt, aest, "", ""},
			ccTransition{ccOnset(y, time.October, ccSunday(y, time.October, 1), 2, aest), aest, aedt, "", ""})
	}
	ccCheckTransitions(t, loc, trs)
	ccCheckZone(t, loc, ccUTC("2025-01-01T00:00:00Z"), aedt, "")
	ccCheckZone(t, loc, ccUTC("2025-07-01T00:00:00Z"), aest, "")
}

// A fixed-offset zone: one STANDARD observance and no rules. From its
// onset on, its TZOFFSETTO and TZNAME apply (§3.6.5). The offset has a
// non-hour part, which UTC-OFFSET allows (§3.3.14).
func TestConformanceVTimezoneFixedOffset(t *testing.T) {
	cal := ccCal(t, `BEGIN:VTIMEZONE
TZID:Custom Nepal
BEGIN:STANDARD
DTSTART:19860101T000000
TZOFFSETFROM:+0530
TZOFFSETTO:+0545
TZNAME:NPT
END:STANDARD
END:VTIMEZONE`)
	loc := ccZone(t, cal, "Custom Nepal")
	const npt = 5*3600 + 45*60
	// Onset 1986-01-01 00:00 at +0530 is 1985-12-31 18:30 UTC. Before it,
	// RFC silent; the documented choice is TZOFFSETFROM.
	ccCheckTransitions(t, loc, []ccTransition{
		{ccUTC("1985-12-31T18:30:00Z"), 5*3600 + 30*60, npt, "", "NPT"},
	})
	for _, s := range []string{"1990-01-01T00:00:00Z", "2024-06-01T00:00:00Z", "2024-12-01T00:00:00Z", "2300-06-01T00:00:00Z"} {
		ccCheckZone(t, loc, ccUTC(s), npt, "NPT")
	}
}

// §3.6.5: TZURL, LAST-MODIFIED (and RFC 7808's TZUNTIL, which RFC 5545
// would read as an iana-prop) carry no offset information, and COMMENT,
// X- properties and a second TZNAME don't either. A zone with all of them
// gives the same offsets and names as one without.
func TestConformanceVTimezoneInformationalProperties(t *testing.T) {
	bare := ccZone(t, ccFixture(t, "rfc5545-3.6.5-e"), "America/New_York")
	rich := ccZone(t, ccCal(t, `BEGIN:VTIMEZONE
TZID:America/New_York
LAST-MODIFIED:20050809T050000Z
TZURL:http://zones.example.com/tz/America-New_York.ics
TZUNTIL:20080101T000000Z
X-LIC-LOCATION:America/New_York
COMMENT:An informational comment
BEGIN:STANDARD
DTSTART:20071104T020000
RRULE:FREQ=YEARLY;BYMONTH=11;BYDAY=1SU
TZOFFSETFROM:-0400
TZOFFSETTO:-0500
TZNAME:EST
TZNAME;LANGUAGE=fr-CA:HNE
COMMENT:standard time
END:STANDARD
BEGIN:DAYLIGHT
DTSTART:20070311T020000
RRULE:FREQ=YEARLY;BYMONTH=3;BYDAY=2SU
TZOFFSETFROM:-0500
TZOFFSETTO:-0400
TZNAME:EDT
X-EXTRA:ignored
END:DAYLIGHT
END:VTIMEZONE`), "America/New_York")
	for at := ccUTC("2007-03-11T07:00:00Z"); at.Year() < 2031; at = at.Add(97 * time.Hour) {
		n1, o1 := at.In(bare).Zone()
		n2, o2 := at.In(rich).Zone()
		if n1 != n2 || o1 != o2 {
			t.Fatalf("at %s: %s %d without informational properties, %s %d with them", at, n1, o1, n2, o2)
		}
	}
}

// §3.6.5: "The VTIMEZONE calendar component MUST include the TZID
// property and at least one definition of a STANDARD or DAYLIGHT
// sub-component. The STANDARD or DAYLIGHT sub-component MUST include the
// DTSTART, TZOFFSETFROM, and TZOFFSETTO properties." Without an
// observance, its onset or its offset, no zone can be built, so compiling
// must fail rather than invent one. A missing TZOFFSETFROM or TZID still
// leaves a usable zone; compiling one anyway is a lenient choice, and
// reporting it is schema validation, which is planned.
func TestConformanceVTimezoneRequiredParts(t *testing.T) {
	tests := []struct {
		name, body string
		gap        string
	}{
		{"no observance", "BEGIN:VTIMEZONE\nTZID:Empty\nEND:VTIMEZONE", ""},
		{"no TZOFFSETTO", "BEGIN:VTIMEZONE\nTZID:X\nBEGIN:STANDARD\nDTSTART:19700101T000000\nTZOFFSETFROM:+0100\nEND:STANDARD\nEND:VTIMEZONE", ""},
		{"no DTSTART", "BEGIN:VTIMEZONE\nTZID:X\nBEGIN:STANDARD\nTZOFFSETFROM:+0100\nTZOFFSETTO:+0100\nEND:STANDARD\nEND:VTIMEZONE", ""},
		{"no TZOFFSETFROM", "BEGIN:VTIMEZONE\nTZID:X\nBEGIN:STANDARD\nDTSTART:19700101T000000\nTZOFFSETTO:+0100\nEND:STANDARD\nEND:VTIMEZONE",
			"§3.6.5 an observance without TZOFFSETFROM compiles, reading it as TZOFFSETTO, with no error or warning"},
		{"no TZID", "BEGIN:VTIMEZONE\nBEGIN:STANDARD\nDTSTART:19700101T000000\nTZOFFSETFROM:+0100\nTZOFFSETTO:+0100\nEND:STANDARD\nEND:VTIMEZONE",
			"§3.6.5 a VTIMEZONE without TZID compiles (to a zone no TZID can name), with no error or warning"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cal := ccCal(t, tt.body)
			for z := range cal.Timezones() {
				loc, err := z.Location()
				switch {
				case err != nil:
					// The doc promises errors.Is for a definition that
					// can't be compiled.
					if !errors.Is(err, tz.ErrInvalidDefinition) {
						t.Errorf("Location() error %v doesn't wrap tz.ErrInvalidDefinition", err)
					}
				case tt.gap != "":
					ccGap(t, tt.gap+" (schema validation is planned)")
				default:
					t.Errorf("Location() = %v, nil; want an error", loc)
				}
			}
		})
	}
}

// §3.2.19: the TZID parameter "uniquely identifies the VTIMEZONE calendar
// component to be used when evaluating the time portion of the property".
// The calendar's own VTIMEZONE is used even when its TZID is also an IANA
// name: here a deliberately different "America/New_York" at +0300.
func TestConformanceTZIDUsesCalendarVTimezone(t *testing.T) {
	cal := ccCal(t, `BEGIN:VTIMEZONE
TZID:America/New_York
BEGIN:STANDARD
DTSTART:19700101T000000
TZOFFSETFROM:+0300
TZOFFSETTO:+0300
TZNAME:XST
END:STANDARD
END:VTIMEZONE
BEGIN:VEVENT
UID:a@example.com
DTSTAMP:20240101T000000Z
DTSTART;TZID=America/New_York:20240701T090000
DTEND;TZID=America/New_York:20240701T100000
RRULE:FREQ=DAILY;COUNT=2
EXDATE;TZID=America/New_York:20240702T090000
RDATE;TZID=America/New_York:20240710T090000
END:VEVENT`)
	ev := ccFirstEvent(t, cal)
	start, err := ev.Start()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := start.Time(), ccUTC("2024-07-01T06:00:00Z"); !got.Equal(want) {
		t.Errorf("Start %s, want %s", got.UTC(), want)
	}
	if name, _ := start.Time().Zone(); name != "XST" {
		t.Errorf("Start zone %s, want XST", name)
	}
	end, err := ev.End()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := end.Time(), ccUTC("2024-07-01T07:00:00Z"); !got.Equal(want) {
		t.Errorf("End %s, want %s", got.UTC(), want)
	}
	// RDATE and EXDATE with the TZID resolve the same way.
	got := ccStarts(t, &ical.Expander{}, cal, "2024-06-01T00:00:00Z", "2024-08-01T00:00:00Z")
	want := []string{"2024-07-01T06:00:00Z", "2024-07-10T06:00:00Z"}
	if !slices.Equal(got, want) {
		t.Errorf("occurrences %v, want %v", got, want)
	}
}

// §3.3.5: "The use of local time in a DATE-TIME value without the TZID
// property parameter is to be interpreted as floating time, regardless of
// the existence of VTIMEZONE calendar components in the iCalendar
// object." So neither the calendar's only VTIMEZONE nor anything else
// gives the value a zone.
func TestConformanceFloatingIgnoresVTimezone(t *testing.T) {
	cal := ccCal(t, `BEGIN:VTIMEZONE
TZID:Only Zone
BEGIN:STANDARD
DTSTART:19700101T000000
TZOFFSETFROM:+0300
TZOFFSETTO:+0300
END:STANDARD
END:VTIMEZONE
BEGIN:VEVENT
UID:f@example.com
DTSTAMP:20240101T000000Z
DTSTART:20240701T090000
DTEND:20240701T100000
END:VEVENT`)
	ev := ccFirstEvent(t, cal)
	start, err := ev.Start()
	if err != nil {
		t.Fatal(err)
	}
	if !start.Floating() || start.TZID != "" || start.Loc != nil || start.Location() != nil {
		t.Errorf("Start %+v: want a floating value with no zone", start)
	}
	// The Expander reads floating values in its Floating zone.
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skip(err)
	}
	x := &ical.Expander{Floating: paris}
	got := ccStarts(t, x, cal, "2024-07-01T00:00:00Z", "2024-07-02T00:00:00Z")
	if want := []string{"2024-07-01T07:00:00Z"}; !slices.Equal(got, want) {
		t.Errorf("occurrences in Europe/Paris %v, want %v", got, want)
	}
}

// §3.2.19: "An individual VTIMEZONE calendar component MUST be specified
// for each unique TZID parameter value". A TZID that neither the calendar
// nor any registry defines can't be evaluated: the library must report an
// error, never read the time as UTC or in the host's zone. The documented
// fallback for a TZID without a VTIMEZONE is tz.Load (IANA, Windows), which
// doesn't know this name either.
func TestConformanceUnknownTZIDIsAnError(t *testing.T) {
	cal := ccCal(t, `BEGIN:VEVENT
UID:bad@example.com
DTSTAMP:20240101T000000Z
DTSTART;TZID=Nowhere/Imaginary:20240701T090000
DTEND;TZID=Nowhere/Imaginary:20240701T100000
END:VEVENT
BEGIN:VEVENT
UID:good@example.com
DTSTAMP:20240101T000000Z
DTSTART:20240701T120000Z
END:VEVENT`)
	ev := ccFirstEvent(t, cal)
	if _, err := ev.Start(); !errors.Is(err, tz.ErrUnknownZone) {
		t.Errorf("Start error %v, want one wrapping tz.ErrUnknownZone", err)
	}
	if _, err := ev.End(); err == nil {
		t.Errorf("End: no error for an unknown TZID")
	}
	if _, err := cal.Resolve("Nowhere/Imaginary"); !errors.Is(err, tz.ErrUnknownZone) {
		t.Errorf("Resolve error %v, want one wrapping tz.ErrUnknownZone", err)
	}
	var starts []string
	var errs []error
	for occ, err := range cal.Occurrences(ccUTC("2024-06-01T00:00:00Z"), ccUTC("2024-08-01T00:00:00Z")) {
		if err != nil {
			errs = append(errs, err)
			continue
		}
		starts = append(starts, occ.Start.Time().UTC().Format(time.RFC3339))
	}
	if len(errs) != 1 || !errors.Is(errs[0], tz.ErrUnknownZone) {
		t.Errorf("errors %v, want one wrapping tz.ErrUnknownZone", errs)
	}
	// The other event is unaffected.
	if want := []string{"2024-07-01T12:00:00Z"}; !slices.Equal(starts, want) {
		t.Errorf("occurrences %v, want %v", starts, want)
	}
}

// §3.2.19 defines tzidparam as paramtext, which can't hold ":", but
// erratum 5505 (Held for Document Update) and real producers quote TZIDs
// such as GMT-03:00. A quoted TZID is matched against the VTIMEZONE's TZID
// property, whose value is TEXT (§3.8.3.1), so "\;" and "\," in the
// property are the characters ";" and "," in the quoted parameter.
func TestConformanceQuotedTZID(t *testing.T) {
	cal := ccCal(t, `BEGIN:VTIMEZONE
TZID:GMT-03:00
BEGIN:STANDARD
DTSTART:19700101T000000
TZOFFSETFROM:-0300
TZOFFSETTO:-0300
END:STANDARD
END:VTIMEZONE
BEGIN:VTIMEZONE
TZID:Zone\; with\, punctuation
BEGIN:STANDARD
DTSTART:19700101T000000
TZOFFSETFROM:+0200
TZOFFSETTO:+0200
END:STANDARD
END:VTIMEZONE
BEGIN:VEVENT
UID:q1@example.com
DTSTAMP:20240101T000000Z
DTSTART;TZID="GMT-03:00":20240701T090000
END:VEVENT
BEGIN:VEVENT
UID:q2@example.com
DTSTAMP:20240101T000000Z
DTSTART;TZID="Zone; with, punctuation":20240701T090000
END:VEVENT`)
	want := map[string]string{
		"q1@example.com": "2024-07-01T12:00:00Z",
		"q2@example.com": "2024-07-01T07:00:00Z",
	}
	for ev := range cal.Events() {
		start, err := ev.Start()
		if err != nil {
			t.Errorf("%s: %v", ev.UID(), err)
			continue
		}
		if got := start.Time().UTC().Format(time.RFC3339); got != want[ev.UID()] {
			t.Errorf("%s: Start %s, want %s", ev.UID(), got, want[ev.UID()])
		}
	}
}

// §3.2.19: a TZID with the "/" prefix names a globally unique ID; it is
// still matched against the calendar's VTIMEZONE of exactly that TZID
// first (§3.8.3.1 example TZID:/example.org/America/New_York).
func TestConformanceGloballyUniqueTZID(t *testing.T) {
	cal := ccCal(t, `BEGIN:VTIMEZONE
TZID:/example.org/America/New_York
BEGIN:STANDARD
DTSTART:19700101T000000
TZOFFSETFROM:+0100
TZOFFSETTO:+0100
END:STANDARD
END:VTIMEZONE
BEGIN:VEVENT
UID:g@example.com
DTSTAMP:20240101T000000Z
DTSTART;TZID=/example.org/America/New_York:20240701T090000
END:VEVENT`)
	start, err := ccFirstEvent(t, cal).Start()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := start.Time(), ccUTC("2024-07-01T08:00:00Z"); !got.Equal(want) {
		t.Errorf("Start %s, want %s (the VTIMEZONE, not IANA)", got.UTC(), want)
	}
}

// §4, the group meeting with erratum 3740 applied: the VTIMEZONE's rules
// cover 1998, and the meeting "begins at 8:30 AM EST on March 12, 1998 and
// ends at 9:30 AM EST on March 12, 1998".
func TestConformanceExampleGroupMeetingCorrected(t *testing.T) {
	cal := ccFixture(t, "rfc5545-4-b-corrected")
	ev := ccFirstEvent(t, cal)
	ccCheckSpan(t, ev, "1998-03-12T13:30:00Z", "1998-03-12T14:30:00Z")
	start, _ := ev.Start()
	if name, off := start.Time().Zone(); name != "EST" || off != ccEST {
		t.Errorf("Start zone %s %d, want EST -0500", name, off)
	}
	got := ccSpans(t, &ical.Expander{}, cal, "1998-03-01T00:00:00Z", "1998-04-01T00:00:00Z")
	if want := []string{"1998-03-12T13:30:00Z/1998-03-12T14:30:00Z"}; !slices.Equal(got, want) {
		t.Errorf("occurrences %v, want %v", got, want)
	}
}

// §4, the group meeting as printed. Erratum 3740: its VTIMEZONE's only
// onsets (1998-10-25 STANDARD, 1999-04-04 DAYLIGHT) are after the
// 1998-03-12 meeting, so the zone data doesn't say what offset applies,
// and the prose's "8:30 AM EST" can't be derived from it. The RFC is
// silent about times before the first onset; the documented choice is the
// first onset's TZOFFSETFROM, -0400 here, so 08:30 is 12:30 UTC.
func TestConformanceExampleGroupMeetingBeforeFirstOnset(t *testing.T) {
	cal := ccFixture(t, "rfc5545-4-b")
	ev := ccFirstEvent(t, cal)
	ccCheckSpan(t, ev, "1998-03-12T12:30:00Z", "1998-03-12T13:30:00Z")
	start, _ := ev.Start()
	name, off := start.Time().Zone()
	t.Logf("before the first onset: %s %+d (the §4 prose says EST, which this zone data can't give)", name, off)
}

// TestConformanceNeverReadsLocal checks that nothing depends on the host's
// zone: time.Local is never consulted (architecture doc; §3.3.5 leaves a
// floating time to the reader, and a zoned one is fixed by its VTIMEZONE).
// It runs the same expansion in child processes with TZ=America/Los_Angeles
// and TZ=UTC and requires identical output.
func TestConformanceNeverReadsLocal(t *testing.T) {
	if path := os.Getenv("ICAL_CC_LOCAL_DUMP"); path != "" {
		if err := os.WriteFile(path, []byte(ccLocalDump(t)), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	dir := t.TempDir()
	var dumps []string
	for _, zone := range []string{"America/Los_Angeles", "UTC", "Asia/Kolkata"} {
		path := filepath.Join(dir, strings.ReplaceAll(zone, "/", "_"))
		cmd := exec.Command(os.Args[0], "-test.run=^TestConformanceNeverReadsLocal$", "-test.count=1")
		cmd.Env = append(os.Environ(), "TZ="+zone, "ICAL_CC_LOCAL_DUMP="+path)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("child with TZ=%s: %v\n%s", zone, err, out)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		dumps = append(dumps, string(b))
		if !strings.Contains(string(b), "host offset ") {
			t.Fatalf("TZ=%s: no output", zone)
		}
	}
	strip := func(s string) string { _, rest, _ := strings.Cut(s, "\n"); return rest }
	for i := 1; i < len(dumps); i++ {
		if strip(dumps[i]) != strip(dumps[0]) {
			t.Errorf("output depends on TZ:\n%s\n---\n%s", dumps[0], dumps[i])
		}
	}
	// The children really ran in different zones.
	if dumps[0] == dumps[1] {
		t.Errorf("children report the same host offset; TZ had no effect")
	}
	t.Logf("identical output under TZ=America/Los_Angeles, UTC and Asia/Kolkata:\n%s", strip(dumps[0]))
}

// ccLocalDump expands floating, DATE, UTC, VTIMEZONE and IANA values and
// writes what the library returns, preceded by the host's offset.
func ccLocalDump(t *testing.T) string {
	cal := ccCal(t, `BEGIN:VTIMEZONE
TZID:Custom
BEGIN:STANDARD
DTSTART:19700101T000000
TZOFFSETFROM:+0300
TZOFFSETTO:+0300
END:STANDARD
END:VTIMEZONE
BEGIN:VEVENT
UID:floating
DTSTAMP:20240101T000000Z
DTSTART:20240310T013000
DURATION:PT2H
RRULE:FREQ=DAILY;COUNT=3
END:VEVENT
BEGIN:VEVENT
UID:date
DTSTAMP:20240101T000000Z
DTSTART;VALUE=DATE:20240310
RRULE:FREQ=DAILY;COUNT=2
END:VEVENT
BEGIN:VEVENT
UID:utc
DTSTAMP:20240101T000000Z
DTSTART:20240310T100000Z
DTEND:20240310T110000Z
END:VEVENT
BEGIN:VEVENT
UID:vtimezone
DTSTAMP:20240101T000000Z
DTSTART;TZID=Custom:20240310T090000
DTEND;TZID=Custom:20240310T100000
END:VEVENT
BEGIN:VEVENT
UID:iana
DTSTAMP:20240101T000000Z
DTSTART;TZID=America/New_York:20240310T023000
DURATION:P1D
END:VEVENT`)
	var b strings.Builder
	_, off := time.Date(2024, 7, 1, 12, 0, 0, 0, time.Local).Zone()
	fmt.Fprintf(&b, "host offset %d\n", off)
	for ev := range cal.Events() {
		start, err1 := ev.Start()
		end, err2 := ev.End()
		fmt.Fprintf(&b, "%s start %s %s end %s %s %v %v\n", ev.UID(),
			start.Time().Format(time.RFC3339), start.Time().Location(),
			end.Time().Format(time.RFC3339), end.Time().Location(), err1, err2)
	}
	for occ, err := range cal.Occurrences(ccUTC("2024-03-01T00:00:00Z"), ccUTC("2024-04-01T00:00:00Z")) {
		fmt.Fprintf(&b, "occurrence %s %s %s %v\n", occ.Component.Get("UID").Value,
			occ.Start.Time().Format(time.RFC3339), occ.End.Time().Format(time.RFC3339), err)
	}
	return b.String()
}

func ccFirstEvent(t *testing.T, cal *ical.Calendar) ical.Event {
	t.Helper()
	for ev := range cal.Events() {
		return ev
	}
	t.Fatal("no VEVENT")
	return ical.Event{}
}

// ccCheckSpan checks an event's Start and End instants.
func ccCheckSpan(t *testing.T, ev ical.Event, start, end string) {
	t.Helper()
	s, err := ev.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	e, err := ev.End()
	if err != nil {
		t.Fatalf("End: %v", err)
	}
	if got := s.Time().UTC().Format(time.RFC3339); got != start {
		t.Errorf("Start %s, want %s", got, start)
	}
	if got := e.Time().UTC().Format(time.RFC3339); got != end {
		t.Errorf("End %s, want %s", got, end)
	}
}

// ccOccurrences expands cal over [from, to), failing on any error.
func ccOccurrences(t *testing.T, x *ical.Expander, cal *ical.Calendar, from, to string) []ical.Occurrence {
	t.Helper()
	var occs []ical.Occurrence
	for occ, err := range x.Occurrences(cal, ccUTC(from), ccUTC(to)) {
		if err != nil {
			t.Fatalf("Occurrences: %v", err)
		}
		occs = append(occs, occ)
	}
	return occs
}

// ccStarts returns the occurrences' start instants in UTC.
func ccStarts(t *testing.T, x *ical.Expander, cal *ical.Calendar, from, to string) []string {
	t.Helper()
	var s []string
	for _, occ := range ccOccurrences(t, x, cal, from, to) {
		s = append(s, ccInstant(x, occ.Start))
	}
	return s
}

// ccSpans returns the occurrences as start/end instants in UTC.
func ccSpans(t *testing.T, x *ical.Expander, cal *ical.Calendar, from, to string) []string {
	t.Helper()
	var s []string
	for _, occ := range ccOccurrences(t, x, cal, from, to) {
		s = append(s, ccInstant(x, occ.Start)+"/"+ccInstant(x, occ.End))
	}
	return s
}

// ccInstant formats v as a UTC instant, reading a floating value in the
// Expander's Floating zone (UTC when nil).
func ccInstant(x *ical.Expander, v ical.DateTime) string {
	loc := x.Floating
	if loc == nil {
		loc = time.UTC
	}
	return v.In(loc).UTC().Format(time.RFC3339)
}
