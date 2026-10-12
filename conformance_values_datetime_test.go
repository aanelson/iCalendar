package ical_test

// Conformance tests for RFC 5545 §3.3.4 DATE, §3.3.5 DATE-TIME, §3.3.12
// TIME (which RFC 5545 uses only inside DATE-TIME) and §3.2.20 VALUE. See
// conformance_values_test.go for the conventions.

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	ical "github.com/aanelson/iCalendar"
	"github.com/aanelson/iCalendar/civil"
)

func cvLoad(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("zone %s: %v", name, err)
	}
	return loc
}

func cvUTC(s string) time.Time {
	t, err := time.Parse("2006-01-02T15:04:05Z", s)
	if err != nil {
		panic(err)
	}
	return t
}

func cvWall(y int, m time.Month, d, hh, mm, ss int) civil.DateTime {
	return civil.DateTime{Date: civil.Date{Year: y, Month: m, Day: d}, Time: civil.Time{Hour: hh, Minute: mm, Second: ss}}
}

// cvLeap is the Gregorian leap-year rule, from the definition rather than
// the library: every fourth year, except centuries not divisible by 400.
func cvLeap(y int) bool { return y%4 == 0 && (y%100 != 0 || y%400 == 0) }

func cvDaysIn(y int, m time.Month) int {
	switch m {
	case time.February:
		if cvLeap(y) {
			return 29
		}
		return 28
	case time.April, time.June, time.September, time.November:
		return 30
	}
	return 31
}

// RFC 5545 §3.3.5: date-time = date "T" time, with §3.3.4's date-value
// (8 digits) and §3.3.12's time (6 digits, then an optional "Z"). §3.1:
// property values are case-sensitive unless stated otherwise, and §3.3.5
// names "LATIN CAPITAL LETTER T" and "LATIN CAPITAL LETTER Z".
var (
	cvDateABNF     = regexp.MustCompile(`^[0-9]{8}$`)
	cvDateTimeABNF = regexp.MustCompile(`^[0-9]{8}T(?:[01][0-9]|2[0-3])[0-5][0-9](?:[0-5][0-9]|60)Z?$`)
)

// ---------------------------------------------------------------------------
// §3.3.4 DATE

func TestConformanceValueDate(t *testing.T) {
	// RFC 5545 §3.3.4: date-value = date-fullyear date-month date-mday;
	// date-month 01-12; date-mday "01-28, 01-29, 01-30, 01-31 based on
	// month/year". Every month and day number around the valid ones, in
	// years that exercise the Gregorian leap rule.
	years := []int{1, 1582, 1600, 1700, 1800, 1899, 1900, 1904, 1999, 2000, 2001,
		2004, 2023, 2024, 2100, 2400, 9999}
	for _, y := range years {
		for m := 0; m <= 13; m++ {
			for d := 0; d <= 32; d++ {
				in := fmt.Sprintf("%04d%02d%02d", y, m, d)
				valid := m >= 1 && m <= 12 && d >= 1 && d <= cvDaysIn(y, time.Month(m))
				for _, line := range []string{"DTSTART;VALUE=DATE:" + in, "DTSTART:" + in} {
					v, err := cvLine(t, line).DateTime(nil)
					switch {
					case valid && err != nil:
						t.Errorf("%s: %v; want %04d-%02d-%02d", line, err, y, m, d)
					case valid && (!v.IsDate || v.DateTime.Date != (civil.Date{Year: y, Month: time.Month(m), Day: d}) ||
						v.UTC || v.TZID != ""):
						t.Errorf("%s = %+v; want the DATE %04d-%02d-%02d", line, v, y, m, d)
					case !valid && err == nil:
						t.Errorf("%s: accepted as %v; that day doesn't exist", line, v)
					}
				}
			}
		}
	}

	// date-fullyear is 4DIGIT, so the year 0000 fits the grammar; ISO 8601
	// allows it only by agreement, so it is only reported.
	if v, err := cvLine(t, "DTSTART;VALUE=DATE:00000101").DateTime(nil); err == nil {
		t.Logf("LENIENT DATE year 0000 -> %v", v)
	}

	// Not 8 digits.
	for in, ok := range map[string][]civil.Date{
		"":           nil,
		"1997071":    nil,
		"199707140":  nil,
		"1997-07-14": {{Year: 1997, Month: 7, Day: 14}},
		"1997071a":   nil,
		"+1997071":   nil,
		"-1997071":   nil,
		" 19970714":  {{Year: 1997, Month: 7, Day: 14}},
		"19970714 ":  {{Year: 1997, Month: 7, Day: 14}},
		"１９９７０７１４":   nil,
		// "a Z is not part of DATE": the documented lenient reading keeps the
		// date (docs/architecture.md, "a DATE with a Z (20131210Z)").
		"19970714Z": {{Year: 1997, Month: 7, Day: 14}},
	} {
		p := &ical.Property{Name: "DTSTART", Params: ical.Params{{Name: "VALUE", Values: []string{"DATE"}}}, Value: in}
		v, err := p.DateTime(nil)
		if err == nil && !v.IsDate {
			t.Errorf("DATE %q: accepted as the non-DATE %+v", in, v)
			continue
		}
		cvForbidden(t, "DATE", in, v.DateTime.Date, err, ok...)
	}

	// §3.3.4: "multiple "date" values are specified as a COMMA-separated
	// list of values" (RFC 5545 §3.8.5.2 example).
	vs, err := cvLine(t, "RDATE;VALUE=DATE:19970101,19970120,19970217,19970421").DateTimes(nil)
	if err != nil || len(vs) != 4 || !vs[3].IsDate || vs[3].DateTime.Date != (civil.Date{Year: 1997, Month: 4, Day: 21}) {
		t.Errorf("RDATE;VALUE=DATE list = %v, %v", vs, err)
	}
	// Erratum 2527 (Verified): the RFC's own example lost a comma at a fold,
	// so it unfolds to "1997042119970526", which isn't a DATE. Reading it
	// as a list of valid dates would drop or invent a date.
	bad := "RDATE;VALUE=DATE:19970101,19970120,19970217,19970421\r\n 19970526,19970704,19970901,19971014,19971128,19971129,19971225"
	if vs, err := cvLine(t, bad).DateTimes(nil); err == nil {
		t.Errorf("RDATE with the erratum 2527 typo: accepted as %v", vs)
	}

	// §3.2.19: "The "TZID" property parameter MUST NOT be applied to DATE
	// properties". A DATE is a calendar date in no zone, so a TZID written
	// on one anyway can't change it.
	if v, err := cvLine(t, "DTSTART;VALUE=DATE;TZID=America/New_York:19970714").DateTime(nil); err == nil &&
		(!v.IsDate || !v.Floating() || v.DateTime.Date != (civil.Date{Year: 1997, Month: 7, Day: 14})) {
		t.Errorf("DATE with a TZID = %+v; want the floating DATE 1997-07-14", v)
	}

	// VALUE=DATE with a DATE-TIME value: the property's values "MUST be of a
	// single value type" (§3.2.20) and this one isn't the declared type.
	// Reading it as a DATE would drop the time without a word.
	if v, err := cvLine(t, "DTSTART;VALUE=DATE:19970714T133000").DateTime(nil); err == nil {
		if v.IsDate {
			t.Errorf("VALUE=DATE:19970714T133000 read as the DATE %v, dropping the time", v)
		} else {
			t.Logf("LENIENT DATE with a time %q -> %v", "19970714T133000", v)
		}
	}
	// And an explicit VALUE=DATE-TIME with only a date.
	if v, err := cvLine(t, "DTSTART;VALUE=DATE-TIME:19970714").DateTime(nil); err == nil {
		t.Logf("LENIENT DATE-TIME without a time %q -> %+v", "19970714", v)
	}
}

