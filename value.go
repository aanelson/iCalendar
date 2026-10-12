package ical

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aanelson/iCalendar/civil"
	"github.com/aanelson/iCalendar/internal/icaltime"
	"github.com/aanelson/iCalendar/recur"
	"github.com/aanelson/iCalendar/tz"
)

// A ValueError reports a property value that doesn't parse as its type.
type ValueError struct {
	Property string // the property name
	Value    string // the value as written
	Err      error
}

func (e *ValueError) Error() string {
	return fmt.Sprintf("ical: %s value %q: %v", e.Property, e.Value, e.Err)
}

func (e *ValueError) Unwrap() error { return e.Err }

func (p *Property) valueError(err error) error {
	return &ValueError{Property: p.Name, Value: p.Value, Err: err}
}

// TEXT (RFC 5545 §3.3.11)

// Text returns the value read as a single TEXT: escapes decoded, commas
// kept. It never fails: an unknown escape like \: reads as the character
// after the backslash, which is how most readers treat what producers
// write.
func (p *Property) Text() string {
	return UnescapeText(p.Value)
}

// Texts returns the value read as a comma-separated list of TEXT, as for
// CATEGORIES and RESOURCES. Escaped commas (\,) stay inside an item.
func (p *Property) Texts() []string {
	var out []string
	start := 0
	for i := 0; i < len(p.Value); i++ {
		switch p.Value[i] {
		case '\\':
			i++
		case ',':
			out = append(out, UnescapeText(p.Value[start:i]))
			start = i + 1
		}
	}
	return append(out, UnescapeText(p.Value[start:]))
}

// SetText sets the value to s, escaped as one TEXT.
func (p *Property) SetText(s string) {
	p.Value = EscapeText(s)
}

// SetTexts sets the value to a comma-separated list of TEXT.
func (p *Property) SetTexts(items ...string) {
	esc := make([]string, len(items))
	for i, s := range items {
		esc[i] = EscapeText(s)
	}
	p.Value = strings.Join(esc, ",")
}

// UnescapeText decodes TEXT escapes: \\ \; \, and \n or \N for a newline.
// A backslash before any other character is dropped; one at the very end
// is kept.
func UnescapeText(s string) string {
	i := strings.IndexByte(s, '\\')
	if i < 0 {
		return s
	}
	b := make([]byte, 0, len(s))
	b = append(b, s[:i]...)
	for ; i < len(s); i++ {
		c := s[i]
		if c != '\\' || i+1 == len(s) {
			b = append(b, c)
			continue
		}
		i++
		switch s[i] {
		case 'n', 'N':
			b = append(b, '\n')
		default:
			b = append(b, s[i])
		}
	}
	return string(b)
}

// EscapeText encodes s as TEXT: backslash, semicolon and comma escaped,
// each newline (CRLF, LF or CR) written as \n. Other control characters
// can't appear in a content line and are dropped.
func EscapeText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '\\', ';', ',':
			b.WriteByte('\\')
			b.WriteByte(c)
		case '\r':
			if i+1 < len(s) && s[i+1] == '\n' {
				i++
			}
			b.WriteString(`\n`)
		case '\n':
			b.WriteString(`\n`)
		default:
			if c < 0x20 && c != '\t' || c == 0x7f {
				continue
			}
			b.WriteByte(c)
		}
	}
	return b.String()
}

// DATE and DATE-TIME (RFC 5545 §3.3.4, §3.3.5)

// A DateTime is a DATE or DATE-TIME value. It keeps the form it was
// written in, which RFC 5545 gives three meanings: a floating wall-clock
// time, a UTC time, or a wall-clock time in a named zone. A DATE is a
// floating date.
type DateTime struct {
	// DateTime is the date and wall-clock time as written; for a DATE the
	// time of day is zero.
	civil.DateTime
	// IsDate marks a DATE value (VALUE=DATE).
	IsDate bool
	// UTC marks the form with a trailing Z.
	UTC bool
	// TZID is the zone the wall clock is in, or "" for floating and UTC
	// values.
	TZID string
	// Loc is TZID resolved, or nil when TZID is "" or couldn't be resolved.
	Loc *time.Location

	// exact is the instant a computation (Add, an expansion) produced, for
	// the one case the wall clock can't carry: the second of two identical
	// readings when clocks go back. It counts only while the wall clock
	// still reads exactWall.
	exact     int64
	exactWall civil.DateTime
	hasExact  bool
}

