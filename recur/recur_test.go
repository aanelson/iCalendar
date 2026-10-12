package recur

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aanelson/iCalendar/civil"
	"github.com/aanelson/iCalendar/internal/icaltime"
)

func dt(s string) civil.DateTime {
	v, _, _, err := icaltime.Parse(s)
	if err != nil {
		panic(err)
	}
	return v
}

func collect(seq func(func(civil.DateTime) bool), n int) []civil.DateTime {
	var out []civil.DateTime
	for t := range seq {
		out = append(out, t)
		if n > 0 && len(out) == n {
			break
		}
	}
	return out
}

func TestParseRoundTrip(t *testing.T) {
	for _, s := range []string{
		"FREQ=DAILY",
		"FREQ=WEEKLY;UNTIL=19971224T000000Z;INTERVAL=2;BYDAY=MO,-1FR,2TU;WKST=SU",
		"FREQ=YEARLY;COUNT=10;BYSECOND=0,30;BYMINUTE=15;BYHOUR=9;BYMONTHDAY=-1,1;BYYEARDAY=100,-366;BYWEEKNO=-53,20;BYMONTH=1,12;BYSETPOS=-1,1",
		"FREQ=MONTHLY;UNTIL=20240101",
		"FREQ=HOURLY;UNTIL=20240101T120000",
	} {
		r, err := ParseRule(s)
		if err != nil {
			t.Fatalf("ParseRule(%q): %v", s, err)
		}
		if got := r.String(); got != s {
			t.Errorf("String() = %q, want %q", got, s)
		}
		back, _ := ParseRule(r.String())
		if !back.Equal(r) {
			t.Errorf("%q doesn't read back equal", s)
		}
	}
}

func TestParseLenient(t *testing.T) {
	r, err := ParseRule("freq=weekly;byday=mo,we;count=3;")
	if err != nil {
		t.Fatal(err)
	}
	if r.String() != "FREQ=WEEKLY;COUNT=3;BYDAY=MO,WE" {
		t.Errorf("got %s", r)
	}
}

// rrule-go reads COUNT=0 as an endless series and INTERVAL=0 as 1; both
// are outside the grammar's meaning ("positive integer").
func TestParseErrors(t *testing.T) {
	for _, s := range []string{
		"", "COUNT=3", "FREQ=FORTNIGHTLY", "FREQ=DAILY;COUNT=0", "FREQ=DAILY;INTERVAL=0",
		"FREQ=DAILY;COUNT=-1", "FREQ=DAILY;FREQ=WEEKLY", "FREQ=DAILY;BYHOUR=24",
		"FREQ=DAILY;BYMONTHDAY=0", "FREQ=DAILY;BYMONTHDAY=32", "FREQ=DAILY;BYDAY=XX",
		"FREQ=DAILY;BYDAY=54MO", "FREQ=DAILY;UNTIL=1997", "FREQ=DAILY;X-FOO=1",
		"FREQ=DAILY;COUNT=99999999999", "FREQ=DAILY;BYMONTH=1,,2", "FREQ=DAILY;WKST",
	} {
		if _, err := ParseRule(s); err == nil {
			t.Errorf("ParseRule(%q) succeeded", s)
		}
	}
}

func TestValidate(t *testing.T) {
	for _, s := range []string{
		"FREQ=DAILY;COUNT=3;UNTIL=20240101T000000Z",
		"FREQ=WEEKLY;BYMONTHDAY=1",
		"FREQ=MONTHLY;BYYEARDAY=1",
		"FREQ=MONTHLY;BYWEEKNO=1",
		"FREQ=WEEKLY;BYDAY=1MO",
		"FREQ=YEARLY;BYWEEKNO=1;BYDAY=1MO",
		"FREQ=DAILY;BYSETPOS=1",
	} {
		r, err := ParseRule(s)
		if err != nil {
			t.Fatalf("ParseRule(%q): %v", s, err)
		}
		if r.Validate() == nil {
			t.Errorf("%q validated", s)
		}
	}
	r, _ := ParseRule("FREQ=MONTHLY;BYDAY=-1FR;BYSETPOS=1")
	if err := r.Validate(); err != nil {
		t.Errorf("valid rule: %v", err)
	}
}