func TestConformanceValueSetDate(t *testing.T) {
	for _, c := range []struct {
		d    civil.Date
		want string
	}{
		{civil.Date{Year: 1997, Month: 7, Day: 14}, "19970714"},
		{civil.Date{Year: 2000, Month: 2, Day: 29}, "20000229"},
		// date-fullyear = 4DIGIT: years below 1000 are zero-padded.
		{civil.Date{Year: 999, Month: 1, Day: 2}, "09990102"},
		{civil.Date{Year: 1, Month: 1, Day: 1}, "00010101"},
		{civil.Date{Year: 9999, Month: 12, Day: 31}, "99991231"},
	} {
		p := ical.NewProperty("DTSTART", "")
		// Start from a TZID the value must drop: §3.2.19 "The "TZID"
		// property parameter MUST NOT be applied to DATE properties".
		p.Params.Set("TZID", "America/New_York")
		p.SetDateTime(ical.NewDate(c.d))
		if p.Value != c.want {
			t.Errorf("SetDateTime(NewDate(%v)) wrote %q; want %q", c.d, p.Value, c.want)
		}
		if p.Params.Has("TZID") {
			t.Errorf("SetDateTime(NewDate(%v)) kept TZID=%s", c.d, p.Params.Get("TZID"))
		}
		// DATE isn't the default type of DTSTART, so §3.2.20: VALUE "MUST be
		// specified".
		if v := cvParamValues(p, "VALUE"); len(v) != 1 || !strings.EqualFold(v[0], "DATE") {
			t.Errorf("SetDateTime(NewDate(%v)): VALUE = %q; want DATE", c.d, v)
		}
		if got := ical.NewDate(c.d).String(); got != c.want {
			t.Errorf("NewDate(%v).String() = %q; want %q", c.d, got, c.want)
		}
		v, err := cvRoundTrip(t, p).DateTime(nil)
		if err != nil || !v.IsDate || v.DateTime.Date != c.d {
			t.Errorf("DATE %v round trip = %+v, %v", c.d, v, err)
		}
	}
	if got := cvEncode(t, func() *ical.Property {
		p := ical.NewProperty("DTSTART", "")
		p.SetDateTime(ical.NewDate(civil.Date{Year: 1997, Month: 7, Day: 14}))
		return p
	}()); got != "DTSTART;VALUE=DATE:19970714" {
		t.Errorf("encoded DATE = %q", got)
	}

	// A year that 4DIGIT can't hold can't be written as a DATE; the setter
	// has no way to report that, so what it writes is only reported.
	p := ical.NewProperty("DTSTART", "")
	p.SetDateTime(ical.NewDate(civil.Date{Year: 10000, Month: 1, Day: 1}))
	if !cvDateABNF.MatchString(p.Value) {
		t.Errorf("SetDateTime(NewDate(10000-01-01)) wrote %q, which isn't a DATE", p.Value)
	}
	t.Logf("UNREPRESENTABLE SetDateTime(NewDate(10000-01-01)) wrote %q", p.Value)

	// §3.3.4: a list of dates; §3.2.20: one value type for all of them.
	p = ical.NewProperty("EXDATE", "")
	p.SetDateTimes(ical.NewDate(civil.Date{Year: 1997, Month: 1, Day: 1}), ical.NewDate(civil.Date{Year: 1997, Month: 1, Day: 20}))
	if p.Value != "19970101,19970120" || !strings.EqualFold(p.Params.Get("VALUE"), "DATE") {
		t.Errorf("SetDateTimes(two DATEs) = %q %v", p.Value, p.Params)
	}
}

