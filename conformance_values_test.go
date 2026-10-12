package ical_test

// Conformance tests for RFC 5545 §3.3 property value types, read and written
// through the public Property API. Every expected value comes from the RFC
// text (and its verified errata), cited next to the case.
//
// The value accessors have no strict mode: they are lenient by design. So an
// input the grammar forbids passes if it is rejected, or if it is accepted
// with the one reading a caller could expect (logged as LENIENT, so the audit
// can list them); it fails if it is accepted with any other value. Valid
// input must be accepted with the right value, and setters must write exactly
// what the grammar allows.

import (
	"bytes"
	"encoding/base64"
	"math"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	ical "github.com/aanelson/iCalendar"
)

// cvLine reads one content line, inside a VEVENT, through the decoder and
// returns its property, so values reach the accessors the way a feed's do.
func cvLine(t *testing.T, line string) *ical.Property {
	t.Helper()
	src := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//conformance//EN\r\n" +
		"BEGIN:VEVENT\r\n" + line + "\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	cal, err := ical.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("parse %q: %v", line, err)
	}
	ev := cal.Children[0]
	if len(ev.Props) != 1 {
		t.Fatalf("parse %q: got %d properties", line, len(ev.Props))
	}
	return ev.Props[0]
}

// cvEncode writes p as a content line and returns it unfolded, without CRLF.
func cvEncode(t *testing.T, p *ical.Property) string {
	t.Helper()
	c := ical.NewComponent("VEVENT")
	c.Props.Add(p)
	var b bytes.Buffer
	if err := ical.NewEncoder(&b).EncodeComponent(c); err != nil {
		t.Fatalf("encode %s: %v", p.Name, err)
	}
	// RFC 5545 §3.1: a CRLF followed by one space or tab is a fold.
	s := strings.NewReplacer("\r\n ", "", "\r\n\t", "").Replace(b.String())
	lines := strings.Split(strings.TrimSuffix(s, "\r\n"), "\r\n")
	if len(lines) != 3 {
		t.Fatalf("encode %s: got %q", p.Name, b.String())
	}
	return lines[1]
}

// cvRoundTrip encodes p in a calendar, parses the result and returns the
// property read back.
func cvRoundTrip(t *testing.T, p *ical.Property) *ical.Property {
	t.Helper()
	cal := ical.NewCalendar("-//conformance//EN")
	ev := ical.NewComponent("VEVENT")
	ev.Props.Add(p)
	cal.Add(ev)
	b, err := cal.Bytes()
	if err != nil {
		t.Fatalf("encode %s: %v", p.Name, err)
	}
	back, err := ical.Parse(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("parse back %q: %v", b, err)
	}
	for c := range back.Components("VEVENT") {
		if q := c.Get(p.Name); q != nil {
			return q
		}
	}
	t.Fatalf("%s lost in %q", p.Name, b)
	return nil
}

// cvForbidden checks the result of reading an input the grammar forbids:
// rejecting it is right, and so is accepting it as one of ok, the readings a
// caller could expect. Anything else is a wrong value.
func cvForbidden[T comparable](t *testing.T, what, in string, got T, err error, ok ...T) {
	t.Helper()
	if err != nil {
		return
	}
	for _, o := range ok {
		if got == o {
			t.Logf("LENIENT %s %q -> %v", what, in, got)
			return
		}
	}
	t.Errorf("%s %q: accepted as %v, which the grammar forbids and no reading of it gives", what, in, got)
}

// cvParamValues returns the values of every parameter named name, so a
// duplicated parameter shows.
func cvParamValues(p *ical.Property, name string) []string {
	var vs []string
	for _, prm := range p.Params {
		if strings.EqualFold(prm.Name, name) {
			vs = append(vs, prm.Values...)
		}
	}
	return vs
}

// ---------------------------------------------------------------------------
// §3.3.1 BINARY

// RFC 5545 §3.3.1:
//
//	binary     = *(4b-char) [b-end]
//	b-end      = (2b-char "==") / (3b-char "=")
//	b-char = ALPHA / DIGIT / "+" / "/"
var cvBinaryABNF = regexp.MustCompile(`^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$`)

