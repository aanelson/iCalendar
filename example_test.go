package ical_test

import (
	"fmt"
	"strings"
	"time"

	ical "github.com/aanelson/iCalendar"
	"github.com/aanelson/iCalendar/civil"
	"github.com/aanelson/iCalendar/recur"
)

const feed = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Example//EN
BEGIN:VEVENT
UID:standup@example.com
DTSTAMP:20240101T000000Z
DTSTART;TZID=Eastern Standard Time:20240304T090000
DURATION:PT15M
RRULE:FREQ=WEEKLY;BYDAY=MO,WE
SUMMARY:Standup
END:VEVENT
BEGIN:VEVENT
UID:standup@example.com
DTSTAMP:20240101T000000Z
RECURRENCE-ID;TZID=Eastern Standard Time:20240311T090000
DTSTART;TZID=Eastern Standard Time:20240311T100000
DURATION:PT15M
SUMMARY:Standup (late start)
END:VEVENT
END:VCALENDAR
`

func Example() {
	cal, err := ical.Parse(strings.NewReader(feed))
	if err != nil {
		panic(err)
	}
	from := time.Date(2024, 3, 4, 0, 0, 0, 0, time.UTC)
	for occ, err := range cal.Occurrences(from, from.AddDate(0, 0, 10)) {
		if err != nil {
			panic(err)
		}
		ev := ical.Event{Item: ical.Item{Component: occ.Component}}
		fmt.Println(occ.Start.Time().Format("Mon Jan 2 15:04 MST"), ev.Summary())
	}
	// Output:
	// Mon Mar 4 09:00 EST Standup
	// Wed Mar 6 09:00 EST Standup
	// Mon Mar 11 10:00 EDT Standup (late start)
	// Wed Mar 13 09:00 EDT Standup
}

func ExampleNewCalendar() {
	cal := ical.NewCalendar("-//Example//EN")
	ev := ical.NewEvent("launch@example.com", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	ev.SetSummary("Launch, finally")
	ev.SetStart(ical.NewDate(civil.Date{Year: 2024, Month: 6, Day: 3}))
	cal.Add(ev.Component)
	b, err := cal.Bytes() // or ical.NewEncoder(w).Encode(cal)
	if err != nil {
		panic(err)
	}
	// The encoding has CRLF line endings, folded at 75 octets.
	fmt.Print(strings.ReplaceAll(string(b), "\r\n", "\n"))
	// Output:
	// BEGIN:VCALENDAR
	// PRODID:-//Example//EN
	// VERSION:2.0
	// BEGIN:VEVENT
	// UID:launch@example.com
	// DTSTAMP:20240101T000000Z
	// SUMMARY:Launch\, finally
	// DTSTART;VALUE=DATE:20240603
	// END:VEVENT
	// END:VCALENDAR
}

func ExampleDecoder() {
	in := "BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:x\nBEGIN:VEVENT\nUID:1\nBAD LINE\nEND:VEVENT\n"
	d := ical.NewDecoder(strings.NewReader(in))
	d.Warn = func(err error) { fmt.Println("warning:", err) }
	_, err := d.Decode()
	fmt.Println("error:", err)
	// Output:
	// warning: ical: line 6: BAD: ' ' where ';' or ':' should be
	// error: ical: line 7: input ends inside VCALENDAR (BEGIN on line 1)
}

func ExampleExpander() {
	cal, _ := ical.Parse(strings.NewReader(feed))
	// One cache for the whole process: it remembers checkpoints and ends
	// of COUNT rules across requests.
	x := ical.Expander{Cache: recur.NewCache(0)}
	from := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	n := 0
	for _, err := range x.Occurrences(cal, from, from.AddDate(1, 0, 0)) {
		if err == nil {
			n++
		}
	}
	fmt.Println(n, "standups in 2030")
	// Output:
	// 104 standups in 2030
}

func ExampleCalendar_AddTimezones() {
	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		panic(err)
	}
	cal := ical.NewCalendar("-//Example//EN")
	ev := ical.NewEvent("standup@example.com", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev.SetSummary("Standup")
	ev.SetStart(ical.NewDateTime(time.Date(2026, 3, 2, 9, 0, 0, 0, chicago)))
	ev.SetDuration(ical.Duration{Minutes: 15})
	ev.SetRule(recur.Rule{Freq: recur.Weekly})
	cal.Add(ev.Component)
	// RFC 5545 requires a VTIMEZONE for each TZID a calendar uses. With it,
	// readers keep the standup at 09:00 in Chicago through DST changes.
	if err := cal.AddTimezones(nil); err != nil {
		panic(err)
	}
	b, err := cal.Bytes()
	if err != nil {
		panic(err)
	}
	fmt.Print(strings.ReplaceAll(string(b), "\r\n", "\n"))
	// With the 2026 zone data it prints the following. (The output isn't
	// checked: it follows the host's zone data, which a change to US
	// daylight saving time would change.)
	//
	// BEGIN:VCALENDAR
	// PRODID:-//Example//EN
	// VERSION:2.0
	// BEGIN:VTIMEZONE
	// TZID:America/Chicago
	// BEGIN:DAYLIGHT
	// DTSTART:20070311T020000
	// RRULE:FREQ=YEARLY;BYDAY=2SU;BYMONTH=3
	// TZOFFSETFROM:-0600
	// TZOFFSETTO:-0500
	// TZNAME:CDT
	// END:DAYLIGHT
	// BEGIN:STANDARD
	// DTSTART:20071104T020000
	// RRULE:FREQ=YEARLY;BYDAY=1SU;BYMONTH=11
	// TZOFFSETFROM:-0500
	// TZOFFSETTO:-0600
	// TZNAME:CST
	// END:STANDARD
	// END:VTIMEZONE
	// BEGIN:VEVENT
	// UID:standup@example.com
	// DTSTAMP:20260101T000000Z
	// SUMMARY:Standup
	// DTSTART;TZID=America/Chicago:20260302T090000
	// DURATION:PT15M
	// RRULE:FREQ=WEEKLY
	// END:VEVENT
	// END:VCALENDAR
}
