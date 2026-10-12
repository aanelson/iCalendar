// Package civil provides zone-less dates and times: a wall-clock reading
// with no location attached.
//
// iCalendar needs these because several of its values have no zone at all.
// DATE values and floating DATE-TIME values (RFC 5545 §3.3.4, §3.3.5) are
// wall-clock readings, and recurrence rules are evaluated on the wall clock
// of DTSTART before any zone is applied (§3.3.10). A time.Time would force a
// location onto them, which is how time.Local leaks into results.
//
// The types follow the shape of cloud.google.com/go/civil. Arithmetic is on
// the proleptic Gregorian calendar, for any int year.
package civil

import (
	"cmp"
	"fmt"
	"time"
)

// A Date is a date on the proleptic Gregorian calendar, with no time of day
// and no location.
type Date struct {
	Year  int
	Month time.Month // 1 is January
	Day   int        // 1 is the first day of the month
}

// DateOf returns the date on t's wall clock, in t's location.
func DateOf(t time.Time) Date {
	y, m, d := t.Date()
	return Date{Year: y, Month: m, Day: d}
}

// IsValid reports whether d names a real day: a month of 1-12 and a day that
// exists in that month.
func (d Date) IsValid() bool {
	return d.Month >= time.January && d.Month <= time.December &&
		d.Day >= 1 && d.Day <= DaysIn(d.Year, d.Month)
}

// String returns d in ISO 8601 extended form, 2006-01-02.
func (d Date) String() string {
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, int(d.Month), d.Day)
}

// EpochDay returns the number of days from 1970-01-01 to d, negative before
// it. It is defined for invalid dates too, by normalizing them the way
// time.Date does: a Day past the month's end runs into the next month.
func (d Date) EpochDay() int64 {
	// Howard Hinnant's days_from_civil, on a year that starts in March so
	// the leap day is the last day of the year.
	y := int64(d.Year)
	m := int64(d.Month)
	// Normalize the month into 1-12 first, carrying whole years.
	if m < 1 || m > 12 {
		y += floorDiv(m-1, 12)
		m = floorMod(m-1, 12) + 1
	}
	if m <= 2 {
		y--
	}
	era := floorDiv(y, 400)
	yoe := y - era*400                     // [0, 399]
	mp := (m + 9) % 12                     // March is 0
	doy := (153*mp + 2) / 5                // the month's first day, [0, 337]
	doe := yoe*365 + yoe/4 - yoe/100 + doy // [0, 146096]
	// 719468 days separate 0000-03-01 from 1970-01-01. Day is added last,
	// unchecked, so an out-of-range Day normalizes into the next month.
	return era*146097 + doe - 719468 + int64(d.Day) - 1
}

// DateOfEpochDay returns the date n days after 1970-01-01.
func DateOfEpochDay(n int64) Date {
	z := n + 719468
	era := floorDiv(z, 146097)
	doe := z - era*146097                                  // [0, 146096]
	yoe := (doe - doe/1460 + doe/36524 - doe/146096) / 365 // [0, 399]
	y := yoe + era*400
	doy := doe - (365*yoe + yoe/4 - yoe/100) // [0, 365]
	mp := (5*doy + 2) / 153                  // [0, 11]
	d := doy - (153*mp+2)/5 + 1              // [1, 31]
	m := mp + 3
	if m > 12 {
		m -= 12
	}
	if m <= 2 {
		y++
	}
	return Date{Year: int(y), Month: time.Month(m), Day: int(d)}
}

// AddDays returns the date n days after d (before it, for negative n).
func (d Date) AddDays(n int) Date {
	return DateOfEpochDay(d.EpochDay() + int64(n))
}

// AddDate returns the date years, months and days after d, normalizing the
// way time.Time.AddDate does: October 31 plus one month is December 1.
func (d Date) AddDate(years, months, days int) Date {
	m := int64(d.Month) - 1 + int64(months)
	y := int64(d.Year) + int64(years) + floorDiv(m, 12)
	m = floorMod(m, 12) + 1
	return Date{Year: int(y), Month: time.Month(m), Day: 1}.AddDays(d.Day - 1 + days)
}

// DaysSince returns the number of days from s to d: positive when d is later.
func (d Date) DaysSince(s Date) int {
	return int(d.EpochDay() - s.EpochDay())
}

// Weekday returns the day of the week d falls on.
func (d Date) Weekday() time.Weekday {
	// 1970-01-01 was a Thursday.
	return time.Weekday(floorMod(d.EpochDay()+int64(time.Thursday), 7))
}

// YearDay returns the day of the year d falls on, 1 to 365, or 366 in a
// leap year.
func (d Date) YearDay() int {
	return int(d.EpochDay()-Date{Year: d.Year, Month: time.January, Day: 1}.EpochDay()) + 1
}

