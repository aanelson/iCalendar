package ical

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/aanelson/iCalendar/contentline"
)

// A SyntaxError reports iCalendar text that can't be read: a malformed
// content line, unbalanced BEGIN and END, or input that ends inside a
// component (which wraps io.ErrUnexpectedEOF).
type SyntaxError struct {
	Line int    // 1-based physical line
	Msg  string // what is wrong
	Err  error  // the underlying error, or nil
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("ical: line %d: %s", e.Line, e.Msg)
}

func (e *SyntaxError) Unwrap() error { return e.Err }

// Default limits for a Decoder whose fields are zero.
const (
	DefaultMaxLineLength = contentline.DefaultMaxLineLength
	DefaultMaxDepth      = 32
)

// A Decoder reads calendars from an input stream.
//
// By default it recovers from the mistakes real producers make and reports
// each recovery to Warn: a content line it can't parse is skipped, a
// mismatched END closes the components it names, and lines outside a
// VCALENDAR are ignored. What it never recovers from is losing data
// silently: input that ends inside a component is an error wrapping
// io.ErrUnexpectedEOF, never a shorter calendar, and so is a line over
// MaxLineLength or nesting deeper than MaxDepth.
//
// Set the fields before the first call to Decode.
type Decoder struct {
	// Strict makes every deviation from RFC 5545 syntax an error.
	Strict bool
	// Warn, if not nil, receives a *SyntaxError for each deviation the
	// decoder recovers from. It is not called in Strict mode.
	Warn func(error)
	// MaxLineLength bounds an unfolded content line, in bytes. Zero means
	// DefaultMaxLineLength.
	MaxLineLength int
	// MaxDepth bounds how deeply components nest. Zero means
	// DefaultMaxDepth.
	MaxDepth int

	src io.Reader
	r   *contentline.Reader
}

// NewDecoder returns a decoder reading from r.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{src: r}
}

// Parse reads exactly one calendar from r. Input with a second VCALENDAR
// after the first is an error; use a Decoder to read a stream of them.
func Parse(r io.Reader) (*Calendar, error) {
	d := NewDecoder(r)
	cal, err := d.Decode()
	if err != nil {
		if err == io.EOF {
			return nil, &SyntaxError{Line: 1, Msg: "no VCALENDAR in the input", Err: io.ErrUnexpectedEOF}
		}
		return nil, err
	}
	if _, err := d.Decode(); err != io.EOF {
		if err == nil {
			return nil, &SyntaxError{Line: d.r.Line(), Msg: "more than one VCALENDAR; use a Decoder"}
		}
		return nil, err
	}
	return cal, nil
}

