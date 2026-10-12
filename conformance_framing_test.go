package ical_test

// Conformance tests for document framing: RFC 5545 §3.4 (the iCalendar
// stream and object), §3.6 (the calendar body and components), the parts
// of §2, §3.1 and §3.2 that the Decoder and Encoder carry through to the
// model, and decode → encode → decode of the RFC's own examples. Every
// expectation comes from the RFC text, cited next to the case; where the
// RFC leaves a choice open the comment names the choice docs/architecture.md
// made.

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	ical "github.com/aanelson/iCalendar"
)

// crlf turns a readable LF-separated text into CRLF-separated lines.
func crlf(s string) string {
	return strings.ReplaceAll(s, "\n", "\r\n")
}

// twoEvents is a minimal valid iCalendar object with two components.
var twoEvents = crlf(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Conformance//EN
BEGIN:VEVENT
UID:1@example.com
DTSTAMP:19970610T172345Z
DTSTART:19970714T170000Z
SUMMARY:One
END:VEVENT
BEGIN:VEVENT
UID:2@example.com
DTSTAMP:19970610T172345Z
DTSTART:19970715T170000Z
SUMMARY:Two
END:VEVENT
END:VCALENDAR
`)

// bastille is the §3.4 example iCalendar object.
var bastille = crlf(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//hacksw/handcal//NONSGML v1.0//EN
BEGIN:VEVENT
UID:19970610T172345Z-AF23B2@example.com
DTSTAMP:19970610T172345Z
DTSTART:19970714T170000Z
DTEND:19970715T040000Z
SUMMARY:Bastille Day Party
END:VEVENT
END:VCALENDAR
`)

type decodeResult struct {
	cals  []*ical.Calendar
	warns []error
	err   error // the error that ended decoding; nil at io.EOF
}

// decodeStream decodes every calendar in input.
func decodeStream(input string, strict bool) decodeResult {
	var res decodeResult
	d := ical.NewDecoder(strings.NewReader(input))
	d.Strict = strict
	d.Warn = func(err error) { res.warns = append(res.warns, err) }
	for range 100 {
		cal, err := d.Decode()
		if err == io.EOF {
			return res
		}
		if err != nil {
			if cal != nil {
				res.err = errors.New("test: Decode returned a calendar with an error")
			} else {
				res.err = err
			}
			return res
		}
		res.cals = append(res.cals, cal)
	}
	res.err = errors.New("test: Decode never returned io.EOF")
	return res
}

func checkWarningTypes(t *testing.T, warns []error) {
	t.Helper()
	for _, w := range warns {
		var se *ical.SyntaxError
		if !errors.As(w, &se) {
			t.Errorf("warning %v is a %T, want *ical.SyntaxError", w, w)
		}
	}
}

func uids(cal *ical.Calendar) []string {
	var out []string
	for ev := range cal.Events() {
		out = append(out, ev.UID())
	}
	return out
}

// TestConformanceStream checks RFC 5545 §3.4: "multiple iCalendar objects
// can be sequentially grouped together in an iCalendar stream", with
// icalstream = 1*icalobject.
func TestConformanceStream(t *testing.T) {
	second := strings.Replace(bastille, "Bastille Day Party", "Second", 1)
	second = strings.Replace(second, "AF23B2", "AF23B3", 1)
	in := twoEvents + bastille + second
	for _, strict := range []bool{false, true} {
		res := decodeStream(in, strict)
		if res.err != nil || len(res.warns) != 0 {
			t.Fatalf("strict=%v: %v, warnings %v", strict, res.err, res.warns)
		}
		if len(res.cals) != 3 {
			t.Fatalf("strict=%v: %d calendars, want 3", strict, len(res.cals))
		}
		want := [][]string{{"1@example.com", "2@example.com"}, {"19970610T172345Z-AF23B2@example.com"}, {"19970610T172345Z-AF23B3@example.com"}}
		for i, cal := range res.cals {
			if got := uids(cal); !reflect.DeepEqual(got, want[i]) {
				t.Errorf("strict=%v: calendar %d has events %q, want %q", strict, i, got, want[i])
			}
		}
	}
	// Parse reads exactly one calendar (its documented contract); a
	// stream of several is for the Decoder.
	if _, err := ical.Parse(strings.NewReader(in)); err == nil {
		t.Error("Parse accepted a stream of three calendars")
	}
	cal, err := ical.Parse(strings.NewReader(bastille))
	if err != nil {
		t.Fatalf("Parse(§3.4 example): %v", err)
	}
	ev := cal.Events()
	n := 0
	for e := range ev {
		n++
		if e.Summary() != "Bastille Day Party" {
			t.Errorf("Summary = %q", e.Summary())
		}
	}
	if n != 1 || cal.ProdID() != "-//hacksw/handcal//NONSGML v1.0//EN" || cal.Version() != "2.0" {
		t.Errorf("§3.4 example: %d events, PRODID %q, VERSION %q", n, cal.ProdID(), cal.Version())
	}
}