// withInstant returns v remembering that it is the instant t.
func (v DateTime) withInstant(t time.Time) DateTime {
	v.exact, v.exactWall, v.hasExact = t.Unix(), v.DateTime, true
	return v
}

// NewDate returns a DATE value.
func NewDate(d civil.Date) DateTime {
	return DateTime{DateTime: civil.DateTime{Date: d}, IsDate: true}
}

// NewFloating returns a floating DATE-TIME: a wall-clock time in no zone.
func NewFloating(dt civil.DateTime) DateTime {
	dt.Nanosecond = 0
	return DateTime{DateTime: dt}
}

// NewDateTime returns t as a DATE-TIME: on t's wall clock with its
// location's name as the TZID when tz.Load can find a zone by that name,
// and in UTC otherwise, so the instant survives. That covers UTC itself,
// time.Local (a name no reader could look up), and the unnamed fixed
// offsets time.Parse makes for RFC 3339 times like 09:00:00-05:00.
func NewDateTime(t time.Time) DateTime {
	t = t.Truncate(time.Second)
	loc := t.Location()
	if loc != time.UTC && loc != time.Local && loc.String() != "" {
		if _, err := tz.Load(loc.String()); err == nil {
			v := DateTime{DateTime: civil.DateTimeOf(t), TZID: loc.String(), Loc: loc}
			if secondReading(t) {
				v = v.withInstant(t)
			}
			return v
		}
	}
	return DateTime{DateTime: civil.DateTimeOf(t.UTC()), UTC: true}
}

// secondReading reports whether t's wall clock read the same a moment
// earlier, in the overlap after clocks go back. A TZID's wall clock names
// the first reading (RFC 5545 §3.3.5), so such a t needs its instant kept.
func secondReading(t time.Time) bool {
	start, _ := t.ZoneBounds()
	if start.IsZero() {
		return false
	}
	_, off := t.Zone()
	_, before := start.Add(-time.Second).Zone()
	return before > off && t.Before(start.Add(time.Duration(before-off)*time.Second))
}

// Floating reports whether v is in no zone: a DATE or a floating
// DATE-TIME.
func (v DateTime) Floating() bool { return !v.UTC && v.TZID == "" }

// unresolved reports whether v names a zone that couldn't be resolved, so
// it has no instant: Time's UTC reading of it is only a fallback.
func (v DateTime) unresolved() bool { return v.TZID != "" && v.Loc == nil }

// unusable reports whether a value read with err has no instant to use: it
// is missing, or its TZID couldn't be resolved.
func unusable(v DateTime, err error) bool { return err != nil && (v.IsZero() || v.unresolved()) }

// Location returns the zone v is in: time.UTC, the resolved TZID, or nil
// for a floating value (or a TZID that couldn't be resolved).
func (v DateTime) Location() *time.Location {
	if v.UTC {
		return time.UTC
	}
	return v.Loc
}

// Time returns the instant v names, in its own zone. A floating value, or
// one whose TZID wasn't resolved, is read on the UTC wall clock; use In to
// read it in a zone of your choosing.
func (v DateTime) Time() time.Time {
	if loc := v.Location(); loc != nil {
		if v.hasExact && v.exactWall == v.DateTime {
			return time.Unix(v.exact, 0).In(loc)
		}
		return v.DateTime.In(loc)
	}
	return v.DateTime.In(time.UTC)
}

// In returns v as an instant shown in loc. A floating value is read on
// loc's wall clock; any other value keeps its instant. Times that a zone
// skips or repeats resolve as RFC 5545 §3.3.5 says (civil.DateTime.In).
func (v DateTime) In(loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	if v.Location() != nil {
		return v.Time().In(loc)
	}
	return v.DateTime.In(loc)
}

// String returns v as a property value: 19970902, 19970902T090000 or
// 19970902T090000Z. The TZID is a parameter, so it isn't included.
func (v DateTime) String() string {
	return icaltime.Format(v.DateTime, v.IsDate, v.UTC)
}

