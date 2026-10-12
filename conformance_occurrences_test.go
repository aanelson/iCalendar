package ical_test

// Conformance tests for the Expander: RECURRENCE-ID overrides (RFC 5545
// §3.8.4.4), instance durations (§3.8.5.3), RDATE and EXDATE (§3.8.5.1,
// §3.8.5.2), DST gaps and repeats (§3.3.5, erratum 4271), and the §4
// examples. Expected instants are worked out by hand from the RFC text;
// America/New_York is the calendar's own VTIMEZONE (§3.6.5's example of
// the rules since 2007): EST is UTC-5, EDT UTC-4, changing on 2024-03-10
// and 2024-11-03 at 02:00.

import (
	"errors"
	"slices"
	"testing"

	ical "github.com/aanelson/iCalendar"
	"github.com/aanelson/iCalendar/tz"
)

// ccOccs describes occurrences as "start/end rid=recurrence-id" in UTC.
func ccOccs(t *testing.T, x *ical.Expander, cal *ical.Calendar, from, to string) []string {
	t.Helper()
	var s []string
	for _, occ := range ccOccurrences(t, x, cal, from, to) {
		s = append(s, ccInstant(x, occ.Start)+"/"+ccInstant(x, occ.End)+" rid="+ccInstant(x, occ.RecurrenceID))
	}
	return s
}

func ccWant(t *testing.T, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("occurrences:\n got %q\nwant %q", got, want)
	}
}

// §3.8.4.4: "The property value is the original value of the DTSTART
// property of the recurrence instance. ... if the intent is to change a
// Friday meeting to Thursday, the DATE-TIME is still set to the original
// Friday meeting." The override replaces that instance and is reported
// where its own DTSTART (and DTEND) put it; its RecurrenceID still names
// the Friday. A RECURRENCE-ID written in UTC (allowed: the form must be
// floating iff DTSTART's is) names the same instance as an instant.
func TestConformanceRecurrenceIDOverride(t *testing.T) {
	for _, rid := range []string{
		"RECURRENCE-ID;TZID=America/New_York:20240112T090000",
		"RECURRENCE-ID:20240112T140000Z",
	} {
		t.Run(rid, func(t *testing.T) {
			cal := ccCal(t, ccNewYork+`
BEGIN:VEVENT
UID:weekly@example.com
DTSTAMP:20240101T000000Z
DTSTART;TZID=America/New_York:20240105T090000
DTEND;TZID=America/New_York:20240105T100000
RRULE:FREQ=WEEKLY;COUNT=4
SUMMARY:Friday meeting
END:VEVENT
BEGIN:VEVENT
UID:weekly@example.com
DTSTAMP:20240101T000000Z
`+rid+`
DTSTART;TZID=America/New_York:20240111T100000
DTEND;TZID=America/New_York:20240111T113000
SUMMARY:Moved to Thursday
END:VEVENT`)
			x := &ical.Expander{}
			ccWant(t, ccOccs(t, x, cal, "2024-01-01T00:00:00Z", "2024-02-01T00:00:00Z"), []string{
				"2024-01-05T14:00:00Z/2024-01-05T15:00:00Z rid=2024-01-05T14:00:00Z",
				"2024-01-11T15:00:00Z/2024-01-11T16:30:00Z rid=2024-01-12T14:00:00Z",
				"2024-01-19T14:00:00Z/2024-01-19T15:00:00Z rid=2024-01-19T14:00:00Z",
				"2024-01-26T14:00:00Z/2024-01-26T15:00:00Z rid=2024-01-26T14:00:00Z",
			})
			for _, occ := range ccOccurrences(t, x, cal, "2024-01-11T00:00:00Z", "2024-01-12T00:00:00Z") {
				if got := occ.Component.Get("SUMMARY").Value; got != "Moved to Thursday" {
					t.Errorf("the moved instance comes from %q", got)
				}
			}
			// The Friday it replaced is gone from its own window.
			ccWant(t, ccOccs(t, x, cal, "2024-01-12T00:00:00Z", "2024-01-13T00:00:00Z"), nil)
		})
	}
}

