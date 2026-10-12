package tz

import (
	"cmp"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aanelson/iCalendar/civil"
	"github.com/aanelson/iCalendar/recur"
)

// A Definition is the content of a VTIMEZONE component (RFC 5545 §3.6.5)
// that matters for computing offsets.
type Definition struct {
	TZID        string
	Observances []Observance
}

// An Observance is one STANDARD or DAYLIGHT sub-component: an offset that
// takes effect at each of a set of onsets.
type Observance struct {
	Daylight bool
	// Start is DTSTART: the first onset, on the wall clock in force before
	// it (OffsetFrom).
	Start civil.DateTime
	// OffsetFrom and OffsetTo are TZOFFSETFROM and TZOFFSETTO, in seconds
	// east of UTC.
	OffsetFrom, OffsetTo int
	// Name is the first TZNAME, or "" for none.
	Name string
	// Rules and RDates give the later onsets, on the same wall clock as
	// Start. A UTC UNTIL in a rule is compared as an instant.
	Rules  []recur.Rule
	RDates []civil.DateTime
}

// Horizon is the last year Compile lists transitions for explicitly. Past
// it, a zone whose final rules are ordinary annual ones keeps following
// them (through the POSIX TZ rule TZif files end with); any other zone
// stays at its last offset.
const Horizon = 2200

// Limits on what one VTIMEZONE may expand to. Real zones have a few
// observances and at most a few thousand onsets (an Outlook zone from 1601
// to Horizon has about 1,200).
const (
	maxObservances = 1000
	maxOnsets      = 20_000
	maxNameLen     = 16 // bytes kept of a TZNAME
)

// ErrInvalidDefinition is wrapped by the error Compile returns for a
// definition it can't build a zone from.
var ErrInvalidDefinition = errors.New("tz: invalid VTIMEZONE")

type compileResult struct {
	loc *time.Location
	err error
}

// Compile builds a location from a VTIMEZONE definition, named after its
// TZID. Results are cached by the definition's content, so the same
// VTIMEZONE repeated across many calendars is built once.
//
// Before its first onset the zone keeps the first observance's
// OffsetFrom, which is how RFC 5545 reads a VTIMEZONE whose observances
// start after the times a calendar uses.
func Compile(def Definition) (*time.Location, error) {
	key := fingerprint(def)
	if r, ok := compiled.get(key); ok {
		return r.loc, r.err
	}
	loc, err := compile(def)
	compiled.put(key, compileResult{loc, err})
	return loc, err
}