// rrule-go issue 70: repeated BYSECOND values gave duplicate instances and
// used up COUNT twice as fast.
func TestDuplicatesCountOnce(t *testing.T) {
	r, _ := ParseRule("FREQ=DAILY;COUNT=4;BYSECOND=0,0;BYMINUTE=0,0")
	got := collect(r.All(dt("20240105T090000")), 0)
	if len(got) != 4 || got[0] == got[1] {
		t.Errorf("got %v", got)
	}
}

// Rules that can never match must end at once instead of walking to year
// 9999, and HOURLY;INTERVAL=24;BYHOUR=11 from 09:00 must not loop forever
// (it does in rrule-go).
func TestNeverMatchingRulesEndQuickly(t *testing.T) {
	for _, s := range []string{
		"FREQ=HOURLY;INTERVAL=24;BYHOUR=11",
		"FREQ=MINUTELY;INTERVAL=120;BYMINUTE=30",
		"FREQ=SECONDLY;INTERVAL=60;BYSECOND=30",
		"FREQ=YEARLY;BYMONTH=2;BYMONTHDAY=30",
		"FREQ=MONTHLY;BYMONTH=4,6,9,11;BYMONTHDAY=31",
		"FREQ=DAILY;BYMONTH=2;BYMONTHDAY=30",
		"FREQ=DAILY;INTERVAL=7;BYDAY=WE", // the start is a Tuesday
		"FREQ=MONTHLY;BYDAY=6MO",
		"FREQ=YEARLY;BYMONTH=3;BYDAY=6MO",
		// Not caught statically: these end by the 400-year cycle bound.
		"FREQ=YEARLY;BYYEARDAY=366;BYMONTH=1",
		"FREQ=DAILY;BYYEARDAY=366;BYMONTH=1",
		"FREQ=HOURLY;BYYEARDAY=366;BYMONTH=1",
		"FREQ=WEEKLY;BYWEEKNO=53;BYMONTH=6",
	} {
		r, err := ParseRule(s)
		if err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		got := collect(r.All(dt("20240102T090000")), 0)
		if d := time.Since(start); d > 2*time.Second {
			t.Errorf("%s took %v", s, d)
		}
		if len(got) != 0 {
			t.Errorf("%s produced %v", s, got[:min(len(got), 3)])
		}
	}
}

func TestEndsAtYear9999(t *testing.T) {
	r, _ := ParseRule("FREQ=YEARLY;INTERVAL=1000")
	got := collect(r.All(dt("20240101T000000")), 0)
	if len(got) != 8 || got[len(got)-1].Year != 9024 {
		t.Errorf("got %v", got)
	}
}

func TestUntilForms(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	start := dt("20240101T090000")
	for _, tc := range []struct {
		until string
		loc   *time.Location
		want  int
	}{
		{"20240105", nil, 5},                // a DATE: the whole day
		{"20240105T090000", nil, 5},         // floating: inclusive
		{"20240105T085959", nil, 4},         //
		{"20240105T140000Z", ny, 5},         // 09:00 EST is 14:00Z
		{"20240105T135959Z", ny, 4},         //
		{"20240105T090000Z", nil, 5},        // UTC with a floating start compares wall clocks
		{"20240105T085959Z", time.UTC, 4},   //
		{"20231231T000000Z", ny, 1},         // before DTSTART: only DTSTART itself
		{"99991231T235959Z", ny, 1_000_000}, // far: limited below
	} {
		r, _ := ParseRule("FREQ=DAILY;UNTIL=" + tc.until)
		s := &Set{Start: start, Location: tc.loc, Rules: []Rule{r}}
		got := collect(s.All(), 1000)
		want := min(tc.want, 1000)
		if len(got) != want {
			t.Errorf("UNTIL=%s in %v: %d instances, want %d", tc.until, tc.loc, len(got), want)
		}
	}
}