// §3.8.4.4: "If the value of the DTSTART property is a DATE type value,
// then the value MUST be the calendar date for the recurrence instance."
func TestConformanceRecurrenceIDDate(t *testing.T) {
	cal := ccCal(t, `BEGIN:VEVENT
UID:days@example.com
DTSTAMP:20240101T000000Z
DTSTART;VALUE=DATE:20240101
RRULE:FREQ=DAILY;COUNT=3
END:VEVENT
BEGIN:VEVENT
UID:days@example.com
DTSTAMP:20240101T000000Z
RECURRENCE-ID;VALUE=DATE:20240102
DTSTART;VALUE=DATE:20240110
DTEND;VALUE=DATE:20240112
END:VEVENT`)
	var got []string
	for _, occ := range ccOccurrences(t, &ical.Expander{}, cal, "2024-01-01T00:00:00Z", "2024-02-01T00:00:00Z") {
		got = append(got, occ.Start.String()+"/"+occ.End.String()+" rid="+occ.RecurrenceID.String())
	}
	ccWant(t, got, []string{"20240101/20240102 rid=20240101", "20240103/20240104 rid=20240103", "20240110/20240112 rid=20240102"})
}

// §3.8.4.4: "This property MUST have the same value type as the DTSTART
// property contained within the recurring component. Furthermore, this
// property MUST be specified as a date with local time if and only if the
// DTSTART property contained within the recurring component is specified
// as a date with local time." An override that breaks this names no
// instance of the series. Whether that is reported is validation, which
// is planned; here the series must at least keep the instance.
func TestConformanceRecurrenceIDTypeMismatch(t *testing.T) {
	for _, rid := range []string{
		"RECURRENCE-ID;VALUE=DATE:20240112",
		"RECURRENCE-ID:20240112T090000",
	} {
		t.Run(rid, func(t *testing.T) {
			cal := ccCal(t, ccNewYork+`
BEGIN:VEVENT
UID:weekly@example.com
DTSTAMP:20240101T000000Z
DTSTART;TZID=America/New_York:20240105T090000
DTEND;TZID=America/New_York:20240105T100000
RRULE:FREQ=WEEKLY;COUNT=2
END:VEVENT
BEGIN:VEVENT
UID:weekly@example.com
DTSTAMP:20240101T000000Z
`+rid+`
DTSTART;TZID=America/New_York:20240111T100000
DTEND;TZID=America/New_York:20240111T113000
END:VEVENT`)
			var errs []error
			var got []string
			for occ, err := range cal.Occurrences(ccUTC("2024-01-01T00:00:00Z"), ccUTC("2024-02-01T00:00:00Z")) {
				if err != nil {
					errs = append(errs, err)
					continue
				}
				got = append(got, ccInstant(&ical.Expander{}, occ.Start))
			}
			if !slices.Contains(got, "2024-01-12T14:00:00Z") {
				t.Errorf("occurrences %v: the Friday instance went missing", got)
			}
			if len(errs) == 0 {
				t.Logf("occurrences %v", got)
				ccGap(t, "§3.8.4.4 a RECURRENCE-ID of another value type than DTSTART is accepted without error (schema validation is planned)")
			}
		})
	}
}

// §3.8.4.4 says RECURRENCE-ID works "in conjunction with the UID and
// SEQUENCE properties to identify a particular instance". When two
// overrides name one instance, the RFC doesn't say which applies; the
// architecture doc's choice is the highest SEQUENCE, in either order.
func TestConformanceRecurrenceIDSequence(t *testing.T) {
	for _, order := range [][2]string{{"1", "2"}, {"2", "1"}} {
		cal := ccCal(t, `BEGIN:VEVENT
UID:s@example.com
DTSTAMP:20240101T000000Z
DTSTART:20240105T140000Z
DTEND:20240105T150000Z
RRULE:FREQ=DAILY;COUNT=2
END:VEVENT
BEGIN:VEVENT
UID:s@example.com
DTSTAMP:20240101T000000Z
SEQUENCE:`+order[0]+`
RECURRENCE-ID:20240106T140000Z
DTSTART:20240106T1`+order[0]+`0000Z
DTEND:20240106T1`+order[0]+`3000Z
END:VEVENT
BEGIN:VEVENT
UID:s@example.com
DTSTAMP:20240101T000000Z
SEQUENCE:`+order[1]+`
RECURRENCE-ID:20240106T140000Z
DTSTART:20240106T1`+order[1]+`0000Z
DTEND:20240106T1`+order[1]+`3000Z
END:VEVENT`)
		ccWant(t, ccOccs(t, &ical.Expander{}, cal, "2024-01-01T00:00:00Z", "2024-02-01T00:00:00Z"), []string{
			"2024-01-05T14:00:00Z/2024-01-05T15:00:00Z rid=2024-01-05T14:00:00Z",
			"2024-01-06T12:00:00Z/2024-01-06T12:30:00Z rid=2024-01-06T14:00:00Z",
		})
	}
}

