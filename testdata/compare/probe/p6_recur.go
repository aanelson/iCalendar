package main

// 6. Recurrence.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	ical "github.com/aanelson/iCalendar"
	"github.com/aanelson/iCalendar/civil"
	"github.com/aanelson/iCalendar/recur"
	ics "github.com/arran4/golang-ical"
	eical "github.com/emersion/go-ical"
	"github.com/teambition/rrule-go"
)

// The repository root; run.sh runs everything from this directory.
const checkout = "../.."

type vector struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	DTStart        string   `json:"dtstart"`
	TZID           string   `json:"tzid"`
	RRule          []string `json:"rrule"`
	ExDate         []string `json:"exdate"`
	RDate          []string `json:"rdate"`
	ExRule         []string `json:"exrule"`
	IncludeDTStart bool     `json:"include_dtstart"`
	Limit          int      `json:"limit"`
	Expected       []string `json:"expected"`
}

func loadRFCVectors() []vector {
	b, err := os.ReadFile(checkout + "/testdata/rfc5545/recur.json")
	if err != nil {
		panic(err)
	}
	var f struct{ Cases []vector }
	if err := json.Unmarshal(b, &f); err != nil {
		panic(err)
	}
	return f.Cases
}

const basic = "20060102T150405"

func fmtCivil(d civil.DateTime) string {
	return fmt.Sprintf("%04d%02d%02dT%02d%02d%02d", d.Year, d.Month, d.Day, d.Hour, d.Minute, d.Second)
}

// vectorICS writes a vector as a VEVENT.
func vectorICS(v vector) []byte {
	tz := ""
	if v.TZID != "" {
		tz = ";TZID=" + v.TZID
	}
	lines := []string{"DTSTART" + tz + ":" + v.DTStart}
	for _, r := range v.RRule {
		lines = append(lines, "RRULE:"+r)
	}
	for _, x := range v.ExDate {
		lines = append(lines, "EXDATE"+tz+":"+x)
	}
	for _, x := range v.RDate {
		lines = append(lines, "RDATE"+tz+":"+x)
	}
	return calWith("", lines...)
}

func take(n int, next func() (time.Time, bool), loc *time.Location) []string {
	var out []string
	for n <= 0 || len(out) < n {
		t, ok := next()
		if !ok {
			break
		}
		out = append(out, t.In(loc).Format(basic))
		if len(out) > 100000 {
			break
		}
	}
	return out
}

// expandRRuleGo expands a vector with rrule-go the way a caller would:
// DTSTART in its zone, the RRULE through StrToROption, EXDATEs on the set.
func expandRRuleGo(v vector) ([]string, error) {
	loc := time.UTC
	if v.TZID != "" {
		loc = nyLocFor(v.TZID)
	}
	dt, err := time.ParseInLocation(basic, v.DTStart, loc)
	if err != nil {
		return nil, err
	}
	set := &rrule.Set{}
	for _, s := range v.RRule {
		opt, err := rrule.StrToROption(s)
		if err != nil {
			return nil, err
		}
		opt.Dtstart = dt
		r, err := rrule.NewRRule(*opt)
		if err != nil {
			return nil, err
		}
		set.RRule(r)
	}
	for _, x := range v.ExDate {
		t, _ := time.ParseInLocation(basic, x, loc)
		set.ExDate(t)
	}
	for _, x := range v.RDate {
		t, _ := time.ParseInLocation(basic, x, loc)
		set.RDate(t)
	}
	return take(v.Limit, set.Iterator(), loc), nil
}

func nyLocFor(name string) *time.Location {
	l, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return l
}

