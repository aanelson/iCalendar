// Package icaltime reads and writes the basic-format DATE and DATE-TIME
// text of RFC 5545 §3.3.4 and §3.3.5 (19970902, 19970902T090000,
// 19970902T090000Z). It is shared by the packages that meet these values
// outside the model: RRULE's UNTIL, VTIMEZONE observances, and the model's
// own value codecs.
package icaltime

import (
	"errors"
	"time"

	"github.com/aanelson/iCalendar/civil"
)

// ErrSyntax is returned for text that is not a DATE or DATE-TIME.
var ErrSyntax = errors.New("not an RFC 5545 DATE or DATE-TIME")

// ErrRange is returned for a DATE or DATE-TIME whose fields are out of
// range, like February 30 or hour 24.
var ErrRange = errors.New("date or time out of range")

// Parse reads s as a DATE (8 digits) or a DATE-TIME (8 digits, T, 6
// digits, optional Z). isDate reports a DATE; utc reports the Z form.
func Parse(s string) (v civil.DateTime, isDate, utc bool, err error) {
	switch {
	case len(s) == 8:
		d, err := ParseDate(s)
		return civil.DateTime{Date: d}, true, false, err
	case len(s) == 15 || len(s) == 16 && s[15] == 'Z':
		v, err := ParseDateTime(s[:15])
		return v, false, len(s) == 16, err
	}
	return civil.DateTime{}, false, false, ErrSyntax
}

// ParseDate reads exactly 8 digits, YYYYMMDD.
func ParseDate(s string) (civil.Date, error) {
	if len(s) != 8 {
		return civil.Date{}, ErrSyntax
	}
	y, ok1 := digits(s[0:4])
	m, ok2 := digits(s[4:6])
	d, ok3 := digits(s[6:8])
	if !ok1 || !ok2 || !ok3 {
		return civil.Date{}, ErrSyntax
	}
	date := civil.Date{Year: y, Month: time.Month(m), Day: d}
	if !date.IsValid() {
		return date, ErrRange
	}
	return date, nil
}

// ParseDateTime reads exactly YYYYMMDDTHHMMSS, with no zone suffix.
func ParseDateTime(s string) (civil.DateTime, error) {
	if len(s) != 15 || s[8] != 'T' && s[8] != 't' {
		return civil.DateTime{}, ErrSyntax
	}
	d, err := ParseDate(s[:8])
	if err != nil && err != ErrRange {
		return civil.DateTime{}, err
	}
	h, ok1 := digits(s[9:11])
	mi, ok2 := digits(s[11:13])
	sec, ok3 := digits(s[13:15])
	if !ok1 || !ok2 || !ok3 {
		return civil.DateTime{}, ErrSyntax
	}
	v := civil.DateTime{Date: d, Time: civil.Time{Hour: h, Minute: mi, Second: sec}}
	if err != nil || !v.Time.IsValid() {
		return v, ErrRange
	}
	return v, nil
}

func digits(s string) (int, bool) {
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

// AppendDate appends d as YYYYMMDD. Years outside 0-9999 can't be written
// in this form and are clamped into it.
func AppendDate(b []byte, d civil.Date) []byte {
	b = appendInt(b, min(max(d.Year, 0), 9999), 4)
	b = appendInt(b, int(d.Month), 2)
	return appendInt(b, d.Day, 2)
}

// AppendDateTime appends v as YYYYMMDDTHHMMSS, and a Z when utc is set.
func AppendDateTime(b []byte, v civil.DateTime, utc bool) []byte {
	b = AppendDate(b, v.Date)
	b = append(b, 'T')
	b = appendInt(b, v.Hour, 2)
	b = appendInt(b, v.Minute, 2)
	b = appendInt(b, v.Second, 2)
	if utc {
		b = append(b, 'Z')
	}
	return b
}

// Format returns v as Parse reads it: a DATE when isDate is set, otherwise a
// DATE-TIME with a Z when utc is set.
func Format(v civil.DateTime, isDate, utc bool) string {
	if isDate {
		return string(AppendDate(nil, v.Date))
	}
	return string(AppendDateTime(nil, v, utc))
}

func appendInt(b []byte, n, width int) []byte {
	var buf [20]byte
	i := len(buf)
	for n > 0 || width > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
		width--
	}
	return append(b, buf[i:]...)
}
