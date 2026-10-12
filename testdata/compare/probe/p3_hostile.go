package main

// 3. Hostile input. Each (probe, lib, size) runs in its own subprocess
// with the driver's 60 s timeout; the driver also records peak RSS.

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	ical "github.com/aanelson/iCalendar"
	eical "github.com/emersion/go-ical"
)

func sizeArg(s string) int {
	mult := 1
	switch {
	case strings.HasSuffix(s, "Ki"):
		mult, s = 1<<10, strings.TrimSuffix(s, "Ki")
	case strings.HasSuffix(s, "Mi"):
		mult, s = 1<<20, strings.TrimSuffix(s, "Mi")
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return n * mult
}

func wrapLine(line string) []byte {
	return []byte(calHead + "BEGIN:VEVENT\r\nUID:x\r\nDTSTAMP:20260101T000000Z\r\n" + line + "\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
}

// timedParse decodes in with lib. For this library it decodes twice: with
// MaxLineLength raised to 256 MiB (for the timing and scaling numbers, as
// the library's own TestHostileFeeds does) and with the default limits
// (reported in Detail).
func timedParse(lib string, in []byte, after func(lib string, p parsed)) Result {
	var r Result
	if lib == This {
		t0 := time.Now()
		def := parseThis(in, nil)
		defMs := since(t0)
		t0 = time.Now()
		p := parseThis(in, func(d *ical.Decoder) { d.MaxLineLength = 256 << 20 })
		if after != nil {
			after(lib, p)
		}
		r.Ms = since(t0)
		r.Outcome, r.Got = verdict(p)
		r.Detail = fmt.Sprintf("with default limits (1 MiB line, depth 32): %s in %.1f ms", short(def), defMs)
		return r
	}
	t0 := time.Now()
	p := parseFeed(lib, in)
	if after != nil {
		after(lib, p)
	}
	r.Ms = since(t0)
	r.Outcome, r.Got = verdict(p)
	return r
}

func verdict(p parsed) (string, string) {
	if p.Err != nil {
		return "error", short(p)
	}
	return "pass", short(p)
}

func short(p parsed) string {
	if p.Err != nil {
		s := p.Err.Error()
		if len(s) > 160 {
			s = s[:160] + "…"
		}
		return "error: " + s
	}
	return fmt.Sprintf("ok: %d calendar(s), %d event(s)", p.Cals, len(p.Events))
}

func init() {
	sizes := []string{"64Ki", "256Ki", "1Mi", "4Mi"}
	register(&Probe{ID: "3.01-long-param-value", Group: "3 hostile input",
		Title:   "one parameter value of N bytes: X;A=bbbb…:v",
		RFC:     "robustness (no RFC limit); pass = finishes, time should scale ~linearly",
		Libs:    parsers,
		Args:    sizes,
		Timeout: 60,
		Run: func(lib, arg string) Result {
			return timedParse(lib, wrapLine("X;A="+strings.Repeat("b", sizeArg(arg))+":v"), nil)
		}})
	register(&Probe{ID: "3.02-many-params", Group: "3 hostile input",
		Title:   "a line of N bytes of short parameters: X;A=b;A=b;…:v",
		RFC:     "robustness; pass = finishes, time/memory should scale ~linearly",
		Libs:    parsers,
		Args:    append(append([]string{}, sizes...), "5Mi"),
		Timeout: 60,
		Run: func(lib, arg string) Result {
			return timedParse(lib, wrapLine("X"+strings.Repeat(";A=b", sizeArg(arg)/4)+":v"), nil)
		}})
	register(&Probe{ID: "3.03-comma-param-values", Group: "3 hostile input",
		Title:   "N comma-separated parameter values: X;A=b,b,b,…:v",
		RFC:     "robustness; pass = finishes, linear",
		Libs:    parsers,
		Args:    []string{"64Ki", "256Ki", "1Mi", "4Mi"},
		Timeout: 60,
		Run: func(lib, arg string) Result {
			return timedParse(lib, wrapLine("X;A="+strings.Repeat("b,", sizeArg(arg))+"b:v"), nil)
		}})
	register(&Probe{ID: "3.04-comma-text-values", Group: "3 hostile input",
		Title:   "CATEGORIES with N comma-separated values, read with the list accessor",
		RFC:     "robustness; §3.8.1.2 CATEGORIES is a TEXT list",
		Libs:    parsers,
		Args:    []string{"64Ki", "256Ki", "1Mi", "4Mi"},
		Timeout: 60,
		Run: func(lib, arg string) Result {
			n := sizeArg(arg)
			in := wrapLine("CATEGORIES:" + strings.Repeat("b,", n) + "b")
			got := -1
			var r Result
			switch lib {
			case GoIcal, GoIcalO:
				r = timedParse(lib, in, nil)
				r.Detail = "golang-ical has no list accessor; parse only"
				return r
			case This:
				t0 := time.Now()
				def := parseThis(in, nil)
				defMs := since(t0)
				t0 = time.Now()
				d := ical.NewDecoder(bytes.NewReader(in))
				d.MaxLineLength = 256 << 20
				cal, err := d.Decode()
				if err != nil {
					return failed(err)
				}
				for ev := range cal.Events() {
					got = len(ev.Categories())
				}
				r.Ms = since(t0)
				r.Detail = fmt.Sprintf("with default limits: %s in %.1f ms", short(def), defMs)
			case Emersion:
				t0 := time.Now()
				cal, err := eical.NewDecoder(bytes.NewReader(in)).Decode()
				if err != nil {
					return failed(err)
				}
				l, err := cal.Events()[0].Props.Get("CATEGORIES").TextList()
				if err != nil {
					return failed(err)
				}
				got = len(l)
				r.Ms = since(t0)
			}
			if got != n+1 {
				r.Outcome, r.Got = "wrong", fmt.Sprintf("list accessor returned %d values, want %d", got, n+1)
			} else {
				r.Outcome, r.Got = "pass", fmt.Sprintf("%d values", got)
			}
			return r
		}})
	register(&Probe{ID: "3.05-deep-nesting", Group: "3 hostile input",
		Title:   "N nested BEGIN:X lines with no END (unterminated nesting)",
		RFC:     "robustness; pass = an error without crashing (the input is truncated)",
		Libs:    parsers,
		Args:    []string{"100000", "1000000", "10000000"},
		Timeout: 60,
		Run: func(lib, arg string) Result {
			n, _ := strconv.Atoi(arg)
			in := []byte(calHead + strings.Repeat("BEGIN:X\r\n", n))
			r := timedParse(lib, in, nil)
			// Here an error is the right answer: the input is truncated.
			if r.Outcome == "error" {
				r.Outcome = "pass"
			} else if r.Outcome == "pass" {
				r.Outcome = "wrong"
			}
			return r
		}})
	register(&Probe{ID: "3.06-deep-nesting-balanced", Group: "3 hostile input",
		Title:   "N nested BEGIN:X … END:X (balanced)",
		RFC:     "robustness; either a clean error (a depth limit) or success, never a crash",
		Libs:    parsers,
		Args:    []string{"100000", "1000000"},
		Timeout: 60,
		Run: func(lib, arg string) Result {
			n, _ := strconv.Atoi(arg)
			in := []byte(calHead + strings.Repeat("BEGIN:X\r\n", n) + strings.Repeat("END:X\r\n", n) + "END:VCALENDAR\r\n")
			r := timedParse(lib, in, nil)
			if r.Outcome == "error" {
				r.Outcome = "pass"
				r.Got = "clean " + r.Got
			}
			return r
		}})

	// Truncated and malformed lines of the kind the audit found to panic
	// emersion/go-ical. Pass = an error or a recovery, never a panic.
	bad := map[string]string{
		"param-no-colon":     "DTSTART;TZID=America/New_York",
		"text-after-quote":   `X;A="b"c:v`,
		"param-at-eof":       "X;A=b",
		"param-no-equals":    "X;A:v",
		"quote-unterminated": `X;A="b:v`,
		"empty-name":         ":v",
		"name-only":          "X",
	}
	var keys []string
	for k := range bad {
		keys = append(keys, k)
	}
	register(&Probe{ID: "3.07-malformed-lines", Group: "3 hostile input",
		Title:   "truncated/malformed content lines (inside a VEVENT, and as the cut-off last line)",
		RFC:     "§3.1 grammar; pass = error or documented recovery, no panic",
		Libs:    parsers,
		Args:    keys,
		Timeout: 20,
		Run: func(lib, arg string) Result {
			line := bad[arg]
			mid := parseFeed(lib, wrapLine(line))
			cut := parseFeed(lib, []byte(calHead+"BEGIN:VEVENT\r\nUID:x\r\n"+line))
			return Result{Outcome: "pass", Got: fmt.Sprintf("mid-feed: %s | cut-off feed: %s", short(mid), short(cut))}
		}})
}