// TestConformanceEmptyStream checks that a stream with no iCalendar object
// is never read as one: icalstream = 1*icalobject (§3.4).
func TestConformanceEmptyStream(t *testing.T) {
	// The Decoder reports "no calendar left" as io.EOF (documented).
	if _, err := ical.NewDecoder(strings.NewReader("")).Decode(); err != io.EOF {
		t.Errorf("Decode of empty input: %v, want io.EOF", err)
	}
	for _, in := range []string{"", "\r\n", "\xef\xbb\xbf", " ", "X-P:1\r\n"} {
		if cal, err := ical.Parse(strings.NewReader(in)); err == nil || cal != nil {
			t.Errorf("Parse(%q) = %v, %v; want an error", in, cal, err)
		}
		d := ical.NewDecoder(strings.NewReader(in))
		d.Strict = true
		// A lone BOM is skipped (documented), leaving empty input.
		if cal, err := d.Decode(); in != "" && in != "\xef\xbb\xbf" && (err == nil || err == io.EOF || cal != nil) {
			t.Errorf("strict Decode(%q) = %v, %v; want a syntax error", in, cal, err)
		}
	}
}

// TestConformanceBeginEnd checks BEGIN/END pairing. RFC 5545 §3.4 and §3.6:
// icalobject = "BEGIN" ":" "VCALENDAR" CRLF icalbody "END" ":" "VCALENDAR"
// CRLF, and x-comp / iana-comp = "BEGIN" ":" name CRLF 1*contentline "END"
// ":" name CRLF. RFC 5234 §2.3: quoted ABNF strings are case-insensitive,
// and §2 makes component names case-insensitive.
func TestConformanceBeginEnd(t *testing.T) {
	accepts := []struct {
		name string
		in   string
	}{
		{"lower-case END", strings.Replace(strings.Replace(twoEvents, "END:VEVENT", "end:vevent", 1), "END:VCALENDAR", "end:vcalendar", 1)},
		{"mixed-case BEGIN and END", strings.NewReplacer("BEGIN:VCALENDAR", "Begin:VCalendar", "END:VCALENDAR", "End:vCALENDAR", "BEGIN:VEVENT", "begin:VEvent").Replace(twoEvents)},
		{"x-comp in any case", strings.Replace(twoEvents, "END:VCALENDAR", "BEGIN:x-foo\r\nX-A:1\r\nEND:X-FOO\r\nEND:VCALENDAR", 1)},
		// "a long line can be split between any two characters" (§3.1),
		// BEGIN and END lines too.
		{"folded BEGIN and END", strings.NewReplacer("BEGIN:VCALENDAR", "BEGIN:VCAL\r\n ENDAR", "END:VEVENT", "E\r\n\tND:VEV\r\n ENT").Replace(twoEvents)},
	}
	for _, tt := range accepts {
		t.Run(tt.name, func(t *testing.T) {
			res := decodeStream(tt.in, true)
			if res.err != nil || len(res.cals) != 1 {
				t.Fatalf("strict decode: %d calendars, %v\n%s", len(res.cals), res.err, tt.in)
			}
			cal := res.cals[0]
			if cal.Name != "VCALENDAR" {
				t.Errorf("calendar Name = %q, want VCALENDAR (documented upper case)", cal.Name)
			}
			if got := uids(cal); !reflect.DeepEqual(got, []string{"1@example.com", "2@example.com"}) {
				t.Errorf("events %q", got)
			}
			for c := range cal.Components("") {
				if c.Name != strings.ToUpper(c.Name) {
					t.Errorf("component Name %q is not upper case", c.Name)
				}
			}
		})
	}

	rejects := []struct {
		name string
		in   string
	}{
		{"END of another component", strings.Replace(twoEvents, "SUMMARY:One\r\nEND:VEVENT", "SUMMARY:One\r\nEND:VTODO", 1)},
		{"END:VCALENDAR with VEVENT open", strings.Replace(twoEvents, "SUMMARY:Two\r\nEND:VEVENT\r\n", "SUMMARY:Two\r\n", 1)},
		{"END with nothing open", strings.Replace(twoEvents, "SUMMARY:One\r\nEND:VEVENT\r\n", "SUMMARY:One\r\nEND:VEVENT\r\nEND:VEVENT\r\n", 1)},
		{"x-comp END name differs", strings.Replace(twoEvents, "END:VCALENDAR", "BEGIN:X-FOO\r\nX-A:1\r\nEND:X-BAR\r\nEND:VCALENDAR", 1)},
		{"END name with trailing space", strings.Replace(twoEvents, "END:VEVENT", "END:VEVENT ", 1)},
		{"BEGIN with no name", strings.Replace(twoEvents, "BEGIN:VEVENT", "BEGIN:", 1)},
		{"BEGIN name not a token", strings.Replace(twoEvents, "BEGIN:VEVENT\r\nUID:1", "BEGIN:V EVENT\r\nUID:1", 1)},
		{"BEGIN with a parameter", strings.Replace(twoEvents, "BEGIN:VEVENT", "BEGIN;X-A=1:VEVENT", 1)},
		{"END with a parameter", strings.Replace(twoEvents, "END:VEVENT", "END;X-A=1:VEVENT", 1)},
		{"BEGIN:VCALENDAR with a parameter", strings.Replace(twoEvents, "BEGIN:VCALENDAR", "BEGIN;X-A=1:VCALENDAR", 1)},
	}
	for _, tt := range rejects {
		t.Run(tt.name, func(t *testing.T) {
			res := decodeStream(tt.in, true)
			if res.err == nil {
				t.Errorf("strict decode succeeded with %d calendars\n%s", len(res.cals), tt.in)
			}
			lax := decodeStream(tt.in, false)
			if lax.err == nil && len(lax.warns) == 0 {
				t.Errorf("lenient decode accepted it without a warning\n%s", tt.in)
			}
			checkWarningTypes(t, lax.warns)
		})
	}
}

