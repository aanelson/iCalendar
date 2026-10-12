package main

// 2. Writing.

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	ical "github.com/aanelson/iCalendar"
	ics "github.com/arran4/golang-ical"
	eical "github.com/emersion/go-ical"
)

// golang-ical+opts writes with ics.WithNewLineWindows (CRLF); plain
// golang-ical uses its default, which is LF on Unix.
var writers = []string{This, GoIcal, GoIcalO, Emersion}

// writeEvent writes a calendar holding one event with SUMMARY summary and,
// if paramName is set, an ATTENDEE carrying that parameter, through each
// library's own setters.
func writeEvent(lib, summary, paramName, paramValue string) ([]byte, error) {
	stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	switch lib {
	case This:
		cal := ical.NewCalendar("-//probe//EN")
		ev := ical.NewEvent("probe-1", stamp)
		ev.SetSummary(summary)
		if paramName != "" {
			p := ical.NewProperty("ATTENDEE", "mailto:a@example.com")
			p.Params.Set(paramName, paramValue)
			ev.Props.Add(p)
		}
		cal.Add(ev.Component)
		return cal.Bytes()
	case GoIcal, GoIcalO:
		cal := ics.NewCalendar()
		ev := cal.AddEvent("probe-1")
		ev.SetDtStampTime(stamp)
		ev.SetSummary(summary)
		if paramName != "" {
			ev.AddAttendee("mailto:a@example.com", &ics.KeyValues{Key: paramName, Value: []string{paramValue}})
		}
		var buf bytes.Buffer
		var opts []any
		if lib == GoIcalO {
			opts = append(opts, ics.WithNewLineWindows)
		}
		err := cal.SerializeTo(&buf, opts...)
		return buf.Bytes(), err
	case Emersion:
		cal := eical.NewCalendar()
		cal.Props.SetText(eical.PropVersion, "2.0")
		cal.Props.SetText(eical.PropProductID, "-//probe//EN")
		ev := eical.NewEvent()
		ev.Props.SetText(eical.PropUID, "probe-1")
		ev.Props.SetDateTime(eical.PropDateTimeStamp, stamp)
		ev.Props.SetText(eical.PropSummary, summary)
		if paramName != "" {
			p := eical.NewProp("ATTENDEE")
			p.Value = "mailto:a@example.com"
			p.Params.Set(paramName, paramValue)
			ev.Props.Add(p)
		}
		cal.Children = append(cal.Children, ev.Component)
		var buf bytes.Buffer
		err := eical.NewEncoder(&buf).Encode(cal)
		return buf.Bytes(), err
	}
	return nil, fmt.Errorf("no writer")
}

