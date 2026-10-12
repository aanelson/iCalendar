package contentline

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// ErrInvalidLine is wrapped by the error a Writer returns for a Line it
// can't write as one content line: a bad name, or a control character that
// would end the line early.
var ErrInvalidLine = errors.New("contentline: invalid line")

// FoldWidth is the longest physical line a Writer writes, in octets,
// excluding the CRLF (RFC 5545 §3.1).
const FoldWidth = 75

// A Writer writes content lines, folding each at FoldWidth octets without
// splitting a UTF-8 sequence and ending every physical line with CRLF.
//
// Parameter values are quoted when they contain ',', ';' or ':', and
// carets, double quotes and newlines in them are written as RFC 6868
// escapes. A Line's Value is written exactly as given; it must already be
// escaped for its value type, and may not contain a control character other
// than horizontal tab, so no value can start a line of its own.
type Writer struct {
	bw   *bufio.Writer
	line []byte
}

// NewWriter returns a Writer writing to w. Call Flush when done.
func NewWriter(w io.Writer) *Writer {
	return &Writer{bw: bufio.NewWriter(w)}
}

// WriteLine writes l, folded. Its Pos is ignored.
func (w *Writer) WriteLine(l Line) error {
	b, err := appendLine(w.line[:0], l)
	if err != nil {
		return err
	}
	w.line = b
	w.writeFolded(b)
	return nil
}

// Flush writes any buffered data to the underlying writer and returns the
// first error any write met.
func (w *Writer) Flush() error {
	return w.bw.Flush()
}

// AppendLine appends l to b, unfolded and without a line ending: the text
// a Reader would return l from.
func AppendLine(b []byte, l Line) ([]byte, error) {
	return appendLine(b, l)
}

func appendLine(b []byte, l Line) ([]byte, error) {
	if err := checkName(l.Name); err != nil {
		return b, err
	}
	b = appendUpper(b, l.Name)
	for _, p := range l.Params {
		if err := checkName(p.Name); err != nil {
			return b, fmt.Errorf("%w (parameter of %s)", err, l.Name)
		}
		b = append(b, ';')
		b = appendUpper(b, p.Name)
		b = append(b, '=')
		for i, v := range p.Values {
			if i > 0 {
				b = append(b, ',')
			}
			var err error
			if b, err = appendParamValue(b, v); err != nil {
				return b, fmt.Errorf("%w: %s;%s", err, l.Name, p.Name)
			}
		}
	}
	b = append(b, ':')
	for i := 0; i < len(l.Value); i++ {
		if isControl(l.Value[i]) {
			return b, fmt.Errorf("%w: %s: control character %#02x in the value", ErrInvalidLine, l.Name, l.Value[i])
		}
	}
	return append(b, l.Value...), nil
}

func checkName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: empty name", ErrInvalidLine)
	}
	for i := 0; i < len(name); i++ {
		if !isNameByte(name[i]) {
			return fmt.Errorf("%w: name %q has %q", ErrInvalidLine, name, name[i])
		}
	}
	return nil
}

func appendUpper(b []byte, s string) []byte {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if 'a' <= c && c <= 'z' {
			c -= 'a' - 'A'
		}
		b = append(b, c)
	}
	return b
}

// appendParamValue appends v as a param-value, RFC 6868-escaped and quoted
// when it needs to be.
func appendParamValue(b []byte, v string) ([]byte, error) {
	quote := strings.ContainsAny(v, ",;:")
	if quote {
		b = append(b, '"')
	}
	for i := 0; i < len(v); i++ {
		switch c := v[i]; c {
		case '^':
			b = append(b, '^', '^')
		case '"':
			b = append(b, '^', '\'')
		case '\r':
			if i+1 < len(v) && v[i+1] == '\n' {
				i++
			}
			b = append(b, '^', 'n')
		case '\n':
			b = append(b, '^', 'n')
		default:
			if isControl(c) {
				return b, fmt.Errorf("%w: control character %#02x in a parameter value", ErrInvalidLine, c)
			}
			b = append(b, c)
		}
	}
	if quote {
		b = append(b, '"')
	}
	return b, nil
}

// writeFolded writes line as physical lines of at most FoldWidth octets,
// each continuation starting with a space, breaking only at the start of a
// UTF-8 sequence.
func (w *Writer) writeFolded(line []byte) {
	room := FoldWidth
	for len(line) > room {
		n := room
		for n > 0 && !utf8.RuneStart(line[n]) {
			n--
		}
		if n == 0 { // not UTF-8 at all; cut where the width says
			n = room
		}
		w.bw.Write(line[:n])
		w.bw.WriteString("\r\n ")
		line = line[n:]
		room = FoldWidth - 1 // the leading space counts
	}
	w.bw.Write(line)
	w.bw.WriteString("\r\n")
}
