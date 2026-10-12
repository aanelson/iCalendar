package main

// Adapters: each library's natural "read a feed" path, normalized so the
// probes can compare them.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	ical "github.com/aanelson/iCalendar"
	ics "github.com/arran4/golang-ical"
	eical "github.com/emersion/go-ical"
)

var parsers = []string{This, GoIcal, GoIcalO, Emersion}

// nProp is a property as a library reports it. Text is the library's
// decoded TEXT reading (for golang-ical that is the stored value, which it
// unescapes while parsing).
type nProp struct {
	Name   string
	Params map[string][]string
	Raw    string
	Text   string
	TextOK bool
}

type nEvent []nProp

func (e nEvent) get(name string) *nProp {
	for i := range e {
		if strings.EqualFold(e[i].Name, name) {
			return &e[i]
		}
	}
	return nil
}

func (e nEvent) all(name string) []nProp {
	var out []nProp
	for _, p := range e {
		if strings.EqualFold(p.Name, name) {
			out = append(out, p)
		}
	}
	return out
}

// param looks a parameter up case-insensitively (golang-ical keeps the case
// it read).
func (p *nProp) param(name string) []string {
	for k, v := range p.Params {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return nil
}

type parsed struct {
	Cals   int
	Events []nEvent
	Err    error // the error that stopped reading, or nil
}

func (p parsed) summary() string {
	s := fmt.Sprintf("%d calendar(s), %d event(s)", p.Cals, len(p.Events))
	if p.Err != nil {
		s += fmt.Sprintf(", error after them: %v", p.Err)
	}
	return s
}

// parseFeed reads a whole stream as each library's API invites:
//   - this: Decoder.Decode until io.EOF;
//   - golang-ical: ParseCalendar (one calendar is all it reads);
//   - golang-ical+opts: ParseCalendarWithOptions with the lenient options;
//   - emersion: Decoder.Decode until io.EOF.
//
// A bare io.EOF from a decoder is the documented end-of-stream signal, so a
// stream loop stops there without an error, as callers write it.
func parseFeed(lib string, b []byte) parsed {
	switch lib {
	case This:
		return parseThis(b, nil)
	case GoIcal, GoIcalO:
		cal, err := goIcalParse(lib, b)
		if err != nil {
			return parsed{Err: err}
		}
		cals := 1
		if len(cal.CalendarProperties) == 0 && len(cal.Components) == 0 {
			cals = 0 // nothing was read: golang-ical returns an empty Calendar, no error
		}
		return parsed{Cals: cals, Events: goIcalEvents(cal)}
	case Emersion:
		var out parsed
		d := eical.NewDecoder(bytes.NewReader(b))
		for {
			cal, err := d.Decode()
			if err == io.EOF {
				return out
			}
			if err != nil {
				out.Err = err
				return out
			}
			out.Cals++
			for _, ev := range cal.Events() {
				out.Events = append(out.Events, emersionEvent(ev.Component))
			}
		}
	}
	panic("unknown lib " + lib)
}

func parseThis(b []byte, tune func(*ical.Decoder)) parsed {
	var out parsed
	d := ical.NewDecoder(bytes.NewReader(b))
	if tune != nil {
		tune(d)
	}
	for {
		cal, err := d.Decode()
		if err == io.EOF {
			return out
		}
		if err != nil {
			out.Err = err
			return out
		}
		out.Cals++
		for ev := range cal.Events() {
			var e nEvent
			for _, p := range ev.Props {
				np := nProp{Name: p.Name, Params: map[string][]string{}, Raw: p.Value, Text: p.Text(), TextOK: true}
				for _, pp := range p.Params {
					np.Params[pp.Name] = append(np.Params[pp.Name], pp.Values...)
				}
				e = append(e, np)
			}
			out.Events = append(out.Events, e)
		}
	}
}

func goIcalOpts(lib string) []any {
	if lib == GoIcalO {
		return []any{
			ics.WithUnknownPropertyHandler(ics.AcceptUnknownPropertyHandler),
			ics.WithPropertyParser(ics.FallbackParser(ics.LooseParser)),
			ics.WithWindowsTimezoneMapping(),
		}
	}
	return nil
}

func goIcalParse(lib string, b []byte) (*ics.Calendar, error) {
	return ics.ParseCalendarWithOptions(bytes.NewReader(b), goIcalOpts(lib)...)
}

func goIcalEvents(cal *ics.Calendar) []nEvent {
	var out []nEvent
	for _, ev := range cal.Events() {
		var e nEvent
		for _, p := range ev.Properties {
			e = append(e, nProp{Name: p.IANAToken, Params: p.ICalParameters, Raw: p.Value, Text: p.Value, TextOK: true})
		}
		out = append(out, e)
	}
	return out
}

func emersionEvent(c *eical.Component) nEvent {
	var e nEvent
	for name, ps := range c.Props {
		for _, p := range ps {
			np := nProp{Name: name, Params: map[string][]string(p.Params), Raw: p.Value}
			if t, err := p.Text(); err == nil {
				np.Text, np.TextOK = t, true
			} else {
				np.Text = "<Text() error: " + err.Error() + ">"
			}
			e = append(e, np)
		}
	}
	return e
}

// firstEvent parses b and returns the first VEVENT in each library's own
// model, for probes that use the typed accessors.
type libEvent struct {
	this *ical.Event
	gi   *ics.VEvent
	em   *eical.Event
}

func firstEvent(lib string, b []byte) (libEvent, error) {
	switch lib {
	case This:
		cal, err := ical.Parse(bytes.NewReader(b))
		if err != nil {
			return libEvent{}, err
		}
		for ev := range cal.Events() {
			return libEvent{this: &ev}, nil
		}
	case GoIcal, GoIcalO:
		cal, err := goIcalParse(lib, b)
		if err != nil {
			return libEvent{}, err
		}
		if evs := cal.Events(); len(evs) > 0 {
			return libEvent{gi: evs[0]}, nil
		}
	case Emersion:
		cal, err := eical.NewDecoder(bytes.NewReader(b)).Decode()
		if err != nil {
			return libEvent{}, err
		}
		if evs := cal.Events(); len(evs) > 0 {
			return libEvent{em: &evs[0]}, nil
		}
	default:
		return libEvent{}, fmt.Errorf("no parser")
	}
	return libEvent{}, errors.New("no VEVENT found")
}

// start returns DTSTART as an instant through the library's typed accessor:
// this: Event.Start().Time(); golang-ical: GetStartAt(); emersion:
// Event.DateTimeStart(nil).
func (e libEvent) start() (time.Time, error) {
	switch {
	case e.this != nil:
		v, err := e.this.Start()
		if err != nil {
			return time.Time{}, err
		}
		return v.Time(), nil
	case e.gi != nil:
		return e.gi.GetStartAt()
	case e.em != nil:
		return e.em.DateTimeStart(nil)
	}
	return time.Time{}, errors.New("no event")
}

// end returns the event's end through the library's typed accessor, which
// for this library and emersion applies DURATION; golang-ical only reads
// DTEND.
func (e libEvent) end() (time.Time, error) {
	switch {
	case e.this != nil:
		v, err := e.this.End()
		if err != nil {
			return time.Time{}, err
		}
		return v.Time(), nil
	case e.gi != nil:
		return e.gi.GetEndAt()
	case e.em != nil:
		return e.em.DateTimeEnd(nil)
	}
	return time.Time{}, errors.New("no event")
}

const calHead = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//probe//EN\r\n"

// calWith returns a calendar with one VEVENT holding lines (CRLF-joined)
// after UID and DTSTAMP, and optional components before it.
func calWith(before string, lines ...string) []byte {
	return []byte(calHead + before + "BEGIN:VEVENT\r\nUID:probe-1\r\nDTSTAMP:20260101T000000Z\r\n" +
		strings.Join(lines, "\r\n") + "\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
}

func utc(t time.Time) string {
	if t.IsZero() {
		return "zero time"
	}
	return t.UTC().Format(time.RFC3339)
}
