package contentline

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"
)

// DefaultMaxLineLength is the longest unfolded content line a Reader accepts
// when its MaxLineLength is zero: 1 MiB.
const DefaultMaxLineLength = 1 << 20

// ErrLineTooLong is wrapped by the error a Reader returns for a content line
// longer than its MaxLineLength. It is never recovered from: the rest of the
// input can't be trusted to line up.
var ErrLineTooLong = errors.New("contentline: line too long")

// A Reader reads content lines from an input, unfolding them.
//
// It accepts CRLF, LF and lone CR line endings (only CRLF in Strict mode),
// and continuation lines that start with a space or a horizontal tab (RFC
// 5545 §3.1). A UTF-8 byte-order mark at the very start is skipped.
//
// By default the Reader recovers from the mistakes real producers make, and
// reports each one to Warn: a line it can't parse at all is skipped, and
// smaller deviations (a quote inside an unquoted parameter value, a
// parameter with no "=") are read the way most readers read them. Strict
// turns every one of these into an error.
type Reader struct {
	// MaxLineLength is the longest unfolded content line accepted, in
	// bytes. Zero means DefaultMaxLineLength.
	MaxLineLength int
	// Strict makes every deviation from RFC 5545 §3.1 an error.
	Strict bool
	// Warn, if not nil, receives a *SyntaxError for each deviation the
	// Reader recovers from. It is not called in Strict mode.
	Warn func(error)

	src     io.Reader
	srcErr  error  // sticky: the error src last returned
	buf     []byte // buffered input, buf[r:w] unread
	r, w    int
	pos     int    // physical lines consumed
	acc     []byte // the logical line being unfolded
	scratch []byte // upper-casing names
	started bool
}

// NewReader returns a Reader reading from src.
func NewReader(src io.Reader) *Reader {
	return &Reader{src: src}
}

// ReadLine returns the next content line. It returns io.EOF, unwrapped,
// once the input is exhausted. Whether the input ended where it should is
// the caller's question: the Reader knows nothing of BEGIN and END.
func (r *Reader) ReadLine() (Line, error) {
	for {
		raw, pos, err := r.logical()
		if err != nil {
			return Line{}, err
		}
		if len(bytes.TrimSpace(raw)) == 0 {
			if err := r.deviate(pos, "empty line"); err != nil {
				return Line{}, err
			}
			continue
		}
		l, err := r.parse(raw, pos)
		if err != nil {
			var se *SyntaxError
			if r.Strict || !errors.As(err, &se) {
				return Line{}, err
			}
			r.warn(err) // the line is skipped
			continue
		}
		return l, nil
	}
}

// Line returns the number of physical lines read so far: the line the
// last content line returned ended on.
func (r *Reader) Line() int { return r.pos }

func (r *Reader) maxLine() int {
	if r.MaxLineLength > 0 {
		return r.MaxLineLength
	}
	return DefaultMaxLineLength
}

// deviate reports a deviation at physical line pos: an error in Strict mode,
// and a warning (nil error) otherwise.
func (r *Reader) deviate(pos int, format string, args ...any) error {
	err := &SyntaxError{Line: pos, Msg: fmt.Sprintf(format, args...)}
	if r.Strict {
		return err
	}
	r.warn(err)
	return nil
}

func (r *Reader) warn(err error) {
	if r.Warn != nil {
		r.Warn(err)
	}
}

// logical returns the next unfolded content line and the physical line it
// started on. The slice is valid until the next call.
func (r *Reader) logical() ([]byte, int, error) {
	if !r.started {
		r.started = true
		if err := r.skipBOM(); err != nil {
			return nil, 0, err
		}
	}
	first, err := r.physical()
	if err != nil {
		return nil, 0, err
	}
	pos := r.pos
	if len(first) > 0 && (first[0] == ' ' || first[0] == '\t') {
		// A continuation with nothing to continue: the first line of the
		// input, or one after a skipped line.
		if err := r.deviate(pos, "continuation line with no line before it"); err != nil {
			return nil, 0, err
		}
		first = first[1:]
	}
	r.acc = append(r.acc[:0], first...)
	for {
		c, ok := r.peek()
		if !ok || (c != ' ' && c != '\t') {
			break
		}
		r.r++ // the fold's whitespace is not content
		cont, err := r.physical()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, 0, err
		}
		if len(r.acc)+len(cont) > r.maxLine() {
			return nil, 0, r.tooLong(pos)
		}
		r.acc = append(r.acc, cont...)
	}
	return r.acc, pos, nil
}

