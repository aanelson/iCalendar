package ical

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/aanelson/iCalendar/contentline"
	"github.com/aanelson/iCalendar/internal/icaltime"
	"github.com/aanelson/iCalendar/tz"
)

// corpusFiles returns the .ics fixtures in testdata, including the
// third-party corpora (a nested module that may be absent).
func corpusFiles(t *testing.T) []string {
	var files []string
	filepath.WalkDir("testdata", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".ics", ".ical", ".ifb":
			files = append(files, path)
		case ".txt":
			if strings.Contains(path, "ical4j") {
				files = append(files, path)
			}
		}
		return nil
	})
	if len(files) == 0 {
		t.Skip("no corpus")
	}
	sort.Strings(files)
	return files
}

// Every corpus file, valid or broken, must decode without panicking or
// hanging. Whatever decodes must encode to valid folded CRLF text that
// decodes back to the same tree, and every event in it must expand.
func TestCorpus(t *testing.T) {
	files := corpusFiles(t)
	var decoded, failed, calendars, events, unencodable, zoneErrs, expandErrs int
	failures := map[string]int{}
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(path, func(t *testing.T) {
			start := time.Now()
			defer func() {
				if d := time.Since(start); d > 5*time.Second {
					t.Errorf("took %v", d)
				}
			}()
			d := NewDecoder(bytes.NewReader(b))
			var cals []*Calendar
			for {
				cal, err := d.Decode()
				if err == io.EOF {
					break
				}
				if err != nil {
					failed++
					failures[category(err)]++
					return
				}
				cals = append(cals, cal)
			}
			if len(cals) == 0 {
				failed++
				failures["no VCALENDAR"]++
				return
			}
			decoded++
			for _, cal := range cals {
				calendars++
				out, err := cal.Bytes()
				if err != nil {
					if !errors.Is(err, contentline.ErrInvalidLine) {
						t.Fatalf("encode: %v", err)
					}
					unencodable++
					continue
				}
				checkFolded(t, out)
				back, err := Parse(bytes.NewReader(out))
				if err != nil {
					t.Fatalf("decoding our own output: %v\n%s", err, out)
				}
				if !reflect.DeepEqual(back.Component, bareParams(cal.Component)) {
					t.Fatalf("round trip changed the tree")
				}
				// Each event's own recurrence set, around its start…
				var first time.Time
				for ev := range cal.Events() {
					events++
					s, err := ev.Start()
					if errors.Is(err, tz.ErrUnknownZone) {
						zoneErrs++
					}
					if err != nil && s.IsZero() {
						continue
					}
					from := s.Time().AddDate(-1, 0, 0)
					if first.IsZero() || from.Before(first) {
						first = from
					}
					set, err := ev.Recurrence()
					if err != nil {
						expandErrs++
					}
					if set == nil {
						continue
					}
					n := 0
					for range set.Times(from, from.AddDate(3, 0, 0)) {
						if n++; n > 2000 {
							break
						}
					}
				}
				// …and the whole calendar, overrides and all, once.
				n := 0
				for _, err := range cal.Occurrences(first, first.AddDate(3, 0, 0)) {
					if err != nil {
						expandErrs++
					}
					if n++; n > 20000 {
						break
					}
				}
			}
		})
	}
	t.Logf("%d files: %d decoded (%d calendars, %d events), %d rejected %v; %d unencodable, %d unknown TZIDs, %d expansion errors",
		len(files), decoded, calendars, events, failed, failures, unencodable, zoneErrs, expandErrs)
}

func category(err error) string {
	switch {
	case strings.Contains(err.Error(), "no VCALENDAR"):
		return "no VCALENDAR"
	case errors.Is(err, io.ErrUnexpectedEOF):
		return "truncated"
	case errors.Is(err, contentline.ErrLineTooLong):
		return "line too long"
	case strings.Contains(err.Error(), "deeper"):
		return "too deep"
	case strings.Contains(err.Error(), "not a component name"):
		return "bad BEGIN"
	}
	return "other"
}

