package contentline

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

func readAll(t *testing.T, r *Reader) []Line {
	t.Helper()
	var lines []Line
	for {
		l, err := r.ReadLine()
		if err == io.EOF {
			return lines
		}
		if err != nil {
			t.Fatalf("ReadLine: %v", err)
		}
		lines = append(lines, l)
	}
}

func TestReadBasic(t *testing.T) {
	in := "BEGIN:VCALENDAR\r\n" +
		"dtstart;tzid=America/New_York:19970902T090000\r\n" +
		"ATTENDEE;ROLE=REQ-PARTICIPANT;DELEGATED-FROM=\"mailto:a@example.com\",\"mailto:b@example.com\";CN=\"Doe, Jane\":mailto:jd@example.com\r\n" +
		"DESCRIPTION:This is a lo\r\n" +
		" ng description\r\n" +
		"\tthat exists on a long line.\r\n" +
		"SUMMARY:Dinner\\, drinks\r\n" +
		"X-EMPTY:\r\n" +
		"END:VCALENDAR" // no final line ending
	got := readAll(t, NewReader(strings.NewReader(in)))
	want := []Line{
		{Name: "BEGIN", Value: "VCALENDAR", Pos: 1},
		{Name: "DTSTART", Params: []Param{{"TZID", []string{"America/New_York"}}}, Value: "19970902T090000", Pos: 2},
		{Name: "ATTENDEE", Params: []Param{
			{"ROLE", []string{"REQ-PARTICIPANT"}},
			{"DELEGATED-FROM", []string{"mailto:a@example.com", "mailto:b@example.com"}},
			{"CN", []string{"Doe, Jane"}},
		}, Value: "mailto:jd@example.com", Pos: 3},
		{Name: "DESCRIPTION", Value: "This is a long descriptionthat exists on a long line.", Pos: 4},
		{Name: "SUMMARY", Value: `Dinner\, drinks`, Pos: 7},
		{Name: "X-EMPTY", Value: "", Pos: 8},
		{Name: "END", Value: "VCALENDAR", Pos: 9},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got\n%#v\nwant\n%#v", got, want)
	}
}

func TestLineEndings(t *testing.T) {
	for name, in := range map[string]string{
		"crlf":  "A:1\r\nB:2\r\n continued\r\nC:3\r\n",
		"lf":    "A:1\nB:2\n continued\nC:3\n",
		"cr":    "A:1\rB:2\r continued\rC:3\r",
		"mixed": "A:1\r\nB:2\n continued\rC:3",
		"bom":   "\xef\xbb\xbfA:1\r\nB:2\r\n continued\r\nC:3\r\n",
	} {
		t.Run(name, func(t *testing.T) {
			// One byte at a time exercises every buffer boundary, including
			// a CR whose LF hasn't been read yet.
			got := readAll(t, NewReader(iotest.OneByteReader(strings.NewReader(in))))
			var s []string
			for _, l := range got {
				s = append(s, l.Name+"="+l.Value)
			}
			if want := []string{"A=1", "B=2continued", "C=3"}; !reflect.DeepEqual(s, want) {
				t.Errorf("got %q, want %q", s, want)
			}
		})
	}
}

func TestFoldInsideUTF8(t *testing.T) {
	// A producer folded in the middle of "é" (C3 A9). Unfolding works on
	// bytes, so the character comes back whole.
	got := readAll(t, NewReader(strings.NewReader("SUMMARY:caf\xc3\r\n \xa9\r\n")))
	if len(got) != 1 || got[0].Value != "café" {
		t.Errorf("got %+v", got)
	}
}

func TestRFC6868(t *testing.T) {
	in := "ATTENDEE;CN=George Herman ^'Babe^' Ruth;X-ADDR=Street^nTown;X-CARET=a^^b^c:mailto:x@example.com\r\n"
	l := readAll(t, NewReader(strings.NewReader(in)))[0]
	if v, _ := l.Param("cn"); v != `George Herman "Babe" Ruth` {
		t.Errorf("CN = %q", v)
	}
	if v, _ := l.Param("X-ADDR"); v != "Street\nTown" {
		t.Errorf("X-ADDR = %q", v)
	}
	if v, _ := l.Param("X-CARET"); v != "a^b^c" {
		t.Errorf("X-CARET = %q", v)
	}
	b, err := AppendLine(nil, l)
	if err != nil {
		t.Fatal(err)
	}
	if want := "ATTENDEE;CN=George Herman ^'Babe^' Ruth;X-ADDR=Street^nTown;X-CARET=a^^b^^c:mailto:x@example.com"; string(b) != want {
		t.Errorf("wrote %q, want %q", b, want)
	}
}

