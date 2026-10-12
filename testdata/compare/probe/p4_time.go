package main

// 4. Time semantics and 5. durations.

import (
	"errors"
	"fmt"
	"time"

	ical "github.com/aanelson/iCalendar"
	ics "github.com/arran4/golang-ical"
	eical "github.com/emersion/go-ical"
	"github.com/teambition/rrule-go"
)

const vtzEastern = "BEGIN:VTIMEZONE\r\nTZID:%s\r\n" +
	"BEGIN:STANDARD\r\nDTSTART:16010101T020000\r\nTZOFFSETFROM:-0400\r\nTZOFFSETTO:-0500\r\nRRULE:FREQ=YEARLY;BYDAY=1SU;BYMONTH=11\r\nEND:STANDARD\r\n" +
	"BEGIN:DAYLIGHT\r\nDTSTART:16010101T020000\r\nTZOFFSETFROM:-0500\r\nTZOFFSETTO:-0400\r\nRRULE:FREQ=YEARLY;BYDAY=2SU;BYMONTH=3\r\nEND:DAYLIGHT\r\n" +
	"END:VTIMEZONE\r\n"

func startProbe(lib string, in []byte, want string) Result {
	ev, err := firstEvent(lib, in)
	if err != nil {
		return failed(err)
	}
	t, err := ev.start()
	if err != nil {
		return failed(err)
	}
	r := check(utc(t), want)
	r.Detail = "as returned: " + t.Format(time.RFC3339+" MST")
	return r
}