// §3.8.4.4: RANGE=THISANDFUTURE indicates "a range defined by the given
// recurrence instance and all subsequent instances. ... When the given
// recurrence instance is rescheduled, all subsequent instances are also
// rescheduled by the same time difference. ... if the duration of the
// given recurrence instance is modified, then all subsequence instances
// are also modified to have this same duration." Here the third of five
// Friday 09:00-10:00 meetings moves two hours later and shrinks to 30
// minutes, so the third, fourth and fifth are 11:00-11:30.
//
// The architecture doc marks RANGE=THISANDFUTURE as planned, with such
// overrides applying "to their own instance only" until then; the test
// checks that much and skips as a known gap.
func TestConformanceRecurrenceIDThisAndFuture(t *testing.T) {
	cal := ccCal(t, ccNewYork+`
BEGIN:VEVENT
UID:range@example.com
DTSTAMP:20240101T000000Z
DTSTART;TZID=America/New_York:20240105T090000
DTEND;TZID=America/New_York:20240105T100000
RRULE:FREQ=WEEKLY;COUNT=5
END:VEVENT
BEGIN:VEVENT
UID:range@example.com
DTSTAMP:20240101T000000Z
SEQUENCE:1
RECURRENCE-ID;RANGE=THISANDFUTURE;TZID=America/New_York:20240119T090000
DTSTART;TZID=America/New_York:20240119T110000
DTEND;TZID=America/New_York:20240119T113000
END:VEVENT`)
	got := ccOccs(t, &ical.Expander{}, cal, "2024-01-01T00:00:00Z", "2024-03-01T00:00:00Z")
	rfc := []string{
		"2024-01-05T14:00:00Z/2024-01-05T15:00:00Z rid=2024-01-05T14:00:00Z",
		"2024-01-12T14:00:00Z/2024-01-12T15:00:00Z rid=2024-01-12T14:00:00Z",
		"2024-01-19T16:00:00Z/2024-01-19T16:30:00Z rid=2024-01-19T14:00:00Z",
		"2024-01-26T16:00:00Z/2024-01-26T16:30:00Z rid=2024-01-26T14:00:00Z",
		"2024-02-02T16:00:00Z/2024-02-02T16:30:00Z rid=2024-02-02T14:00:00Z",
	}
	documented := slices.Clone(rfc)
	documented[3] = "2024-01-26T14:00:00Z/2024-01-26T15:00:00Z rid=2024-01-26T14:00:00Z"
	documented[4] = "2024-02-02T14:00:00Z/2024-02-02T15:00:00Z rid=2024-02-02T14:00:00Z"
	switch {
	case slices.Equal(got, rfc):
	case slices.Equal(got, documented):
		ccGap(t, "§3.8.4.4 RANGE=THISANDFUTURE applies to its own instance only (planned in the architecture doc)")
	default:
		t.Errorf("occurrences:\n got %q\nwant %q\n(or, as documented until RANGE is implemented, %q)", got, rfc, documented)
	}
}

