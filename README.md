# iCalendar

A dependency-free Go library for iCalendar (RFC 5545): parsing, writing,
time zones, and recurrence expansion with its own date generator.

```go
import ical "github.com/aanelson/iCalendar"

cal, err := ical.Parse(r)
for occ, err := range cal.Occurrences(from, to) {
	// every instance of every event overlapping [from, to), sorted,
	// with RECURRENCE-ID overrides, EXDATE, RDATE and VTIMEZONEs applied
}
```

Status: the foundation is in place and tested (see below); the API may
still change before a first tagged release.

## Why another one

The existing Go libraries panic or go quadratic on hostile input, read
floating times in `time.Local`, ignore VTIMEZONE, and lean on rrule-go,
which walks every recurrence from DTSTART and has open bugs that loop
forever. This library:

- **Owns date generation.** Its recurrence engine seeks straight to the
  window you ask for (a month of a daily series begun in 1990 costs ~1 µs)
  and can cache what it learns about COUNT rules (~140× faster on repeat
  queries). Every iteration terminates, provably.
- **Follows RFC 5545 and its errata,** and where the RFC is silent, the
  libraries built for iCalendar (libical, ical.js, ical.net). It passes all
  43 RFC recurrence examples and 1,217 vectors from seven other libraries'
  test suites, with every deliberate difference documented.
- **Is safe on untrusted feeds.** Linear-time parsing, fuzzed; a truncated
  feed is an error, never a shorter calendar; bounded line length and
  nesting; output that can't inject lines.
- **Compiles VTIMEZONE into a real `*time.Location`**, so Outlook's
  custom zones work; maps Windows zone names through CLDR; never guesses.
- **Round-trips losslessly:** unknown properties, parameters and
  components survive, in order.

## Packages

| Package | |
|---|---|
| [`ical`](.) | Model, `Decoder`/`Encoder`, value codecs, typed views (`Event`, `Todo`, …), `Expander` |
| [`recur`](recur) | `Rule` (RRULE), the expansion engine, `Set`, `Iterator`, `Cache` |
| [`tz`](tz) | TZID resolution: VTIMEZONE compiler, IANA and Windows names |
| [`civil`](civil) | Zone-less dates and times; RFC 5545 wall-clock → instant |
| [`contentline`](contentline) | The content-line layer (folding, parameters, RFC 6868) |

## Documentation

- [Architecture](docs/architecture.md): design, semantics decisions, caching, roadmap
- [API survey](docs/api-survey.md): what this API borrows from ical.js, python-icalendar, dateutil, ical4j, sabre/vobject, ical.net, libical and the Go libraries
- [RFC texts](docs/rfc/) and [RFC 5545 errata](docs/rfc/rfc5545-errata.md)

## Testing

```sh
go test ./...
go test ./recur -run '^$' -fuzz FuzzRule
```

The third-party fixtures in `testdata/thirdparty` (a nested module, with
each source's licence and provenance) are used when present and skipped
otherwise. See the testing section of the architecture document for what
each suite covers.

## Licence

BSD 3-Clause; see [LICENSE](LICENSE). The test fixtures in
`testdata/thirdparty` keep their own licences.