// Compare returns -1 if d is before d2, +1 if after, and 0 if they are the
// same date.
func (d Date) Compare(d2 Date) int {
	switch {
	case d.Year != d2.Year:
		return cmp.Compare(d.Year, d2.Year)
	case d.Month != d2.Month:
		return cmp.Compare(int(d.Month), int(d2.Month))
	default:
		return cmp.Compare(d.Day, d2.Day)
	}
}

// Before reports whether d is before d2.
func (d Date) Before(d2 Date) bool { return d.Compare(d2) < 0 }

// After reports whether d is after d2.
func (d Date) After(d2 Date) bool { return d.Compare(d2) > 0 }

// IsZero reports whether d is the zero Date, which is not a valid date.
func (d Date) IsZero() bool { return d == Date{} }

// In returns the instant d starts at in loc: midnight on d's wall clock,
// resolved as DateTime.In resolves it.
func (d Date) In(loc *time.Location) time.Time {
	return DateTime{Date: d}.In(loc)
}

// MarshalText implements encoding.TextMarshaler with the String form.
func (d Date) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler, reading the String form.
func (d *Date) UnmarshalText(b []byte) error {
	v, err := ParseDate(string(b))
	if err != nil {
		return err
	}
	*d = v
	return nil
}

// ParseDate reads an ISO 8601 extended date, 2006-01-02.
func ParseDate(s string) (Date, error) {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return Date{}, err
	}
	return DateOf(t), nil
}