// Each deviation is recovered from with a warning by default and is an
// error in strict mode.
func TestDeviations(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want []string // NAME=VALUE of each line read in lenient mode
	}{
		{"empty line", "A:1\r\n\r\nB:2\r\n", []string{"A=1", "B=2"}},
		{"leading continuation", " A:1\r\nB:2\r\n", []string{"A=1", "B=2"}},
		{"bare parameter", "A;QUOTED-PRINTABLE:1\r\n", []string{"A=1"}},
		{"text after quote", "LOCATION;ALTREP=\"http://x\"y:Room\r\n", []string{"LOCATION=Room"}},
		{"quote inside value", "A;X=a\"b:1\r\n", []string{"A=1"}},
		{"control in param", "A;X=a\x01b:1\r\n", []string{"A=1"}},
		// Unparseable lines are skipped entirely.
		{"truncated parameters", "A:1\r\nDTSTART;TZID=America/New_York\r\nB:2\r\n", []string{"A=1", "B=2"}},
		{"unterminated quote", "A;X=\"abc:1\r\nB:2\r\n", []string{"B=2"}},
		{"space in name", "DT START:1\r\nB:2\r\n", []string{"B=2"}},
		{"no name", ":1\r\nB:2\r\n", []string{"B=2"}},
		{"empty param name", "A;=x:1\r\nB:2\r\n", []string{"B=2"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var warnings int
			r := NewReader(strings.NewReader(tc.in))
			r.Warn = func(err error) {
				var se *SyntaxError
				if !errors.As(err, &se) {
					t.Errorf("warning %v is not a *SyntaxError", err)
				}
				warnings++
			}
			var got []string
			for _, l := range readAll(t, r) {
				got = append(got, l.Name+"="+l.Value)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("lenient read %q, want %q", got, tc.want)
			}
			if warnings == 0 {
				t.Error("no warning")
			}

			r = NewReader(strings.NewReader(tc.in))
			r.Strict = true
			var err error
			for err == nil {
				_, err = r.ReadLine()
			}
			var se *SyntaxError
			if !errors.As(err, &se) {
				t.Errorf("strict read ended with %v, want a *SyntaxError", err)
			}
		})
	}
}

func TestStrictValue(t *testing.T) {
	for _, in := range []string{"A:a\x01b\r\n", "A:\xff\r\n"} {
		r := NewReader(strings.NewReader(in))
		if _, err := r.ReadLine(); err != nil {
			t.Errorf("lenient %q: %v", in, err)
		}
		r = NewReader(strings.NewReader(in))
		r.Strict = true
		if _, err := r.ReadLine(); err == nil {
			t.Errorf("strict %q: no error", in)
		}
	}
}

func TestLineTooLong(t *testing.T) {
	long := "A:" + strings.Repeat("x", 100) + "\r\n"
	folded := "A:" + strings.Repeat("x\r\n ", 100) + "\r\n"
	for _, in := range []string{long, folded} {
		r := NewReader(strings.NewReader(in))
		r.MaxLineLength = 50
		if _, err := r.ReadLine(); !errors.Is(err, ErrLineTooLong) {
			t.Errorf("err = %v, want ErrLineTooLong", err)
		}
	}
	r := NewReader(strings.NewReader(long))
	r.MaxLineLength = 102
	if _, err := r.ReadLine(); err != nil {
		t.Errorf("a line at the limit: %v", err)
	}
}

