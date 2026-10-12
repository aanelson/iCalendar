// Command driver runs every probe in its own subprocess with a timeout and
// writes raw results (logs/*.jsonl) and a summary matrix (logs/matrix.md).
//
//	go run ./driver                 probes (groups 1-6)
//	go run ./driver -corpus         the corpus (group 7)
//	go run ./driver -only '^3\.'    a subset, by probe-ID regexp
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Probe struct {
	ID      string   `json:"id"`
	Group   string   `json:"group"`
	Title   string   `json:"title"`
	RFC     string   `json:"rfc"`
	Libs    []string `json:"libs"`
	Args    []string `json:"args"`
	TZ      []string `json:"tz"`
	Timeout float64  `json:"timeout"`
	Race    bool     `json:"race"`
}

type Result struct {
	Outcome string         `json:"outcome"`
	Got     string         `json:"got,omitempty"`
	Want    string         `json:"want,omitempty"`
	Ms      float64        `json:"ms,omitempty"`
	Detail  string         `json:"detail,omitempty"`
	Extra   map[string]any `json:"extra,omitempty"`
}

type Task struct {
	Probe   string  `json:"probe"`
	Lib     string  `json:"lib"`
	Arg     string  `json:"arg,omitempty"`
	TZ      string  `json:"tz,omitempty"`
	File    string  `json:"file,omitempty"`
	Timeout float64 `json:"timeout"`
	Race    bool    `json:"race,omitempty"`
	Serial  bool    `json:"-"`

	Result
	WallMs float64 `json:"wall_ms"`
	RSSMB  float64 `json:"max_rss_mb"`
	Stderr string  `json:"stderr,omitempty"`
}

var (
	bin     = flag.String("bin", "bin/probe", "probe binary")
	raceBin = flag.String("racebin", "bin/probe-race", "probe binary built with -race")
	outDir  = flag.String("out", "logs", "output directory")
	jobs    = flag.Int("j", 6, "parallel subprocesses for non-timing probes")
	only    = flag.String("only", "", "regexp on probe IDs")
	corpus  = flag.Bool("corpus", false, "run the corpus instead of the probes")
	corpTO  = flag.Float64("corpus-timeout", 20, "per-file timeout, seconds")
)

// The repository root; run.sh runs everything from this directory.
const checkout = "../.."

func main() {
	flag.Parse()
	os.MkdirAll(*outDir, 0o755)
	if *corpus {
		runCorpus()
		return
	}
	out, err := exec.Command(*bin, "list").Output()
	if err != nil {
		panic(err)
	}
	var probes []Probe
	if err := json.Unmarshal(out, &probes); err != nil {
		panic(err)
	}
	re := regexp.MustCompile(*only)
	var tasks []*Task
	for _, p := range probes {
		if !re.MatchString(p.ID) {
			continue
		}
		args, tzs := p.Args, p.TZ
		if len(args) == 0 {
			args = []string{""}
		}
		if len(tzs) == 0 {
			tzs = []string{""}
		}
		serial := strings.HasPrefix(p.Group, "3") || p.ID == "6.06-month-of-1990-daily" || p.ID == "6.04-unmatchable"
		for _, lib := range p.Libs {
			for _, a := range args {
				for _, tz := range tzs {
					tasks = append(tasks, &Task{Probe: p.ID, Lib: lib, Arg: a, TZ: tz, Timeout: p.Timeout, Race: p.Race, Serial: serial})
				}
			}
		}
	}
	start := time.Now()
	var par, ser []*Task
	for _, t := range tasks {
		if t.Serial {
			ser = append(ser, t)
		} else {
			par = append(par, t)
		}
	}
	fmt.Fprintf(os.Stderr, "%d tasks (%d parallel, %d serial)\n", len(tasks), len(par), len(ser))
	runAll(par, *jobs)
	runAll(ser, 1)
	fmt.Fprintf(os.Stderr, "done in %v\n", time.Since(start).Round(time.Second))

	name := "results"
	if *only != "" {
		name = "results-" + regexp.MustCompile(`[^A-Za-z0-9.]+`).ReplaceAllString(*only, "_")
	}
	f, _ := os.Create(filepath.Join(*outDir, name+".jsonl"))
	for _, t := range tasks {
		b, _ := json.Marshal(t)
		f.Write(append(b, '\n'))
	}
	f.Close()
	md := matrix(probes, tasks)
	os.WriteFile(filepath.Join(*outDir, strings.Replace(name, "results", "matrix", 1)+".md"), []byte(md), 0o644)
	fmt.Println(md)
}