// Equal reports whether v and w name the same time: the same instant for
// values in a zone or UTC, and the same wall clock and kind for floating
// values. A floating value never equals a zoned one.
func (v DateTime) Equal(w DateTime) bool {
	if v.Floating() || w.Floating() {
		return v.Floating() == w.Floating() && v.IsDate == w.IsDate && v.DateTime == w.DateTime
	}
	return v.Time().Equal(w.Time())
}

// Add returns v moved by d, as RFC 5545 §3.3.6 adds durations: weeks and
// days on the wall clock, so P1D keeps the time of day across a DST
// change; hours, minutes and seconds on the instant, so PT24H doesn't.
// The result keeps v's form; a DATE plus a duration with a time part
// becomes a floating DATE-TIME.
func (v DateTime) Add(d Duration) DateTime {
	sign := 1
	if d.Negative {
		sign = -1
	}
	days := sign * (d.Weeks*7 + d.Days)
	exact := int64(sign) * (int64(d.Hours)*3600 + int64(d.Minutes)*60 + int64(d.Seconds))
	out := v
	out.DateTime = v.DateTime.AddDate(0, 0, days)
	if exact == 0 {
		return out
	}
	if v.IsDate {
		out.IsDate = false
	}
	loc := v.Location()
	if loc == nil {
		out.DateTime = out.DateTime.AddSeconds(exact)
		return out
	}
	base := out.DateTime.In(loc)
	if days == 0 {
		base = v.Time() // keep which side of a fall-back v is on
	}
	t := time.Unix(base.Unix()+exact, 0).In(loc)
	out.DateTime = civil.DateTimeOf(t)
	return out.withInstant(t)
}

// DateTime returns the value read as a DATE or DATE-TIME. VALUE=DATE, or
// an 8-digit value with no VALUE parameter, is a DATE. A TZID parameter
// is resolved with r, or with tz.Load when r is nil; if that fails the
// value is still returned, with a nil Loc, alongside an error wrapping
// tz.ErrUnknownZone.
func (p *Property) DateTime(r tz.Resolver) (DateTime, error) {
	return p.parseDateTime(strings.TrimSpace(p.Value), r)
}

func (p *Property) parseDateTime(s string, r tz.Resolver) (DateTime, error) {
	s = strings.ToUpper(s)
	wall, isDate, utc, err := icaltime.Parse(s)
	if err == icaltime.ErrSyntax {
		// Two shapes real producers write: a time without seconds
		// (19970701T1300Z) and a DATE with a Z (20131210Z).
		switch {
		case len(s) == 13 && s[8] == 'T', len(s) == 14 && s[8] == 'T' && s[13] == 'Z':
			wall, isDate, utc, err = icaltime.Parse(s[:13] + "00" + s[13:])
		case len(s) == 9 && s[8] == 'Z':
			wall, isDate, utc, err = icaltime.Parse(s[:8])
		}
	}
	if err != nil {
		return DateTime{}, p.valueError(err)
	}
	v := DateTime{DateTime: wall, IsDate: isDate, UTC: utc}
	if strings.EqualFold(p.Params.Get("VALUE"), "DATE-TIME") && isDate {
		return DateTime{}, p.valueError(errors.New("VALUE=DATE-TIME with a DATE"))
	}
	if tzid := p.Params.Get("TZID"); tzid != "" && !isDate && !utc {
		v.TZID = tzid
		loc, err := resolve(r, tzid)
		if err != nil {
			return v, p.valueError(err)
		}
		v.Loc = loc
	}
	return v, nil
}

func resolve(r tz.Resolver, tzid string) (*time.Location, error) {
	if r == nil {
		return tz.Load(tzid)
	}
	return r.Resolve(tzid)
}

// DateTimes returns the value read as a comma-separated list of DATE or
// DATE-TIME values, as for EXDATE and RDATE. With VALUE=PERIOD it returns
// each period's start; use Periods for the periods.
func (p *Property) DateTimes(r tz.Resolver) ([]DateTime, error) {
	if strings.EqualFold(p.Params.Get("VALUE"), "PERIOD") {
		ps, err := p.Periods(r)
		if errors.Is(err, errBackwards) {
			err = nil // a period's end doesn't change its start
		}
		out := make([]DateTime, len(ps))
		for i, per := range ps {
			out[i] = per.Start
		}
		return out, err
	}
	var out []DateTime
	var firstErr error
	for item := range strings.SplitSeq(p.Value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		v, err := p.parseDateTime(item, r)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			if v.IsZero() {
				continue
			}
		}
		out = append(out, v)
	}
	return out, firstErr
}

