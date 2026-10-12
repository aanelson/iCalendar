package main

// 3.08 Throughput on an ordinary feed, and what reading times costs.

import (
	"bytes"
	"fmt"
	"os"
	"runtime/pprof"
	"strings"
	"time"

	ical "github.com/aanelson/iCalendar"
	ics "github.com/arran4/golang-ical"
	eical "github.com/emersion/go-ical"
)

// realisticFeed is n ordinary events in a zone, each with an RRULE,
// folded DESCRIPTION, quoted parameters and escapes.
func realisticFeed(n int) []byte {
	var b strings.Builder
	b.WriteString(calHead)
	for i := range n {
		fmt.Fprintf(&b, "BEGIN:VEVENT\r\nUID:event-%d@example.com\r\nDTSTAMP:20260101T000000Z\r\n"+
			"DTSTART;TZID=America/New_York:20260105T090000\r\nDTEND;TZID=America/New_York:20260105T100000\r\n"+
			"SUMMARY:Team meeting %d with a reasonably long summary\\, and an escape\r\n"+
			"DESCRIPTION:Line one\\nLine two with more words so that this line is longer t\r\n han seventy-five octets and has to be folded.\r\n"+
			"LOCATION:Room 101\r\nATTENDEE;CN=\"Doe, John\";ROLE=REQ-PARTICIPANT;PARTSTAT=ACCEPTED:mailto:john@example.com\r\n"+
			"RRULE:FREQ=WEEKLY;BYDAY=MO\r\nEND:VEVENT\r\n", i, i)
	}
	b.WriteString("END:VCALENDAR\r\n")
	return []byte(b.String())
}

// readTimes parses in and reads DTSTART and DTEND of every event with the
// library's typed accessors.
func readTimes(lib string, in []byte) (int, error) {
	n := 0
	switch lib {
	case This:
		cal, err := ical.Parse(bytes.NewReader(in))
		if err != nil {
			return 0, err
		}
		for ev := range cal.Events() {
			s, err1 := ev.Start()
			e, err2 := ev.End()
			if err1 != nil || err2 != nil {
				return n, fmt.Errorf("%v %v", err1, err2)
			}
			if e.Time().After(s.Time()) {
				n++
			}
		}
	case GoIcal, GoIcalO:
		cal, err := goIcalParse(lib, in)
		if err != nil {
			return 0, err
		}
		for _, ev := range cal.Events() {
			s, err1 := ev.GetStartAt()
			e, err2 := ev.GetEndAt()
			if err1 != nil || err2 != nil {
				return n, fmt.Errorf("%v %v", err1, err2)
			}
			if e.After(s) {
				n++
			}
		}
	case Emersion:
		cal, err := eical.NewDecoder(bytes.NewReader(in)).Decode()
		if err != nil {
			return 0, err
		}
		for _, ev := range cal.Events() {
			s, err1 := ev.DateTimeStart(nil)
			e, err2 := ev.DateTimeEnd(nil)
			if err1 != nil || err2 != nil {
				return n, fmt.Errorf("%v %v", err1, err2)
			}
			if e.After(s) {
				n++
			}
		}
	}
	return n, nil
}

func init() {
	_ = ics.ComponentVEvent
	register(&Probe{ID: "3.08-throughput", Group: "3 hostile input",
		Title:   "throughput on an ordinary 20,000-event feed (~9 MB): parse only, and parse + read DTSTART/DTEND of every event (best of 5)",
		RFC:     "performance (the docs claim regexes and per-property time.LoadLocation cost the other libraries)",
		Libs:    parsers,
		Args:    []string{"parse", "parse+times"},
		Timeout: 120,
		Run: func(lib, arg string) Result {
			in := realisticFeed(20000)
			best := time.Duration(1 << 62)
			var events int
			var err error
			for range 5 {
				t0 := time.Now()
				if arg == "parse" {
					events, err = parseOnly(lib, in)
				} else {
					events, err = readTimes(lib, in)
				}
				if d := time.Since(t0); d < best {
					best = d
				}
				if err != nil {
					return failed(err)
				}
			}
			if f := os.Getenv("PROBE_CPUPROFILE"); f != "" && arg == "parse+times" {
				out, _ := os.Create(f)
				pprof.StartCPUProfile(out)
				for range 5 {
					readTimes(lib, in)
				}
				pprof.StopCPUProfile()
				out.Close()
			}
			mbs := float64(len(in)) / (1 << 20) / best.Seconds()
			r := Result{Outcome: "pass", Got: fmt.Sprintf("%.0f MB/s (%.1f ms for %.1f MB, %d events)", mbs, float64(best.Microseconds())/1000, float64(len(in))/(1<<20), events),
				Ms: float64(best.Microseconds()) / 1000, Extra: map[string]any{"mb_per_s": mbs}}
			if events != 20000 {
				r.Outcome = "wrong"
			}
			return r
		}})
}

// parseOnly is each library's parse call and nothing else.
func parseOnly(lib string, in []byte) (int, error) {
	n := 0
	switch lib {
	case This:
		cal, err := ical.NewDecoder(bytes.NewReader(in)).Decode()
		if err != nil {
			return 0, err
		}
		for range cal.Events() {
			n++
		}
	case GoIcal, GoIcalO:
		cal, err := goIcalParse(lib, in)
		if err != nil {
			return 0, err
		}
		n = len(cal.Events())
	case Emersion:
		cal, err := eical.NewDecoder(bytes.NewReader(in)).Decode()
		if err != nil {
			return 0, err
		}
		n = len(cal.Events())
	}
	return n, nil
}