func runAll(tasks []*Task, n int) {
	ch := make(chan *Task)
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range ch {
				run(t)
				if t.Serial || t.Outcome == "hang" || t.Outcome == "crash" {
					fmt.Fprintf(os.Stderr, "  %-34s %-18s %-12s %-7s %8.0f ms  %s\n", t.Probe, t.Lib, t.Arg, t.Outcome, t.WallMs, clip(t.Got, 60))
				}
			}
		}()
	}
	for _, t := range tasks {
		ch <- t
	}
	close(ch)
	wg.Wait()
}

func run(t *Task) {
	b := *bin
	if t.Race {
		b = *raceBin
	}
	args := []string{"run", t.Probe, t.Lib, t.Arg}
	if t.File != "" {
		args = []string{"corpus", t.Lib, t.File}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(t.Timeout*float64(time.Second)))
	defer cancel()
	cmd := exec.CommandContext(ctx, b, args...)
	cmd.Env = os.Environ()
	if t.TZ != "" {
		cmd.Env = append(cmd.Env, "TZ="+t.TZ)
	}
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	t0 := time.Now()
	err := cmd.Run()
	t.WallMs = float64(time.Since(t0).Microseconds()) / 1000
	if cmd.ProcessState != nil {
		if ru, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage); ok {
			t.RSSMB = float64(ru.Maxrss) / (1 << 20) // bytes on darwin
		}
	}
	stderr := se.String()
	if len(stderr) > 2000 {
		stderr = stderr[:2000]
	}
	t.Stderr = stderr
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		t.Outcome = "hang"
		t.Got = fmt.Sprintf("killed after %.0f s timeout", t.Timeout)
		return
	case t.Race && strings.Contains(stderr, "WARNING: DATA RACE"):
		json.Unmarshal(so.Bytes(), &t.Result)
		t.Outcome = "wrong"
		t.Got = "DATA RACE reported by the race detector"
		t.Detail = firstRace(stderr)
		return
	case err != nil && so.Len() == 0:
		t.Outcome = "crash"
		t.Got = crashLine(stderr)
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			t.Detail = fmt.Sprintf("exit %d", ee.ExitCode())
		}
		return
	}
	if err := json.Unmarshal(so.Bytes(), &t.Result); err != nil {
		t.Outcome = "crash"
		t.Got = "bad output: " + clip(so.String(), 200)
	}
}

func crashLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(l, "fatal error") || strings.HasPrefix(l, "runtime:") || strings.HasPrefix(l, "panic") {
			return l
		}
	}
	return clip(s, 200)
}

func firstRace(s string) string {
	i := strings.Index(s, "WARNING: DATA RACE")
	s = s[i:]
	lines := strings.Split(s, "\n")
	var keep []string
	for _, l := range lines[1:] {
		if strings.Contains(l, ".go:") || strings.HasPrefix(strings.TrimSpace(l), "github.com") {
			keep = append(keep, strings.TrimSpace(l))
		}
		if len(keep) >= 6 {
			break
		}
	}
	return strings.Join(keep, " | ")
}

