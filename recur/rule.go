// Package recur implements RFC 5545 recurrence: the RECUR value type
// (§3.3.10) and the expansion of RRULE, RDATE, EXDATE and EXRULE into the
// instances of a recurrence set (§3.8.5).
//
// Expansion happens on the wall clock, in civil time, which is how the RFC
// defines it: instances are computed from DTSTART's local date and time,
// and only then resolved to instants in DTSTART's zone. So the iterators
// here yield civil.DateTime values; a Set with a Location also resolves
// them to time.Time.
//
// The generator seeks: a query for instances in a window starts at the
// first period that can hold one, rather than walking every period from
// DTSTART, for any rule without COUNT. A Cache extends that to COUNT rules
// and remembers rules proven to have ended. Every iteration terminates, even
// for rules that can never match: see Rule.All.
package recur

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/aanelson/iCalendar/civil"
	"github.com/aanelson/iCalendar/internal/icaltime"
)

// A Frequency is the FREQ of a rule: the length of its periods.
type Frequency int

// The frequencies, shortest first. The zero Frequency is not valid.
const (
	Secondly Frequency = iota + 1
	Minutely
	Hourly
	Daily
	Weekly
	Monthly
	Yearly
)

var freqNames = [...]string{"", "SECONDLY", "MINUTELY", "HOURLY", "DAILY", "WEEKLY", "MONTHLY", "YEARLY"}

// String returns the FREQ value, like "WEEKLY".
func (f Frequency) String() string {
	if f >= Secondly && f <= Yearly {
		return freqNames[f]
	}
	return "Frequency(" + strconv.Itoa(int(f)) + ")"
}

// A Weekday is a day of the week as RRULE names them. Monday is the zero
// value because it is the RFC's default week start (WKST); convert with
// Time and WeekdayOf when mixing with time.Weekday.
type Weekday int

// The days of the week, Monday first.
const (
	Monday Weekday = iota
	Tuesday
	Wednesday
	Thursday
	Friday
	Saturday
	Sunday
)

var weekdayNames = [...]string{"MO", "TU", "WE", "TH", "FR", "SA", "SU"}

// String returns the two-letter RRULE name, like "MO".
func (d Weekday) String() string {
	if d >= Monday && d <= Sunday {
		return weekdayNames[d]
	}
	return "Weekday(" + strconv.Itoa(int(d)) + ")"
}

// Time returns d as a time.Weekday.
func (d Weekday) Time() time.Weekday {
	return time.Weekday((int(d) + 1) % 7)
}

// WeekdayOf returns d as a Weekday.
func WeekdayOf(d time.Weekday) Weekday {
	return Weekday((int(d) + 6) % 7)
}

// A WeekdayNum is one entry of BYDAY: a day of the week, and optionally
// which one of them in the month or year.
type WeekdayNum struct {
	// N picks one occurrence of Day in the month or year: 1 is the first,
	// -1 the last. Zero means every Day.
	N   int
	Day Weekday
}

// String returns the BYDAY form, like "MO", "1MO" or "-1FR".
func (w WeekdayNum) String() string {
	if w.N == 0 {
		return w.Day.String()
	}
	return strconv.Itoa(w.N) + w.Day.String()
}

// Until is the UNTIL of a rule: the last instant a rule may produce,
// inclusive. It keeps the form it was written in, because RFC 5545 compares
// each form differently: a DATE against the instance's date, a floating
// DATE-TIME against its wall clock, and a UTC DATE-TIME against its instant.
type Until struct {
	civil.DateTime      // the date and time; for UTC, the UTC wall clock
	IsDate         bool // a DATE: the time of day is zero and unused
	UTC            bool // the Z form
}

// IsZero reports whether u is unset.
func (u Until) IsZero() bool { return u == Until{} }

// UntilTime returns the UTC Until for t.
func UntilTime(t time.Time) Until {
	return Until{DateTime: civil.DateTimeOf(t.UTC()), UTC: true}
}

// String returns u as RRULE writes it, like 19971224T000000Z.
func (u Until) String() string {
	return icaltime.Format(u.DateTime, u.IsDate, u.UTC)
}