func TestConformanceValueBinary(t *testing.T) {
	valid := []struct{ in, want string }{
		// RFC 5545 §3.3.1: "*(4b-char)" may be empty.
		{"", ""},
		// RFC 4648 §10 test vectors, which §3.3.1 cites for BASE64.
		{"Zg==", "f"},
		{"Zm8=", "fo"},
		{"Zm9v", "foo"},
		{"Zm9vYg==", "foob"},
		{"Zm9vYmE=", "fooba"},
		{"Zm9vYmFy", "foobar"},
		// RFC 5545 §3.3.1: b-char includes "+" and "/".
		{"+/+/", "\xfb\xff\xbf"},
		// RFC 5545 §3.1.3 example, with the "=" padding erratum 5602
		// (Verified) appends.
		{"VGhlIHF1aWNrIGJyb3duIGZveCBqdW1wcyBvdmVyIHRoZSBsYXp5IGRvZy4=",
			"The quick brown fox jumps over the lazy dog."},
	}
	for _, c := range valid {
		// RFC 5545 §3.3.1: "Property values with this value type MUST also
		// include the inline encoding parameter sequence of
		// ";ENCODING=BASE64"."
		p := cvLine(t, "ATTACH;FMTTYPE=text/plain;ENCODING=BASE64;VALUE=BINARY:"+c.in)
		got, err := p.Binary()
		if err != nil || string(got) != c.want {
			t.Errorf("Binary(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}

	// RFC 5545 §3.2: parameter values such as BASE64 and BINARY are
	// case-insensitive.
	p := cvLine(t, "ATTACH;encoding=base64;value=binary:Zm9v")
	if got, err := p.Binary(); err != nil || string(got) != "foo" {
		t.Errorf("Binary with lower-case parameters = %q, %v; want foo", got, err)
	}

	forbidden := []struct {
		in string
		ok []string // readings a lenient reader may give
	}{
		// Erratum 5602 (Verified): BASE64 values carry "=" padding as
		// RFC 4648 requires; b-end needs it.
		{"Zg", []string{"f"}},
		{"Zm8", []string{"fo"}},
		{"Zg=", []string{"f"}},
		// No b-end has one b-char.
		{"Z", nil},
		{"Zm9vY", nil},
		{"Z===", nil},
		// Padding only at the end.
		{"Zg==Zg==", []string{"ff"}},
		// b-char is ALPHA / DIGIT / "+" / "/": not the RFC 4648 §5 URL
		// alphabet, not whitespace, not anything else.
		{"-_-_", []string{"\xfb\xff\xbf"}},
		{"Zm9v Yg==", []string{"foob"}},
		{"Zm9v*", nil},
		{"Zm9v\\,", nil},
	}
	for _, c := range forbidden {
		p := cvLine(t, "ATTACH;ENCODING=BASE64;VALUE=BINARY:"+c.in)
		got, err := p.Binary()
		cvForbidden(t, "BINARY", c.in, string(got), err, c.ok...)
	}

	// The grammar allows non-zero pad bits ("Zh==" has them), but RFC 4648
	// §3.5 lets a decoder reject them; either way is conformant, so only
	// the value is checked.
	if got, err := cvLine(t, "ATTACH;ENCODING=BASE64;VALUE=BINARY:Zh==").Binary(); err == nil && string(got) != "f" {
		t.Errorf("Binary(Zh==) = %q; want f or an error", got)
	}

	// RFC 5545 §3.3.1: the ENCODING=BASE64 parameter is REQUIRED with
	// BINARY; without it the value isn't BINARY. §3.2.7: "If the value
	// type parameter is ";VALUE=BINARY", then the inline encoding
	// parameter MUST be specified with the value ";ENCODING=BASE64"."
	for _, line := range []string{
		"ATTACH;VALUE=BINARY:Zm9v",
		"ATTACH;ENCODING=8BIT;VALUE=BINARY:Zm9v",
	} {
		got, err := cvLine(t, line).Binary()
		cvForbidden(t, "BINARY", line, string(got), err, "foo")
	}
}

func TestConformanceValueSetBinary(t *testing.T) {
	all := make([]byte, 256)
	for i := range all {
		all[i] = byte(i)
	}
	for _, b := range [][]byte{
		{}, {0}, {0, 1}, {0, 1, 2}, {0xff, 0xfe, 0xfd, 0xfc},
		[]byte("The quick brown fox jumps over the lazy dog."),
		all, bytes.Repeat(all, 4),
	} {
		p := ical.NewProperty("ATTACH", "")
		// Start from parameters the value must replace, not add to.
		p.Params.Set("ENCODING", "8BIT")
		p.Params.Set("VALUE", "URI")
		p.SetBinary(b)

		// RFC 5545 §3.3.1 grammar, padded as erratum 5602 requires.
		if !cvBinaryABNF.MatchString(p.Value) {
			t.Errorf("SetBinary(% x) wrote %q, which isn't §3.3.1 binary", b, p.Value)
		}
		// RFC 5545 §3.3.1: "MUST also include ... ;ENCODING=BASE64";
		// §3.2.20: a non-default type "MUST be specified" with VALUE.
		if v := cvParamValues(p, "ENCODING"); len(v) != 1 || !strings.EqualFold(v[0], "BASE64") {
			t.Errorf("SetBinary: ENCODING = %q; want exactly BASE64", v)
		}
		if v := cvParamValues(p, "VALUE"); len(v) != 1 || !strings.EqualFold(v[0], "BINARY") {
			t.Errorf("SetBinary: VALUE = %q; want exactly BINARY", v)
		}
		// RFC 4648, which §3.3.1 cites, defines the one encoding of b.
		if want := base64.StdEncoding.EncodeToString(b); p.Value != want {
			t.Errorf("SetBinary(% x) = %q; want %q", b, p.Value, want)
		}
		if got, err := p.Binary(); err != nil || !bytes.Equal(got, b) {
			t.Errorf("SetBinary then Binary = % x, %v; want % x", got, err, b)
		}
		// Through the encoder (folded at 75 octets) and back.
		if got, err := cvRoundTrip(t, p).Binary(); err != nil || !bytes.Equal(got, b) {
			t.Errorf("SetBinary, encode, parse, Binary = % x, %v; want % x", got, err, b)
		}
	}
}

// ---------------------------------------------------------------------------
// §3.3.2 BOOLEAN

func TestConformanceValueBool(t *testing.T) {
	// RFC 5545 §3.3.2: boolean = "TRUE" / "FALSE"; "These values are
	// case-insensitive text."
	for in, want := range map[string]bool{
		"TRUE": true, "FALSE": false,
		"true": true, "false": false,
		"True": true, "fAlSe": false,
	} {
		got, err := cvLine(t, "X-BOOL;VALUE=BOOLEAN:"+in).Bool()
		if err != nil || got != want {
			t.Errorf("Bool(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	// Anything else isn't a BOOLEAN.
	for in, ok := range map[string][]bool{
		"":         nil,
		"T":        {true},
		"F":        {false},
		"1":        {true},
		"0":        {false},
		"YES":      {true},
		"NO":       {false},
		"ON":       {true},
		"TRUEX":    nil,
		"FALS":     nil,
		"TRUE,":    nil,
		"\"TRUE\"": {true},
		" TRUE":    {true},
		"TRUE ":    {true},
	} {
		p := &ical.Property{Name: "X-BOOL", Value: in}
		got, err := p.Bool()
		cvForbidden(t, "BOOLEAN", in, got, err, ok...)
	}
}

// ---------------------------------------------------------------------------
// §3.3.3 CAL-ADDRESS and §3.3.13 URI

func TestConformanceValueURI(t *testing.T) {
	src := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//conformance//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:1\r\nDTSTAMP:19970101T000000Z\r\n" +
		// RFC 5545 §3.3.3 example.
		"ORGANIZER;CN=\"Doe, Jane\":mailto:jane_doe@example.com\r\n" +
		// RFC 3986 §2.2: "," and ";" are sub-delims, legal in a URI; RFC
		// 5545 §3.3.3, §3.3.13: "No additional content value encoding
		// (i.e., BACKSLASH character encoding ...) is defined for this
		// value type", so neither is a list separator or an escape.
		"ATTENDEE;MEMBER=\"mailto:dev@example.com\":mailto:a,b;c@example.com\r\n" +
		"URL:http://example.com/a,b;c=d?q=1,2;3#x\r\n" +
		"END:VEVENT\r\nEND:VCALENDAR\r\n"
	cal, err := ical.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	for ev := range cal.Events() {
		if got := ev.Organizer().Value; got != "mailto:jane_doe@example.com" {
			t.Errorf("ORGANIZER = %q", got)
		}
		for a := range ev.Attendees() {
			if a.Value != "mailto:a,b;c@example.com" {
				t.Errorf("ATTENDEE = %q", a.Value)
			}
			// RFC 5545 §3.3.13: "When a property parameter value is a URI
			// value type, the URI MUST be specified as a quoted-string
			// value"; the quotes aren't part of it.
			if got := a.Params.Get("MEMBER"); got != "mailto:dev@example.com" {
				t.Errorf("MEMBER = %q", got)
			}
		}
		if got := ev.URL(); got != "http://example.com/a,b;c=d?q=1,2;3#x" {
			t.Errorf("URL() = %q; want the URI as written", got)
		}
	}

	// A backslash isn't legal in a URI (RFC 3986), so a TEXT-escaped URL is
	// malformed; since URI has no BACKSLASH encoding, its value is the raw
	// text. Undoing the escape is a repair a lenient reader may make.
	ev := ical.Event{Item: ical.Item{Component: ical.NewComponent("VEVENT")}}
	ev.Props.Add(ical.NewProperty("URL", `http://example.com/a\,b`))
	if got := ev.URL(); got != `http://example.com/a\,b` {
		t.Logf("LENIENT URI %q -> %q", `http://example.com/a\,b`, got)
	}

	// Writing: the value is written as is, with no escaping.
	for name, v := range map[string]string{
		"URL":       "http://example.com/a,b;c=d?q=1,2;3#x",
		"ATTENDEE":  "mailto:a,b;c@example.com",
		"ORGANIZER": "mailto:jane_doe@example.com",
	} {
		if got := cvEncode(t, ical.NewProperty(name, v)); got != name+":"+v {
			t.Errorf("encode %s = %q; want %q", name, got, name+":"+v)
		}
	}

	// RFC 5545 §3.3.13: URI-valued parameters (ALTREP §3.2.1, DELEGATED-FROM
	// §3.2.4, DELEGATED-TO §3.2.5, DIR §3.2.6, MEMBER §3.2.11, SENT-BY
	// §3.2.18) "MUST be specified as a quoted-string value".
	for _, prm := range []string{"ALTREP", "DELEGATED-FROM", "DELEGATED-TO", "DIR", "MEMBER", "SENT-BY"} {
		p := ical.NewProperty("ATTENDEE", "mailto:a@example.com")
		p.Params.Set(prm, "mailto:b@example.com")
		want := prm + `="mailto:b@example.com"`
		if got := cvEncode(t, p); !strings.Contains(got, want) {
			t.Errorf("encode %s parameter = %q; want it quoted: %s", prm, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// §3.3.7 FLOAT

func TestConformanceValueFloat(t *testing.T) {
	// RFC 5545 §3.3.7: float = (["+"] / "-") 1*DIGIT ["." 1*DIGIT]
	for in, want := range map[string]float64{
		// The section's examples.
		"1000000.0000001": 1000000.0000001,
		"1.333":           1.333,
		"-3.14":           -3.14,
		"+3.14":           3.14,
		"0":               0,
		"-0":              0,
		"10":              10,
		"007.50":          7.5,
		"0.000001":        0.000001,
	} {
		got, err := cvLine(t, "X-FLOAT;VALUE=FLOAT:"+in).Float()
		if err != nil || got != want {
			t.Errorf("Float(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for in, ok := range map[string][]float64{
		"":      nil,
		"+":     nil,
		"-":     nil,
		".":     nil,
		".5":    {0.5},
		"5.":    {5},
		"1.2.3": nil,
		"--1":   nil,
		"+-1":   nil,
		"1,5":   nil,
		" 1.5":  {1.5},
		"1.5 ":  {1.5},
		"1_000": nil,
		"١٢":    nil,
		// No exponent in the grammar.
		"1e3":    {1000},
		"1E3":    {1000},
		"1.5e-3": {0.0015},
		// Not real numbers, and not decimal.
		"NaN":       nil,
		"nan":       nil,
		"Inf":       nil,
		"+Inf":      nil,
		"-Infinity": nil,
		"0x10":      nil,
		"0x1p3":     nil,
	} {
		p := &ical.Property{Name: "X-FLOAT", Value: in}
		got, err := p.Float()
		cvForbidden(t, "FLOAT", in, got, err, ok...)
	}
}

// ---------------------------------------------------------------------------
// §3.3.8 INTEGER

func TestConformanceValueInt(t *testing.T) {
	// RFC 5545 §3.3.8: integer = (["+"] / "-") 1*DIGIT; "The valid range
	// for "integer" is -2147483648 to 2147483647. If the sign is not
	// specified, then the value is assumed to be positive."
	for in, want := range map[string]int{
		// The section's examples.
		"1234567890":  1234567890,
		"-1234567890": -1234567890,
		"+1234567890": 1234567890,
		"432109876":   432109876,
		"0":           0,
		"-0":          0,
		"+0":          0,
		"007":         7,
		"2147483647":  2147483647,
		"-2147483648": -2147483648,
	} {
		got, err := cvLine(t, "PRIORITY:"+in).Int()
		if err != nil || got != want {
			t.Errorf("Int(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for in, ok := range map[string][]int{
		// Outside the valid range: the number itself is unambiguous.
		"2147483648":  {2147483648},
		"-2147483649": {-2147483649},
		// Beyond any Go int: no value it could be read as is right.
		"99999999999999999999":  nil,
		"-99999999999999999999": nil,
		"":                      nil,
		"+":                     nil,
		"-":                     nil,
		"--1":                   nil,
		"+-1":                   nil,
		"1.0":                   {1},
		"1e3":                   nil,
		"0x10":                  nil,
		"0b1":                   nil,
		"1_000":                 nil,
		" 1":                    {1},
		"1 ":                    {1},
		"١":                     nil,
	} {
		p := &ical.Property{Name: "PRIORITY", Value: in}
		got, err := p.Int()
		cvForbidden(t, "INTEGER", in, got, err, ok...)
	}

	for _, n := range []int{0, 1, -1, 9, 2147483647, -2147483648} {
		p := ical.NewProperty("SEQUENCE", "x")
		p.SetInt(n)
		if !regexp.MustCompile(`^[+-]?[0-9]+$`).MatchString(p.Value) {
			t.Errorf("SetInt(%d) wrote %q", n, p.Value)
		}
		if got, err := cvRoundTrip(t, p).Int(); err != nil || got != n {
			t.Errorf("SetInt(%d) round trip = %d, %v", n, got, err)
		}
	}
}

// ---------------------------------------------------------------------------
// §3.8.1.6 GEO

// RFC 5545 §3.8.1.6: geovalue = float ";" float, with §3.3.7's float.
var cvGeoABNF = regexp.MustCompile(`^[+-]?[0-9]+(?:\.[0-9]+)?;[+-]?[0-9]+(?:\.[0-9]+)?$`)

func TestConformanceValueGeo(t *testing.T) {
	type ll struct{ lat, lon float64 }
	for in, want := range map[string]ll{
		// RFC 5545 §3.8.1.6 example.
		"37.386013;-122.082932": {37.386013, -122.082932},
		"+37.5;+122":            {37.5, 122},
		"0;0":                   {0, 0},
		"-90;180":               {-90, 180},
		"90.000000;-180.000000": {90, -180},
	} {
		lat, lon, err := cvLine(t, "GEO:"+in).Geo()
		if err != nil || lat != want.lat || lon != want.lon {
			t.Errorf("Geo(%q) = %v, %v, %v; want %v", in, lat, lon, err, want)
		}
	}
	for in, ok := range map[string][]ll{
		// Common producer mistakes with one obvious reading.
		"37.386013,-122.082932":  {{37.386013, -122.082932}},
		`37.386013\;-122.082932`: {{37.386013, -122.082932}},
		"37.386013; -122.082932": {{37.386013, -122.082932}},
		"37.386013;-122.082932;": {{37.386013, -122.082932}},
		"1e1;2":                  {{10, 2}},
		// Not two FLOATs.
		"37.386013":    nil,
		"37.386013;":   nil,
		";-122.082932": nil,
		"1;2;3":        nil,
		"abc;def":      nil,
		"NaN;0":        nil,
		"0;Inf":        nil,
	} {
		p := &ical.Property{Name: "GEO", Value: in}
		lat, lon, err := p.Geo()
		cvForbidden(t, "GEO", in, ll{lat, lon}, err, ok...)
	}
	// RFC 5545 §3.8.1.6 describes latitude 0–90 and longitude 0–180 with a
	// lower-case "shall", which isn't an RFC 2119 requirement, so values
	// outside are only reported.
	for _, in := range []string{"91;0", "-91;0", "0;181", "0;-181"} {
		if lat, lon, err := (&ical.Property{Name: "GEO", Value: in}).Geo(); err == nil {
			t.Logf("LENIENT GEO out of range %q -> %v;%v", in, lat, lon)
		}
	}

	for _, c := range []ll{
		{37.386013, -122.082932},
		{0, 0},
		{-90, 180},
		{90, -180},
		// "Receiving applications MUST accept values of this precision"
		// (six decimal places), so they must survive a round trip.
		{0.000001, -0.000001},
		{12.345678, -0.000009},
		// Smaller than six places: the grammar has no exponent, so it must
		// still be written as digits.
		{0.0000001, 1e-10},
		{-0.00000001, 179.9999999},
	} {
		p := ical.NewProperty("GEO", "")
		p.SetGeo(c.lat, c.lon)
		if !cvGeoABNF.MatchString(p.Value) {
			t.Errorf("SetGeo(%v, %v) wrote %q, which isn't float \";\" float", c.lat, c.lon, p.Value)
			continue
		}
		lat, lon, err := cvRoundTrip(t, p).Geo()
		// §3.8.1.6: receivers "MAY truncate values of greater precision"
		// than six decimal places.
		if err != nil || math.Abs(lat-c.lat) > 5e-7 || math.Abs(lon-c.lon) > 5e-7 {
			t.Errorf("SetGeo(%v, %v) round trip = %v, %v, %v", c.lat, c.lon, lat, lon, err)
		}
	}
}

// ---------------------------------------------------------------------------
// §3.3.14 UTC-OFFSET

// RFC 5545 §3.3.14: utc-offset = time-numzone;
// time-numzone = ("+" / "-") time-hour time-minute [time-second], with
// §3.3.12's ranges (hour 00-23, minute 00-59) and "The time-second, if
// present, MUST NOT be 60".
var cvUTCOffsetABNF = regexp.MustCompile(`^[+-](?:[01][0-9]|2[0-3])[0-5][0-9](?:[0-5][0-9])?$`)

func TestConformanceValueUTCOffset(t *testing.T) {
	for in, want := range map[string]int{
		// The section's examples: New York and Geneva standard time.
		"-0500":   -5 * 3600,
		"+0100":   3600,
		"+0000":   0,
		"+000000": 0,
		"+0530":   19800,
		"-0330":   -12600,
		"+0545":   20700,
		"+1400":   14 * 3600,
		"-1200":   -12 * 3600,
		// "[time-second]": "if absent, it defaults to zero".
		"+013015": 3600 + 30*60 + 15,
		"-000001": -1,
		"+2359":   23*3600 + 59*60,
		"-235959": -(23*3600 + 59*60 + 59),
	} {
		got, err := ical.ParseUTCOffset(in)
		if err != nil || got != want {
			t.Errorf("ParseUTCOffset(%q) = %v, %v; want %v", in, got, err, want)
		}
		got, err = cvLine(t, "TZOFFSETFROM:"+in).UTCOffset()
		if err != nil || got != want {
			t.Errorf("UTCOffset(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for in, ok := range map[string][]int{
		// "The PLUS SIGN character MUST be specified for positive UTC
		// offsets".
		"0500":   {5 * 3600},
		"053000": {5*3600 + 30*60},
		// "The value of "-0000" and "-000000" are not allowed."
		"-0000":   {0},
		"-000000": {0},
		// Field ranges: hour 00-23, minute 00-59, second 00-59 ("MUST
		// NOT be 60").
		"+2400":   nil,
		"+9900":   nil,
		"+0560":   nil,
		"+050060": nil,
		"+050099": nil,
		// Lengths: 4 or 6 digits.
		"+05":      nil,
		"+050":     nil,
		"+05000":   nil,
		"+0500000": nil,
		// Other shapes.
		"":       nil,
		"+":      nil,
		"Z":      nil,
		"+05:00": {5 * 3600},
		"++0500": nil,
		"+-0500": nil,
		"+05a0":  nil,
		" +0500": {5 * 3600},
		"+0500 ": {5 * 3600},
		"UTC":    nil,
	} {
		got, err := ical.ParseUTCOffset(in)
		cvForbidden(t, "UTC-OFFSET", in, got, err, ok...)
		got, err = (&ical.Property{Name: "TZOFFSETTO", Value: in}).UTCOffset()
		cvForbidden(t, "UTC-OFFSET (Property)", in, got, err, ok...)
	}

	for _, n := range []int{0, 1, -1, 59, -59, 60, 3600, -18000, 19800, 20700, 5415, -12600,
		14 * 3600, -12 * 3600, 86399, -86399} {
		s := ical.FormatUTCOffset(n)
		// "-0000" and "-000000" are not allowed, and the sign is required.
		if !cvUTCOffsetABNF.MatchString(s) || s == "-0000" || s == "-000000" {
			t.Errorf("FormatUTCOffset(%d) = %q, which isn't a UTC-OFFSET", n, s)
			continue
		}
		if got, err := ical.ParseUTCOffset(s); err != nil || got != n {
			t.Errorf("FormatUTCOffset(%d) = %q, which reads back as %d, %v", n, s, got, err)
		}
		p := ical.NewProperty("TZOFFSETTO", "")
		p.SetUTCOffset(n)
		if p.Value != s {
			t.Errorf("SetUTCOffset(%d) wrote %q; want %q", n, p.Value, s)
		}
		if got, err := cvRoundTrip(t, p).UTCOffset(); err != nil || got != n {
			t.Errorf("SetUTCOffset(%d) round trip = %d, %v", n, got, err)
		}
	}
	// The examples' exact text.
	for n, want := range map[int]string{-18000: "-0500", 3600: "+0100", 0: "+0000"} {
		if got := ical.FormatUTCOffset(n); got != want && got != want+"00" {
			t.Errorf("FormatUTCOffset(%d) = %q; want %q", n, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// §3.3.11 TEXT

// cvValidText reports whether s matches RFC 5545 §3.3.11:
//
//	text       = *(TSAFE-CHAR / ":" / DQUOTE / ESCAPED-CHAR)
//	ESCAPED-CHAR = ("\\" / "\;" / "\," / "\N" / "\n")
//	TSAFE-CHAR = WSP / %x21 / %x23-2B / %x2D-39 / %x3C-5B /
//	             %x5D-7E / NON-US-ASCII
//
// with a list of TEXT values separated by unescaped commas when list is set.
func cvValidText(s string, list bool) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\':
			if i+1 == len(s) || !strings.ContainsRune(`\;,Nn`, rune(s[i+1])) {
				return false
			}
			i++
		case c == ',':
			if !list {
				return false
			}
		case c == ';':
			return false
		case c == ' ' || c == '\t':
		case c < 0x20 || c == 0x7f: // CONTROL, §3.1
			return false
		}
	}
	return true
}

// cvTextNormal is what s reads back as after EscapeText: RFC 5545 §3.3.11
// has one way to write a line break (\n or \N), so CRLF and CR become LF,
// and a CONTROL (§3.1: %x00-08 / %x0A-1F / %x7F) other than a line break
// can't be written in TEXT at all. Dropping it is the documented choice
// (EscapeText's doc comment).
func cvTextNormal(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.Map(func(r rune) rune {
		if r != '\n' && r != '\t' && (r < 0x20 || r == 0x7f) {
			return -1
		}
		return r
	}, s)
}

func TestConformanceValueText(t *testing.T) {
	for in, want := range map[string]string{
		// RFC 5545 §3.3.11 example.
		`Project XYZ Final Review\nConference Room - 3B\nCome Prepared.`: "Project XYZ Final Review\nConference Room - 3B\nCome Prepared.",
		// ESCAPED-CHAR: "\\ encodes \, \N or \n encodes newline; \; encodes
		// ;, \, encodes ,".
		`a\\b`:   `a\b`,
		`a\;b`:   "a;b",
		`a\,b`:   "a,b",
		`a\nb`:   "a\nb",
		`a\Nb`:   "a\nb",
		`\\n`:    `\n`,
		`\\\n`:   "\\\n",
		`\\\\`:   `\\`,
		`\\N\N`:  "\\N\n",
		`\;\,\\`: `;,\`,
		// "a COLON character in a "TEXT" property value SHALL NOT be
		// escaped": it appears as is. DQUOTE is in the text rule too.
		"a:b":        "a:b",
		`say "hi"`:   `say "hi"`,
		"mailto:x@y": "mailto:x@y",
		// TSAFE-CHAR includes WSP (space and HTAB) and NON-US-ASCII.
		"a\tb  c":  "a\tb  c",
		"Café ☕ 😀": "Café ☕ 😀",
		"":         "",
	} {
		if got := cvLine(t, "SUMMARY:"+in).Text(); got != want {
			t.Errorf("Text(%q) = %q; want %q", in, got, want)
		}
		if got := ical.UnescapeText(in); got != want {
			t.Errorf("UnescapeText(%q) = %q; want %q", in, got, want)
		}
	}

	// RFC 5545 §3.3.11: "If the property permits, multiple TEXT values are
	// specified by a COMMA-separated list of values", and a comma inside a
	// value "MUST be escaped".
	for in, want := range map[string][]string{
		// RFC 5545 §3.8.1.2 and §3.8.1.10 examples.
		"APPOINTMENT,EDUCATION": {"APPOINTMENT", "EDUCATION"},
		"EASEL,PROJECTOR,VCR":   {"EASEL", "PROJECTOR", "VCR"},
		"MEETING":               {"MEETING"},
		`a\,b,c`:                {"a,b", "c"},
		// An escaped backslash before a comma leaves the comma a
		// separator.
		`a\\,b`:     {`a\`, "b"},
		`a\\\,b`:    {`a\,b`},
		`a\;b,c\nd`: {"a;b", "c\nd"},
		// text = *(...) may be empty.
		"a,,b": {"a", "", "b"},
		",":    {"", ""},
	} {
		got := cvLine(t, "CATEGORIES:"+in).Texts()
		if strings.Join(got, "\x00") != strings.Join(want, "\x00") || len(got) != len(want) {
			t.Errorf("Texts(%q) = %q; want %q", in, got, want)
		}
	}

	// An unescaped comma in a single TEXT is outside the grammar (TSAFE-CHAR
	// excludes ","); keeping it is the only reading.
	if got := cvLine(t, "SUMMARY:Dinner, drinks").Text(); got != "Dinner, drinks" {
		t.Errorf("Text(%q) = %q", "Dinner, drinks", got)
	} else {
		t.Logf("LENIENT TEXT %q -> %q", "Dinner, drinks", got)
	}

	// A backslash before any other character isn't an ESCAPED-CHAR, and
	// the RFC doesn't say how to read one. It mustn't panic; what it gives
	// is reported.
	for _, in := range []string{`\:`, `\t`, `a\`, `\`, `\x`, `\"`, `a\,b\`, `\é`} {
		t.Logf("LENIENT TEXT undefined escape %q -> Text %q, Texts %q",
			in, (&ical.Property{Name: "SUMMARY", Value: in}).Text(),
			(&ical.Property{Name: "CATEGORIES", Value: in}).Texts())
	}
}

func TestConformanceValueEscapeText(t *testing.T) {
	for in, want := range map[string]string{
		// RFC 5545 §3.3.11: backslash, comma and semicolon "MUST be escaped
		// with a BACKSLASH character".
		`\`: `\\`,
		";": `\;`,
		",": `\,`,
		// "An intentional formatted text line break MUST only be included
		// ... by representing the line break with the character sequence of
		// BACKSLASH, followed by a LATIN SMALL LETTER N or a LATIN CAPITAL
		// LETTER N".
		"a\nb": `a\nb`,
		// "a COLON character ... SHALL NOT be escaped".
		":": ":",
		// DQUOTE, WSP and non-ASCII are text as they stand.
		`"`:    `"`,
		"a\tb": "a\tb",
		"é😀":   "é😀",
		"Project XYZ Final Review\nConference Room - 3B\nCome Prepared.": `Project XYZ Final Review\nConference Room - 3B\nCome Prepared.`,
	} {
		got := ical.EscapeText(in)
		if got != want && got != strings.ReplaceAll(want, `\n`, `\N`) {
			t.Errorf("EscapeText(%q) = %q; want %q", in, got, want)
		}
	}

	// Every string over the characters that matter, up to length 5: the
	// escaped form is valid TEXT, and it reads back as the original (with
	// line breaks and CONTROLs as cvTextNormal says).
	alphabet := []string{"a", `\`, ";", ",", "\n", "\r", ":", `"`, "\t", "N", "n", "\x00", "\x7f", "é"}
	var gen func(prefix string, n int)
	gen = func(prefix string, n int) {
		esc := ical.EscapeText(prefix)
		if !cvValidText(esc, false) {
			t.Errorf("EscapeText(%q) = %q, which isn't §3.3.11 text", prefix, esc)
		}
		if got, want := ical.UnescapeText(esc), cvTextNormal(prefix); got != want {
			t.Errorf("UnescapeText(EscapeText(%q)) = %q; want %q", prefix, got, want)
		}
		if n == 0 {
			return
		}
		for _, a := range alphabet {
			gen(prefix+a, n-1)
		}
	}
	gen("", 5)
}

func TestConformanceValueSetText(t *testing.T) {
	long := strings.Repeat("x", 73) + `\;,` + strings.Repeat("é", 40) + "\n:" + strings.Repeat(`\`, 80)
	for _, s := range []string{
		"", "plain", `a\b;c,d:e"f`, "line1\nline2\r\nline3\rline4", "\\n", "\\\n", ",", ";", `\`,
		"trailing\\", "tab\there", "Café ☕", long,
	} {
		p := ical.NewProperty("DESCRIPTION", "")
		p.SetText(s)
		if !cvValidText(p.Value, false) {
			t.Errorf("SetText(%q) wrote %q, which isn't §3.3.11 text", s, p.Value)
		}
		want := cvTextNormal(s)
		if got := p.Text(); got != want {
			t.Errorf("SetText(%q) then Text = %q; want %q", s, got, want)
		}
		// Through the encoder, folded, and back.
		if got := cvRoundTrip(t, p).Text(); got != want {
			t.Errorf("SetText(%q) round trip = %q; want %q", s, got, want)
		}
	}

	for _, items := range [][]string{
		{"APPOINTMENT", "EDUCATION"},
		{"a,b", "c"},
		{`a\`, "b"},
		{`a\,b`},
		{"a;b", "c\nd"},
		{"a", "", "b"},
		{"", ""},
		{"x:y", `"q"`},
		{long, long},
	} {
		p := ical.NewProperty("CATEGORIES", "")
		p.SetTexts(items...)
		if !cvValidText(p.Value, true) {
			t.Errorf("SetTexts(%q) wrote %q, which isn't a list of §3.3.11 text", items, p.Value)
		}
		for _, got := range [][]string{p.Texts(), cvRoundTrip(t, p).Texts()} {
			if strings.Join(got, "\x00") != strings.Join(items, "\x00") || len(got) != len(items) {
				t.Errorf("SetTexts(%q) then Texts = %q", items, got)
			}
		}
	}
}
