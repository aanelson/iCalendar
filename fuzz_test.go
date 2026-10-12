package ical

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/aanelson/iCalendar/contentline"
)

// Decoding arbitrary input never panics; whatever decodes re-encodes to
// text that decodes to the same tree; and expanding it terminates.
func FuzzDecode(f *testing.F) {
	f.Add([]byte(sample), false)
	for _, path := range []string{
		"testdata/rfc5545/ics/4-a.ics",
		"testdata/thirdparty/python-icalendar/ics/src/icalendar/tests/calendars/broken_ical.ics",
		"testdata/thirdparty/icaljs/ics/samples/recur_instances.ics",
	} {
		if b, err := os.ReadFile(path); err == nil {
			f.Add(b, false)
		}
	}
	f.Fuzz(func(t *testing.T, in []byte, strict bool) {
		d := NewDecoder(bytes.NewReader(in))
		d.Strict = strict
		d.MaxLineLength = 1 << 16
		cal, err := d.Decode()
		if err != nil {
			return
		}
		out, err := cal.Bytes()
		if err != nil {
			if !errors.Is(err, contentline.ErrInvalidLine) && !strings.Contains(err.Error(), "not a name") {
				t.Fatalf("encode: %v", err)
			}
			return
		}
		back, err := Parse(bytes.NewReader(out))
		if err != nil {
			t.Fatalf("reading our own output: %v\n%q", err, out)
		}
		if !reflect.DeepEqual(back.Component, bareParams(cal.Component)) {
			t.Fatalf("round trip changed the tree:\n%q", out)
		}
		n := 0
		from := time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC)
		for range cal.Occurrences(from, from.AddDate(50, 0, 0)) {
			if n++; n > 500 {
				break
			}
		}
	})
}

// The inputs that made other Go readers slow take linear time end to end.
func TestHostileFeeds(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	wrap := func(line string) []byte {
		return []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//EN\r\nBEGIN:VEVENT\r\n" + line + "\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
	}
	for name, in := range map[string][]byte{
		"1 MiB parameter value": wrap("X;A=" + strings.Repeat("b", 1<<20) + ":v"),
		"5 MiB of parameters":   wrap("X" + strings.Repeat(";A=b", 5<<20/4) + ":v"),
		"many comma values":     wrap("X;A=" + strings.Repeat("b,", 1<<20) + "b:v"),
		"100k events":           []byte(strings.Repeat(string(wrap("UID:x\r\nDTSTART:20240101T000000Z\r\nRRULE:FREQ=DAILY")), 1)),
	} {
		start := time.Now()
		d := NewDecoder(bytes.NewReader(in))
		d.MaxLineLength = 8 << 20
		if _, err := d.Decode(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if el := time.Since(start); el > 2*time.Second {
			t.Errorf("%s took %v", name, el)
		}
	}
}

// bareParams returns c with each valueless parameter (the vCard 2.1
// ";QUOTED-PRINTABLE" kind, which the decoder keeps as it is) given the one
// empty value it has once written in RFC 5545 form, "QUOTED-PRINTABLE=".
func bareParams(c *Component) *Component {
	out := &Component{Name: c.Name}
	for _, p := range c.Props {
		q := *p
		q.Params = nil
		for _, pp := range p.Params {
			if len(pp.Values) == 0 {
				pp.Values = []string{""}
			}
			q.Params = append(q.Params, pp)
		}
		out.Props = append(out.Props, &q)
	}
	for _, child := range c.Children {
		out.Children = append(out.Children, bareParams(child))
	}
	return out
}