// physLines splits output into physical lines, accepting LF or CRLF.
func physLines(out []byte) []string {
	return strings.Split(strings.TrimSuffix(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n"), "\n")
}

// unfolded returns the content lines of out, unfolded.
func unfolded(out []byte) []string {
	s := strings.ReplaceAll(string(out), "\r\n", "\n")
	s = strings.ReplaceAll(s, "\n ", "")
	s = strings.ReplaceAll(s, "\n\t", "")
	return strings.Split(s, "\n")
}

// lineCheck reports problems with the physical lines of out: line breaks
// that aren't CRLF, lines over 75 octets, lines that aren't valid UTF-8.
func lineCheck(out []byte) (longest int, problems []string) {
	s := string(out)
	crlf := strings.Count(s, "\r\n")
	if lf := strings.Count(s, "\n") - crlf; lf > 0 {
		problems = append(problems, fmt.Sprintf("%d LF-only line breaks (CRLF required)", lf))
	}
	if cr := strings.Count(s, "\r") - crlf; cr > 0 {
		problems = append(problems, fmt.Sprintf("%d bare CR", cr))
	}
	for i, l := range physLines(out) {
		if len(l) > longest {
			longest = len(l)
		}
		if len(l) > 75 {
			problems = append(problems, fmt.Sprintf("line %d is %d octets", i+1, len(l)))
		}
		if !utf8.ValidString(l) {
			problems = append(problems, fmt.Sprintf("line %d is not valid UTF-8 (a rune split by the fold)", i+1))
		}
	}
	return longest, problems
}

func dedupe(ps []string) []string {
	var out []string
	seen := map[string]int{}
	for _, p := range ps {
		k := p
		if i := strings.Index(p, " is "); i > 0 && strings.HasPrefix(p, "line ") {
			k = p[i:]
		}
		seen[k]++
		if seen[k] <= 2 {
			out = append(out, p)
		}
	}
	if len(ps) > len(out) {
		out = append(out, fmt.Sprintf("(%d problems in all)", len(ps)))
	}
	return out
}

// readBack parses out with every parser and reports SUMMARY and any
// ATTENDEE each one sees.
func readBack(out []byte) map[string]string {
	m := map[string]string{}
	for _, lib := range []string{This, GoIcalO, Emersion} {
		var p parsed
		func() {
			defer func() {
				if v := recover(); v != nil {
					p.Err = fmt.Errorf("PANIC in this reader: %v", v)
				}
			}()
			p = parseFeed(lib, out)
		}()
		if p.Err != nil || len(p.Events) == 0 {
			m[lib] = "read error: " + p.summary()
			continue
		}
		var s []string
		if sm := p.Events[0].get("SUMMARY"); sm != nil {
			s = append(s, fmt.Sprintf("SUMMARY=%q", sm.Text))
		}
		for _, a := range p.Events[0].all("ATTENDEE") {
			s = append(s, fmt.Sprintf("ATTENDEE=%q params=%v", a.Raw, a.Params))
		}
		m[lib] = strings.Join(s, " ")
	}
	return m
}

func init() {
	long := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 5)
	register(&Probe{ID: "2.01-fold-long-line", Group: "2 writing",
		Title: "a 233-octet SUMMARY is folded to ≤75 octets with CRLF and reads back",
		RFC:   "§3.1: lines SHOULD NOT exceed 75 octets; CRLF line breaks",
		Libs:  writers,
		Run: func(lib, _ string) Result {
			out, err := writeEvent(lib, long, "", "")
			if err != nil {
				return failed(err)
			}
			longest, probs := lineCheck(out)
			p := parseFeed(lib, out)
			if p.Err == nil && len(p.Events) == 1 && p.Events[0].get("SUMMARY") != nil && p.Events[0].get("SUMMARY").Text != long {
				probs = append(probs, "own reader gets a different SUMMARY back")
			}
			r := Result{Extra: map[string]any{"longest_line": longest}}
			if len(probs) == 0 {
				r.Outcome, r.Got = "pass", fmt.Sprintf("longest line %d octets", longest)
			} else {
				r.Outcome, r.Got = "wrong", strings.Join(dedupe(probs), "; ")
			}
			return r
		}})

	runes := strings.Repeat("é€😀", 20) // 2-, 3- and 4-octet runes, no spaces
	register(&Probe{ID: "2.02-fold-utf8", Group: "2 writing",
		Title: "folding never splits a UTF-8 sequence (SUMMARY of é€😀 ×20)",
		RFC:   "§3.1 note and §6: fold between characters, not inside a multi-octet sequence",
		Libs:  writers,
		Run: func(lib, _ string) Result {
			out, err := writeEvent(lib, runes, "", "")
			if err != nil {
				return failed(err)
			}
			longest, probs := lineCheck(out)
			var split []string
			for _, p := range probs {
				if strings.Contains(p, "UTF-8") {
					split = append(split, p)
				}
			}
			if len(split) > 0 {
				return wrong(strings.Join(dedupe(split), "; "), "every line valid UTF-8")
			}
			if longest <= 75 {
				return pass(fmt.Sprintf("folded, longest line %d octets, no split rune", longest))
			}
			return Result{Outcome: "wrong", Got: fmt.Sprintf("not folded at all (longest line %d octets), so nothing to split", longest), Want: "folded at ≤75 octets on rune boundaries"}
		}})

	register(&Probe{ID: "2.03-text-escape", Group: "2 writing",
		Title: `TEXT escaping of , ; \ and newline`,
		RFC:   "§3.3.11 TEXT: BACKSLASH, SEMICOLON, COMMA escaped; newline as \\n",
		Libs:  writers,
		Run: func(lib, _ string) Result {
			out, err := writeEvent(lib, "a,b;c\\d\ne", "", "")
			if err != nil {
				return failed(err)
			}
			for _, l := range unfolded(out) {
				if strings.HasPrefix(l, "SUMMARY") {
					return check(l, `SUMMARY:a\,b\;c\\d\ne`)
				}
			}
			return wrong("no SUMMARY line in output", "")
		}})

	inj := func(id, title, summary string) {
		register(&Probe{ID: id, Group: "2 writing", Title: title,
			RFC:  "§3.1: a value can't contain a line break; §3.3.11: newline is written \\n. Pass = escaped or refused, never a new content line",
			Libs: writers,
			Run: func(lib, _ string) Result {
				out, err := writeEvent(lib, summary, "", "")
				if err != nil {
					return Result{Outcome: "pass", Got: "refused: " + err.Error()}
				}
				return injectionVerdict(out)
			}})
	}
	inj("2.04a-inject-crlf", `SUMMARY = "x\r\nATTENDEE:mailto:evil@example.com"`, "x\r\nATTENDEE:mailto:evil@example.com")
	inj("2.04b-inject-cr", `SUMMARY = "x\rATTENDEE:mailto:evil@example.com" (lone CR)`, "x\rATTENDEE:mailto:evil@example.com")
	inj("2.04c-inject-lf", `SUMMARY = "x\nATTENDEE:mailto:evil@example.com" (lone LF)`, "x\nATTENDEE:mailto:evil@example.com")

	register(&Probe{ID: "2.04d-inject-param", Group: "2 writing",
		Title: `parameter CN = "x\r\nATTENDEE:mailto:evil@example.com"`,
		RFC:   "§3.1: parameter values can't hold CONTROL characters. Pass = refused or neutralized",
		Libs:  writers,
		Run: func(lib, _ string) Result {
			out, err := writeEvent(lib, "s", "CN", "x\r\nATTENDEE:mailto:evil@example.com")
			if err != nil {
				return Result{Outcome: "pass", Got: "refused: " + err.Error()}
			}
			return injectionVerdict(out)
		}})

	register(&Probe{ID: "2.05-param-colon-quoted", Group: "2 writing",
		Title: `a parameter value containing ':' (CN="Dr: Who; Jr, III") is written as a quoted-string`,
		RFC:   "§3.2: values with COLON, SEMICOLON or COMMA MUST be quoted-strings (no backslash escapes in parameters)",
		Libs:  writers,
		Run: func(lib, _ string) Result {
			val := "Dr: Who; Jr, III"
			out, err := writeEvent(lib, "s", "CN", val)
			if err != nil {
				return failed(err)
			}
			line := ""
			for _, l := range unfolded(out) {
				if strings.HasPrefix(l, "ATTENDEE") {
					line = l
				}
			}
			rb := readBack(out)
			r := Result{Got: line, Want: `ATTENDEE;CN="Dr: Who; Jr, III":mailto:a@example.com`, Extra: map[string]any{"read_back": rb}}
			if strings.Contains(line, `CN="Dr: Who; Jr, III"`) {
				r.Outcome = "pass"
			} else {
				r.Outcome = "wrong"
				r.Detail = fmt.Sprintf("other readers see: this=%s | emersion=%s", rb[This], rb[Emersion])
			}
			return r
		}})
}

// injectionVerdict checks out for a physical line that is the injected
// property, for raw CRs inside a line, and asks every reader whether it
// sees an ATTENDEE:mailto:evil… (the source event has none).
var contentLineRE = regexp.MustCompile(`^[A-Za-z0-9-]+[;:]`)

func injectionVerdict(out []byte) Result {
	rb := readBack(out)
	var injected []string
	for _, lib := range []string{This, GoIcalO, Emersion} {
		if strings.Contains(rb[lib], "ATTENDEE=\"mailto:evil") {
			injected = append(injected, lib)
		}
	}
	rawLine, rawCR := false, false
	var malformed []string
	for _, l := range physLines(out) {
		if strings.HasPrefix(l, "ATTENDEE:mailto:evil") {
			rawLine = true
		}
		if strings.Contains(l, "\r") {
			rawCR = true
		}
	}
	for _, l := range unfolded(out) {
		if l != "" && !contentLineRE.MatchString(l) {
			malformed = append(malformed, fmt.Sprintf("%q", l))
		}
	}
	r := Result{Extra: map[string]any{"read_back": rb, "output": string(out)}}
	switch {
	case rawLine:
		r.Outcome = "wrong"
		r.Got = "INJECTED: the output has a CRLF-delimited line ATTENDEE:mailto:evil@example.com"
		if len(injected) > 0 {
			r.Got += fmt.Sprintf("; readers that report the extra ATTENDEE: %v", injected)
		}
	case len(injected) > 0:
		r.Outcome = "wrong"
		r.Got = fmt.Sprintf("INJECTED via a raw CR in the output: readers %v take the lone CR as a line break and report an extra ATTENDEE:mailto:evil@example.com", injected)
	case len(malformed) > 0:
		r.Outcome = "wrong"
		r.Got = "raw line break written: the output has lines that aren't content lines: " + strings.Join(malformed, ", ")
	case rawCR:
		r.Outcome = "wrong"
		r.Got = "raw CR written inside a line (readers that accept a lone CR as a line break would split it)"
	default:
		r.Outcome = "pass"
		r.Got = "escaped"
	}
	r.Detail = fmt.Sprintf("this=%s | golang-ical=%s | emersion=%s", rb[This], rb[GoIcalO], rb[Emersion])
	return r
}
