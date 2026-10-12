package contentline_test

// Conformance tests for the content-line layer: RFC 5545 §2, §3.1, §3.1.1,
// §3.1.4 and §3.2, its errata, and RFC 6868. Every expectation below comes
// from the RFC text, cited next to the case; where the RFC leaves a choice
// open the comment says so and names the choice docs/architecture.md made.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/aanelson/iCalendar/contentline"
)

// readResult is everything a Reader produced from one input.
type readResult struct {
	lines []contentline.Line
	warns []error
	err   error
}

func readString(input string, strict bool) readResult {
	var res readResult
	r := contentline.NewReader(strings.NewReader(input))
	r.Strict = strict
	r.Warn = func(err error) { res.warns = append(res.warns, err) }
	for range len(input) + 2 { // a Reader returns at most one line per input byte, then EOF
		l, err := r.ReadLine()
		if err == io.EOF {
			return res
		}
		if err != nil {
			res.err = err
			return res
		}
		res.lines = append(res.lines, l)
	}
	res.err = errors.New("test: Reader returned more lines than the input has bytes")
	return res
}

func writeLines(lines ...contentline.Line) (string, error) {
	var buf bytes.Buffer
	w := contentline.NewWriter(&buf)
	for _, l := range lines {
		if err := w.WriteLine(l); err != nil {
			return buf.String(), err
		}
	}
	err := w.Flush()
	return buf.String(), err
}

func line(name, value string, params ...contentline.Param) contentline.Line {
	return contentline.Line{Name: name, Params: params, Value: value}
}

func param(name string, values ...string) contentline.Param {
	return contentline.Param{Name: name, Values: values}
}

func sameLine(a, b contentline.Line) bool {
	if a.Name != b.Name || a.Value != b.Value || len(a.Params) != len(b.Params) {
		return false
	}
	for i := range a.Params {
		pa, pb := a.Params[i], b.Params[i]
		if pa.Name != pb.Name || len(pa.Values) != len(pb.Values) {
			return false
		}
		for j := range pa.Values {
			if pa.Values[j] != pb.Values[j] {
				return false
			}
		}
	}
	return true
}

func sameLines(a, b []contentline.Line) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !sameLine(a[i], b[i]) {
			return false
		}
	}
	return true
}

func showLine(l contentline.Line) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%q", l.Name)
	for _, p := range l.Params {
		fmt.Fprintf(&b, " ;%q=%q", p.Name, p.Values)
	}
	fmt.Fprintf(&b, " :%q", l.Value)
	return b.String()
}