// SetDateTime sets the value to v, with the VALUE and TZID parameters its
// form needs.
func (p *Property) SetDateTime(v DateTime) {
	p.SetDateTimes(v)
}

// SetDateTimes sets the value to a comma-separated list of DATE or
// DATE-TIME values. They are written in the form of the first; later
// values in other zones are converted to it. A floating value has no
// instant to lose, so a list that starts with one takes the form of its
// first zoned value instead. A TZID's wall clock names the first of two
// repeated readings (RFC 5545 §3.3.5), so a list holding the second, when
// clocks go back, is written in UTC.
func (p *Property) SetDateTimes(vs ...DateTime) {
	if len(vs) == 0 {
		p.Value = ""
		return
	}
	first := vs[0]
	if first.Floating() && !first.IsDate {
		for _, v := range vs[1:] {
			if !v.IsDate && !v.Floating() {
				first = convertForm(first, v)
				break
			}
		}
	}
	if first.TZID != "" && !first.UTC && !first.IsDate && first.Loc != nil {
		for _, v := range vs {
			if !v.IsDate && v.Location() != nil && secondReading(v.In(first.Loc)) {
				first = NewDateTime(first.Time().UTC())
				break
			}
		}
	}
	p.Params.Del("TZID")
	if first.IsDate {
		p.setValueType("DATE")
	} else {
		p.setValueType("DATE-TIME")
		if strings.EqualFold(p.Name, "TRIGGER") {
			// An absolute trigger is in UTC, and RELATED is for a relative
			// one (§3.8.6.3).
			p.Params.Del("RELATED")
			if first.Location() != nil {
				first = NewDateTime(first.Time().UTC())
			}
		}
	}
	if first.TZID != "" && !first.UTC && !first.IsDate {
		p.Params.Set("TZID", first.TZID)
	}
	var b []byte
	for i, v := range vs {
		if i == 0 {
			v = first
		} else {
			b = append(b, ',')
			v = convertForm(v, first)
		}
		if first.IsDate {
			b = icaltime.AppendDate(b, v.Date)
		} else {
			b = icaltime.AppendDateTime(b, v.DateTime, first.UTC)
		}
	}
	p.Value = string(b)
}

// convertForm rewrites v in the form (zone) of like.
func convertForm(v, like DateTime) DateTime {
	switch {
	case like.IsDate || v.IsDate || v.Floating() && like.Floating():
		return v
	case like.UTC:
		return NewDateTime(v.Time().UTC())
	case like.Loc != nil:
		t := v.In(like.Loc)
		out := DateTime{DateTime: civil.DateTimeOf(t), TZID: like.TZID, Loc: like.Loc}
		if secondReading(t) {
			out = out.withInstant(t)
		}
		return out
	}
	return v
}

// defaultTypes are the default value types of the properties the DATE,
// DATE-TIME and DURATION setters are used on (RFC 5545 §3.8, RFC 7986,
// RFC 9074). Any other property's is TEXT (§3.8.8). REFRESH-INTERVAL has
// none: its VALUE=DURATION is required.
var defaultTypes = map[string]string{
	"DTSTART": "DATE-TIME", "DTEND": "DATE-TIME", "DUE": "DATE-TIME",
	"RECURRENCE-ID": "DATE-TIME", "RDATE": "DATE-TIME", "EXDATE": "DATE-TIME",
	"DTSTAMP": "DATE-TIME", "CREATED": "DATE-TIME", "LAST-MODIFIED": "DATE-TIME",
	"COMPLETED": "DATE-TIME", "ACKNOWLEDGED": "DATE-TIME",
	"DURATION": "DURATION", "TRIGGER": "DURATION",
	"REFRESH-INTERVAL": "",
}

// setValueType sets the VALUE parameter a value of type t needs: none when
// t is the property's default type, t otherwise (§3.2.20).
func (p *Property) setValueType(t string) {
	def, ok := defaultTypes[strings.ToUpper(p.Name)]
	if !ok {
		def = "TEXT"
	}
	if t == def {
		p.Params.Del("VALUE")
	} else {
		p.Params.Set("VALUE", t)
	}
}

// DURATION (RFC 5545 §3.3.6)

