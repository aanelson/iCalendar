package recur_test

// Targeted checks of the normative sentences of RFC 5545 §3.3.10 (RECUR)
// and §3.8.5 (EXDATE, RDATE, RRULE) that the differential test in
// conformance_recur_diff_test.go can't reach: the grammar, the MUST NOTs
// between rule parts, UNTIL's forms, DATE starts, and recurrence sets in a
// time zone. Every expected value is worked out from the RFC text (and the
// Verified errata in docs/rfc/rfc5545-errata.md) in the comments.

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aanelson/iCalendar/civil"
	"github.com/aanelson/iCalendar/recur"
)

// cdt reads a basic-format local DATE-TIME (19970902T090000) or DATE.
func cdt(s string) civil.DateTime {
	n := func(i, j int) int {
		v, err := strconv.Atoi(s[i:j])
		if err != nil {
			panic(s)
		}
		return v
	}
	d := civil.DateTime{Date: civil.Date{Year: n(0, 4), Month: time.Month(n(4, 6)), Day: n(6, 8)}}
	if len(s) > 8 {
		d.Time = civil.Time{Hour: n(9, 11), Minute: n(11, 13), Second: n(13, 15)}
	}
	return d
}

func cdts(ss ...string) []civil.DateTime {
	var out []civil.DateTime
	for _, s := range ss {
		out = append(out, cdt(s))
	}
	return out
}

func mustRule(t *testing.T, s string) recur.Rule {
	t.Helper()
	r, err := recur.ParseRule(s)
	if err != nil {
		t.Fatalf("ParseRule(%q): %v", s, err)
	}
	return r
}

func all(seq func(func(civil.DateTime) bool), max int) []civil.DateTime {
	var out []civil.DateTime
	for t := range seq {
		if out = append(out, t); len(out) == max {
			break
		}
	}
	return out
}

func newYork(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("no zone data: %v", err)
	}
	return loc
}

// ---- §3.3.10 Format Definition ----

func TestConformanceParseGrammar(t *testing.T) {
	for _, c := range []struct {
		in string
		ok bool
	}{
		// recur-rule-part values, at the ends of the ranges the ABNF
		// comments give.
		{"FREQ=MINUTELY;BYSECOND=0,60", true}, // seconds = 1*2DIGIT ;0 to 60
		{"FREQ=MINUTELY;BYSECOND=61", false},
		{"FREQ=MINUTELY;BYSECOND=-1", false}, // no sign in "seconds"
		{"FREQ=MINUTELY;BYSECOND=+1", false},
		{"FREQ=HOURLY;BYMINUTE=0,59", true}, // minutes = 1*2DIGIT ;0 to 59
		{"FREQ=HOURLY;BYMINUTE=60", false},
		{"FREQ=DAILY;BYHOUR=0,23", true}, // hour = 1*2DIGIT ;0 to 23
		{"FREQ=DAILY;BYHOUR=24", false},
		{"FREQ=DAILY;BYHOUR=-1", false},
		{"FREQ=MONTHLY;BYMONTHDAY=1,31,-1,-31,+1", true}, // monthdaynum = [plus / minus] ordmoday ;1 to 31
		{"FREQ=MONTHLY;BYMONTHDAY=0", false},
		{"FREQ=MONTHLY;BYMONTHDAY=32", false},
		{"FREQ=MONTHLY;BYMONTHDAY=-32", false},
		{"FREQ=YEARLY;BYYEARDAY=1,366,-1,-366,+366", true}, // ordyrday = 1*3DIGIT ;1 to 366
		{"FREQ=YEARLY;BYYEARDAY=0", false},
		{"FREQ=YEARLY;BYYEARDAY=367", false},
		{"FREQ=YEARLY;BYYEARDAY=-367", false},
		{"FREQ=YEARLY;BYWEEKNO=1,53,-1,-53,+20", true}, // weeknum = [plus / minus] ordwk ;1 to 53
		{"FREQ=YEARLY;BYWEEKNO=0", false},
		{"FREQ=YEARLY;BYWEEKNO=54", false},
		{"FREQ=YEARLY;BYWEEKNO=-54", false},
		{"FREQ=YEARLY;BYMONTH=1,12", true}, // monthnum = 1*2DIGIT ;1 to 12
		{"FREQ=YEARLY;BYMONTH=0", false},
		{"FREQ=YEARLY;BYMONTH=13", false},
		{"FREQ=YEARLY;BYMONTH=-1", false}, // no sign in "monthnum"
		{"FREQ=YEARLY;BYMONTH=+1", false},
		{"FREQ=MONTHLY;BYDAY=MO;BYSETPOS=1,366,-1,-366,+2", true}, // setposday = yeardaynum
		{"FREQ=MONTHLY;BYDAY=MO;BYSETPOS=0", false},
		{"FREQ=MONTHLY;BYDAY=MO;BYSETPOS=367", false},
		{"FREQ=MONTHLY;BYDAY=MO;BYSETPOS=-367", false},
		{"FREQ=YEARLY;BYDAY=1MO,+53SU,-53SA,-1FR,TU", true}, // weekdaynum = [[plus / minus] ordwk] weekday
		{"FREQ=YEARLY;BYDAY=54MO", false},
		{"FREQ=YEARLY;BYDAY=-54MO", false},
		{"FREQ=YEARLY;BYDAY=MO1", false},
		{"FREQ=YEARLY;BYDAY=XX", false},
		{"FREQ=YEARLY;BYDAY=+MO", false}, // a sign needs an ordwk
		{"FREQ=WEEKLY;WKST=SU", true},    // weekday = "SU" / "MO" / ...
		{"FREQ=WEEKLY;WKST=1MO", false},
		{"FREQ=WEEKLY;WKST=XX", false},
		{"FREQ=DAILY;INTERVAL=2;COUNT=10", true}, // 1*DIGIT
		{"FREQ=DAILY;INTERVAL=-1", false},
		{"FREQ=DAILY;COUNT=-1", false},
		{"FREQ=DAILY;COUNT=", false},
		{"FREQ=DAILY;UNTIL=19971224", true}, // enddate = date / date-time
		{"FREQ=DAILY;UNTIL=19971224T000000", true},
		{"FREQ=DAILY;UNTIL=19971224T000000Z", true},
		{"FREQ=DAILY;UNTIL=1997122", false},
		{"FREQ=DAILY;UNTIL=19971224T0000Z", false},
		{"FREQ=DAILY;UNTIL=19970230", false}, // date-mday for the month (§3.3.4)
		{"FREQ=FORTNIGHTLY", false},          // freq = "SECONDLY" / ... / "YEARLY"
		{"FREQ=DAILY;BYDAY=", false},         // bywdaylist = ( weekdaynum *("," weekdaynum) )
		{"FREQ=DAILY;BYDAY=MO,,TU", false},
		{"FREQ=DAILY;FOO=1", false}, // not a recur-rule-part
		{"FREQ=DAILY;BYDAY", false},

		// "The FREQ rule part is REQUIRED, but MUST NOT occur more than
		// once." "The other rule parts are OPTIONAL, but MUST NOT occur
		// more than once."
		{"COUNT=3", false},
		{"", false},
		{"FREQ=DAILY;FREQ=DAILY", false},
		{"FREQ=DAILY;FREQ=WEEKLY", false},
		{"FREQ=DAILY;BYDAY=MO;BYDAY=TU", false},
		{"FREQ=DAILY;COUNT=2;COUNT=2", false},
		{"FREQ=DAILY;INTERVAL=2;INTERVAL=2", false},
		{"FREQ=DAILY;WKST=MO;WKST=MO", false},

		// "Compliant applications MUST accept rule parts ordered in any
		// sequence".
		{"COUNT=3;FREQ=DAILY", true},
		{"BYDAY=MO;WKST=SU;FREQ=WEEKLY;INTERVAL=2", true},
	} {
		_, err := recur.ParseRule(c.in)
		if (err == nil) != c.ok {
			t.Errorf("ParseRule(%q) = %v; want ok %v", c.in, err, c.ok)
		}
	}
}

