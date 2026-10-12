package tz

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aanelson/iCalendar/civil"
	"github.com/aanelson/iCalendar/recur"
)

func mustRule(t *testing.T, s string) recur.Rule {
	t.Helper()
	r, err := recur.ParseRule(s)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func wall(y int, m time.Month, d, h, min int) civil.DateTime {
	return civil.DateTime{Date: civil.Date{Year: y, Month: m, Day: d}, Time: civil.Time{Hour: h, Minute: min}}
}

// usEastern is America/New_York since 2007, written the way Outlook and
// most producers write it, with DTSTART in 1601 or 1970.
func usEastern(t *testing.T, tzid string, start int) Definition {
	return Definition{TZID: tzid, Observances: []Observance{
		{Start: wall(start, 11, 4, 2, 0), OffsetFrom: -4 * 3600, OffsetTo: -5 * 3600, Name: "EST",
			Rules: []recur.Rule{mustRule(t, "FREQ=YEARLY;BYMONTH=11;BYDAY=1SU")}},
		{Daylight: true, Start: wall(start, 3, 11, 2, 0), OffsetFrom: -5 * 3600, OffsetTo: -4 * 3600, Name: "EDT",
			Rules: []recur.Rule{mustRule(t, "FREQ=YEARLY;BYMONTH=3;BYDAY=2SU")}},
	}}
}

func loadOrSkip(t *testing.T, name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skip("no tzdata:", err)
	}
	return loc
}

// A compiled VTIMEZONE agrees with the IANA zone hour by hour, inside
// the explicit transitions and past the horizon, where the POSIX footer
// takes over.
func TestCompileMatchesIANA(t *testing.T) {
	ny := loadOrSkip(t, "America/New_York")
	for _, start := range []int{1601, 1970, 2007} {
		loc, err := Compile(usEastern(t, "Eastern", start))
		if err != nil {
			t.Fatal(err)
		}
		for _, span := range [][2]int{{2007, 2030}, {2195, 2210}} {
			for u := time.Date(span[0], 1, 1, 0, 0, 0, 0, time.UTC); u.Year() < span[1]; u = u.Add(time.Hour) {
				gotName, got := u.In(loc).Zone()
				wantName, want := u.In(ny).Zone()
				if got != want || gotName != wantName {
					t.Fatalf("start %d: at %v got %s %d, want %s %d", start, u, gotName, got, wantName, want)
				}
			}
		}
	}
}

// RFC 5545 §3.3.5's examples hold for a compiled zone too.
func TestCompiledZoneWallClock(t *testing.T) {
	loc, err := Compile(usEastern(t, "Eastern", 1970))
	if err != nil {
		t.Fatal(err)
	}
	if got := wall(2007, 3, 11, 2, 30).In(loc).Format(time.RFC3339); got != "2007-03-11T03:30:00-04:00" {
		t.Errorf("gap: %s", got)
	}
	if got := wall(2007, 11, 4, 1, 30).In(loc).Format(time.RFC3339); got != "2007-11-04T01:30:00-04:00" {
		t.Errorf("repeat: %s", got)
	}
	if loc.String() != "Eastern" {
		t.Errorf("name %q", loc.String())
	}
}

// A zone whose rules end (here: DST abolished after 2015) stays at its
// last offset, and times before the first onset use the first
// observance's TZOFFSETFROM.
func TestCompileEndingRules(t *testing.T) {
	def := Definition{TZID: "Custom", Observances: []Observance{
		{Start: wall(2010, 10, 31, 3, 0), OffsetFrom: 4 * 3600, OffsetTo: 3 * 3600, Name: "STD",
			Rules: []recur.Rule{mustRule(t, "FREQ=YEARLY;BYMONTH=10;BYDAY=-1SU;UNTIL=20141026T000000Z")}},
		{Daylight: true, Start: wall(2011, 3, 27, 2, 0), OffsetFrom: 3 * 3600, OffsetTo: 4 * 3600, Name: "DST",
			Rules: []recur.Rule{mustRule(t, "FREQ=YEARLY;BYMONTH=3;BYDAY=-1SU;UNTIL=20150329T000000Z")}},
	}}
	loc, err := Compile(def)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		at   time.Time
		want int
	}{
		{time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), 4 * 3600}, // before any onset: TZOFFSETFROM
		{time.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC), 3 * 3600},
		{time.Date(2012, 7, 1, 0, 0, 0, 0, time.UTC), 4 * 3600},
		{time.Date(2016, 1, 1, 0, 0, 0, 0, time.UTC), 4 * 3600}, // the last onset (DST, 2015) holds
		{time.Date(2300, 1, 1, 0, 0, 0, 0, time.UTC), 4 * 3600},
	} {
		if _, off := tc.at.In(loc).Zone(); off != tc.want {
			t.Errorf("at %v: offset %d, want %d", tc.at, off, tc.want)
		}
	}
}