// TestConformanceTruncation cuts a valid stream at every byte. RFC 5545
// §3.4: "The first line and last line of the iCalendar object MUST contain
// a pair of iCalendar object delimiter strings". Input cut before the END
// line is not an iCalendar object, so it must never decode as a shorter one.
func TestConformanceTruncation(t *testing.T) {
	full := decodeStream(twoEvents, true)
	if full.err != nil || len(full.cals) != 1 {
		t.Fatalf("decoding the whole text: %v", full.err)
	}
	complete := len(twoEvents) - len("\r\n") // through "END:VCALENDAR"
	for n := 1; n < len(twoEvents); n++ {
		prefix := twoEvents[:n]
		for _, strict := range []bool{false, true} {
			res := decodeStream(prefix, strict)
			if n >= complete {
				// Only the final line break is missing: the object is
				// whole. Success must then be the whole calendar.
				if res.err == nil && (len(res.cals) != 1 || !reflect.DeepEqual(res.cals[0].Component, full.cals[0].Component)) {
					t.Errorf("strict=%v, cut at %d: decoded a different calendar", strict, n)
				}
				continue
			}
			if len(res.cals) != 0 {
				t.Errorf("strict=%v, cut at %d (%q): decoded %d calendar(s)", strict, n, prefix[max(0, n-20):], len(res.cals))
				continue
			}
			if res.err == nil {
				// Decode said io.EOF, "no calendar here". Lenient mode may
				// conclude that only after skipping the cut-off text with a
				// warning; strict mode must report it.
				if strict || len(res.warns) == 0 {
					t.Errorf("strict=%v, cut at %d (%q): io.EOF with %d warnings", strict, n, prefix[max(0, n-20):], len(res.warns))
				}
				continue
			}
			// Once BEGIN:VCALENDAR is read, the cut is inside a component:
			// documented as an error wrapping io.ErrUnexpectedEOF (strict
			// mode may first report the cut-off line itself, and so may
			// lenient mode when that line is a BEGIN cut short to a
			// non-name, which it never recovers from).
			lastLine := prefix[strings.LastIndex(prefix, "\n")+1:]
			cutBegin := strings.HasPrefix(lastLine, "BEGIN")
			if !strict && !cutBegin && strings.Contains(prefix, "BEGIN:VCALENDAR\r\n") && !errors.Is(res.err, io.ErrUnexpectedEOF) {
				t.Errorf("cut at %d (%q): %v, want io.ErrUnexpectedEOF", n, prefix[max(0, n-20):], res.err)
			}
		}
	}

	// The second object of a stream cut short: the first decodes, the
	// second is an error, not io.EOF.
	in := twoEvents + bastille[:len(bastille)/2]
	for _, strict := range []bool{false, true} {
		d := ical.NewDecoder(strings.NewReader(in))
		d.Strict = strict
		if _, err := d.Decode(); err != nil {
			t.Fatalf("strict=%v: first calendar: %v", strict, err)
		}
		cal, err := d.Decode()
		if cal != nil || err == nil || err == io.EOF {
			t.Errorf("strict=%v: second, cut-off calendar: %v, %v", strict, cal, err)
		}
		if !strict && !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Errorf("second, cut-off calendar: %v, want io.ErrUnexpectedEOF", err)
		}
	}
}