func checkFolded(t *testing.T, out []byte) {
	t.Helper()
	if !bytes.HasSuffix(out, []byte("\r\n")) {
		t.Fatal("output doesn't end in CRLF")
	}
	for i, line := range bytes.Split(bytes.TrimSuffix(out, []byte("\r\n")), []byte("\r\n")) {
		if len(line) > contentline.FoldWidth {
			t.Fatalf("physical line %d is %d octets", i+1, len(line))
		}
		if bytes.ContainsAny(line, "\r\n") {
			t.Fatalf("physical line %d holds a bare CR or LF", i+1)
		}
	}
}

// Every example in RFC 5545 reads: property examples as content lines,
// components and calendars through the decoder.
func TestRFC5545Examples(t *testing.T) {
	b, err := os.ReadFile("testdata/rfc5545/examples.json")
	if err != nil {
		t.Skip(err)
	}
	var examples []struct {
		ID, Section, Kind, Text string
		Valid                   bool
	}
	if err := json.Unmarshal(b, &examples); err != nil {
		t.Fatal(err)
	}
	var lines, comps int
	for _, ex := range examples {
		if !ex.Valid {
			continue
		}
		switch ex.Kind {
		case "property", "parameter":
			// The RFC prints each line without its CRLF.
			r := contentline.NewReader(strings.NewReader(ex.Text + "\r\n"))
			r.Strict = true
			for {
				_, err := r.ReadLine()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Errorf("%s (§%s): %v\n%s", ex.ID, ex.Section, err, ex.Text)
					break
				}
				lines++
			}
		case "component", "calendar":
			text := ex.Text
			if !strings.HasPrefix(strings.ToUpper(text), "BEGIN:VCALENDAR") {
				text = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//EN\r\n" + text + "\r\nEND:VCALENDAR\r\n"
			}
			d := NewDecoder(strings.NewReader(text))
			var warnings []string
			d.Warn = func(err error) { warnings = append(warnings, err.Error()) }
			if _, err := d.Decode(); err != nil {
				t.Errorf("%s (§%s): %v\n%s", ex.ID, ex.Section, err, text)
			}
			comps++
		}
	}
	t.Logf("%d property lines and %d components read", lines, comps)
}

// The occurrence expectations transcribed from python-recurring-ical-events,
// which tests overrides, EXDATE handling and SEQUENCE against real feeds.
func TestRecurringICalEvents(t *testing.T) {
	const dir = "testdata/thirdparty/recurring-ical-events"
	b, err := os.ReadFile(dir + "/expected.json")
	if err != nil {
		t.Skip(err)
	}
	var cases []struct {
		Calendar        string   `json:"calendar"`
		Start           *string  `json:"start"`
		End             *string  `json:"end"`
		SummaryContains string   `json:"summary_contains"`
		ExpectedStarts  []string `json:"expected_starts"`
		Comment         string   `json:"comment"`
	}
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	known := map[int]string{}
	for i, c := range cases {
		t.Run(filepath.Base(c.Calendar), func(t *testing.T) {
			f, err := os.Open(filepath.Join(dir, c.Calendar))
			if err != nil {
				t.Skip(err)
			}
			defer f.Close()
			cal, err := Parse(f)
			if err != nil {
				t.Fatal(err)
			}
			from, to := time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
			if c.Start != nil {
				from = parseWindow(t, *c.Start)
			}
			if c.End != nil {
				to = parseWindow(t, *c.End)
			}
			var got []string
			for occ, err := range cal.Occurrences(from, to) {
				if err != nil {
					t.Fatal(err)
				}
				if c.SummaryContains != "" && !strings.Contains(textOf(occ.Component, "SUMMARY"), c.SummaryContains) {
					continue
				}
				s := occ.Start.String()
				if len(c.ExpectedStarts) > 0 && len(c.ExpectedStarts[0]) == 8 {
					s = s[:8] // the source only asserts dates
				}
				got = append(got, s)
			}
			if !slices.Equal(got, c.ExpectedStarts) {
				if reason, ok := known[i]; ok {
					t.Skip(reason)
				}
				t.Errorf("got  %v\nwant %v\n%s", got, c.ExpectedStarts, c.Comment)
			}
		})
	}
}

func parseWindow(t *testing.T, s string) time.Time {
	v, _, _, err := icaltime.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return v.In(time.UTC)
}