// §3.8.5.3: "If the duration of the recurring component is specified with
// the DTEND or DUE property, then the same exact duration will apply to
// all the members of the generated recurrence set." A daily 00:30-02:30
// event lasts exactly two hours on the days clocks change: 00:30 EDT +
// 2h is 01:30 EST on Nov 3, and 00:30 EST + 2h is 03:30 EDT on Mar 10.
func TestConformanceDTEndExactDuration(t *testing.T) {
	cal := ccCal(t, ccNewYork+`
BEGIN:VEVENT
UID:fall@example.com
DTSTAMP:20240101T000000Z
DTSTART;TZID=America/New_York:20241102T003000
DTEND;TZID=America/New_York:20241102T023000
RRULE:FREQ=DAILY;COUNT=3
END:VEVENT
BEGIN:VEVENT
UID:spring@example.com
DTSTAMP:20240101T000000Z
DTSTART;TZID=America/New_York:20240309T003000
DTEND;TZID=America/New_York:20240309T023000
RRULE:FREQ=DAILY;COUNT=3
END:VEVENT`)
	x := &ical.Expander{}
	ccWant(t, ccSpans(t, x, cal, "2024-11-01T00:00:00Z", "2024-11-10T00:00:00Z"), []string{
		"2024-11-02T04:30:00Z/2024-11-02T06:30:00Z",
		"2024-11-03T04:30:00Z/2024-11-03T06:30:00Z",
		"2024-11-04T05:30:00Z/2024-11-04T07:30:00Z",
	})
	ccWant(t, ccSpans(t, x, cal, "2024-03-01T00:00:00Z", "2024-03-20T00:00:00Z"), []string{
		"2024-03-09T05:30:00Z/2024-03-09T07:30:00Z",
		"2024-03-10T05:30:00Z/2024-03-10T07:30:00Z",
		"2024-03-11T04:30:00Z/2024-03-11T06:30:00Z",
	})
}

// §3.8.5.3: "if the duration of the recurring component is specified with
// the DURATION property, then the same nominal duration will apply to all
// the members of the generated recurrence set and the exact duration of
// each recurrence instance will depend on its specific start time. For
// example, recurrence instances of a nominal duration of one day will have
// an exact duration of more or less than 24 hours on a day where a time
// zone shift occurs." Daily at 12:00 with P1D: the Mar 9 instance ends at
// 12:00 EDT on Mar 10, 23 hours later. PT24H is exact by contrast.
func TestConformanceDurationNominal(t *testing.T) {
	cal := ccCal(t, ccNewYork+`
BEGIN:VEVENT
UID:nominal@example.com
DTSTAMP:20240101T000000Z
DTSTART;TZID=America/New_York:20240309T120000
DURATION:P1D
RRULE:FREQ=DAILY;COUNT=3
END:VEVENT`)
	x := &ical.Expander{}
	ccWant(t, ccSpans(t, x, cal, "2024-03-01T00:00:00Z", "2024-03-20T00:00:00Z"), []string{
		"2024-03-09T17:00:00Z/2024-03-10T16:00:00Z",
		"2024-03-10T16:00:00Z/2024-03-11T16:00:00Z",
		"2024-03-11T16:00:00Z/2024-03-12T16:00:00Z",
	})
	exact := ccCal(t, ccNewYork+`
BEGIN:VEVENT
UID:exact@example.com
DTSTAMP:20240101T000000Z
DTSTART;TZID=America/New_York:20240309T120000
DURATION:PT24H
RRULE:FREQ=DAILY;COUNT=2
END:VEVENT`)
	ccWant(t, ccSpans(t, x, exact, "2024-03-01T00:00:00Z", "2024-03-20T00:00:00Z"), []string{
		"2024-03-09T17:00:00Z/2024-03-10T17:00:00Z",
		"2024-03-10T16:00:00Z/2024-03-11T16:00:00Z",
	})
}

