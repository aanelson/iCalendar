package ical_test

// Conformance tests for the typed views of VCALENDAR (RFC 5545 §3.6,
// §3.7), VEVENT (§3.6.1), VTODO (§3.6.2), VJOURNAL (§3.6.3) and VALARM
// (§3.6.6), and a survey of which of §3.6's and §3.8's REQUIRED / MUST NOT
// rules the library currently enforces. Expected values come from the RFC
// text and its examples.

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	ical "github.com/aanelson/iCalendar"
)

// ccNewYork is §3.6.5's third example: New York's rules since 2007.
const ccNewYork = `BEGIN:VTIMEZONE
TZID:America/New_York
BEGIN:STANDARD
DTSTART:20071104T020000
RRULE:FREQ=YEARLY;BYMONTH=11;BYDAY=1SU
TZOFFSETFROM:-0400
TZOFFSETTO:-0500
TZNAME:EST
END:STANDARD
BEGIN:DAYLIGHT
DTSTART:20070311T020000
RRULE:FREQ=YEARLY;BYMONTH=3;BYDAY=2SU
TZOFFSETFROM:-0500
TZOFFSETTO:-0400
TZNAME:EDT
END:DAYLIGHT
END:VTIMEZONE`

// ccValue describes a DATE or DATE-TIME as "<form> <value> <instant>":
// the form (DATE, floating, UTC or TZID=…), the value as written, and the
// instant in UTC, reading floating values on the UTC wall clock.
func ccValue(v ical.DateTime) string {
	form := "floating"
	switch {
	case v.IsDate:
		form = "DATE"
	case v.UTC:
		form = "UTC"
	case v.TZID != "":
		form = "TZID=" + v.TZID
	}
	return form + " " + v.String() + " " + v.In(time.UTC).UTC().Format(time.RFC3339)
}

// §3.6.1: "The DTSTART property for a VEVENT specifies the inclusive start
// of the event. ... The DTEND property for a VEVENT calendar component
// specifies the non-inclusive end of the event. For cases where a VEVENT
// calendar component specifies a DTSTART property with a DATE value type
// but no DTEND nor DURATION property, the event's duration is taken to be
// one day. For cases where a VEVENT calendar component specifies a DTSTART
// property with a DATE-TIME value type but no DTEND property, the event
// ends on the same calendar date and time of day specified by the DTSTART
// property." DURATION adds as §3.3.6 says: days on the wall clock.
func TestConformanceEventStartEnd(t *testing.T) {
	tests := []struct {
		name, props          string
		start, end, duration string
	}{{
		// §3.6.1's first example.
		"Annual Employee Review",
		"DTSTART:19970903T163000Z\nDTEND:19970903T190000Z",
		"UTC 19970903T163000Z 1997-09-03T16:30:00Z", "UTC 19970903T190000Z 1997-09-03T19:00:00Z", "PT2H30M",
	}, {
		// §3.6.1's second example, ending the next day.
		"Laurel",
		"DTSTART:19970401T163000Z\nDTEND:19970402T010000Z",
		"UTC 19970401T163000Z 1997-04-01T16:30:00Z", "UTC 19970402T010000Z 1997-04-02T01:00:00Z", "PT8H30M",
	}, {
		// §3.6.1's anniversary: a DATE and nothing else lasts one day.
		"anniversary",
		"DTSTART;VALUE=DATE:19971102",
		"DATE 19971102 1997-11-02T00:00:00Z", "DATE 19971103 1997-11-03T00:00:00Z", "P1D",
	}, {
		// §3.6.1's festival "from June 28th, 2007 to July 8th, 2007
		// inclusively": DTEND is the non-inclusive July 9.
		"festival",
		"DTSTART;VALUE=DATE:20070628\nDTEND;VALUE=DATE:20070709",
		"DATE 20070628 2007-06-28T00:00:00Z", "DATE 20070709 2007-07-09T00:00:00Z", "P11D",
	}, {
		"DATE-TIME alone ends at its start",
		"DTSTART:19970903T163000Z",
		"UTC 19970903T163000Z 1997-09-03T16:30:00Z", "UTC 19970903T163000Z 1997-09-03T16:30:00Z", "PT0S",
	}, {
		"zoned DATE-TIME alone ends at its start",
		"DTSTART;TZID=America/New_York:20240309T120000",
		"TZID=America/New_York 20240309T120000 2024-03-09T17:00:00Z", "TZID=America/New_York 20240309T120000 2024-03-09T17:00:00Z", "PT0S",
	}, {
		"floating DTEND",
		"DTSTART:20240101T090000\nDTEND:20240101T103000",
		"floating 20240101T090000 2024-01-01T09:00:00Z", "floating 20240101T103000 2024-01-01T10:30:00Z", "PT1H30M",
	}, {
		// §3.3.6/§3.8.5.3: P1D is nominal, so it keeps 12:00 across the
		// 2024-03-10 change to EDT (23 hours).
		"nominal DURATION across DST",
		"DTSTART;TZID=America/New_York:20240309T120000\nDURATION:P1D",
		"TZID=America/New_York 20240309T120000 2024-03-09T17:00:00Z", "TZID=America/New_York 20240310T120000 2024-03-10T16:00:00Z", "P1D",
	}, {
		// PT24H is exact: 24 hours later is 13:00 EDT.
		"exact DURATION across DST",
		"DTSTART;TZID=America/New_York:20240309T120000\nDURATION:PT24H",
		"TZID=America/New_York 20240309T120000 2024-03-09T17:00:00Z", "TZID=America/New_York 20240310T130000 2024-03-10T17:00:00Z", "PT24H",
	}, {
		// §3.3.6: "the number of days MUST be added first, followed by the
		// number of hours": 01:30 EST + P1D is 01:30 EDT on Mar 11 (05:30Z),
		// then one hour. (Hours first would give 03:30 EDT on Mar 11.)
		"days before hours",
		"DTSTART;TZID=America/New_York:20240310T013000\nDURATION:P1DT1H",
		"TZID=America/New_York 20240310T013000 2024-03-10T06:30:00Z", "TZID=America/New_York 20240311T023000 2024-03-11T06:30:00Z", "P1DT1H",
	}, {
		// A DATE event's DURATION in weeks (§3.8.2.5 allows dur-week).
		"DATE with dur-week",
		"DTSTART;VALUE=DATE:20240101\nDURATION:P1W",
		"DATE 20240101 2024-01-01T00:00:00Z", "DATE 20240108 2024-01-08T00:00:00Z", "P1W",
	}, {
		// §3.6.5: "events, such as airline flights, that start in one time
		// zone and end in another". 09:00 EDT is 13:00Z; 21:00 BST is
		// 20:00Z: 7 hours.
		"flight across zones",
		"DTSTART;TZID=America/New_York:20240601T090000\nDTEND;TZID=Europe/London:20240601T210000",
		"TZID=America/New_York 20240601T090000 2024-06-01T13:00:00Z", "TZID=Europe/London 20240601T210000 2024-06-01T20:00:00Z", "PT7H",
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cal := ccCal(t, ccNewYork+`
BEGIN:VTIMEZONE
TZID:Europe/London
BEGIN:STANDARD
DTSTART:19961027T020000
RRULE:FREQ=YEARLY;BYMONTH=10;BYDAY=-1SU
TZOFFSETFROM:+0100
TZOFFSETTO:+0000
TZNAME:GMT
END:STANDARD
BEGIN:DAYLIGHT
DTSTART:19810329T010000
RRULE:FREQ=YEARLY;BYMONTH=3;BYDAY=-1SU
TZOFFSETFROM:+0000
TZOFFSETTO:+0100
TZNAME:BST
END:DAYLIGHT
END:VTIMEZONE
BEGIN:VEVENT
UID:e@example.com
DTSTAMP:19970901T130000Z
`+tt.props+`
END:VEVENT`)
			ev := ccFirstEvent(t, cal)
			start, err := ev.Start()
			if err != nil {
				t.Fatal(err)
			}
			end, err := ev.End()
			if err != nil {
				t.Fatal(err)
			}
			d, err := ev.Duration()
			if err != nil {
				t.Fatal(err)
			}
			if got := ccValue(start); got != tt.start {
				t.Errorf("Start  %s\nwant   %s", got, tt.start)
			}
			if got := ccValue(end); got != tt.end {
				t.Errorf("End    %s\nwant   %s", got, tt.end)
			}
			if got := d.String(); got != tt.duration {
				t.Errorf("Duration %s, want %s", got, tt.duration)
			}
		})
	}
}