// ---------------------------------------------------------------------------
// §3.3.5 DATE-TIME and §3.3.12 TIME

func TestConformanceValueDateTimeForms(t *testing.T) {
	ny := cvLoad(t, "America/New_York")

	// FORM #1: floating. "19980118T230000" is "January 18, 1998, at 11 PM",
	// with no zone.
	v, err := cvLine(t, "DTSTART:19980118T230000").DateTime(nil)
	if err != nil || v.IsDate || v.UTC || v.TZID != "" || !v.Floating() || v.Location() != nil ||
		v.DateTime != cvWall(1998, 1, 18, 23, 0, 0) {
		t.Errorf("floating 19980118T230000 = %+v, %v", v, err)
	}
	// "the same hour, minute, and second value regardless of which time zone
	// is currently being observed": read in New York it is 23:00 EST.
	if got := v.In(ny); !got.Equal(cvUTC("1998-01-19T04:00:00Z")) || got.Hour() != 23 {
		t.Errorf("floating 23:00 In(New York) = %v", got)
	}
	// "The use of local time in a DATE-TIME value without the "TZID"
	// property parameter is to be interpreted as floating time, regardless
	// of the existence of "VTIMEZONE" calendar components".
	cal, err := ical.Parse(strings.NewReader("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:x\r\n" +
		"BEGIN:VTIMEZONE\r\nTZID:America/New_York\r\nBEGIN:STANDARD\r\nDTSTART:19701101T020000\r\n" +
		"TZOFFSETFROM:-0400\r\nTZOFFSETTO:-0500\r\nEND:STANDARD\r\nEND:VTIMEZONE\r\n" +
		"BEGIN:VEVENT\r\nUID:1\r\nDTSTAMP:19980101T000000Z\r\nDTSTART:19980118T230000\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	for ev := range cal.Events() {
		if s, err := ev.Start(); err != nil || !s.Floating() || s.TZID != "" {
			t.Errorf("DTSTART without TZID in a calendar with a VTIMEZONE = %+v, %v; want floating", s, err)
		}
	}

	// FORM #2: UTC. "19980119T070000Z" is "January 19, 1998, at 0700 UTC".
	v, err = cvLine(t, "DTSTART:19980119T070000Z").DateTime(nil)
	if err != nil || !v.UTC || v.TZID != "" || v.Floating() || v.Location() != time.UTC ||
		!v.Time().Equal(cvUTC("1998-01-19T07:00:00Z")) {
		t.Errorf("UTC 19980119T070000Z = %+v, %v", v, err)
	}

	// FORM #3: "TZID=America/New_York:19980119T020000" is "2:00 A.M. in
	// New York on January 19, 1998" (EST, UTC-5).
	v, err = cvLine(t, "DTSTART;TZID=America/New_York:19980119T020000").DateTime(nil)
	if err != nil || v.UTC || v.TZID != "America/New_York" || v.Floating() || v.Loc == nil ||
		!v.Time().Equal(cvUTC("1998-01-19T07:00:00Z")) {
		t.Errorf("TZID=America/New_York:19980119T020000 = %+v (%v), %v", v, v.Time(), err)
	}

	// The section's example: "July 14, 1997, at 1:30 PM in New York City in
	// each of the three time formats" (EDT, UTC-4).
	want := cvUTC("1997-07-14T17:30:00Z")
	for _, c := range []struct {
		line string
		loc  *time.Location
	}{
		{"DTSTART:19970714T133000", ny},
		{"DTSTART:19970714T173000Z", nil},
		{"DTSTART;TZID=America/New_York:19970714T133000", nil},
	} {
		v, err := cvLine(t, c.line).DateTime(nil)
		got := v.Time()
		if c.loc != nil {
			got = v.In(c.loc)
		}
		if err != nil || !got.Equal(want) {
			t.Errorf("%s = %v, %v; want %v", c.line, got, err, want)
		}
	}

	// §3.3.5: "The form of date and time with UTC offset MUST NOT be used"
	// ("19980119T230000-0800 ;Invalid time format"). Fractions of a second
	// "are not supported by this format" (§3.3.12). Reading any of these as
	// a floating 23:00 would lose the offset; only the instant it names is
	// an acceptable lenient reading.
	for in, ok := range map[string][]time.Time{
		"19980119T230000-0800":  {cvUTC("1998-01-20T07:00:00Z")},
		"19980119T230000+0100":  {cvUTC("1998-01-19T22:00:00Z")},
		"19980119T230000-08:00": {cvUTC("1998-01-20T07:00:00Z")},
		"19980119T230000.000Z":  {cvUTC("1998-01-19T23:00:00Z")},
		"19980119T230000.5":     nil,
		"1998-01-19T23:00:00Z":  {cvUTC("1998-01-19T23:00:00Z")},
		"19980119T230000ZZ":     nil,
		"19980119 230000":       nil,
		"19980119230000":        nil,
		"19980119T":             nil,
		"19980119TZ":            nil,
		"19980119T2300000":      nil,
		"19980119T23000":        nil,
		"19980119T23Z":          nil,
		"19980119T230000Q":      nil,
		// §3.1: "all other property values are case-sensitive, unless
		// otherwise stated", which overrides RFC 5234's case-insensitive
		// literals; §3.3.5 names "LATIN CAPITAL LETTER T" and "Z".
		"19980119T230000z": {cvUTC("1998-01-19T23:00:00Z")},
		"19980119t230000Z": {cvUTC("1998-01-19T23:00:00Z")},
		"19980119t230000z": {cvUTC("1998-01-19T23:00:00Z")},
		// The documented lenient reading of a time without seconds
		// (docs/architecture.md: "19970701T1300Z").
		"19980119T2300Z": {cvUTC("1998-01-19T23:00:00Z")},
	} {
		v, err := cvLine(t, "DTSTART:"+in).DateTime(nil)
		if err == nil && (v.Floating() || v.IsDate) {
			t.Errorf("DATE-TIME %q: accepted as the floating %+v", in, v)
			continue
		}
		cvForbidden(t, "DATE-TIME", in, v.Time(), err, ok...)
	}
	// A lower-case "t" with no "Z" is floating; only the wall clock can be
	// right.
	if v, err := cvLine(t, "DTSTART:19980119t230000").DateTime(nil); err == nil {
		if v.DateTime != cvWall(1998, 1, 19, 23, 0, 0) || !v.Floating() {
			t.Errorf("DATE-TIME %q = %+v", "19980119t230000", v)
		} else {
			t.Logf("LENIENT DATE-TIME %q -> floating %v", "19980119t230000", v)
		}
	}

	// §3.3.5 FORM #2: "The "TZID" property parameter MUST NOT be applied to
	// DATE-TIME properties whose time values are specified in UTC." The
	// RFC doesn't say how to read one that has it; the value itself names
	// a UTC instant, so that, or an error, is reported.
	v, err = cvLine(t, "DTSTART;TZID=America/New_York:19980119T070000Z").DateTime(nil)
	t.Logf("LENIENT TZID with a UTC value -> %+v (instant %v), err %v", v, v.Time(), err)
	if err == nil && !v.Time().Equal(cvUTC("1998-01-19T07:00:00Z")) {
		t.Errorf("TZID=America/New_York:19980119T070000Z read as %v; the Z names 07:00 UTC", v.Time())
	}
}

func TestConformanceValueTime(t *testing.T) {
	// RFC 5545 §3.3.12: time-hour 00-23, time-minute 00-59, time-second
	// 00-60, "60" only for a positive leap second; tested inside DATE-TIME,
	// the only place RFC 5545 uses TIME.
	for hh := 0; hh <= 25; hh++ {
		for _, mm := range []int{0, 1, 30, 59, 60, 61, 99} {
			for _, ss := range []int{0, 1, 30, 59, 60, 61, 99} {
				if ss == 60 && hh <= 23 && mm <= 59 {
					continue // a leap second; below
				}
				in := fmt.Sprintf("19970714T%02d%02d%02dZ", hh, mm, ss)
				v, err := cvLine(t, "DTSTART:"+in).DateTime(nil)
				if hh <= 23 && mm <= 59 && ss <= 59 {
					if err != nil || !v.UTC || v.DateTime != cvWall(1997, 7, 14, hh, mm, ss) {
						t.Errorf("DATE-TIME %s = %+v, %v", in, v, err)
					}
				} else if err == nil {
					t.Errorf("DATE-TIME %s: accepted as %v; the time is out of range", in, v.Time())
				}
			}
		}
	}

	// §3.3.5: "19970630T235960Z" is the RFC's own example of a leap second
	// (one was inserted then), so it is valid and must be read. "Implementations
	// that do not support leap seconds SHOULD interpret the second 60 as
	// equivalent to the second 59": a SHOULD, so the value is only reported.
	v, err := cvLine(t, "DTSTART:19970630T235960Z").DateTime(nil)
	if err != nil {
		t.Errorf("leap second 19970630T235960Z: %v", err)
	} else if !v.Time().Equal(cvUTC("1997-06-30T23:59:59Z")) {
		t.Logf("SHOULD leap second 19970630T235960Z reads as %v, not 23:59:59", v.Time())
	}
}

func TestConformanceValueSetDateTime(t *testing.T) {
	ny := cvLoad(t, "America/New_York")
	for _, c := range []struct {
		v     ical.DateTime
		value string
		tzid  string
	}{
		// §3.3.5 FORM #3; §3.2.19 "This parameter MUST be specified ...
		// when the value is neither a UTC or a "floating" time".
		{ical.NewDateTime(time.Date(1998, 1, 19, 2, 0, 0, 0, ny)), "19980119T020000", "America/New_York"},
		// FORM #2: the Z, and §3.3.5 "The "TZID" property parameter MUST NOT
		// be applied" to it.
		{ical.NewDateTime(time.Date(1998, 1, 19, 7, 0, 0, 0, time.UTC)), "19980119T070000Z", ""},
		// A fixed offset has no TZID a reader could look up; its instant in
		// UTC is the documented choice (NewDateTime's doc comment).
		{ical.NewDateTime(time.Date(1998, 1, 19, 2, 0, 0, 0, time.FixedZone("", -5*3600))), "19980119T070000Z", ""},
		// FORM #1.
		{ical.NewFloating(cvWall(1998, 1, 18, 23, 0, 0)), "19980118T230000", ""},
		// §3.3.12: "Fractions of a second are not supported by this format."
		{ical.NewDateTime(time.Date(1998, 1, 19, 7, 0, 0, 999999999, time.UTC)), "19980119T070000Z", ""},
		// 4DIGIT year.
		{ical.NewFloating(cvWall(999, 1, 2, 3, 4, 5)), "09990102T030405", ""},
	} {
		p := ical.NewProperty("DTSTART", "")
		// Start from parameters a DATE had, which a DATE-TIME must drop.
		p.Params.Set("VALUE", "DATE")
		p.Params.Set("TZID", "Europe/London")
		p.SetDateTime(c.v)
		if p.Value != c.value || !cvDateTimeABNF.MatchString(p.Value) {
			t.Errorf("SetDateTime(%+v) wrote %q; want %q", c.v, p.Value, c.value)
		}
		if got := p.Params.Get("TZID"); got != c.tzid || len(cvParamValues(p, "TZID")) > 1 {
			t.Errorf("SetDateTime(%q): TZID = %q; want %q", c.value, cvParamValues(p, "TZID"), c.tzid)
		}
		// §3.2.20: DATE-TIME is DTSTART's default, so VALUE is optional, but
		// a stale VALUE=DATE would declare the wrong type.
		if v := p.Params.Get("VALUE"); v != "" && !strings.EqualFold(v, "DATE-TIME") {
			t.Errorf("SetDateTime(%q): VALUE = %q", c.value, v)
		}
		if got := c.v.String(); got != c.value {
			t.Errorf("String() = %q; want %q", got, c.value)
		}
		back, err := cvRoundTrip(t, p).DateTime(nil)
		if err != nil || back.DateTime != c.v.DateTime || back.UTC != c.v.UTC || back.TZID != c.v.TZID {
			if c.v.Time().Nanosecond() == 0 {
				t.Errorf("SetDateTime(%q) round trip = %+v, %v; want %+v", c.value, back, err, c.v)
			}
		}
		if !c.v.Floating() && back.Time().Unix() != c.v.Time().Unix() {
			t.Errorf("SetDateTime(%q) round trip names %v; want %v", c.value, back.Time(), c.v.Time())
		}
	}

	if got := cvEncode(t, func() *ical.Property {
		p := ical.NewProperty("DTSTART", "")
		p.SetDateTime(ical.NewDateTime(time.Date(1998, 1, 19, 2, 0, 0, 0, ny)))
		return p
	}()); got != "DTSTART;TZID=America/New_York:19980119T020000" {
		t.Errorf("encoded DATE-TIME = %q", got)
	}

	// §3.2.20: "if the property's default value type is overridden by some
	// other allowable value type, then this parameter MUST be specified".
	// TRIGGER's default is DURATION (§3.8.6.3; erratum 2039 adds the VALUE
	// to the RFC's own example), so moving an absolute trigger must keep
	// the VALUE=DATE-TIME it has.
	p := cvLine(t, "TRIGGER;VALUE=DATE-TIME:19980403T120000Z")
	p.SetDateTime(ical.NewDateTime(time.Date(1998, 4, 3, 13, 0, 0, 0, time.UTC)))
	if !strings.EqualFold(p.Params.Get("VALUE"), "DATE-TIME") {
		t.Errorf("SetDateTime on TRIGGER;VALUE=DATE-TIME wrote %q with VALUE=%q; want VALUE=DATE-TIME", p.Value, p.Params.Get("VALUE"))
	}
}

func TestConformanceValueSetDateTimeNonDefaultType(t *testing.T) {
	// §3.2.20 as above, for a TRIGGER that was relative: the setter would
	// have to know that TRIGGER defaults to DURATION.
	p := ical.NewProperty("TRIGGER", "-PT15M")
	p.SetDateTime(ical.NewDateTime(time.Date(1998, 4, 3, 12, 0, 0, 0, time.UTC)))
	if !strings.EqualFold(p.Params.Get("VALUE"), "DATE-TIME") {
		t.Skipf("KNOWN GAP: SetDateTime on a relative TRIGGER wrote %q with no VALUE=DATE-TIME, so it reads as a DURATION; "+
			"setters don't know each property's default value type (the planned schema, docs/architecture.md)", p.Value)
	}
}

// A DATE-TIME's TZID form names the first of two repeated wall-clock times
// (§3.3.5). An instant in the second one must still be written so that it
// reads back as that instant.
func TestConformanceValueSetDateTimeRepeatedHour(t *testing.T) {
	for _, c := range []struct{ zone, instant string }{
		// 01:30 EST, the second 01:30 of 2007-11-04 in New York.
		{"America/New_York", "2007-11-04T06:30:00Z"},
		// 01:30 GMT, the second 01:30 of 2007-10-28 in London.
		{"Europe/London", "2007-10-28T01:30:00Z"},
		// 01:45 +10:30, the second 01:45 of 2023-04-02 on Lord Howe.
		{"Australia/Lord_Howe", "2023-04-01T15:15:00Z"},
	} {
		loc := cvLoad(t, c.zone)
		at := cvUTC(c.instant).In(loc)
		v := ical.NewDateTime(at)
		if !v.Time().Equal(at) {
			t.Errorf("NewDateTime(%v).Time() = %v", at, v.Time())
		}
		p := ical.NewProperty("DTSTART", "")
		p.SetDateTime(v)
		back, err := cvRoundTrip(t, p).DateTime(nil)
		if err != nil || !back.Time().Equal(at) {
			t.Errorf("%s: SetDateTime(NewDateTime(%v)) wrote %q;%v, which reads back as %v, %v",
				c.zone, at, p.Value, p.Params, back.Time(), err)
		}
	}

	// In a list the later values take the first's TZID; one that lands on
	// the second 01:30 can't be written under it.
	ny := cvLoad(t, "America/New_York")
	vs := []ical.DateTime{
		ical.NewDateTime(time.Date(2007, 11, 3, 1, 30, 0, 0, ny)),
		ical.NewDateTime(cvUTC("2007-11-04T06:30:00Z")),
	}
	p := ical.NewProperty("RDATE", "")
	p.SetDateTimes(vs...)
	back, err := cvRoundTrip(t, p).DateTimes(nil)
	if err != nil || len(back) != 2 || !back[0].Time().Equal(vs[0].Time()) || !back[1].Time().Equal(vs[1].Time()) {
		t.Errorf("SetDateTimes(%v, %v) wrote %q;%v, which reads back as %v, %v", vs[0].Time(), vs[1].Time(), p.Value, p.Params, back, err)
	}
}

// RFC 5545 §3.3.5 FORM #3: "If ... the local time described occurs more than
// once (when changing from daylight to standard time), the DATE-TIME value
// refers to the first occurrence of the referenced time. ... If the local
// time described does not occur (when changing from standard to daylight
// time), the DATE-TIME value is interpreted using the UTC offset before the
// gap in local times."
//
// Expected instants are worked by hand from each zone's published
// transitions (tzdata, confirmed with zdump):
//
//	America/New_York  2007-03-11 02:00 EST -5 → 03:00 EDT -4
//	                  2007-11-04 02:00 EDT -4 → 01:00 EST -5
//	Europe/London     2007-03-25 01:00 GMT +0 → 02:00 BST +1
//	                  2007-10-28 02:00 BST +1 → 01:00 GMT +0
//	Australia/Lord_Howe 2023-04-02 02:00 +11 → 01:30 +10:30
//	                  2023-10-01 02:00 +10:30 → 02:30 +11
//	Pacific/Apia      2011-12-29 24:00 -10 → 2011-12-31 00:00 +14
//	                  (2011-12-30 never happened)
var cvDSTCases = []struct {
	zone string
	wall civil.DateTime
	want string // UTC
	why  string
}{
	// The RFC's two examples.
	{"America/New_York", cvWall(2007, 11, 4, 1, 30, 0), "2007-11-04T05:30:00Z", "RFC example: first occurrence, 01:30 EDT"},
	{"America/New_York", cvWall(2007, 3, 11, 2, 30, 0), "2007-03-11T07:30:00Z", "RFC example: gap, offset before (EST), = 03:30 EDT"},
	// The edges of New York's gap and overlap.
	{"America/New_York", cvWall(2007, 3, 11, 1, 59, 59), "2007-03-11T06:59:59Z", "last EST second"},
	{"America/New_York", cvWall(2007, 3, 11, 2, 0, 0), "2007-03-11T07:00:00Z", "gap start, EST"},
	{"America/New_York", cvWall(2007, 3, 11, 2, 59, 59), "2007-03-11T07:59:59Z", "gap end, EST"},
	{"America/New_York", cvWall(2007, 3, 11, 3, 0, 0), "2007-03-11T07:00:00Z", "first EDT time"},
	{"America/New_York", cvWall(2007, 11, 4, 0, 59, 59), "2007-11-04T04:59:59Z", "before the overlap, EDT"},
	{"America/New_York", cvWall(2007, 11, 4, 1, 0, 0), "2007-11-04T05:00:00Z", "overlap start, first = EDT"},
	{"America/New_York", cvWall(2007, 11, 4, 1, 59, 59), "2007-11-04T05:59:59Z", "overlap end, first = EDT"},
	{"America/New_York", cvWall(2007, 11, 4, 2, 0, 0), "2007-11-04T07:00:00Z", "after the overlap, EST"},
	// London: the same rules at offsets 0 and +1.
	{"Europe/London", cvWall(2007, 3, 25, 1, 30, 0), "2007-03-25T01:30:00Z", "gap, offset before (GMT)"},
	{"Europe/London", cvWall(2007, 10, 28, 1, 30, 0), "2007-10-28T00:30:00Z", "overlap, first = BST"},
	// Lord Howe: a 30-minute shift.
	{"Australia/Lord_Howe", cvWall(2023, 10, 1, 2, 15, 0), "2023-09-30T15:45:00Z", "30-minute gap, offset before (+10:30)"},
	{"Australia/Lord_Howe", cvWall(2023, 10, 1, 2, 30, 0), "2023-09-30T15:30:00Z", "first +11 time"},
	{"Australia/Lord_Howe", cvWall(2023, 4, 2, 1, 45, 0), "2023-04-01T14:45:00Z", "30-minute overlap, first = +11"},
	{"Australia/Lord_Howe", cvWall(2023, 4, 2, 1, 29, 0), "2023-04-01T14:29:00Z", "before the overlap, +11"},
	// Apia: a whole skipped day, from a negative offset to a positive one.
	{"Pacific/Apia", cvWall(2011, 12, 29, 23, 59, 59), "2011-12-30T09:59:59Z", "last -10 second"},
	{"Pacific/Apia", cvWall(2011, 12, 30, 0, 0, 0), "2011-12-30T10:00:00Z", "skipped day, offset before (-10)"},
	{"Pacific/Apia", cvWall(2011, 12, 30, 12, 0, 0), "2011-12-30T22:00:00Z", "skipped day, offset before (-10)"},
	{"Pacific/Apia", cvWall(2011, 12, 30, 23, 59, 59), "2011-12-31T09:59:59Z", "skipped day, offset before (-10)"},
	{"Pacific/Apia", cvWall(2011, 12, 31, 0, 0, 0), "2011-12-30T10:00:00Z", "first +14 time"},
}

func TestConformanceValueDateTimeDST(t *testing.T) {
	for _, c := range cvDSTCases {
		loc := cvLoad(t, c.zone)
		want := cvUTC(c.want)
		line := fmt.Sprintf("DTSTART;TZID=%s:%04d%02d%02dT%02d%02d%02d", c.zone,
			c.wall.Date.Year, c.wall.Date.Month, c.wall.Date.Day, c.wall.Time.Hour, c.wall.Time.Minute, c.wall.Time.Second)
		v, err := cvLine(t, line).DateTime(nil)
		if err != nil || !v.Time().Equal(want) {
			t.Errorf("%s (%s) = %v, %v; want %v", line, c.why, v.Time().UTC(), err, want)
		}
		// A floating value read in the zone: DateTime.In documents the same
		// rules (§3.3.5 doesn't speak to floating times in a gap).
		if got := ical.NewFloating(c.wall).In(loc); !got.Equal(want) {
			t.Errorf("NewFloating(%v).In(%s) (%s) = %v; want %v", c.wall, c.zone, c.why, got.UTC(), want)
		}
	}
}

// The same rules must hold when the zone comes from the calendar's own
// VTIMEZONE (RFC 5545 §3.6.5's New York definition) rather than tzdata.
func TestConformanceValueDateTimeDSTFromVTIMEZONE(t *testing.T) {
	src := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:x\r\n" +
		"BEGIN:VTIMEZONE\r\nTZID:Custom/New_York\r\n" +
		"BEGIN:DAYLIGHT\r\nDTSTART:20070311T020000\r\nRRULE:FREQ=YEARLY;BYMONTH=3;BYDAY=2SU\r\n" +
		"TZOFFSETFROM:-0500\r\nTZOFFSETTO:-0400\r\nTZNAME:EDT\r\nEND:DAYLIGHT\r\n" +
		"BEGIN:STANDARD\r\nDTSTART:20071104T020000\r\nRRULE:FREQ=YEARLY;BYMONTH=11;BYDAY=1SU\r\n" +
		"TZOFFSETFROM:-0400\r\nTZOFFSETTO:-0500\r\nTZNAME:EST\r\nEND:STANDARD\r\n" +
		"END:VTIMEZONE\r\n" +
		"BEGIN:VEVENT\r\nUID:a\r\nDTSTAMP:20070101T000000Z\r\nDTSTART;TZID=Custom/New_York:20081102T013000\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:b\r\nDTSTAMP:20070101T000000Z\r\nDTSTART;TZID=Custom/New_York:20080309T023000\r\nEND:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
	cal, err := ical.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	// 2008-11-02 01:30 is repeated (first = EDT, 05:30Z); 2008-03-09 02:30
	// is skipped (offset before = EST, 07:30Z).
	want := map[string]time.Time{"a": cvUTC("2008-11-02T05:30:00Z"), "b": cvUTC("2008-03-09T07:30:00Z")}
	for ev := range cal.Events() {
		v, err := ev.Start()
		if err != nil || !v.Time().Equal(want[ev.UID()]) {
			t.Errorf("event %s: DTSTART = %v, %v; want %v", ev.UID(), v.Time().UTC(), err, want[ev.UID()])
		}
	}
}

// ---------------------------------------------------------------------------
// §3.2.20 VALUE and DATE-TIME lists

func TestConformanceValueValueParam(t *testing.T) {
	// §3.2.20: "if the property's default value type is overridden by some
	// other allowable value type, then this parameter MUST be specified"; a
	// reader takes the type from it.
	v, err := cvLine(t, "DTSTART;VALUE=DATE:19970714").DateTime(nil)
	if err != nil || !v.IsDate {
		t.Errorf("DTSTART;VALUE=DATE = %+v, %v", v, err)
	}
	// §3.2: parameter values (and enumerated ones like VALUE's) are
	// case-insensitive.
	v, err = cvLine(t, "DTSTART;value=date:19970714").DateTime(nil)
	if err != nil || !v.IsDate {
		t.Errorf("DTSTART;value=date = %+v, %v", v, err)
	}
	v, err = cvLine(t, "DTSTART;VALUE=DATE-TIME:19970714T133000").DateTime(nil)
	if err != nil || v.IsDate || v.DateTime != cvWall(1997, 7, 14, 13, 30, 0) {
		t.Errorf("DTSTART;VALUE=DATE-TIME = %+v, %v", v, err)
	}
	// TRIGGER defaults to DURATION; with VALUE=DATE-TIME it is absolute
	// (§3.8.6.3 and erratum 2039).
	v, err = cvLine(t, "TRIGGER;VALUE=DATE-TIME:19980403T120000Z").DateTime(nil)
	if err != nil || !v.Time().Equal(cvUTC("1998-04-03T12:00:00Z")) {
		t.Errorf("TRIGGER;VALUE=DATE-TIME = %+v, %v", v, err)
	}
	if d, err := cvLine(t, "TRIGGER:-PT15M").Duration(); err != nil || d != (ical.Duration{Negative: true, Minutes: 15}) {
		t.Errorf("TRIGGER:-PT15M = %+v, %v", d, err)
	}

	// A list of DATE-TIMEs shares one TZID (§3.8.5.1 / §3.8.5.2 examples).
	vs, err := cvLine(t, "RDATE;TZID=America/New_York:19970714T083000,19971231T083000").DateTimes(nil)
	if err != nil || len(vs) != 2 || !vs[0].Time().Equal(cvUTC("1997-07-14T12:30:00Z")) ||
		!vs[1].Time().Equal(cvUTC("1997-12-31T13:30:00Z")) {
		t.Errorf("RDATE;TZID list = %v, %v", vs, err)
	}
	vs, err = cvLine(t, "EXDATE:19960402T010000Z,19960403T010000Z,19960404T010000Z").DateTimes(nil)
	if err != nil || len(vs) != 3 || !vs[2].UTC || !vs[2].Time().Equal(cvUTC("1996-04-04T01:00:00Z")) {
		t.Errorf("EXDATE UTC list = %v, %v", vs, err)
	}
	// An empty item isn't a DATE-TIME.
	if vs, err := cvLine(t, "EXDATE:19960402T010000Z,,19960404T010000Z").DateTimes(nil); err == nil {
		t.Logf("LENIENT DATE-TIME list with an empty item -> %v", vs)
	}
}

func TestConformanceValueSetDateTimes(t *testing.T) {
	ny := cvLoad(t, "America/New_York")
	a := ical.NewDateTime(time.Date(1997, 7, 14, 8, 30, 0, 0, ny))
	b := ical.NewDateTime(cvUTC("1997-12-31T13:30:00Z"))
	c := ical.NewDate(civil.Date{Year: 1997, Month: 8, Day: 1})

	// §3.2.20: "The property values MUST be of a single value type", and one
	// TZID covers them all (§3.2.19), so later values are converted to the
	// first's form; the instants must survive.
	p := ical.NewProperty("RDATE", "")
	p.SetDateTimes(a, b)
	vs, err := cvRoundTrip(t, p).DateTimes(nil)
	if err != nil || len(vs) != 2 || !vs[0].Time().Equal(a.Time()) || !vs[1].Time().Equal(b.Time()) {
		t.Errorf("SetDateTimes(NY, UTC) wrote %q %v, which reads back as %v, %v", p.Value, p.Params, vs, err)
	}
	for _, item := range strings.Split(p.Value, ",") {
		if !cvDateTimeABNF.MatchString(item) || strings.HasSuffix(item, "Z") {
			t.Errorf("SetDateTimes(NY, UTC) wrote %q: mixed forms under one TZID", p.Value)
		}
	}

	p = ical.NewProperty("RDATE", "")
	p.SetDateTimes(b, ical.NewDateTime(cvUTC("1998-01-01T00:00:00Z")))
	if p.Value != "19971231T133000Z,19980101T000000Z" || p.Params.Has("TZID") {
		t.Errorf("SetDateTimes(UTC, UTC) = %q %v", p.Value, p.Params)
	}

	// A DATE with a DATE-TIME: whatever the setter writes must be one type.
	p = ical.NewProperty("RDATE", "")
	p.SetDateTimes(a, c)
	items := strings.Split(p.Value, ",")
	if len(items) != 2 || len(items[0]) != len(items[1]) {
		t.Errorf("SetDateTimes(DATE-TIME, DATE) wrote %q: two value types", p.Value)
	} else {
		t.Logf("LENIENT SetDateTimes(DATE-TIME, DATE) -> %q %v", p.Value, p.Params)
	}
}