func TestWriteFolds(t *testing.T) {
	value := strings.Repeat("ab€", 60) // a 3-byte character at every third position
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if err := w.WriteLine(Line{Name: "description", Value: value}); err != nil {
		t.Fatal(err)
	}
	w.Flush()
	out := buf.String()
	if !strings.HasSuffix(out, "\r\n") {
		t.Fatal("no final CRLF")
	}
	for i, phys := range strings.Split(strings.TrimSuffix(out, "\r\n"), "\r\n") {
		if len(phys) > FoldWidth {
			t.Errorf("physical line %d is %d octets", i, len(phys))
		}
		if i > 0 && phys[0] != ' ' {
			t.Errorf("continuation %d doesn't start with a space", i)
		}
		if !strings.ContainsRune(phys, '€') && len(phys) > 2 {
			t.Errorf("physical line %d looks split mid-character: %q", i, phys)
		}
	}
	got := readAll(t, NewReader(strings.NewReader(out)))
	if len(got) != 1 || got[0].Name != "DESCRIPTION" || got[0].Value != value {
		t.Errorf("read back %+v", got)
	}
}

func TestWriteRejects(t *testing.T) {
	for _, l := range []Line{
		{Name: "", Value: "x"},
		{Name: "A B", Value: "x"},
		{Name: "SUMMARY", Value: "line one\r\nBEGIN:VEVENT"},
		{Name: "SUMMARY", Value: "bell\x07"},
		{Name: "SUMMARY", Params: []Param{{Name: "X_Y", Values: []string{"v"}}}},
		{Name: "SUMMARY", Params: []Param{{Name: "X", Values: []string{"a\x00b"}}}},
	} {
		w := NewWriter(io.Discard)
		if err := w.WriteLine(l); !errors.Is(err, ErrInvalidLine) {
			t.Errorf("WriteLine(%+v) = %v, want ErrInvalidLine", l, err)
		}
	}
}

func TestWriteQuotes(t *testing.T) {
	b, err := AppendLine(nil, Line{Name: "ATTENDEE", Params: []Param{
		{Name: "CN", Values: []string{"Doe, Jane"}},
		{Name: "DELEGATED-FROM", Values: []string{"mailto:a@x", "mailto:b@x"}},
		{Name: "X-EMPTY", Values: []string{""}},
	}, Value: "mailto:jd@x"})
	if err != nil {
		t.Fatal(err)
	}
	want := `ATTENDEE;CN="Doe, Jane";DELEGATED-FROM="mailto:a@x","mailto:b@x";X-EMPTY=:mailto:jd@x`
	if string(b) != want {
		t.Errorf("got  %s\nwant %s", b, want)
	}
}

// The inputs that made other Go readers quadratic must take linear time:
// one huge parameter value, and a line made of tiny parameters.
func TestHostileInputsAreLinear(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	cases := map[string]func(n int) string{
		"long param value": func(n int) string { return "X;A=" + strings.Repeat("b", n) + ":v\r\n" },
		"many params":      func(n int) string { return "X" + strings.Repeat(";A=b", n/4) + ":v\r\n" },
		"many values":      func(n int) string { return "X;A=" + strings.Repeat("b,", n/2) + "b:v\r\n" },
		"long folded":      func(n int) string { return "X:" + strings.Repeat("abcdefgh\r\n ", n/8) + "\r\n" },
	}
	for name, gen := range cases {
		t.Run(name, func(t *testing.T) {
			measure := func(n int) time.Duration {
				in := gen(n)
				start := time.Now()
				r := NewReader(strings.NewReader(in))
				r.MaxLineLength = 16 << 20
				if _, err := r.ReadLine(); err != nil {
					t.Fatal(err)
				}
				return time.Since(start)
			}
			measure(1 << 16) // warm up
			small, big := measure(1<<20), measure(4<<20)
			// Linear predicts 4x; quadratic would be 16x. Allow noise.
			if big > 10*small+20*time.Millisecond {
				t.Errorf("1 MiB took %v, 4 MiB took %v: not linear", small, big)
			}
		})
	}
}