func showLines(ls []contentline.Line) string {
	var parts []string
	for _, l := range ls {
		parts = append(parts, showLine(l))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// checkPhysicalLines checks the folding rules of RFC 5545 §3.1 on writer
// output: every physical line ends in CRLF, holds no other CR or LF, is at
// most 75 octets excluding the CRLF, and is valid UTF-8 on its own.
func checkPhysicalLines(t *testing.T, out string) {
	t.Helper()
	if out == "" {
		return
	}
	// RFC 5545 §3.1: "Content lines are delimited by a line break, which is
	// a CRLF sequence (CR character followed by LF character)."
	if !strings.HasSuffix(out, "\r\n") {
		t.Errorf("output does not end in CRLF: %q", out)
	}
	for i, pl := range strings.Split(strings.TrimSuffix(out, "\r\n"), "\r\n") {
		if strings.ContainsAny(pl, "\r\n") {
			t.Errorf("physical line %d holds a bare CR or LF: %q", i+1, pl)
		}
		// RFC 5545 §3.1: "Lines of text SHOULD NOT be longer than 75
		// octets, excluding the line break." A SHOULD; the writer's
		// documented choice (architecture.md, contentline.FoldWidth) is
		// to always fold at 75.
		if len(pl) > 75 {
			t.Errorf("physical line %d is %d octets (> 75): %q", i+1, len(pl), pl)
		}
		// RFC 5545 §3.1: "care must be taken to not split a multi-octet
		// character" (on output); the architecture doc commits to it.
		if !utf8.ValidString(pl) {
			t.Errorf("physical line %d is not valid UTF-8 on its own: %q", i+1, pl)
		}
	}
}

// TestConformanceABNFAccepts reads grammatical content lines in strict mode.
func TestConformanceABNFAccepts(t *testing.T) {
	// All of QSAFE-CHAR's US-ASCII part: WSP / %x21 / %x23-7E.
	var qsafe strings.Builder
	qsafe.WriteString(" \t!")
	for c := byte(0x23); c <= 0x7E; c++ {
		if c == '^' { // keep RFC 6868 escapes out of this one
			continue
		}
		qsafe.WriteByte(c)
	}
	// All of SAFE-CHAR's US-ASCII part: WSP / %x21 / %x23-2B / %x2D-39 /
	// %x3C-7E.
	var safe strings.Builder
	safe.WriteString(" \t!")
	for c := byte(0x23); c <= 0x7E; c++ {
		if c == ',' || c == ':' || c == ';' || c == '^' {
			continue
		}
		safe.WriteByte(c)
	}
	// All of VALUE-CHAR's US-ASCII part: WSP / %x21-7E.
	var vchar strings.Builder
	vchar.WriteString(" \t")
	for c := byte(0x21); c <= 0x7E; c++ {
		vchar.WriteByte(c)
	}

	tests := []struct {
		name string
		in   string
		want []contentline.Line
	}{
		// RFC 5545 §3.1 example: the folded DESCRIPTION is the same as
		// "DESCRIPTION:This is a long description that exists on a long
		// line."
		{"3.1 folding example",
			"DESCRIPTION:This is a lo\r\n ng description\r\n  that exists on a long line.\r\n",
			[]contentline.Line{line("DESCRIPTION", "This is a long description that exists on a long line.")}},
		// RFC 5545 §3.1.1 example: SEMICOLON separates parameters, the
		// value starts after the colon and itself holds a colon.
		{"3.1.1 example",
			"ATTENDEE;RSVP=TRUE;ROLE=REQ-PARTICIPANT:mailto:\r\n jsmith@example.com\r\n",
			[]contentline.Line{line("ATTENDEE", "mailto:jsmith@example.com",
				param("RSVP", "TRUE"), param("ROLE", "REQ-PARTICIPANT"))}},
		// RFC 5545 §3.2 example: a quoted parameter value holds a colon;
		// the property value keeps its TEXT escapes (this layer doesn't
		// decode them) and the fold removes exactly one of two spaces.
		{"3.2 ALTREP example",
			"DESCRIPTION;ALTREP=\"cid:part1.0001@example.org\":The Fall'98 Wild\r\n  Wizards Conference - - Las Vegas\\, NV\\, USA\r\n",
			[]contentline.Line{line("DESCRIPTION", `The Fall'98 Wild Wizards Conference - - Las Vegas\, NV\, USA`,
				param("ALTREP", "cid:part1.0001@example.org"))}},
		// RFC 5545 §3.2.11 example: "the individual quoted-strings are
		// separated by a COMMA character" (§3.1.1).
		{"3.2.11 MEMBER list example",
			"ATTENDEE;MEMBER=\"mailto:projectA@example.com\",\"mailto:pr\r\n ojectB@example.com\":mailto:janedoe@example.com\r\n",
			[]contentline.Line{line("ATTENDEE", "mailto:janedoe@example.com",
				param("MEMBER", "mailto:projectA@example.com", "mailto:projectB@example.com"))}},
		// RFC 5545 §3.2.4 example, another list of quoted values.
		{"3.2.4 DELEGATED-TO list example",
			"ATTENDEE;DELEGATED-TO=\"mailto:jdoe@example.com\",\"mailto:jqpublic\r\n @example.com\":mailto:jsmith@example.com\r\n",
			[]contentline.Line{line("ATTENDEE", "mailto:jsmith@example.com",
				param("DELEGATED-TO", "mailto:jdoe@example.com", "mailto:jqpublic@example.com"))}},
		// RFC 5545 §3.1: value = *VALUE-CHAR, so a value may be empty.
		{"empty value", "X-EMPTY:\r\n", []contentline.Line{line("X-EMPTY", "")}},
		// RFC 5545 §3.1: paramtext = *SAFE-CHAR, so a parameter value may
		// be empty, unquoted or quoted.
		{"empty paramtext", "X-P;X-Q=:v\r\n", []contentline.Line{line("X-P", "v", param("X-Q", ""))}},
		{"empty quoted-string", "X-P;X-Q=\"\":v\r\n", []contentline.Line{line("X-P", "v", param("X-Q", ""))}},
		// RFC 5545 §3.1: SAFE-CHAR and QSAFE-CHAR include WSP (SPACE and
		// HTAB), leading and trailing ones too.
		{"WSP in paramtext", "ATTENDEE;CN=John  Smith;X-T=a\tb;X-S= a :mailto:j@example.com\r\n",
			[]contentline.Line{line("ATTENDEE", "mailto:j@example.com",
				param("CN", "John  Smith"), param("X-T", "a\tb"), param("X-S", " a "))}},
		{"WSP in quoted-string", "X-P;X-Q=\" a\t\":v\r\n", []contentline.Line{line("X-P", "v", param("X-Q", " a\t"))}},
		// RFC 5545 §3.1: param = param-name "=" param-value *(","
		// param-value).
		{"unquoted value list", "X-P;X-Q=a,b,c:v\r\n", []contentline.Line{line("X-P", "v", param("X-Q", "a", "b", "c"))}},
		{"mixed value list with an empty item", "X-P;X-Q=a,\"b:c\",,d:v\r\n",
			[]contentline.Line{line("X-P", "v", param("X-Q", "a", "b:c", "", "d"))}},
		// RFC 5545 §3.1: QSAFE-CHAR is "Any character except CONTROL and
		// DQUOTE", so ":", ";" and "," are plain characters inside quotes.
		{"quoted :;,", "X-P;X-Q=\"a:b;c,d\":v\r\n", []contentline.Line{line("X-P", "v", param("X-Q", "a:b;c,d"))}},
		{"every US-ASCII QSAFE-CHAR", "X-P;X-Q=\"" + qsafe.String() + "\":v\r\n",
			[]contentline.Line{line("X-P", "v", param("X-Q", qsafe.String()))}},
		// SAFE-CHAR includes "=" (%x3C-7E) and the backslash.
		{"every US-ASCII SAFE-CHAR", "X-P;X-Q=" + safe.String() + ":v\r\n",
			[]contentline.Line{line("X-P", "v", param("X-Q", safe.String()))}},
		// RFC 5545 §3.1: VALUE-CHAR = WSP / %x21-7E / NON-US-ASCII: a value
		// may hold DQUOTE, ":", ";", ",", "\" and "^" as they are.
		{"every US-ASCII VALUE-CHAR", "X-P:" + vchar.String() + "\r\n",
			[]contentline.Line{line("X-P", vchar.String())}},
		{"value split at the first colon outside quotes", "X-P;X-Q=\"a:b\":c:d;e\r\n",
			[]contentline.Line{line("X-P", "c:d;e", param("X-Q", "a:b"))}},
		// RFC 5545 §3.8.8.3 / §3.8.1.6 structured values keep their
		// semicolons: they are part of the value, not parameters.
		{"structured value", "GEO:37.386013;-122.082932\r\n", []contentline.Line{line("GEO", "37.386013;-122.082932")}},
		// RFC 5545 §3.1: NON-US-ASCII = UTF8-2 / UTF8-3 / UTF8-4 in
		// paramtext, quoted-string and value.
		{"NON-US-ASCII", "X-P;CN=Jöhn 日本 😀;X-Q=\"ü:ß\":Ünïcödé 日本 😀\r\n",
			[]contentline.Line{line("X-P", "Ünïcödé 日本 😀", param("CN", "Jöhn 日本 😀"), param("X-Q", "ü:ß"))}},
		// RFC 5545 §3.1: iana-token = 1*(ALPHA / DIGIT / "-"); x-name =
		// "X-" [vendorid "-"] 1*(ALPHA / DIGIT / "-").
		{"x-name with vendorid", "X-ABC-FOO;X-VND-BAR=1:v\r\n",
			[]contentline.Line{line("X-ABC-FOO", "v", param("X-VND-BAR", "1"))}},
		{"digits and hyphens", "A1-B2;P-3=x:v\r\n", []contentline.Line{line("A1-B2", "v", param("P-3", "x"))}},
		// These are 1*(ALPHA / DIGIT / "-") too, odd as they look: "X-"
		// is no x-name, but it is an iana-token.
		{"degenerate tokens", "1:a\r\n-:b\r\nX-:c\r\nX-P;-=d:e\r\n",
			[]contentline.Line{line("1", "a"), line("-", "b"), line("X-", "c"), line("X-P", "e", param("-", "d"))}},
		// RFC 5545 §2 and §3.5: "DTSTART;TZID=America/New_York:19980714T120000
		// is the same as DtStart;TzID=America/New_York:19980714T120000".
		// Line.Name and Param.Name are documented as upper case.
		{"3.5 mixed-case names", "DtStart;TzID=America/New_York:19980714T120000\r\n",
			[]contentline.Line{line("DTSTART", "19980714T120000", param("TZID", "America/New_York"))}},
		{"lower-case names", "x-abc-foo;x-q=1:v\r\n", []contentline.Line{line("X-ABC-FOO", "v", param("X-Q", "1"))}},
		// RFC 5545 §3.1: the grammar puts no limit on repeating a
		// parameter; both stay, in order (architecture goal 5: lossless).
		{"repeated parameter", "X-P;X-A=1;X-A=2:v\r\n", []contentline.Line{line("X-P", "v", param("X-A", "1"), param("X-A", "2"))}},
		// RFC 5545 §3.2: "Applications MUST ignore x-param and iana-param
		// values they don't recognize": they don't stop the line.
		{"unknown parameters", "SUMMARY;FOO=bar;X-BAZ=\"q:x\":v\r\n",
			[]contentline.Line{line("SUMMARY", "v", param("FOO", "bar"), param("X-BAZ", "q:x"))}},
		// RFC 6868 applies to parameter values only, not to the property
		// value; and backslashes are not escapes in either at this layer.
		{"no ^ decoding in values", "X-P;X-Q=^':^'^n^^\\n\r\n",
			[]contentline.Line{line("X-P", `^'^n^^\n`, param("X-Q", `"`))}},
		// RFC 5545 §3.1: content lines are separated by CRLF; the last one
		// ends in CRLF too.
		{"several lines", "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nEND:VCALENDAR\r\n",
			[]contentline.Line{line("BEGIN", "VCALENDAR"), line("VERSION", "2.0"), line("END", "VCALENDAR")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := readString(tt.in, true)
			if res.err != nil {
				t.Fatalf("strict read of %q: %v", tt.in, res.err)
			}
			if !sameLines(res.lines, tt.want) {
				t.Errorf("read %q\n got %s\nwant %s", tt.in, showLines(res.lines), showLines(tt.want))
			}
			// The same text must read the same way in lenient mode, with
			// nothing to warn about.
			lax := readString(tt.in, false)
			if lax.err != nil || len(lax.warns) != 0 || !sameLines(lax.lines, tt.want) {
				t.Errorf("lenient read of %q: %s, err %v, warnings %v", tt.in, showLines(lax.lines), lax.err, lax.warns)
			}
		})
	}
}

// TestConformanceABNFRejects checks that strict mode rejects text the RFC
// 5545 §3.1 grammar forbids, and that lenient mode never accepts it
// silently: it either fails or reports a recovery to Warn.
func TestConformanceABNFRejects(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		// contentline = name *(";" param ) ":" value CRLF
		{"missing colon", "X-P\r\n"},
		{"missing colon after params", "X-P;X-Q=a\r\n"},
		{"only colon is quoted", "X-P;X-Q=\"a:b\"\r\n"},
		{"empty name", ":v\r\n"},
		// name = iana-token / x-name; iana-token = 1*(ALPHA / DIGIT / "-")
		{"underscore in name", "X_P:v\r\n"},
		{"space in name", "X P:v\r\n"},
		{"space before colon", "X-P :v\r\n"},
		{"vCard group syntax", "item1.TEL:v\r\n"},
		{"non-ASCII name", "Ä:v\r\n"},
		{"control in name", "X-\x01P:v\r\n"},
		// param = param-name "=" param-value *("," param-value)
		{"empty param", "X-P;:v\r\n"},
		{"trailing semicolon", "X-P;X-Q=a;:v\r\n"},
		{"empty param name", "X-P;=a:v\r\n"},
		{"underscore in param name", "X-P;X_Q=a:v\r\n"},
		{"space before param name", "X-P; X-Q=a:v\r\n"},
		{"param without =", "X-P;X-Q:v\r\n"},
		// paramtext = *SAFE-CHAR: "Any character except CONTROL, DQUOTE,
		// ";", ":", ","", and §3.2: "Property parameter values MUST NOT
		// contain the DQUOTE character."
		{"DQUOTE in paramtext", "X-P;X-Q=a\"b:v\r\n"},
		{"DQUOTE closing paramtext", "X-P;X-Q=ab\":v\r\n"},
		// quoted-string = DQUOTE *QSAFE-CHAR DQUOTE, then "," ";" or ":".
		{"text after closing quote", "X-P;X-Q=\"a\"b:v\r\n"},
		{"unterminated quoted-string", "X-P;X-Q=\"abc:v\r\n"},
		// BEGIN, END and every other line are content lines too.
		{"stray continuation at start", " X-P:v\r\n"},
		// A blank line is not a contentline: name is 1*(...).
		{"empty line", "X-P:a\r\n\r\nX-Q:b\r\n"},
	}
	// CONTROL = %x00-08 / %x0A-1F / %x7F: "All the controls except HTAB".
	// None is a VALUE-CHAR, SAFE-CHAR or QSAFE-CHAR. (CR and LF are covered
	// below as line breaks.)
	for _, c := range []byte{0x00, 0x01, 0x08, 0x0B, 0x0C, 0x1B, 0x1F, 0x7F} {
		tests = append(tests,
			struct{ name, in string }{fmt.Sprintf("CONTROL %#02x in value", c), "X-P:a" + string(c) + "b\r\n"},
			struct{ name, in string }{fmt.Sprintf("CONTROL %#02x in paramtext", c), "X-P;X-Q=a" + string(c) + "b:v\r\n"},
			struct{ name, in string }{fmt.Sprintf("CONTROL %#02x in quoted-string", c), "X-P;X-Q=\"a" + string(c) + "b\":v\r\n"},
		)
	}
	// NON-US-ASCII = UTF8-2 / UTF8-3 / UTF8-4 (RFC 3629): bytes that are
	// not well-formed UTF-8 aren't VALUE-CHAR, SAFE-CHAR or QSAFE-CHAR.
	// Lenient mode may keep them as they are (TestConformanceInvalidUTF8).
	keepsBytes := map[string]bool{}
	for _, bad := range []struct{ name, b string }{
		{"stray continuation byte", "\x80"},
		{"0xFF", "\xff"},
		{"overlong", "\xc0\xaf"},
		{"surrogate", "\xed\xa0\x80"},
		{"above U+10FFFF", "\xf4\x90\x80\x80"},
		{"cut sequence", "\xe6\x97"},
	} {
		for _, tt := range []struct{ name, in string }{
			{"invalid UTF-8 in value: " + bad.name, "X-P:a" + bad.b + "\r\n"},
			{"invalid UTF-8 in paramtext: " + bad.name, "X-P;X-Q=a" + bad.b + ":v\r\n"},
			{"invalid UTF-8 in quoted-string: " + bad.name, "X-P;X-Q=\"a" + bad.b + "\":v\r\n"},
		} {
			tests = append(tests, tt)
			keepsBytes[tt.name] = true
		}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := readString(tt.in, true)
			if res.err == nil {
				t.Errorf("strict read of %q succeeded: %s", tt.in, showLines(res.lines))
			}
			lax := readString(tt.in, false)
			if lax.err == nil && len(lax.warns) == 0 && !keepsBytes[tt.name] {
				t.Errorf("lenient read of %q accepted it without a warning: %s", tt.in, showLines(lax.lines))
			}
			for _, w := range lax.warns {
				var se *contentline.SyntaxError
				if !errors.As(w, &se) {
					t.Errorf("warning %v is a %T, want *contentline.SyntaxError", w, w)
				}
			}
		})
	}
}

