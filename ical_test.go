package ical

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aanelson/iCalendar/civil"
	"github.com/aanelson/iCalendar/recur"
)

const sample = "BEGIN:VCALENDAR\r\n" +
	"VERSION:2.0\r\n" +
	"PRODID:-//Example//Test//EN\r\n" +
	"X-WR-CALNAME:Team\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:1@example.com\r\n" +
	"DTSTAMP:20240101T000000Z\r\n" +
	"DTSTART;TZID=America/New_York:20240108T090000\r\n" +
	"DTEND;TZID=America/New_York:20240108T100000\r\n" +
	"SUMMARY:Dinner\\, drinks\\; more\r\n" +
	"CATEGORIES:WORK,MEETING\\, WEEKLY\r\n" +
	"X-CUSTOM;X-PARAM=\"a:b\":kept as is\r\n" +
	"BEGIN:VALARM\r\n" +
	"ACTION:DISPLAY\r\n" +
	"TRIGGER;RELATED=END:-PT15M\r\n" +
	"DESCRIPTION:Reminder\r\n" +
	"END:VALARM\r\n" +
	"END:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

func mustParse(t *testing.T, s string) *Calendar {
	t.Helper()
	cal, err := Parse(strings.NewReader(s))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return cal
}

func TestParseAndAccess(t *testing.T) {
	cal := mustParse(t, sample)
	if cal.ProdID() != "-//Example//Test//EN" || cal.Version() != "2.0" {
		t.Errorf("PRODID %q VERSION %q", cal.ProdID(), cal.Version())
	}
	var events []Event
	for ev := range cal.Events() {
		events = append(events, ev)
	}
	if len(events) != 1 {
		t.Fatalf("%d events", len(events))
	}
	ev := events[0]
	if ev.Summary() != "Dinner, drinks; more" {
		t.Errorf("Summary %q", ev.Summary())
	}
	if got := ev.Categories(); !slices.Equal(got, []string{"WORK", "MEETING, WEEKLY"}) {
		t.Errorf("Categories %q", got)
	}
	start, err := ev.Start()
	if err != nil {
		t.Fatal(err)
	}
	if start.TZID != "America/New_York" || start.Loc == nil || start.Time().Format(time.RFC3339) != "2024-01-08T09:00:00-05:00" {
		t.Errorf("Start %+v → %v", start, start.Time())
	}
	end, _ := ev.End()
	if d, _ := ev.Duration(); d != (Duration{Hours: 1}) || end.Time().Sub(start.Time()) != time.Hour {
		t.Errorf("Duration %v End %v", d, end)
	}
	if p := ev.Get("x-custom"); p == nil || p.Value != "kept as is" || p.Params.Get("X-PARAM") != "a:b" {
		t.Errorf("X-CUSTOM %+v", p)
	}
	for a := range ev.Alarms() {
		rel, _, toEnd, err := a.Trigger()
		if err != nil || rel.String() != "-PT15M" || !toEnd || a.Action() != "DISPLAY" {
			t.Errorf("alarm %v %v %v", rel, toEnd, err)
		}
	}
}

// Decoding and encoding give back the same text, byte for byte, for a
// calendar already in canonical form.
func TestRoundTrip(t *testing.T) {
	cal := mustParse(t, sample)
	out, err := cal.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != sample {
		t.Errorf("round trip changed the text:\n%s", out)
	}
}

func TestTruncationIsNeverSuccess(t *testing.T) {
	for _, in := range []string{
		"BEGIN:VCALENDAR\r\nVERSION:2.0\r\n",
		"BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:1\r\nEND:VEVENT\r\n",
		"BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nDTSTART;TZID=America/New_York",
		"BEGIN:VCALENDAR",
	} {
		for _, strict := range []bool{false, true} {
			d := NewDecoder(strings.NewReader(in))
			d.Strict = strict
			// Strict mode may first fail on the cut-off line itself;
			// either way there is no calendar.
			cal, err := d.Decode()
			if cal != nil || err == nil || !strict && !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Errorf("strict=%v %q: %v, %v; want io.ErrUnexpectedEOF", strict, in, cal, err)
			}
		}
	}
	if _, err := NewDecoder(strings.NewReader("")).Decode(); err != io.EOF {
		t.Errorf("empty input: %v", err)
	}
	if _, err := Parse(strings.NewReader("")); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("Parse of empty input: %v", err)
	}
}