// A Rule is a recurrence rule, the RECUR value type of RFC 5545 §3.3.10.
//
// A Rule is a plain value. Iterating it never changes it, so one Rule can
// be expanded from many goroutines at once. The BYxxx lists are sets: order
// and duplicates don't matter.
type Rule struct {
	Freq Frequency
	// Interval is the number of periods between periods with instances.
	// Zero is read as 1.
	Interval int
	// Count is the number of instances, or zero for no limit. It can't be
	// combined with Until.
	Count int
	// Until is the last instant the rule may produce, inclusive.
	Until Until
	// WeekStart is WKST: the day weeks start on, for WEEKLY periods and
	// BYWEEKNO. The zero value is Monday, the RFC's default.
	WeekStart Weekday

	BySecond   []int        // 0 to 60
	ByMinute   []int        // 0 to 59
	ByHour     []int        // 0 to 23
	ByDay      []WeekdayNum // N from -53 to 53
	ByMonthDay []int        // 1 to 31 and -31 to -1
	ByYearDay  []int        // 1 to 366 and -366 to -1
	ByWeekNo   []int        // 1 to 53 and -53 to -1
	ByMonth    []int        // 1 to 12
	BySetPos   []int        // 1 to 366 and -366 to -1

	// Ignored holds the parts ParseRule read but doesn't use, as written: a
	// part given again with the same value, RSCALE=GREGORIAN, SKIP=OMIT, and
	// x-name parts. String doesn't write them, so a caller can see what a
	// rule written back leaves out, and a strict one can refuse the rule.
	Ignored []string
}

// A ParseError reports a RECUR value that can't be read.
type ParseError struct {
	Value string // the whole value
	Part  string // the rule part at fault, if any
	Msg   string
}

func (e *ParseError) Error() string {
	if e.Part != "" {
		return fmt.Sprintf("recur: %s in %q: %s", e.Part, e.Value, e.Msg)
	}
	return fmt.Sprintf("recur: %q: %s", e.Value, e.Msg)
}

// ParseRule reads a RECUR value such as "FREQ=WEEKLY;BYDAY=MO,WE;COUNT=10".
//
// Part names and values are read case-insensitively, spaces around them and
// around list items are ignored, numbers may have leading zeros (BYMONTHDAY=
// 001), and an empty part (a trailing ";") is ignored, as most producers
// expect. Some parts are read and kept in the rule's Ignored, rather than
// costing the whole rule: a part given again with the same value; RFC
// 7529's RSCALE=GREGORIAN and SKIP=OMIT, which together are RFC 5545's own
// recurrence; and an x-name part (X-FOO=1, which RFC 2445 allowed), whose
// meaning is unknown, so it is skipped as any reader that doesn't know it
// would. Everything else follows the §3.3.10 grammar: an unknown part, a
// part repeated with a different value, a value out of its range, a COUNT
// or INTERVAL below 1, a missing FREQ, or another RSCALE or SKIP is an
// error. Constraints between parts (such as COUNT with UNTIL) are checked by
// Validate, not here, so a rule that breaks one can still be expanded.
func ParseRule(s string) (Rule, error) {
	var r Rule
	seen := map[string]Rule{} // part name → the part alone, its lists as sets
	for part := range strings.SplitSeq(s, ";") {
		if strings.TrimSpace(part) == "" {
			continue
		}
		name, value, ok := strings.Cut(part, "=")
		name = strings.ToUpper(strings.TrimSpace(name))
		value = strings.TrimSpace(value)
		fail := func(msg string, args ...any) (Rule, error) {
			return Rule{}, &ParseError{Value: s, Part: name, Msg: fmt.Sprintf(msg, args...)}
		}
		if !ok {
			return fail("no '='")
		}
		var one Rule
		if err := one.setPart(name, value); err != nil {
			return fail("%v", err)
		}
		was, repeated := seen[name]
		if repeated {
			// "Individual rule parts MUST only be specified once", but one
			// given again with the same value says nothing new.
			if !was.Equal(one) {
				return fail("appears more than once, with different values")
			}
		} else {
			// Kept as a set, so comparing a repeat costs only the repeat's
			// own length, however long the first list was.
			seen[name] = one.asSets()
			r.merge(one)
		}
		if repeated || name == "RSCALE" || name == "SKIP" || isXName(name) {
			r.Ignored = append(r.Ignored, strings.TrimSpace(part))
		}
	}
	if r.Freq == 0 {
		return Rule{}, &ParseError{Value: s, Msg: "no FREQ"}
	}
	return r, nil
}