// Leniencies the architecture doc settles, outside the grammar: a 0MO entry
// (ordwk is 1 to 53) and COUNT=0 or INTERVAL=0 (INTERVAL "contains a
// positive integer"; COUNT=0 contradicts DTSTART always counting).
func TestConformanceParseDocumentedChoices(t *testing.T) {
	for _, c := range []struct {
		in   string
		ok   bool
		note string
	}{
		{"FREQ=DAILY;INTERVAL=0", false, "INTERVAL ... contains a positive integer"},
		{"FREQ=DAILY;COUNT=0", false, "architecture doc: COUNT below 1 is refused"},
		{"freq=daily;byday=mo", true, "architecture doc: names and values are case-insensitive"},
		{"FREQ=DAILY;COUNT=3;", true, "architecture doc: an empty part is ignored"},
		{"FREQ=DAILY;BYDAY=MO, TU, WE", true, "ParseRule doc: spaces around list items are ignored (a corpus feed writes this)"},
		// The grammar's 1*2DIGIT and 1*3DIGIT leave no room for these, but
		// the number is unambiguous, and refusing it would drop a feed's
		// whole rule.
		{"FREQ=MINUTELY;BYSECOND=005", true, "ParseRule doc: leading zeros are read"},
		{"FREQ=YEARLY;BYYEARDAY=0001", true, "ParseRule doc: leading zeros are read"},
	} {
		_, err := recur.ParseRule(c.in)
		if (err == nil) != c.ok {
			t.Errorf("ParseRule(%q) = %v; want ok %v (%s)", c.in, err, c.ok, c.note)
		}
	}
	if r := mustRule(t, "FREQ=MONTHLY;BYMONTHDAY=001;BYDAY=001MO"); !slices.Equal(r.ByMonthDay, []int{1}) ||
		!slices.Equal(r.ByDay, []recur.WeekdayNum{{N: 1, Day: recur.Monday}}) {
		t.Errorf("leading zeros read as BYMONTHDAY=%v BYDAY=%v", r.ByMonthDay, r.ByDay)
	}
	// 0MO: ordwk is 1 to 53, so the grammar has no 0MO. The architecture
	// doc says a 0MO entry is dropped, as ical.net does; either reading
	// may not let it stand for "every Monday".
	if r, err := recur.ParseRule("FREQ=MONTHLY;BYDAY=0MO,TU"); err == nil {
		if slices.Contains(r.ByDay, recur.WeekdayNum{Day: recur.Monday}) {
			t.Errorf("BYDAY=0MO,TU read as %v: 0MO became every Monday", r.ByDay)
		}
	}
}

func TestConformanceParseUntilForms(t *testing.T) {
	// "The UNTIL rule part defines a DATE or DATE-TIME value"; it keeps the
	// form it was written in, since its comparison depends on it.
	for _, c := range []struct {
		in          string
		want        civil.DateTime
		isDate, utc bool
	}{
		{"FREQ=DAILY;UNTIL=19971224", cdt("19971224"), true, false},
		{"FREQ=DAILY;UNTIL=19971224T235959", cdt("19971224T235959"), false, false},
		{"FREQ=DAILY;UNTIL=19971224T000000Z", cdt("19971224T000000"), false, true},
	} {
		r := mustRule(t, c.in)
		if r.Until.DateTime != c.want || r.Until.IsDate != c.isDate || r.Until.UTC != c.utc {
			t.Errorf("%s: Until = %+v", c.in, r.Until)
		}
		if got := r.String(); got != c.in {
			t.Errorf("%s: String() = %s", c.in, got)
		}
	}
}

// ---- The MUST NOTs between rule parts ----