// §3.6.1 through the Expander: DTEND is non-inclusive, a DATE event takes
// up its whole day, and a DATE-TIME event with neither DTEND nor DURATION
// takes no time (it is found in a window that it starts in).
func TestConformanceEventExpanderBounds(t *testing.T) {
	cal := ccCal(t, `BEGIN:VEVENT
UID:timed@example.com
DTSTAMP:19970901T130000Z
DTSTART:19970903T163000Z
DTEND:19970903T190000Z
END:VEVENT
BEGIN:VEVENT
UID:festival@example.com
DTSTAMP:20070423T123432Z
DTSTART;VALUE=DATE:20070628
DTEND;VALUE=DATE:20070709
END:VEVENT
BEGIN:VEVENT
UID:instant@example.com
DTSTAMP:19970901T130000Z
DTSTART:19970910T120000Z
END:VEVENT`)
	x := &ical.Expander{}
	tests := []struct {
		from, to string
		want     []string
	}{
		{"1997-09-03T19:00:00Z", "1997-09-03T20:00:00Z", nil}, // DTEND excluded
		{"1997-09-03T18:59:59Z", "1997-09-03T20:00:00Z", []string{"1997-09-03T16:30:00Z/1997-09-03T19:00:00Z"}},
		{"1997-09-03T15:00:00Z", "1997-09-03T16:30:00Z", nil}, // ends at DTSTART
		{"1997-09-03T15:00:00Z", "1997-09-03T16:30:01Z", []string{"1997-09-03T16:30:00Z/1997-09-03T19:00:00Z"}},
		// "June 28th, 2007 to July 8th, 2007 inclusively"
		{"2007-07-08T00:00:00Z", "2007-07-09T00:00:00Z", []string{"2007-06-28T00:00:00Z/2007-07-09T00:00:00Z"}},
		{"2007-07-09T00:00:00Z", "2007-07-10T00:00:00Z", nil},
		{"2007-06-27T00:00:00Z", "2007-06-28T00:00:00Z", nil},
		// A zero-length event: found when it starts in the window.
		{"1997-09-10T12:00:00Z", "1997-09-10T13:00:00Z", []string{"1997-09-10T12:00:00Z/1997-09-10T12:00:00Z"}},
		{"1997-09-10T11:00:00Z", "1997-09-10T12:00:00Z", nil},
	}
	for _, tt := range tests {
		got := ccSpans(t, x, cal, tt.from, tt.to)
		if !slices.Equal(got, tt.want) {
			t.Errorf("[%s, %s): %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

// §3.6.1's anniversary: "DTSTART;VALUE=DATE:19971102" with
// "RRULE:FREQ=YEARLY" occurs on November 2 every year, each occurrence a
// whole day, and stays a DATE.
func TestConformanceEventAnniversary(t *testing.T) {
	cal := ccCal(t, `BEGIN:VEVENT
UID:19970901T130000Z-123403@example.com
DTSTAMP:19970901T130000Z
DTSTART;VALUE=DATE:19971102
SUMMARY:Our Blissful Anniversary
TRANSP:TRANSPARENT
CLASS:CONFIDENTIAL
CATEGORIES:ANNIVERSARY,PERSONAL,SPECIAL OCCASION
RRULE:FREQ=YEARLY
END:VEVENT`)
	ev := ccFirstEvent(t, cal)
	if !ev.Transparent() || ev.Class() != "CONFIDENTIAL" {
		t.Errorf("Transparent %v Class %q", ev.Transparent(), ev.Class())
	}
	if got, want := ev.Categories(), []string{"ANNIVERSARY", "PERSONAL", "SPECIAL OCCASION"}; !slices.Equal(got, want) {
		t.Errorf("Categories %q, want %q", got, want)
	}
	occs := ccOccurrences(t, &ical.Expander{}, cal, "1997-01-01T00:00:00Z", "2001-01-01T00:00:00Z")
	var got []string
	for _, occ := range occs {
		if !occ.Start.IsDate || !occ.End.IsDate {
			t.Errorf("occurrence %v–%v isn't a DATE", occ.Start, occ.End)
		}
		got = append(got, occ.Start.String()+"/"+occ.End.String())
	}
	want := []string{"19971102/19971103", "19981102/19981103", "19991102/19991103", "20001102/20001103"}
	if !slices.Equal(got, want) {
		t.Errorf("occurrences %v, want %v", got, want)
	}
}

// §3.8.1.3: CLASS "If not specified in a component that allows this
// property, the default value is PUBLIC". §3.8.2.7: TRANSP's "Default
// value is OPAQUE". §3.8.1.9: PRIORITY's "Default is zero (i.e.,
// undefined)". §3.8.7.4: SEQUENCE's "Default is "0"". The typed views
// apply the RFC's defaults (architecture doc, "Typed views").
func TestConformanceItemDefaults(t *testing.T) {
	cal := ccCal(t, `BEGIN:VEVENT
UID:d@example.com
DTSTAMP:20240101T000000Z
DTSTART:20240101T090000Z
END:VEVENT
BEGIN:VEVENT
UID:x@example.com
DTSTAMP:20240101T000000Z
DTSTART:20240101T090000Z
CLASS:x-secret
TRANSP:OPAQUE
PRIORITY:0
END:VEVENT`)
	var evs []ical.Event
	for ev := range cal.Events() {
		evs = append(evs, ev)
	}
	ev := evs[0]
	if ev.Transparent() || ev.Priority() != 0 || ev.Sequence() != 0 {
		t.Errorf("Transparent %v Priority %d Sequence %d, want false 0 0", ev.Transparent(), ev.Priority(), ev.Sequence())
	}
	if got := ev.Class(); got != "PUBLIC" {
		t.Errorf("Class() with no CLASS = %q, want the default PUBLIC", got)
	}
	// §3.8.1.3: "Applications MUST treat x-name and iana-token values they
	// don't recognize the same way as they would the PRIVATE value." Class
	// returns the value for the application to apply that rule; it must
	// not turn it into PUBLIC.
	if got := evs[1].Class(); got == "PUBLIC" || got == "" {
		t.Errorf("Class() for CLASS:x-secret = %q", got)
	}
}

// §3.6.1: "DTEND and DURATION MUST NOT occur in the same eventprop".
// Not enforced: the typed view should report the conflict rather than
// pick one, since End and Duration disagree otherwise.
func TestConformanceEventDTEndWithDuration(t *testing.T) {
	cal := ccCal(t, `BEGIN:VEVENT
UID:both@example.com
DTSTAMP:20240101T000000Z
DTSTART:20240101T090000Z
DTEND:20240101T100000Z
DURATION:PT3H
END:VEVENT`)
	ev := ccFirstEvent(t, cal)
	end, errEnd := ev.End()
	d, errDur := ev.Duration()
	if errEnd != nil && errDur != nil {
		return
	}
	t.Logf("End %v (%v), Duration %v (%v)", end.Time(), errEnd, d, errDur)
	ccGap(t, "§3.6.1 DTEND with DURATION is accepted; End uses DTEND and Duration uses DURATION, with no error (schema validation is planned)")
}

// §3.6.1: "If such a VEVENT has a DTEND property, it MUST be specified
// as a DATE value also" (for a DATE DTSTART); §3.8.2.2: "The value type
// of this property MUST be the same as the DTSTART property, and its value
// MUST be later in time than the value of the DTSTART property.
// Furthermore, this property MUST be specified as a date with local time
// if and only if the DTSTART property is also specified as a date with
// local time." §3.8.2.5: "When the DURATION property relates to a DTSTART
// property that is specified as a DATE value, then the DURATION property
// MUST be specified as a dur-day or dur-week value."
func TestConformanceEventEndConstraints(t *testing.T) {
	tests := []struct{ name, props string }{
		{"DATE DTSTART, DATE-TIME DTEND", "DTSTART;VALUE=DATE:20240101\nDTEND:20240102T000000Z"},
		{"DATE-TIME DTSTART, DATE DTEND", "DTSTART:20240101T090000Z\nDTEND;VALUE=DATE:20240102"},
		{"DTEND before DTSTART", "DTSTART:20240101T090000Z\nDTEND:20240101T080000Z"},
		{"DTEND equal to DTSTART", "DTSTART:20240101T090000Z\nDTEND:20240101T090000Z"},
		{"floating DTSTART, UTC DTEND", "DTSTART:20240101T090000\nDTEND:20240101T100000Z"},
		{"zoned DTSTART, floating DTEND", "DTSTART;TZID=America/New_York:20240101T090000\nDTEND:20240101T100000"},
		{"DATE DTSTART, DURATION with a time", "DTSTART;VALUE=DATE:20240101\nDURATION:PT1H"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text := "BEGIN:VCALENDAR\nPRODID:-//conformance//EN\nVERSION:2.0\n" + ccNewYork +
				"\nBEGIN:VEVENT\nUID:c@example.com\nDTSTAMP:20240101T000000Z\n" + tt.props + "\nEND:VEVENT\nEND:VCALENDAR\n"
			how := ccEnforced(t, text)
			ev := ccFirstEvent(t, ccRaw(t, text))
			_, errEnd := ev.End()
			_, errDur := ev.Duration()
			if errEnd != nil || errDur != nil {
				how = strings.TrimPrefix(how+", typed view error", ", ")
			}
			if how != "" {
				t.Logf("enforced: %s", how)
				return
			}
			end, _ := ev.End()
			d, _ := ev.Duration()
			t.Logf("End %s, Duration %s", ccValue(end), d)
			ccGap(t, "§3.6.1/§3.8.2.2/§3.8.2.5 "+tt.name+" is accepted without error or warning (schema validation is planned)")
		})
	}
}

// §3.6.2: the RFC's two VTODO examples, and DUE from DTSTART + DURATION.
func TestConformanceTodo(t *testing.T) {
	cal := ccCal(t, `BEGIN:VTODO
UID:20070313T123432Z-456553@example.com
DTSTAMP:20070313T123432Z
DUE;VALUE=DATE:20070501
SUMMARY:Submit Quebec Income Tax Return for 2006
CLASS:CONFIDENTIAL
CATEGORIES:FAMILY,FINANCE
STATUS:NEEDS-ACTION
END:VTODO
BEGIN:VTODO
UID:20070514T103211Z-123404@example.com
DTSTAMP:20070514T103211Z
DTSTART:20070514T110000Z
DUE:20070709T130000Z
COMPLETED:20070707T100000Z
SUMMARY:Submit Revised Internet-Draft
PRIORITY:1
STATUS:NEEDS-ACTION
END:VTODO
BEGIN:VTODO
UID:dur@example.com
DTSTAMP:20070514T103211Z
DTSTART;TZID=America/New_York:20240309T120000
DURATION:P1DT2H
PERCENT-COMPLETE:39
END:VTODO
BEGIN:VTODO
UID:nostart@example.com
DTSTAMP:20070514T103211Z
DURATION:PT1H
END:VTODO
`+ccNewYork)
	var todos []ical.Todo
	for td := range cal.Todos() {
		todos = append(todos, td)
	}
	if len(todos) != 4 {
		t.Fatalf("%d VTODOs", len(todos))
	}

	// "needs to be completed before May 1st, 2007"
	due, err := todos[0].Due()
	if err != nil || ccValue(due) != "DATE 20070501 2007-05-01T00:00:00Z" {
		t.Errorf("Due %s, %v", ccValue(due), err)
	}
	if todos[0].Class() != "CONFIDENTIAL" || todos[0].Status() != "NEEDS-ACTION" {
		t.Errorf("Class %q Status %q", todos[0].Class(), todos[0].Status())
	}
	if _, err := todos[0].Start(); !errors.Is(err, ical.ErrNoProperty) {
		t.Errorf("Start of a VTODO with no DTSTART: %v, want ErrNoProperty", err)
	}

	// "due before 1:00 P.M. UTC on July 9th, 2007 and was completed on
	// July 7th, 2007 at 10:00 A.M. UTC"
	start, err1 := todos[1].Start()
	due, err2 := todos[1].Due()
	done, err3 := todos[1].Completed()
	if err := errors.Join(err1, err2, err3); err != nil {
		t.Fatal(err)
	}
	if ccValue(start) != "UTC 20070514T110000Z 2007-05-14T11:00:00Z" ||
		ccValue(due) != "UTC 20070709T130000Z 2007-07-09T13:00:00Z" ||
		ccValue(done) != "UTC 20070707T100000Z 2007-07-07T10:00:00Z" {
		t.Errorf("Start %s Due %s Completed %s", ccValue(start), ccValue(due), ccValue(done))
	}
	if todos[1].Priority() != 1 {
		t.Errorf("Priority %d", todos[1].Priority())
	}

	// Without DUE, the to-do lasts DURATION from DTSTART (§3.6.2: "Either
	// 'due' or 'duration' MAY appear"), added as §3.3.6 says.
	due, err = todos[2].Due()
	if err != nil || ccValue(due) != "TZID=America/New_York 20240310T140000 2024-03-10T18:00:00Z" {
		t.Errorf("Due %s, %v", ccValue(due), err)
	}
	if todos[2].PercentComplete() != 39 {
		t.Errorf("PercentComplete %d", todos[2].PercentComplete())
	}

	// §3.6.2: "If 'duration' appear in a 'todoprop', then 'dtstart' MUST
	// also appear". With no DTSTART there's no due time to compute, so
	// Due must fail rather than invent one.
	if due, err := todos[3].Due(); err == nil {
		t.Errorf("Due of a VTODO with DURATION and no DTSTART: %s, nil", ccValue(due))
	}
}

// §3.8.5.3: "If the duration of the recurring component is specified with
// the DTEND or DUE property, then the same exact duration will apply to
// all the members of the generated recurrence set." For a VTODO, DUE.
func TestConformanceTodoExpanderUsesDue(t *testing.T) {
	cal := ccCal(t, ccNewYork+`
BEGIN:VTODO
UID:weekly-todo@example.com
DTSTAMP:20240101T000000Z
DTSTART;TZID=America/New_York:20240305T090000
DUE;TZID=America/New_York:20240305T110000
RRULE:FREQ=WEEKLY;COUNT=2
END:VTODO`)
	x := &ical.Expander{Components: []string{"VTODO"}}
	got := ccSpans(t, x, cal, "2024-03-01T00:00:00Z", "2024-04-01T00:00:00Z")
	// 09:00 EST is 14:00Z; 09:00 EDT on Mar 12 is 13:00Z; two hours each.
	want := []string{"2024-03-05T14:00:00Z/2024-03-05T16:00:00Z", "2024-03-12T13:00:00Z/2024-03-12T15:00:00Z"}
	if !slices.Equal(got, want) {
		t.Errorf("occurrences %v, want %v", got, want)
	}
}

// §3.6.2: "Either 'due' or 'duration' MAY appear in a 'todoprop', but
// 'due' and 'duration' MUST NOT occur in the same 'todoprop'."
func TestConformanceTodoDueWithDuration(t *testing.T) {
	cal := ccCal(t, `BEGIN:VTODO
UID:both@example.com
DTSTAMP:20240101T000000Z
DTSTART:20240101T090000Z
DUE:20240101T100000Z
DURATION:PT3H
END:VTODO`)
	for td := range cal.Todos() {
		due, err := td.Due()
		if err != nil {
			return
		}
		t.Logf("Due %s", ccValue(due))
	}
	ccGap(t, "§3.6.2 DUE with DURATION is accepted; Due returns DUE with no error (schema validation is planned)")
}

// §3.6.3: the RFC's VJOURNAL example: "The DTSTART property is used to
// specify the calendar date with which the journal entry is associated."
func TestConformanceJournal(t *testing.T) {
	cal := ccCal(t, `BEGIN:VJOURNAL
UID:19970901T130000Z-123405@example.com
DTSTAMP:19970901T130000Z
DTSTART;VALUE=DATE:19970317
SUMMARY:Staff meeting minutes
DESCRIPTION:1. Staff meeting: Participants include Joe\,
  Lisa\, and Bob. Aurora project plans were reviewed.
  There is currently no budget reserves for this project.
  Lisa will escalate to management. Next meeting on Tuesday.\n
 2. Telephone Conference: ABC Corp. sales representative
  called to discuss new printer. Promised to get us a demo by
  Friday.\n3. Henry Miller (Handsoff Insurance): Car was
  totaled by tree. Is looking into a loaner car. 555-2323
  (tel).
END:VJOURNAL`)
	var n int
	for j := range cal.Journals() {
		n++
		start, err := j.Start()
		if err != nil || ccValue(start) != "DATE 19970317 1997-03-17T00:00:00Z" {
			t.Errorf("Start %s, %v", ccValue(start), err)
		}
		if j.Summary() != "Staff meeting minutes" {
			t.Errorf("Summary %q", j.Summary())
		}
		if d := j.Description(); !strings.HasPrefix(d, "1. Staff meeting: Participants include Joe, Lisa, and Bob.") ||
			!strings.Contains(d, "Tuesday.\n2. Telephone Conference") {
			t.Errorf("Description %q", d)
		}
	}
	if n != 1 {
		t.Fatalf("%d VJOURNALs", n)
	}
	x := &ical.Expander{Components: []string{"VJOURNAL"}}
	occs := ccOccurrences(t, x, cal, "1997-03-01T00:00:00Z", "1997-04-01T00:00:00Z")
	if len(occs) != 1 || ccValue(occs[0].Start) != "DATE 19970317 1997-03-17T00:00:00Z" {
		t.Errorf("occurrences %+v", occs)
	}
}

// §3.6.6 and §3.8.6.3: the RFC's three VALARM examples, the RELATED
// parameter, and erratum 2039's TRIGGER;VALUE=DATE-TIME. "The default
// value type is DURATION ... The default duration is relative to the start
// of an event or to-do"; "RELATED=END" makes it relative to the end;
// "Property parameter values that are not in quoted-strings are
// case-insensitive" (§3.2).
func TestConformanceAlarmTrigger(t *testing.T) {
	cal := ccCal(t, `BEGIN:VEVENT
UID:alarms@example.com
DTSTAMP:19970901T130000Z
DTSTART:19970317T150000Z
BEGIN:VALARM
TRIGGER;VALUE=DATE-TIME:19970317T133000Z
REPEAT:4
DURATION:PT15M
ACTION:AUDIO
ATTACH;FMTTYPE=audio/basic:ftp://example.com/pub/
 sounds/bell-01.aud
END:VALARM
BEGIN:VALARM
TRIGGER:-PT30M
REPEAT:2
DURATION:PT15M
ACTION:DISPLAY
DESCRIPTION:Breakfast meeting with executive\n
 team at 8:30 AM EST.
END:VALARM
BEGIN:VALARM
TRIGGER;RELATED=END:-P2D
ACTION:EMAIL
ATTENDEE:mailto:john_doe@example.com
SUMMARY:*** REMINDER: SEND AGENDA FOR WEEKLY STAFF MEETING ***
DESCRIPTION:A draft agenda needs to be sent out to the attendees
  to the weekly managers meeting (MGR-LIST). Attached is a
  pointer the document template for the agenda file.
ATTACH;FMTTYPE=application/msword:http://example.com/
 templates/agenda.doc
END:VALARM
BEGIN:VALARM
ACTION:DISPLAY
DESCRIPTION:start
TRIGGER;RELATED=START:-PT15M
END:VALARM
BEGIN:VALARM
ACTION:DISPLAY
DESCRIPTION:end
TRIGGER;related=end:PT5M
END:VALARM
BEGIN:VALARM
ACTION:DISPLAY
DESCRIPTION:explicit
TRIGGER;VALUE=DURATION:PT0S
END:VALARM
BEGIN:VALARM
ACTION:DISPLAY
DESCRIPTION:no trigger
END:VALARM
END:VEVENT`)
	type trig struct {
		action, rel, abs string
		toEnd, err       bool
	}
	want := []trig{
		{"AUDIO", "PT0S", "UTC 19970317T133000Z 1997-03-17T13:30:00Z", false, false},
		{"DISPLAY", "-PT30M", "", false, false},
		{"EMAIL", "-P2D", "", true, false},
		{"DISPLAY", "-PT15M", "", false, false},
		{"DISPLAY", "PT5M", "", true, false},
		{"DISPLAY", "PT0S", "", false, false},
		// §3.6.6: "The VALARM calendar component MUST include the ACTION
		// and TRIGGER properties": with none there is no trigger to
		// return, and no default.
		{"DISPLAY", "", "", false, true},
	}
	var got []trig
	for a := range ccFirstEvent(t, cal).Alarms() {
		rel, abs, toEnd, err := a.Trigger()
		g := trig{action: a.Action(), toEnd: toEnd, err: err != nil}
		if err == nil {
			g.rel = rel.String()
			if !abs.IsZero() {
				g.abs = ccValue(abs)
			}
		}
		got = append(got, g)
	}
	if !slices.Equal(got, want) {
		t.Errorf("alarms\n%+v\nwant\n%+v", got, want)
	}
}

// §4's to-do: "An audio alarm has been specified to remind the calendar
// user at noon, the day before the to-do is expected to be completed and
// repeat hourly, four additional times." (The TRIGGER says April 3, not
// April 14, so the prose and the value disagree; the value is checked.)
// Erratum 2039 adds VALUE=DATE-TIME, because TRIGGER's default type is
// DURATION. The printed form, TRIGGER:19980403T120000Z, is not a valid
// DURATION: a reader may reject it or read it as the DATE-TIME it looks
// like (erratum 2039), but must not take it as a relative trigger.
func TestConformanceExampleTodoAlarm(t *testing.T) {
	for _, name := range []string{"rfc5545-4-d-corrected", "rfc5545-4-d"} {
		t.Run(name, func(t *testing.T) {
			cal := ccFixture(t, name)
			var n int
			for td := range cal.Todos() {
				n++
				due, err := td.Due()
				if err != nil || ccValue(due) != "floating 19980415T000000 1998-04-15T00:00:00Z" {
					t.Errorf("Due %s, %v", ccValue(due), err)
				}
				if td.Sequence() != 2 || td.Status() != "NEEDS-ACTION" || td.Summary() != "Submit Income Taxes" {
					t.Errorf("Sequence %d Status %q Summary %q", td.Sequence(), td.Status(), td.Summary())
				}
				for a := range td.Alarms() {
					if a.Action() != "AUDIO" {
						t.Errorf("Action %q", a.Action())
					}
					if a.Get("REPEAT") == nil || a.Get("REPEAT").Value != "4" || a.Get("DURATION") == nil || a.Get("DURATION").Value != "PT1H" {
						t.Errorf("REPEAT/DURATION missing")
					}
					rel, abs, toEnd, err := a.Trigger()
					switch {
					case err == nil && !abs.IsZero() && ccValue(abs) == "UTC 19980403T120000Z 1998-04-03T12:00:00Z" && !toEnd:
					case err != nil && name == "rfc5545-4-d":
						t.Logf("the uncorrected TRIGGER is rejected: %v", err)
					default:
						t.Errorf("Trigger: rel %s abs %+v toEnd %v err %v", rel, abs, toEnd, err)
					}
				}
			}
			if n != 1 {
				t.Errorf("%d VTODOs", n)
			}
		})
	}
}

// §3.6, §3.7: "An iCalendar object MUST include the PRODID and VERSION
// calendar properties"; VERSION "2.0" corresponds to RFC 5545; METHOD is
// optional ("If this property is not present in the iCalendar object, then
// a scheduling transaction MUST NOT be assumed").
func TestConformanceCalendarProperties(t *testing.T) {
	cal := ical.NewCalendar("-//ABC Corporation//NONSGML My Product//EN")
	ev := ical.NewEvent("uid3@example.com", time.Date(1997, 3, 24, 12, 0, 0, 0, time.UTC))
	ev.SetStart(ical.NewDateTime(time.Date(1997, 3, 24, 12, 30, 0, 0, time.UTC)))
	cal.Add(ev.Component)
	b, err := cal.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(b, []byte("\r\nPRODID:")) != 1 || bytes.Count(b, []byte("\r\nVERSION:2.0\r\n")) != 1 {
		t.Errorf("want exactly one PRODID and one VERSION:2.0:\n%s", b)
	}
	if !bytes.Contains(b, []byte("\r\nUID:uid3@example.com\r\n")) || !bytes.Contains(b, []byte("\r\nDTSTAMP:19970324T120000Z\r\n")) {
		t.Errorf("NewEvent lacks UID or a UTC DTSTAMP (§3.8.7.2: \"The value MUST be specified in the UTC time format\"):\n%s", b)
	}
	back := ccRaw(t, string(b))
	if back.ProdID() != "-//ABC Corporation//NONSGML My Product//EN" || back.Version() != "2.0" || back.Method() != "" {
		t.Errorf("ProdID %q Version %q Method %q", back.ProdID(), back.Version(), back.Method())
	}

	// §4's MIME example carries METHOD:xyz.
	c := ccFixture(t, "rfc5545-4-c")
	if c.Method() != "xyz" || c.Version() != "2.0" || c.ProdID() != "-//ABC Corporation//NONSGML My Product//EN" {
		t.Errorf("ProdID %q Version %q Method %q", c.ProdID(), c.Version(), c.Method())
	}
}

// ccGap records an RFC requirement the library doesn't meet yet, for a
// reason the architecture doc already lists as planned, as a skipped
// subtest: a top-level skip still means only that a fixture suite's
// testdata is missing, which CI checks for.
func ccGap(t *testing.T, why string) {
	t.Helper()
	t.Run("KNOWN_GAP", func(t *testing.T) { t.Skip("KNOWN GAP: " + why) })
}

// ccEnforced reports how decoding treats text: "error" when strict
// decoding fails, "Warn" when lenient decoding reports a recovery, both,
// or "" when neither notices anything.
func ccEnforced(t *testing.T, text string) string {
	t.Helper()
	d := ical.NewDecoder(strings.NewReader(ccCRLF(text)))
	d.Strict = true
	_, strictErr := d.Decode()
	var warns []string
	d = ical.NewDecoder(strings.NewReader(ccCRLF(text)))
	d.Warn = func(err error) { warns = append(warns, err.Error()) }
	_, lenientErr := d.Decode()
	var how []string
	if strictErr != nil {
		how = append(how, "error in strict mode ("+strictErr.Error()+")")
	}
	if lenientErr != nil {
		how = append(how, "error in lenient mode ("+lenientErr.Error()+")")
	}
	if len(warns) > 0 {
		how = append(how, "Warn in lenient mode ("+strings.Join(warns, "; ")+")")
	}
	return strings.Join(how, ", ")
}

// TestConformanceCardinality surveys the REQUIRED, "MUST NOT occur more
// than once" and value rules of §3.6 and §3.8 for the main components:
// each case is a valid calendar with one rule broken. A rule is enforced
// when strict decoding fails or lenient decoding warns; the architecture
// doc marks schema validation as planned, so the rest are known gaps.
func TestConformanceCardinality(t *testing.T) {
	const base = `BEGIN:VCALENDAR
PRODID:-//conformance//EN
VERSION:2.0
BEGIN:VEVENT
UID:e@example.com
DTSTAMP:20240101T000000Z
DTSTART:20240101T090000Z
DTEND:20240101T100000Z
END:VEVENT
BEGIN:VTODO
UID:t@example.com
DTSTAMP:20240101T000000Z
DTSTART:20240101T090000Z
DUE:20240101T100000Z
END:VTODO
BEGIN:VJOURNAL
UID:j@example.com
DTSTAMP:20240101T000000Z
DTSTART;VALUE=DATE:20240101
END:VJOURNAL
BEGIN:VTIMEZONE
TZID:Zone
BEGIN:STANDARD
DTSTART:19700101T000000
TZOFFSETFROM:+0100
TZOFFSETTO:+0100
END:STANDARD
END:VTIMEZONE
END:VCALENDAR
`
	if how := ccEnforced(t, base); how != "" {
		t.Fatalf("the valid base calendar is rejected: %s", how)
	}
	// edit returns base with old replaced by new in the named component
	// (the first occurrence after its BEGIN line).
	edit := func(comp, old, new string) string {
		i := strings.Index(base, "BEGIN:"+comp+"\n")
		j := strings.Index(base[i:], old)
		if i < 0 || j < 0 {
			panic(comp + ": " + old)
		}
		return base[:i+j] + new + base[i+j+len(old):]
	}
	add := func(comp, lines string) string {
		return edit(comp, "BEGIN:"+comp+"\n", "BEGIN:"+comp+"\n"+lines+"\n")
	}
	drop := func(comp, line string) string { return edit(comp, line+"\n", "") }
	twice := func(comp, line string) string { return add(comp, line+"\n"+line) }
	alarm := func(lines string) string {
		return edit("VEVENT", "END:VEVENT\n", "BEGIN:VALARM\n"+lines+"\nEND:VALARM\nEND:VEVENT\n")
	}
	type rule struct{ section, name, text string }
	rules := []rule{
		{"§3.6", "VCALENDAR without PRODID", drop("VCALENDAR", "PRODID:-//conformance//EN")},
		{"§3.6", "VCALENDAR without VERSION", drop("VCALENDAR", "VERSION:2.0")},
		{"§3.6", "PRODID twice", twice("VCALENDAR", "PRODID:-//other//EN")},
		{"§3.6", "VERSION twice", twice("VCALENDAR", "VERSION:2.0")},
		{"§3.6", "CALSCALE twice", twice("VCALENDAR", "CALSCALE:GREGORIAN")},
		{"§3.6", "METHOD twice", twice("VCALENDAR", "METHOD:PUBLISH")},
		{"§3.6", "VCALENDAR without components", "BEGIN:VCALENDAR\nPRODID:-//conformance//EN\nVERSION:2.0\nEND:VCALENDAR\n"},
		{"§3.7.1", "CALSCALE other than GREGORIAN", add("VCALENDAR", "CALSCALE:JULIAN")},
		{"§3.7.4", "VERSION other than 2.0", edit("VCALENDAR", "VERSION:2.0", "VERSION:1.0")},
		{"§3.7", "calendar property inside a component", add("VEVENT", "METHOD:PUBLISH")},

		{"§3.6.1", "VEVENT without UID", drop("VEVENT", "UID:e@example.com")},
		{"§3.6.1", "VEVENT without DTSTAMP", drop("VEVENT", "DTSTAMP:20240101T000000Z")},
		{"§3.6.1", "VEVENT without DTSTART (no METHOD)", drop("VEVENT", "DTSTART:20240101T090000Z")},
		{"§3.6.1", "VEVENT with DTEND and DURATION", add("VEVENT", "DURATION:PT1H")},
		{"§3.8.2.2", "VEVENT DTEND before DTSTART", edit("VEVENT", "DTEND:20240101T100000Z", "DTEND:20240101T080000Z")},
		{"§3.8.2.2", "VEVENT DTEND type differs from DTSTART", edit("VEVENT", "DTEND:20240101T100000Z", "DTEND;VALUE=DATE:20240102")},
		{"§3.8.2.5", "VEVENT DATE DTSTART with a time DURATION", edit("VEVENT", "DTSTART:20240101T090000Z\nDTEND:20240101T100000Z", "DTSTART;VALUE=DATE:20240101\nDURATION:PT1H")},
		{"§3.8.7.2", "VEVENT DTSTAMP not in UTC", edit("VEVENT", "DTSTAMP:20240101T000000Z", "DTSTAMP:20240101T000000")},
		{"§3.8.7.1", "VEVENT CREATED not in UTC", add("VEVENT", "CREATED:20240101T000000")},
		{"§3.8.7.3", "VEVENT LAST-MODIFIED not in UTC", add("VEVENT", "LAST-MODIFIED:20240101T000000")},
		{"§3.8.1.9", "VEVENT PRIORITY 10", add("VEVENT", "PRIORITY:10")},
		{"§3.8.1.11", "VEVENT STATUS:COMPLETED (a VTODO status)", add("VEVENT", "STATUS:COMPLETED")},
		{"§3.8.2.7", "VEVENT TRANSP:MAYBE", add("VEVENT", "TRANSP:MAYBE")},

		{"§3.6.2", "VTODO without UID", drop("VTODO", "UID:t@example.com")},
		{"§3.6.2", "VTODO without DTSTAMP", drop("VTODO", "DTSTAMP:20240101T000000Z")},
		{"§3.6.2", "VTODO with DUE and DURATION", add("VTODO", "DURATION:PT1H")},
		{"§3.6.2", "VTODO DURATION without DTSTART", edit("VTODO", "DTSTART:20240101T090000Z\nDUE:20240101T100000Z", "DURATION:PT1H")},
		{"§3.8.2.3", "VTODO DUE before DTSTART", edit("VTODO", "DUE:20240101T100000Z", "DUE:20240101T080000Z")},
		{"§3.8.2.3", "VTODO DUE type differs from DTSTART", edit("VTODO", "DUE:20240101T100000Z", "DUE;VALUE=DATE:20240102")},
		{"§3.8.2.1", "VTODO COMPLETED not in UTC", add("VTODO", "COMPLETED:20240101T000000")},
		{"§3.8.1.8", "VTODO PERCENT-COMPLETE 101", add("VTODO", "PERCENT-COMPLETE:101")},
		{"§3.8.1.8", "VTODO PERCENT-COMPLETE -1", add("VTODO", "PERCENT-COMPLETE:-1")},
		{"§3.8.1.11", "VTODO STATUS:TENTATIVE (a VEVENT status)", add("VTODO", "STATUS:TENTATIVE")},

		{"§3.6.3", "VJOURNAL without UID", drop("VJOURNAL", "UID:j@example.com")},
		{"§3.6.3", "VJOURNAL without DTSTAMP", drop("VJOURNAL", "DTSTAMP:20240101T000000Z")},
		{"§3.8.1.11", "VJOURNAL STATUS:CONFIRMED (a VEVENT status)", add("VJOURNAL", "STATUS:CONFIRMED")},
		{"§3.8.2.2", "VJOURNAL with DTEND (not a jourprop)", add("VJOURNAL", "DTEND;VALUE=DATE:20240102")},

		{"§3.6.5", "VTIMEZONE without TZID", drop("VTIMEZONE", "TZID:Zone")},
		{"§3.6.5", "VTIMEZONE TZID twice", twice("VTIMEZONE", "TZID:Zone")},
		{"§3.6.5", "VTIMEZONE LAST-MODIFIED twice", twice("VTIMEZONE", "LAST-MODIFIED:20240101T000000Z")},
		{"§3.6.5", "VTIMEZONE TZURL twice", twice("VTIMEZONE", "TZURL:http://example.com/zone.ics")},
		{"§3.6.5", "VTIMEZONE without STANDARD or DAYLIGHT", edit("VTIMEZONE", "BEGIN:STANDARD\nDTSTART:19700101T000000\nTZOFFSETFROM:+0100\nTZOFFSETTO:+0100\nEND:STANDARD\n", "")},
		{"§3.6.5", "STANDARD without DTSTART", drop("VTIMEZONE", "DTSTART:19700101T000000")},
		{"§3.6.5", "STANDARD without TZOFFSETFROM", drop("VTIMEZONE", "TZOFFSETFROM:+0100")},
		{"§3.6.5", "STANDARD without TZOFFSETTO", drop("VTIMEZONE", "TZOFFSETTO:+0100")},
		{"§3.6.5", "STANDARD TZOFFSETTO twice", edit("VTIMEZONE", "TZOFFSETTO:+0100", "TZOFFSETTO:+0100\nTZOFFSETTO:+0200")},
		{"§3.6.5", "STANDARD DTSTART in UTC", edit("VTIMEZONE", "DTSTART:19700101T000000", "DTSTART:19700101T000000Z")},
		{"§3.8.2.4", "STANDARD DTSTART with TZID", edit("VTIMEZONE", "DTSTART:19700101T000000", "DTSTART;TZID=Zone:19700101T000000")},
		{"§3.6.5", "STANDARD RRULE with a floating UNTIL", edit("VTIMEZONE", "TZOFFSETTO:+0100", "TZOFFSETTO:+0100\nRRULE:FREQ=YEARLY;UNTIL=19800101T000000")},
		{"§3.6.5", "two VTIMEZONEs with one TZID", edit("VCALENDAR", "END:VCALENDAR", "BEGIN:VTIMEZONE\nTZID:Zone\nBEGIN:STANDARD\nDTSTART:19700101T000000\nTZOFFSETFROM:+0200\nTZOFFSETTO:+0200\nEND:STANDARD\nEND:VTIMEZONE\nEND:VCALENDAR")},
		{"§3.2.19", "TZID with no VTIMEZONE", edit("VEVENT", "DTSTART:20240101T090000Z\nDTEND:20240101T100000Z", "DTSTART;TZID=America/New_York:20240101T090000")},
		{"§3.2.19", "TZID on a DATE", edit("VJOURNAL", "DTSTART;VALUE=DATE:20240101", "DTSTART;VALUE=DATE;TZID=Zone:20240101")},
		{"§3.3.5", "TZID on a UTC time", edit("VEVENT", "DTSTART:20240101T090000Z", "DTSTART;TZID=Zone:20240101T090000Z")},

		{"§3.6.6", "VALARM without ACTION", alarm("TRIGGER:-PT15M\nDESCRIPTION:x")},
		{"§3.6.6", "VALARM without TRIGGER", alarm("ACTION:DISPLAY\nDESCRIPTION:x")},
		{"§3.6.6", "VALARM ACTION twice", alarm("ACTION:DISPLAY\nACTION:AUDIO\nTRIGGER:-PT15M\nDESCRIPTION:x")},
		{"§3.6.6", "VALARM TRIGGER twice", alarm("ACTION:DISPLAY\nTRIGGER:-PT15M\nTRIGGER:-PT5M\nDESCRIPTION:x")},
		{"§3.6.6", "DISPLAY VALARM without DESCRIPTION", alarm("ACTION:DISPLAY\nTRIGGER:-PT15M")},
		{"§3.6.6", "EMAIL VALARM without SUMMARY", alarm("ACTION:EMAIL\nTRIGGER:-PT15M\nDESCRIPTION:x\nATTENDEE:mailto:a@example.com")},
		{"§3.6.6", "EMAIL VALARM without ATTENDEE", alarm("ACTION:EMAIL\nTRIGGER:-PT15M\nDESCRIPTION:x\nSUMMARY:s")},
		{"§3.6.6", "EMAIL VALARM without DESCRIPTION", alarm("ACTION:EMAIL\nTRIGGER:-PT15M\nSUMMARY:s\nATTENDEE:mailto:a@example.com")},
		{"§3.6.6", "VALARM REPEAT without DURATION", alarm("ACTION:DISPLAY\nTRIGGER:-PT15M\nDESCRIPTION:x\nREPEAT:2")},
		{"§3.6.6", "VALARM DURATION without REPEAT", alarm("ACTION:DISPLAY\nTRIGGER:-PT15M\nDESCRIPTION:x\nDURATION:PT5M")},
		{"§3.6.6", "AUDIO VALARM ATTACH twice", alarm("ACTION:AUDIO\nTRIGGER:-PT15M\nATTACH:http://example.com/a.aud\nATTACH:http://example.com/b.aud")},
		{"§3.8.6.3", "TRIGGER RELATED with VALUE=DATE-TIME", alarm("ACTION:DISPLAY\nTRIGGER;VALUE=DATE-TIME;RELATED=END:20240101T080000Z\nDESCRIPTION:x")},
		{"§3.8.6.3", "absolute TRIGGER not in UTC", alarm("ACTION:DISPLAY\nTRIGGER;VALUE=DATE-TIME:20240101T080000\nDESCRIPTION:x")},
		{"§3.6.6", "VALARM in a VJOURNAL", edit("VJOURNAL", "END:VJOURNAL", "BEGIN:VALARM\nACTION:DISPLAY\nTRIGGER:-PT15M\nDESCRIPTION:x\nEND:VALARM\nEND:VJOURNAL")},
		{"§3.6.1", "VEVENT nested in a VEVENT", edit("VEVENT", "END:VEVENT", "BEGIN:VEVENT\nUID:n@example.com\nDTSTAMP:20240101T000000Z\nDTSTART:20240101T090000Z\nEND:VEVENT\nEND:VEVENT")},
	}
	// Each property §3.6.1-§3.6.3 says "MUST NOT occur more than once".
	once := map[string][]string{
		"VEVENT": {"UID:e@example.com", "DTSTAMP:20240101T000000Z", "DTSTART:20240101T090000Z",
			"CLASS:PUBLIC", "CREATED:20240101T000000Z", "DESCRIPTION:x", "GEO:37.386013;-122.082932",
			"LAST-MODIFIED:20240101T000000Z", "LOCATION:x", "ORGANIZER:mailto:a@example.com",
			"PRIORITY:1", "SEQUENCE:1", "STATUS:CONFIRMED", "SUMMARY:x", "TRANSP:OPAQUE",
			"URL:http://example.com/", "RECURRENCE-ID:20240101T090000Z", "DTEND:20240101T100000Z"},
		"VTODO": {"UID:t@example.com", "DTSTAMP:20240101T000000Z", "CLASS:PUBLIC",
			"COMPLETED:20240101T000000Z", "CREATED:20240101T000000Z", "DESCRIPTION:x",
			"DTSTART:20240101T090000Z", "GEO:37.386013;-122.082932", "LAST-MODIFIED:20240101T000000Z",
			"LOCATION:x", "ORGANIZER:mailto:a@example.com", "PERCENT-COMPLETE:10", "PRIORITY:1",
			"RECURRENCE-ID:20240101T090000Z", "SEQUENCE:1", "STATUS:NEEDS-ACTION", "SUMMARY:x",
			"URL:http://example.com/", "DUE:20240101T100000Z"},
		"VJOURNAL": {"UID:j@example.com", "DTSTAMP:20240101T000000Z", "CLASS:PUBLIC",
			"CREATED:20240101T000000Z", "DTSTART;VALUE=DATE:20240101", "LAST-MODIFIED:20240101T000000Z",
			"ORGANIZER:mailto:a@example.com", "RECURRENCE-ID;VALUE=DATE:20240101", "SEQUENCE:1",
			"STATUS:DRAFT", "SUMMARY:x", "URL:http://example.com/"},
	}
	section := map[string]string{"VEVENT": "§3.6.1", "VTODO": "§3.6.2", "VJOURNAL": "§3.6.3"}
	for _, comp := range []string{"VEVENT", "VTODO", "VJOURNAL"} {
		i := strings.Index(base, "BEGIN:"+comp+"\n")
		body := base[i : i+strings.Index(base[i:], "END:"+comp)]
		for _, line := range once[comp] {
			name, _, _ := strings.Cut(line, ":")
			name, _, _ = strings.Cut(name, ";")
			text := twice(comp, line)
			if strings.Contains(body, "\n"+name+":") || strings.Contains(body, "\n"+name+";") {
				text = add(comp, line) // already there once
			}
			rules = append(rules, rule{section[comp], comp + " " + name + " twice", text})
		}
	}
	for _, r := range rules {
		t.Run(r.section+" "+r.name, func(t *testing.T) {
			if how := ccEnforced(t, r.text); how != "" {
				t.Logf("enforced: %s", how)
				return
			}
			ccGap(t, r.section+" "+r.name+": accepted without error or warning (schema validation is planned)")
		})
	}
}
