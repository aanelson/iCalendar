// Command probe runs one probe against one library and prints a JSON result.
// It is driven by ../driver, which runs every probe in its own subprocess
// with a timeout, so a hang or a fatal crash in one library is recorded and
// doesn't stop the run.
//
//	probe list                      probes as JSON
//	probe run <probe> <lib> [arg]   one result as JSON
//	probe corpus <lib> <file>       parse one corpus file
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"sort"
	"time"
)

// Libraries. "golang-ical+opts" is golang-ical with every opt-in it offers
// for real-world feeds: the consumer app's lenient parsing
// (AcceptUnknownPropertyHandler, LooseParser fallback) plus
// WithWindowsTimezoneMapping.
const (
	This     = "this"
	GoIcal   = "golang-ical"
	GoIcalO  = "golang-ical+opts"
	Emersion = "emersion"
	RRuleGo  = "rrule-go"
)

type Result struct {
	Outcome string         `json:"outcome"` // pass, wrong, error, panic, n/a, info
	Got     string         `json:"got,omitempty"`
	Want    string         `json:"want,omitempty"`
	Ms      float64        `json:"ms,omitempty"`
	Detail  string         `json:"detail,omitempty"`
	Extra   map[string]any `json:"extra,omitempty"`
}

type Probe struct {
	ID      string   `json:"id"`
	Group   string   `json:"group"`
	Title   string   `json:"title"`
	RFC     string   `json:"rfc"`
	Libs    []string `json:"libs"`
	Args    []string `json:"args,omitempty"`    // run once per arg (sizes, …)
	TZ      []string `json:"tz,omitempty"`      // run once per TZ env value
	Timeout float64  `json:"timeout"`           // seconds
	Race    bool     `json:"race,omitempty"`    // needs the -race binary
	Run     func(lib, arg string) Result `json:"-"`
}

var probes = map[string]*Probe{}

func register(p *Probe) {
	if p.Timeout == 0 {
		p.Timeout = 20
	}
	probes[p.ID] = p
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: probe list | run <probe> <lib> [arg] | corpus <lib> <file>")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "list":
		var ps []*Probe
		for _, p := range probes {
			ps = append(ps, p)
		}
		sort.Slice(ps, func(i, j int) bool { return ps[i].ID < ps[j].ID })
		json.NewEncoder(os.Stdout).Encode(ps)
	case "run":
		p := probes[os.Args[2]]
		if p == nil {
			fmt.Fprintln(os.Stderr, "no probe", os.Args[2])
			os.Exit(2)
		}
		arg := ""
		if len(os.Args) > 4 {
			arg = os.Args[4]
		}
		emit(safeRun(func() Result { return p.Run(os.Args[3], arg) }))
	case "corpus":
		emit(safeRun(func() Result { return corpusOne(os.Args[2], os.Args[3]) }))
	default:
		os.Exit(2)
	}
}

func emit(r Result) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	if r.Extra == nil {
		r.Extra = map[string]any{}
	}
	r.Extra["total_alloc_mb"] = float64(ms.TotalAlloc) / (1 << 20)
	b, _ := json.Marshal(r)
	os.Stdout.Write(append(b, '\n'))
}

// safeRun turns a panic into a "panic" result. Fatal runtime errors (stack
// overflow, out of memory) can't be recovered; the driver sees those as a
// crashed subprocess.
func safeRun(f func() Result) (r Result) {
	defer func() {
		if v := recover(); v != nil {
			stack := string(debug.Stack())
			if len(stack) > 1500 {
				stack = stack[:1500]
			}
			r = Result{Outcome: "panic", Detail: fmt.Sprint(v), Extra: map[string]any{"stack": stack}}
		}
	}()
	return f()
}

func since(t time.Time) float64 { return float64(time.Since(t).Microseconds()) / 1000 }

func pass(got string) Result          { return Result{Outcome: "pass", Got: got} }
func wrong(got, want string) Result   { return Result{Outcome: "wrong", Got: got, Want: want} }
func failed(err error) Result         { return Result{Outcome: "error", Detail: err.Error()} }
func na(why string) Result            { return Result{Outcome: "n/a", Detail: why} }
func check(got, want string) Result {
	if got == want {
		return Result{Outcome: "pass", Got: got, Want: want}
	}
	return wrong(got, want)
}