func TestSetMerges(t *testing.T) {
	r1, _ := ParseRule("FREQ=WEEKLY;BYDAY=MO;COUNT=3")
	r2, _ := ParseRule("FREQ=WEEKLY;BYDAY=MO,WE;COUNT=4")
	ex, _ := ParseRule("FREQ=MONTHLY;BYMONTHDAY=10")
	s := &Set{
		Start:   dt("20240101T090000"), // a Monday
		Rules:   []Rule{r1, r2},
		ExRules: []Rule{ex},
		RDates:  []civil.DateTime{dt("20240102T120000"), dt("20240101T090000"), dt("20240301T090000")},
		ExDates: []civil.DateTime{dt("20240103T090000"), dt("20240201T000000")},
	}
	var got []string
	for t := range s.All() {
		got = append(got, icaltime.Format(t, false, false))
	}
	want := []string{"20240101T090000", "20240102T120000", "20240108T090000", "20240115T090000", "20240301T090000"}
	if !slices.Equal(got, want) {
		t.Errorf("got  %v\nwant %v", got, want)
	}
}

// RFC 5545 §3.8.5.3: DTSTART counts as the first occurrence even when the
// rule doesn't generate it.
func TestCountIncludesUnmatchedStart(t *testing.T) {
	r, _ := ParseRule("FREQ=MONTHLY;BYDAY=1FR;COUNT=3")
	s := &Set{Start: dt("20120107T090000"), Rules: []Rule{r}} // a Saturday
	got := collect(s.All(), 0)
	if len(got) != 3 || got[0] != dt("20120107T090000") || got[1] != dt("20120203T090000") {
		t.Errorf("got %v", got)
	}
	// The rule alone, as dateutil reads it, counts only what it generates.
	if got := collect(r.All(dt("20120107T090000")), 0); len(got) != 3 || got[0] != dt("20120203T090000") {
		t.Errorf("Rule.All got %v", got)
	}
}

func TestAllDayIgnoresTimeParts(t *testing.T) {
	r, _ := ParseRule("FREQ=DAILY;BYHOUR=9,17;BYMINUTE=30;COUNT=3")
	s := &Set{Start: dt("20240101"), Rules: []Rule{r}, AllDay: true}
	got := collect(s.All(), 0)
	want := []civil.DateTime{dt("20240101"), dt("20240102"), dt("20240103")}
	if !slices.Equal(got, want) {
		t.Errorf("got %v", got)
	}
}