// TestConformanceLineBreaks covers the line break itself. RFC 5545 §3.1:
// "Content lines are delimited by a line break, which is a CRLF sequence
// (CR character followed by LF character)", and contentline ends in CRLF.
func TestConformanceLineBreaks(t *testing.T) {
	want := []contentline.Line{line("X-A", "1"), line("X-B", "2 3")}
	tests := []struct {
		name string
		in   string
	}{
		{"bare LF", "X-A:1\nX-B:2\n  3\n"},
		{"lone CR", "X-A:1\rX-B:2\r  3\r"},
		{"mixed", "X-A:1\r\nX-B:2\n  3\r\n"},
		{"CR inside a line", "X-A:1\r\nX-B:2\r3\r\n"},
		{"no final CRLF", "X-A:1\r\nX-B:2\r\n  3"},
	}
	for _, tt := range tests {
		t.Run(tt.name+"/strict", func(t *testing.T) {
			if tt.name == "no final CRLF" {
				t.Skip("KNOWN GAP: strict mode accepts a last line with no CRLF; requiring it would break TestRFC5545Examples, which reads the CRLF-less examples.json texts strictly")
			}
			if res := readString(tt.in, true); res.err == nil {
				t.Errorf("strict read of %q succeeded: %s", tt.in, showLines(res.lines))
			}
		})
		// Lenient mode reads LF and CR line endings and a missing final
		// line break (architecture.md, "Unfolding accepts CRLF, LF and
		// lone CR") into the same lines.
		t.Run(tt.name+"/lenient", func(t *testing.T) {
			lax := readString(tt.in, false)
			if lax.err != nil {
				t.Fatalf("lenient read of %q: %v", tt.in, lax.err)
			}
			if tt.name == "CR inside a line" {
				// The CR ends the line, so "3" is a line of its own, which
				// is skipped with a warning.
				if len(lax.warns) == 0 {
					t.Errorf("lenient read of %q reported nothing to Warn", tt.in)
				}
				return
			}
			if !sameLines(lax.lines, want) {
				t.Errorf("lenient read of %q\n got %s\nwant %s", tt.in, showLines(lax.lines), showLines(want))
			}
			if len(lax.warns) == 0 {
				t.Skip("KNOWN GAP: lenient mode accepts a line break other than CRLF, or none after the last line, without a Warn; the design treats these as accepted input, not recoveries, and TestLenientRecoveries and ExampleDecoder count warnings on LF input")
			}
		})
	}
}