// setPart sets the rule part name, in upper case, to value. RSCALE, SKIP and
// an x-name part are checked and set nothing.
func (r *Rule) setPart(name, value string) error {
	var err error
	switch name {
	case "FREQ":
		for f := Secondly; f <= Yearly; f++ {
			if strings.EqualFold(value, freqNames[f]) {
				r.Freq = f
				return nil
			}
		}
		return fmt.Errorf("unknown frequency %q", value)
	case "UNTIL":
		v, isDate, utc, err := icaltime.Parse(strings.ToUpper(value))
		if err != nil {
			return err
		}
		r.Until = Until{DateTime: v, IsDate: isDate, UTC: utc}
	case "COUNT":
		r.Count, err = positive(value)
	case "INTERVAL":
		r.Interval, err = positive(value)
	case "BYSECOND":
		r.BySecond, err = intList(value, 0, 60, false)
	case "BYMINUTE":
		r.ByMinute, err = intList(value, 0, 59, false)
	case "BYHOUR":
		r.ByHour, err = intList(value, 0, 23, false)
	case "BYDAY":
		r.ByDay, err = weekdayList(value)
	case "BYMONTHDAY":
		r.ByMonthDay, err = intList(value, 1, 31, true)
	case "BYYEARDAY":
		r.ByYearDay, err = intList(value, 1, 366, true)
	case "BYWEEKNO":
		r.ByWeekNo, err = intList(value, 1, 53, true)
	case "BYMONTH":
		r.ByMonth, err = intList(value, 1, 12, false)
	case "BYSETPOS":
		r.BySetPos, err = intList(value, 1, 366, true)
	case "WKST":
		d, ok := parseWeekday(value)
		if !ok {
			return fmt.Errorf("unknown weekday %q", value)
		}
		r.WeekStart = d
	// RFC 7529: "SKIP=OMIT: this is the default option and corresponds to
	// the existing iCalendar behavior", and RSCALE=GREGORIAN is iCalendar's
	// own calendar. Another calendar, or filling in the dates OMIT drops,
	// changes the instances.
	case "RSCALE":
		if !strings.EqualFold(value, "GREGORIAN") {
			return fmt.Errorf("calendar %q is not supported", value)
		}
	case "SKIP":
		if !strings.EqualFold(value, "OMIT") {
			return fmt.Errorf("%q is not supported", value)
		}
	default:
		if !isXName(name) {
			return errors.New("unknown rule part")
		}
	}
	return err
}

// merge sets in r the parts set in o.
func (r *Rule) merge(o Rule) {
	r.Freq = cmp.Or(o.Freq, r.Freq)
	r.Until = cmp.Or(o.Until, r.Until)
	r.Count = cmp.Or(o.Count, r.Count)
	r.Interval = cmp.Or(o.Interval, r.Interval)
	r.WeekStart = cmp.Or(o.WeekStart, r.WeekStart)
	r.BySecond = orList(o.BySecond, r.BySecond)
	r.ByMinute = orList(o.ByMinute, r.ByMinute)
	r.ByHour = orList(o.ByHour, r.ByHour)
	r.ByDay = orList(o.ByDay, r.ByDay)
	r.ByMonthDay = orList(o.ByMonthDay, r.ByMonthDay)
	r.ByYearDay = orList(o.ByYearDay, r.ByYearDay)
	r.ByWeekNo = orList(o.ByWeekNo, r.ByWeekNo)
	r.ByMonth = orList(o.ByMonth, r.ByMonth)
	r.BySetPos = orList(o.BySetPos, r.BySetPos)
}

// orList returns a if it is set, else b.
func orList[T any](a, b []T) []T {
	if a != nil {
		return a
	}
	return b
}

// isXName reports whether name, in upper case, is an x-name (RFC 5545
// §3.1): "X-" [vendorid "-"] 1*(ALPHA / DIGIT / "-").
func isXName(name string) bool {
	rest, ok := strings.CutPrefix(name, "X-")
	return ok && rest != "" && strings.Trim(rest, "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-") == ""
}

func positive(s string) (int, error) {
	n, err := number(s)
	if err != nil {
		return 0, err
	}
	if n < 1 {
		return 0, errors.New("must be at least 1")
	}
	return n, nil
}

// number reads 1*DIGIT, refusing values too large to mean anything (more
// than nine digits).
func number(s string) (int, error) {
	if s == "" || len(s) > 9 || strings.Trim(s, "0123456789") != "" {
		return 0, fmt.Errorf("%q is not a number", s)
	}
	return strconv.Atoi(s)
}

// intList reads a comma-separated list of integers in [lo, hi], or also
// [-hi, -lo] when signed.
func intList(s string, lo, hi int, signed bool) ([]int, error) {
	var out []int
	for item := range strings.SplitSeq(s, ",") {
		item = strings.TrimSpace(item)
		neg := false
		if signed && len(item) > 0 && (item[0] == '-' || item[0] == '+') {
			neg = item[0] == '-'
			item = item[1:]
		}
		n, err := number(item)
		if err != nil {
			return nil, err
		}
		if n < lo || n > hi {
			return nil, fmt.Errorf("%d is outside %d to %d", n, lo, hi)
		}
		if neg {
			n = -n
		}
		out = append(out, n)
	}
	return out, nil
}