// The rules here are grammatical, so ParseRule takes them (it documents
// that "Constraints between parts (such as COUNT with UNTIL) are checked
// by Validate, not here"), and Validate must report each.
func TestConformanceValidateMustNot(t *testing.T) {
	for _, c := range []struct {
		in    string
		valid bool
	}{
		// "The UNTIL or COUNT rule parts are OPTIONAL, but they MUST NOT
		// occur in the same 'recur'."
		{"FREQ=DAILY;COUNT=3;UNTIL=19971224T000000Z", false},
		{"FREQ=DAILY;COUNT=3", true},
		{"FREQ=DAILY;UNTIL=19971224T000000Z", true},
		// "The BYDAY rule part MUST NOT be specified with a numeric value
		// when the FREQ rule part is not set to MONTHLY or YEARLY."
		{"FREQ=SECONDLY;BYDAY=1MO", false},
		{"FREQ=MINUTELY;BYDAY=1MO", false},
		{"FREQ=HOURLY;BYDAY=-1MO", false},
		{"FREQ=DAILY;BYDAY=1MO", false},
		{"FREQ=WEEKLY;BYDAY=2TU", false},
		{"FREQ=WEEKLY;BYDAY=TU", true},
		{"FREQ=MONTHLY;BYDAY=2TU", true},
		{"FREQ=YEARLY;BYDAY=20MO", true},
		// "Furthermore, the BYDAY rule part MUST NOT be specified with a
		// numeric value with the FREQ rule part set to YEARLY when the
		// BYWEEKNO rule part is specified." (errata 1913, 3779 keep it)
		{"FREQ=YEARLY;BYWEEKNO=20;BYDAY=1MO", false},
		{"FREQ=YEARLY;BYWEEKNO=20;BYDAY=MO", true},
		// "The BYMONTHDAY rule part MUST NOT be specified when the FREQ
		// rule part is set to WEEKLY."
		{"FREQ=WEEKLY;BYMONTHDAY=1", false},
		{"FREQ=DAILY;BYMONTHDAY=1", true},
		{"FREQ=SECONDLY;BYMONTHDAY=1", true},
		// "The BYYEARDAY rule part MUST NOT be specified when the FREQ rule
		// part is set to DAILY, WEEKLY, or MONTHLY."
		{"FREQ=DAILY;BYYEARDAY=1", false},
		{"FREQ=WEEKLY;BYYEARDAY=1", false},
		{"FREQ=MONTHLY;BYYEARDAY=1", false},
		{"FREQ=YEARLY;BYYEARDAY=1", true},
		{"FREQ=HOURLY;BYYEARDAY=1", true}, // the table: Limit
		// "This rule part [BYWEEKNO] MUST NOT be used when the FREQ rule
		// part is set to anything other than YEARLY."
		{"FREQ=SECONDLY;BYWEEKNO=1", false},
		{"FREQ=MINUTELY;BYWEEKNO=1", false},
		{"FREQ=HOURLY;BYWEEKNO=1", false},
		{"FREQ=DAILY;BYWEEKNO=1", false},
		{"FREQ=WEEKLY;BYWEEKNO=1", false},
		{"FREQ=MONTHLY;BYWEEKNO=1", false},
		{"FREQ=YEARLY;BYWEEKNO=1", true},
		// BYSETPOS "MUST only be used in conjunction with another BYxxx
		// rule part."
		{"FREQ=MONTHLY;BYSETPOS=1", false},
		{"FREQ=MONTHLY;COUNT=2;INTERVAL=2;WKST=SU;BYSETPOS=-1", false},
		{"FREQ=MONTHLY;BYSETPOS=-1;BYSECOND=0", true},
		{"FREQ=MONTHLY;BYDAY=MO,TU,WE,TH,FR;BYSETPOS=-1", true}, // the RFC's example
	} {
		r := mustRule(t, c.in)
		if err := r.Validate(); (err == nil) != c.valid {
			t.Errorf("%s: Validate() = %v; want valid %v", c.in, err, c.valid)
		}
	}
	// FREQ is REQUIRED; a Rule value can still lack it. A Rule built in Go
	// can also hold values the grammar's ranges don't allow, which String
	// would write out: Validate must catch those too.
	for _, r := range []recur.Rule{
		{Count: 1},
		{Freq: 8},
		{Freq: recur.Daily, Interval: -1},
		{Freq: recur.Daily, Count: -1},
		{Freq: recur.Daily, WeekStart: 7},
		{Freq: recur.Daily, BySecond: []int{61}},
		{Freq: recur.Daily, ByMinute: []int{60}},
		{Freq: recur.Daily, ByHour: []int{-1}},
		{Freq: recur.Monthly, ByMonthDay: []int{0}},
		{Freq: recur.Monthly, ByMonthDay: []int{-32}},
		{Freq: recur.Yearly, ByYearDay: []int{367}},
		{Freq: recur.Yearly, ByWeekNo: []int{54}},
		{Freq: recur.Yearly, ByMonth: []int{0}},
		{Freq: recur.Monthly, ByMonthDay: []int{1}, BySetPos: []int{0}},
		{Freq: recur.Monthly, ByDay: []recur.WeekdayNum{{N: 54, Day: recur.Monday}}},
		{Freq: recur.Monthly, ByDay: []recur.WeekdayNum{{Day: 7}}},
	} {
		if err := r.Validate(); err == nil {
			t.Errorf("%+v: Validate() = nil", r)
		}
	}
}

// "The value of the UNTIL rule part MUST have the same value type as the
// "DTSTART" property." With a floating DTSTART it is floating, with a UTC
// or zoned one it is UTC (erratum 4414), and with a DATE it is a DATE.
func TestConformanceUntilForm(t *testing.T) {
	t.Run("validate", func(t *testing.T) {
		t.Skip("KNOWN GAP: nothing checks UNTIL's form against DTSTART's: Rule.Validate has no DTSTART " +
			"and Set has no Validate. docs/architecture.md plans it (Roadmap 3, erratum 4414).")
	})
	ny := newYork(t)
	// Expansion compares each form as written (architecture doc: "UNTIL is
	// inclusive and compared in its own form"), so a mismatched UNTIL
	// still bounds the series predictably.
	for _, c := range []struct {
		name   string
		set    recur.Set
		until  string
		allDay bool
		want   int
	}{
		// A DATE UNTIL on a DATE-TIME series includes the whole day
		// (architecture doc, open question; dateutil reads midnight).
		{"DATE on DATE-TIME", recur.Set{Start: cdt("20240101T090000")}, "20240103", false, 3},
		// A UTC UNTIL on a floating series: floating times read as UTC.
		{"UTC on floating", recur.Set{Start: cdt("20240101T090000")}, "20240103T090000Z", false, 3},
		{"UTC on floating, before", recur.Set{Start: cdt("20240101T090000")}, "20240103T085959Z", false, 2},
		// A floating UNTIL on a zoned series: wall clocks.
		{"floating on zoned", recur.Set{Start: cdt("20240308T090000"), Location: ny}, "20240310T090000", false, 3},
		// A DATE-TIME UNTIL on a DATE series: instances are midnights.
		{"DATE-TIME on DATE", recur.Set{Start: cdt("20240101")}, "20240103T000000", true, 3},
		{"DATE-TIME on DATE, before", recur.Set{Start: cdt("20240101")}, "20240102T235959", true, 2},
	} {
		c.set.Rules = []recur.Rule{mustRule(t, "FREQ=DAILY;UNTIL="+c.until)}
		c.set.AllDay = c.allDay
		if got := all(c.set.All(), 10); len(got) != c.want {
			t.Errorf("%s: got %v, want %d instances", c.name, got, c.want)
		}
	}
}