// A Duration is a DURATION value. Weeks and days are nominal: added on
// the wall clock, so a day across a DST change is 23 or 25 hours. Hours,
// minutes and seconds are exact. See DateTime.Add.
type Duration struct {
	Negative                bool
	Weeks, Days             int
	Hours, Minutes, Seconds int
}

// DurationOf returns d as an exact Duration of hours, minutes and whole
// seconds.
func DurationOf(d time.Duration) Duration {
	var out Duration
	if d < 0 {
		out.Negative, d = true, -d
	}
	s := int64(d / time.Second)
	out.Hours, out.Minutes, out.Seconds = int(s/3600), int(s/60%60), int(s%60)
	return out
}

// IsZero reports whether d is zero.
func (d Duration) IsZero() bool {
	return d.Weeks == 0 && d.Days == 0 && d.Hours == 0 && d.Minutes == 0 && d.Seconds == 0
}

// Approx returns d with every day taken as 24 hours: the exact length
// away from DST changes.
// It saturates rather than overflow for absurd durations.
func (d Duration) Approx() time.Duration {
	secs := ((int64(d.Weeks)*7+int64(d.Days))*24+int64(d.Hours))*3600 + int64(d.Minutes)*60 + int64(d.Seconds)
	s := time.Duration(1<<63 - 1)
	if secs < int64(s/time.Second) {
		s = time.Duration(secs) * time.Second
	}
	if d.Negative {
		return -s
	}
	return s
}

// String returns d as RFC 5545 writes it: P2W, P1DT2H, -PT15M, or PT0S
// for zero. Weeks combined with any other part, which the grammar doesn't
// allow, are written as days.
func (d Duration) String() string {
	var b strings.Builder
	if d.Negative && !d.IsZero() {
		b.WriteByte('-')
	}
	b.WriteByte('P')
	if d.Weeks != 0 && d.Days == 0 && d.Hours == 0 && d.Minutes == 0 && d.Seconds == 0 {
		b.WriteString(strconv.Itoa(d.Weeks) + "W")
		return b.String()
	}
	if days := d.Weeks*7 + d.Days; days != 0 {
		b.WriteString(strconv.Itoa(days) + "D")
	}
	if d.Hours != 0 || d.Minutes != 0 || d.Seconds != 0 || d.IsZero() {
		b.WriteByte('T')
		if d.Hours != 0 {
			b.WriteString(strconv.Itoa(d.Hours) + "H")
		}
		// Seconds can only follow hours by way of minutes (dur-hour), as
		// in the RFC's P15DT5H0M20S.
		if d.Minutes != 0 || d.Hours != 0 && d.Seconds != 0 {
			b.WriteString(strconv.Itoa(d.Minutes) + "M")
		}
		if d.Seconds != 0 || d.IsZero() {
			b.WriteString(strconv.Itoa(d.Seconds) + "S")
		}
	}
	return b.String()
}

// ParseDuration reads a DURATION value such as P1D, PT1H30M or -P2W.
// Parts must come in order (weeks, days, then hours, minutes, seconds
// after a T); combining weeks with other parts, which RFC 5545's grammar
// doesn't allow, is accepted. An empty duration (P, PT) is an error.
func ParseDuration(s string) (Duration, error) {
	var d Duration
	orig := s
	s = strings.ToUpper(strings.TrimSpace(s))
	if strings.HasPrefix(s, "-") {
		d.Negative = true
		s = s[1:]
	} else {
		s = strings.TrimPrefix(s, "+")
	}
	if !strings.HasPrefix(s, "P") {
		return Duration{}, fmt.Errorf("duration %q doesn't start with P", orig)
	}
	s = s[1:]
	inTime := false
	order := "WD" // designators allowed next, in order
	any := false
	for len(s) > 0 {
		if s[0] == 'T' {
			if inTime {
				return Duration{}, fmt.Errorf("duration %q has two T", orig)
			}
			inTime, order, s = true, "HMS", s[1:]
			if s == "" {
				return Duration{}, fmt.Errorf("duration %q has nothing after T", orig)
			}
			continue
		}
		i := 0
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == 0 || i == len(s) || i > 9 {
			return Duration{}, fmt.Errorf("duration %q is malformed", orig)
		}
		n, _ := strconv.Atoi(s[:i])
		unit := s[i]
		k := strings.IndexByte(order, unit)
		if k < 0 {
			return Duration{}, fmt.Errorf("duration %q has %q out of place", orig, unit)
		}
		order = order[k+1:]
		switch unit {
		case 'W':
			d.Weeks = n
		case 'D':
			d.Days = n
		case 'H':
			d.Hours = n
		case 'M':
			d.Minutes = n
		case 'S':
			d.Seconds = n
		}
		any = true
		s = s[i+1:]
	}
	if !any {
		return Duration{}, fmt.Errorf("duration %q is empty", orig)
	}
	return d, nil
}