// expandGoIcal parses the vector with golang-ical (which has no expander)
// and expands with rrule-go, as the consuming app does.
func expandGoIcal(v vector) ([]string, error) {
	cal, err := ics.ParseCalendar(bytes.NewReader(vectorICS(v)))
	if err != nil {
		return nil, err
	}
	ev := cal.Events()[0]
	start, err := ev.GetStartAt()
	if err != nil {
		return nil, err
	}
	set := &rrule.Set{}
	for _, p := range ev.GetProperties(ics.ComponentPropertyRrule) {
		opt, err := rrule.StrToROption(p.Value)
		if err != nil {
			return nil, err
		}
		opt.Dtstart = start
		r, err := rrule.NewRRule(*opt)
		if err != nil {
			return nil, err
		}
		set.RRule(r)
	}
	ex, err := ev.GetExDates()
	if err != nil {
		return nil, err
	}
	for _, t := range ex {
		set.ExDate(t)
	}
	return take(v.Limit, set.Iterator(), start.Location()), nil
}

func expandEmersion(v vector) ([]string, error) {
	cal, err := eical.NewDecoder(bytes.NewReader(vectorICS(v))).Decode()
	if err != nil {
		return nil, err
	}
	set, err := cal.Events()[0].RecurrenceSet(nil)
	if err != nil {
		return nil, err
	}
	loc := time.UTC
	if v.TZID != "" {
		loc = nyLocFor(v.TZID)
	}
	return take(v.Limit, set.Iterator(), loc), nil
}

// expandThis reads the vector through the ical package: the event's
// recurrence set (wall clock), and, as a cross-check, the Expander over a
// window ending just after the last expected instance.
func expandThis(v vector) ([]string, string, error) {
	cal, err := ical.Parse(bytes.NewReader(vectorICS(v)))
	if err != nil {
		return nil, "", err
	}
	var ev ical.Event
	for e := range cal.Events() {
		ev = e
	}
	set, err := ev.Recurrence()
	if err != nil {
		return nil, "", err
	}
	var out []string
	for d := range set.All() {
		out = append(out, fmtCivil(d))
		if v.Limit > 0 && len(out) == v.Limit || len(out) > 100000 {
			break
		}
	}
	// Expander cross-check.
	loc := time.UTC
	if v.TZID != "" {
		loc = nyLocFor(v.TZID)
	}
	first, _ := time.ParseInLocation(basic, v.DTStart, loc)
	last, _ := time.ParseInLocation(basic, v.Expected[len(v.Expected)-1], loc)
	var occ []string
	for o, err := range cal.Occurrences(first.Add(-time.Second), last.Add(time.Second)) {
		if err != nil {
			return out, "expander error: " + err.Error(), nil
		}
		occ = append(occ, fmtCivil(o.Start.DateTime))
	}
	xcheck := "expander agrees"
	if !slices.Equal(occ, v.Expected) {
		xcheck = fmt.Sprintf("expander window gives %v", occ)
	}
	return out, xcheck, nil
}