func TestConformanceForever(t *testing.T) {
	// "If not present, and the COUNT rule part is also not present, the
	// "RRULE" is considered to repeat forever." Far from DTSTART: every
	// day of the window. (Iteration ends at year 9999, the last year a
	// DATE-TIME can be written in; architecture doc.)
	r := mustRule(t, "FREQ=DAILY")
	set := &recur.Set{Start: cdt("19970902T090000"), Rules: []recur.Rule{r}}
	got := all(set.Between(cdt("25000101T000000"), cdt("25000104T000000")), 10)
	if want := cdts("25000101T090000", "25000102T090000", "25000103T090000"); !slices.Equal(got, want) {
		t.Errorf("year 2500: got %v, want %v", got, want)
	}
	got = all(set.Between(cdt("99991230T000000"), cdt("99991231T235959")), 10)
	if want := cdts("99991230T090000", "99991231T090000"); !slices.Equal(got, want) {
		t.Errorf("year 9999: got %v, want %v", got, want)
	}
}

// The engine gives the forbidden combinations a meaning (architecture doc,
// "rules that break a MUST NOT still expand predictably"). These check
// that meaning; the RFC itself defines no result for them.
func TestConformanceMustNotExpansion(t *testing.T) {
	for _, c := range []struct {
		start, rule string
		want        []civil.DateTime
	}{
		// A numbered BYDAY outside MONTHLY/YEARLY loses its number:
		// every Monday. 1997-09-01 is a Monday.
		{"19970901T090000", "FREQ=DAILY;BYDAY=1MO;COUNT=3",
			cdts("19970901T090000", "19970908T090000", "19970915T090000")},
		{"19970901T090000", "FREQ=WEEKLY;BYDAY=-1MO;COUNT=2",
			cdts("19970901T090000", "19970908T090000")},
		// N/A day parts limit the period's days: every 23rd, not only the
		// DTSTART weekday's.
		{"20130331T090000", "FREQ=WEEKLY;BYMONTHDAY=23;COUNT=3",
			cdts("20130331T090000", "20130423T090000", "20130523T090000")},
		// BYYEARDAY=1 in MONTHLY: January 1 each year.
		{"20240101T090000", "FREQ=MONTHLY;BYYEARDAY=1;COUNT=2",
			cdts("20240101T090000", "20250101T090000")},
		// BYWEEKNO=1 in DAILY: the days of ISO week 1. Week 1 of 2025 is
		// 2024-12-30 to 2025-01-05; of 2026, 2025-12-29 to 2026-01-04.
		{"20241229T090000", "FREQ=DAILY;BYWEEKNO=1;COUNT=9",
			cdts("20241229T090000", "20241230T090000", "20241231T090000", "20250101T090000",
				"20250102T090000", "20250103T090000", "20250104T090000", "20250105T090000",
				"20251229T090000")},
		// BYSETPOS alone picks from the set DTSTART's defaults make: one
		// day a month, so BYSETPOS=1 keeps it and BYSETPOS=2 drops it.
		{"20240115T090000", "FREQ=MONTHLY;BYSETPOS=1;COUNT=3",
			cdts("20240115T090000", "20240215T090000", "20240315T090000")},
		{"20240115T090000", "FREQ=MONTHLY;BYSETPOS=2;COUNT=3", cdts("20240115T090000")},
	} {
		set := &recur.Set{Start: cdt(c.start), Rules: []recur.Rule{mustRule(t, c.rule)}}
		if got := all(set.All(), 20); !slices.Equal(got, c.want) {
			t.Errorf("DTSTART:%s RRULE:%s\n got %v\nwant %v", c.start, c.rule, got, c.want)
		}
	}
}

// COUNT with UNTIL breaks a MUST NOT. Both bounds can only be honoured
// together; check the result never goes past either.
func TestConformanceCountAndUntil(t *testing.T) {
	for _, s := range []string{
		"FREQ=DAILY;COUNT=3;UNTIL=19970910T090000",  // COUNT ends first
		"FREQ=DAILY;COUNT=30;UNTIL=19970904T090000", // UNTIL ends first
	} {
		r := mustRule(t, s)
		set := &recur.Set{Start: cdt("19970902T090000"), Rules: []recur.Rule{r}}
		got := all(set.All(), 100)
		want := cdts("19970902T090000", "19970903T090000", "19970904T090000")
		if !slices.Equal(got, want) {
			t.Errorf("%s: got %v, want %v", s, got, want)
		}
	}
}

// ---- Rule.String and the grammar ----

// recurGrammar matches one recur-rule-part of the §3.3.10 ABNF, written as
// regular expressions with the digit counts the ABNF gives.
var recurGrammar = map[string]*regexp.Regexp{
	"FREQ":       regexp.MustCompile(`^(SECONDLY|MINUTELY|HOURLY|DAILY|WEEKLY|MONTHLY|YEARLY)$`),
	"UNTIL":      regexp.MustCompile(`^[0-9]{8}(T[0-9]{6}Z?)?$`),
	"COUNT":      regexp.MustCompile(`^[0-9]+$`),
	"INTERVAL":   regexp.MustCompile(`^[0-9]+$`),
	"BYSECOND":   regexp.MustCompile(`^[0-9]{1,2}(,[0-9]{1,2})*$`),
	"BYMINUTE":   regexp.MustCompile(`^[0-9]{1,2}(,[0-9]{1,2})*$`),
	"BYHOUR":     regexp.MustCompile(`^[0-9]{1,2}(,[0-9]{1,2})*$`),
	"BYDAY":      regexp.MustCompile(`^(([+-]?[0-9]{1,2})?(SU|MO|TU|WE|TH|FR|SA))(,([+-]?[0-9]{1,2})?(SU|MO|TU|WE|TH|FR|SA))*$`),
	"BYMONTHDAY": regexp.MustCompile(`^[+-]?[0-9]{1,2}(,[+-]?[0-9]{1,2})*$`),
	"BYYEARDAY":  regexp.MustCompile(`^[+-]?[0-9]{1,3}(,[+-]?[0-9]{1,3})*$`),
	"BYWEEKNO":   regexp.MustCompile(`^[+-]?[0-9]{1,2}(,[+-]?[0-9]{1,2})*$`),
	"BYMONTH":    regexp.MustCompile(`^[0-9]{1,2}(,[0-9]{1,2})*$`),
	"BYSETPOS":   regexp.MustCompile(`^[+-]?[0-9]{1,3}(,[+-]?[0-9]{1,3})*$`),
	"WKST":       regexp.MustCompile(`^(SU|MO|TU|WE|TH|FR|SA)$`),
}