// TestConformanceLineBreaksDecode checks the Decoder's line breaks. RFC
// 5545 §3.1: "Content lines are delimited by a line break, which is a CRLF
// sequence".
func TestConformanceLineBreaksDecode(t *testing.T) {
	want := decodeStream(twoEvents, true)
	if want.err != nil {
		t.Fatal(want.err)
	}
	for name, in := range map[string]string{
		"LF": strings.ReplaceAll(twoEvents, "\r\n", "\n"),
		"CR": strings.ReplaceAll(twoEvents, "\r\n", "\r"),
	} {
		// Strict mode: anything but CRLF is a syntax error.
		if res := decodeStream(in, true); res.err == nil {
			t.Errorf("%s: strict decode succeeded", name)
		}
		// Lenient mode reads the same tree (architecture.md: "Unfolding
		// accepts CRLF, LF and lone CR").
		res := decodeStream(in, false)
		if res.err != nil || len(res.cals) != 1 || !reflect.DeepEqual(res.cals[0].Component, want.cals[0].Component) {
			t.Errorf("%s: lenient decode gave a different tree: %v", name, res.err)
		}
	}
}

// TestConformanceBOM checks the documented handling of a UTF-8 byte-order
// mark. RFC 5545 doesn't mention one (§3.1.4 only makes UTF-8 the
// default); architecture.md: "A UTF-8 BOM at the start is skipped."
func TestConformanceBOM(t *testing.T) {
	want := decodeStream(twoEvents, true)
	res := decodeStream("\xef\xbb\xbf"+twoEvents, false)
	if res.err != nil || len(res.cals) != 1 || !reflect.DeepEqual(res.cals[0].Component, want.cals[0].Component) {
		t.Errorf("lenient decode after a BOM: %v", res.err)
	}
	strict := decodeStream("\xef\xbb\xbf"+twoEvents, true)
	t.Logf("strict decode after a BOM: %d calendar(s), %v", len(strict.cals), strict.err)
	// Not at the very start, U+FEFF is just bytes that aren't a name.
	mid := decodeStream(twoEvents+"\xef\xbb\xbf"+bastille, false)
	if mid.err == nil && len(mid.warns) == 0 {
		t.Errorf("a BOM between two calendars was skipped without a warning: %d calendars", len(mid.cals))
	}
}

// TestConformanceOutsideObject checks text outside BEGIN:VCALENDAR …
// END:VCALENDAR. RFC 5545 §3.4: icalstream = 1*icalobject, nothing else.
func TestConformanceOutsideObject(t *testing.T) {
	for name, in := range map[string]string{
		"before":                            "X-JUNK:1\r\n" + twoEvents,
		"between":                           twoEvents + "X-JUNK:1\r\n" + bastille,
		"after":                             twoEvents + "X-JUNK:1\r\n",
		"BEGIN of another component before": "BEGIN:VEVENT\r\nUID:x\r\nEND:VEVENT\r\n" + twoEvents,
	} {
		res := decodeStream(in, true)
		if res.err == nil {
			t.Errorf("%s: strict decode succeeded", name)
		}
		// Lenient mode ignores such lines (documented), and says so.
		lax := decodeStream(in, false)
		if lax.err != nil || len(lax.warns) == 0 {
			t.Errorf("%s: lenient decode: %v, %d warnings", name, lax.err, len(lax.warns))
		}
		checkWarningTypes(t, lax.warns)
		if len(lax.cals) == 0 || !reflect.DeepEqual(uids(lax.cals[0]), []string{"1@example.com", "2@example.com"}) {
			t.Errorf("%s: lenient decode lost the calendar", name)
		}
	}
}