func init() {
	var ids []string
	for _, v := range loadRFCVectors() {
		ids = append(ids, v.ID)
	}
	register(&Probe{ID: "6.01-rfc-examples", Group: "6 recurrence",
		Title: "the 43 RFC 5545 §3.8.5.3 examples (testdata/rfc5545/recur.json)",
		RFC:   "§3.8.5.3 examples (erratum 3883 applied to case 33)",
		Libs:  []string{This, RRuleGo, GoIcal, Emersion},
		Args:  ids,
		Run: func(lib, id string) Result {
			var v vector
			for _, c := range loadRFCVectors() {
				if c.ID == id {
					v = c
				}
			}
			var got []string
			var err error
			note := ""
			switch lib {
			case This:
				got, note, err = expandThis(v)
			case RRuleGo:
				got, err = expandRRuleGo(v)
			case GoIcal:
				got, err = expandGoIcal(v)
				note = "golang-ical parses, rrule-go expands (golang-ical has no expander)"
			case Emersion:
				got, err = expandEmersion(v)
				note = "emersion's RecurrenceSet wraps rrule-go"
			}
			if err != nil {
				return failed(err)
			}
			r := check(strings.Join(got, ","), strings.Join(v.Expected, ","))
			if r.Outcome == "pass" && lib == This && note != "expander agrees" {
				r.Outcome = "wrong"
			}
			r.Detail = note
			if r.Outcome != "pass" {
				r.Got = diffSummary(got, v.Expected)
				r.Want = ""
			}
			return r
		}})

	// HOURLY;INTERVAL=24;BYHOUR=11 from 09:00 can never match.
	register(&Probe{ID: "6.02-hourly-interval24-byhour", Group: "6 recurrence",
		Title: "FREQ=HOURLY;INTERVAL=24;BYHOUR=11 from 09:00: ask for the first instance after DTSTART",
		RFC:   "§3.3.10: 09:00 + 24h·k never has hour 11, so the rule generates nothing; must terminate",
		Libs:  []string{This, RRuleGo, Emersion},
		Run: func(lib, _ string) Result {
			const rule = "FREQ=HOURLY;INTERVAL=24;BYHOUR=11"
			ny := nyLoc()
			t0 := time.Now()
			switch lib {
			case This:
				r, err := recur.ParseRule(rule)
				if err != nil {
					return failed(err)
				}
				n := 0
				for range r.All(civil.DateTime{Date: civil.Date{Year: 2026, Month: 1, Day: 1}, Time: civil.Time{Hour: 9}}) {
					n++
				}
				res := check(fmt.Sprintf("%d instances", n), "0 instances")
				res.Ms = since(t0)
				return res
			case RRuleGo:
				opt, _ := rrule.StrToROption(rule)
				opt.Dtstart = time.Date(2026, 1, 1, 9, 0, 0, 0, ny)
				r, err := rrule.NewRRule(*opt)
				if err != nil {
					return failed(err)
				}
				next := r.Iterator()
				t, ok := next()
				res := Result{Outcome: "pass", Got: fmt.Sprintf("returned (%v, %v)", t, ok), Ms: since(t0)}
				if ok {
					res.Outcome = "wrong"
				}
				return res
			case Emersion:
				cal, err := eical.NewDecoder(bytes.NewReader(calWith("", "DTSTART;TZID=America/New_York:20260101T090000", "RRULE:"+rule))).Decode()
				if err != nil {
					return failed(err)
				}
				set, err := cal.Events()[0].RecurrenceSet(nil)
				if err != nil {
					return failed(err)
				}
				t := set.After(time.Date(2026, 1, 1, 9, 0, 0, 0, ny), false)
				res := Result{Outcome: "pass", Got: fmt.Sprintf("returned %v", t), Ms: since(t0)}
				if !t.IsZero() {
					res.Outcome = "wrong"
				}
				return res
			}
			return na("")
		}})

	register(&Probe{ID: "6.03-duplicate-by-values", Group: "6 recurrence",
		Title: "duplicate BYxxx values: FREQ=MINUTELY;COUNT=5;BYSECOND=0,0 and FREQ=DAILY;COUNT=3;BYHOUR=9,9 from 09:00",
		RFC:   "§3.8.5.3: duplicate instances are ignored; COUNT counts instances",
		Libs:  []string{This, RRuleGo, Emersion},
		Args:  []string{"FREQ=MINUTELY;COUNT=5;BYSECOND=0,0", "FREQ=DAILY;COUNT=3;BYHOUR=9,9"},
		Run: func(lib, rule string) Result {
			want := map[string]string{
				"FREQ=MINUTELY;COUNT=5;BYSECOND=0,0": "20260101T090000,20260101T090100,20260101T090200,20260101T090300,20260101T090400",
				"FREQ=DAILY;COUNT=3;BYHOUR=9,9":      "20260101T090000,20260102T090000,20260103T090000",
			}[rule]
			v := vector{DTStart: "20260101T090000", TZID: "America/New_York", RRule: []string{rule}}
			var got []string
			var err error
			switch lib {
			case This:
				v.Expected = strings.Split(want, ",")
				got, _, err = expandThis(v)
			case RRuleGo:
				got, err = expandRRuleGo(v)
			case Emersion:
				got, err = expandEmersion(v)
			}
			if err != nil {
				return failed(err)
			}
			r := check(strings.Join(got, ","), want)
			if lib == RRuleGo {
				// rrule.Set collapses equal consecutive instants, but COUNT
				// was already spent on the duplicates; RRule.All shows them.
				opt, _ := rrule.StrToROption(rule)
				opt.Dtstart = time.Date(2026, 1, 1, 9, 0, 0, 0, nyLoc())
				rr, _ := rrule.NewRRule(*opt)
				var raw []string
				for _, t := range rr.All() {
					raw = append(raw, t.Format(basic))
				}
				r.Detail = "rrule.Set output above; RRule.All() gives " + strings.Join(raw, ",")
			}
			return r
		}})

	unmatchable := []string{
		"FREQ=YEARLY;BYMONTH=2;BYMONTHDAY=30",
		"FREQ=MONTHLY;BYMONTH=4,6,9,11;BYMONTHDAY=31",
		"FREQ=MONTHLY;BYDAY=6MO",
		"FREQ=DAILY;BYMONTH=2;BYMONTHDAY=30",
		"FREQ=DAILY;BYMONTH=2;BYMONTHDAY=30 @9000",
	}
	register(&Probe{ID: "6.04-unmatchable", Group: "6 recurrence",
		Title:   "rules that can never match: time to learn there are no instances (DTSTART 2026-01-01 09:00 New York)",
		RFC:     "§3.3.10: invalid dates are ignored, so these rules generate nothing; must terminate quickly",
		Libs:    []string{This, RRuleGo, Emersion},
		Args:    unmatchable,
		Timeout: 60,
		Run: func(lib, rule string) Result {
			ny := nyLoc()
			start := time.Date(2026, 1, 1, 9, 0, 0, 0, ny)
			if r, ok := strings.CutSuffix(rule, " @9000"); ok {
				// Same rule from DTSTART 9000-01-01: if the cost falls ~8×,
				// the walk is to year 9999, not to a fixed horizon.
				rule, start = r, time.Date(9000, 1, 1, 9, 0, 0, 0, ny)
			}
			t0 := time.Now()
			n := 0
			switch lib {
			case This:
				r, err := recur.ParseRule(rule)
				if err != nil {
					return failed(err)
				}
				for range r.All(civil.DateTimeOf(start)) {
					n++
				}
			case RRuleGo, Emersion:
				opt, err := rrule.StrToROption(rule)
				if err != nil {
					return failed(err)
				}
				opt.Dtstart = start
				r, err := rrule.NewRRule(*opt)
				if err != nil {
					return failed(err)
				}
				n = len(r.All())
			}
			res := check(fmt.Sprintf("%d instances", n), "0 instances")
			res.Ms = since(t0)
			res.Detail = fmt.Sprintf("%.3f ms to exhaust", res.Ms)
			return res
		}})

	register(&Probe{ID: "6.08-unbounded-horizon", Group: "6 recurrence",
		Title:   "FREQ=YEARLY;BYMONTH=2;BYDAY=5MO from 2026-01-01 (5th Monday of February: leap years whose Feb 29 is a Monday), all instances",
		RFC:     "§3.3.10 (numbered BYDAY counts within BYMONTH, errata 1913/3779); no UNTIL/COUNT, so it runs to year 9999. Brute force: 299 instances, 2044 … 9988",
		Libs:    []string{This, RRuleGo, Emersion},
		Timeout: 60,
		Run: func(lib, _ string) Result {
			ny := nyLoc()
			start := time.Date(2026, 1, 1, 9, 0, 0, 0, ny)
			var got []string
			switch lib {
			case This:
				r, _ := recur.ParseRule("FREQ=YEARLY;BYMONTH=2;BYDAY=5MO")
				for d := range r.All(civil.DateTimeOf(start)) {
					got = append(got, fmt.Sprint(d.Year))
				}
			default:
				opt, _ := rrule.StrToROption("FREQ=YEARLY;BYMONTH=2;BYDAY=5MO")
				opt.Dtstart = start
				r, _ := rrule.NewRRule(*opt)
				for _, t := range r.All() {
					got = append(got, fmt.Sprint(t.Year()))
				}
			}
			last := ""
			if len(got) > 0 {
				last = got[len(got)-1]
			}
			return check(fmt.Sprintf("%d instances, first %s, last %s", len(got), got[0], last), "299 instances, first 2044, last 9988")
		}})

	register(&Probe{ID: "6.05a-iterator-mutates-rule", Group: "6 recurrence",
		Title: "does iterating change the rule value? (rrule-go: unexported RRule.len; Set.Iterator sorts RDATEs in place)",
		RFC:   "API property (not an RFC requirement)",
		Libs:  []string{This, RRuleGo},
		Run: func(lib, _ string) Result {
			ny := nyLoc()
			switch lib {
			case RRuleGo:
				r, _ := rrule.NewRRule(rrule.ROption{Freq: rrule.DAILY, Count: 5, Dtstart: time.Date(2026, 1, 1, 9, 0, 0, 0, ny)})
				before := reflect.ValueOf(r).Elem().FieldByName("len").Int()
				r.All()
				after := reflect.ValueOf(r).Elem().FieldByName("len").Int()
				set := &rrule.Set{}
				set.RRule(r)
				set.RDate(time.Date(2026, 3, 1, 9, 0, 0, 0, ny))
				set.RDate(time.Date(2026, 2, 1, 9, 0, 0, 0, ny))
				order0 := set.GetRDate()[0].Format("01-02")
				set.All()
				order1 := set.GetRDate()[0].Format("01-02")
				got := fmt.Sprintf("RRule.len %d → %d after All(); Set RDATE[0] %s → %s after All()", before, after, order0, order1)
				if before != after || order0 != order1 {
					return Result{Outcome: "wrong", Got: got, Want: "unchanged"}
				}
				return pass(got)
			case This:
				r, _ := recur.ParseRule("FREQ=DAILY;COUNT=5;BYHOUR=9,10")
				c := r
				c.ByHour = slices.Clone(r.ByHour)
				for range r.All(civil.DateTimeOf(time.Date(2026, 1, 1, 9, 0, 0, 0, ny))) {
				}
				set := &recur.Set{Start: civil.DateTimeOf(time.Date(2026, 1, 1, 9, 0, 0, 0, ny)), Location: ny, Rules: []recur.Rule{r},
					RDates: []civil.DateTime{civil.DateTimeOf(time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)), civil.DateTimeOf(time.Date(2026, 2, 1, 9, 0, 0, 0, time.UTC))}}
				o0 := set.RDates[0]
				for range set.All() {
				}
				got := fmt.Sprintf("rule equal after iteration: %v; RDates[0] unchanged: %v", reflect.DeepEqual(r, c), set.RDates[0] == o0)
				if !reflect.DeepEqual(r, c) || set.RDates[0] != o0 {
					return Result{Outcome: "wrong", Got: got}
				}
				return pass(got)
			}
			return na("")
		}})

	register(&Probe{ID: "6.05b-concurrent-iteration", Group: "6 recurrence",
		Title: "8 goroutines iterate one rule (and one set) at once, under the race detector",
		RFC:   "API property: is a rule safe to share?",
		Libs:  []string{This, RRuleGo},
		Race:  true,
		Run: func(lib, _ string) Result {
			ny := nyLoc()
			var wg sync.WaitGroup
			counts := make([]int, 8)
			switch lib {
			case RRuleGo:
				r, _ := rrule.NewRRule(rrule.ROption{Freq: rrule.DAILY, Count: 50, Dtstart: time.Date(2026, 1, 1, 9, 0, 0, 0, ny)})
				set := &rrule.Set{}
				set.RRule(r)
				set.RDate(time.Date(2027, 3, 1, 9, 0, 0, 0, ny))
				set.RDate(time.Date(2027, 2, 1, 9, 0, 0, 0, ny))
				for i := range 8 {
					wg.Add(1)
					go func() { defer wg.Done(); counts[i] = len(r.All()) + len(set.All()) }()
				}
			case This:
				r, _ := recur.ParseRule("FREQ=DAILY;COUNT=50")
				start := civil.DateTimeOf(time.Date(2026, 1, 1, 9, 0, 0, 0, ny))
				set := &recur.Set{Start: start, Location: ny, Rules: []recur.Rule{r}, Cache: recur.NewCache(0),
					RDates: []civil.DateTime{civil.DateTimeOf(time.Date(2027, 3, 1, 9, 0, 0, 0, time.UTC)), civil.DateTimeOf(time.Date(2027, 2, 1, 9, 0, 0, 0, time.UTC))}}
				for i := range 8 {
					wg.Add(1)
					go func() {
						defer wg.Done()
						for range r.All(start) {
							counts[i]++
						}
						for range set.All() {
							counts[i]++
						}
					}()
				}
			}
			wg.Wait()
			return Result{Outcome: "pass", Got: fmt.Sprintf("counts %v (the driver checks stderr for DATA RACE)", counts)}
		}})

	register(&Probe{ID: "6.06-month-of-1990-daily", Group: "6 recurrence",
		Title:   "one month (Oct 2026) of FREQ=DAILY begun 1990-01-01 09:00 New York (testing.Benchmark)",
		RFC:     "performance; correctness = 31 instances",
		Libs:    []string{This, RRuleGo, Emersion},
		Args:    []string{"no COUNT", "COUNT=20000", "COUNT=20000 + cache", "no COUNT, recur.Rule.Between (civil)", "COUNT=20000, recur.Rule.Between (civil)", "COUNT=20000 + cache, recur.Set.Between (civil)"},
		Timeout: 120,
		Run:     benchMonth,
	})

	register(&Probe{ID: "6.07-daily-across-dst-gap", Group: "6 recurrence",
		Title: "FREQ=DAILY;COUNT=5 at 02:30 New York from 2007-03-09: the 03-11 instance is 03:30 EDT (07:30Z)",
		RFC:   "§3.3.10 + erratum 4271 + §3.3.5: a gap instance is kept and uses the offset before the gap",
		Libs:  []string{This, RRuleGo, GoIcal, Emersion},
		Run: func(lib, _ string) Result {
			want := "2007-03-09T07:30:00Z,2007-03-10T07:30:00Z,2007-03-11T07:30:00Z,2007-03-12T06:30:00Z,2007-03-13T06:30:00Z"
			v := vector{DTStart: "20070309T023000", TZID: "America/New_York", RRule: []string{"FREQ=DAILY;COUNT=5"}}
			var got []string
			switch lib {
			case This:
				cal, err := ical.Parse(bytes.NewReader(vectorICS(v)))
				if err != nil {
					return failed(err)
				}
				for o, err := range cal.Occurrences(time.Date(2007, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2008, 1, 1, 0, 0, 0, 0, time.UTC)) {
					if err != nil {
						return failed(err)
					}
					got = append(got, utc(o.Start.Time()))
				}
			default:
				got = instantsVia(lib, v)
			}
			return check(strings.Join(got, ","), want)
		}})
}