func weekdayList(s string) ([]WeekdayNum, error) {
	var out []WeekdayNum
	for item := range strings.SplitSeq(s, ",") {
		item = strings.TrimSpace(item)
		if len(item) < 2 {
			return nil, fmt.Errorf("%q is not a weekday", item)
		}
		d, ok := parseWeekday(item[len(item)-2:])
		if !ok {
			return nil, fmt.Errorf("%q is not a weekday", item)
		}
		w := WeekdayNum{Day: d}
		if num := item[:len(item)-2]; num != "" {
			ns, err := intList(num, 0, 53, true)
			if err != nil {
				return nil, err
			}
			if ns[0] == 0 {
				// 0MO is outside the grammar (ordwk is 1 to 53). Like
				// ical.net, drop the entry and keep the rest of the list.
				continue
			}
			w.N = ns[0]
		}
		out = append(out, w)
	}
	return out, nil
}

func parseWeekday(s string) (Weekday, bool) {
	for d, name := range weekdayNames {
		if strings.EqualFold(s, name) {
			return Weekday(d), true
		}
	}
	return 0, false
}

// String returns the rule as a RECUR value, with its parts in the order
// of the §3.3.10 grammar and FREQ first (which RFC 5545 asks for, for
// RFC 2445 readers). INTERVAL=1 and WKST=MO are left out as defaults. A
// rule read by ParseRule and written by String reads back the same.
func (r Rule) String() string {
	var b strings.Builder
	b.WriteString("FREQ=")
	b.WriteString(r.Freq.String())
	if !r.Until.IsZero() {
		b.WriteString(";UNTIL=")
		b.WriteString(r.Until.String())
	}
	if r.Count > 0 {
		b.WriteString(";COUNT=")
		b.WriteString(strconv.Itoa(r.Count))
	}
	if r.Interval > 1 {
		b.WriteString(";INTERVAL=")
		b.WriteString(strconv.Itoa(r.Interval))
	}
	writeInts(&b, "BYSECOND", r.BySecond)
	writeInts(&b, "BYMINUTE", r.ByMinute)
	writeInts(&b, "BYHOUR", r.ByHour)
	if len(r.ByDay) > 0 {
		b.WriteString(";BYDAY=")
		for i, w := range r.ByDay {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(w.String())
		}
	}
	writeInts(&b, "BYMONTHDAY", r.ByMonthDay)
	writeInts(&b, "BYYEARDAY", r.ByYearDay)
	writeInts(&b, "BYWEEKNO", r.ByWeekNo)
	writeInts(&b, "BYMONTH", r.ByMonth)
	writeInts(&b, "BYSETPOS", r.BySetPos)
	if r.WeekStart != Monday {
		b.WriteString(";WKST=")
		b.WriteString(r.WeekStart.String())
	}
	return b.String()
}

func writeInts(b *strings.Builder, name string, list []int) {
	if len(list) == 0 {
		return
	}
	b.WriteByte(';')
	b.WriteString(name)
	b.WriteByte('=')
	for i, n := range list {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Itoa(n))
	}
}

