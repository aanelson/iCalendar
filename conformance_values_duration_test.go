package ical_test

// Conformance tests for RFC 5545 §3.3.6 DURATION (grammar and arithmetic)
// and §3.3.9 PERIOD. See conformance_values_test.go for the conventions.

import (
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	ical "github.com/aanelson/iCalendar"
	"github.com/aanelson/iCalendar/civil"
)

// RFC 5545 §3.3.6, transcribed rule by rule:
//
//	dur-value  = (["+"] / "-") "P" (dur-date / dur-time / dur-week)
//	dur-date   = dur-day [dur-time]
//	dur-time   = "T" (dur-hour / dur-minute / dur-second)
//	dur-week   = 1*DIGIT "W"
//	dur-hour   = 1*DIGIT "H" [dur-minute]
//	dur-minute = 1*DIGIT "M" [dur-second]
//	dur-second = 1*DIGIT "S"
//	dur-day    = 1*DIGIT "D"
//
// §3.1: property values are case-sensitive unless stated otherwise, and
// §3.3.6 doesn't state otherwise, so the designators are upper case only.
var cvDurationABNF = func() *regexp.Regexp {
	const (
		second = `[0-9]+S`
		minute = `[0-9]+M(?:` + second + `)?`
		hour   = `[0-9]+H(?:` + minute + `)?`
		tpart  = `T(?:` + hour + `|` + minute + `|` + second + `)`
		day    = `[0-9]+D`
		date   = day + `(?:` + tpart + `)?`
		week   = `[0-9]+W`
	)
	return regexp.MustCompile(`^[+-]?P(?:` + date + `|` + tpart + `|` + week + `)$`)
}()

// cvDurationLoose is the superset of the grammar a lenient reader might
// accept with one obvious reading: each part at most once, in the order
// weeks, days, T, hours, minutes, seconds, in either case.
var cvDurationLoose = regexp.MustCompile(`(?i)^([+-]?)P(?:([0-9]+)W)?(?:([0-9]+)D)?(?:(T)(?:([0-9]+)H)?(?:([0-9]+)M)?(?:([0-9]+)S)?)?$`)

// cvDur is what a duration means: its sign, its nominal days (weeks are
// seven nominal days, §3.3.6) and its exact seconds. Two durations with the
// same cvDur behave the same.
type cvDur struct {
	neg     bool
	days    int64
	seconds int64
}

func cvDurOf(d ical.Duration) cvDur {
	k := cvDur{d.Negative, int64(d.Weeks)*7 + int64(d.Days), int64(d.Hours)*3600 + int64(d.Minutes)*60 + int64(d.Seconds)}
	if k.days == 0 && k.seconds == 0 {
		k.neg = false
	}
	return k
}

// cvLooseValue reads s by cvDurationLoose, or reports false when s is outside
// it (or names no part at all, or a T with nothing after it).
func cvLooseValue(s string) (cvDur, bool) {
	m := cvDurationLoose.FindStringSubmatch(s)
	if m == nil || (m[2] == "" && m[3] == "" && m[4] == "") || (m[4] != "" && m[5] == "" && m[6] == "" && m[7] == "") {
		return cvDur{}, false
	}
	n := func(s string) int64 {
		if s == "" {
			return 0
		}
		v, err := strconv.ParseInt(s, 10, 32)
		if err != nil {
			return -1 << 62
		}
		return v
	}
	k := cvDur{m[1] == "-", n(m[2])*7 + n(m[3]), n(m[5])*3600 + n(m[6])*60 + n(m[7])}
	if k.days == 0 && k.seconds == 0 {
		k.neg = false
	}
	return k, k.days >= 0 && k.seconds >= 0
}

// ---------------------------------------------------------------------------
// §3.3.6 DURATION grammar