// instantsVia expands v with an rrule-go based library and returns UTC
// instants.
func instantsVia(lib string, v vector) []string {
	ny := nyLoc()
	var next func() (time.Time, bool)
	switch lib {
	case RRuleGo:
		dt, _ := time.ParseInLocation(basic, v.DTStart, ny)
		opt, _ := rrule.StrToROption(v.RRule[0])
		opt.Dtstart = dt
		r, _ := rrule.NewRRule(*opt)
		next = r.Iterator()
	case GoIcal:
		cal, _ := ics.ParseCalendar(bytes.NewReader(vectorICS(v)))
		start, _ := cal.Events()[0].GetStartAt()
		opt, _ := rrule.StrToROption(v.RRule[0])
		opt.Dtstart = start
		r, _ := rrule.NewRRule(*opt)
		next = r.Iterator()
	case Emersion:
		cal, _ := eical.NewDecoder(bytes.NewReader(vectorICS(v))).Decode()
		set, _ := cal.Events()[0].RecurrenceSet(nil)
		next = set.Iterator()
	}
	var out []string
	for range 100 {
		t, ok := next()
		if !ok {
			break
		}
		out = append(out, utc(t))
	}
	return out
}

func diffSummary(got, want []string) string {
	i := 0
	for i < len(got) && i < len(want) && got[i] == want[i] {
		i++
	}
	g, w := "end", "end"
	if i < len(got) {
		g = got[i]
	}
	if i < len(want) {
		w = want[i]
	}
	return fmt.Sprintf("%d instances (want %d); first difference at #%d: got %s, want %s", len(got), len(want), i+1, g, w)
}