// Decode reads the next VCALENDAR. It returns io.EOF, unwrapped, when the
// input holds no further calendar.
func (d *Decoder) Decode() (*Calendar, error) {
	if d.r == nil {
		d.r = contentline.NewReader(d.src)
		d.r.Strict = d.Strict
		if d.Warn != nil {
			d.r.Warn = func(err error) { d.Warn(d.wrap(err)) }
		}
		d.r.MaxLineLength = d.MaxLineLength
	}
	maxDepth := d.MaxDepth
	if maxDepth <= 0 {
		maxDepth = DefaultMaxDepth
	}

	// Find BEGIN:VCALENDAR.
	var start int
	for {
		l, err := d.r.ReadLine()
		if err == io.EOF {
			return nil, io.EOF
		}
		if err != nil {
			return nil, d.wrap(err)
		}
		if l.Name == "BEGIN" && strings.EqualFold(strings.TrimSpace(l.Value), "VCALENDAR") {
			if _, err := d.blockName(l); err != nil {
				return nil, err
			}
			start = l.Pos
			break
		}
		if err := d.deviate(l.Pos, "%s:%s outside a VCALENDAR", l.Name, l.Value); err != nil {
			return nil, err
		}
	}

	root := &Component{Name: "VCALENDAR"}
	stack := []*Component{root}
	last := start
	for {
		l, err := d.r.ReadLine()
		if err == io.EOF {
			return nil, &SyntaxError{
				Line: last,
				Msg:  fmt.Sprintf("input ends inside %s (BEGIN on line %d)", stack[len(stack)-1].Name, start),
				Err:  io.ErrUnexpectedEOF,
			}
		}
		if err != nil {
			return nil, d.wrap(err)
		}
		last = l.Pos
		top := stack[len(stack)-1]
		switch l.Name {
		case "BEGIN":
			name, err := d.blockName(l)
			if err != nil {
				return nil, err
			}
			if !isName(name) {
				return nil, &SyntaxError{Line: l.Pos, Msg: fmt.Sprintf("BEGIN:%s is not a component name", l.Value)}
			}
			if len(stack) >= maxDepth {
				return nil, &SyntaxError{Line: l.Pos, Msg: fmt.Sprintf("components nest deeper than %d", maxDepth)}
			}
			c := &Component{Name: name}
			top.Children = append(top.Children, c)
			stack = append(stack, c)
		case "END":
			name, err := d.blockName(l)
			if err != nil {
				return nil, err
			}
			if name == top.Name {
				stack = stack[:len(stack)-1]
				if len(stack) == 0 {
					return d.finish(root, start)
				}
				continue
			}
			// END for an enclosing component closes the ones in between;
			// END for nothing open is ignored.
			at := -1
			for i := len(stack) - 2; i >= 0; i-- {
				if stack[i].Name == name {
					at = i
					break
				}
			}
			if at < 0 {
				if err := d.deviate(l.Pos, "END:%s with no BEGIN:%s", l.Value, l.Value); err != nil {
					return nil, err
				}
				continue
			}
			if err := d.deviate(l.Pos, "END:%s closes %s, which has no END", name, top.Name); err != nil {
				return nil, err
			}
			stack = stack[:at]
			if len(stack) == 0 {
				return d.finish(root, start)
			}
		default:
			p := &Property{Name: l.Name, Value: l.Value}
			if len(l.Params) > 0 {
				p.Params = make(Params, len(l.Params))
				for i, lp := range l.Params {
					p.Params[i] = Param{Name: lp.Name, Values: lp.Values}
				}
			}
			top.Props = append(top.Props, p)
		}
	}
}

// blockName returns the component name a BEGIN or END line names, in upper
// case. RFC 5545 §3.4 and §3.6 write these lines as "BEGIN" ":" name, so
// parameters or space around the name are deviations.
func (d *Decoder) blockName(l contentline.Line) (string, error) {
	if len(l.Params) > 0 {
		if err := d.deviate(l.Pos, "%s:%s has parameters", l.Name, l.Value); err != nil {
			return "", err
		}
	}
	name := strings.TrimSpace(l.Value)
	if name != l.Value {
		if err := d.deviate(l.Pos, "%s:%q has space around the name", l.Name, l.Value); err != nil {
			return "", err
		}
	}
	return strings.ToUpper(name), nil
}

// finish checks what RFC 5545 §3.6 requires of every calendar: PRODID and
// VERSION exactly once, CALSCALE and METHOD at most once, and at least one
// component. Lenient mode reads a calendar with no components as the empty
// calendar it is, without a warning.
func (d *Decoder) finish(root *Component, line int) (*Calendar, error) {
	for _, name := range []string{"PRODID", "VERSION", "CALSCALE", "METHOD"} {
		n := 0
		for range root.Props.All(name) {
			n++
		}
		if n == 0 && (name == "PRODID" || name == "VERSION") {
			if err := d.deviate(line, "VCALENDAR has no %s", name); err != nil {
				return nil, err
			}
		}
		if n > 1 {
			if err := d.deviate(line, "VCALENDAR has %d %s properties", n, name); err != nil {
				return nil, err
			}
		}
	}
	if d.Strict && len(root.Children) == 0 {
		return nil, &SyntaxError{Line: line, Msg: "VCALENDAR has no components"}
	}
	return &Calendar{Component: root}, nil
}

func (d *Decoder) deviate(line int, format string, args ...any) error {
	err := &SyntaxError{Line: line, Msg: fmt.Sprintf(format, args...)}
	if d.Strict {
		return err
	}
	if d.Warn != nil {
		d.Warn(err)
	}
	return nil
}

// wrap turns a content-line error into a SyntaxError.
func (d *Decoder) wrap(err error) error {
	var se *contentline.SyntaxError
	if errors.As(err, &se) {
		return &SyntaxError{Line: se.Line, Msg: se.Msg, Err: err}
	}
	return err
}

// isName reports whether s is an iana-token or x-name: ALPHA, DIGIT, "-".
func isName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !('A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || '0' <= c && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}