func TestConformanceValueDurationGrammar(t *testing.T) {
	for in, want := range map[string]ical.Duration{
		// The section's examples.
		"P15DT5H0M20S": {Days: 15, Hours: 5, Seconds: 20},
		"P7W":          {Weeks: 7},
		// Every branch of the grammar.
		"P1D":        {Days: 1},
		"P1DT2H":     {Days: 1, Hours: 2},
		"P1DT2M":     {Days: 1, Minutes: 2},
		"P1DT2S":     {Days: 1, Seconds: 2},
		"P1DT2H3M":   {Days: 1, Hours: 2, Minutes: 3},
		"P1DT2H3M4S": {Days: 1, Hours: 2, Minutes: 3, Seconds: 4},
		"P1DT2M3S":   {Days: 1, Minutes: 2, Seconds: 3},
		"PT1H":       {Hours: 1},
		"PT1H30M":    {Hours: 1, Minutes: 30},
		"PT1H0M5S":   {Hours: 1, Seconds: 5},
		"PT30M":      {Minutes: 30},
		"PT30M10S":   {Minutes: 30, Seconds: 10},
		"PT10S":      {Seconds: 10},
		"PT0S":       {},
		"P0D":        {},
		"P0W":        {},
		"PT90M":      {Minutes: 90},
		"PT36H":      {Hours: 36},
		"P400D":      {Days: 400},
		"P007D":      {Days: 7},
		// (["+"] / "-"): "Negative durations are typically used to schedule
		// an alarm to trigger before an associated time".
		"+P1D":    {Days: 1},
		"-P1D":    {Negative: true, Days: 1},
		"-PT15M":  {Negative: true, Minutes: 15},
		"-P2W":    {Negative: true, Weeks: 2},
		"+PT1H":   {Hours: 1},
		"-P1DT1S": {Negative: true, Days: 1, Seconds: 1},
	} {
		if !cvDurationABNF.MatchString(in) {
			t.Fatalf("test bug: %q isn't dur-value", in)
		}
		got, err := ical.ParseDuration(in)
		if err != nil || cvDurOf(got) != cvDurOf(want) {
			t.Errorf("ParseDuration(%q) = %+v, %v; want %+v", in, got, err, want)
		}
		// Weeks stay weeks: P7W re-encodes as P7W (docs/architecture.md).
		if in == "P7W" && got.String() != "P7W" {
			t.Errorf("ParseDuration(P7W).String() = %q", got.String())
		}
		if got, err := cvLine(t, "DURATION:"+in).Duration(); err != nil || cvDurOf(got) != cvDurOf(want) {
			t.Errorf("Duration(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}

	// Inputs the grammar forbids, with the reading a lenient reader may
	// give them (nil: none).
	for in, ok := range map[string][]cvDur{
		// No part at all.
		"": nil, "P": nil, "PT": nil, "-P": nil, "P1DT": nil, "-PT": nil,
		// "this value type doesn't support the "Y" and "M" designators to
		// specify durations in terms of years and months". P1M is a month in
		// ISO 8601, not a minute.
		"P1Y": nil, "P1M": nil, "P1Y2M": nil, "P1MT1H": nil, "P1D1M": nil,
		// dur-week stands alone.
		"P1W2D":  {{false, 9, 0}},
		"P1WT1H": {{false, 7, 3600}},
		"P1W1D1": nil,
		// dur-hour = 1*DIGIT "H" [dur-minute]: seconds after hours need the
		// minutes in between (hence the example's "0M").
		"PT1H5S":   {{false, 0, 3605}},
		"P1DT1H5S": {{false, 1, 3605}},
		// Order and repetition.
		"PT1M1H":   nil,
		"PT1S1M":   nil,
		"P1DT1S1H": nil,
		"PT1H1H":   nil,
		"P1D1D":    nil,
		"P1D1W":    nil,
		// Designators in the wrong half.
		"PT1D": nil, "PT1W": nil, "P1H": nil, "P1S": nil, "P1DT1D": nil,
		// No digits, or not 1*DIGIT.
		"PD": nil, "PTH": nil, "PW": nil, "P-1D": nil, "PT-1H": nil, "P+1D": nil,
		"PT1.5H": nil, "PT1,5H": nil, "P0.5D": nil, "PT1.0S": nil, "P1e2D": nil,
		"P١D": nil, "P1_0D": nil,
		// Signs.
		"--P1D": nil, "+-P1D": nil, "-+P1D": nil, "P1D-": nil,
		// No P.
		"1D": nil, "T1H": nil, "-1D": nil, "D": nil,
		// Whitespace and junk.
		" P1D": {{false, 1, 0}}, "P1D ": {{false, 1, 0}}, "P 1D": nil, "P1D,": nil,
		"P1DX": nil, "P1D\x00": nil, "PT1H 30M": nil,
		// §3.1: values are case-sensitive; §3.3.6 doesn't say otherwise.
		"p1d": {{false, 1, 0}}, "P1d": {{false, 1, 0}}, "pt1h": {{false, 0, 3600}},
		"PT1h": {{false, 0, 3600}}, "Pt1H": {{false, 0, 3600}}, "p7w": {{false, 49, 0}},
		"-pt15m": {{true, 0, 900}},
		// Too large for any integer the parts could be stored in: no value
		// read from it is right.
		"P99999999999999999999D":  nil,
		"PT99999999999999999999S": nil,
		"P99999999999999999999W":  nil,
	} {
		got, err := ical.ParseDuration(in)
		cvForbidden(t, "DURATION", in, cvDurOf(got), err, ok...)
	}

	// Every string built from the grammar's pieces and some that aren't: a
	// string the grammar allows must parse with its value; one it forbids
	// must be rejected, or read as cvDurationLoose reads it (logged by
	// shape); nothing else.
	tokens := []string{"1W", "2D", "T", "3H", "4M", "5S", "0S", "1Y", "1", "H", "1.5H", "M"}
	prefixes := []string{"P", "-P", "+P", "", "p", "P-"}
	lenient := map[string]bool{}
	var gen func(s string, n int)
	gen = func(s string, n int) {
		got, err := ical.ParseDuration(s)
		valid := cvDurationABNF.MatchString(s)
		want, loose := cvLooseValue(s)
		switch {
		case valid && (err != nil || cvDurOf(got) != want):
			t.Errorf("ParseDuration(%q) = %+v, %v; want %+v", s, got, err, want)
		case !valid && err == nil && (!loose || cvDurOf(got) != want):
			t.Errorf("ParseDuration(%q) = %+v; the grammar forbids it and no reading gives that", s, got)
		case !valid && err == nil:
			shape := strings.Map(func(r rune) rune {
				if r >= '0' && r <= '9' {
					return -1
				}
				return r
			}, s)
			lenient[shape] = true
		}
		if n == 0 {
			return
		}
		for _, tok := range tokens {
			gen(s+tok, n-1)
		}
	}
	for _, p := range prefixes {
		gen(p, 5)
	}
	shapes := slices.Sorted(maps.Keys(lenient))
	if len(shapes) > 0 {
		t.Logf("LENIENT DURATION shapes accepted outside the grammar (digits removed): %q", shapes)
	}
}

// The writer must produce exactly dur-value, and it must read back as the
// same duration.
func TestConformanceValueDurationString(t *testing.T) {
	for d, want := range map[ical.Duration]string{
		// The section's examples.
		{Days: 15, Hours: 5, Seconds: 20}: "P15DT5H0M20S",
		{Weeks: 7}:                        "P7W",
		{Negative: true, Minutes: 15}:     "-PT15M",
		{Days: 1}:                         "P1D",
		{Hours: 1, Seconds: 5}:            "PT1H0M5S",
	} {
		if got := d.String(); got != want {
			t.Errorf("%+v.String() = %q; want %q", d, got, want)
		}
	}

	var ds []ical.Duration
	for _, neg := range []bool{false, true} {
		for _, w := range []int{0, 1, 3} {
			for _, dd := range []int{0, 1, 15} {
				for _, h := range []int{0, 1, 25} {
					for _, m := range []int{0, 1, 90} {
						for _, s := range []int{0, 1, 20, 3600} {
							ds = append(ds, ical.Duration{Negative: neg, Weeks: w, Days: dd, Hours: h, Minutes: m, Seconds: s})
						}
					}
				}
			}
		}
	}
	for _, d := range append(ds, ical.DurationOf(-90*time.Minute), ical.DurationOf(1500*time.Millisecond),
		ical.DurationOf(0), ical.DurationOf(49*time.Hour+time.Second)) {
		s := d.String()
		if !cvDurationABNF.MatchString(s) {
			t.Errorf("%+v.String() = %q, which isn't §3.3.6 dur-value", d, s)
			continue
		}
		// Round trip: the same sign, nominal days and exact seconds. Hours
		// must never become days: they aren't the same length across a DST
		// change.
		back, err := ical.ParseDuration(s)
		if err != nil || cvDurOf(back) != cvDurOf(d) {
			t.Errorf("%+v.String() = %q, which reads back as %+v, %v", d, s, back, err)
		}
		p := ical.NewProperty("DURATION", "")
		p.SetDuration(d)
		if p.Value != s {
			t.Errorf("SetDuration(%+v) wrote %q; want %q", d, p.Value, s)
		}
	}

	// DurationOf: exact, whole seconds; §3.3.12/§3.3.6 have no fractions.
	for in, want := range map[time.Duration]cvDur{
		-90 * time.Minute:          {true, 0, 5400},
		1500 * time.Millisecond:    {false, 0, 1},
		0:                          {},
		49*time.Hour + time.Second: {false, 0, 49*3600 + 1},
	} {
		if got := cvDurOf(ical.DurationOf(in)); got != want {
			t.Errorf("DurationOf(%v) = %+v; want %+v", in, got, want)
		}
	}

	// §3.2.20: a stale VALUE=DATE-TIME (an absolute TRIGGER) would declare
	// the wrong type for a duration.
	p := ical.NewProperty("TRIGGER", "19980403T120000Z")
	p.Params.Set("VALUE", "DATE-TIME")
	p.SetDuration(ical.Duration{Negative: true, Minutes: 15})
	if v := p.Params.Get("VALUE"); v != "" && !strings.EqualFold(v, "DURATION") {
		t.Errorf("SetDuration on TRIGGER;VALUE=DATE-TIME left VALUE=%s on %q", v, p.Value)
	}
}

// ---------------------------------------------------------------------------
// §3.3.6 DURATION arithmetic

// cvAt reads a DATE-TIME property value in zone ("" for floating, "Z" for a
// UTC value written with its Z).
func cvAt(t *testing.T, zone, value string) ical.DateTime {
	t.Helper()
	line := "DTSTART:" + value
	if zone != "" {
		line = "DTSTART;TZID=" + zone + ":" + value
	}
	v, err := cvLine(t, line).DateTime(nil)
	if err != nil {
		t.Fatalf("%s: %v", line, err)
	}
	return v
}

func cvMustDuration(t *testing.T, s string) ical.Duration {
	t.Helper()
	d, err := ical.ParseDuration(s)
	if err != nil {
		t.Fatalf("ParseDuration(%q): %v", s, err)
	}
	return d
}

// RFC 5545 §3.3.6: "The duration of a week or a day depends on its position
// in the calendar. In the case of discontinuities in the time scale, such as
// the change from standard time to daylight time and back, the computation
// of the exact duration requires the subtraction or addition of the change
// of duration of the discontinuity. ... When computing an exact duration,
// the greatest order time components MUST be added first, that is, the
// number of days MUST be added first, followed by the number of hours,
// number of minutes, and number of seconds."
//
// So days and weeks move the wall clock (P1D across a DST change is 23 or 25
// hours) and hours, minutes and seconds move the instant (PT24H is 24
// hours). A wall clock that lands in a gap or an overlap is read by §3.3.5.
// Transitions as in cvDSTCases.
func TestConformanceValueDurationArithmetic(t *testing.T) {
	for _, c := range []struct {
		zone, start, dur, want, why string
	}{
		// New York, spring forward on 2007-03-11 (23-hour day).
		{"America/New_York", "20070310T120000", "P1D", "2007-03-11T16:00:00Z", "noon EST + 1 day = noon EDT, 23 h"},
		{"America/New_York", "20070310T120000", "PT24H", "2007-03-11T17:00:00Z", "noon EST + 24 h = 13:00 EDT"},
		{"America/New_York", "20070307T120000", "P1W", "2007-03-14T16:00:00Z", "a week is 7 nominal days"},
		{"America/New_York", "20070310T120000", "P0DT24H", "2007-03-11T17:00:00Z", "the time part is exact"},
		// New York, fall back on 2007-11-04 (25-hour day).
		{"America/New_York", "20071103T120000", "P1D", "2007-11-04T17:00:00Z", "noon EDT + 1 day = noon EST, 25 h"},
		{"America/New_York", "20071103T120000", "PT24H", "2007-11-04T16:00:00Z", "noon EDT + 24 h = 11:00 EST"},
		// Negative durations go back the same way.
		{"America/New_York", "20070311T120000", "-P1D", "2007-03-10T17:00:00Z", "noon EDT - 1 day = noon EST"},
		{"America/New_York", "20070311T120000", "-PT24H", "2007-03-10T16:00:00Z", "noon EDT - 24 h = 11:00 EST"},
		{"America/New_York", "20071104T120000", "-P1D", "2007-11-03T16:00:00Z", "noon EST - 1 day = noon EDT"},
		// Landing in a gap or an overlap: §3.3.5.
		{"America/New_York", "20070310T023000", "P1D", "2007-03-11T07:30:00Z", "02:30 doesn't exist: offset before the gap"},
		{"America/New_York", "20071103T013000", "P1D", "2007-11-04T05:30:00Z", "01:30 occurs twice: the first"},
		// Days first, then hours. From 02:30 EST on 2007-03-10, days first
		// gives 02:30 on 03-11 (in the gap: 07:30Z) + 1 h = 08:30Z; hours
		// first would give 03:30 + 1 day = 03:30 EDT = 07:30Z.
		{"America/New_York", "20070310T023000", "P1DT1H", "2007-03-11T08:30:00Z", "days added before hours"},
		// From 01:30 EDT on 2007-11-03: days first gives the first 01:30 on
		// 11-04 (05:30Z) + 1 h = 06:30Z; hours first would give 02:30 + 1
		// day = 02:30 EST = 07:30Z.
		{"America/New_York", "20071103T013000", "P1DT1H", "2007-11-04T06:30:00Z", "days added before hours"},
		// Lord Howe's 30-minute shift on 2023-10-01 (23.5-hour day).
		{"Australia/Lord_Howe", "20230930T120000", "P1D", "2023-10-01T01:00:00Z", "noon +10:30 + 1 day = noon +11"},
		{"Australia/Lord_Howe", "20230930T120000", "PT24H", "2023-10-01T01:30:00Z", "24 h later = 12:30 +11"},
		// Apia skipped 2011-12-30: noon that day never happened, and reads
		// with the offset before the gap (-10), which is 12:00 on 12-31.
		{"Pacific/Apia", "20111229T120000", "P1D", "2011-12-30T22:00:00Z", "into the skipped day"},
		{"Pacific/Apia", "20111229T120000", "P2D", "2011-12-30T22:00:00Z", "noon on 12-31 (+14)"},
		{"Pacific/Apia", "20111229T120000", "PT24H", "2011-12-30T22:00:00Z", "24 h later"},
		// 12-31 12:00 (+14) is 22:00Z on 12-30; a day back on the wall
		// clock is 12-30 12:00, skipped, so read at -10: 22:00Z again.
		{"Pacific/Apia", "20111231T120000", "-P1D", "2011-12-30T22:00:00Z", "back into the skipped day"},
		// UTC has no discontinuities: a day is 24 hours.
		{"", "20070310T120000Z", "P1D", "2007-03-11T12:00:00Z", "UTC"},
		{"", "20070310T120000Z", "-P1DT1H", "2007-03-09T11:00:00Z", "UTC, negative"},
	} {
		start := cvAt(t, c.zone, c.start)
		got := start.Add(cvMustDuration(t, c.dur))
		want := cvUTC(c.want)
		if !got.Time().Equal(want) {
			t.Errorf("%s %s + %s = %v; want %v (%s)", c.zone, c.start, c.dur, got.Time().UTC(), want, c.why)
		}
		// The result keeps the start's form (§3.3.6 doesn't change it), and
		// written out and read back it names the same instant.
		if got.UTC != start.UTC || got.TZID != start.TZID || got.IsDate {
			t.Errorf("%s %s + %s changed form: %+v", c.zone, c.start, c.dur, got)
		}
		p := ical.NewProperty("DTSTART", "")
		p.SetDateTime(got)
		if back, err := cvRoundTrip(t, p).DateTime(nil); err != nil || !back.Time().Equal(want) {
			t.Errorf("%s %s + %s written as %q;%v reads back as %v, %v; want %v",
				c.zone, c.start, c.dur, p.Value, p.Params, back.Time().UTC(), err, want)
		}
	}

	// Floating: no zone, so no discontinuity; both kinds move the wall
	// clock alike.
	f := cvAt(t, "", "20070310T120000")
	for _, d := range []string{"P1D", "PT24H"} {
		got := f.Add(cvMustDuration(t, d))
		if !got.Floating() || got.IsDate || got.DateTime != cvWall(2007, 3, 11, 12, 0, 0) {
			t.Errorf("floating 20070310T120000 + %s = %+v", d, got)
		}
	}

	// DATE values: days and weeks move the date.
	for _, c := range []struct {
		start civil.Date
		dur   string
		want  civil.Date
	}{
		{civil.Date{Year: 2007, Month: 3, Day: 10}, "P1D", civil.Date{Year: 2007, Month: 3, Day: 11}},
		{civil.Date{Year: 2007, Month: 3, Day: 10}, "P1W", civil.Date{Year: 2007, Month: 3, Day: 17}},
		{civil.Date{Year: 2007, Month: 3, Day: 10}, "-P1D", civil.Date{Year: 2007, Month: 3, Day: 9}},
		{civil.Date{Year: 2007, Month: 12, Day: 31}, "P1D", civil.Date{Year: 2008, Month: 1, Day: 1}},
		{civil.Date{Year: 2008, Month: 2, Day: 28}, "P1D", civil.Date{Year: 2008, Month: 2, Day: 29}},
		{civil.Date{Year: 2100, Month: 2, Day: 28}, "P1D", civil.Date{Year: 2100, Month: 3, Day: 1}},
		{civil.Date{Year: 2000, Month: 3, Day: 1}, "-P1D", civil.Date{Year: 2000, Month: 2, Day: 29}},
		{civil.Date{Year: 1999, Month: 1, Day: 1}, "P365D", civil.Date{Year: 2000, Month: 1, Day: 1}},
	} {
		got := ical.NewDate(c.start).Add(cvMustDuration(t, c.dur))
		if !got.IsDate || got.DateTime.Date != c.want {
			t.Errorf("DATE %v + %s = %+v; want the DATE %v", c.start, c.dur, got, c.want)
		}
	}
	// A DATE plus a duration with a time part: the RFC doesn't define it.
	// The documented choice (DateTime.Add) is a floating DATE-TIME.
	got := ical.NewDate(civil.Date{Year: 2007, Month: 3, Day: 10}).Add(cvMustDuration(t, "PT1H"))
	if got.IsDate || !got.Floating() || got.DateTime != cvWall(2007, 3, 10, 1, 0, 0) {
		t.Errorf("DATE 2007-03-10 + PT1H = %+v; want the floating 2007-03-10T01:00:00 (documented)", got)
	}
}

// ---------------------------------------------------------------------------
// §3.3.10 RECUR, the grammar only (expansion is the recur package's)

// RFC 5545 §3.3.10's ABNF for one rule part: name "=" value, with the
// ranges its comments give.
var cvRecurPartABNF = func() *regexp.Regexp {
	const (
		sec     = `(?:[0-9]|[0-5][0-9]|60)`
		min     = `(?:[0-9]|[0-5][0-9])`
		hour    = `(?:[0-9]|[01][0-9]|2[0-3])`
		ordwk   = `(?:0?[1-9]|[1-4][0-9]|5[0-3])`
		ordmo   = `(?:0?[1-9]|[12][0-9]|3[01])`
		ordyr   = `(?:0{0,2}[1-9]|0?[1-9][0-9]|[12][0-9][0-9]|3[0-5][0-9]|36[0-6])`
		wday    = `(?:SU|MO|TU|WE|TH|FR|SA)`
		sign    = `[+-]?`
		date    = `[0-9]{8}`
		dt      = `[0-9]{8}T[0-9]{6}Z?`
		freq    = `(?:SECONDLY|MINUTELY|HOURLY|DAILY|WEEKLY|MONTHLY|YEARLY)`
		list    = `(?:%[1]s)(?:,(?:%[1]s))*`
		wdaynum = `(?:` + sign + ordwk + `)?` + wday
	)
	l := func(s string) string { return strings.ReplaceAll(list, "%[1]s", s) }
	return regexp.MustCompile(`^(?:FREQ=` + freq + `|UNTIL=(?:` + dt + `|` + date + `)|COUNT=[0-9]+|INTERVAL=[0-9]+` +
		`|BYSECOND=` + l(sec) + `|BYMINUTE=` + l(min) + `|BYHOUR=` + l(hour) + `|BYDAY=` + l(wdaynum) +
		`|BYMONTHDAY=` + l(sign+ordmo) + `|BYYEARDAY=` + l(sign+ordyr) + `|BYWEEKNO=` + l(sign+ordwk) +
		`|BYMONTH=` + l(`(?:0?[1-9]|1[0-2])`) + `|BYSETPOS=` + l(sign+ordyr) + `|WKST=` + wday + `)$`)
}()

func TestConformanceValueRecurGrammar(t *testing.T) {
	for _, in := range []string{
		"FREQ=DAILY;COUNT=10",
		// "The rule parts are not ordered in any particular sequence."
		"COUNT=10;FREQ=DAILY",
		"INTERVAL=2;FREQ=WEEKLY;WKST=SU;BYDAY=TU,TH",
		// enddate = date / date-time.
		"FREQ=DAILY;UNTIL=19971224T000000Z",
		"FREQ=DAILY;UNTIL=19971224",
		"FREQ=DAILY;UNTIL=19971224T000000",
		// Every BYxxx at the ends of its range.
		"FREQ=MINUTELY;BYSECOND=0,60",
		"FREQ=HOURLY;BYMINUTE=0,59",
		"FREQ=DAILY;BYHOUR=0,23",
		"FREQ=MONTHLY;BYMONTHDAY=1,31,-1,-31,+15",
		"FREQ=YEARLY;BYYEARDAY=1,366,-1,-366",
		"FREQ=YEARLY;BYWEEKNO=1,53,-1,-53",
		"FREQ=YEARLY;BYMONTH=1,12",
		"FREQ=YEARLY;BYMONTH=12;BYDAY=MO,+1MO,-53SU,53FR",
		"FREQ=MONTHLY;BYDAY=MO,TU,WE,TH,FR;BYSETPOS=-1,1,366,-366",
		// The §3.8.5.3 examples' shapes.
		"FREQ=YEARLY;INTERVAL=2;BYMONTH=1;BYDAY=SU;BYHOUR=8,9;BYMINUTE=30",
		"FREQ=MONTHLY;BYDAY=FR;BYMONTHDAY=13",
	} {
		r, err := cvLine(t, "RRULE:"+in).Recur()
		if err != nil {
			t.Errorf("Recur(%q): %v", in, err)
			continue
		}
		// The writer emits the grammar, and the rule reads back the same.
		p := ical.NewProperty("RRULE", "")
		p.SetRecur(r)
		for part := range strings.SplitSeq(p.Value, ";") {
			if !cvRecurPartABNF.MatchString(part) {
				t.Errorf("SetRecur(Recur(%q)) wrote %q: %q isn't a recur-rule-part", in, p.Value, part)
			}
		}
		if back, err := cvRoundTrip(t, p).Recur(); err != nil || !back.Equal(r) {
			t.Errorf("SetRecur(Recur(%q)) wrote %q, which reads back as %v, %v", in, p.Value, back, err)
		}
	}

	for in, why := range map[string]string{
		"":                                   "FREQ is REQUIRED",
		"COUNT=10":                           "FREQ is REQUIRED",
		"FREQ=DAILY;FREQ=WEEKLY":             "FREQ MUST NOT occur more than once",
		"FREQ=DAILY;COUNT=5;COUNT=6":         "other parts MUST NOT occur more than once",
		"FREQ=DAILY;BYHOUR=1;BYHOUR=2":       "other parts MUST NOT occur more than once",
		"FREQ=FORTNIGHTLY":                   "freq",
		"FREQ=DAILY;UNTIL=1997":              "enddate",
		"FREQ=DAILY;COUNT=ten":               "1*DIGIT",
		"FREQ=DAILY;INTERVAL=-1":             "1*DIGIT",
		"FREQ=MINUTELY;BYSECOND=61":          "seconds 0 to 60",
		"FREQ=HOURLY;BYMINUTE=60":            "minutes 0 to 59",
		"FREQ=DAILY;BYHOUR=24":               "hour 0 to 23",
		"FREQ=MONTHLY;BYMONTHDAY=0":          "ordmoday 1 to 31",
		"FREQ=MONTHLY;BYMONTHDAY=32":         "ordmoday 1 to 31",
		"FREQ=YEARLY;BYYEARDAY=0":            "ordyrday 1 to 366",
		"FREQ=YEARLY;BYYEARDAY=367":          "ordyrday 1 to 366",
		"FREQ=YEARLY;BYWEEKNO=0":             "ordwk 1 to 53",
		"FREQ=YEARLY;BYWEEKNO=54":            "ordwk 1 to 53",
		"FREQ=YEARLY;BYMONTH=0":              "monthnum 1 to 12",
		"FREQ=YEARLY;BYMONTH=13":             "monthnum 1 to 12",
		"FREQ=YEARLY;BYSETPOS=0;BYMONTH=1":   "setposday 1 to 366",
		"FREQ=YEARLY;BYSETPOS=367;BYMONTH=1": "setposday 1 to 366",
		"FREQ=YEARLY;BYDAY=54MO":             "ordwk 1 to 53",
		"FREQ=WEEKLY;BYDAY=XX":               "weekday",
		"FREQ=WEEKLY;WKST=XX":                "weekday",
		"FREQ=DAILY;BYHOUR=1,,2":             "a list item is required",
		"FREQ=DAILY;X-NAME=1":                "recur-rule-part has no extension parts",
		"FREQ":                               "\"=\" freq",
	} {
		if r, err := (&ical.Property{Name: "RRULE", Value: in}).Recur(); err == nil {
			t.Errorf("Recur(%q) = %v; %s", in, r, why)
		}
	}
	// "The UNTIL or COUNT rule parts are OPTIONAL, but they MUST NOT occur
	// in the same 'recur'." Reading is lenient about MUSTs between parts;
	// Rule.Validate is the strict check (docs/architecture.md).
	in := "FREQ=DAILY;COUNT=5;UNTIL=19971224T000000Z"
	if r, err := (&ical.Property{Name: "RRULE", Value: in}).Recur(); err == nil {
		t.Logf("LENIENT RECUR %q -> %v", in, r)
		if r.Validate() == nil {
			t.Errorf("Recur(%q).Validate() = nil; UNTIL and COUNT MUST NOT occur in the same recur", in)
		}
	}
	// Grammatical but outside the description: "The INTERVAL rule part
	// contains a positive integer"; COUNT=0 and BYDAY=0MO (ordwk is 1 to
	// 53) name nothing. A lenient reader may take them; only reported.
	for _, in := range []string{"FREQ=DAILY;INTERVAL=0", "FREQ=DAILY;COUNT=0", "FREQ=MONTHLY;BYDAY=0MO,1TU"} {
		if r, err := (&ical.Property{Name: "RRULE", Value: in}).Recur(); err == nil {
			t.Logf("LENIENT RECUR %q -> %v", in, r)
		}
	}
}

// ---------------------------------------------------------------------------
// §3.3.9 PERIOD

func TestConformanceValuePeriod(t *testing.T) {
	// The section's first example: "The period starting at 18:00:00 UTC, on
	// January 1, 1997 and ending at 07:00:00 UTC on January 2, 1997".
	ps, err := cvLine(t, "FREEBUSY:19970101T180000Z/19970102T070000Z").Periods(nil)
	if err != nil || len(ps) != 1 || !ps[0].Start.UTC || !ps[0].Start.Time().Equal(cvUTC("1997-01-01T18:00:00Z")) ||
		!ps[0].End.UTC || !ps[0].End.Time().Equal(cvUTC("1997-01-02T07:00:00Z")) || !ps[0].Duration.IsZero() ||
		!ps[0].EndTime().Time().Equal(cvUTC("1997-01-02T07:00:00Z")) {
		t.Errorf("period-explicit = %+v, %v", ps, err)
	}

	// The second: a start and "lasting 5 hours and 30 minutes", as printed
	// (UTC) and as corrected by erratum 7332 (Verified; floating).
	ps, err = cvLine(t, "FREEBUSY:19970101T180000Z/PT5H30M").Periods(nil)
	if err != nil || len(ps) != 1 || !ps[0].Start.UTC || ps[0].Duration != (ical.Duration{Hours: 5, Minutes: 30}) ||
		!ps[0].EndTime().UTC || !ps[0].EndTime().Time().Equal(cvUTC("1997-01-01T23:30:00Z")) {
		t.Errorf("period-start = %+v, %v", ps, err)
	}
	ps, err = cvLine(t, "RDATE;VALUE=PERIOD:19970101T180000/PT5H30M").Periods(nil)
	if err != nil || len(ps) != 1 || !ps[0].Start.Floating() || ps[0].Start.DateTime != cvWall(1997, 1, 1, 18, 0, 0) ||
		!ps[0].EndTime().Floating() || ps[0].EndTime().DateTime != cvWall(1997, 1, 1, 23, 30, 0) {
		t.Errorf("erratum 7332 period = %+v, %v", ps, err)
	}

	// With a TZID both ends are in the zone. A nominal day in the duration
	// follows §3.3.6: across the DST change it ends at the same wall time.
	ps, err = cvLine(t, "RDATE;VALUE=PERIOD;TZID=America/New_York:19970101T180000/19970102T070000,20070310T120000/P1D").Periods(nil)
	if err != nil || len(ps) != 2 ||
		ps[0].Start.TZID != "America/New_York" || !ps[0].Start.Time().Equal(cvUTC("1997-01-01T23:00:00Z")) ||
		ps[0].End.TZID != "America/New_York" || !ps[0].End.Time().Equal(cvUTC("1997-01-02T12:00:00Z")) ||
		!ps[1].EndTime().Time().Equal(cvUTC("2007-03-11T16:00:00Z")) {
		t.Errorf("RDATE;VALUE=PERIOD;TZID = %+v, %v", ps, err)
	}

	// §3.8.2.6 example: a list mixing both forms.
	ps, err = cvLine(t, "FREEBUSY;FBTYPE=FREE:19970308T160000Z/PT3H,19970308T200000Z/PT1H,19970308T230000Z/19970309T000000Z").Periods(nil)
	if err != nil || len(ps) != 3 || !ps[2].EndTime().Time().Equal(cvUTC("1997-03-09T00:00:00Z")) ||
		!ps[1].EndTime().Time().Equal(cvUTC("1997-03-08T21:00:00Z")) {
		t.Errorf("FREEBUSY list = %+v, %v", ps, err)
	}
	// RDATE;VALUE=PERIOD read as DATE-TIMEs gives the starts (documented).
	vs, err := cvLine(t, "RDATE;VALUE=PERIOD:19960403T020000Z/19960403T040000Z,19960404T010000Z/PT3H").DateTimes(nil)
	if err != nil || len(vs) != 2 || !vs[1].Time().Equal(cvUTC("1996-04-04T01:00:00Z")) {
		t.Errorf("RDATE;VALUE=PERIOD DateTimes = %v, %v", vs, err)
	}

	// FREEBUSY's fbvalue is period *("," period): an empty value isn't
	// one, but an empty list is its only reading.
	if ps, err := (&ical.Property{Name: "FREEBUSY", Value: ""}).Periods(nil); err == nil {
		if len(ps) != 0 {
			t.Errorf("Periods(%q) = %+v", "", ps)
		}
		t.Logf("LENIENT PERIOD %q -> %+v", "", ps)
	}

	// Not a PERIOD.
	for _, in := range []string{
		"/", "19970101T180000Z", "19970101T180000Z/", "/PT1H", "/19970102T070000Z",
		"19970101T180000Z/PT1H/PT1H", "19970101T180000Z/garbage", "19970101T180000Z/P1Y",
		"19970101T180000Z/P", "19970101T180000Z//PT1H", "19970101T180000Z PT1H",
		"19970101T180000Z/19970102T250000Z",
	} {
		if ps, err := (&ical.Property{Name: "FREEBUSY", Value: in}).Periods(nil); err == nil {
			t.Errorf("Periods(%q) = %+v; that isn't a PERIOD", in, ps)
		}
	}

	// "The start of the period MUST be before the end of the period" and
	// period-start has "a start and positive duration". A reader with no
	// strict mode may still return what was written; that is reported.
	for _, in := range []string{
		"19970102T070000Z/19970101T180000Z",
		"19970101T180000Z/19970101T180000Z",
		"19970101T180000Z/-PT1H",
		"19970101T180000Z/PT0S",
		// date-time is required at both ends.
		"19970101/19970102",
		"19970101T180000Z/19970102",
		// Both ends are date-time, but in different forms.
		"19970101T180000Z/19970102T070000",
	} {
		ps, err := (&ical.Property{Name: "FREEBUSY", Value: in}).Periods(nil)
		if err == nil {
			t.Logf("LENIENT PERIOD %q -> %+v", in, ps)
		}
	}
}