// Duration returns the value read as a DURATION.
func (p *Property) Duration() (Duration, error) {
	d, err := ParseDuration(p.Value)
	if err != nil {
		return Duration{}, p.valueError(err)
	}
	return d, nil
}

// SetDuration sets the value to d, with the VALUE parameter its property
// needs for a DURATION.
func (p *Property) SetDuration(d Duration) {
	p.setValueType("DURATION")
	p.Value = d.String()
}

// PERIOD (RFC 5545 §3.3.9)

// A Period is a PERIOD value: a start and either an end or a duration.
type Period struct {
	Start    DateTime
	End      DateTime // zero when the period was written with a duration
	Duration Duration // zero when it was written with an end
}

// EndTime returns the period's end, computing it from the duration when
// that is how it was written.
func (p Period) EndTime() DateTime {
	if !p.End.IsZero() {
		return p.End
	}
	return p.Start.Add(p.Duration)
}

// valid reports whether p ends after it starts, as RFC 5545 §3.3.9 requires.
func (p Period) valid() bool { return p.EndTime().Time().After(p.Start.Time()) }

// errBackwards is wrapped by the error Periods returns for a period that
// isn't valid.
var errBackwards = errors.New("doesn't end after it starts")

// Periods returns the value read as a comma-separated list of PERIOD
// values, as for FREEBUSY and RDATE;VALUE=PERIOD. A period that doesn't
// end after it starts is returned as written, with an error (§3.3.9).
func (p *Property) Periods(r tz.Resolver) ([]Period, error) {
	var out []Period
	var backwards error
	for item := range strings.SplitSeq(p.Value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		startText, rest, ok := strings.Cut(item, "/")
		if !ok {
			return out, p.valueError(fmt.Errorf("period %q has no /", item))
		}
		start, err := p.parseDateTime(startText, r)
		if err != nil {
			return out, err
		}
		per := Period{Start: start}
		if strings.HasPrefix(rest, "P") || strings.HasPrefix(rest, "+P") || strings.HasPrefix(rest, "-P") {
			if per.Duration, err = ParseDuration(rest); err != nil {
				return out, p.valueError(err)
			}
		} else if per.End, err = p.parseDateTime(rest, r); err != nil {
			return out, err
		}
		// "The start MUST be before the end", or the duration positive.
		if backwards == nil && !per.valid() {
			backwards = p.valueError(fmt.Errorf("period %q %w", item, errBackwards))
		}
		out = append(out, per)
	}
	return out, backwards
}

// RECUR (RFC 5545 §3.3.10)

// Recur returns the value read as a recurrence rule.
func (p *Property) Recur() (recur.Rule, error) {
	r, err := recur.ParseRule(p.Value)
	if err != nil {
		return recur.Rule{}, p.valueError(err)
	}
	return r, nil
}

// SetRecur sets the value to r.
func (p *Property) SetRecur(r recur.Rule) {
	p.Value = r.String()
}

// INTEGER, FLOAT, BOOLEAN, UTC-OFFSET, BINARY, and GEO

// Int returns the value read as an INTEGER.
func (p *Property) Int() (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(p.Value))
	if err != nil {
		return 0, p.valueError(err)
	}
	return n, nil
}

// SetInt sets the value to n.
func (p *Property) SetInt(n int) { p.Value = strconv.Itoa(n) }

// Float returns the value read as a FLOAT.
func (p *Property) Float() (float64, error) {
	f, err := parseFloat(p.Value)
	if err != nil {
		return 0, p.valueError(err)
	}
	return f, nil
}