// checkRecurGrammar returns why s isn't a RECUR value String should write,
// or "".
func checkRecurGrammar(s string) string {
	seen := map[string]bool{}
	for i, part := range strings.Split(s, ";") {
		name, value, ok := strings.Cut(part, "=")
		re := recurGrammar[name]
		switch {
		case !ok || re == nil:
			return fmt.Sprintf("%q is not a recur-rule-part", part)
		case seen[name]:
			return name + " occurs twice; rule parts \"MUST NOT occur more than once\""
		case i == 0 && name != "FREQ":
			return "\"the FREQ rule part MUST be the first rule part specified in a RECUR value\""
		case !re.MatchString(value):
			return fmt.Sprintf("%s=%s doesn't match the ABNF", name, value)
		}
		seen[name] = true
	}
	if !seen["FREQ"] {
		return "no FREQ"
	}
	return ""
}

func TestConformanceRuleStringGrammar(t *testing.T) {
	check := func(r recur.Rule) {
		t.Helper()
		s := r.String()
		if why := checkRecurGrammar(s); why != "" {
			t.Errorf("String() = %q: %s", s, why)
			return
		}
		back, err := recur.ParseRule(s)
		if err != nil {
			t.Errorf("ParseRule(String() = %q): %v", s, err)
			return
		}
		if !back.Equal(r) || back.String() != s {
			t.Errorf("%q reads back as %q", s, back.String())
		}
	}
	for i := range 3000 {
		check(genCase(7, i).rule)
	}
	// Written forms the grammar allows, which String may normalize.
	for _, s := range []string{
		"freq=yearly;byday=+1mo,-1fr;bymonth=1;until=20301231t235959z",
		"BYSETPOS=-1;FREQ=MONTHLY;BYDAY=MO,TU,WE,TH,FR",
		"FREQ=YEARLY;BYWEEKNO=+1,-53;BYYEARDAY=+366;BYMONTHDAY=+31;WKST=SU",
		"FREQ=WEEKLY;INTERVAL=1;WKST=MO;BYDAY=TU",
		"FREQ=SECONDLY;BYSECOND=60",
	} {
		check(mustRule(t, s))
	}
}

// ---- DATE DTSTART ----

func TestConformanceDateStartIgnoresTimeParts(t *testing.T) {
	// "The BYSECOND, BYMINUTE and BYHOUR rule parts MUST NOT be specified
	// when the associated "DTSTART" property has a DATE value type. These
	// rule parts MUST be ignored in RECUR value that violate the above
	// requirement". So BYHOUR=10,12 neither moves nor multiplies the days.
	r := mustRule(t, "FREQ=DAILY;BYHOUR=10,12;BYMINUTE=30;BYSECOND=15;COUNT=3")
	set := &recur.Set{Start: cdt("20240101"), Rules: []recur.Rule{r}, AllDay: true}
	if got, want := all(set.All(), 10), cdts("20240101", "20240102", "20240103"); !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	// With BYSETPOS the ignored parts don't make the day's set bigger:
	// one instance a week, the Monday, and BYSETPOS=-1 keeps it.
	r = mustRule(t, "FREQ=WEEKLY;BYDAY=MO;BYHOUR=1,2,3;BYSETPOS=-1;COUNT=2")
	set = &recur.Set{Start: cdt("20240101"), Rules: []recur.Rule{r}, AllDay: true}
	if got, want := all(set.All(), 10), cdts("20240101", "20240108"); !slices.Equal(got, want) {
		t.Errorf("BYSETPOS: got %v, want %v", got, want)
	}
	// "If the "DTSTART" property is specified as a date ..." the UNTIL
	// "MUST have the same value type": a DATE, compared as a date and
	// inclusive.
	r = mustRule(t, "FREQ=DAILY;UNTIL=20240103")
	set = &recur.Set{Start: cdt("20240101"), Rules: []recur.Rule{r}, AllDay: true}
	if got, want := all(set.All(), 10), cdts("20240101", "20240102", "20240103"); !slices.Equal(got, want) {
		t.Errorf("DATE UNTIL: got %v, want %v", got, want)
	}
}

// ---- WKST ----

func TestConformanceWeekStart(t *testing.T) {
	// The §3.8.5.3 WKST example. With WKST=MO the weeks are Mon-Sun: Aug
	// 4-10 holds TU 5 and SU 10; INTERVAL=2 skips Aug 11-17; Aug 18-24
	// holds TU 19 and SU 24. With WKST=SU the weeks are Sun-Sat: Aug 3-9
	// holds SU 3 (before DTSTART) and TU 5; Aug 17-23 holds SU 17 and TU
	// 19; Aug 31-Sep 6 holds SU 31.
	start := cdt("19970805T090000")
	for _, c := range []struct {
		rule string
		want []civil.DateTime
	}{
		{"FREQ=WEEKLY;INTERVAL=2;COUNT=4;BYDAY=TU,SU;WKST=MO",
			cdts("19970805T090000", "19970810T090000", "19970819T090000", "19970824T090000")},
		{"FREQ=WEEKLY;INTERVAL=2;COUNT=4;BYDAY=TU,SU;WKST=SU",
			cdts("19970805T090000", "19970817T090000", "19970819T090000", "19970831T090000")},
	} {
		set := &recur.Set{Start: start, Rules: []recur.Rule{mustRule(t, c.rule)}}
		if got := all(set.All(), 10); !slices.Equal(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.rule, got, c.want)
		}
	}
	// WKST "is also significant when in a YEARLY "RRULE" when a BYWEEKNO
	// rule part is specified". 2026 starts on a Thursday. Weeks starting
	// Monday: Dec 29-Jan 4 has four 2026 days, so it is week 1 and its
	// Monday is 2025-12-29. Weeks starting Sunday: Dec 28-Jan 3 has three,
	// so week 1 is Jan 4-10 and its Monday is 2026-01-05.
	for wkst, want := range map[string]civil.DateTime{"MO": cdt("20251229T090000"), "SU": cdt("20260105T090000")} {
		r := mustRule(t, "FREQ=YEARLY;BYWEEKNO=1;BYDAY=MO;WKST="+wkst)
		got := all(r.Between(cdt("20250601T090000"), cdt("20250601T090000"), cdt("20270101T000000")), 1)
		if len(got) != 1 || got[0] != want {
			t.Errorf("WKST=%s: week 1 Monday %v, want %v", wkst, got, want)
		}
	}
}

