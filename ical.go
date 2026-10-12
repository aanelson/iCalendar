// Package ical reads, writes and expands iCalendar data (RFC 5545).
//
// A calendar is a tree of components, each holding properties in the order
// they were written:
//
//	cal, err := ical.Parse(r)
//	for ev := range cal.Events() {
//		start, err := ev.Start()
//		fmt.Println(ev.Summary(), start.Time())
//	}
//
// Property values are kept as written and decoded on demand by typed
// accessors (Property.Text, Property.DateTime, Property.Recur, …), so
// unknown properties, parameters and components survive a round trip
// unchanged and nothing is decoded that nobody reads. Typed views (Event,
// Todo, Journal, Alarm, …) name the RFC's properties and apply its defaults.
//
// Recurrence is expanded by this module's own engine (package recur), with
// RECURRENCE-ID overrides, RDATE, EXDATE and VTIMEZONE handling, and an
// optional cache:
//
//	for occ, err := range cal.Occurrences(from, to) {
//		…
//	}
//
// The library never reads time.Local. Floating times and dates stay
// floating until the caller names a zone for them.
package ical

import (
	"iter"
	"strings"
)

// A Component is a BEGIN/END block: VCALENDAR, VEVENT, VALARM, an X-
// component, or any other.
type Component struct {
	// Name is the component name in upper case, like "VEVENT".
	Name string
	// Props are the component's properties in document order.
	Props Props
	// Children are the components nested inside this one, in order.
	Children []*Component
}

// NewComponent returns an empty component named name.
func NewComponent(name string) *Component {
	return &Component{Name: strings.ToUpper(name)}
}

// Get returns the component's first property named name, compared
// case-insensitively, or nil.
func (c *Component) Get(name string) *Property {
	return c.Props.Get(name)
}

// Components returns the children named name, compared
// case-insensitively, or all children when name is "".
func (c *Component) Components(name string) iter.Seq[*Component] {
	return func(yield func(*Component) bool) {
		for _, child := range c.Children {
			if (name == "" || strings.EqualFold(child.Name, name)) && !yield(child) {
				return
			}
		}
	}
}

// Add appends children to the component.
func (c *Component) Add(children ...*Component) {
	c.Children = append(c.Children, children...)
}

// Props is a list of properties in document order. A name may occur more
// than once (ATTENDEE, EXDATE), so lookups return the first match or
// iterate over all of them. Names compare case-insensitively.
type Props []*Property

// Get returns the first property named name, or nil.
func (ps Props) Get(name string) *Property {
	for _, p := range ps {
		if strings.EqualFold(p.Name, name) {
			return p
		}
	}
	return nil
}

// All returns every property named name, in order.
func (ps Props) All(name string) iter.Seq[*Property] {
	return func(yield func(*Property) bool) {
		for _, p := range ps {
			if strings.EqualFold(p.Name, name) && !yield(p) {
				return
			}
		}
	}
}

// Add appends p.
func (ps *Props) Add(p *Property) {
	*ps = append(*ps, p)
}

// Set replaces every property named p.Name with p, which takes the place
// of the first one, or is appended if there was none.
func (ps *Props) Set(p *Property) {
	out := (*ps)[:0]
	placed := false
	for _, q := range *ps {
		if !strings.EqualFold(q.Name, p.Name) {
			out = append(out, q)
		} else if !placed {
			out = append(out, p)
			placed = true
		}
	}
	if !placed {
		out = append(out, p)
	}
	if len(out) < len(*ps) {
		clear((*ps)[len(out):]) // let dropped properties be collected
	}
	*ps = out
}

// Del removes every property named name.
func (ps *Props) Del(name string) {
	out := (*ps)[:0]
	for _, q := range *ps {
		if !strings.EqualFold(q.Name, name) {
			out = append(out, q)
		}
	}
	clear((*ps)[len(out):])
	*ps = out
}

// A Property is one content line of a component: a name, parameters and a
// value.
type Property struct {
	// Name is the property name in upper case, like "DTSTART".
	Name string
	// Params are the property's parameters in document order.
	Params Params
	// Value is the value exactly as written: TEXT escapes and list commas
	// are still in it. Use the typed accessors to read it.
	Value string
}

// NewProperty returns a property with a raw value, written as given. Use
// the typed setters (SetText, SetDateTime, …) to have it encoded.
func NewProperty(name, value string) *Property {
	return &Property{Name: strings.ToUpper(name), Value: value}
}

// Params is a list of parameters in document order. Names compare
// case-insensitively.
type Params []Param

// A Param is one parameter: a name and its values, unquoted and with RFC
// 6868 escapes decoded.
type Param struct {
	Name   string
	Values []string
}

// Get returns the first value of the parameter named name, or "".
func (ps Params) Get(name string) string {
	if vs := ps.Values(name); len(vs) > 0 {
		return vs[0]
	}
	return ""
}

// Has reports whether a parameter named name is present.
func (ps Params) Has(name string) bool {
	for _, p := range ps {
		if strings.EqualFold(p.Name, name) {
			return true
		}
	}
	return false
}

// Values returns the values of the parameter named name.
func (ps Params) Values(name string) []string {
	for _, p := range ps {
		if strings.EqualFold(p.Name, name) {
			return p.Values
		}
	}
	return nil
}

// Set replaces the parameter named name with one holding values, or adds
// it.
func (ps *Params) Set(name string, values ...string) {
	name = strings.ToUpper(name)
	for i, p := range *ps {
		if strings.EqualFold(p.Name, name) {
			(*ps)[i] = Param{Name: name, Values: values}
			// Drop any repeats of the same parameter.
			rest := (*ps)[i+1:]
			out := rest[:0]
			for _, q := range rest {
				if !strings.EqualFold(q.Name, name) {
					out = append(out, q)
				}
			}
			*ps = (*ps)[:i+1+len(out)]
			return
		}
	}
	*ps = append(*ps, Param{Name: name, Values: values})
}

// Del removes the parameter named name.
func (ps *Params) Del(name string) {
	out := (*ps)[:0]
	for _, p := range *ps {
		if !strings.EqualFold(p.Name, name) {
			out = append(out, p)
		}
	}
	*ps = out
}