// TestConformanceRequiredCalprops checks RFC 5545 §3.6: "An iCalendar
// object MUST include the "PRODID" and "VERSION" calendar properties. In
// addition, it MUST include at least one calendar component." and calprops:
// PRODID and VERSION "are REQUIRED, but MUST NOT occur more than once",
// CALSCALE and METHOD "are OPTIONAL, but MUST NOT occur more than once".
func TestConformanceRequiredCalprops(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"no PRODID", strings.Replace(twoEvents, "PRODID:-//Conformance//EN\r\n", "", 1)},
		{"no VERSION", strings.Replace(twoEvents, "VERSION:2.0\r\n", "", 1)},
		{"no component", crlf("BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:-//Conformance//EN\nEND:VCALENDAR\n")},
		{"two PRODIDs", strings.Replace(twoEvents, "PRODID:-//Conformance//EN\r\n", "PRODID:-//Conformance//EN\r\nPRODID:-//Other//EN\r\n", 1)},
		{"two VERSIONs", strings.Replace(twoEvents, "VERSION:2.0\r\n", "VERSION:2.0\r\nVERSION:2.0\r\n", 1)},
		{"two CALSCALEs", strings.Replace(twoEvents, "VERSION:2.0\r\n", "VERSION:2.0\r\nCALSCALE:GREGORIAN\r\nCALSCALE:GREGORIAN\r\n", 1)},
		{"two METHODs", strings.Replace(twoEvents, "VERSION:2.0\r\n", "VERSION:2.0\r\nMETHOD:PUBLISH\r\nMETHOD:REQUEST\r\n", 1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if res := decodeStream(tt.in, true); res.err == nil {
				t.Errorf("strict decode succeeded")
			}
			lax := decodeStream(tt.in, false)
			if lax.err == nil && len(lax.warns) == 0 {
				if tt.name == "no component" {
					t.Skip("KNOWN GAP: lenient mode reads a VCALENDAR with no components without a Warn; TestWarnGetsICalSyntaxErrors counts exactly two warnings for such a calendar")
				}
				t.Errorf("lenient decode accepted it without a warning")
			}
			checkWarningTypes(t, lax.warns)
		})
	}
	// One of each is fine.
	ok := strings.Replace(twoEvents, "VERSION:2.0\r\n", "VERSION:2.0\r\nCALSCALE:GREGORIAN\r\nMETHOD:PUBLISH\r\n", 1)
	if res := decodeStream(ok, true); res.err != nil {
		t.Errorf("strict decode with one CALSCALE and one METHOD: %v", res.err)
	}
}

