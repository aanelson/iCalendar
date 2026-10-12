package ical

import (
	"bytes"
	"fmt"
	"io"

	"github.com/aanelson/iCalendar/contentline"
)

// An Encoder writes calendars as iCalendar text: CRLF line endings, lines
// folded at 75 octets without splitting a UTF-8 sequence, properties and
// components in the order the tree holds them.
//
// It refuses to write what it couldn't read back as the same tree: an
// invalid name, or a control character in a value, which would otherwise
// let a value start a line (or a component) of its own.
type Encoder struct {
	w *contentline.Writer
}

// NewEncoder returns an encoder writing to w.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: contentline.NewWriter(w)}
}

// Encode writes cal and flushes. On error, the output may hold part of the
// calendar.
func (e *Encoder) Encode(cal *Calendar) error {
	if cal == nil || cal.Component == nil {
		return fmt.Errorf("ical: encoding a nil calendar")
	}
	if err := e.component(cal.Component); err != nil {
		return err
	}
	return e.w.Flush()
}

// EncodeComponent writes a single component, such as a VEVENT for an
// iTIP message body, and flushes.
func (e *Encoder) EncodeComponent(c *Component) error {
	if err := e.component(c); err != nil {
		return err
	}
	return e.w.Flush()
}

func (e *Encoder) component(c *Component) error {
	if !isName(c.Name) {
		return fmt.Errorf("ical: component name %q is not a name", c.Name)
	}
	if err := e.w.WriteLine(contentline.Line{Name: "BEGIN", Value: c.Name}); err != nil {
		return err
	}
	for _, p := range c.Props {
		if p == nil {
			continue
		}
		l := contentline.Line{Name: p.Name, Value: p.Value}
		if len(p.Params) > 0 {
			l.Params = make([]contentline.Param, len(p.Params))
			for i, pp := range p.Params {
				l.Params[i] = contentline.Param{Name: pp.Name, Values: pp.Values}
			}
		}
		if err := e.w.WriteLine(l); err != nil {
			return fmt.Errorf("ical: %s: %w", c.Name, err)
		}
	}
	for _, child := range c.Children {
		if child == nil {
			continue
		}
		if err := e.component(child); err != nil {
			return err
		}
	}
	return e.w.WriteLine(contentline.Line{Name: "END", Value: c.Name})
}

// Bytes returns cal encoded, or the error Encode would return.
func (cal *Calendar) Bytes() ([]byte, error) {
	var b bytes.Buffer
	if err := NewEncoder(&b).Encode(cal); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