// fingerprint is a hash of everything compile reads.
func fingerprint(def Definition) string {
	h := sha256.New()
	fmt.Fprintf(h, "%q\n", def.TZID)
	for _, o := range def.Observances {
		fmt.Fprintf(h, "%t %v %d %d %q\n", o.Daylight, o.Start, o.OffsetFrom, o.OffsetTo, o.Name)
		for _, r := range o.Rules {
			fmt.Fprintf(h, "r %s\n", r)
		}
		for _, d := range o.RDates {
			fmt.Fprintf(h, "d %v\n", d)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// A transition is an onset: from instant at on, the zone is at offset.
type transition struct {
	at     int64 // Unix seconds
	offset int
	dst    bool
	name   string
}

func compile(def Definition) (*time.Location, error) {
	if len(def.Observances) == 0 {
		return nil, fmt.Errorf("%w %q: no STANDARD or DAYLIGHT", ErrInvalidDefinition, def.TZID)
	}
	if len(def.Observances) > maxObservances {
		return nil, fmt.Errorf("%w %q: more than %d STANDARD and DAYLIGHT", ErrInvalidDefinition, def.TZID, maxObservances)
	}
	end := civil.DateTime{Date: civil.Date{Year: Horizon + 1, Month: 1, Day: 1}}
	var txs []transition
	n := 0 // onsets so far, across every observance
	for _, o := range def.Observances {
		if !o.Start.IsValid() || abs(o.OffsetFrom) >= 86400 || abs(o.OffsetTo) >= 86400 {
			return nil, fmt.Errorf("%w %q: bad DTSTART or offset", ErrInvalidDefinition, def.TZID)
		}
		from := time.FixedZone("", o.OffsetFrom)
		set := recur.Set{Start: o.Start, Location: from, Rules: o.Rules, RDates: o.RDates}
		for onset := range set.Between(o.Start, end) {
			txs = append(txs, transition{
				at:     onset.In(from).Unix(),
				offset: o.OffsetTo,
				dst:    o.Daylight,
				name:   zoneName(o.Name, o.OffsetTo),
			})
			if n++; n > maxOnsets {
				return nil, fmt.Errorf("%w %q: more than %d onsets", ErrInvalidDefinition, def.TZID, maxOnsets)
			}
		}
	}
	slices.SortStableFunc(txs, func(a, b transition) int { return cmp.Compare(a.at, b.at) })
	// Drop transitions that change nothing, and duplicates at one instant
	// (the later observance wins).
	var clean []transition
	for _, t := range txs {
		if n := len(clean); n > 0 && clean[n-1].at == t.at {
			clean[n-1] = t
			continue
		}
		if n := len(clean); n > 0 && clean[n-1].offset == t.offset && clean[n-1].dst == t.dst && clean[n-1].name == t.name {
			continue
		}
		clean = append(clean, t)
	}

	first := earliest(def.Observances)
	initial := transition{offset: first.OffsetFrom, dst: false, name: zoneName("", first.OffsetFrom)}
	// The standard observance the first onset leaves, if one ends at the
	// first offset, names the initial period better than a number does.
	for _, o := range def.Observances {
		if !o.Daylight && o.OffsetTo == first.OffsetFrom && o.Name != "" {
			initial.name = zoneName(o.Name, o.OffsetTo)
		}
	}
	footer := posixFooter(def.Observances, clean)
	data, err := encodeTZif(initial, clean, footer)
	if err != nil {
		return nil, fmt.Errorf("%w %q: %v", ErrInvalidDefinition, def.TZID, err)
	}
	loc, err := time.LoadLocationFromTZData(def.TZID, data)
	if err != nil {
		return nil, fmt.Errorf("%w %q: %v", ErrInvalidDefinition, def.TZID, err)
	}
	return loc, nil
}

func earliest(obs []Observance) Observance {
	e := obs[0]
	for _, o := range obs[1:] {
		if o.Start.Before(e.Start) {
			e = o
		}
	}
	return e
}

// zoneName returns name, cut to maxNameLen bytes on a UTF-8 boundary, or
// the offset written as "+0530" when there is none.
func zoneName(name string, offset int) string {
	if name != "" {
		if len(name) > maxNameLen {
			cut := maxNameLen
			for cut > 0 && !utf8.RuneStart(name[cut]) {
				cut--
			}
			name = name[:cut]
		}
		return name
	}
	sign := '+'
	if offset < 0 {
		sign, offset = '-', -offset
	}
	s := fmt.Sprintf("%c%02d%02d", sign, offset/3600, offset/60%60)
	if offset%60 != 0 {
		s += fmt.Sprintf("%02d", offset%60)
	}
	return s
}

// posixFooter returns the POSIX TZ string (RFC 9636 §3.3) that continues
// the zone past the last explicit transition, or "" when the definition's
// open-ended rules can't be written as one.
func posixFooter(obs []Observance, txs []transition) string {
	var std, dst *Observance
	for i := range obs {
		o := &obs[i]
		if !openEnded(o) {
			continue
		}
		if o.Daylight {
			if dst != nil {
				return ""
			}
			dst = o
		} else {
			if std != nil {
				return ""
			}
			std = o
		}
	}
	switch {
	case std == nil && dst == nil:
		// No rule runs forever: the last transition's offset holds.
		if len(txs) == 0 {
			return ""
		}
		last := txs[len(txs)-1]
		if last.dst {
			return ""
		}
		return posixName(last.name) + posixOffset(-last.offset)
	case std == nil || dst == nil:
		return ""
	}
	stdRule, ok1 := posixRule(std)
	dstRule, ok2 := posixRule(dst)
	if !ok1 || !ok2 || std.OffsetTo != dst.OffsetFrom || dst.OffsetTo != std.OffsetFrom {
		return ""
	}
	return posixName(zoneName(std.Name, std.OffsetTo)) + posixOffset(-std.OffsetTo) +
		posixName(zoneName(dst.Name, dst.OffsetTo)) + posixOffset(-dst.OffsetTo) +
		"," + dstRule + "," + stdRule
}

// openEnded reports whether an observance has a rule with no end.
func openEnded(o *Observance) bool {
	for _, r := range o.Rules {
		if r.Count == 0 && r.Until.IsZero() {
			return true
		}
	}
	return false
}

// posixRule writes an observance's open-ended rule as a POSIX TZ date and
// time, "Mm.w.d/time", when it is an annual nth-weekday rule: the form
// nearly every VTIMEZONE uses.
func posixRule(o *Observance) (string, bool) {
	var rule *recur.Rule
	for i := range o.Rules {
		if o.Rules[i].Count == 0 && o.Rules[i].Until.IsZero() {
			rule = &o.Rules[i]
		}
	}
	r := rule
	if r.Freq != recur.Yearly || max(r.Interval, 1) != 1 || len(r.ByMonth) != 1 || len(r.ByDay) != 1 ||
		len(r.ByHour)+len(r.ByMinute)+len(r.BySecond)+len(r.ByYearDay)+len(r.ByWeekNo)+len(r.BySetPos) > 0 {
		return "", false
	}
	day := r.ByDay[0]
	week := day.N
	switch {
	case len(r.ByMonthDay) == 0 && week >= 1 && week <= 4:
	case len(r.ByMonthDay) == 0 && week == -1:
		week = 5
	case len(r.ByMonthDay) == 7 && week == 0:
		// The RFC 2445 style: BYDAY=SU;BYMONTHDAY=8,9,10,11,12,13,14 is
		// the second Sunday.
		days := slices.Clone(r.ByMonthDay)
		slices.Sort(days)
		if days[0] < 1 || (days[0]-1)%7 != 0 || days[6] != days[0]+6 || days[0] > 22 {
			return "", false
		}
		week = (days[0]-1)/7 + 1
	default:
		return "", false
	}
	secs := o.Start.SecondOfDay()
	t := fmt.Sprintf("M%d.%d.%d", r.ByMonth[0], week, int(day.Day.Time()))
	if secs != 7200 {
		t += "/" + strconv.Itoa(secs/3600)
		if secs%3600 != 0 {
			t += fmt.Sprintf(":%02d", secs/60%60)
			if secs%60 != 0 {
				t += fmt.Sprintf(":%02d", secs%60)
			}
		}
	}
	return t, true
}

// posixName quotes a zone abbreviation for a POSIX TZ string.
func posixName(name string) string {
	clean := strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '+' || r == '-' {
			return r
		}
		return -1
	}, name)
	for len(clean) < 3 {
		clean += "x"
	}
	return "<" + clean + ">"
}

// posixOffset writes seconds as a POSIX TZ offset ("5", "-5:30"). POSIX
// counts west of UTC as positive, so callers pass the negated offset.
func posixOffset(secs int) string {
	sign := ""
	if secs < 0 {
		sign, secs = "-", -secs
	}
	s := sign + strconv.Itoa(secs/3600)
	if secs%3600 != 0 {
		s += fmt.Sprintf(":%02d", secs/60%60)
		if secs%60 != 0 {
			s += fmt.Sprintf(":%02d", secs%60)
		}
	}
	return s
}

// encodeTZif writes a version 2 TZif file (RFC 9636): initial is the
// local time type before the first transition.
func encodeTZif(initial transition, txs []transition, footer string) ([]byte, error) {
	type ttinfo struct {
		offset int
		dst    bool
		name   string
	}
	// Type 0 is the initial type, unused by any transition, which makes
	// readers apply it before the first one.
	types := []ttinfo{{initial.offset, initial.dst, initial.name}}
	index := map[ttinfo]int{}
	var idx []byte
	for _, t := range txs {
		k := ttinfo{t.offset, t.dst, t.name}
		i, ok := index[k]
		if !ok {
			i = len(types)
			index[k] = i
			types = append(types, k)
		}
		idx = append(idx, byte(i))
	}
	// One byte indexes the types, and one byte each type's abbreviation.
	// A real zone has a handful of each.
	if len(types) > 256 {
		return nil, fmt.Errorf("%d local time types, more than TZif allows", len(types))
	}
	var chars []byte
	desig := map[string]int{}
	for _, t := range types {
		if _, ok := desig[t.name]; !ok {
			desig[t.name] = len(chars)
			chars = append(append(chars, t.name...), 0)
		}
	}
	if len(chars) > 256 {
		return nil, fmt.Errorf("zone abbreviations take %d bytes, more than TZif can index", len(chars))
	}

	var b []byte
	header := func(version byte, timecnt, typecnt, charcnt int) {
		b = append(b, "TZif"...)
		b = append(b, version)
		b = append(b, make([]byte, 15)...)
		for _, n := range []int{0, 0, 0, timecnt, typecnt, charcnt} { // isut, isstd, leap
			b = binary.BigEndian.AppendUint32(b, uint32(n))
		}
	}
	// A minimal version 1 block, which version 2 readers skip.
	header('2', 0, 1, 1)
	b = binary.BigEndian.AppendUint32(b, 0)
	b = append(b, 0, 0, 0)
	// The version 2 block, with 64-bit times.
	header('2', len(txs), len(types), len(chars))
	for _, t := range txs {
		b = binary.BigEndian.AppendUint64(b, uint64(t.at))
	}
	b = append(b, idx...)
	for _, t := range types {
		b = binary.BigEndian.AppendUint32(b, uint32(int32(t.offset)))
		dst := byte(0)
		if t.dst {
			dst = 1
		}
		b = append(b, dst, byte(desig[t.name]))
	}
	b = append(b, chars...)
	b = append(b, '\n')
	b = append(b, footer...)
	b = append(b, '\n')
	return b, nil
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