func TestConformanceWeek53(t *testing.T) {
	// "Note: Assuming a Monday week start, week 53 can only occur when
	// Thursday is January 1 or if it is a leap year and Wednesday is
	// January 1." And conversely each such year has one: from week 1's
	// Monday to the next year's is 53 weeks then.
	r := mustRule(t, "FREQ=YEARLY;BYWEEKNO=53;BYDAY=TH")
	has := map[int]bool{}
	for d := range r.Between(cdt("19000104T000000"), cdt("19000104T000000"), cdt("21010101T000000")) {
		// A Thursday is in the year its week is numbered in.
		has[d.Year] = true
	}
	for y := 1900; y <= 2100; y++ {
		jan1 := time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC).Weekday()
		leap := time.Date(y, 12, 31, 0, 0, 0, 0, time.UTC).YearDay() == 366
		if want := jan1 == time.Thursday || leap && jan1 == time.Wednesday; has[y] != want {
			t.Errorf("%d: week 53 %v, want %v (Jan 1 a %v, leap %v)", y, has[y], want, jan1, leap)
		}
	}
	// -1 is the last week, 53 or 52: 2020 has 53 (Jan 1 a Wednesday, leap),
	// so BYWEEKNO=-1 and 53 agree there; 2021 has 52.
	r = mustRule(t, "FREQ=YEARLY;BYWEEKNO=-1;BYDAY=MO")
	got := all(r.Between(cdt("20200101T000000"), cdt("20200101T000000"), cdt("20230101T000000")), 5)
	if want := cdts("20201228T000000", "20211227T000000", "20221226T000000"); !slices.Equal(got, want) {
		t.Errorf("BYWEEKNO=-1: got %v, want %v", got, want)
	}
}

// ---- Invalid dates (erratum 4271) ----

func TestConformanceInvalidDatesNotCounted(t *testing.T) {
	// "Recurrence rules may generate recurrence instances with an invalid
	// date (e.g., February 30) ... Such recurrence instances MUST be
	// ignored and MUST NOT be counted as part of the recurrence set."
	for _, c := range []struct {
		start, rule string
		want        []civil.DateTime
	}{
		// Jan 30, (Feb 30), Mar 30, Apr 30.
		{"20070130T090000", "FREQ=MONTHLY;COUNT=3",
			cdts("20070130T090000", "20070330T090000", "20070430T090000")},
		// Months with a 31st: Jan, Mar, May.
		{"20070131T090000", "FREQ=MONTHLY;COUNT=3",
			cdts("20070131T090000", "20070331T090000", "20070531T090000")},
		// Feb 29 every year exists only in leap years, and 2100 isn't one.
		{"20880229T090000", "FREQ=YEARLY;COUNT=4",
			cdts("20880229T090000", "20920229T090000", "20960229T090000", "21040229T090000")},
		// BYMONTHDAY=-1 is always a valid date; BYMONTHDAY=31 is not.
		{"20070131T090000", "FREQ=MONTHLY;BYMONTHDAY=-1;COUNT=3",
			cdts("20070131T090000", "20070228T090000", "20070331T090000")},
		// BYYEARDAY=366 only in leap years.
		{"20231231T090000", "FREQ=YEARLY;BYYEARDAY=366;COUNT=3",
			cdts("20231231T090000", "20241231T090000", "20281231T090000")},
		// The 5th Friday of a month exists in some months only: 2024 has
		// them in March, May, August, November.
		{"20240101T090000", "FREQ=MONTHLY;BYDAY=5FR;COUNT=4",
			cdts("20240101T090000", "20240329T090000", "20240531T090000", "20240830T090000")},
	} {
		set := &recur.Set{Start: cdt(c.start), Rules: []recur.Rule{mustRule(t, c.rule)}}
		if got := all(set.All(), 20); !slices.Equal(got, c.want) {
			t.Errorf("DTSTART:%s RRULE:%s\n got %v\nwant %v", c.start, c.rule, got, c.want)
		}
	}
}

// BYSECOND=60 is in the grammar ("0 to 60"), but the wall clock the rules run
// on has no leap seconds, so no instance can be at second 60; the engine
// drops them like an invalid date. The other seconds are unaffected.
func TestConformanceLeapSecond(t *testing.T) {
	r := mustRule(t, "FREQ=MINUTELY;BYSECOND=59,60;COUNT=3")
	set := &recur.Set{Start: cdt("20161231T235859"), Rules: []recur.Rule{r}}
	if got, want := all(set.All(), 10), cdts("20161231T235859", "20161231T235959", "20170101T000059"); !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	r = mustRule(t, "FREQ=MINUTELY;BYSECOND=60")
	set = &recur.Set{Start: cdt("20161231T235800"), Rules: []recur.Rule{r}}
	if got, want := all(set.All(), 10), cdts("20161231T235800"); !slices.Equal(got, want) {
		t.Errorf("only second 60: got %v, want %v", got, want)
	}
}

// ---- UNTIL ----

func TestConformanceUntilInclusive(t *testing.T) {
	// "The UNTIL rule part defines a DATE or DATE-TIME value that bounds
	// the recurrence rule in an inclusive manner. If the value specified
	// by UNTIL is synchronized with the specified recurrence, this DATE or
	// DATE-TIME becomes the last instance". Floating DTSTART, floating
	// UNTIL (erratum 4414).
	start := cdt("20240101T090000")
	for _, c := range []struct {
		until string
		n     int
	}{
		{"20240103T090000", 3},
		{"20240103T085959", 2},
		{"20240103T090001", 3},
		{"20231231T090000", 1}, // DTSTART is still the first instance
	} {
		r := mustRule(t, "FREQ=DAILY;UNTIL="+c.until)
		set := &recur.Set{Start: start, Rules: []recur.Rule{r}}
		if got := all(set.All(), 10); len(got) != c.n {
			t.Errorf("UNTIL=%s: got %v, want %d instances", c.until, got, c.n)
		}
	}
}