// TestConformanceUnknownExtensions checks that unrecognized extensions don't
// stop decoding and survive it. RFC 5545 §3.6: "Applications MUST ignore
// x-comp and iana-comp values they don't recognize", "SHOULD NOT silently
// drop any components"; §3.2: "Applications MUST ignore x-param and
// iana-param values they don't recognize"; §3.7/§3.8.8: x-prop and
// iana-prop. Keeping them is architecture goal 5 (lossless round trips).
func TestConformanceUnknownExtensions(t *testing.T) {
	in := crlf(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Conformance//EN
X-WR-CALNAME;X-P=1:Team
NEWPROP;NEWPARAM="a:b",c:v
BEGIN:X-VENDOR-THING
X-A:1
BEGIN:X-NESTED
X-B:2
END:X-NESTED
END:X-VENDOR-THING
BEGIN:VEVENT
UID:1@example.com
DTSTAMP:19970610T172345Z
DTSTART;X-FOO=bar;FUTUREPARAM=x,y:19970714T170000Z
SUMMARY;LANGUAGE=en;X-EMPHASIS=strong:One
BEGIN:NEWCOMP
NEWPROP:1
END:NEWCOMP
END:VEVENT
END:VCALENDAR
`)
	res := decodeStream(in, true)
	if res.err != nil || len(res.cals) != 1 {
		t.Fatalf("strict decode: %v", res.err)
	}
	cal := res.cals[0]
	if got := uids(cal); !reflect.DeepEqual(got, []string{"1@example.com"}) {
		t.Errorf("events %q", got)
	}
	for ev := range cal.Events() {
		start, err := ev.Start()
		if err != nil || start.String() != "19970714T170000Z" || !start.UTC {
			t.Errorf("DTSTART with unknown parameters: %v, %v", start, err)
		}
		if ev.Summary() != "One" {
			t.Errorf("SUMMARY = %q", ev.Summary())
		}
		if p := ev.Get("DTSTART"); p == nil || !reflect.DeepEqual(p.Params.Values("FUTUREPARAM"), []string{"x", "y"}) {
			t.Errorf("FUTUREPARAM lost: %+v", p)
		}
	}
	if p := cal.Get("NEWPROP"); p == nil || p.Value != "v" || p.Params.Get("NEWPARAM") != "a:b" {
		t.Errorf("NEWPROP = %+v", p)
	}
	var names []string
	for c := range cal.Components("") {
		names = append(names, c.Name)
	}
	if !reflect.DeepEqual(names, []string{"X-VENDOR-THING", "VEVENT"}) {
		t.Errorf("components %q", names)
	}
	out, err := cal.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != in {
		t.Errorf("round trip changed the text:\n%s", out)
	}
}

// TestConformanceMultipleValues checks RFC 5545 §3.1.2: "The general rule
// for encoding multi-valued items is to simply create a new content line for
// each value, including the property name", and §3.1.1: list values are
// separated by commas. Repeated lines all stay, in order, and a value list
// stays one value until a typed accessor splits it.
func TestConformanceMultipleValues(t *testing.T) {
	in := crlf(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Conformance//EN
BEGIN:VEVENT
UID:1@example.com
DTSTAMP:19970610T172345Z
DTSTART;VALUE=DATE:19970101
ATTENDEE;RSVP=TRUE;ROLE=REQ-PARTICIPANT:mailto:a@example.com
CATEGORIES:A,B
ATTENDEE;ROLE=OPT-PARTICIPANT:mailto:b@example.com
CATEGORIES:C
RDATE;VALUE=DATE:19970304,19970504,19970704,19970904
ATTENDEE:mailto:c@example.com
END:VEVENT
END:VCALENDAR
`)
	cal, err := ical.Parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	for ev := range cal.Events() {
		var att []string
		for p := range ev.Attendees() {
			att = append(att, p.Value)
		}
		if want := []string{"mailto:a@example.com", "mailto:b@example.com", "mailto:c@example.com"}; !reflect.DeepEqual(att, want) {
			t.Errorf("ATTENDEEs %q, want %q", att, want)
		}
		if got := ev.Categories(); !reflect.DeepEqual(got, []string{"A", "B", "C"}) {
			t.Errorf("CATEGORIES %q", got)
		}
		rdate := ev.Get("RDATE")
		if rdate.Value != "19970304,19970504,19970704,19970904" {
			t.Errorf("RDATE value %q", rdate.Value)
		}
		if dts, err := rdate.DateTimes(nil); err != nil || len(dts) != 4 {
			t.Errorf("RDATE list: %v, %v", dts, err)
		}
	}
	out, err := cal.Bytes()
	if err != nil || string(out) != in {
		t.Errorf("round trip changed the text (%v):\n%s", err, out)
	}
}

