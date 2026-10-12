package main

// 1. Parsing.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	ical "github.com/aanelson/iCalendar"
	eical "github.com/emersion/go-ical"
)

// textProbe parses a calendar whose VEVENT has line and checks the TEXT
// reading of property prop.
func textProbe(id, title, rfc, line, prop, want string) {
	register(&Probe{ID: id, Group: "1 parsing", Title: title, RFC: rfc, Libs: parsers,
		Run: func(lib, _ string) Result {
			p := parseFeed(lib, calWith("", line))
			if p.Err != nil {
				return failed(p.Err)
			}
			if len(p.Events) != 1 {
				return wrong(p.summary(), "1 event")
			}
			pr := p.Events[0].get(prop)
			if pr == nil {
				return wrong("no "+prop+" property", fmt.Sprintf("%q", want))
			}
			return check(fmt.Sprintf("%q", pr.Text), fmt.Sprintf("%q", want))
		}})
}

func init() {
	register(&Probe{ID: "1.01-lowercase-names", Group: "1 parsing",
		Title: "lowercase names (begin:vcalendar, dtstart:…)",
		RFC:   "§3.1 / §2: names of properties, parameters and enumerated values are case-insensitive",
		Libs:  parsers,
		Run: func(lib, _ string) Result {
			in := []byte("begin:vcalendar\r\nversion:2.0\r\nprodid:-//probe//EN\r\nbegin:vevent\r\nuid:1\r\ndtstamp:20260101T000000Z\r\ndtstart:20260101T100000Z\r\nsummary:hi\r\nend:vevent\r\nend:vcalendar\r\n")
			ev, err := firstEvent(lib, in)
			if err != nil {
				return failed(err)
			}
			t, err := ev.start()
			if err != nil {
				return Result{Outcome: "error", Detail: "parsed, but DTSTART accessor: " + err.Error()}
			}
			return check(utc(t), "2026-01-01T10:00:00Z")
		}})

	textProbe("1.02-fold-space", "folding with SPACE", "§3.1 unfolding: CRLF + one SPACE or HTAB removed",
		"SUMMARY:Hello\r\n World", "SUMMARY", "HelloWorld")
	textProbe("1.03-fold-htab", "folding with HTAB", "§3.1 unfolding: CRLF + one SPACE or HTAB removed",
		"SUMMARY:Hello\r\n\tWorld", "SUMMARY", "HelloWorld")
	textProbe("1.04-fold-in-utf8", "fold inside a multi-byte UTF-8 character", "§3.1 note: unfold so a split multi-octet sequence is restored",
		"SUMMARY:caf\xc3\r\n \xa9 \xe2\x82\r\n \xac\xf0\x9f\r\n \x98\x80", "SUMMARY", "café €😀")

	register(&Probe{ID: "1.05-quoted-param", Group: "1 parsing",
		Title: "quoted parameter values containing : ; ,",
		RFC:   "§3.1 quoted-string / §3.2: values with COLON, SEMICOLON or COMMA MUST be quoted",
		Libs:  parsers,
		Run: func(lib, _ string) Result {
			p := parseFeed(lib, calWith("", `ATTENDEE;CN="Doe, John; Jr: III";X-URL="http://x.example/a:b;c,d":mailto:j@example.com`))
			if p.Err != nil {
				return failed(p.Err)
			}
			if len(p.Events) != 1 {
				return wrong(p.summary(), "1 event")
			}
			a := p.Events[0].get("ATTENDEE")
			if a == nil {
				return wrong("no ATTENDEE", "")
			}
			got := fmt.Sprintf("CN=%q X-URL=%q value=%q", a.param("CN"), a.param("X-URL"), a.Raw)
			return check(got, fmt.Sprintf("CN=%q X-URL=%q value=%q", []string{"Doe, John; Jr: III"}, []string{"http://x.example/a:b;c,d"}, "mailto:j@example.com"))
		}})

	register(&Probe{ID: "1.06-rfc6868", Group: "1 parsing",
		Title: "RFC 6868 parameter escapes ^' ^n ^^",
		RFC:   "RFC 6868 §3 (an extension RFC, not part of 5545)",
		Libs:  parsers,
		Run: func(lib, _ string) Result {
			p := parseFeed(lib, calWith("", `ATTENDEE;CN="George ^'Babe^' Ruth ^^ ^nX":mailto:b@example.com`))
			if p.Err != nil {
				return failed(p.Err)
			}
			if len(p.Events) != 1 {
				return wrong(p.summary(), "1 event")
			}
			a := p.Events[0].get("ATTENDEE")
			if a == nil {
				return wrong("no ATTENDEE", "")
			}
			return check(fmt.Sprintf("%q", a.param("CN")), fmt.Sprintf("%q", []string{"George \"Babe\" Ruth ^ \nX"}))
		}})

	textProbe("1.07-text-unescape", `TEXT unescaping \, \; \n \\ (and \\n is backslash+n)`, "§3.3.11 TEXT: ESCAPED-CHAR",
		`DESCRIPTION:a\,b\;c\nd\\e\\nf`, "DESCRIPTION", "a,b;c\nd\\e\\nf")
	textProbe("1.08-summary-comma", "SUMMARY:Dinner, drinks is one value",
		"§3.8.1.12: SUMMARY is a single TEXT (the unescaped comma is itself non-conforming per §3.3.11, but SUMMARY is not a list)",
		"SUMMARY:Dinner, drinks", "SUMMARY", "Dinner, drinks")

	register(&Probe{ID: "1.09-two-calendars", Group: "1 parsing",
		Title: "two VCALENDARs in one stream",
		RFC:   "§3.4: an iCalendar stream holds one or more iCalendar objects (icalstream = 1*icalobject)",
		Libs:  parsers,
		Run: func(lib, _ string) Result {
			one := string(calWith("", "SUMMARY:a"))
			p := parseFeed(lib, []byte(one+strings.Replace(one, "SUMMARY:a", "SUMMARY:b", 1)))
			if p.Err != nil {
				return Result{Outcome: "error", Detail: p.summary()}
			}
			return check(fmt.Sprintf("%d calendars, %d events", p.Cals, len(p.Events)), "2 calendars, 2 events")
		}})

	register(&Probe{ID: "1.10-lf-only", Group: "1 parsing",
		Title: "LF-only line endings",
		RFC:   "§3.1 requires CRLF; LF-only is non-conforming but common. Pass = read the same event",
		Libs:  parsers,
		Run: func(lib, _ string) Result {
			in := strings.ReplaceAll(string(calWith("", "SUMMARY:Hello", " World")), "\r\n", "\n")
			p := parseFeed(lib, []byte(in))
			if p.Err != nil {
				return failed(p.Err)
			}
			if len(p.Events) != 1 || p.Events[0].get("SUMMARY") == nil {
				return wrong(p.summary(), "1 event")
			}
			return check(p.Events[0].get("SUMMARY").Text, "HelloWorld")
		}})

	// Truncation. Pass = an error the caller can't mistake for success.
	// Empty input is different: a stream decoder's io.EOF ("no calendars")
	// is the usual Go answer, and this library's own Decoder gives it too.
	truncated := func(id, title, in string, empty bool) {
		rfc := "§3.4/§3.6: every BEGIN needs its END. Pass = an error, not a silent shorter calendar"
		if empty {
			rfc = "§3.4: an iCalendar stream holds at least one object. Pass = an error, or io.EOF from a stream decoder"
		}
		register(&Probe{ID: id, Group: "1 parsing", Title: title,
			RFC:  rfc,
			Libs: parsers,
			Run: func(lib, _ string) Result { return truncProbe(lib, []byte(in), empty) }})
	}
	trunc := func(id, title, in string) { truncated(id, title, in, false) }
	full := string(calWith("", "DTSTART;TZID=America/New_York:20260101T100000", "SUMMARY:x"))
	trunc("1.11a-missing-end-vcalendar", "feed missing only END:VCALENDAR", strings.TrimSuffix(full, "END:VCALENDAR\r\n"))
	trunc("1.11b-cut-inside-vevent", "feed cut inside a VEVENT (after a whole line)", full[:strings.Index(full, "SUMMARY")])
	trunc("1.11c-cut-mid-line", "feed cut mid-line inside a parameter (…DTSTART;TZID=Amer)", full[:strings.Index(full, "TZID=")+9])
	truncated("1.12-empty", "empty input", "", true)

	register(&Probe{ID: "1.13-bom", Group: "1 parsing",
		Title: "UTF-8 BOM before BEGIN:VCALENDAR",
		RFC:   "§3.1.4 (UTF-8); RFC is silent on a BOM, real feeds have one. Pass = read the event",
		Libs:  parsers,
		Run: func(lib, _ string) Result {
			p := parseFeed(lib, append([]byte("\xEF\xBB\xBF"), calWith("", "SUMMARY:x")...))
			if p.Err != nil {
				return failed(p.Err)
			}
			return check(fmt.Sprintf("%d events", len(p.Events)), "1 events")
		}})
}