// §3.8.5.3: "The duration of a specific recurrence may be modified in an
// exception component or simply by using an RDATE property of PERIOD value
// type." §5 (recommended practice 2): when an RDATE PERIOD and the RRULE
// give the same start, "they should be collapsed to, and considered as, a
// single instance", with "the duration ... specified by the RDATE".
func TestConformanceRDatePeriod(t *testing.T) {
	cal := ccCal(t, `BEGIN:VEVENT
UID:periods@example.com
DTSTAMP:20240101T000000Z
DTSTART:20240101T090000Z
DTEND:20240101T100000Z
RRULE:FREQ=DAILY;COUNT=3
RDATE;VALUE=PERIOD:20240102T090000Z/PT4H,20240105T150000Z/PT3H
RDATE;VALUE=PERIOD:20240106T150000Z/20240106T160000Z
RDATE:20240107T090000Z
END:VEVENT`)
	ccWant(t, ccSpans(t, &ical.Expander{}, cal, "2024-01-01T00:00:00Z", "2024-02-01T00:00:00Z"), []string{
		"2024-01-01T09:00:00Z/2024-01-01T10:00:00Z",
		"2024-01-02T09:00:00Z/2024-01-02T13:00:00Z",
		"2024-01-03T09:00:00Z/2024-01-03T10:00:00Z",
		"2024-01-05T15:00:00Z/2024-01-05T18:00:00Z",
		"2024-01-06T15:00:00Z/2024-01-06T16:00:00Z",
		"2024-01-07T09:00:00Z/2024-01-07T10:00:00Z",
	})
}

// §3.3.9: "The start of the period MUST be before the end of the period",
// and a period-start has "a positive duration". A period that breaks this
// is an error, and its instance keeps the series' own length rather than
// ending before it starts.
func TestConformanceRDatePeriodBackwards(t *testing.T) {
	cal := ccCal(t, `BEGIN:VEVENT
UID:backwards@example.com
DTSTAMP:20240101T000000Z
DTSTART:20240101T090000Z
DTEND:20240101T100000Z
RDATE;VALUE=PERIOD:20240102T090000Z/20240102T080000Z,20240103T090000Z/-PT1H,20240104T090000Z/PT2H
END:VEVENT`)
	var got []string
	var errs int
	x := &ical.Expander{}
	for occ, err := range x.Occurrences(cal, ccUTC("2024-01-01T00:00:00Z"), ccUTC("2024-02-01T00:00:00Z")) {
		if err != nil {
			errs++
			continue
		}
		got = append(got, ccInstant(x, occ.Start)+"/"+ccInstant(x, occ.End))
	}
	ccWant(t, got, []string{
		"2024-01-01T09:00:00Z/2024-01-01T10:00:00Z",
		"2024-01-02T09:00:00Z/2024-01-02T10:00:00Z",
		"2024-01-03T09:00:00Z/2024-01-03T10:00:00Z",
		"2024-01-04T09:00:00Z/2024-01-04T11:00:00Z",
	})
	if errs == 0 {
		t.Error("no error for the backwards periods")
	}
}

// §3.8.5.1: "The final recurrence set is generated by gathering all of the
// start DATE-TIME values generated by any of the specified RRULE and RDATE
// properties, and then excluding any start DATE-TIME values specified by
// EXDATE properties." COUNT counts the rule's instances before the
// exclusion, so COUNT=5 less two EXDATEs is three. "The EXDATE property
// can be used to exclude the value specified in DTSTART." An EXDATE in
// UTC excludes the zoned instance at that instant, and one that matches
// nothing excludes nothing.
func TestConformanceExDate(t *testing.T) {
	cal := ccCal(t, ccNewYork+`
BEGIN:VEVENT
UID:ex@example.com
DTSTAMP:20240101T000000Z
DTSTART;TZID=America/New_York:20240308T090000
DTEND;TZID=America/New_York:20240308T100000
RRULE:FREQ=DAILY;COUNT=5
EXDATE;TZID=America/New_York:20240308T090000
EXDATE:20240310T130000Z,20240311T150000Z
RDATE;TZID=America/New_York:20240320T090000
EXDATE;TZID=America/New_York:20240320T090000
END:VEVENT`)
	// Mar 8 (DTSTART) and Mar 10 (09:00 EDT = 13:00Z) are excluded; the
	// 15:00Z EXDATE matches nothing; the RDATE is excluded too.
	ccWant(t, ccStarts(t, &ical.Expander{}, cal, "2024-03-01T00:00:00Z", "2024-04-01T00:00:00Z"), []string{
		"2024-03-09T14:00:00Z",
		"2024-03-11T13:00:00Z",
		"2024-03-12T13:00:00Z",
	})

	// The same for a DATE series, with DATE values (§3.8.5.1: "The value
	// type can be set to DATE").
	days := ccCal(t, `BEGIN:VEVENT
UID:days@example.com
DTSTAMP:20240101T000000Z
DTSTART;VALUE=DATE:20240101
RRULE:FREQ=DAILY;COUNT=4
EXDATE;VALUE=DATE:20240102,20240104
RDATE;VALUE=DATE:20240110
END:VEVENT`)
	ccWant(t, ccSpans(t, &ical.Expander{}, days, "2024-01-01T00:00:00Z", "2024-02-01T00:00:00Z"), []string{
		"2024-01-01T00:00:00Z/2024-01-02T00:00:00Z",
		"2024-01-03T00:00:00Z/2024-01-04T00:00:00Z",
		"2024-01-10T00:00:00Z/2024-01-11T00:00:00Z",
	})
}

