package recur

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/aanelson/iCalendar/civil"
	"github.com/aanelson/iCalendar/internal/icaltime"
)

// The recurrence vectors: RFC 5545's own examples, and test cases converted
// from other libraries' suites (testdata/thirdparty, a nested module that
// may be absent). See testdata/thirdparty/README.md for the format.

type vectorFile struct {
	Source    string   `json:"source"`
	Semantics string   `json:"semantics"`
	Cases     []vector `json:"cases"`
}

type vector struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	DTStart        string   `json:"dtstart"`
	TZID           string   `json:"tzid"`
	RRule          []string `json:"rrule"`
	ExRule         []string `json:"exrule"`
	RDate          []string `json:"rdate"`
	ExDate         []string `json:"exdate"`
	IncludeDTStart bool     `json:"include_dtstart"`
	Between        *struct {
		Start     string `json:"start"`
		End       string `json:"end"`
		Inclusive bool   `json:"inclusive"`
	} `json:"between"`
	Limit      int      `json:"limit"`
	Expected   []string `json:"expected"`
	Error      bool     `json:"error"`
	Extensions []string `json:"extensions"`
	Comment    string   `json:"comment"`
}

func vectorFiles(t *testing.T) []string {
	files := []string{"../testdata/rfc5545/recur.json"}
	more, _ := filepath.Glob("../testdata/thirdparty/*/recur.json")
	return append(files, more...)
}

func loadVectors(t *testing.T, path string) vectorFile {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("no vectors: %v", err)
	}
	var f vectorFile
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return f
}

func TestVectors(t *testing.T) {
	for _, path := range vectorFiles(t) {
		name := filepath.Base(filepath.Dir(path))
		t.Run(name, func(t *testing.T) {
			f := loadVectors(t, path)
			known := knownDifferences[name]
			var pass, fail, skipped, expectedFail int
			for _, v := range f.Cases {
				ok := t.Run(v.ID, func(t *testing.T) {
					// RFC 7529 that ParseRule reads as RFC 5545 itself
					// (RSCALE=GREGORIAN, SKIP=OMIT) runs; every other
					// extension is skipped.
					if len(v.Extensions) > 0 && !(onlyRScale(v.Extensions) && parsesAll(v.RRule, v.ExRule)) {
						skipped++
						t.Skipf("uses %v, beyond RFC 5545", v.Extensions)
					}
					got, err := expandVector(v)
					if reason, isKnown := known[v.ID]; isKnown {
						if err == nil && !v.Error && slices.Equal(got, v.Expected) {
							t.Errorf("listed as a known difference (%s) but now matches", reason)
							return
						}
						expectedFail++
						t.Skipf("known difference: %s", reason)
					}
					switch {
					case v.Error && err == nil:
						t.Errorf("want an error, got %v", got)
					case !v.Error && err != nil:
						t.Errorf("unexpected error: %v", err)
					case !v.Error && !slices.Equal(got, v.Expected):
						t.Errorf("%s\nDTSTART %s %s\nRRULE %v RDATE %v EXDATE %v EXRULE %v between %+v limit %d\n got  %v\n want %v\n%s",
							v.Name, v.TZID, v.DTStart, v.RRule, v.RDate, v.ExDate, v.ExRule, v.Between, v.Limit, got, v.Expected, v.Comment)
					}
				})
				if ok {
					pass++
				} else {
					fail++
				}
			}
			t.Logf("%s: %d cases, %d failed, %d skipped for extensions, %d known differences",
				name, len(f.Cases), fail, skipped, expectedFail)
			_ = pass
		})
	}
}

func onlyRScale(extensions []string) bool {
	for _, e := range extensions {
		if e != "RSCALE" && e != "SKIP" {
			return false
		}
	}
	return true
}

func parsesAll(lists ...[]string) bool {
	for _, list := range lists {
		for _, s := range list {
			if _, err := ParseRule(s); err != nil {
				return false
			}
		}
	}
	return true
}

