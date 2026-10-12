package civil

import (
	"testing"
	"time"
)

// The epoch-day arithmetic is checked against package time over 1600-2400
// and a sample of distant years, including negative ones.
func TestEpochDayMatchesTime(t *testing.T) {
	check := func(d Date) {
		t.Helper()
		want := time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC).Unix() / 86400
		if got := d.EpochDay(); got != want {
			t.Fatalf("%v.EpochDay() = %d, want %d", d, got, want)
		}
		if got := DateOfEpochDay(want); got != d {
			t.Fatalf("DateOfEpochDay(%d) = %v, want %v", want, got, d)
		}
		if got, want := d.Weekday(), time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC).Weekday(); got != want {
			t.Fatalf("%v.Weekday() = %v, want %v", d, got, want)
		}
		if got, want := d.YearDay(), time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC).YearDay(); got != want {
			t.Fatalf("%v.YearDay() = %d, want %d", d, got, want)
		}
	}
	for d := (Date{1600, time.January, 1}); d.Year < 2400; d = d.AddDays(1) {
		check(d)
	}
	for _, y := range []int{-4800, -401, -400, -1, 0, 1, 4, 100, 9999, 10000, 123456} {
		for m := time.January; m <= time.December; m++ {
			check(Date{y, m, 1})
			check(Date{y, m, DaysIn(y, m)})
		}
	}
}