// Erratum 4271: an instance whose local time falls in a DST gap is not
// dropped; §3.3.5 resolves it "using the UTC offset before the gap", so
// 02:30 on 2024-03-10 is 07:30Z (03:30 EDT), and it counts toward COUNT.
// §3.3.5: a local time that occurs twice "refers to the first occurrence",
// so 01:30 on 2024-11-03 is 01:30 EDT (05:30Z).
func TestConformanceDSTGapAndRepeat(t *testing.T) {
	cal := ccCal(t, ccNewYork+`
BEGIN:VEVENT
UID:gap@example.com
DTSTAMP:20240101T000000Z
DTSTART;TZID=America/New_York:20240309T023000
DURATION:PT30M
RRULE:FREQ=DAILY;COUNT=3
END:VEVENT
BEGIN:VEVENT
UID:repeat@example.com
DTSTAMP:20240101T000000Z
DTSTART;TZID=America/New_York:20241102T013000
DURATION:PT30M
RRULE:FREQ=DAILY;COUNT=3
END:VEVENT
BEGIN:VEVENT
UID:rdate-gap@example.com
DTSTAMP:20240101T000000Z
DTSTART;TZID=America/New_York:20240301T120000
RDATE;TZID=America/New_York:20240310T021500
END:VEVENT`)
	x := &ical.Expander{}
	ccWant(t, ccSpans(t, x, cal, "2024-03-09T00:00:00Z", "2024-03-12T00:00:00Z"), []string{
		"2024-03-09T07:30:00Z/2024-03-09T08:00:00Z",
		"2024-03-10T07:15:00Z/2024-03-10T07:15:00Z",
		"2024-03-10T07:30:00Z/2024-03-10T08:00:00Z",
		"2024-03-11T06:30:00Z/2024-03-11T07:00:00Z",
	})
	ccWant(t, ccSpans(t, x, cal, "2024-11-01T00:00:00Z", "2024-11-10T00:00:00Z"), []string{
		"2024-11-02T05:30:00Z/2024-11-02T06:00:00Z",
		"2024-11-03T05:30:00Z/2024-11-03T06:00:00Z",
		"2024-11-04T06:30:00Z/2024-11-04T07:00:00Z",
	})
	for _, occ := range ccOccurrences(t, x, cal, "2024-03-10T07:00:00Z", "2024-03-10T08:00:00Z") {
		if name, off := occ.Start.Time().Zone(); name != "EDT" || off != ccEDT {
			t.Errorf("gap instance %s is in %s %d, want EDT", occ.Start.Time(), name, off)
		}
	}
}

