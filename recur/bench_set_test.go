package recur

import (
	"testing"
	"time"
)

// A zoned weekly series queried for one month: the shape of a calendar
// feed request.
func BenchmarkSetTimesZoned(b *testing.B) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		b.Skip(err)
	}
	r, _ := ParseRule("FREQ=WEEKLY;BYDAY=MO,WE,FR")
	s := &Set{Start: dt("20100104T090000"), Location: ny, Rules: []Rule{r}}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, 0)
	b.ReportAllocs()
	for b.Loop() {
		for range s.Times(from, to) {
		}
	}
}