// IsLeap reports whether year is a leap year on the Gregorian calendar.
func IsLeap(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

// DaysIn returns the number of days in month m of year.
func DaysIn(year int, m time.Month) int {
	switch m {
	case time.February:
		if IsLeap(year) {
			return 29
		}
		return 28
	case time.April, time.June, time.September, time.November:
		return 30
	default:
		return 31
	}
}

// DaysInYear returns 366 for a leap year and 365 otherwise.
func DaysInYear(year int) int {
	if IsLeap(year) {
		return 366
	}
	return 365
}

// A Time is a time of day with no date and no location.
type Time struct {
	Hour       int // 0-23
	Minute     int // 0-59
	Second     int // 0-59, or 60 for a leap second
	Nanosecond int // 0-999999999
}

// TimeOf returns the time of day on t's wall clock, in t's location.
func TimeOf(t time.Time) Time {
	h, m, s := t.Clock()
	return Time{Hour: h, Minute: m, Second: s, Nanosecond: t.Nanosecond()}
}

// IsValid reports whether every field of t is in range. A Second of 60 is
// valid: RFC 5545 §3.3.12 allows it for a positive leap second.
func (t Time) IsValid() bool {
	return t.Hour >= 0 && t.Hour < 24 &&
		t.Minute >= 0 && t.Minute < 60 &&
		t.Second >= 0 && t.Second <= 60 &&
		t.Nanosecond >= 0 && t.Nanosecond < 1e9
}

// String returns t in ISO 8601 extended form, 15:04:05, with a fraction
// only when Nanosecond is not zero.
func (t Time) String() string {
	s := fmt.Sprintf("%02d:%02d:%02d", t.Hour, t.Minute, t.Second)
	if t.Nanosecond != 0 {
		s += fmt.Sprintf(".%09d", t.Nanosecond)
	}
	return s
}

// SecondOfDay returns the whole seconds since midnight: Hour*3600 +
// Minute*60 + Second.
func (t Time) SecondOfDay() int {
	return t.Hour*3600 + t.Minute*60 + t.Second
}

// Compare returns -1 if t is before t2, +1 if after, and 0 if equal.
func (t Time) Compare(t2 Time) int {
	if c := cmp.Compare(t.SecondOfDay(), t2.SecondOfDay()); c != 0 {
		return c
	}
	return cmp.Compare(t.Nanosecond, t2.Nanosecond)
}

// Before reports whether t is before t2.
func (t Time) Before(t2 Time) bool { return t.Compare(t2) < 0 }

// After reports whether t is after t2.
func (t Time) After(t2 Time) bool { return t.Compare(t2) > 0 }

// IsZero reports whether t is midnight.
func (t Time) IsZero() bool { return t == Time{} }

// MarshalText implements encoding.TextMarshaler with the String form.
func (t Time) MarshalText() ([]byte, error) { return []byte(t.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler, reading the String form.
func (t *Time) UnmarshalText(b []byte) error {
	v, err := ParseTime(string(b))
	if err != nil {
		return err
	}
	*t = v
	return nil
}

// ParseTime reads an ISO 8601 extended time of day, 15:04:05 with an
// optional fraction.
func ParseTime(s string) (Time, error) {
	t, err := time.Parse("15:04:05.999999999", s)
	if err != nil {
		return Time{}, err
	}
	return TimeOf(t), nil
}

// A DateTime is a date and a time of day with no location: a wall-clock
// reading. The Date and Time fields are embedded, so dt.Year and dt.Hour
// work directly.
type DateTime struct {
	Date
	Time
}

// DateTimeOf returns the wall-clock reading of t in t's location.
func DateTimeOf(t time.Time) DateTime {
	return DateTime{Date: DateOf(t), Time: TimeOf(t)}
}

// IsValid reports whether both the date and the time of day are valid.
func (dt DateTime) IsValid() bool {
	return dt.Date.IsValid() && dt.Time.IsValid()
}

// String returns dt in ISO 8601 extended form, 2006-01-02T15:04:05.
func (dt DateTime) String() string {
	return dt.Date.String() + "T" + dt.Time.String()
}

// Compare returns -1 if dt is before dt2, +1 if after, and 0 if equal.
func (dt DateTime) Compare(dt2 DateTime) int {
	if c := dt.Date.Compare(dt2.Date); c != 0 {
		return c
	}
	return dt.Time.Compare(dt2.Time)
}

// Before reports whether dt is before dt2.
func (dt DateTime) Before(dt2 DateTime) bool { return dt.Compare(dt2) < 0 }

// After reports whether dt is after dt2.
func (dt DateTime) After(dt2 DateTime) bool { return dt.Compare(dt2) > 0 }

// IsZero reports whether dt is the zero DateTime.
func (dt DateTime) IsZero() bool { return dt == DateTime{} }

// EpochSecond returns the seconds from 1970-01-01T00:00:00 to dt's wall
// clock, as though dt were in UTC. It ignores Nanosecond.
func (dt DateTime) EpochSecond() int64 {
	return dt.EpochDay()*86400 + int64(dt.SecondOfDay())
}

// DateTimeOfEpochSecond is the inverse of EpochSecond.
func DateTimeOfEpochSecond(s int64) DateTime {
	day := floorDiv(s, 86400)
	sec := int(s - day*86400)
	return DateTime{
		Date: DateOfEpochDay(day),
		Time: Time{Hour: sec / 3600, Minute: sec / 60 % 60, Second: sec % 60},
	}
}

// AddSeconds returns the wall-clock reading n seconds after dt, with no
// regard to any zone's transitions.
func (dt DateTime) AddSeconds(n int64) DateTime {
	r := DateTimeOfEpochSecond(dt.EpochSecond() + n)
	r.Nanosecond = dt.Nanosecond
	return r
}

// AddDate returns dt with its date moved as Date.AddDate moves it and its
// time of day unchanged.
func (dt DateTime) AddDate(years, months, days int) DateTime {
	return DateTime{Date: dt.Date.AddDate(years, months, days), Time: dt.Time}
}

// In returns the instant dt names on loc's wall clock, resolving the two
// cases where the wall clock is not one-to-one the way RFC 5545 §3.3.5 does:
//
//   - A reading that occurs twice, when clocks go back, names the first
//     occurrence: 01:30 on 2007-11-04 in America/New_York is 01:30 EDT.
//   - A reading that never occurs, when clocks go forward, is read with
//     the offset in force before the gap: 02:30 on 2007-03-11 in
//     America/New_York is 03:30 EDT.
//
// time.Date makes no promise for either case, and in practice moves gap
// readings backwards. A nil loc is UTC. A Second of 60 is read as the first
// second of the next minute.
func (dt DateTime) In(loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	wall := dt.EpochSecond()
	ns := int64(dt.Nanosecond)
	if loc == time.UTC {
		return time.Unix(wall, ns).UTC()
	}
	// The offsets in force a day either side of the reading cover every
	// transition near it. Zones change offset at most once in any two days.
	before := offsetAt(loc, wall-86400)
	after := offsetAt(loc, wall+86400)
	first, second := wall-before, wall-after
	firstOK := offsetAt(loc, first) == before
	secondOK := offsetAt(loc, second) == after
	var u int64
	switch {
	case firstOK && secondOK:
		u = min(first, second) // the reading occurs twice; take the first
	case firstOK:
		u = first
	case secondOK:
		u = second
	default:
		u = first // a gap: the offset before it
	}
	return time.Unix(u, ns).In(loc)
}

// offsetAt returns loc's UTC offset in seconds at the instant unix.
func offsetAt(loc *time.Location, unix int64) int64 {
	_, off := time.Unix(unix, 0).In(loc).Zone()
	return int64(off)
}

// MarshalText implements encoding.TextMarshaler with the String form.
func (dt DateTime) MarshalText() ([]byte, error) { return []byte(dt.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler, reading the String form.
func (dt *DateTime) UnmarshalText(b []byte) error {
	v, err := ParseDateTime(string(b))
	if err != nil {
		return err
	}
	*dt = v
	return nil
}

// ParseDateTime reads an ISO 8601 extended date and time with no zone,
// 2006-01-02T15:04:05 with an optional fraction.
func ParseDateTime(s string) (DateTime, error) {
	t, err := time.Parse("2006-01-02T15:04:05.999999999", s)
	if err != nil {
		return DateTime{}, err
	}
	return DateTimeOf(t), nil
}

func floorDiv(a, b int64) int64 {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

func floorMod(a, b int64) int64 {
	return a - floorDiv(a, b)*b
}