// MarshalText implements encoding.TextMarshaler with String.
func (r Rule) MarshalText() ([]byte, error) { return []byte(r.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler with ParseRule.
func (r *Rule) UnmarshalText(b []byte) error {
	v, err := ParseRule(string(b))
	if err != nil {
		return err
	}
	*r = v
	return nil
}

// Validate reports the first way r breaks a MUST or MUST NOT of RFC 5545
// §3.3.10, or nil. Expansion doesn't require a valid rule: every part is
// given a meaning even where the RFC forbids it, as noted on Rule.All.
func (r Rule) Validate() error {
	fail := func(msg string) error { return &ParseError{Value: r.String(), Msg: msg} }
	switch {
	case r.Freq < Secondly || r.Freq > Yearly:
		return fail("FREQ is required")
	case r.Interval < 0:
		return fail("INTERVAL must be positive")
	case r.Count < 0:
		return fail("COUNT must be positive")
	case r.Count > 0 && !r.Until.IsZero():
		return fail("COUNT and UNTIL must not both be set")
	case len(r.ByMonthDay) > 0 && r.Freq == Weekly:
		return fail("BYMONTHDAY must not be used with FREQ=WEEKLY")
	case len(r.ByYearDay) > 0 && (r.Freq == Daily || r.Freq == Weekly || r.Freq == Monthly):
		return fail("BYYEARDAY must not be used with FREQ=DAILY, WEEKLY or MONTHLY")
	case len(r.ByWeekNo) > 0 && r.Freq != Yearly:
		return fail("BYWEEKNO must only be used with FREQ=YEARLY")
	case len(r.BySetPos) > 0 && !r.hasByPart():
		return fail("BYSETPOS must be used with another BYxxx part")
	}
	for _, w := range r.ByDay {
		if w.N == 0 {
			continue
		}
		if r.Freq != Monthly && r.Freq != Yearly {
			return fail("BYDAY must not have a number unless FREQ is MONTHLY or YEARLY")
		}
		if r.Freq == Yearly && len(r.ByWeekNo) > 0 {
			return fail("BYDAY must not have a number with FREQ=YEARLY and BYWEEKNO")
		}
	}
	checks := []struct {
		name   string
		list   []int
		lo, hi int
		signed bool
	}{
		{"BYSECOND", r.BySecond, 0, 60, false},
		{"BYMINUTE", r.ByMinute, 0, 59, false},
		{"BYHOUR", r.ByHour, 0, 23, false},
		{"BYMONTHDAY", r.ByMonthDay, 1, 31, true},
		{"BYYEARDAY", r.ByYearDay, 1, 366, true},
		{"BYWEEKNO", r.ByWeekNo, 1, 53, true},
		{"BYMONTH", r.ByMonth, 1, 12, false},
		{"BYSETPOS", r.BySetPos, 1, 366, true},
	}
	for _, c := range checks {
		for _, n := range c.list {
			m := n
			if c.signed && m < 0 {
				m = -m
			}
			if m < c.lo || m > c.hi || !c.signed && n < 0 {
				return fail(fmt.Sprintf("%s value %d is out of range", c.name, n))
			}
		}
	}
	for _, w := range r.ByDay {
		if w.N < -53 || w.N > 53 || w.Day < Monday || w.Day > Sunday {
			return fail("BYDAY value " + w.String() + " is out of range")
		}
	}
	if r.WeekStart < Monday || r.WeekStart > Sunday {
		return fail("WKST is out of range")
	}
	return nil
}

func (r Rule) hasByPart() bool {
	return len(r.BySecond)+len(r.ByMinute)+len(r.ByHour)+len(r.ByDay)+
		len(r.ByMonthDay)+len(r.ByYearDay)+len(r.ByWeekNo)+len(r.ByMonth) > 0
}

// Equal reports whether r and r2 describe the same rule: the same parts,
// with BYxxx lists compared as sets. Ignored parts don't count.
func (r Rule) Equal(r2 Rule) bool {
	a, b := r.asSets(), r2.asSets()
	return a.Freq == b.Freq && max(a.Interval, 1) == max(b.Interval, 1) &&
		a.Count == b.Count && a.Until == b.Until && a.WeekStart == b.WeekStart &&
		slices.Equal(a.BySecond, b.BySecond) && slices.Equal(a.ByMinute, b.ByMinute) &&
		slices.Equal(a.ByHour, b.ByHour) && slices.Equal(a.ByMonthDay, b.ByMonthDay) &&
		slices.Equal(a.ByYearDay, b.ByYearDay) && slices.Equal(a.ByWeekNo, b.ByWeekNo) &&
		slices.Equal(a.ByMonth, b.ByMonth) && slices.Equal(a.BySetPos, b.BySetPos) &&
		slices.Equal(a.ByDay, b.ByDay)
}

// asSets returns r with each BYxxx list sorted and holding each item once,
// in new slices.
func (r Rule) asSets() Rule {
	r.BySecond = asSet(r.BySecond, cmp.Compare[int])
	r.ByMinute = asSet(r.ByMinute, cmp.Compare[int])
	r.ByHour = asSet(r.ByHour, cmp.Compare[int])
	r.ByDay = asSet(r.ByDay, func(a, b WeekdayNum) int {
		return cmp.Or(cmp.Compare(a.N, b.N), cmp.Compare(a.Day, b.Day))
	})
	r.ByMonthDay = asSet(r.ByMonthDay, cmp.Compare[int])
	r.ByYearDay = asSet(r.ByYearDay, cmp.Compare[int])
	r.ByWeekNo = asSet(r.ByWeekNo, cmp.Compare[int])
	r.ByMonth = asSet(r.ByMonth, cmp.Compare[int])
	r.BySetPos = asSet(r.BySetPos, cmp.Compare[int])
	return r
}

func asSet[T comparable](list []T, compare func(T, T) int) []T {
	return slices.Compact(slices.SortedFunc(slices.Values(list), compare))
}