// truncProbe reports what the single-calendar call returns, and what a
// stream loop (Decode until io.EOF) would conclude.
func truncProbe(lib string, in []byte, empty bool) Result {
	var err error
	var events int
	switch lib {
	case This:
		var cal *ical.Calendar
		cal, err = ical.Parse(bytes.NewReader(in))
		if cal != nil {
			for range cal.Events() {
				events++
			}
		}
	case GoIcal, GoIcalO:
		cal, e := goIcalParse(lib, in)
		err = e
		if cal != nil && e == nil {
			events = len(cal.Events())
		}
	case Emersion:
		var cal *eical.Calendar
		cal, err = eical.NewDecoder(bytes.NewReader(in)).Decode()
		if cal != nil {
			events = len(cal.Events())
		}
	}
	stream := parseFeed(lib, in)
	detail := fmt.Sprintf("single-calendar call: err=%v, %d event(s); stream loop sees: %s", err, events, stream.summary())
	switch {
	case err == nil:
		return Result{Outcome: "wrong", Got: fmt.Sprintf("silent success, %d event(s)", events), Want: "error", Detail: detail}
	case err == io.EOF && empty:
		return Result{Outcome: "pass", Got: "io.EOF: no calendars", Detail: detail}
	case errors.Is(err, io.EOF) && err == io.EOF:
		// A bare io.EOF is the end-of-stream signal: a Decode loop stops
		// there with no error.
		return Result{Outcome: "wrong", Got: "bare io.EOF (a Decode loop reads it as end of stream: 0 calendars, no error)", Want: "error", Detail: detail}
	default:
		return Result{Outcome: "pass", Got: "error: " + err.Error(), Detail: detail}
	}
}