func TestLenientRecoveries(t *testing.T) {
	in := "\xef\xbb\xbfjunk before:x\r\n" +
		"begin:vcalendar\n" +
		"prodid:-//x//EN\n" +
		"BEGIN:VEVENT\n" +
		"UID:1\n" +
		"DTSTART:20240101\n" + // a DATE without VALUE=DATE
		"BAD LINE\n" +
		"SUMMARY:https\\://example.com\n" +
		"END:VTODO\n" + // nothing to close
		"BEGIN:VALARM\n" +
		"ACTION:DISPLAY\n" +
		"END:VEVENT\n" + // closes VALARM too
		"END:VCALENDAR\n"
	var warnings []string
	d := NewDecoder(strings.NewReader(in))
	d.Warn = func(err error) { warnings = append(warnings, err.Error()) }
	cal, err := d.Decode()
	if err != nil {
		t.Fatal(err)
	}
	ev := Event{Item{cal.Children[0], cal}}
	start, err := ev.Start()
	if err != nil || !start.IsDate || start.Date != (civil.Date{Year: 2024, Month: 1, Day: 1}) {
		t.Errorf("Start %+v %v", start, err)
	}
	if ev.Summary() != "https://example.com" {
		t.Errorf("Summary %q", ev.Summary())
	}
	if len(cal.Children[0].Children) != 1 {
		t.Errorf("the VALARM went missing")
	}
	// junk, bad line, stray END, unclosed VALARM, missing VERSION
	if len(warnings) != 5 {
		t.Errorf("%d warnings: %q", len(warnings), warnings)
	}
	d = NewDecoder(strings.NewReader(in))
	d.Strict = true
	if _, err := d.Decode(); err == nil {
		t.Error("strict decoding succeeded")
	}
}

// Input with content but no calendar is an error, not an empty stream, so a
// feed that comes back as an error page doesn't read as an empty calendar.
func TestDecodeNoCalendar(t *testing.T) {
	cal := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//EN\r\nEND:VCALENDAR\r\n"
	for _, tc := range []struct {
		name, in string
		cals     int // calendars before the end
		eof      bool
	}{
		{"empty", "", 0, true},
		{"BOM only", "\xef\xbb\xbf", 0, true},
		{"HTML page", "<!DOCTYPE html>\r\n<html><body>Not Found</body></html>\r\n", 0, false},
		{"vCard", "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:A\r\nEND:VCARD\r\n", 0, false},
		{"bare VEVENT", "BEGIN:VEVENT\r\nUID:1\r\nEND:VEVENT\r\n", 0, false},
		{"blank lines", "\r\n\r\n", 0, false},
		{"calendar then junk", cal + "<html>\r\nX:1\r\n", 1, true},
		{"junk then calendar", "<html>\r\n" + cal, 1, true},
	} {
		for _, warn := range []bool{false, true} {
			d := NewDecoder(strings.NewReader(tc.in))
			if warn {
				d.Warn = func(error) {}
			}
			n := 0
			var err error
			for err == nil {
				if _, err = d.Decode(); err == nil {
					n++
				}
			}
			var se *SyntaxError
			if n != tc.cals || tc.eof && err != io.EOF || !tc.eof && !errors.As(err, &se) {
				t.Errorf("%s (Warn set: %v): %d calendars, then %v", tc.name, warn, n, err)
			}
		}
	}
}

func TestDepthLimit(t *testing.T) {
	in := "BEGIN:VCALENDAR\r\n" + strings.Repeat("BEGIN:X\r\n", 100)
	d := NewDecoder(strings.NewReader(in))
	if _, err := d.Decode(); err == nil || !strings.Contains(err.Error(), "deeper") {
		t.Errorf("err = %v", err)
	}
}

func TestStream(t *testing.T) {
	in := sample + sample
	d := NewDecoder(strings.NewReader(in))
	for i := range 2 {
		if _, err := d.Decode(); err != nil {
			t.Fatalf("calendar %d: %v", i, err)
		}
	}
	if _, err := d.Decode(); err != io.EOF {
		t.Errorf("after two: %v", err)
	}
	if _, err := Parse(strings.NewReader(in)); err == nil {
		t.Error("Parse accepted two calendars")
	}
}