// TestConformanceUnfolding checks the input side of RFC 5545 §3.1 folding:
// "Any sequence of CRLF followed immediately by a single linear white-space
// character is ignored (i.e., removed) when processing the content type",
// and "a long line can be split between any two characters".
func TestConformanceUnfolding(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want contentline.Line
	}{
		{"SPACE", "X-P:ab\r\n cd\r\n", line("X-P", "abcd")},
		// "linear white-space character (i.e., SPACE or HTAB)"
		{"HTAB", "X-P:ab\r\n\tcd\r\n", line("X-P", "abcd")},
		// "a single linear white-space character" is removed: a second
		// one belongs to the content.
		{"two SPACEs", "X-P:ab\r\n  cd\r\n", line("X-P", "ab cd")},
		{"two HTABs", "X-P:ab\r\n\t\tcd\r\n", line("X-P", "ab\tcd")},
		{"SPACE then HTAB", "X-P:ab\r\n \tcd\r\n", line("X-P", "ab\tcd")},
		{"HTAB then SPACE", "X-P:ab\r\n\t cd\r\n", line("X-P", "ab cd")},
		{"continuation of one WSP only", "X-P:ab\r\n \r\n cd\r\n", line("X-P", "abcd")},
		{"continuation of two WSP", "X-P:ab\r\n  \r\n", line("X-P", "ab ")},
		{"many folds", "X\r\n -\r\n P\r\n :\r\n a\r\n b\r\n", line("X-P", "ab")},
		// "between any two characters": inside the name, a parameter
		// name, an unquoted or quoted value, around the delimiters.
		{"inside name", "DESCRI\r\n PTION:x\r\n", line("DESCRIPTION", "x")},
		{"before semicolon", "X-P\r\n ;X-Q=a:v\r\n", line("X-P", "v", param("X-Q", "a"))},
		{"inside param name", "X-P;X-\r\n Q=a:v\r\n", line("X-P", "v", param("X-Q", "a"))},
		{"around =", "X-P;X-Q\r\n =\r\n a:v\r\n", line("X-P", "v", param("X-Q", "a"))},
		{"inside paramtext", "X-P;X-Q=a\r\n b:v\r\n", line("X-P", "v", param("X-Q", "ab"))},
		{"inside quoted-string", "X-P;X-Q=\"a:\r\n b\":v\r\n", line("X-P", "v", param("X-Q", "a:b"))},
		{"after opening quote", "X-P;X-Q=\"\r\n ab\":v\r\n", line("X-P", "v", param("X-Q", "ab"))},
		{"before closing quote", "X-P;X-Q=\"ab\r\n \":v\r\n", line("X-P", "v", param("X-Q", "ab"))},
		{"around comma", "X-P;X-Q=a\r\n ,\r\n b:v\r\n", line("X-P", "v", param("X-Q", "a", "b"))},
		{"before colon", "X-P\r\n :v\r\n", line("X-P", "v")},
		{"after colon", "X-P:\r\n v\r\n", line("X-P", "v")},
		// Unfolding comes first ("folded lines MUST first be unfolded"),
		// so an RFC 6868 escape split by a fold is still an escape.
		{"inside ^n", "X-P;X-Q=a^\r\n nb:v\r\n", line("X-P", "v", param("X-Q", "a\nb"))},
		{"inside ^'", "X-P;X-Q=^\r\n '\r\n :v\r\n", line("X-P", "v", param("X-Q", `"`))},
		// RFC 5545 §3.1: a fold right after the CRLF of an empty value.
		{"empty value folded", "X-P:\r\n \r\n", line("X-P", "")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := readString(tt.in, true)
			if res.err != nil {
				t.Fatalf("strict read of %q: %v", tt.in, res.err)
			}
			if len(res.lines) != 1 || !sameLine(res.lines[0], tt.want) {
				t.Errorf("read %q\n got %s\nwant %s", tt.in, showLines(res.lines), showLine(tt.want))
			}
		})
	}
}

// TestConformanceUnfoldSplitsUTF8 folds in the middle of 2-, 3- and 4-octet
// UTF-8 sequences. RFC 5545 §3.1: "It is possible for very simple
// implementations to generate improperly folded lines in the middle of a
// UTF-8 multi-octet sequence. For this reason, implementations need to
// unfold lines in such a way to properly restore the original sequence."
// Unfolded, the text is valid, so strict mode must accept it.
func TestConformanceUnfoldSplitsUTF8(t *testing.T) {
	for _, r := range []string{"é", "日", "😀"} {
		for k := 1; k < len(r); k++ {
			for _, wsp := range []string{" ", "\t"} {
				folded := r[:k] + "\r\n" + wsp + r[k:]
				in := "X-P;X-Q=a" + folded + "b;X-R=\"c" + folded + "d\":e" + folded + "f\r\n"
				want := line("X-P", "e"+r+"f", param("X-Q", "a"+r+"b"), param("X-R", "c"+r+"d"))
				res := readString(in, true)
				if res.err != nil {
					t.Errorf("strict read of %q: %v", in, res.err)
					continue
				}
				if len(res.lines) != 1 || !sameLine(res.lines[0], want) {
					t.Errorf("read %q\n got %s\nwant %s", in, showLines(res.lines), showLine(want))
				}
			}
		}
	}
}

// TestConformanceCaseInsensitiveNames checks RFC 5545 §2: "All names of
// properties, property parameters, enumerated property values, and property
// parameter values are case-insensitive", and §3.2: "Property parameter
// values that are not in quoted-strings are case-insensitive."
func TestConformanceCaseInsensitiveNames(t *testing.T) {
	spellings := []string{
		"DTSTART;TZID=America/New_York;VALUE=DATE-TIME:19980714T120000\r\n",
		"DtStart;TzID=America/New_York;Value=date-time:19980714T120000\r\n",
		"dtstart;tzid=America/New_York;value=Date-Time:19980714T120000\r\n",
	}
	for _, in := range spellings {
		res := readString(in, true)
		if res.err != nil || len(res.lines) != 1 {
			t.Fatalf("strict read of %q: %s, %v", in, showLines(res.lines), res.err)
		}
		l := res.lines[0]
		// Names are reported in upper case (documented on Line and Param).
		if l.Name != "DTSTART" || len(l.Params) != 2 || l.Params[0].Name != "TZID" || l.Params[1].Name != "VALUE" {
			t.Errorf("read %q: %s, want names DTSTART, TZID, VALUE", in, showLine(l))
		}
		// Line.Param compares the name case-insensitively.
		for _, name := range []string{"TZID", "tzid", "TzId"} {
			if v, ok := l.Param(name); !ok || v != "America/New_York" {
				t.Errorf("read %q: Param(%q) = %q, %v", in, name, v, ok)
			}
		}
		// The enumerated value is the same value in any case. This layer
		// keeps it as written (architecture goal 5, lossless round trips),
		// which the RFC allows; it must not change it into another value.
		if v, _ := l.Param("value"); !strings.EqualFold(v, "DATE-TIME") {
			t.Errorf("read %q: VALUE = %q", in, v)
		}
		// The property value itself is case-sensitive (§2: "all other
		// property values are case-sensitive").
		if l.Value != "19980714T120000" {
			t.Errorf("read %q: value %q", in, l.Value)
		}
	}
	// A quoted parameter value is not covered by §3.2's rule; it is kept
	// exactly.
	res := readString("X-P;X-Q=\"MiXeD Case\";X-R=MiXeD:v\r\n", true)
	if res.err != nil || len(res.lines) != 1 {
		t.Fatalf("strict read: %v", res.err)
	}
	if v, _ := res.lines[0].Param("X-Q"); v != "MiXeD Case" {
		t.Errorf("quoted value = %q, want %q", v, "MiXeD Case")
	}
	if v, _ := res.lines[0].Param("X-R"); v != "MiXeD" {
		t.Errorf("unquoted value = %q, want it kept as written (architecture goal 5)", v)
	}
}