// §3.2.19 with the Expander: a TZID that can't be resolved leaves the
// value with no instant, so nothing placed by it may be reported at a
// guessed one (the UTC wall clock, or the host's zone). The error names
// the component; the rest of the calendar, and every part of the
// component that could be read (Item.Recurrence: "On error the set holds
// every part that could be read"), are still expanded.
func TestConformanceUnknownTZIDInSeries(t *testing.T) {
	cal := ccCal(t, `BEGIN:VEVENT
UID:start@example.com
DTSTAMP:20240101T000000Z
DTSTART;TZID=Nowhere/Imaginary:20240701T090000
RRULE:FREQ=DAILY;COUNT=3
END:VEVENT
BEGIN:VEVENT
UID:rdate@example.com
DTSTAMP:20240101T000000Z
DTSTART:20240701T100000Z
RRULE:FREQ=DAILY;COUNT=2
RDATE;TZID=Nowhere/Imaginary:20240705T100000
END:VEVENT
BEGIN:VEVENT
UID:exdate@example.com
DTSTAMP:20240101T000000Z
DTSTART:20240701T110000Z
RRULE:FREQ=DAILY;COUNT=2
EXDATE;TZID=Nowhere/Imaginary:20240702T110000
END:VEVENT
BEGIN:VEVENT
UID:override@example.com
DTSTAMP:20240101T000000Z
DTSTART:20240701T120000Z
RRULE:FREQ=DAILY;COUNT=2
END:VEVENT
BEGIN:VEVENT
UID:override@example.com
DTSTAMP:20240101T000000Z
RECURRENCE-ID;TZID=Nowhere/Imaginary:20240702T120000
DTSTART:20240710T120000Z
END:VEVENT
BEGIN:VEVENT
UID:moved@example.com
DTSTAMP:20240101T000000Z
DTSTART:20240701T130000Z
RRULE:FREQ=DAILY;COUNT=2
END:VEVENT
BEGIN:VEVENT
UID:moved@example.com
DTSTAMP:20240101T000000Z
RECURRENCE-ID:20240702T130000Z
DTSTART;TZID=Nowhere/Imaginary:20240702T150000
END:VEVENT`)
	var got []string
	failed := map[string]int{}
	for occ, err := range cal.Occurrences(ccUTC("2024-06-01T00:00:00Z"), ccUTC("2024-08-01T00:00:00Z")) {
		uid := occ.Component.Get("UID").Value
		if err != nil {
			if !errors.Is(err, tz.ErrUnknownZone) {
				t.Errorf("%s: error %v doesn't wrap tz.ErrUnknownZone", uid, err)
			}
			failed[uid]++
			continue
		}
		got = append(got, uid+" "+ccInstant(&ical.Expander{}, occ.Start))
	}
	for _, uid := range []string{"start@example.com", "rdate@example.com", "exdate@example.com", "override@example.com", "moved@example.com"} {
		if failed[uid] != 1 {
			t.Errorf("%s: %d errors, want 1", uid, failed[uid])
		}
	}
	ccWant(t, got, []string{
		// start: nothing; its DTSTART has no instant.
		// rdate: the rule's instances, not the unresolved RDATE.
		"rdate@example.com 2024-07-01T10:00:00Z",
		"exdate@example.com 2024-07-01T11:00:00Z",
		"override@example.com 2024-07-01T12:00:00Z",
		"moved@example.com 2024-07-01T13:00:00Z",
		"rdate@example.com 2024-07-02T10:00:00Z",
		// exdate: the unresolved EXDATE excludes nothing.
		"exdate@example.com 2024-07-02T11:00:00Z",
		// override: which instance it replaces is unknown, so none is.
		"override@example.com 2024-07-02T12:00:00Z",
		// moved: the Jul 2 instance was moved to an unknown time, so it
		// isn't reported at all.
	})
}