func TestEpochDayNormalizes(t *testing.T) {
	for _, tc := range []struct {
		in, want Date
	}{
		{Date{2023, time.February, 29}, Date{2023, time.March, 1}},
		{Date{2024, time.February, 30}, Date{2024, time.March, 1}},
		{Date{2024, time.January, 0}, Date{2023, time.December, 31}},
		{Date{2024, 13, 1}, Date{2025, time.January, 1}},
		{Date{2024, 0, 1}, Date{2023, time.December, 1}},
		{Date{2024, -11, 1}, Date{2023, time.January, 1}},
	} {
		if got := DateOfEpochDay(tc.in.EpochDay()); got != tc.want {
			t.Errorf("%v normalizes to %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestAddDateMatchesTime(t *testing.T) {
	base := []Date{{2024, time.January, 31}, {2024, time.February, 29}, {2023, time.December, 31}, {1999, time.March, 1}}
	for _, d := range base {
		for _, delta := range [][3]int{{0, 1, 0}, {0, -1, 0}, {1, 0, 0}, {-1, 0, 0}, {0, 13, 5}, {0, -25, -40}, {3, 2, 1}} {
			got := d.AddDate(delta[0], delta[1], delta[2])
			want := DateOf(time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC).AddDate(delta[0], delta[1], delta[2]))
			if got != want {
				t.Errorf("%v.AddDate%v = %v, want %v", d, delta, got, want)
			}
		}
	}
}

func TestIsValid(t *testing.T) {
	valid := []Date{{2024, time.February, 29}, {2000, time.February, 29}, {2023, time.December, 31}, {0, time.January, 1}}
	invalid := []Date{{2023, time.February, 29}, {1900, time.February, 29}, {2024, time.April, 31}, {2024, 13, 1}, {2024, 0, 1}, {2024, 1, 0}, {}}
	for _, d := range valid {
		if !d.IsValid() {
			t.Errorf("%v.IsValid() = false", d)
		}
	}
	for _, d := range invalid {
		if d.IsValid() {
			t.Errorf("%v.IsValid() = true", d)
		}
	}
	if !(Time{23, 59, 60, 0}).IsValid() {
		t.Error("a leap second is valid")
	}
	if (Time{24, 0, 0, 0}).IsValid() {
		t.Error("hour 24 is not valid")
	}
}

func TestCompare(t *testing.T) {
	a := DateTime{Date{2024, 3, 10}, Time{9, 0, 0, 0}}
	b := DateTime{Date{2024, 3, 10}, Time{9, 0, 0, 1}}
	c := DateTime{Date{2024, 3, 11}, Time{0, 0, 0, 0}}
	if !a.Before(b) || !b.Before(c) || !c.After(a) || a.Compare(a) != 0 {
		t.Error("DateTime ordering is wrong")
	}
}

func TestEpochSecondRoundTrip(t *testing.T) {
	for _, s := range []int64{-86401, -86400, -1, 0, 1, 86399, 86400, 1700000000, 253402300799} {
		dt := DateTimeOfEpochSecond(s)
		if got := dt.EpochSecond(); got != s {
			t.Errorf("DateTimeOfEpochSecond(%d) = %v, which is %d", s, dt, got)
		}
		if want := DateTimeOf(time.Unix(s, 0).UTC()); dt != want {
			t.Errorf("DateTimeOfEpochSecond(%d) = %v, want %v", s, dt, want)
		}
	}
}

// RFC 5545 §3.3.5 gives the two worked examples this resolution follows.
func TestInRFC5545(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no tzdata:", err)
	}
	berlin, _ := time.LoadLocation("Europe/Berlin")
	lordHowe, _ := time.LoadLocation("Australia/Lord_Howe") // a 30-minute DST shift
	for _, tc := range []struct {
		loc  *time.Location
		wall string
		want string // RFC 3339
	}{
		// Repeated: the first occurrence.
		{ny, "2007-11-04T01:30:00", "2007-11-04T01:30:00-04:00"},
		// Missing: the offset before the gap, so 02:30 EST is 03:30 EDT.
		{ny, "2007-03-11T02:30:00", "2007-03-11T03:30:00-04:00"},
		{ny, "2007-03-11T02:00:00", "2007-03-11T03:00:00-04:00"},
		{ny, "2007-03-11T03:00:00", "2007-03-11T03:00:00-04:00"},
		{ny, "2007-03-11T01:59:59", "2007-03-11T01:59:59-05:00"},
		{ny, "2007-11-04T00:59:59", "2007-11-04T00:59:59-04:00"},
		{ny, "2007-11-04T02:00:00", "2007-11-04T02:00:00-05:00"},
		{berlin, "2024-03-31T02:30:00", "2024-03-31T03:30:00+02:00"},
		{berlin, "2024-10-27T02:30:00", "2024-10-27T02:30:00+02:00"},
		{lordHowe, "2024-10-06T02:15:00", "2024-10-06T02:45:00+11:00"},
		{lordHowe, "2024-04-07T01:45:00", "2024-04-07T01:45:00+11:00"},
		{nil, "2024-01-01T00:00:00", "2024-01-01T00:00:00Z"},
	} {
		dt, err := ParseDateTime(tc.wall)
		if err != nil {
			t.Fatal(err)
		}
		got := dt.In(tc.loc).Format(time.RFC3339)
		if got != tc.want {
			t.Errorf("%s in %v = %s, want %s", tc.wall, tc.loc, got, tc.want)
		}
	}
}

func TestInMatchesTimeDateAwayFromTransitions(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no tzdata:", err)
	}
	for d := (Date{2020, time.January, 1}); d.Year < 2022; d = d.AddDays(1) {
		dt := DateTime{d, Time{12, 0, 0, 0}}
		got := dt.In(ny)
		want := time.Date(d.Year, d.Month, d.Day, 12, 0, 0, 0, ny)
		if !got.Equal(want) {
			t.Fatalf("%v.In(NY) = %v, want %v", dt, got, want)
		}
	}
}

func TestText(t *testing.T) {
	dt := DateTime{Date{2024, 2, 29}, Time{23, 5, 9, 120}}
	b, _ := dt.MarshalText()
	var back DateTime
	if err := back.UnmarshalText(b); err != nil || back != dt {
		t.Errorf("round trip of %v gave %v, %v", dt, back, err)
	}
	if s := (Date{3, 1, 2}).String(); s != "0003-01-02" {
		t.Errorf("String = %q", s)
	}
}
