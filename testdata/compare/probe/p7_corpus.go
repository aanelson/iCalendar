package main

// 7. Corpus robustness: parse one file with one library.

import (
	"fmt"
	"os"
	"time"

	ical "github.com/aanelson/iCalendar"
)

const ThisStrict = "this(strict)"

var corpusLibs = []string{This, ThisStrict, GoIcal, GoIcalO, Emersion}

func corpusOne(lib, path string) Result {
	b, err := os.ReadFile(path)
	if err != nil {
		return failed(err)
	}
	t0 := time.Now()
	var p parsed
	if lib == ThisStrict {
		p = parseThis(b, func(d *ical.Decoder) { d.Strict = true })
	} else {
		p = parseFeed(lib, b)
	}
	r := Result{Ms: since(t0), Extra: map[string]any{"cals": p.Cals, "events": len(p.Events)}}
	switch {
	case p.Err != nil:
		r.Outcome = "error"
		msg := p.Err.Error()
		if len(msg) > 200 {
			msg = msg[:200]
		}
		r.Detail = msg
	case p.Cals == 0:
		r.Outcome = "empty"
		r.Detail = "no error and no calendar"
	default:
		r.Outcome = "ok"
	}
	r.Got = fmt.Sprintf("%d cal, %d events", p.Cals, len(p.Events))
	return r
}