// physical returns the next physical line without its line ending, or
// io.EOF when the input is exhausted. The slice is valid until the next
// read from the buffer.
func (r *Reader) physical() ([]byte, error) {
	scanned := 0 // bytes of buf[r:w] already searched for a line ending
	for {
		if i := indexLineEnd(r.buf[r.r+scanned : r.w]); i >= 0 {
			end := r.r + scanned + i
			if r.buf[end] == '\r' && end+1 == r.w && r.srcErr == nil {
				// A CR at the end of the buffer: whether it is CRLF or a lone
				// CR depends on the next byte.
				scanned += i
				r.fill()
				continue
			}
			line := r.buf[r.r:end]
			if len(line) > r.maxLine() {
				return nil, r.tooLong(r.pos + 1)
			}
			next := end + 1
			if r.buf[end] == '\r' && next < r.w && r.buf[next] == '\n' {
				next++
			} else if r.Strict {
				return nil, &SyntaxError{Line: r.pos + 1, Msg: "line ends in a bare CR or LF, not CRLF"}
			}
			r.r = next
			r.pos++
			return line, nil
		}
		scanned = r.w - r.r
		if r.srcErr != nil {
			if r.w > r.r {
				line := r.buf[r.r:r.w]
				if len(line) > r.maxLine() {
					return nil, r.tooLong(r.pos + 1)
				}
				r.r = r.w
				r.pos++
				return line, nil
			}
			return nil, r.srcErr
		}
		if r.w-r.r > r.maxLine()+1 {
			return nil, r.tooLong(r.pos + 1)
		}
		r.fill()
	}
}

func (r *Reader) tooLong(pos int) error {
	return &SyntaxError{Line: pos, Msg: fmt.Sprintf("line over %d bytes", r.maxLine()), Err: ErrLineTooLong}
}

// indexLineEnd returns the index of the first CR or LF in b, or -1.
func indexLineEnd(b []byte) int {
	lf := bytes.IndexByte(b, '\n')
	head := b
	if lf >= 0 {
		head = b[:lf]
	}
	if cr := bytes.IndexByte(head, '\r'); cr >= 0 {
		return cr
	}
	return lf
}

// peek returns the next unread byte, reading more input if needed.
func (r *Reader) peek() (byte, bool) {
	for r.r == r.w {
		if r.srcErr != nil {
			return 0, false
		}
		r.fill()
	}
	return r.buf[r.r], true
}

// fill reads more input into the buffer, moving unread bytes to the front
// and growing it when full. It records src's error rather than returning it.
func (r *Reader) fill() {
	if r.r > 0 {
		r.w = copy(r.buf, r.buf[r.r:r.w])
		r.r = 0
	}
	if r.w == len(r.buf) {
		size := max(2*len(r.buf), 32<<10)
		nb := make([]byte, size)
		copy(nb, r.buf[:r.w])
		r.buf = nb
	}
	for range 100 {
		n, err := r.src.Read(r.buf[r.w:])
		r.w += n
		if err != nil {
			r.srcErr = err
			return
		}
		if n > 0 {
			return
		}
	}
	r.srcErr = io.ErrNoProgress
}

// skipBOM drops a UTF-8 byte-order mark at the start of the input.
func (r *Reader) skipBOM() error {
	for r.w-r.r < 3 && r.srcErr == nil {
		r.fill()
	}
	if bytes.HasPrefix(r.buf[r.r:r.w], []byte("\xef\xbb\xbf")) {
		r.r += 3
	}
	if r.srcErr != nil && r.srcErr != io.EOF && r.r == r.w {
		return r.srcErr
	}
	return nil
}

// parse splits an unfolded line into name, parameters and value.
//
//	contentline = name *(";" param) ":" value
//	param       = param-name "=" param-value *("," param-value)
func (r *Reader) parse(b []byte, pos int) (Line, error) {
	l := Line{Pos: pos}
	i := 0
	for i < len(b) && isNameByte(b[i]) {
		i++
	}
	if i == 0 {
		return l, &SyntaxError{Line: pos, Msg: fmt.Sprintf("line starts with %s, not a name", quoteByte(b, 0))}
	}
	l.Name = r.name(b[:i])
	for i < len(b) && b[i] == ';' {
		i++
		j := i
		for i < len(b) && isNameByte(b[i]) {
			i++
		}
		if i == j {
			return l, &SyntaxError{Line: pos, Msg: fmt.Sprintf("%s: parameter starts with %s, not a name", l.Name, quoteByte(b, i))}
		}
		p := Param{Name: r.name(b[j:i])}
		if i >= len(b) || b[i] != '=' {
			// vCard 2.1 style, like ";QUOTED-PRINTABLE". Kept as a
			// parameter with no values.
			if err := r.deviate(pos, "%s: parameter %s has no value", l.Name, p.Name); err != nil {
				return l, err
			}
			l.Params = append(l.Params, p)
			continue
		}
		i++
		for {
			v, n, err := r.paramValue(b[i:], pos, l.Name, p.Name)
			if err != nil {
				return l, err
			}
			p.Values = append(p.Values, v)
			i += n
			if i < len(b) && b[i] == ',' {
				i++
				continue
			}
			break
		}
		l.Params = append(l.Params, p)
	}
	if i >= len(b) {
		return l, &SyntaxError{Line: pos, Msg: fmt.Sprintf("%s: no ':' before the end of the line", l.Name)}
	}
	if b[i] != ':' {
		return l, &SyntaxError{Line: pos, Msg: fmt.Sprintf("%s: %s where ';' or ':' should be", l.Name, quoteByte(b, i))}
	}
	value := b[i+1:]
	for k, c := range value {
		if isControl(c) {
			if err := r.deviate(pos, "%s: control character %#02x in the value at byte %d", l.Name, c, k); err != nil {
				return l, err
			}
			break
		}
	}
	if r.Strict && !utf8.Valid(value) {
		return l, &SyntaxError{Line: pos, Msg: l.Name + ": value is not valid UTF-8"}
	}
	l.Value = string(value)
	return l, nil
}