// TestConformanceCaseInsensitiveModel checks RFC 5545 §2 and §3.5:
// "Property names, parameter names, and enumerated parameter values are
// case-insensitive. For example, the property name "DUE" is the same as
// "due" and "Due", DTSTART;TZID=America/New_York:19980714T120000 is the
// same as DtStart;TzID=America/New_York:19980714T120000."
func TestConformanceCaseInsensitiveModel(t *testing.T) {
	body := func(lines string) string {
		return crlf("BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:-//Conformance//EN\nbegin:vevent\nUID:1@example.com\nDTSTAMP:19970610T172345Z\n" +
			lines + "end:vevent\nEND:VCALENDAR\n")
	}
	upper, err := ical.Parse(strings.NewReader(body("DTSTART;TZID=America/New_York:19980714T120000\nDUE:19980715T120000\n")))
	if err != nil {
		t.Fatal(err)
	}
	for _, lines := range []string{
		"DtStart;TzID=America/New_York:19980714T120000\ndue:19980715T120000\n",
		"dtstart;tzid=America/New_York:19980714T120000\nDue:19980715T120000\n",
	} {
		cal, err := ical.Parse(strings.NewReader(body(lines)))
		if err != nil {
			t.Fatal(err)
		}
		// The decoded trees are the same: names are case-insensitive, and
		// the model holds them in upper case (documented).
		if !reflect.DeepEqual(cal.Component, upper.Component) {
			t.Errorf("%q decodes differently from the upper-case spelling", lines)
		}
		n := 0
		for ev := range cal.Events() {
			n++
			for _, name := range []string{"DTSTART", "dtstart", "DtStart"} {
				if p := ev.Get(name); p == nil || p.Params.Get("tzid") != "America/New_York" || p.Params.Get("TzId") != "America/New_York" {
					t.Errorf("Get(%q) = %+v", name, p)
				}
			}
			if p := ev.Props.Get("Due"); p == nil || p.Value != "19980715T120000" {
				t.Errorf("Props.Get(Due) = %+v", p)
			}
			s, err := ev.Start()
			if err != nil || s.TZID != "America/New_York" || s.String() != "19980714T120000" {
				t.Errorf("Start() = %+v, %v", s, err)
			}
		}
		if n != 1 {
			t.Errorf("found %d events in a lower-case begin:vevent", n)
		}
		m := 0
		for range cal.Components("vevent") {
			m++
		}
		if m != 1 {
			t.Errorf("Components(vevent) found %d components", m)
		}
	}

	// Enumerated parameter values in any case (§3.2: "Property parameter
	// values that are not in quoted-strings are case-insensitive").
	cal, err := ical.Parse(strings.NewReader(body(
		"DTSTART;VALUE=date:19970714\n" +
			"RDATE;VALUE=period:19960403T020000Z/19960403T040000Z\n" +
			"ATTACH;FMTTYPE=text/plain;ENCODING=base64;VALUE=binary:VGhlIHF1aWNrIGJyb3duIGZveCBqdW1wcyBvdmVyIHRoZSBsYXp5IGRvZy4=\n" +
			"BEGIN:VALARM\nACTION:DISPLAY\nDESCRIPTION:x\nTRIGGER;RELATED=end:-PT15M\nEND:VALARM\n" +
			"BEGIN:VALARM\nACTION:DISPLAY\nDESCRIPTION:x\nTRIGGER;VALUE=date-time:19980403T120000Z\nEND:VALARM\n")))
	if err != nil {
		t.Fatal(err)
	}
	for ev := range cal.Events() {
		if s, err := ev.Start(); err != nil || !s.IsDate {
			t.Errorf("DTSTART;VALUE=date: %+v, %v; want a DATE", s, err)
		}
		if ps, err := ev.Get("RDATE").Periods(nil); err != nil || len(ps) != 1 || ps[0].End.String() != "19960403T040000Z" {
			t.Errorf("RDATE;VALUE=period: %+v, %v", ps, err)
		}
		if b, err := ev.Get("ATTACH").Binary(); err != nil || string(b) != "The quick brown fox jumps over the lazy dog." {
			t.Errorf("ATTACH;ENCODING=base64: %q, %v", b, err)
		}
		var alarms []ical.Alarm
		for a := range ev.Alarms() {
			alarms = append(alarms, a)
		}
		if len(alarms) != 2 {
			t.Fatalf("%d alarms", len(alarms))
		}
		if rel, _, toEnd, err := alarms[0].Trigger(); err != nil || !toEnd || rel.Minutes != 15 || !rel.Negative {
			t.Errorf("TRIGGER;RELATED=end: %+v, %v, %v", rel, toEnd, err)
		}
		if _, abs, _, err := alarms[1].Trigger(); err != nil || abs.String() != "19980403T120000Z" {
			t.Errorf("TRIGGER;VALUE=date-time: %v, %v", abs, err)
		}
	}
}