func clip(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// ---- report ----

func cell(t *Task) string {
	switch t.Outcome {
	case "hang":
		return "**HANG** (" + t.Got + ")"
	case "crash":
		return "**CRASH** " + clip(t.Got, 90)
	case "panic":
		return "**PANIC** " + clip(t.Detail, 90)
	}
	s := t.Outcome
	switch t.Outcome {
	case "pass":
		s = "pass"
	case "wrong":
		s = "**WRONG**"
	case "error":
		s = "error"
	}
	val := t.Got
	if t.Outcome == "error" || t.Outcome == "n/a" {
		val = t.Detail
	}
	if val != "" {
		s += ": " + clip(val, 140)
	}
	if t.Ms > 0 && (strings.HasPrefix(t.Probe, "3.") || strings.HasPrefix(t.Probe, "6.0")) {
		s += fmt.Sprintf(" [%.1f ms", t.Ms)
		if t.RSSMB > 0 && strings.HasPrefix(t.Probe, "3.") {
			s += fmt.Sprintf(", RSS %.0f MB", t.RSSMB)
		}
		s += "]"
	}
	if t.Outcome == "hang" || t.Outcome == "crash" {
		s += fmt.Sprintf(" [RSS %.0f MB]", t.RSSMB)
	}
	return strings.ReplaceAll(s, "|", "\\|")
}

func matrix(probes []Probe, tasks []*Task) string {
	byProbe := map[string][]*Task{}
	for _, t := range tasks {
		byProbe[t.Probe] = append(byProbe[t.Probe], t)
	}
	var b strings.Builder
	group := ""
	for _, p := range probes {
		ts := byProbe[p.ID]
		if len(ts) == 0 {
			continue
		}
		if p.Group != group {
			group = p.Group
			fmt.Fprintf(&b, "\n## %s\n", group)
		}
		fmt.Fprintf(&b, "\n### %s — %s\n\n_RFC basis: %s_\n\n", p.ID, p.Title, p.RFC)
		switch {
		case len(p.TZ) > 0:
			fmt.Fprintf(&b, "| lib | TZ=%s | TZ=%s | verdict |\n|---|---|---|---|\n", p.TZ[0], p.TZ[1])
			for _, lib := range p.Libs {
				var a, c *Task
				for _, t := range ts {
					if t.Lib == lib && t.TZ == p.TZ[0] {
						a = t
					}
					if t.Lib == lib && t.TZ == p.TZ[1] {
						c = t
					}
				}
				v := "host-independent (pass)"
				if a.Outcome != c.Outcome || a.Got != c.Got {
					v = "**HOST-DEPENDENT (wrong)**"
				}
				fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", lib, cell(a), cell(c), v)
			}
		case p.ID == "6.01-rfc-examples":
			fmt.Fprintf(&b, "| lib | exact matches of %d | failures |\n|---|---|---|\n", len(p.Args))
			for _, lib := range p.Libs {
				n := 0
				var bad []string
				for _, t := range ts {
					if t.Lib != lib {
						continue
					}
					if t.Outcome == "pass" {
						n++
					} else {
						bad = append(bad, fmt.Sprintf("%s: %s %s", strings.TrimPrefix(t.Arg, "rfc5545-3.8.5.3-"), t.Outcome, clip(t.Got+" "+t.Detail, 110)))
					}
				}
				fmt.Fprintf(&b, "| %s | %d / %d | %s |\n", lib, n, len(p.Args), strings.ReplaceAll(strings.Join(bad, "<br>"), "|", "\\|"))
			}
		default:
			args := p.Args
			if len(args) == 0 {
				args = []string{""}
			}
			fmt.Fprintf(&b, "| arg |")
			for _, lib := range p.Libs {
				fmt.Fprintf(&b, " %s |", lib)
			}
			fmt.Fprintf(&b, "\n|---|%s\n", strings.Repeat("---|", len(p.Libs)))
			for _, a := range args {
				fmt.Fprintf(&b, "| %s |", a)
				for _, lib := range p.Libs {
					for _, t := range ts {
						if t.Lib == lib && t.Arg == a {
							fmt.Fprintf(&b, " %s |", cell(t))
						}
					}
				}
				b.WriteString("\n")
			}
			if strings.HasPrefix(p.Group, "3") && p.ID < "3.07" {
				b.WriteString("\nScaling (time at the largest size that finished ÷ time at the size below it, for 4× the input; ~4 is linear, ~16 quadratic):\n\n")
				for _, lib := range p.Libs {
					var prev, last *Task
					for _, a := range args {
						for _, t := range ts {
							if t.Lib == lib && t.Arg == a && t.Outcome != "hang" && t.Outcome != "crash" && t.Ms > 0 {
								prev, last = last, t
							}
						}
					}
					hangs := []string{}
					for _, t := range ts {
						if t.Lib == lib && (t.Outcome == "hang" || t.Outcome == "crash") {
							hangs = append(hangs, t.Arg+" "+t.Outcome)
						}
					}
					if prev != nil && last != nil && prev.Ms > 0 {
						fmt.Fprintf(&b, "- %s: %s→%s ×%.1f (%.1f ms → %.1f ms)", lib, prev.Arg, last.Arg, last.Ms/prev.Ms, prev.Ms, last.Ms)
					} else {
						fmt.Fprintf(&b, "- %s: n/a", lib)
					}
					if len(hangs) > 0 {
						fmt.Fprintf(&b, "; did not finish: %s", strings.Join(hangs, ", "))
					}
					b.WriteString("\n")
				}
			}
		}
	}
	return b.String()
}

// ---- corpus ----

func corpusFiles() []string {
	var files []string
	dirs, _ := filepath.Glob(checkout + "/testdata/thirdparty/*/ics")
	dirs = append(dirs, checkout+"/testdata/rfc5545/ics")
	for _, d := range dirs {
		filepath.WalkDir(d, func(path string, de fs.DirEntry, err error) error {
			if err == nil && !de.IsDir() {
				files = append(files, path)
			}
			return nil
		})
	}
	sort.Strings(files)
	return files
}

// icsText reports whether a file is meant to be iCalendar text (the
// library's own corpus test selects the same).
func icsText(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ics", ".ical", ".ifb":
		return true
	case ".txt":
		return strings.Contains(path, "ical4j")
	}
	return false
}