func init() {
	hostTZ := []string{"UTC", "America/Los_Angeles"}
	register(&Probe{ID: "4.01-floating-host-tz", Group: "4 time semantics",
		Title: "floating DTSTART:20260715T090000 read with the default accessor under TZ=UTC vs TZ=America/Los_Angeles",
		RFC:   "§3.3.5 form #1: floating is bound to no zone; result must not silently depend on the host's zone",
		Libs:  parsers, TZ: hostTZ,
		Run: func(lib, _ string) Result {
			ev, err := firstEvent(lib, calWith("", "DTSTART:20260715T090000"))
			if err != nil {
				return failed(err)
			}
			t, err := ev.start()
			if err != nil {
				return failed(err)
			}
			return Result{Outcome: "info", Got: utc(t), Detail: "as returned: " + t.Format(time.RFC3339+" MST")}
		}})
	register(&Probe{ID: "4.02-date-host-tz", Group: "4 time semantics",
		Title: "DATE DTSTART;VALUE=DATE:20260715 read with the default accessor under TZ=UTC vs TZ=America/Los_Angeles",
		RFC:   "§3.3.4: a DATE has no zone; result must not silently depend on the host's zone",
		Libs:  parsers, TZ: hostTZ,
		Run: func(lib, _ string) Result {
			ev, err := firstEvent(lib, calWith("", "DTSTART;VALUE=DATE:20260715"))
			if err != nil {
				return failed(err)
			}
			t, err := ev.start()
			if err != nil {
				return failed(err)
			}
			return Result{Outcome: "info", Got: utc(t), Detail: "as returned: " + t.Format(time.RFC3339+" MST")}
		}})

	register(&Probe{ID: "4.03-windows-tzid-no-vtimezone", Group: "4 time semantics",
		Title: "DTSTART;TZID=Eastern Standard Time:20260715T090000 with no VTIMEZONE → 13:00Z",
		RFC:   "§3.2.19 requires a VTIMEZONE for each TZID, so this is outside the RFC; it is what Outlook/Exchange feeds send",
		Libs:  parsers,
		Run: func(lib, _ string) Result {
			return startProbe(lib, calWith("", "DTSTART;TZID=Eastern Standard Time:20260715T090000"), "2026-07-15T13:00:00Z")
		}})
	register(&Probe{ID: "4.04-windows-tzid-with-vtimezone", Group: "4 time semantics",
		Title: "TZID=Eastern Standard Time with Outlook's VTIMEZONE for it → 13:00Z",
		RFC:   "§3.6.5 / §3.2.19: the TZID refers to the VTIMEZONE in the calendar",
		Libs:  parsers,
		Run: func(lib, _ string) Result {
			return startProbe(lib, calWith(fmt.Sprintf(vtzEastern, "Eastern Standard Time"), "DTSTART;TZID=Eastern Standard Time:20260715T090000"), "2026-07-15T13:00:00Z")
		}})
	register(&Probe{ID: "4.05-custom-vtimezone", Group: "4 time semantics",
		Title: "Outlook custom zone TZID:Customized Time Zone (US-Eastern rules): July 09:00 → 13:00Z, January 09:00 → 14:00Z",
		RFC:   "§3.6.5: the VTIMEZONE defines the zone; no IANA name exists",
		Libs:  parsers,
		Args:  []string{"20260715T090000=2026-07-15T13:00:00Z", "20260115T090000=2026-01-15T14:00:00Z"},
		Run: func(lib, arg string) Result {
			local, want := arg[:15], arg[16:]
			return startProbe(lib, calWith(fmt.Sprintf(vtzEastern, "Customized Time Zone"), "DTSTART;TZID=Customized Time Zone:"+local), want)
		}})

	gapProbe := func(id, title, local, want string) {
		register(&Probe{ID: id, Group: "4 time semantics", Title: title,
			RFC:  "§3.3.5 form #3: a repeated time is the first occurrence; a time in a gap uses the offset before the gap",
			Libs: append(append([]string{}, parsers...), RRuleGo),
			Run: func(lib, _ string) Result {
				if lib == RRuleGo {
					t, err := rrule.StrToDtStart("TZID=America/New_York:"+local, time.UTC)
					if err != nil {
						return failed(err)
					}
					r := check(utc(t), want)
					r.Detail = "rrule.StrToDtStart: " + t.Format(time.RFC3339+" MST")
					return r
				}
				return startProbe(lib, calWith("", "DTSTART;TZID=America/New_York:"+local), want)
			}})
	}
	gapProbe("4.06-dst-gap", "DTSTART;TZID=America/New_York:20070311T023000 (spring-forward gap) → 07:30Z", "20070311T023000", "2007-03-11T07:30:00Z")
	gapProbe("4.07-dst-fold", "DTSTART;TZID=America/New_York:20071104T013000 (repeated hour) → 05:30Z", "20071104T013000", "2007-11-04T05:30:00Z")

	// 5. Durations.
	endProbe := func(id, title, rfc, start, dur, want string) {
		register(&Probe{ID: id, Group: "5 durations", Title: title, RFC: rfc, Libs: parsers,
			Run: func(lib, _ string) Result {
				if lib == GoIcal || lib == GoIcalO {
					return na("golang-ical has no DURATION reader: GetEndAt reads only DTEND (SetDuration exists for writing)")
				}
				ev, err := firstEvent(lib, calWith("", "DTSTART;TZID=America/New_York:"+start, "DURATION:"+dur))
				if err != nil {
					return failed(err)
				}
				s, err := ev.start()
				if err != nil {
					return failed(err)
				}
				e, err := ev.end()
				if err != nil {
					return failed(err)
				}
				r := check(utc(e), want)
				r.Detail = fmt.Sprintf("end %s, elapsed %v", e.In(nyLoc()).Format(time.RFC3339), e.Sub(s))
				return r
			}})
	}
	nominal := "§3.3.6: days and weeks are nominal (a day across a DST change is 23 or 25 h); §3.8.5.3 nominal vs exact"
	endProbe("5.01-p1d-spring", "DURATION:P1D from 2026-03-07 12:00 New York → 2026-03-08 12:00 EDT (23 h)", nominal, "20260307T120000", "P1D", "2026-03-08T16:00:00Z")
	endProbe("5.02-p1d-fall", "DURATION:P1D from 2026-10-31 12:00 New York → 2026-11-01 12:00 EST (25 h)", nominal, "20261031T120000", "P1D", "2026-11-01T17:00:00Z")
	endProbe("5.03-pt24h-spring", "DURATION:PT24H from 2026-03-07 12:00 New York → exactly 24 h later (13:00 EDT)", "§3.3.6: hours, minutes, seconds are exact", "20260307T120000", "PT24H", "2026-03-08T17:00:00Z")
	endProbe("5.05-p2w-spring", "DURATION:P2W from 2026-03-01 12:00 New York → 2026-03-15 12:00 EDT (335 h)", nominal, "20260301T120000", "P2W", "2026-03-15T16:00:00Z")

	register(&Probe{ID: "5.04-negative-duration", Group: "5 durations",
		Title: "parse -PT15M (a typical VALARM TRIGGER) and P2W",
		RFC:   "§3.3.6 dur-value = ([\"+\"] / \"-\") \"P\" …",
		Libs:  parsers,
		Run: func(lib, _ string) Result {
			switch lib {
			case This:
				a, err1 := ical.ParseDuration("-PT15M")
				b, err2 := ical.ParseDuration("P2W")
				if err := errors.Join(err1, err2); err != nil {
					return failed(err)
				}
				return check(fmt.Sprintf("%v, %s (%v)", a.Approx(), b, b.Approx()), "-15m0s, P2W (336h0m0s)")
			case Emersion:
				a, err1 := (&eical.Prop{Name: "TRIGGER", Params: eical.Params{}, Value: "-PT15M"}).Duration()
				b, err2 := (&eical.Prop{Name: "DURATION", Params: eical.Params{}, Value: "P2W"}).Duration()
				if err := errors.Join(err1, err2); err != nil {
					return failed(err)
				}
				return Result{Outcome: "pass", Got: fmt.Sprintf("%v, %v (as a time.Duration: weeks are exact)", a, b)}
			default:
				_ = ics.ComponentPropertyDuration
				return na("golang-ical has no DURATION parser")
			}
		}})
}

func nyLoc() *time.Location {
	l, err := time.LoadLocation("America/New_York")
	if err != nil {
		panic(err)
	}
	return l
}