func TestConformanceUntilUTCWithZone(t *testing.T) {
	// "If the "DTSTART" property is specified as ... a date with local
	// time and time zone reference, then the UNTIL rule part MUST be
	// specified as a date with UTC time." The UTC UNTIL bounds the
	// instants, which are found on the local clock and resolved as
	// §3.3.5 says ("If the computed local start time of a recurrence
	// instance does not exist, or occurs more than once ... interpreted
	// in the same manner as an explicit DATE-TIME value").
	ny := newYork(t)
	for _, c := range []struct {
		start, rule string
		want        []string // UTC
	}{
		// 01:30 on 2007-11-04 occurs twice; the first is 01:30 EDT =
		// 05:30Z, which UNTIL=05:30Z includes and 05:29:59Z excludes.
		{"20071103T013000", "FREQ=DAILY;UNTIL=20071104T053000Z",
			[]string{"2007-11-03T05:30:00Z", "2007-11-04T05:30:00Z"}},
		{"20071103T013000", "FREQ=DAILY;UNTIL=20071104T052959Z",
			[]string{"2007-11-03T05:30:00Z"}},
		// 02:30 on 2007-03-11 doesn't exist; with the offset before the
		// gap (EST) it is 07:30Z, the same as 02:30 EST the day before.
		{"20070310T023000", "FREQ=DAILY;UNTIL=20070311T073000Z",
			[]string{"2007-03-10T07:30:00Z", "2007-03-11T07:30:00Z"}},
		{"20070310T023000", "FREQ=DAILY;UNTIL=20070311T072959Z",
			[]string{"2007-03-10T07:30:00Z"}},
		// UNTIL bounds every instance, not the walk: on the wall clock the
		// rule gives 01:40, 02:05, 02:30, 02:55, 03:20, 03:45; the three in
		// the gap are 07:05Z, 07:30Z and 07:55Z, and 03:20 and 03:45 EDT
		// (07:20Z, 07:45Z) are within UNTIL though 02:55 isn't.
		{"20070311T014000", "FREQ=MINUTELY;INTERVAL=25;UNTIL=20070311T075000Z",
			[]string{"2007-03-11T06:40:00Z", "2007-03-11T07:05:00Z", "2007-03-11T07:20:00Z", "2007-03-11T07:30:00Z", "2007-03-11T07:45:00Z"}},
		// An instant compare, not a wall-clock one: 09:00 EDT is 13:00Z,
		// after UNTIL=12:00Z although the UTC wall clock reads 09:00 < 12.
		{"20240601T090000", "FREQ=DAILY;UNTIL=20240602T120000Z",
			[]string{"2024-06-01T13:00:00Z"}},
	} {
		set := &recur.Set{Start: cdt(c.start), Location: ny, Rules: []recur.Rule{mustRule(t, c.rule)}}
		var got []string
		for tm := range set.Times(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)) {
			got = append(got, tm.UTC().Format(time.RFC3339))
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("DTSTART;TZID=America/New_York:%s RRULE:%s\n got %v\nwant %v", c.start, c.rule, got, c.want)
		}
	}
}

// ---- §3.8.5: the recurrence set ----

func TestConformanceRecurrenceSet(t *testing.T) {
	daily3 := mustRule(t, "FREQ=DAILY;COUNT=3")
	for _, c := range []struct {
		name string
		set  recur.Set
		want []civil.DateTime
	}{{
		// "the recurrence instances are defined by the union of
		// occurrences defined by both the "RDATE" and "RRULE"" and "When
		// duplicate instances are generated by the "RRULE" and "RDATE"
		// properties, only one recurrence is considered."
		name: "RDATE union, duplicates once",
		set: recur.Set{Start: cdt("20240101T090000"), Rules: []recur.Rule{daily3},
			RDates: cdts("20240102T090000", "20240110T090000", "20240101T120000", "20240110T090000")},
		want: cdts("20240101T090000", "20240101T120000", "20240102T090000", "20240103T090000", "20240110T090000"),
	}, {
		// "excluding any start DATE-TIME values specified by "EXDATE"
		// properties ... EXDATE ... take precedence over those specified
		// by inclusion properties (i.e., "RDATE" and "RRULE")". COUNT
		// belongs to the RRULE, so an excluded instance still used up one.
		name: "EXDATE over RRULE and RDATE",
		set: recur.Set{Start: cdt("20240101T090000"), Rules: []recur.Rule{daily3},
			RDates:  cdts("20240110T090000"),
			ExDates: cdts("20240102T090000", "20240110T090000", "20240105T090000")},
		want: cdts("20240101T090000", "20240103T090000"),
	}, {
		// "The "EXDATE" property can be used to exclude the value
		// specified in "DTSTART"."
		name: "EXDATE of DTSTART",
		set: recur.Set{Start: cdt("20240101T090000"), Rules: []recur.Rule{daily3},
			ExDates: cdts("20240101T090000")},
		want: cdts("20240102T090000", "20240103T090000"),
	}, {
		// "The "DTSTART" property defines the first instance in the
		// recurrence set", and "always counts as the first occurrence"
		// (§3.3.10 COUNT). Not synchronized with BYDAY=MO (2024-01-03 is
		// a Wednesday): the RFC calls that set undefined; the
		// architecture doc keeps DTSTART first and counted.
		name: "DTSTART not synchronized",
		set:  recur.Set{Start: cdt("20240103T090000"), Rules: []recur.Rule{mustRule(t, "FREQ=WEEKLY;BYDAY=MO;COUNT=3")}},
		want: cdts("20240103T090000", "20240108T090000", "20240115T090000"),
	}, {
		// An RDATE isn't part of the RRULE, so it doesn't use up COUNT.
		name: "RDATE outside COUNT",
		set: recur.Set{Start: cdt("20240101T090000"), Rules: []recur.Rule{daily3},
			RDates: cdts("20240101T100000")},
		want: cdts("20240101T090000", "20240101T100000", "20240102T090000", "20240103T090000"),
	}, {
		// No RRULE: DTSTART and the RDATEs.
		name: "RDATE only",
		set:  recur.Set{Start: cdt("20240101T090000"), RDates: cdts("20240301T090000", "20240201T090000")},
		want: cdts("20240101T090000", "20240201T090000", "20240301T090000"),
	}} {
		if got := all(c.set.All(), 20); !slices.Equal(got, c.want) {
			t.Errorf("%s:\n got %v\nwant %v", c.name, got, c.want)
		}
	}
}