func benchMonth(lib, arg string) Result {
	ny := nyLoc()
	dt := time.Date(1990, 1, 1, 9, 0, 0, 0, ny)
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, ny)
	to := time.Date(2026, 11, 1, 0, 0, 0, 0, ny)
	count := 0
	if strings.HasPrefix(arg, "COUNT") {
		count = 20000
	}
	cached := strings.Contains(arg, "cache")
	var run func() int
	switch lib {
	case RRuleGo, Emersion:
		if cached {
			return na("no cache in rrule-go")
		}
		if strings.Contains(arg, "Rule.Between") || strings.Contains(arg, "Set.Between") {
			return na("this library's API only")
		}
		var r *rrule.RRule
		var set *rrule.Set
		if lib == RRuleGo {
			r, _ = rrule.NewRRule(rrule.ROption{Freq: rrule.DAILY, Count: count, Dtstart: dt})
			run = func() int { return len(r.Between(from, to, true)) }
		} else {
			rr := "RRULE:FREQ=DAILY"
			if count > 0 {
				rr += fmt.Sprintf(";COUNT=%d", count)
			}
			cal, err := eical.NewDecoder(bytes.NewReader(calWith("", "DTSTART;TZID=America/New_York:19900101T090000", rr))).Decode()
			if err != nil {
				return failed(err)
			}
			set, err = cal.Events()[0].RecurrenceSet(nil)
			if err != nil {
				return failed(err)
			}
			run = func() int { return len(set.Between(from, to, true)) }
		}
	case This:
		rule := recur.Rule{Freq: recur.Daily, Count: count}
		if strings.Contains(arg, "Rule.Between") {
			cs, cf, ct := civil.DateTimeOf(dt), civil.DateTimeOf(from), civil.DateTimeOf(to)
			run = func() int {
				n := 0
				for range rule.Between(cs, cf, ct) {
					n++
				}
				return n
			}
			break
		}
		set := &recur.Set{Start: civil.DateTimeOf(dt), Location: ny, Rules: []recur.Rule{rule}}
		if cached {
			set.Cache = recur.NewCache(0)
		}
		if strings.Contains(arg, "Set.Between") {
			cf, ct := civil.DateTimeOf(from), civil.DateTimeOf(to)
			run = func() int {
				n := 0
				for range set.Between(cf, ct) {
					n++
				}
				return n
			}
			break
		}
		run = func() int {
			n := 0
			for range set.Times(from, to) {
				n++
			}
			return n
		}
	}
	n := run()
	b := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			run()
		}
	})
	r := Result{Outcome: "pass", Got: fmt.Sprintf("%d instances; %s/op, %d allocs/op", n, fmtNs(float64(b.NsPerOp())), b.AllocsPerOp()),
		Extra: map[string]any{"ns_per_op": b.NsPerOp(), "allocs_per_op": b.AllocsPerOp(), "bytes_per_op": b.AllocedBytesPerOp(), "iterations": b.N}}
	if n != 31 {
		r.Outcome = "wrong"
		r.Want = "31 instances"
	}
	return r
}

func fmtNs(ns float64) string {
	switch {
	case ns >= 1e6:
		return fmt.Sprintf("%.2f ms", ns/1e6)
	case ns >= 1e3:
		return fmt.Sprintf("%.2f µs", ns/1e3)
	}
	return fmt.Sprintf("%.0f ns", ns)
}