// expandVector computes a vector's instances, formatted as its expected
// list is.
func expandVector(v vector) ([]string, error) {
	start, isDate, utc, err := icaltime.Parse(v.DTStart)
	if err != nil {
		return nil, fmt.Errorf("DTSTART: %w", err)
	}
	var loc *time.Location
	if v.TZID != "" {
		if loc, err = time.LoadLocation(v.TZID); err != nil {
			return nil, err
		}
	} else if utc {
		loc = time.UTC
	}
	var rules, exRules []Rule
	for _, s := range v.RRule {
		r, err := ParseRule(s)
		if err != nil {
			return nil, err
		}
		if err := r.Validate(); err != nil && v.Error {
			return nil, err
		}
		rules = append(rules, r)
	}
	for _, s := range v.ExRule {
		r, err := ParseRule(s)
		if err != nil {
			return nil, err
		}
		exRules = append(exRules, r)
	}
	times := func(list []string) ([]civil.DateTime, error) {
		var out []civil.DateTime
		for _, s := range list {
			t, _, tUTC, err := icaltime.Parse(s)
			if err != nil {
				return nil, err
			}
			if tUTC && !utc && loc != nil {
				t = civil.DateTimeOf(t.In(time.UTC).In(loc))
			}
			out = append(out, t)
		}
		return out, nil
	}
	if v.Error && utc && v.TZID != "" {
		return nil, errors.New("a UTC DTSTART can't have a TZID")
	}
	if v.Error {
		// A rule that can never produce an instance yields nothing here;
		// several libraries raise an error for it instead.
		empty := len(rules) > 0
		for _, r := range rules {
			empty = empty && compile(r, start, isDate).empty
		}
		if empty {
			return nil, errors.New("the rule can never produce an instance")
		}
	}
	set := &Set{Start: start, Location: loc, Rules: rules, ExRules: exRules, AllDay: isDate, omitStart: !v.IncludeDTStart}
	if set.RDates, err = times(v.RDate); err != nil {
		return nil, err
	}
	if set.ExDates, err = times(v.ExDate); err != nil {
		return nil, err
	}
	from := start
	to := civil.DateTimeOfEpochSecond(lastSecond + 1)
	if b := v.Between; b != nil {
		// Either end may be open, for after() and before() queries.
		if b.Start != "" {
			if from, _, _, err = icaltime.Parse(b.Start); err != nil {
				return nil, err
			}
			if !b.Inclusive {
				from = from.AddSeconds(1)
			}
		}
		if b.End != "" {
			if to, _, _, err = icaltime.Parse(b.End); err != nil {
				return nil, err
			}
			if b.Inclusive {
				to = to.AddSeconds(1)
			}
		}
	}
	// Sources write the instances of a DATE start either as dates or as
	// midnight date-times; match theirs.
	asDate := isDate && (len(v.Expected) == 0 || len(v.Expected[0]) == 8)
	limit := v.Limit
	if limit == 0 && v.Between == nil {
		// A safety net for an unbounded vector the converter didn't limit.
		limit = max(len(v.Expected)+1, 1)
		if len(v.Expected) > 0 {
			limit = len(v.Expected) + 5
		}
	}
	out := []string{}
	for t := range set.Between(from, to) {
		if loc != nil && loc != time.UTC && !isDate {
			// Sources list the wall clock an instance resolves to, so a
			// time in a DST gap appears moved forward (RFC 5545 §3.3.5).
			t = civil.DateTimeOf(t.In(loc))
		}
		out = append(out, icaltime.Format(t, asDate, utc))
		if limit > 0 && len(out) == limit {
			break
		}
	}
	return out, nil
}

// knownDifferences lists vectors whose expected output this library
// deliberately doesn't match, with why. Each is checked to still differ, so
// the list can't go stale. docs/architecture.md explains the choices.
var knownDifferences = func() map[string]map[string]string {
	const (
		monthDefault = "YEARLY;BYMONTHDAY without BYMONTH takes DTSTART's month, as libical, ical.js and ical.net do; dateutil expands every month"
		weekDefault  = "YEARLY;BYWEEKNO without BYDAY takes DTSTART's weekday, as libical and ical.net do; dateutil expands every day of the week"
		dateUntil    = "a DATE UNTIL on a DATE-TIME series includes that whole day; dateutil reads it as midnight"
		alias        = "BYWEEKDAY is a dateutil alias, not RFC 5545"
		untilForm    = "whether UNTIL's form matches DTSTART's is checked by the model, not by recur"
		year9999     = "iteration ends at year 9999, the last year iCalendar can write; ical.net raises an error instead"
		unimpl       = "libical doesn't implement BYWEEKNO with BYMONTHDAY; this library treats them as limits"
		unsynced     = "DTSTART doesn't match the rule, which RFC 5545 §3.8.5.3 leaves undefined; libical numbers the periods from DTSTART's calendar year here, unlike its other BYWEEKNO cases"
		icaljsSetPos = "ical.js's iterator emits an unmatched DTSTART when BYSETPOS is present, though not otherwise"
		gapByHour    = "sabre/vobject drops a DST-gap instance that BYHOUR names; RFC 5545 erratum 4271 keeps it, moved past the gap"
		sabreYearDay = "sabre/vobject maps BYYEARDAY=-366 in a 365-day year to December 31 of the year before; the day doesn't exist and is skipped"
		sabreWeekNo  = "sabre/vobject puts week -20 of 2012 at ISO week 32; 2012 has 52 weeks, so it is week 33"
	)
	m := map[string]map[string]string{
		"dateutil": {
			"dateutil-0005": monthDefault, "dateutil-0013": monthDefault, "dateutil-0034": monthDefault,
			"dateutil-0019": weekDefault,
			"dateutil-0240": dateUntil,
			"dateutil-0275": alias,
			"dateutil-0281": untilForm,
		},
		"rrulejs": {
			"rrulejs-0010": monthDefault, "rrulejs-0018": monthDefault, "rrulejs-0039": monthDefault,
			"rrulejs-0024": weekDefault,
		},
		"rust-rrule": {
			"rust-rrule-0047": monthDefault, "rust-rrule-0055": monthDefault, "rust-rrule-0072": monthDefault,
			"rust-rrule-0060": weekDefault,
		},
		"icaljs": {"icaljs-0096": icaljsSetPos, "icaljs-0097": icaljsSetPos},
		"icalnet": {
			"icalnet-0024": year9999, "icalnet-0026": year9999, "icalnet-0028": year9999,
			"icalnet-0029": year9999, "icalnet-0031": year9999,
		},
		"libical":       {"libical-0080": unimpl, "libical-0134": unsynced, "libical-0135": unsynced},
		"sabre-vobject": {"sabre-0027": gapByHour, "sabre-0063": sabreYearDay, "sabre-0075": sabreWeekNo},
	}
	return m
}()