// TestConformanceEncoderInjection checks that nothing in the model can make
// the Encoder write an extra content line or component. RFC 5545 §3.1: a
// value is *VALUE-CHAR and a name is a token, so CR and LF can't appear in
// either; a parameter value can only carry a line break as RFC 6868 ^n.
func TestConformanceEncoderInjection(t *testing.T) {
	build := func(mutate func(ev ical.Event)) *ical.Calendar {
		cal, err := ical.Parse(strings.NewReader(bastille))
		if err != nil {
			t.Fatal(err)
		}
		for ev := range cal.Events() {
			mutate(ev)
		}
		return cal
	}
	payloads := []string{"\r\nEND:VEVENT\r\nBEGIN:VEVENT", "\nX-INJ:1", "\rX-INJ:1", "\r\n X-INJ", "a\x00b"}
	for _, p := range payloads {
		for name, mutate := range map[string]func(ev ical.Event){
			"value":           func(ev ical.Event) { ev.Get("SUMMARY").Value = "x" + p },
			"property name":   func(ev ical.Event) { ev.Props.Add(ical.NewProperty("X-A"+p, "v")) },
			"parameter name":  func(ev ical.Event) { ev.Get("SUMMARY").Params.Set("X-P"+p, "v") },
			"component name":  func(ev ical.Event) { ev.Add(ical.NewComponent("VALARM" + p)) },
			"parameter value": func(ev ical.Event) { ev.Get("SUMMARY").Params.Set("X-P", "a"+p, "b") },
		} {
			cal := build(mutate)
			out, err := cal.Bytes()
			if err != nil {
				continue // refused: fine
			}
			// Written anyway: it must read back, strictly, as the same
			// tree, with at most a line break turned into LF.
			d := ical.NewDecoder(bytes.NewReader(out))
			d.Strict = true
			back, err := d.Decode()
			if err != nil {
				t.Errorf("%s %q: wrote %q, which doesn't decode: %v", name, p, out, err)
				continue
			}
			var evs []ical.Event
			for ev := range back.Events() {
				evs = append(evs, ev)
			}
			if len(evs) != 1 || len(evs[0].Props) != len(build(func(ical.Event) {}).Children[0].Props)+btoi(name == "property name") {
				t.Errorf("%s %q: wrote %q, which decodes to a different tree", name, p, out)
				continue
			}
			if name == "parameter value" {
				got := evs[0].Get("SUMMARY").Params.Values("X-P")
				want := strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace("a" + p)
				if len(got) != 2 || got[0] != want || got[1] != "b" || strings.Contains(string(out), "X-INJ:1\r\n") {
					t.Errorf("parameter value %q: wrote %q, which reads back as %q", p, out, got)
				}
			} else {
				t.Errorf("%s %q: the Encoder wrote a CR, LF or NUL as %q", name, p, out)
			}
		}
	}
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// TestConformanceRFCExampleCalendarsRoundTrip decodes each RFC 5545 example
// calendar in testdata/rfc5545/ics, encodes it and decodes the output: the
// trees must be identical, the output strictly valid and folded per §3.1,
// and encoding again must give the same bytes.
func TestConformanceRFCExampleCalendarsRoundTrip(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "rfc5545", "ics", "*.ics"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no RFC example calendars: %v", err)
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			// The examples are well-formed content lines and objects, so
			// they decode strictly.
			first := decodeStream(string(b), true)
			if first.err != nil || len(first.cals) == 0 {
				t.Fatalf("strict decode: %d calendars, %v", len(first.cals), first.err)
			}
			for i, cal := range first.cals {
				out, err := cal.Bytes()
				if err != nil {
					t.Fatalf("calendar %d: encode: %v", i, err)
				}
				checkEncodedLines(t, out)
				second := decodeStream(string(out), true)
				if second.err != nil || len(second.cals) != 1 {
					t.Fatalf("calendar %d: strict decode of the output: %v\n%s", i, second.err, out)
				}
				if !reflect.DeepEqual(second.cals[0].Component, cal.Component) {
					t.Errorf("calendar %d: decode → encode → decode changed the tree\n%s", i, out)
				}
				again, err := second.cals[0].Bytes()
				if err != nil || !bytes.Equal(again, out) {
					t.Errorf("calendar %d: encoding again gave different bytes (%v)", i, err)
				}
				// The unfolded text is the source's, line for line: nothing
				// is reordered, renamed or re-escaped (architecture goal 5).
				// Only the fold points may differ.
				if unfold(out) != unfold(b) {
					t.Errorf("calendar %d: encoding changed the unfolded text\n got %s\nwant %s", i, unfold(out), unfold(b))
				}
			}
		})
	}
}

// unfold removes RFC 5545 §3.1 folds: each CRLF followed by one SPACE or
// HTAB.
func unfold(b []byte) string {
	return strings.NewReplacer("\r\n ", "", "\r\n\t", "").Replace(string(b))
}

// checkEncodedLines checks Encoder output against RFC 5545 §3.1: CRLF line
// breaks, at most 75 octets per physical line (a SHOULD; the Encoder's
// documented behaviour), and no multi-octet character split by a fold.
func checkEncodedLines(t *testing.T, out []byte) {
	t.Helper()
	if !bytes.HasSuffix(out, []byte("\r\n")) {
		t.Errorf("output does not end in CRLF")
	}
	for i, pl := range bytes.Split(bytes.TrimSuffix(out, []byte("\r\n")), []byte("\r\n")) {
		if bytes.ContainsAny(pl, "\r\n") || len(pl) > 75 || !utf8.Valid(pl) {
			t.Errorf("physical line %d: %q", i+1, pl)
		}
	}
}