// The old RFC 2445 way of writing the second Sunday still gets a footer.
func TestPosixFooterForms(t *testing.T) {
	def := usEastern(t, "Eastern", 2007)
	def.Observances[1].Rules = []recur.Rule{mustRule(t, "FREQ=YEARLY;BYMONTH=3;BYDAY=SU;BYMONTHDAY=8,9,10,11,12,13,14")}
	if got := posixFooter(def.Observances, nil); got != "<EST>5<EDT>4,M3.2.0,M11.1.0" {
		t.Errorf("footer %q", got)
	}
	def.Observances[1].Rules = []recur.Rule{mustRule(t, "FREQ=YEARLY;BYMONTH=3;BYDAY=-1SU")}
	def.Observances[1].Start.Minute = 30
	if got := posixFooter(def.Observances, nil); got != "<EST>5<EDT>4,M3.5.0/2:30,M11.1.0" {
		t.Errorf("footer %q", got)
	}
	def.Observances[1].Rules = []recur.Rule{mustRule(t, "FREQ=YEARLY;BYMONTH=3;BYMONTHDAY=20")}
	if got := posixFooter(def.Observances, nil); got != "" {
		t.Errorf("an inexpressible rule gave %q", got)
	}
}

func TestCompileCaches(t *testing.T) {
	a, err := Compile(usEastern(t, "Cached", 1970))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Compile(usEastern(t, "Cached", 1970))
	if a != b {
		t.Error("the same definition compiled twice")
	}
	c, _ := Compile(usEastern(t, "Cached", 1601))
	if a == c {
		t.Error("different definitions shared a location")
	}
}

func TestCompileRejects(t *testing.T) {
	for _, def := range []Definition{
		{TZID: "empty"},
		{TZID: "bad offset", Observances: []Observance{{Start: wall(2000, 1, 1, 0, 0), OffsetTo: 100000}}},
		{TZID: "bad start", Observances: []Observance{{Start: wall(2000, 2, 30, 0, 0)}}},
	} {
		if _, err := Compile(def); !errors.Is(err, ErrInvalidDefinition) {
			t.Errorf("%s: %v", def.TZID, err)
		}
	}
}

func TestLoad(t *testing.T) {
	ny := loadOrSkip(t, "America/New_York")
	for _, tzid := range []string{
		"America/New_York",
		`"America/New_York"`,
		"america/new_york",
		"/mozilla.org/20050126_1/America/New_York",
		"/softwarestudio.org/Olson_20011030_5/America/New_York",
		"/America/New_York",
		"/./America/New_York",
		"Eastern Standard Time",
	} {
		loc, err := Load(tzid)
		if err != nil {
			t.Errorf("Load(%q): %v", tzid, err)
			continue
		}
		if strings.HasPrefix(loc.String(), ".") {
			t.Errorf("Load(%q) is named %q", tzid, loc)
		}
		at := time.Date(2024, 7, 1, 12, 0, 0, 0, time.UTC)
		if _, off := at.In(loc).Zone(); off != func() int { _, o := at.In(ny).Zone(); return o }() {
			t.Errorf("Load(%q) is %v", tzid, loc)
		}
	}
	for tzid, want := range map[string]int{"UTC-04:00": -4 * 3600, "GMT+0530": 19800, "UTC+5": 5 * 3600, "utc": 0} {
		loc, err := Load(tzid)
		if err != nil {
			t.Errorf("Load(%q): %v", tzid, err)
			continue
		}
		if _, off := time.Unix(0, 0).In(loc).Zone(); off != want {
			t.Errorf("Load(%q) offset %d, want %d", tzid, off, want)
		}
	}
	for _, tzid := range []string{"", "Local", "Nowhere/Special", "(UTC-05:00) Eastern Time (US & Canada)", "../../etc/passwd", "/etc/localtime", "localtime", "posix/localtime", "LocalTime", "/usr/share/zoneinfo/localtime", "/", "//", `"/"`, "UTC+15", "GMT-04:75", "UTCX"} {
		if _, err := Load(tzid); !errors.Is(err, ErrUnknownZone) {
			t.Errorf("Load(%q) = %v, want ErrUnknownZone", tzid, err)
		}
	}
}