// indexControl, which skips eight bytes at a time, finds what a byte loop
// finds: every byte value at every position, on its own, after an HTAB, and
// beside high bytes.
func TestIndexControl(t *testing.T) {
	naive := func(b []byte) int {
		for i, c := range b {
			if isControl(c) {
				return i
			}
		}
		return -1
	}
	for n := 1; n <= 24; n++ {
		for _, fill := range []byte{'a', 0x80, 0xff, '\t'} {
			for at := range n {
				for c := range 256 {
					b := bytes.Repeat([]byte{fill}, n)
					b[at] = byte(c)
					if got, want := indexControl(b), naive(b); got != want {
						t.Fatalf("indexControl(%q) = %d, want %d", b, got, want)
					}
					if at+1 < n {
						b[n-1] = 0x01
						if got, want := indexControl(b), naive(b); got != want {
							t.Fatalf("indexControl(%q) = %d, want %d", b, got, want)
						}
					}
				}
			}
		}
	}
}

func FuzzReader(f *testing.F) {
	for _, s := range []string{
		"BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n",
		"A;B=\"c,d\";E=f,g:h\r\n i\r\n",
		"ATTENDEE;CN=^'x^':mailto:a\n",
		"\xef\xbb\xbfA:1\rB:2",
		"A;B\r\n:c",
		"LOCATION;ALTREP=\"http://x\"y:Room",
	} {
		f.Add(s, false)
	}
	f.Fuzz(func(t *testing.T, in string, strict bool) {
		r := NewReader(strings.NewReader(in))
		r.Strict = strict
		r.MaxLineLength = 4096
		for range 10_000 {
			l, err := r.ReadLine()
			if err != nil {
				return
			}
			// Whatever was read writes back and reads back identically,
			// unless the value holds a control character the Writer refuses.
			b, err := AppendLine(nil, l)
			if err != nil {
				if !errors.Is(err, ErrInvalidLine) {
					t.Fatalf("AppendLine: %v", err)
				}
				continue
			}
			var buf bytes.Buffer
			w := NewWriter(&buf)
			if err := w.WriteLine(l); err != nil {
				t.Fatalf("WriteLine after AppendLine succeeded: %v", err)
			}
			w.Flush()
			back, err := NewReader(&buf).ReadLine()
			if err != nil {
				t.Fatalf("reading back %q: %v", b, err)
			}
			back.Pos = l.Pos
			if !reflect.DeepEqual(normalize(back), normalize(l)) {
				t.Fatalf("round trip of %q:\n got %#v\nwant %#v", b, back, l)
			}
		}
	})
}

// normalize maps the differences a round trip may legitimately introduce:
// a CRLF in a parameter value comes back as LF, and a valueless parameter
// comes back with one empty value.
func normalize(l Line) Line {
	params := make([]Param, len(l.Params))
	for i, p := range l.Params {
		vs := make([]string, 0, len(p.Values))
		for _, v := range p.Values {
			vs = append(vs, strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(v))
		}
		if len(vs) == 0 {
			vs = append(vs, "")
		}
		params[i] = Param{Name: p.Name, Values: vs}
	}
	l.Params = params
	return l
}

func BenchmarkReader(b *testing.B) {
	var sb strings.Builder
	sb.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//bench//EN\r\n")
	for range 1000 {
		sb.WriteString("BEGIN:VEVENT\r\nUID:1234567890@example.com\r\nDTSTAMP:20240101T000000Z\r\n" +
			"DTSTART;TZID=America/New_York:20240101T090000\r\nDTEND;TZID=America/New_York:20240101T100000\r\n" +
			"SUMMARY:Weekly sync\\, with notes\r\nRRULE:FREQ=WEEKLY;BYDAY=MO,WE\r\n" +
			"DESCRIPTION:" + strings.Repeat("Lorem ipsum dolor sit amet ", 8) + "\r\n" +
			"ATTENDEE;CN=\"Doe, Jane\";ROLE=REQ-PARTICIPANT:mailto:jane@example.com\r\nEND:VEVENT\r\n")
	}
	sb.WriteString("END:VCALENDAR\r\n")
	var folded bytes.Buffer
	w := NewWriter(&folded)
	for _, l := range readAll(&testing.T{}, NewReader(strings.NewReader(sb.String()))) {
		w.WriteLine(l)
	}
	w.Flush()
	in := folded.Bytes()
	b.SetBytes(int64(len(in)))
	b.ReportAllocs()
	for b.Loop() {
		r := NewReader(bytes.NewReader(in))
		for {
			if _, err := r.ReadLine(); err != nil {
				break
			}
		}
	}
}