func TestEncoderRejectsInjection(t *testing.T) {
	cal := NewCalendar("-//x//EN")
	ev := NewEvent("1", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	ev.Props.Add(NewProperty("SUMMARY", "x\r\nBEGIN:VALARM"))
	cal.Add(ev.Component)
	if _, err := cal.Bytes(); err == nil {
		t.Error("a raw CRLF in a value was written")
	}
	// SetText makes the same text safe.
	ev.SetSummary("x\r\nBEGIN:VALARM")
	out, err := cal.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	back := mustParse(t, string(out))
	for e := range back.Events() {
		if e.Summary() != "x\nBEGIN:VALARM" || len(e.Children) != 0 {
			t.Errorf("summary %q children %d", e.Summary(), len(e.Children))
		}
	}
}

func TestEmptyCalendarEncodes(t *testing.T) {
	out, err := NewCalendar("-//x//EN").Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if want := "BEGIN:VCALENDAR\r\nPRODID:-//x//EN\r\nVERSION:2.0\r\nEND:VCALENDAR\r\n"; string(out) != want {
		t.Errorf("got %q", out)
	}
}

func TestText(t *testing.T) {
	for _, tc := range []struct{ raw, text string }{
		{`plain`, "plain"},
		{`a\,b\;c\\d\ne\Nf`, "a,b;c\\d\ne\nf"},
		{`trailing\`, `trailing\`},
		{`unknown\:escape`, "unknown:escape"},
	} {
		if got := UnescapeText(tc.raw); got != tc.text {
			t.Errorf("UnescapeText(%q) = %q, want %q", tc.raw, got, tc.text)
		}
	}
	if got := EscapeText("a,b;c\\d\r\ne\nf\rg\x07h\ti"); got != `a\,b\;c\\d\ne\nf\ng`+"h\ti" {
		t.Errorf("EscapeText = %q", got)
	}
	p := &Property{Name: "CATEGORIES", Value: `a\,b,c,,d\\`}
	if got := p.Texts(); !slices.Equal(got, []string{"a,b", "c", "", `d\`}) {
		t.Errorf("Texts = %q", got)
	}
}

func TestDateTimeForms(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	for _, tc := range []struct {
		prop   *Property
		want   string // RFC 3339 of Time()
		date   bool
		utc    bool
		tzid   string
		failOK bool
	}{
		{&Property{Name: "DTSTART", Value: "19970714T133000"}, "1997-07-14T13:30:00Z", false, false, "", false},
		{&Property{Name: "DTSTART", Value: "19970714T173000Z"}, "1997-07-14T17:30:00Z", false, true, "", false},
		{&Property{Name: "DTSTART", Value: "19970714T133000", Params: Params{{"TZID", []string{"America/New_York"}}}}, "1997-07-14T13:30:00-04:00", false, false, "America/New_York", false},
		{&Property{Name: "DTSTART", Value: "19970714", Params: Params{{"VALUE", []string{"DATE"}}}}, "1997-07-14T00:00:00Z", true, false, "", false},
		{&Property{Name: "DTSTART", Value: "19970714"}, "1997-07-14T00:00:00Z", true, false, "", false},
		{&Property{Name: "DTSTART", Value: "19970714T133000", Params: Params{{"TZID", []string{"Eastern Standard Time"}}}}, "1997-07-14T13:30:00-04:00", false, false, "Eastern Standard Time", false},
		{&Property{Name: "DTSTART", Value: "19970714T133000", Params: Params{{"TZID", []string{"Nowhere"}}}}, "1997-07-14T13:30:00Z", false, false, "Nowhere", true},
	} {
		v, err := tc.prop.DateTime(nil)
		if (err != nil) != tc.failOK {
			t.Errorf("%v: err %v", tc.prop, err)
		}
		if got := v.Time().Format(time.RFC3339); got != tc.want || v.IsDate != tc.date || v.UTC != tc.utc || v.TZID != tc.tzid {
			t.Errorf("%v: %+v → %s", tc.prop, v, got)
		}
	}
	// Floating times stay floating: In reads them on the given clock.
	f := NewFloating(civil.DateTime{Date: civil.Date{Year: 2024, Month: 7, Day: 1}, Time: civil.Time{Hour: 9}})
	if got := f.In(ny).Format(time.RFC3339); got != "2024-07-01T09:00:00-04:00" {
		t.Errorf("floating in NY: %s", got)
	}
	// Producers that drop the seconds, or put a Z on a DATE, are read.
	if v, err := (&Property{Name: "DTEND", Value: "19970701T1300Z"}).DateTime(nil); err != nil || v.Hour != 13 || !v.UTC {
		t.Errorf("19970701T1300Z: %+v %v", v, err)
	}
	if v, err := (&Property{Name: "RDATE", Value: "20131210Z"}).DateTime(nil); err != nil || !v.IsDate {
		t.Errorf("20131210Z: %+v %v", v, err)
	}
	for _, s := range []string{"1997071", "19970714T13", "19971314", "19970714T250000", "x"} {
		if _, err := (&Property{Name: "DTSTART", Value: s}).DateTime(nil); err == nil {
			t.Errorf("%q parsed", s)
		}
	}
}

func TestSetDateTimes(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	p := NewProperty("EXDATE", "")
	p.SetDateTimes(
		NewDateTime(time.Date(2024, 1, 8, 9, 0, 0, 0, ny)),
		NewDateTime(time.Date(2024, 1, 15, 14, 0, 0, 0, time.UTC)), // converted to NY
	)
	if p.Value != "20240108T090000,20240115T090000" || p.Params.Get("TZID") != "America/New_York" {
		t.Errorf("got %v %q", p.Params, p.Value)
	}
	p.SetDateTime(NewDate(civil.Date{Year: 2024, Month: 2, Day: 3}))
	if p.Value != "20240203" || p.Params.Get("VALUE") != "DATE" || p.Params.Has("TZID") {
		t.Errorf("got %v %q", p.Params, p.Value)
	}
}

func TestDuration(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Duration
		out  string
	}{
		{"P15DT5H0M20S", Duration{Days: 15, Hours: 5, Seconds: 20}, "P15DT5H0M20S"},
		{"P7W", Duration{Weeks: 7}, "P7W"},
		{"-PT15M", Duration{Negative: true, Minutes: 15}, "-PT15M"},
		{"+P1D", Duration{Days: 1}, "P1D"},
		{"PT0S", Duration{}, "PT0S"},
		{"P1W2D", Duration{Weeks: 1, Days: 2}, "P9D"},
		{"pt1h", Duration{Hours: 1}, "PT1H"},
	} {
		got, err := ParseDuration(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("ParseDuration(%q) = %+v, %v", tc.in, got, err)
		}
		if s := got.String(); s != tc.out {
			t.Errorf("%+v.String() = %q, want %q", got, s, tc.out)
		}
	}
	for _, s := range []string{"", "P", "PT", "1D", "P1H", "PT1D", "P1DT", "PT1S2M", "P9999999999D", "P1D1W"} {
		if _, err := ParseDuration(s); err == nil {
			t.Errorf("ParseDuration(%q) succeeded", s)
		}
	}
}

// P1D keeps the wall-clock time across a DST change; PT24H doesn't
// (RFC 5545 §3.3.6).
func TestDurationAcrossDST(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	start := NewDateTime(time.Date(2024, 3, 9, 12, 0, 0, 0, ny))
	if got := start.Add(Duration{Days: 1}).Time().Format(time.RFC3339); got != "2024-03-10T12:00:00-04:00" {
		t.Errorf("P1D: %s", got)
	}
	if got := start.Add(Duration{Hours: 24}).Time().Format(time.RFC3339); got != "2024-03-10T13:00:00-04:00" {
		t.Errorf("PT24H: %s", got)
	}
	date := NewDate(civil.Date{Year: 2024, Month: 2, Day: 28})
	if got := date.Add(Duration{Days: 2}); !got.IsDate || got.Date != (civil.Date{Year: 2024, Month: 3, Day: 1}) {
		t.Errorf("date + P2D = %+v", got)
	}
}

func TestOtherValues(t *testing.T) {
	if n, err := ParseUTCOffset("-0530"); err != nil || n != -19800 {
		t.Errorf("-0530: %d %v", n, err)
	}
	if n, err := ParseUTCOffset("+013045"); err != nil || n != 5445 || FormatUTCOffset(n) != "+013045" {
		t.Errorf("+013045: %d %v", n, err)
	}
	for _, s := range []string{"0500", "+5", "+05:00", "+0560", "+-0500"} {
		if _, err := ParseUTCOffset(s); err == nil {
			t.Errorf("%q parsed", s)
		}
	}
	p := NewProperty("GEO", "")
	p.SetGeo(37.386013, -122.082932)
	if lat, lon, err := p.Geo(); err != nil || lat != 37.386013 || lon != -122.082932 || p.Value != "37.386013;-122.082932" {
		t.Errorf("GEO %q %v %v %v", p.Value, lat, lon, err)
	}
	p = &Property{Name: "FREEBUSY", Value: "19970308T160000Z/PT8H30M,19970308T230000Z/19970309T000000Z"}
	ps, err := p.Periods(nil)
	if err != nil || len(ps) != 2 || ps[0].EndTime().Time().Format(time.RFC3339) != "1997-03-09T00:30:00Z" || ps[1].EndTime().Time().Hour() != 0 {
		t.Errorf("Periods %+v %v", ps, err)
	}
	b := NewProperty("ATTACH", "")
	b.SetBinary([]byte("hello"))
	if got, err := b.Binary(); err != nil || string(got) != "hello" || b.Params.Get("ENCODING") != "BASE64" {
		t.Errorf("Binary %q %v", got, err)
	}
}

// The RFC 5545 §3.6.5 VTIMEZONE example for America/New_York (as in
// §4), compiled from the calendar, agrees with the IANA zone for the
// years its rules cover.
func TestCalendarVTIMEZONE(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	in := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//EN\r\n" +
		"BEGIN:VTIMEZONE\r\nTZID:My Eastern\r\n" +
		"BEGIN:DAYLIGHT\r\nDTSTART:20070311T020000\r\nRRULE:FREQ=YEARLY;BYMONTH=3;BYDAY=2SU\r\n" +
		"TZOFFSETFROM:-0500\r\nTZOFFSETTO:-0400\r\nTZNAME:EDT\r\nEND:DAYLIGHT\r\n" +
		"BEGIN:STANDARD\r\nDTSTART:20071104T020000\r\nRRULE:FREQ=YEARLY;BYMONTH=11;BYDAY=1SU\r\n" +
		"TZOFFSETFROM:-0400\r\nTZOFFSETTO:-0500\r\nTZNAME:EST\r\nEND:STANDARD\r\n" +
		"END:VTIMEZONE\r\n" +
		"BEGIN:VEVENT\r\nUID:1\r\nDTSTAMP:20240101T000000Z\r\nDTSTART;TZID=My Eastern:20240701T090000\r\nEND:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
	cal := mustParse(t, in)
	loc, err := cal.Resolve("My Eastern")
	if err != nil {
		t.Fatal(err)
	}
	for u := time.Date(2008, 1, 1, 0, 0, 0, 0, time.UTC); u.Year() < 2040; u = u.Add(time.Hour) {
		if _, a := u.In(loc).Zone(); a != func() int { _, b := u.In(ny).Zone(); return b }() {
			t.Fatalf("offsets differ at %v", u)
		}
	}
	for ev := range cal.Events() {
		start, err := ev.Start()
		if err != nil || start.Time().Format(time.RFC3339) != "2024-07-01T09:00:00-04:00" {
			t.Errorf("start %v %v", start.Time(), err)
		}
		if name, _ := start.Time().Zone(); name != "EDT" {
			t.Errorf("zone name %q", name)
		}
	}
}

func occStarts(t *testing.T, cal *Calendar, x *Expander, from, to time.Time) []string {
	t.Helper()
	var out []string
	for occ, err := range x.Occurrences(cal, from, to) {
		if err != nil {
			t.Fatalf("occurrence error: %v", err)
		}
		out = append(out, occ.Start.String()+"/"+textOf(occ.Component, "SUMMARY"))
	}
	return out
}

func TestOccurrences(t *testing.T) {
	in := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//EN\r\n" +
		// Weekly on Monday, one instance moved, one cancelled by EXDATE,
		// one excluded by a whole-day DATE EXDATE, and an extra RDATE.
		"BEGIN:VEVENT\r\nUID:weekly\r\nDTSTAMP:20240101T000000Z\r\nSUMMARY:standup\r\n" +
		"DTSTART;TZID=America/New_York:20240101T090000\r\nDURATION:PT30M\r\n" +
		"RRULE:FREQ=WEEKLY;BYDAY=MO;COUNT=6\r\n" +
		"EXDATE;TZID=America/New_York:20240115T090000\r\n" +
		"EXDATE;VALUE=DATE:20240129\r\n" +
		"RDATE:20240110T170000Z\r\n" +
		"END:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:weekly\r\nDTSTAMP:20240101T000000Z\r\nSUMMARY:moved\r\n" +
		"RECURRENCE-ID;TZID=America/New_York:20240108T090000\r\n" +
		"DTSTART;TZID=America/New_York:20240109T150000\r\nDURATION:PT30M\r\nSEQUENCE:1\r\n" +
		"END:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:weekly\r\nDTSTAMP:20240101T000000Z\r\nSUMMARY:stale\r\n" +
		"RECURRENCE-ID;TZID=America/New_York:20240108T090000\r\n" +
		"DTSTART;TZID=America/New_York:20240108T100000\r\nSEQUENCE:0\r\n" +
		"END:VEVENT\r\n" +
		// A one-off all-day event.
		"BEGIN:VEVENT\r\nUID:holiday\r\nDTSTAMP:20240101T000000Z\r\nSUMMARY:holiday\r\n" +
		"DTSTART;VALUE=DATE:20240115\r\nEND:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
	cal := mustParse(t, in)
	var x Expander
	from, to := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	got := occStarts(t, cal, &x, from, to)
	want := []string{
		"20240101T090000/standup",
		"20240109T150000/moved",
		"20240110T120000/standup", // the RDATE 17:00Z, on the series clock
		"20240115/holiday",
		"20240122T090000/standup",
		"20240205T090000/standup",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	// A window that holds only the moved instance's new time.
	got = occStarts(t, cal, &x, time.Date(2024, 1, 9, 20, 0, 0, 0, time.UTC), time.Date(2024, 1, 9, 20, 15, 0, 0, time.UTC))
	if !slices.Equal(got, []string{"20240109T150000/moved"}) {
		t.Errorf("moved: %q", got)
	}
	// An overlap test: an instance that starts before the window but ends
	// inside it is included.
	got = occStarts(t, cal, &x, time.Date(2024, 1, 22, 14, 15, 0, 0, time.UTC), time.Date(2024, 1, 22, 14, 20, 0, 0, time.UTC))
	if !slices.Equal(got, []string{"20240122T090000/standup"}) {
		t.Errorf("overlap: %q", got)
	}
	// With a cache, the same answers.
	x.Cache = recur.NewCache(0)
	for range 2 {
		if again := occStarts(t, cal, &x, from, to); !slices.Equal(again, want) {
			t.Errorf("with cache: %q", again)
		}
	}
}

func TestOccurrencesReportErrors(t *testing.T) {
	in := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:bad\r\nDTSTAMP:20240101T000000Z\r\nDTSTART:20240101T090000Z\r\nRRULE:FREQ=SOMETIMES\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:good\r\nDTSTAMP:20240101T000000Z\r\nDTSTART:20240102T090000Z\r\nEND:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
	cal := mustParse(t, in)
	var errs, occs int
	for occ, err := range cal.Occurrences(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)) {
		if err != nil {
			errs++
			if textOf(occ.Component, "UID") != "bad" {
				t.Errorf("error names %q", textOf(occ.Component, "UID"))
			}
			continue
		}
		occs++
	}
	// The bad rule is dropped, its DTSTART still occurs, and the good
	// event is unaffected.
	if errs != 1 || occs != 2 {
		t.Errorf("errs %d occs %d", errs, occs)
	}
}

func TestNewEventRoundTrip(t *testing.T) {
	cal := NewCalendar("-//x//EN")
	ev := NewEvent("abc@example.com", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	ev.SetSummary("Review, then ship")
	ev.SetStart(NewDate(civil.Date{Year: 2024, Month: 5, Day: 1}))
	ev.SetRule(recur.Rule{Freq: recur.Monthly, Count: 3})
	cal.Add(ev.Component)
	out, err := cal.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	back := mustParse(t, string(out))
	if !reflect.DeepEqual(back.Component, cal.Component) {
		t.Errorf("tree changed:\n%s", out)
	}
	var starts []string
	for occ, err := range back.Occurrences(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)) {
		if err != nil {
			t.Fatal(err)
		}
		starts = append(starts, occ.Start.String()+"-"+occ.End.String())
	}
	if want := []string{"20240501-20240502", "20240601-20240602", "20240701-20240702"}; !slices.Equal(starts, want) {
		t.Errorf("occurrences %q", starts)
	}
}

func BenchmarkDecode(b *testing.B) {
	var sb strings.Builder
	sb.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//bench//EN\r\n")
	for range 1000 {
		sb.WriteString(strings.TrimPrefix(strings.TrimSuffix(sample, "END:VCALENDAR\r\n"), "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Example//Test//EN\r\nX-WR-CALNAME:Team\r\n"))
	}
	sb.WriteString("END:VCALENDAR\r\n")
	in := []byte(sb.String())
	b.SetBytes(int64(len(in)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Parse(bytes.NewReader(in)); err != nil {
			b.Fatal(err)
		}
	}
}

// Regressions found in review.

func TestOverrideCancelledOnSeriesClock(t *testing.T) {
	in := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:u\r\nDTSTAMP:20240101T000000Z\r\nSEQUENCE:2\r\nSUMMARY:master\r\n" +
		"DTSTART;TZID=America/New_York:20240101T210000\r\nRRULE:FREQ=DAILY;COUNT=5\r\nEXDATE;VALUE=DATE:20240103\r\nEND:VEVENT\r\n" +
		// RECURRENCE-ID 02:00Z on Jan 3 is the Jan 2 21:00 NY instance: not
		// the excluded day on the series clock.
		"BEGIN:VEVENT\r\nUID:u\r\nDTSTAMP:20240101T000000Z\r\nSEQUENCE:1\r\nSUMMARY:override\r\n" +
		"RECURRENCE-ID:20240103T020000Z\r\nDTSTART;TZID=America/New_York:20240102T220000\r\nEND:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
	cal := mustParse(t, in)
	var x Expander
	got := occStarts(t, cal, &x, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC))
	want := []string{"20240101T210000/master", "20240102T220000/override", "20240104T210000/master", "20240105T210000/master"}
	if !slices.Equal(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestFloatingSeriesWithInstantDates(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	in := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:f\r\nDTSTAMP:20240101T000000Z\r\nSUMMARY:f\r\n" +
		"DTSTART:20240101T090000\r\nRRULE:FREQ=DAILY;COUNT=3\r\n" +
		"EXDATE:20240102T140000Z\r\nRDATE:20240110T140000Z\r\nEND:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
	cal := mustParse(t, in)
	x := Expander{Floating: ny}
	got := occStarts(t, cal, &x, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC))
	want := []string{"20240101T090000/f", "20240103T090000/f", "20240110T090000/f"}
	if !slices.Equal(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// One hour after the first 01:30 of a fall-back night is the second 01:30.
func TestAddAcrossFallBack(t *testing.T) {
	in := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:b\r\nDTSTAMP:20240101T000000Z\r\n" +
		"DTSTART;TZID=America/New_York:20071104T013000\r\nDURATION:PT1H\r\nEND:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
	cal := mustParse(t, in)
	for ev := range cal.Events() {
		start, _ := ev.Start()
		end, err := ev.End()
		if err != nil || end.Time().Sub(start.Time()) != time.Hour {
			t.Errorf("end %v - start %v", end.Time(), start.Time())
		}
	}
	n := 0
	for _, err := range cal.Occurrences(time.Date(2007, 11, 4, 6, 0, 0, 0, time.UTC), time.Date(2007, 11, 4, 6, 20, 0, 0, time.UTC)) {
		if err != nil {
			t.Fatal(err)
		}
		n++
	}
	if n != 1 {
		t.Errorf("%d occurrences in the event's second half hour", n)
	}
}

// One Calendar and one Cache shared by many goroutines, as a feed server
// would share them.
func TestConcurrentExpansion(t *testing.T) {
	cal := mustParse(t, strings.ReplaceAll(sample, "DTEND;TZID=America/New_York:20240108T100000\r\n",
		"DTEND;TZID=America/New_York:20240108T100000\r\nRRULE:FREQ=WEEKLY;BYDAY=MO,WE;COUNT=500\r\n"))
	x := Expander{Cache: recur.NewCache(0)}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	count := func() int {
		n := 0
		for _, err := range x.Occurrences(cal, from, from.AddDate(1, 0, 0)) {
			if err == nil {
				n++
			}
		}
		return n
	}
	want := count()
	done := make(chan int)
	for range 8 {
		go func() { done <- count() }()
	}
	for range 8 {
		if got := <-done; got != want {
			t.Fatalf("concurrent expansion found %d, want %d", got, want)
		}
	}
}

func TestNewDateTimeKeepsInstant(t *testing.T) {
	tm, _ := time.Parse(time.RFC3339, "2024-01-01T09:00:00-05:00")
	p := NewProperty("DTSTART", "")
	p.SetDateTime(NewDateTime(tm))
	back, err := p.DateTime(nil)
	if err != nil || !back.Time().Equal(tm) || p.Value != "20240101T140000Z" {
		t.Errorf("wrote %q, read back %v %v", p.Value, back.Time(), err)
	}
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	if v := NewDateTime(tm.In(ny)); v.TZID != "America/New_York" {
		t.Errorf("an IANA zone became %+v", v)
	}
}

func TestWarnGetsICalSyntaxErrors(t *testing.T) {
	d := NewDecoder(strings.NewReader("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:x\r\nBAD LINE\r\n\r\nEND:VCALENDAR\r\n"))
	n := 0
	d.Warn = func(err error) {
		var se *SyntaxError
		if !errors.As(err, &se) {
			t.Errorf("warning %T is not an *ical.SyntaxError", err)
		}
		n++
	}
	if _, err := d.Decode(); err != nil || n != 2 {
		t.Errorf("err %v, %d warnings", err, n)
	}
}

func TestTodoOccurrencesEndAtDue(t *testing.T) {
	in := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//EN\r\n" +
		"BEGIN:VTODO\r\nUID:t\r\nDTSTAMP:20240101T000000Z\r\n" +
		"DTSTART:20240101T090000Z\r\nDUE:20240103T090000Z\r\nRRULE:FREQ=WEEKLY;COUNT=2\r\nEND:VTODO\r\n" +
		"END:VCALENDAR\r\n"
	cal := mustParse(t, in)
	x := Expander{Components: []string{"VTODO"}}
	var ends []string
	// The window opens after the first to-do starts but before it is due.
	for occ, err := range x.Occurrences(cal, time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)) {
		if err != nil {
			t.Fatal(err)
		}
		ends = append(ends, occ.End.String())
	}
	if !slices.Equal(ends, []string{"20240103T090000Z", "20240110T090000Z"}) {
		t.Errorf("ends %v", ends)
	}
}

func TestVTIMEZONEWithUTCDTSTART(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	// DTSTART written in UTC, and with a stray TZID, as some producers do.
	in := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//EN\r\n" +
		"BEGIN:VTIMEZONE\r\nTZID:Odd\r\n" +
		"BEGIN:DAYLIGHT\r\nDTSTART:20070311T070000Z\r\nRRULE:FREQ=YEARLY;BYMONTH=3;BYDAY=2SU\r\n" +
		"TZOFFSETFROM:-0500\r\nTZOFFSETTO:-0400\r\nEND:DAYLIGHT\r\n" +
		"BEGIN:STANDARD\r\nDTSTART;TZID=Nowhere:20071104T020000\r\nRRULE:FREQ=YEARLY;BYMONTH=11;BYDAY=1SU\r\n" +
		"TZOFFSETFROM:-0400\r\nTZOFFSETTO:-0500\r\nEND:STANDARD\r\n" +
		"END:VTIMEZONE\r\nEND:VCALENDAR\r\n"
	cal := mustParse(t, in)
	loc, err := cal.Resolve("Odd")
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []time.Time{time.Date(2024, 3, 10, 6, 59, 0, 0, time.UTC), time.Date(2024, 3, 10, 7, 0, 0, 0, time.UTC), time.Date(2024, 11, 3, 6, 0, 0, 0, time.UTC)} {
		if _, a := u.In(loc).Zone(); a != func() int { _, b := u.In(ny).Zone(); return b }() {
			t.Errorf("offset at %v is %d", u, a)
		}
	}
}

func TestRecurrenceAppliesDateExdates(t *testing.T) {
	in := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:g\r\nDTSTAMP:20240101T000000Z\r\nDTSTART:20240101T090000Z\r\n" +
		"RRULE:FREQ=WEEKLY;COUNT=4\r\nEXDATE;VALUE=DATE:20240115\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	cal := mustParse(t, in)
	for ev := range cal.Events() {
		set, err := ev.Recurrence()
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for at := range set.Times(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)) {
			if at.Day() == 15 {
				t.Errorf("Jan 15 wasn't excluded")
			}
			n++
		}
		if n != 3 {
			t.Errorf("%d instances", n)
		}
	}
}