// §4's examples, as the prose describes them.
func TestConformanceExampleEvents(t *testing.T) {
	x := &ical.Expander{}

	// §3.4: "DTSTART:19970714T170000Z", "DTEND:19970715T040000Z".
	bastille := ccFixture(t, "rfc5545-3.4-a")
	ccCheckSpan(t, ccFirstEvent(t, bastille), "1997-07-14T17:00:00Z", "1997-07-15T04:00:00Z")
	ccWant(t, ccSpans(t, x, bastille, "1997-07-15T00:00:00Z", "1997-07-16T00:00:00Z"), []string{"1997-07-14T17:00:00Z/1997-07-15T04:00:00Z"})

	// "a three-day conference that begins at 2:30 P.M. UTC, September 18,
	// 1996 and ends at 10:00 P.M. UTC, September 20, 1996."
	conf := ccFixture(t, "rfc5545-4-a")
	ev := ccFirstEvent(t, conf)
	ccCheckSpan(t, ev, "1996-09-18T14:30:00Z", "1996-09-20T22:00:00Z")
	if ev.Status() != "CONFIRMED" || ev.Summary() != "Networld+Interop Conference" ||
		ev.Description() != "Networld+Interop Conference and Exhibit\nAtlanta World Congress Center\nAtlanta, Georgia" {
		t.Errorf("Status %q Summary %q Description %q", ev.Status(), ev.Summary(), ev.Description())
	}
	ccWant(t, ccSpans(t, x, conf, "1996-09-19T00:00:00Z", "1996-09-20T00:00:00Z"), []string{"1996-09-18T14:30:00Z/1996-09-20T22:00:00Z"})
	ccWant(t, ccSpans(t, x, conf, "1996-09-20T22:00:00Z", "1996-09-21T00:00:00Z"), nil)

	// The MIME example: METHOD:xyz, a meeting from 12:30 to 21:00 UTC.
	mime := ccFixture(t, "rfc5545-4-c")
	ev = ccFirstEvent(t, mime)
	ccCheckSpan(t, ev, "1997-03-24T12:30:00Z", "1997-03-24T21:00:00Z")
	if mime.Method() != "xyz" || ev.Sequence() != 0 || ev.Class() != "PUBLIC" || ev.Location() != "LDB Lobby" ||
		!slices.Equal(ev.Categories(), []string{"MEETING", "PROJECT"}) {
		t.Errorf("Method %q Sequence %d Class %q Location %q Categories %q", mime.Method(), ev.Sequence(), ev.Class(), ev.Location(), ev.Categories())
	}
}

// §4's journal entry has no DTSTART, which §3.6.3 allows (dtstart is
// OPTIONAL in a jourprop). It is associated with no date, so it has no
// occurrence; being valid, it shouldn't be reported as an error either.
func TestConformanceExampleJournal(t *testing.T) {
	cal := ccFixture(t, "rfc5545-4-e")
	for j := range cal.Journals() {
		if _, err := j.Start(); !errors.Is(err, ical.ErrNoProperty) {
			t.Errorf("Start: %v, want ErrNoProperty", err)
		}
		if j.Status() != "DRAFT" || j.Class() != "PUBLIC" ||
			!slices.Equal(j.Categories(), []string{"Project Report", "XYZ", "Weekly Meeting"}) {
			t.Errorf("Status %q Class %q Categories %q", j.Status(), j.Class(), j.Categories())
		}
	}
	x := &ical.Expander{Components: []string{"VJOURNAL"}}
	var occs []ical.Occurrence
	var errs []error
	for occ, err := range x.Occurrences(cal, ccUTC("1997-01-01T00:00:00Z"), ccUTC("1998-01-01T00:00:00Z")) {
		if err != nil {
			errs = append(errs, err)
			continue
		}
		occs = append(occs, occ)
	}
	if len(occs) != 0 {
		t.Errorf("occurrences %+v, want none", occs)
	}
	if len(errs) > 0 {
		t.Logf("errors: %v", errs)
		ccGap(t, "§3.6.3 a VJOURNAL without DTSTART (valid) is reported as an error by the Expander (VJOURNAL defaults in the Expander are planned)")
	}
}

// §3.6.1/§3.7.2: DTSTART is REQUIRED in a VEVENT only when the calendar
// has no METHOD. With METHOD (an iTIP message, say a CANCEL) a VEVENT may
// lack it; the typed view reports it missing, and the Expander has no
// instance to give it.
func TestConformanceEventWithoutDTStart(t *testing.T) {
	cal := ccRaw(t, `BEGIN:VCALENDAR
PRODID:-//conformance//EN
VERSION:2.0
METHOD:CANCEL
BEGIN:VEVENT
UID:cancel@example.com
DTSTAMP:20240101T000000Z
SEQUENCE:2
END:VEVENT
END:VCALENDAR
`)
	ev := ccFirstEvent(t, cal)
	if _, err := ev.Start(); !errors.Is(err, ical.ErrNoProperty) {
		t.Errorf("Start: %v, want ErrNoProperty", err)
	}
	if _, err := ev.End(); err == nil {
		t.Errorf("End: no error with no DTSTART")
	}
	for occ, err := range cal.Occurrences(ccUTC("1900-01-01T00:00:00Z"), ccUTC("2100-01-01T00:00:00Z")) {
		if err == nil {
			t.Errorf("an occurrence at %v for an event with no DTSTART", occ.Start)
		}
	}
}