func runCorpus() {
	libs := []string{"this", "this(strict)", "golang-ical", "golang-ical+opts", "emersion"}
	files := corpusFiles()
	var tasks []*Task
	for _, f := range files {
		for _, lib := range libs {
			tasks = append(tasks, &Task{Probe: "7-corpus", Lib: lib, File: f, Timeout: *corpTO})
		}
	}
	fmt.Fprintf(os.Stderr, "%d files × %d libs\n", len(files), len(libs))
	t0 := time.Now()
	runAll(tasks, *jobs)
	fmt.Fprintf(os.Stderr, "done in %v\n", time.Since(t0).Round(time.Second))
	f, _ := os.Create(filepath.Join(*outDir, "corpus.jsonl"))
	for _, t := range tasks {
		t.File = strings.TrimPrefix(t.File, checkout+"/")
		b, _ := json.Marshal(t)
		f.Write(append(b, '\n'))
	}
	f.Close()

	var b strings.Builder
	fmt.Fprintf(&b, "## 7 corpus robustness\n\n%d files under testdata/thirdparty/*/ics and testdata/rfc5545/ics; per-file subprocess, %.0f s timeout.\n", len(files), *corpTO)
	for _, subset := range []string{"all files", "iCalendar-text files (.ics/.ical/.ifb, ical4j .txt)", "other files (jCal .json, extensionless libical fuzz inputs, …)"} {
		cnt := map[string]map[string]int{}
		nfiles := 0
		for i, t := range tasks {
			in := subset == "all files" || (strings.HasPrefix(subset, "iCal") == icsText(t.File))
			if !in {
				continue
			}
			if i%len(libs) == 0 {
				nfiles++
			}
			if cnt[t.Lib] == nil {
				cnt[t.Lib] = map[string]int{}
			}
			cnt[t.Lib][t.Outcome]++
		}
		fmt.Fprintf(&b, "\n### %s (%d)\n\n| lib | ok (≥1 calendar) | empty success (no error, no calendar) | error | panic | crash | hang |\n|---|---|---|---|---|---|---|\n", subset, nfiles)
		for _, lib := range libs {
			c := cnt[lib]
			fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %d | %d |\n", lib, c["ok"], c["empty"], c["error"], c["panic"], c["crash"], c["hang"])
		}
	}
	// VEVENT counts on files every library reads.
	byFile := map[string]map[string]*Task{}
	for _, t := range tasks {
		if byFile[t.File] == nil {
			byFile[t.File] = map[string]*Task{}
		}
		byFile[t.File][t.Lib] = t
	}
	var allOK, differ int
	var examples []string
	var names []string
	for f := range byFile {
		names = append(names, f)
	}
	sort.Strings(names)
	for _, f := range names {
		m := byFile[f]
		ok := true
		for _, lib := range []string{"this", "golang-ical", "emersion"} {
			if m[lib].Outcome != "ok" {
				ok = false
			}
		}
		if !ok {
			continue
		}
		allOK++
		ev := func(lib string) int { return int(m[lib].Extra["events"].(float64)) }
		if ev("this") != ev("golang-ical") || ev("this") != ev("emersion") {
			differ++
			if len(examples) < 15 {
				examples = append(examples, fmt.Sprintf("%s: this %d, golang-ical %d, emersion %d", f, ev("this"), ev("golang-ical"), ev("emersion")))
			}
		}
	}
	fmt.Fprintf(&b, "\nFiles that this, golang-ical (default) and emersion all read without error: %d. VEVENT counts differ on %d of them.\n", allOK, differ)
	for _, e := range examples {
		fmt.Fprintf(&b, "- %s\n", e)
	}
	// Every panic/crash/hang, and files only one side reads.
	fmt.Fprintf(&b, "\n### Panics, crashes and hangs\n\n")
	for _, f := range names {
		for _, lib := range libs {
			t := byFile[f][lib]
			if t.Outcome == "panic" || t.Outcome == "crash" || t.Outcome == "hang" {
				fmt.Fprintf(&b, "- %s — %s: %s %s\n", f, lib, t.Outcome, clip(t.Detail+" "+t.Got, 120))
			}
		}
	}
	os.WriteFile(filepath.Join(*outDir, "corpus.md"), []byte(b.String()), 0o644)
	fmt.Println(b.String())
}