func TestTimesResolvesGaps(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	r, _ := ParseRule("FREQ=DAILY;COUNT=3")
	s := &Set{Start: dt("20070310T023000"), Location: ny, Rules: []Rule{r}}
	var got []string
	for at := range s.Times(time.Date(2007, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2008, 1, 1, 0, 0, 0, 0, time.UTC)) {
		got = append(got, at.Format(time.RFC3339))
	}
	want := []string{"2007-03-10T02:30:00-05:00", "2007-03-11T03:30:00-04:00", "2007-03-12T02:30:00-04:00"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// Seeking must never change the answer: Between on any window equals the
// full walk cut to that window, with and without a cache, for every rule
// in the vectors and every window boundary near their instances.
func TestSeekMatchesWalk(t *testing.T) {
	cache := NewCache(0)
	rng := rand.New(rand.NewPCG(1, 2))
	checked := 0
	for _, path := range vectorFiles(t) {
		f := loadVectors(t, path)
		for _, v := range f.Cases {
			if len(v.Extensions) > 0 || v.Error || len(v.RRule) == 0 {
				continue
			}
			s, ok := vectorSet(v)
			if !ok {
				continue
			}
			full := collect(s.All(), 400)
			if len(full) == 0 {
				continue
			}
			windows := [][2]civil.DateTime{{full[0], full[len(full)-1].AddSeconds(1)}}
			for range 6 {
				i, j := rng.IntN(len(full)), rng.IntN(len(full))
				if i > j {
					i, j = j, i
				}
				a := full[i].AddSeconds(-int64(rng.IntN(3)))
				b := full[j].AddSeconds(int64(rng.IntN(3)))
				windows = append(windows, [2]civil.DateTime{a, b})
			}
			for _, w := range windows {
				var want []civil.DateTime
				for _, x := range full {
					if !x.Before(w[0]) && x.Before(w[1]) {
						want = append(want, x)
					}
				}
				for _, c := range []*Cache{nil, cache, cache} {
					s.Cache = c
					got := collect(s.Between(w[0], w[1]), 0)
					if !slices.Equal(got, want) {
						t.Fatalf("%s %v from %s: Between(%v, %v) cache=%v\n got  %v\n want %v",
							v.ID, v.RRule, v.DTStart, w[0], w[1], c != nil, got, want)
					}
				}
				checked++
			}
		}
	}
	t.Logf("%d windows checked; cache %+v", checked, cache.Stats())
}

// vectorSet builds the Set a vector describes, for the differential test.
func vectorSet(v vector) (*Set, bool) {
	start, isDate, utc, err := icaltime.Parse(v.DTStart)
	if err != nil {
		return nil, false
	}
	var loc *time.Location
	if v.TZID != "" {
		if loc, err = time.LoadLocation(v.TZID); err != nil {
			return nil, false
		}
	} else if utc {
		loc = time.UTC
	}
	s := &Set{Start: start, Location: loc, AllDay: isDate, omitStart: !v.IncludeDTStart}
	for _, x := range v.RRule {
		r, err := ParseRule(x)
		if err != nil {
			return nil, false
		}
		s.Rules = append(s.Rules, r)
	}
	for _, x := range v.ExDate {
		e, _, eUTC, err := icaltime.Parse(x)
		if err != nil || eUTC != utc {
			return nil, false
		}
		s.ExDates = append(s.ExDates, e)
	}
	for _, x := range v.RDate {
		e, _, eUTC, err := icaltime.Parse(x)
		if err != nil || eUTC != utc {
			return nil, false
		}
		s.RDates = append(s.RDates, e)
	}
	return s, true
}

// A COUNT rule queried far from DTSTART is answered from the cache's
// checkpoints and end on later calls, and agrees with the walk.
func TestCacheCountRule(t *testing.T) {
	r, _ := ParseRule("FREQ=DAILY;COUNT=100000")
	start := dt("19900101T090000")
	cache := NewCache(10)
	from, to := dt("20200101T000000"), dt("20200201T000000")
	want := collect(r.between(start, from, to, nil), 0)
	if len(want) != 31 {
		t.Fatalf("want 31 instances, got %d", len(want))
	}
	for range 3 {
		if got := collect(r.between(start, from, to, cache), 0); !slices.Equal(got, want) {
			t.Fatalf("with cache: %v", got)
		}
	}
	// Past the end (in 2263), which the first walk didn't reach.
	late := collect(r.between(start, dt("23000101T000000"), dt("23010101T000000"), cache), 0)
	if len(late) != 0 {
		t.Fatalf("after the 100000th instance: %v", late)
	}
	if end, ok := cache.end(cacheKey(r, start, iterOpts{})); !ok || civil.DateTimeOfEpochSecond(end) != start.AddDate(0, 0, 99999) {
		t.Errorf("cached end = %v, %v", civil.DateTimeOfEpochSecond(end), ok)
	}
	if st := cache.Stats(); st.Hits == 0 || st.Rules != 1 {
		t.Errorf("stats %+v", st)
	}
}

func TestCacheEvicts(t *testing.T) {
	cache := NewCache(2)
	for i := range 5 {
		r := Rule{Freq: Daily, Count: 1000 + i}
		collect(r.between(dt("20000101T000000"), dt("20010101T000000"), dt("20010102T000000"), cache), 0)
	}
	if st := cache.Stats(); st.Rules != 2 || st.Evicted != 3 {
		t.Errorf("stats %+v", st)
	}
}

func TestConcurrentUse(t *testing.T) {
	r, _ := ParseRule("FREQ=WEEKLY;BYDAY=MO,FR;COUNT=5000")
	cache := NewCache(0)
	s := &Set{Start: dt("20000103T090000"), Rules: []Rule{r}, Cache: cache}
	want := collect(s.Between(dt("20300101T000000"), dt("20310101T000000")), 0)
	done := make(chan []civil.DateTime)
	for range 8 {
		go func() { done <- collect(s.Between(dt("20300101T000000"), dt("20310101T000000")), 0) }()
	}
	for range 8 {
		if got := <-done; !slices.Equal(got, want) {
			t.Fatal("concurrent expansion disagreed")
		}
	}
}

func FuzzRule(f *testing.F) {
	for _, s := range []string{
		"FREQ=DAILY;COUNT=10", "FREQ=YEARLY;BYWEEKNO=1,-1;BYDAY=MO", "FREQ=MONTHLY;BYDAY=-1FR;BYSETPOS=1",
		"FREQ=HOURLY;INTERVAL=7;BYHOUR=1,9", "FREQ=MINUTELY;BYSECOND=0,30;BYMONTH=2", "FREQ=WEEKLY;INTERVAL=2;WKST=SU;BYDAY=TU,SU",
		"FREQ=YEARLY;BYYEARDAY=-1,100;BYMONTHDAY=10", "FREQ=SECONDLY;INTERVAL=86401;COUNT=5",
	} {
		f.Add(s, int64(19970902090000), int64(3), int64(40))
	}
	f.Fuzz(func(t *testing.T, s string, startSeed, i, j int64) {
		r, err := ParseRule(s)
		if err != nil {
			return
		}
		if strings.Count(s, ",") > 40 {
			return
		}
		start := civil.DateTimeOfEpochSecond(floorMod(startSeed, 4102444800)) // 1970 to 2100
		set := &Set{Start: start, Rules: []Rule{r}}
		full := collect(set.All(), 200)
		if !slices.IsSortedFunc(full, civil.DateTime.Compare) {
			t.Fatalf("%s from %v: out of order", s, start)
		}
		for k := 1; k < len(full); k++ {
			if full[k] == full[k-1] {
				t.Fatalf("%s from %v: duplicate %v", s, start, full[k])
			}
		}
		if len(full) < 2 {
			return
		}
		a := full[floorMod(i, int64(len(full)))]
		b := full[floorMod(j, int64(len(full)))]
		if b.Before(a) {
			a, b = b, a
		}
		var want []civil.DateTime
		for _, x := range full {
			if !x.Before(a) && x.Before(b) {
				want = append(want, x)
			}
		}
		if got := collect(set.Between(a, b), 0); !slices.Equal(got, want) {
			t.Fatalf("%s from %v: Between(%v, %v)\n got  %v\n want %v", s, start, a, b, got, want)
		}
	})
}

// A query for one month of a daily series begun in 1990 costs one month
// of work, however far 2026 is from DTSTART.
func BenchmarkBetweenFarFromStart(b *testing.B) {
	r, _ := ParseRule("FREQ=DAILY;BYDAY=MO,WE,FR")
	start, from, to := dt("19900101T090000"), dt("20260101T000000"), dt("20260201T000000")
	b.ReportAllocs()
	for b.Loop() {
		for range r.Between(start, from, to) {
		}
	}
}

// The same with COUNT, which needs the cache to avoid counting from 1990.
func BenchmarkBetweenCountCached(b *testing.B) {
	r, _ := ParseRule("FREQ=DAILY;BYDAY=MO,WE,FR;COUNT=100000")
	start, from, to := dt("19900101T090000"), dt("20260101T000000"), dt("20260201T000000")
	cache := NewCache(0)
	b.ReportAllocs()
	for b.Loop() {
		for range r.between(start, from, to, cache) {
		}
	}
}

func BenchmarkBetweenCountUncached(b *testing.B) {
	r, _ := ParseRule("FREQ=DAILY;BYDAY=MO,WE,FR;COUNT=100000")
	start, from, to := dt("19900101T090000"), dt("20260101T000000"), dt("20260201T000000")
	b.ReportAllocs()
	for b.Loop() {
		for range r.between(start, from, to, nil) {
		}
	}
}

func BenchmarkAllYearly(b *testing.B) {
	r, _ := ParseRule("FREQ=YEARLY;BYMONTH=1,4,7,10;BYDAY=-1FR;COUNT=1000")
	start := dt("20000101T090000")
	b.ReportAllocs()
	for b.Loop() {
		for range r.All(start) {
		}
	}
}

// Regressions found in review.

// An all-day series with a sub-daily rule keeps to its window.
func TestAllDaySubDailyWindow(t *testing.T) {
	r, _ := ParseRule("FREQ=HOURLY;INTERVAL=5")
	s := &Set{Start: dt("20240101"), Rules: []Rule{r}, AllDay: true}
	if got := collect(s.Between(dt("20240102T120000"), dt("20240104")), 0); !slices.Equal(got, []civil.DateTime{dt("20240103")}) {
		t.Errorf("from midday: %v", got)
	}
	if got := collect(s.Between(dt("20240102"), dt("20240103T000001")), 0); !slices.Equal(got, []civil.DateTime{dt("20240102"), dt("20240103")}) {
		t.Errorf("to just past midnight: %v", got)
	}
}

// An instance in a DST gap resolves to after later wall-clock times;
// instants still come out in order and the window is by instant.
func TestGapOrdering(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	r, _ := ParseRule("FREQ=DAILY;COUNT=2")
	s := &Set{Start: dt("20070311T023000"), Location: ny, Rules: []Rule{r}, RDates: []civil.DateTime{dt("20070311T031000")}}
	var got []string
	for at := range s.Times(time.Date(2007, 3, 11, 0, 0, 0, 0, ny), time.Date(2007, 3, 11, 3, 20, 0, 0, ny)) {
		got = append(got, at.Format(time.RFC3339))
	}
	if !slices.Equal(got, []string{"2007-03-11T03:10:00-04:00"}) {
		t.Errorf("got %v", got)
	}
	got = nil
	for at := range s.Times(time.Date(2007, 3, 1, 0, 0, 0, 0, ny), time.Date(2007, 4, 1, 0, 0, 0, 0, ny)) {
		got = append(got, at.Format(time.RFC3339))
	}
	want := []string{"2007-03-11T03:10:00-04:00", "2007-03-11T03:30:00-04:00", "2007-03-12T02:30:00-04:00"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// An EXDATE given as an instant excludes an instance a DST gap moved, and
// an RDATE instant can name the second of two repeated wall-clock times.
func TestInstantRDatesAndExDates(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	r, _ := ParseRule("FREQ=DAILY;COUNT=3")
	s := &Set{Start: dt("20070310T023000"), Location: ny, Rules: []Rule{r},
		ExTimes: []time.Time{time.Date(2007, 3, 11, 7, 30, 0, 0, time.UTC)}}
	if got := collect(s.All(), 0); len(got) != 2 || got[1] != dt("20070312T023000") {
		t.Errorf("gap EXDATE: %v", got)
	}
	s = &Set{Start: dt("20071103T090000"), Location: ny,
		RTimes: []time.Time{time.Date(2007, 11, 4, 6, 30, 0, 0, time.UTC)}} // 01:30 EST
	var got []time.Time
	for at := range s.Times(time.Date(2007, 11, 1, 0, 0, 0, 0, time.UTC), time.Date(2007, 12, 1, 0, 0, 0, 0, time.UTC)) {
		got = append(got, at)
	}
	if len(got) != 2 || got[1].Unix() != time.Date(2007, 11, 4, 6, 30, 0, 0, time.UTC).Unix() {
		t.Errorf("fall-back RDATE: %v", got)
	}
}

// An EXRULE is the rule alone: DTSTART doesn't use up its COUNT.
func TestExRuleCount(t *testing.T) {
	r, _ := ParseRule("FREQ=DAILY;COUNT=10")
	ex, _ := ParseRule("FREQ=WEEKLY;BYDAY=TU;COUNT=2")
	s := &Set{Start: dt("20240101T090000"), Rules: []Rule{r}, ExRules: []Rule{ex}}
	got := collect(s.All(), 0)
	if len(got) != 8 || slices.Contains(got, dt("20240102T090000")) || slices.Contains(got, dt("20240109T090000")) {
		t.Errorf("got %v", got)
	}
}

// A BYSETPOS beyond every period's candidates ends at once, even for
// sub-daily rules with their long cycle bound.
func TestImpossibleSetPosEndsQuickly(t *testing.T) {
	for _, s := range []string{"FREQ=MINUTELY;BYSETPOS=2", "FREQ=SECONDLY;BYSETPOS=2", "FREQ=HOURLY;BYMINUTE=0,30;BYSETPOS=3,-3"} {
		r, _ := ParseRule(s)
		start := time.Now()
		if got := collect(r.All(dt("20240101T000000")), 0); len(got) != 0 {
			t.Errorf("%s: %v", s, got[:1])
		}
		if d := time.Since(start); d > 100*time.Millisecond {
			t.Errorf("%s took %v", s, d)
		}
	}
}

// A dense COUNT rule queried years after DTSTART counts the years in
// between arithmetically, and agrees with walking them.
func TestCountDaysFastAndExact(t *testing.T) {
	r, _ := ParseRule("FREQ=SECONDLY;COUNT=999999999")
	start := time.Now()
	got := collect(r.Between(dt("20240101T000000"), dt("20340101T000000"), dt("20340101T000100")), 0)
	if len(got) != 60 || got[0] != dt("20340101T000000") {
		t.Errorf("got %d instances from %v", len(got), got[:min(len(got), 1)])
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Errorf("took %v", d)
	}
	for _, s := range []string{
		"FREQ=HOURLY;INTERVAL=5;BYHOUR=1,6,11,16,21;COUNT=1000",
		"FREQ=MINUTELY;INTERVAL=7;BYHOUR=9,17;BYSECOND=0,30;COUNT=5000",
		"FREQ=SECONDLY;INTERVAL=13;BYMINUTE=0,1;BYSECOND=5,10,15;BYDAY=MO,TH;COUNT=3000",
		"FREQ=MINUTELY;INTERVAL=90;BYMONTH=2,3;BYSETPOS=1;BYSECOND=0,20;COUNT=4000",
	} {
		r, err := ParseRule(s)
		if err != nil {
			t.Fatal(err)
		}
		begin := dt("20240105T083000")
		full := collect(r.All(begin), 0)
		for _, i := range []int{len(full) / 3, len(full) - 2, len(full) - 1} {
			from := full[i].AddSeconds(-1)
			want := full[i:]
			if got := collect(r.Between(begin, from, dt("21000101T000000")), 0); !slices.Equal(got, want) {
				t.Fatalf("%s from %v: %d instances, want %d", s, from, len(got), len(want))
			}
		}
	}
}

// Two RTimes in the second of a fall-back night's repeated hours come out
// in instant order with the rule's instance between them.
func TestFallBackRTimesOrder(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	r, _ := ParseRule("FREQ=DAILY;COUNT=3")
	s := &Set{Start: dt("20071103T014500"), Location: ny, Rules: []Rule{r},
		RTimes: []time.Time{time.Date(2007, 11, 4, 6, 0, 0, 0, time.UTC), time.Date(2007, 11, 4, 6, 30, 0, 0, time.UTC)}}
	var got []time.Time
	for at := range s.Times(time.Date(2007, 11, 1, 0, 0, 0, 0, time.UTC), time.Date(2007, 12, 1, 0, 0, 0, 0, time.UTC)) {
		got = append(got, at)
	}
	if !slices.IsSortedFunc(got, time.Time.Compare) || len(got) != 5 {
		t.Errorf("got %v", got)
	}
}

// A dense EXRULE under a sparse RRULE seeks instead of stepping.
func TestDenseExRuleSeeks(t *testing.T) {
	r, _ := ParseRule("FREQ=MONTHLY")
	ex, _ := ParseRule("FREQ=SECONDLY;BYSECOND=1")
	s := &Set{Start: dt("20240101T090000"), Rules: []Rule{r}, ExRules: []Rule{ex}}
	start := time.Now()
	if got := collect(s.Between(dt("20240101T000000"), dt("20340101T000000")), 0); len(got) != 120 {
		t.Errorf("%d instances", len(got))
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Errorf("took %v", d)
	}
}

// ExDays exclude every instance on a day.
func TestExDays(t *testing.T) {
	r, _ := ParseRule("FREQ=HOURLY;INTERVAL=6;COUNT=12")
	s := &Set{Start: dt("20240101T000000"), Rules: []Rule{r}, ExDays: []civil.Date{{Year: 2024, Month: 1, Day: 2}}}
	for _, x := range collect(s.All(), 0) {
		if x.Date == (civil.Date{Year: 2024, Month: 1, Day: 2}) {
			t.Fatalf("%v is on an excluded day", x)
		}
	}
}
