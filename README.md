# iCalendar

[![CI](https://github.com/aanelson/iCalendar/actions/workflows/ci.yml/badge.svg)](https://github.com/aanelson/iCalendar/actions/workflows/ci.yml)

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

The other Go options are [arran4/golang-ical](https://github.com/arran4/golang-ical),
[emersion/go-ical](https://github.com/emersion/go-ical) and
[teambition/rrule-go](https://github.com/teambition/rrule-go), which both
of the others use for recurrence. [`testdata/compare`](testdata/compare)
runs the same probes, with expectations from the RFC, against all four.
This library gave no wrong value, panic or hang. golang-ical failed 12 of
them (10 with all its options), emersion/go-ical 19, and rrule-go 7 of the
recurrence probes. The ones that matter most:

| | this | golang-ical | emersion/go-ical | rrule-go |
|---|---|---|---|---|
| `TZID=America/New_York:20070311T023000`, in the spring-forward gap (§3.3.5) | 07:30Z | 06:30Z | 06:30Z | 06:30Z |
| A floating time or a DATE | the same on every host | read in the host's `time.Local` | the same on every host | – |
| An Outlook `Customized Time Zone` VTIMEZONE | used | ignored: error | ignored: error | – |
| `DURATION:P1D` across a DST change (§3.3.6) | 23 or 25 hours | not read | 24 hours | – |
| A feed cut off before `END:VCALENDAR` | error | success | `io.EOF`: no calendars | – |
| One 1 MiB parameter / 4 MiB of short parameters (this library: with `MaxLineLength` raised; by default it refuses lines over 1 MiB at once) | 1.4 ms / 94 ms | 2.1 ms / hangs | 26 s / 54 ms | – |
| CR or LF in a value or parameter, on write | escaped | a lone CR starts a new line | a parameter's CRLF starts a new line | – |
| `FREQ=HOURLY;INTERVAL=24;BYHOUR=11` from 09:00 | no instances, at once | – | hangs | hangs |
| One month of a daily series begun in 1990 | 1.5–5 µs | – | 2.4 ms | 2.2 ms |
| `FREQ=YEARLY;BYMONTH=2;BYDAY=5MO`, unbounded | 299 instances, to 9999 | – | 11, ending in 2304 | 11, ending in 2304 |

This library:

- **Owns date generation.** Its recurrence engine seeks straight to the
  window you ask for, and can cache what it learns about COUNT rules (the
  series above with a COUNT: ~6 µs cached, ~370 µs not). Every iteration
  terminates, provably.
- **Follows RFC 5545 and its errata,** and where the RFC is silent, the
  libraries built for iCalendar (libical, ical.js, ical.net). The
  conformance tests check RFC sentences one by one, with expectations
  worked out from the RFC rather than the code. An independent, naive
  implementation of §3.3.10 agrees with the engine on 300,000 random
  rules. All 43 RFC recurrence examples pass, and 992 vectors from seven
  other libraries' test suites match; the 28 deliberate differences are
  documented.
- **Is safe on untrusted feeds.** Linear-time parsing, fuzzed; a truncated
  feed is an error, never a shorter calendar, and input that isn't a
  calendar is an error, not an empty one; bounded line length and nesting;
  output that can't inject lines.
- **Compiles VTIMEZONE into a real `*time.Location`**, so Outlook's
  custom zones work; maps Windows zone names through CLDR; never guesses.
  And the reverse: `cal.AddTimezones(nil)` writes the VTIMEZONE RFC 5545
  requires for each TZID a calendar uses, from the Go zone, so a
  recurring event written in local time stays at that local time across
  DST changes.
- **Round-trips losslessly:** unknown properties, parameters and
  components survive, in order.

Not done yet (see the [roadmap](docs/architecture.md#roadmap)):

- **Validation.** A calendar that breaks a §3.6/§3.8 property rule (no
  UID, `PRIORITY:10`, DTEND before DTSTART, a property given twice) is
  read, not reported. Syntax errors are reported.
- **RANGE=THISANDFUTURE** overrides apply to their own instance only.
- The Expander does VEVENT; VTODO and VJOURNAL without DTSTART aren't
  expanded yet.
- RFC 7986, RFC 7529 (RSCALE), jCal/xCal and iTIP helpers. Their
  properties already round-trip as raw values.

## Packages

| Package | |
|---|---|
| [`ical`](.) | Model, `Decoder`/`Encoder`, value codecs, typed views (`Event`, `Todo`, …), `Expander` |
| [`recur`](recur) | `Rule` (RRULE), the expansion engine, `Set`, `Iterator`, `Cache` |
| [`tz`](tz) | TZID resolution: VTIMEZONE compiler, IANA and Windows names; `*time.Location` → VTIMEZONE |
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
go test ./recur -run Conformance -recur.conformance=300000   # the long RRULE differential run
testdata/compare/run.sh                                      # the comparison, ~15 min
```

The third-party fixtures in `testdata/thirdparty` (a nested module, with
each source's licence and provenance) are used when present and skipped
otherwise. The comparison in `testdata/compare` is another nested module,
since it depends on the other libraries; its last results are in
`testdata/compare/logs`. See the testing section of the architecture
document for what each suite covers.

## Licence

BSD 3-Clause; see [LICENSE](LICENSE). The test fixtures in
`testdata/thirdparty` keep their own licences.