// paramValue reads one parameter value from the start of b and returns it
// with the number of bytes it took up.
//
//	param-value   = paramtext / quoted-string
//	paramtext     = *SAFE-CHAR   ; not CONTROL, DQUOTE, ";", ":", ","
//	quoted-string = DQUOTE *QSAFE-CHAR DQUOTE   ; not CONTROL, DQUOTE
func (r *Reader) paramValue(b []byte, pos int, prop, param string) (string, int, error) {
	if len(b) > 0 && b[0] == '"' {
		end := bytes.IndexByte(b[1:], '"')
		if end < 0 {
			return "", 0, &SyntaxError{Line: pos, Msg: fmt.Sprintf("%s: %s: quoted value has no closing quote", prop, param)}
		}
		v := b[1 : 1+end]
		n := end + 2
		if err := r.checkParamText(v, pos, prop, param); err != nil {
			return "", 0, err
		}
		if n < len(b) && !isParamDelim(b[n]) {
			// Text after the closing quote, like ALTREP="http://x"y. Most
			// readers keep it as part of the value.
			k := n
			for k < len(b) && !isParamDelim(b[k]) {
				k++
			}
			if err := r.deviate(pos, "%s: %s: text after the closing quote", prop, param); err != nil {
				return "", 0, err
			}
			return decode6868(string(v) + string(b[n:k])), k, nil
		}
		return decode6868(string(v)), n, nil
	}
	k := 0
	quote := false
	for k < len(b) && !isParamDelim(b[k]) {
		if b[k] == '"' {
			quote = true
		}
		k++
	}
	if quote {
		if err := r.deviate(pos, "%s: %s: '\"' inside an unquoted value", prop, param); err != nil {
			return "", 0, err
		}
	}
	if err := r.checkParamText(b[:k], pos, prop, param); err != nil {
		return "", 0, err
	}
	return decode6868(string(b[:k])), k, nil
}

func (r *Reader) checkParamText(v []byte, pos int, prop, param string) error {
	for _, c := range v {
		if isControl(c) {
			return r.deviate(pos, "%s: %s: control character %#02x in the value", prop, param, c)
		}
	}
	if r.Strict && !utf8.Valid(v) {
		return &SyntaxError{Line: pos, Msg: fmt.Sprintf("%s: %s: value is not valid UTF-8", prop, param)}
	}
	return nil
}

func isParamDelim(c byte) bool { return c == ',' || c == ';' || c == ':' }

// decode6868 decodes RFC 6868 parameter value escapes: ^n is a newline, ^'
// a double quote and ^^ a caret. A caret before anything else, ^N included,
// is literal.
func decode6868(s string) string {
	i := indexByteString(s, '^')
	if i < 0 {
		return s
	}
	out := make([]byte, 0, len(s))
	out = append(out, s[:i]...)
	for ; i < len(s); i++ {
		c := s[i]
		if c == '^' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				out = append(out, '\n')
				i++
				continue
			case '\'':
				out = append(out, '"')
				i++
				continue
			case '^':
				out = append(out, '^')
				i++
				continue
			}
		}
		out = append(out, c)
	}
	return string(out)
}

func indexByteString(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

// name returns b upper-cased, as a string, without allocating for the names
// RFC 5545 and common producers use.
func (r *Reader) name(b []byte) string {
	for _, c := range b {
		if 'a' <= c && c <= 'z' {
			r.scratch = append(r.scratch[:0], b...)
			for k, c := range r.scratch {
				if 'a' <= c && c <= 'z' {
					r.scratch[k] = c - ('a' - 'A')
				}
			}
			b = r.scratch
			break
		}
	}
	if s, ok := knownNames[string(b)]; ok {
		return s
	}
	return string(b)
}

func quoteByte(b []byte, i int) string {
	if i >= len(b) {
		return "the end of the line"
	}
	return fmt.Sprintf("%q", b[i])
}