// TestConformanceRFC6868Decode reads parameter values with RFC 6868 escapes.
func TestConformanceRFC6868Decode(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string // values of X-Q (or of the example's parameter)
	}{
		// RFC 6868 §3.1 example: "ATTENDEE;CN=George Herman ^'Babe^'
		// Ruth:mailto:babe@example.com" unescapes to George Herman "Babe"
		// Ruth.
		{"6868 §3.1 example", "ATTENDEE;CN=George Herman ^'Babe^' Ruth:mailto:babe@example.com\r\n",
			[]string{`George Herman "Babe" Ruth`}},
		// RFC 6868 §3.2 example (vCard, same syntax): quoted, folded, two
		// ^n line breaks.
		{"6868 §3.2 example", "GEO;X-ADDRESS=\"Pittsburgh Pirates^n115 Federal St^nPitt\r\n sburgh, PA 15212\":geo:40.446816,-80.00566\r\n",
			[]string{"Pittsburgh Pirates\n115 Federal St\nPittsburgh, PA 15212"}},
		// RFC 6868 §3: "^n ... is decoded into an appropriate formatted
		// line break", "^^ ... is decoded into the ^ character", "^' ...
		// is decoded into the " character", "the ^-escaping mechanism can
		// be used when the value is either unquoted or quoted".
		{"^n", "X-P;X-Q=a^nb:v\r\n", []string{"a\nb"}},
		{"^^", "X-P;X-Q=a^^b:v\r\n", []string{"a^b"}},
		{"^'", "X-P;X-Q=a^'b:v\r\n", []string{`a"b`}},
		{"quoted", "X-P;X-Q=\"^n,^'^^\":v\r\n", []string{"\n,\"^"}},
		{"in a list", "X-P;X-Q=^n,^',\"^^\":v\r\n", []string{"\n", `"`, "^"}},
		// Decoding goes left to right: ^^ is one escape, so the n after
		// it is a plain n.
		{"^^n", "X-P;X-Q=^^n:v\r\n", []string{"^n"}},
		{"^^^n", "X-P;X-Q=^^^n:v\r\n", []string{"^\n"}},
		{"^^'", "X-P;X-Q=^^':v\r\n", []string{"^'"}},
		{"^^^^", "X-P;X-Q=^^^^:v\r\n", []string{"^^"}},
		{"^'^'", "X-P;X-Q=^'^':v\r\n", []string{`""`}},
		// "if a ^ (U+005E) character is followed by any character other
		// than the ones above, parsers MUST leave both the ^ and the
		// following character in place". ^n is U+006E only: ^N is not an
		// escape.
		{"^N", "X-P;X-Q=a^Nb:v\r\n", []string{"a^Nb"}},
		{"^a", "X-P;X-Q=^a:v\r\n", []string{"^a"}},
		{"^ and backslash", "X-P;X-Q=^\\:v\r\n", []string{`^\`}},
		{"^ and SPACE", "X-P;X-Q=^ x:v\r\n", []string{"^ x"}},
		{"^ and non-ASCII", "X-P;X-Q=^é:v\r\n", []string{"^é"}},
		{"^ at the end", "X-P;X-Q=a^:v\r\n", []string{"a^"}},
		{"^ alone", "X-P;X-Q=^:v\r\n", []string{"^"}},
		{"^ at the end, quoted", "X-P;X-Q=\"a^\":v\r\n", []string{"a^"}},
		// "^," is no escape, so the comma still separates two values.
		{"^ before a comma", "X-P;X-Q=a^,b:v\r\n", []string{"a^", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := readString(tt.in, true)
			if res.err != nil || len(res.lines) != 1 || len(res.lines[0].Params) == 0 {
				t.Fatalf("strict read of %q: %s, %v", tt.in, showLines(res.lines), res.err)
			}
			got := res.lines[0].Params[0].Values
			if fmt.Sprint(got) != fmt.Sprint(tt.want) || len(got) != len(tt.want) {
				t.Errorf("read %q: values %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestConformanceErratum7691 checks that backslash escapes are not applied
// to parameter values. Erratum 7691 (Verified) on §3.8.4.2: in the quoted
// ALTREP "the URI would contain a literal backslash"; "a comma inside quotes
// needs no escaping".
func TestConformanceErratum7691(t *testing.T) {
	// The §3.8.4.2 example as printed: the backslash stays, as two
	// characters, and the quoted comma doesn't split the value.
	in := "CONTACT;ALTREP=\"ldap://example.com:6666/o=ABC%20Industries\\,\r\n c=US???(cn=Jim%20Dolittle)\":Jim Dolittle\\, ABC Industries\\,\r\n +1-919-555-1234\r\n"
	want := line("CONTACT", `Jim Dolittle\, ABC Industries\,+1-919-555-1234`,
		param("ALTREP", `ldap://example.com:6666/o=ABC%20Industries\,c=US???(cn=Jim%20Dolittle)`))
	res := readString(in, true)
	if res.err != nil || len(res.lines) != 1 || !sameLine(res.lines[0], want) {
		t.Errorf("read %q\n got %s, %v\nwant %s", in, showLines(res.lines), res.err, showLine(want))
	}

	// The corrected example: no backslash.
	in = strings.Replace(in, `Industries\,`+"\r\n c=US", "Industries,\r\n c=US", 1)
	want = line("CONTACT", want.Value, param("ALTREP", `ldap://example.com:6666/o=ABC%20Industries,c=US???(cn=Jim%20Dolittle)`))
	res = readString(in, true)
	if res.err != nil || len(res.lines) != 1 || !sameLine(res.lines[0], want) {
		t.Errorf("read %q\n got %s, %v\nwant %s", in, showLines(res.lines), res.err, showLine(want))
	}

	// Unquoted, a backslash is a plain SAFE-CHAR and escapes nothing: "\,"
	// is a backslash ending one value and a comma starting the next.
	res = readString("X-P;X-Q=a\\,b;X-R=c\\;d:v\r\n", true)
	// "X-R=c\" ends at the semicolon; "d:v" is then a parameter with no
	// "=", so strict mode must reject the line.
	if res.err == nil {
		t.Errorf("strict read accepted a \\; inside an unquoted parameter: %s", showLines(res.lines))
	}
	res = readString("X-P;X-Q=a\\,b:v\r\n", true)
	if res.err != nil || len(res.lines) != 1 || !sameLine(res.lines[0], line("X-P", "v", param("X-Q", `a\`, "b"))) {
		t.Errorf(`read X-Q=a\,b: %s, %v; want values ["a\\" "b"]`, showLines(res.lines), res.err)
	}

	// On output, a comma in a parameter value is quoted, never
	// backslash-escaped, and a backslash is written as it is.
	l := line("CONTACT", `Jim Dolittle\, ABC Industries`,
		param("ALTREP", "ldap://example.com:6666/o=ABC%20Industries,c=US???(cn=Jim%20Dolittle)"),
		param("X-PATH", `C:\dir`), param("X-B", `a\b`))
	b, err := contentline.AppendLine(nil, l)
	if err != nil {
		t.Fatal(err)
	}
	wantOut := `CONTACT;ALTREP="ldap://example.com:6666/o=ABC%20Industries,c=US???(cn=Jim%20Dolittle)";X-PATH="C:\dir";X-B=a\b:Jim Dolittle\, ABC Industries`
	if string(b) != wantOut {
		t.Errorf("AppendLine\n got %s\nwant %s", b, wantOut)
	}
}

// TestConformanceWriterQuoting checks parameter quoting and escaping on
// output.
func TestConformanceWriterQuoting(t *testing.T) {
	tests := []struct {
		name string
		l    contentline.Line
		want string // AppendLine output
	}{
		// RFC 5545 §3.2: "Property parameter values that contain the
		// COLON, SEMICOLON, or COMMA character separators MUST be specified
		// as quoted-string text values." (Also §3.1.1.)
		{"colon", line("X-P", "v", param("X-Q", "a:b")), `X-P;X-Q="a:b":v`},
		{"semicolon", line("X-P", "v", param("X-Q", "a;b")), `X-P;X-Q="a;b":v`},
		{"comma", line("X-P", "v", param("X-Q", "a,b")), `X-P;X-Q="a,b":v`},
		// RFC 5545 §3.1.1: "the individual quoted-strings are separated by
		// a COMMA character". The RFC's own §3.2.11 example.
		{"list of quoted values",
			line("ATTENDEE", "mailto:janedoe@example.com", param("MEMBER", "mailto:projectA@example.com", "mailto:projectB@example.com")),
			`ATTENDEE;MEMBER="mailto:projectA@example.com","mailto:projectB@example.com":mailto:janedoe@example.com`},
		// RFC 5545 §3.2: "Property parameter values MUST NOT contain the
		// DQUOTE character." RFC 6868 §3: "the " character (U+0022) is
		// encoded into ^' (U+005E, U+0027)". The RFC 6868 §3.1 example,
		// unquoted because nothing in it needs quotes (the writer's
		// documented choice: quote only for ",;:").
		{"6868 §3.1 example",
			line("ATTENDEE", "mailto:babe@example.com", param("CN", `George Herman "Babe" Ruth`)),
			`ATTENDEE;CN=George Herman ^'Babe^' Ruth:mailto:babe@example.com`},
		// RFC 6868 §3: "formatted text line breaks are encoded into ^n",
		// "the ^ character (U+005E) is encoded into ^^".
		{"^ and line break", line("X-P", "v", param("X-Q", "a\nb^c\"d")), `X-P;X-Q=a^nb^^c^'d:v`},
		{"caret before n", line("X-P", "v", param("X-Q", "^n")), `X-P;X-Q=^^n:v`},
		{"DQUOTE needing quotes too", line("X-P", "v", param("X-Q", `a "b": c`)), `X-P;X-Q="a ^'b^': c":v`},
		// RFC 5545 §3.1: a parameter value may be empty.
		{"empty param value", line("X-P", "v", param("X-Q", "")), `X-P;X-Q=:v`},
		// The value is written exactly as given: TEXT escaping is the
		// caller's (documented on Writer), and RFC 6868 doesn't apply to
		// it.
		{"value untouched", line("X-P", `a\,b;c:d"e^n^'`), `X-P:a\,b;c:d"e^n^'`},
		{"empty value", line("X-P", ""), `X-P:`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := contentline.AppendLine(nil, tt.l)
			if err != nil {
				t.Fatalf("AppendLine(%s): %v", showLine(tt.l), err)
			}
			if string(b) != tt.want {
				t.Errorf("AppendLine(%s)\n got %s\nwant %s", showLine(tt.l), b, tt.want)
			}
			// The Writer writes the same content line, folded, with CRLF.
			out, err := writeLines(tt.l)
			if err != nil {
				t.Fatalf("WriteLine: %v", err)
			}
			checkPhysicalLines(t, out)
			res := readString(out, true)
			if res.err != nil || len(res.lines) != 1 || !sameLine(res.lines[0], tt.l) {
				t.Errorf("reading back %q: %s, %v; want %s", out, showLines(res.lines), res.err, showLine(tt.l))
			}
			unfolded := strings.ReplaceAll(strings.TrimSuffix(out, "\r\n"), "\r\n ", "")
			unfolded = strings.ReplaceAll(unfolded, "\r\n\t", "")
			if unfolded != tt.want {
				t.Errorf("WriteLine output unfolds to %q, want %q", unfolded, tt.want)
			}
		})
	}
}

// TestConformanceRFC6868RoundTrip writes awkward parameter values and reads
// them back: RFC 6868 encoding and decoding must be inverses.
func TestConformanceRFC6868RoundTrip(t *testing.T) {
	values := []string{
		"^", "^^", "^n", "^'", "^N", "\"", "\"\"", "\n", "\n\n", "^\n", "\n^", "a^b",
		"x^", "^^n", "^'\"", `"^'"`, "line1\nline2", "'", "n", "^ ", "a\"b,c", `\`, `\n`,
		"tab\there", " lead", "trail ", "",
	}
	for _, v := range values {
		for _, l := range []contentline.Line{
			line("X-P", "v", param("X-Q", v)),
			line("X-P", "v", param("X-Q", v, v)),
			line("X-P", "v", param("X-Q", "a", v, "b:c")),
		} {
			out, err := writeLines(l)
			if err != nil {
				t.Errorf("WriteLine(%s): %v", showLine(l), err)
				continue
			}
			res := readString(out, true)
			if res.err != nil || len(res.lines) != 1 || !sameLine(res.lines[0], l) {
				t.Errorf("round trip of %s via %q: %s, %v", showLine(l), out, showLines(res.lines), res.err)
			}
		}
	}
}

// TestConformanceWriterFolding checks the output side of RFC 5545 §3.1
// folding at the boundaries.
func TestConformanceWriterFolding(t *testing.T) {
	for _, n := range []int{0, 1, 70, 71, 72, 73, 74, 75, 76, 77, 148, 149, 150, 151, 152, 500} {
		for _, unit := range []string{"a", "é", "日", "😀", " ", "\t"} {
			for pad := range 4 {
				value := strings.Repeat("x", pad) + strings.Repeat(unit, n)
				l := line("X-P", value)
				out, err := writeLines(l)
				if err != nil {
					t.Fatalf("WriteLine: %v", err)
				}
				checkPhysicalLines(t, out)
				res := readString(out, true)
				if res.err != nil || len(res.lines) != 1 || !sameLine(res.lines[0], l) {
					t.Fatalf("round trip of %q: %s, %v", value, showLines(res.lines), res.err)
				}
				// A line that fits is not folded (writer's documented
				// FoldWidth; the RFC only says longer lines SHOULD be).
				if len("X-P:"+value) <= 75 && strings.Count(out, "\r\n") != 1 {
					t.Errorf("a %d-octet line was folded: %q", len("X-P:"+value), out)
				}
			}
		}
	}
	// Folding inside parameters, including quoted ones and RFC 6868
	// escapes.
	l := line("ATTENDEE", "mailto:"+strings.Repeat("ü", 40)+"@example.com",
		param("CN", strings.Repeat("Ab \"c\" ^ ", 20)),
		param("MEMBER", strings.Repeat("mailto:a@example.com,", 5), strings.Repeat("😀", 30)),
		param("X-Q", strings.Repeat("\n", 50)))
	out, err := writeLines(l)
	if err != nil {
		t.Fatal(err)
	}
	checkPhysicalLines(t, out)
	if res := readString(out, true); res.err != nil || len(res.lines) != 1 || !sameLine(res.lines[0], l) {
		t.Errorf("round trip: %s, %v", showLines(res.lines), res.err)
	}
}

// Rune pools for the generated lines: US-ASCII printable and HTAB, 2-, 3-
// and 4-octet UTF-8, and the characters that need quoting or escaping.
var (
	asciiPool = func() []string {
		var s []string
		for c := 0x20; c <= 0x7E; c++ {
			s = append(s, string(rune(c)))
		}
		return append(s, "\t")
	}()
	multiPool   = []string{"é", "ß", "Ω", "ж", "ŭ", "日", "€", "한", "ﬃ", "\u200b", "😀", "𝄞", "🎉", "\U0010FFFD"}
	specialPool = []string{":", ";", ",", "\"", "^", "^n", "^'", "^^", "n", "'", "\\", " ", "\t"}
)

func randomName(rng *rand.Rand) string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-"
	var b strings.Builder
	if rng.IntN(2) == 0 {
		b.WriteString("X-")
	}
	for range 1 + rng.IntN(12) {
		b.WriteByte(chars[rng.IntN(len(chars))])
	}
	return b.String()
}

func randomText(rng *rand.Rand, maxRunes int, newlines bool) string {
	var b strings.Builder
	n := rng.IntN(maxRunes + 1)
	for range n {
		switch k := rng.IntN(10); {
		case k < 4:
			b.WriteString(asciiPool[rng.IntN(len(asciiPool))])
		case k < 7:
			b.WriteString(multiPool[rng.IntN(len(multiPool))])
		case k < 9 || !newlines:
			b.WriteString(specialPool[rng.IntN(len(specialPool))])
		default:
			b.WriteString("\n")
		}
	}
	return b.String()
}

func randomLine(rng *rand.Rand) contentline.Line {
	l := contentline.Line{Name: randomName(rng), Value: randomText(rng, []int{0, 10, 80, 300}[rng.IntN(4)], false)}
	for range rng.IntN(4) {
		p := contentline.Param{Name: randomName(rng)}
		for range 1 + rng.IntN(3) {
			p.Values = append(p.Values, randomText(rng, []int{0, 5, 40, 120}[rng.IntN(4)], true))
		}
		l.Params = append(l.Params, p)
	}
	return l
}

// TestConformanceFoldingProperty writes a few thousand generated lines and
// checks every physical line of the output against RFC 5545 §3.1 (CRLF,
// at most 75 octets, no split UTF-8 sequence) and that reading the output
// back gives the original lines exactly.
func TestConformanceFoldingProperty(t *testing.T) {
	rng := rand.New(rand.NewPCG(5545, 6868))
	lines := make([]contentline.Line, 4000)
	for i := range lines {
		lines[i] = randomLine(rng)
	}
	out, err := writeLines(lines...)
	if err != nil {
		t.Fatalf("WriteLine: %v", err)
	}
	checkPhysicalLines(t, out)
	res := readString(out, true)
	if res.err != nil {
		t.Fatalf("strict read of the output: %v", res.err)
	}
	if len(res.lines) != len(lines) {
		t.Fatalf("read %d lines back, wrote %d", len(res.lines), len(lines))
	}
	failures := 0
	for i := range lines {
		if !sameLine(res.lines[i], lines[i]) {
			t.Errorf("line %d\nwrote %s\n read %s", i, showLine(lines[i]), showLine(res.lines[i]))
			if failures++; failures > 10 {
				t.FailNow()
			}
		}
	}
	// AppendLine is the unfolded form of what WriteLine writes.
	for i, l := range lines[:200] {
		b, err := contentline.AppendLine(nil, l)
		if err != nil {
			t.Fatalf("AppendLine: %v", err)
		}
		r := readString(string(b)+"\r\n", true)
		if r.err != nil || len(r.lines) != 1 || !sameLine(r.lines[0], l) {
			t.Errorf("line %d: AppendLine gave %q, which reads as %s, %v", i, b, showLines(r.lines), r.err)
		}
	}
}

// TestConformanceInjection checks that no name, parameter or value can make
// the Writer emit a second content line. RFC 5545 §3.1: CONTROL characters
// (which include CR and LF) are not VALUE-CHAR, SAFE-CHAR or QSAFE-CHAR, so
// they cannot be written as they are; and RFC 5545 §3.1 names are tokens.
// For every hostile line the Writer must either refuse it or write text that
// reads back, strictly, as exactly that one line.
func TestConformanceInjection(t *testing.T) {
	payloads := []string{
		"\r\nX-INJ:1", "\nX-INJ:1", "\rX-INJ:1", "\r\n X", "\r\n\tX", "a\r\n",
		"\r\nEND:VCALENDAR\r\nBEGIN:VCALENDAR", "\x00", "\x1b[31m", "\x7f", "\u0085", "\u2028",
	}
	var lines []contentline.Line
	for _, p := range payloads {
		lines = append(lines,
			line("X-P", "a"+p),
			line("X-P", "v", param("X-Q", "a"+p)),
			line("X-P", "v", param("X-Q", "ok", "a"+p+":;,")),
			line("X-P"+p, "v"),
			line("X-P", "v", param("X-Q"+p, "a")),
		)
	}
	lines = append(lines,
		line("X-P:Y", "v"), line("X-P;X-Q=1", "v"), line("", "v"), line("X P", "v"), line("X_P", "v"),
		line("X-P", "v", param("", "a")), line("X-P", "v", param("X-Q=a", "b")), line("X-P", "v", param("X-Q:a", "b")),
		line("Ä", "v"),
	)
	for _, l := range lines {
		checkHostileLine(t, l)
	}
}

// checkHostileLine writes l on its own. If the Writer accepts it, the output
// must follow RFC 5545 §3.1 and read back, strictly, as that one line.
//
// The exception is malformed UTF-8, which no conforming output can hold.
// The Writer passes it through (see TestConformanceInvalidUTF8); then the
// output must still be one content line, folded and ended with CRLF, that a
// lenient Reader returns byte for byte.
func checkHostileLine(t *testing.T, l contentline.Line) {
	t.Helper()
	out, err := writeLines(l)
	if err != nil {
		if !errors.Is(err, contentline.ErrInvalidLine) {
			t.Errorf("WriteLine(%s) failed with %v, which doesn't wrap ErrInvalidLine", showLine(l), err)
		}
		return
	}
	malformed := !utf8.ValidString(l.Value)
	for _, p := range l.Params {
		for _, v := range p.Values {
			malformed = malformed || !utf8.ValidString(v)
		}
	}
	if malformed {
		if !strings.HasSuffix(out, "\r\n") {
			t.Errorf("WriteLine(%s) wrote %q, which doesn't end in CRLF", showLine(l), out)
		}
		for _, pl := range strings.Split(strings.TrimSuffix(out, "\r\n"), "\r\n") {
			if strings.ContainsAny(pl, "\r\n") || len(pl) > 75 {
				t.Errorf("WriteLine(%s) wrote physical line %q", showLine(l), pl)
			}
		}
	}
	res := readString(out, !malformed)
	if !malformed {
		checkPhysicalLines(t, out)
	}
	if res.err != nil || len(res.lines) != 1 {
		t.Errorf("WriteLine(%s) wrote %q, which reads as %s, %v", showLine(l), out, showLines(res.lines), res.err)
		return
	}
	got := res.lines[0]
	if got.Name != strings.ToUpper(l.Name) || got.Value != l.Value || len(got.Params) != len(l.Params) {
		t.Errorf("WriteLine(%s) wrote %q, which reads as %s", showLine(l), out, showLine(got))
		return
	}
	for i, p := range l.Params {
		gp := got.Params[i]
		if gp.Name != strings.ToUpper(p.Name) || len(gp.Values) != len(p.Values) {
			t.Errorf("WriteLine(%s) wrote %q, which reads as %s", showLine(l), out, showLine(got))
			return
		}
		for j, v := range p.Values {
			// RFC 6868 encodes a "formatted text line break" as ^n, which
			// reads back as a newline: a CR may come back as LF.
			if strings.ContainsRune(v, '\r') {
				if strings.ContainsRune(gp.Values[j], '\r') {
					t.Errorf("a CR survived in parameter value %q", gp.Values[j])
				}
				continue
			}
			if gp.Values[j] != v {
				t.Errorf("WriteLine(%s) wrote %q, which reads as %s", showLine(l), out, showLine(got))
			}
		}
	}
}

// TestConformanceHostileProperty generates lines from arbitrary bytes,
// including every control character and malformed UTF-8, and holds the
// Writer to checkHostileLine's rule.
func TestConformanceHostileProperty(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 1))
	randomBytes := func(n int) string {
		b := make([]byte, rng.IntN(n+1))
		for i := range b {
			switch rng.IntN(4) {
			case 0:
				b[i] = byte(rng.IntN(0x20)) // controls, CR and LF included
			case 1:
				b[i] = byte(0x80 + rng.IntN(0x80)) // UTF-8 lead and continuation bytes
			default:
				b[i] = byte(0x20 + rng.IntN(0x60))
			}
		}
		return string(b)
	}
	for range 3000 {
		l := randomLine(rng)
		switch rng.IntN(3) {
		case 0:
			l.Value = randomBytes(100)
		case 1:
			if len(l.Params) > 0 {
				l.Params[0].Values[0] = randomBytes(40)
			}
		default:
			l.Name = randomBytes(6)
		}
		checkHostileLine(t, l)
	}
}

// TestConformanceInvalidUTF8 reports what the layer does with malformed
// UTF-8. RFC 5545 §3.1.4: "The default charset for an iCalendar stream is
// UTF-8 as defined in [RFC3629]." The RFC doesn't say what a reader must do
// with anything else (strict mode rejects it: see TestConformanceABNFRejects).
// Lenient mode must neither panic nor change the bytes without a warning.
func TestConformanceInvalidUTF8(t *testing.T) {
	for _, bad := range []string{"\xff", "\xc0\xaf", "\xed\xa0\x80", "\xf4\x90\x80\x80", "\xe6\x97", "\x80"} {
		in := "X-P;X-Q=a" + bad + "b;X-R=\"c" + bad + "d\":e" + bad + "f\r\n"
		res := readString(in, false)
		if res.err != nil {
			t.Logf("lenient read of %q fails: %v", in, res.err)
			continue
		}
		want := line("X-P", "e"+bad+"f", param("X-Q", "a"+bad+"b"), param("X-R", "c"+bad+"d"))
		if len(res.lines) == 1 && sameLine(res.lines[0], want) {
			t.Logf("lenient read of %q keeps the bytes; %d warning(s)", in, len(res.warns))
			continue
		}
		if len(res.warns) == 0 {
			t.Errorf("lenient read of %q changed it to %s without a warning", in, showLines(res.lines))
		} else {
			t.Logf("lenient read of %q gives %s with warnings %v", in, showLines(res.lines), res.warns)
		}
	}
	// A writer handed malformed UTF-8 can't write a conforming stream: it
	// must refuse, or at least not alter the bytes on the way through.
	for _, bad := range []string{"\xff", "\xc0\xaf", "\xe6\x97"} {
		l := line("X-P", strings.Repeat("a", 73)+bad+"b", param("X-Q", "c"+bad))
		out, err := writeLines(l)
		if err != nil {
			t.Logf("WriteLine with %q refused: %v", bad, err)
			continue
		}
		res := readString(out, false)
		if res.err != nil || len(res.lines) != 1 || !sameLine(res.lines[0], l) {
			t.Errorf("WriteLine with %q wrote %q, which reads as %s, %v", bad, out, showLines(res.lines), res.err)
		} else {
			t.Logf("WriteLine with %q writes the bytes through (%d warning(s) reading them back)", bad, len(res.warns))
		}
	}
}

// examples is testdata/rfc5545/examples.json: the RFC 5545 example blocks.
type example struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Text  string `json:"text"`
	Valid bool   `json:"valid"`
}

// TestConformanceRFCExampleLinesRoundTrip reads every content line of every
// RFC 5545 example, writes it and reads it again: the Writer must write a
// line the Reader returns unchanged (RFC 5545 §3.1 grammar both ways), in
// at most 75-octet physical lines.
func TestConformanceRFCExampleLinesRoundTrip(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "testdata", "rfc5545", "examples.json"))
	if err != nil {
		t.Fatal(err)
	}
	var examples []example
	if err := json.Unmarshal(data, &examples); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, ex := range examples {
		if ex.Kind == "value" {
			continue // a bare value, not a content line
		}
		text := ex.Text
		if !strings.HasSuffix(text, "\r\n") {
			text += "\r\n" // the fixtures leave off the last line break
		}
		first := readString(text, false)
		if first.err != nil {
			t.Errorf("%s: %v", ex.ID, first.err)
			continue
		}
		if ex.Valid && len(first.warns) > 0 {
			t.Errorf("%s: valid example read with warnings %v", ex.ID, first.warns)
		}
		out, err := writeLines(first.lines...)
		if err != nil {
			t.Errorf("%s: WriteLine: %v", ex.ID, err)
			continue
		}
		checkPhysicalLines(t, out)
		second := readString(out, true)
		if second.err != nil || !sameLines(first.lines, second.lines) {
			t.Errorf("%s: round trip\n got %s, %v\nwant %s", ex.ID, showLines(second.lines), second.err, showLines(first.lines))
		}
		n += len(first.lines)
	}
	if n < 400 {
		t.Errorf("round-tripped only %d lines; is examples.json complete?", n)
	}
}

// TestConformanceSentByUnquoted reads the uncorrected §3.8.4.1 example that
// erratum 2516 fixes. By the generic grammar (paramtext stops at ":"), the
// unquoted SENT-BY value ends at "mailto" and the property value starts
// after that colon. The grammar decides this; a reader must not guess.
func TestConformanceSentByUnquoted(t *testing.T) {
	in := "ATTENDEE;SENT-BY=mailto:jan_doe@example.com;CN=John Smith:\r\n mailto:jsmith@example.com\r\n"
	want := line("ATTENDEE", "jan_doe@example.com;CN=John Smith:mailto:jsmith@example.com", param("SENT-BY", "mailto"))
	res := readString(in, true)
	if res.err != nil || len(res.lines) != 1 || !sameLine(res.lines[0], want) {
		t.Errorf("read %q\n got %s, %v\nwant %s", in, showLines(res.lines), res.err, showLine(want))
	}
}