// Debian links /usr/share/zoneinfo/localtime to the host's zone, which a
// calendar can't mean. Go reads ZONEINFO once per process, so a child
// process gets a zoneinfo directory laid out that way.
func TestLoadRefusesHostZone(t *testing.T) {
	if os.Getenv("TZ_TEST_CHILD") == "1" {
		for _, tzid := range []string{"localtime", "LOCALTIME", "/etc/localtime", "/x.org/localtime"} {
			if loc, err := Load(tzid); err == nil {
				fmt.Printf("Load(%q) = %v\n", tzid, loc)
			}
		}
		if _, err := Load("Asia/Kolkata"); err != nil {
			fmt.Println("the zoneinfo directory isn't in use:", err)
		}
		return
	}
	zone, err := os.ReadFile("/usr/share/zoneinfo/Asia/Kolkata")
	if err != nil {
		t.Skip("no zoneinfo directory:", err)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "Asia"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"localtime", "Asia/Kolkata"} {
		if err := os.WriteFile(filepath.Join(dir, name), zone, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestLoadRefusesHostZone$")
	cmd.Env = append(os.Environ(), "TZ_TEST_CHILD=1", "ZONEINFO="+dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	// Only the child's own lines: a coverage build adds its own.
	for line := range strings.Lines(string(out)) {
		if strings.HasPrefix(line, "Load(") || strings.HasPrefix(line, "the zoneinfo") {
			t.Error(strings.TrimSpace(line))
		}
	}
}

// Hostile definitions are refused or bounded rather than compiled into a
// wrong zone or an enormous one.
func TestCompileLimits(t *testing.T) {
	var many Definition
	many.TZID = "many"
	for i := range maxObservances + 1 {
		many.Observances = append(many.Observances, Observance{Start: wall(2000, 1, 1+i%28, 0, 0), OffsetFrom: 3600, OffsetTo: 0})
	}
	if _, err := Compile(many); !errors.Is(err, ErrInvalidDefinition) {
		t.Errorf("too many observances: %v", err)
	}
	dense := Definition{TZID: "dense"}
	for i := range 3 {
		dense.Observances = append(dense.Observances, Observance{Start: wall(2000, 1, 1, i, 0), OffsetFrom: 0, OffsetTo: 3600 * (i + 1),
			Rules: []recur.Rule{mustRule(t, "FREQ=HOURLY")}})
	}
	if _, err := Compile(dense); !errors.Is(err, ErrInvalidDefinition) {
		t.Errorf("too many onsets across observances: %v", err)
	}
	types := Definition{TZID: "types"}
	for i := range 300 {
		types.Observances = append(types.Observances, Observance{Start: wall(2000+i, 1, 1, 0, 0), OffsetFrom: 0, OffsetTo: 60 * i, Name: fmt.Sprintf("Z%03d", i)})
	}
	if _, err := Compile(types); !errors.Is(err, ErrInvalidDefinition) {
		t.Errorf("too many local time types: %v", err)
	}
	long := usEastern(t, "long", 1970)
	long.Observances[0].Name = strings.Repeat("S", 300)
	long.Observances[1].Name = strings.Repeat("D", 300)
	loc, err := Compile(long)
	if err != nil {
		t.Fatalf("long names: %v", err)
	}
	if name, _ := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC).In(loc).Zone(); name != strings.Repeat("D", maxNameLen) {
		t.Errorf("long DST name became %q", name)
	}
}