func TestConformanceRecurrenceSetZoned(t *testing.T) {
	ny := newYork(t)
	utc := func(s string) time.Time {
		v, err := time.Parse("20060102T150405Z", s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	times := func(s *recur.Set) []string {
		var out []string
		for tm := range s.Times(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)) {
			out = append(out, tm.UTC().Format("20060102T150405Z"))
		}
		return out
	}
	daily := mustRule(t, "FREQ=DAILY;COUNT=5")

	// An EXDATE written in UTC names an instant (§3.3.5 form 2). The series
	// is 09:00 New York time: EST (14:00Z) to March 9, EDT (13:00Z) from
	// March 10, 2024.
	s := &recur.Set{Start: cdt("20240308T090000"), Location: ny, Rules: []recur.Rule{daily},
		ExTimes: []time.Time{utc("20240311T130000Z"), utc("20240310T140000Z")}}
	// 20240310T140000Z is 10:00 EDT, which is no instance: nothing to
	// exclude. 20240311T130000Z is 09:00 EDT on the 11th.
	if got, want := times(s), []string{"20240308T140000Z", "20240309T140000Z", "20240310T130000Z", "20240312T130000Z"}; !slices.Equal(got, want) {
		t.Errorf("EXDATE in UTC:\n got %v\nwant %v", got, want)
	}

	// An RDATE in UTC that is an RRULE instance is a duplicate: "only one
	// recurrence is considered".
	s = &recur.Set{Start: cdt("20240308T090000"), Location: ny, Rules: []recur.Rule{daily},
		RTimes: []time.Time{utc("20240310T130000Z"), utc("20240320T130000Z")}}
	if got, want := times(s), []string{"20240308T140000Z", "20240309T140000Z", "20240310T130000Z",
		"20240311T130000Z", "20240312T130000Z", "20240320T130000Z"}; !slices.Equal(got, want) {
		t.Errorf("RDATE in UTC:\n got %v\nwant %v", got, want)
	}
}

func TestConformanceDSTGap(t *testing.T) {
	// Erratum 4271: an instance at a nonexistent local time is kept and
	// counted, and resolved as §3.3.5 says: "If the local time described
	// does not occur ... the DATE-TIME value is interpreted using the UTC
	// offset before the gap in local times. Thus, TZID=America/
	// New_York:20070311T023000 indicates March 11, 2007 at 3:30 A.M. EDT".
	// An invalid date is still dropped and not counted.
	ny := newYork(t)
	for _, c := range []struct {
		start, rule string
		want        []string // New York wall clock with zone
	}{
		{"20070310T023000", "FREQ=DAILY;COUNT=3",
			[]string{"2007-03-10 02:30 EST", "2007-03-11 03:30 EDT", "2007-03-12 02:30 EDT"}},
		{"20060311T023000", "FREQ=YEARLY;COUNT=3",
			[]string{"2006-03-11 02:30 EST", "2007-03-11 03:30 EDT", "2008-03-11 02:30 EDT"}},
		// Feb 30 isn't counted; Mar 11 02:30 is.
		{"20070130T023000", "FREQ=MONTHLY;BYMONTHDAY=11,30;COUNT=4",
			[]string{"2007-01-30 02:30 EST", "2007-02-11 02:30 EST", "2007-03-11 03:30 EDT", "2007-03-30 02:30 EDT"}},
		// The repeated hour: 01:30 on 2007-11-04 is the first, EDT.
		{"20071103T013000", "FREQ=DAILY;COUNT=3",
			[]string{"2007-11-03 01:30 EDT", "2007-11-04 01:30 EDT", "2007-11-05 01:30 EST"}},
	} {
		s := &recur.Set{Start: cdt(c.start), Location: ny, Rules: []recur.Rule{mustRule(t, c.rule)}}
		var got []string
		for tm := range s.Times(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)) {
			got = append(got, tm.In(ny).Format("2006-01-02 15:04 MST"))
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("DTSTART;TZID=America/New_York:%s RRULE:%s\n got %v\nwant %v", c.start, c.rule, got, c.want)
		}
	}
}

// A sanity check of the helpers the differential test shares: the
// reference itself must reproduce instances the RFC prints, so that a
// disagreement points at the library.
func TestConformanceReferenceSelfCheck(t *testing.T) {
	for _, c := range []struct {
		start, rule string
		want        []civil.DateTime
	}{
		// §3.8.5.3 "Monthly on the first and last day of the month for 10
		// occurrences" (first six).
		{"19970930T090000", "FREQ=MONTHLY;COUNT=6;BYMONTHDAY=1,-1", cdts("19970930T090000", "19971001T090000",
			"19971031T090000", "19971101T090000", "19971130T090000", "19971201T090000")},
		// §3.8.5.3 "Monday of week number 20 (where the default start of
		// the week is Monday), forever" (first three).
		{"19970512T090000", "FREQ=YEARLY;BYWEEKNO=20;BYDAY=MO", cdts("19970512T090000", "19980511T090000", "19990517T090000")},
		// §3.8.5.3 "The third instance into the month of one of Tuesday,
		// Wednesday, or Thursday, for the next 3 months".
		{"19970904T090000", "FREQ=MONTHLY;COUNT=3;BYDAY=TU,WE,TH;BYSETPOS=3",
			cdts("19970904T090000", "19971007T090000", "19971106T090000")},
		// §3.8.5.3 "Every 20th Monday of the year, forever" (first three).
		{"19970519T090000", "FREQ=YEARLY;BYDAY=20MO", cdts("19970519T090000", "19980518T090000", "19990517T090000")},
	} {
		r := mustRule(t, c.rule)
		x := newRef(r, cdt(c.start), false)
		raw, _ := x.expand(fromCivil(cdt(c.start).AddDate(5, 0, 0)), 50)
		var got []civil.DateTime
		for _, tm := range x.finish(raw, true) {
			got = append(got, toCivil(tm))
		}
		if len(got) > len(c.want) {
			got = got[:len(c.want)]
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("reference: %s from %s\n got %v\nwant %v", c.rule, c.start, got, c.want)
		}
	}
}