// parseFloat reads a FLOAT (RFC 5545 §3.3.7). strconv.ParseFloat also reads
// NaN, Inf, hex and digits with underscores, which aren't decimal numbers,
// so only signs, digits, a point and an exponent get that far.
func parseFloat(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if strings.Trim(s, "+-.0123456789eE") != "" {
		return 0, fmt.Errorf("%q is not a FLOAT", s)
	}
	return strconv.ParseFloat(s, 64)
}

// Bool returns the value read as a BOOLEAN: TRUE or FALSE, in any case.
func (p *Property) Bool() (bool, error) {
	switch strings.ToUpper(strings.TrimSpace(p.Value)) {
	case "TRUE":
		return true, nil
	case "FALSE":
		return false, nil
	}
	return false, p.valueError(errors.New("not TRUE or FALSE"))
}

// UTCOffset returns the value read as a UTC-OFFSET, in seconds east of
// UTC: +0530 is 19800.
func (p *Property) UTCOffset() (int, error) {
	n, err := ParseUTCOffset(p.Value)
	if err != nil {
		return 0, p.valueError(err)
	}
	return n, nil
}

// ParseUTCOffset reads a UTC-OFFSET, ±HHMM or ±HHMMSS, as seconds east of
// UTC. RFC 5545 forbids -0000; it is read as zero.
func ParseUTCOffset(s string) (int, error) {
	s = strings.TrimSpace(s)
	if len(s) != 5 && len(s) != 7 || s[0] != '+' && s[0] != '-' {
		return 0, fmt.Errorf("UTC offset %q is not ±HHMM[SS]", s)
	}
	n := 0
	for i := 1; i < len(s); i += 2 {
		v, err := strconv.Atoi(s[i : i+2])
		if err != nil || s[i] == '+' || s[i] == '-' {
			return 0, fmt.Errorf("UTC offset %q is not ±HHMM[SS]", s)
		}
		switch i {
		case 1:
			if v > 23 {
				return 0, fmt.Errorf("UTC offset %q has hours over 23", s)
			}
			n += v * 3600
		case 3:
			if v > 59 {
				return 0, fmt.Errorf("UTC offset %q has minutes over 59", s)
			}
			n += v * 60
		case 5:
			if v > 59 {
				return 0, fmt.Errorf("UTC offset %q has seconds over 59", s)
			}
			n += v
		}
	}
	if s[0] == '-' {
		n = -n
	}
	return n, nil
}

// FormatUTCOffset writes seconds east of UTC as a UTC-OFFSET.
func FormatUTCOffset(n int) string {
	sign := byte('+')
	if n < 0 {
		sign, n = '-', -n
	}
	s := fmt.Sprintf("%c%02d%02d", sign, n/3600, n/60%60)
	if n%60 != 0 {
		s += fmt.Sprintf("%02d", n%60)
	}
	return s
}

// SetUTCOffset sets the value to a UTC-OFFSET of n seconds east of UTC.
func (p *Property) SetUTCOffset(n int) { p.Value = FormatUTCOffset(n) }

// Binary returns the value decoded from base64, as ENCODING=BASE64
// (RFC 5545 §3.3.1) requires.
func (p *Property) Binary() ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(p.Value))
	if err != nil {
		return nil, p.valueError(err)
	}
	return b, nil
}

// SetBinary sets the value to b in base64, with ENCODING=BASE64 and
// VALUE=BINARY.
func (p *Property) SetBinary(b []byte) {
	p.Params.Set("ENCODING", "BASE64")
	p.Params.Set("VALUE", "BINARY")
	p.Value = base64.StdEncoding.EncodeToString(b)
}

// Geo returns a GEO value (RFC 5545 §3.8.1.6): latitude and longitude.
func (p *Property) Geo() (lat, lon float64, err error) {
	a, b, ok := strings.Cut(p.Value, ";")
	if !ok {
		return 0, 0, p.valueError(errors.New("GEO needs latitude;longitude"))
	}
	if lat, err = parseFloat(a); err == nil {
		lon, err = parseFloat(b)
	}
	if err != nil {
		return 0, 0, p.valueError(err)
	}
	return lat, lon, nil
}

// SetGeo sets a GEO value. The two floats are separated by a raw
// semicolon, not a TEXT escape.
func (p *Property) SetGeo(lat, lon float64) {
	p.Params.Del("VALUE")
	p.Value = strconv.FormatFloat(lat, 'f', -1, 64) + ";" + strconv.FormatFloat(lon, 'f', -1, 64)
}
