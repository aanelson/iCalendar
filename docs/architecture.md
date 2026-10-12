# Architecture

This document describes the design of `github.com/aanelson/iCalendar`
(package `ical`): a dependency-free Go implementation of iCalendar,
starting with RFC 5545 and built so later RFCs can be added without
reworking the core.

It is a living document. Anything marked **(planned)** is designed but not
built yet; everything else describes code in this repository and is
covered by its tests.

- The spec texts are in [`docs/rfc/`](rfc/), with RFC 5545's errata
  summarized in [`docs/rfc/rfc5545-errata.md`](rfc/rfc5545-errata.md).
- The survey of existing libraries this API borrows from is in
  [`docs/api-survey.md`](api-survey.md).
- Test fixtures and where they come from: [`testdata/`](../testdata/).

## Goals

1. **Correct RFC 5545.** Follow the RFC and its verified errata. Where the
   RFC is silent, follow the reading the iCalendar-focused libraries
   (libical, ical.js, ical.net, sabre/vobject) agree on, and record it here
   and in the test vectors.
2. **The library owns date generation.** Recurrence expansion (RRULE,
   RDATE, EXDATE, EXRULE, RECURRENCE-ID overrides, VTIMEZONE observances)
   is implemented here, not delegated to rrule-go. Owning the generator is
   what makes the two big wins possible: seeking straight to a window
   without walking from DTSTART, and caching what a walk learns.
3. **Safe on untrusted input.** Parsing is linear-time, never panics,
   bounds memory, and never reports a truncated file as a shorter one.
   Every iteration terminates, even for rules that can never match.
4. **Idiomatic Go.** `io.Reader`/`io.Writer` decoders and encoders, plain
   structs with exported fields, `time.Time` and `*time.Location` at the
   edges, range-over-func iterators for anything that can be unbounded,
   errors as values, no hidden global mutable state, zero dependencies.
5. **Lossless round trips.** Unknown properties, parameters, components
   and `X-` extensions survive decode → encode unchanged and in order.
6. **Extensible** to the other iCalendar RFCs (7986, 5546, 6868, 7529,
   9073, 9074, 9253, 7953, jCal/xCal) as additive packages.

Not goals for now: CalDAV, iTIP workflows, iMIP mail, vCard. The
content-line layer is shared with vCard, so `contentline` is reusable for
it.

## Why not the existing Go libraries

Two audits done for the consuming app tested the Go options against
hostile and real-world input. Their findings drove these decisions, and
[`testdata/compare`](../testdata/compare) re-measures each row against
golang-ical v0.3.7, emersion/go-ical and rrule-go v1.8.2 (last run
2026-10-11; results in `testdata/compare/logs`):

| Problem found | In | Response here |
|---|---|---|
| Panics on truncated or malformed lines | emersion/go-ical | Hand-written byte scanner; fuzzed (`FuzzReader`, `FuzzDecode`) |
| Quadratic time on a long parameter value (1 MB → 25 s) | emersion/go-ical | Values are sub-slices of the line; 1 MiB parameter decodes in ms (`TestHostileFeeds`) |
| Quadratic time and allocation on many parameters or many parameter values (a 5 MiB line: 4 min, 9.8 TB allocated) | arran4/golang-ical | No per-parameter copy; `TestHostileInputsAreLinear` checks 1 vs 4 MiB scaling |
| Regexes on every name and value | arran4/golang-ical | Byte loops; ~740 MB/s line scanning, ~290 MB/s full tree |
| Empty input, or a feed missing `END:VCALENDAR`, is a silent success; every truncation is a bare `io.EOF` | golang-ical; emersion/go-ical | Missing `END:VCALENDAR` wraps `io.ErrUnexpectedEOF`, also in lenient mode; input that isn't a calendar is an error |
| `SUMMARY:Dinner, drinks` → `"Dinner"` | emersion/go-ical | Values stay raw; `Text()` never splits, `Texts()` always does |
| Floating times and dates read in `time.Local` | arran4/golang-ical | `time.Local` is never read |
| VTIMEZONE ignored (both); `TZID=Eastern Standard Time` fails (emersion/go-ical, and golang-ical without its `WithWindowsTimezoneMapping` option) | both | VTIMEZONE compiled to a real `*time.Location`; CLDR Windows table |
| `time.LoadLocation` on every property (70–76% of CPU reading 20,000 zoned events) | both | Zone lookups cached, failures included |
| `DURATION:P1D` treated as 24 hours | emersion/go-ical | Nominal and exact parts kept apart |
| Lowercase names fail the whole feed | arran4/golang-ical | Names are case-insensitive (§2) |
| Writer doesn't fold (emersion/go-ical) or writes LF line ends by default (golang-ical) | both | Folds at 75 octets on rune boundaries, always CRLF |
| Writer lets a CR or LF start a new line: a lone CR in a value (golang-ical), a CRLF in a parameter (emersion/go-ical) | both | TEXT escaped, parameters RFC 6868-encoded, other control characters refused |
| A time in a spring-forward gap goes back an hour (`02:30` → 01:30 EST, not 03:30 EDT) | all three | §3.3.5 resolution in `civil.DateTime.In`; erratum 4271 for recurrences |
| rrule-go walks from DTSTART on every query | teambition/rrule-go | Seeks to the window (~1 µs for a month of a 1990 daily series) |
| rrule-go loops on `FREQ=HOURLY;INTERVAL=24;BYHOUR=11` | teambition/rrule-go | Static check plus a proven termination bound |
| rrule-go duplicates instances for `BYSECOND=0,0` | teambition/rrule-go | Every BYxxx list is a set |
| rrule-go walks to year 9999 on unmatchable rules | teambition/rrule-go | Detected up front, or bounded and remembered |
| rrule-go's iterator mutates its rule (a data race when goroutines share one) | teambition/rrule-go | Rules are values; iterators hold their own state |
| rrule-go silently ends unbounded rules ~292 years after DTSTART | teambition/rrule-go | Iteration runs to year 9999 |

## Package layout

```
github.com/aanelson/iCalendar
├── ical (root)        model, Decoder/Encoder, values, typed views, occurrences
├── civil              zone-less Date, Time, DateTime; RFC 5545 wall-clock → instant
├── contentline        RFC 5545 §3.1 content lines: unfold/fold, scan, write
├── recur              RRULE values, the recurrence engine, Set, Iterator, Cache
├── tz                 TZID → *time.Location: VTIMEZONE compiler, IANA, Windows;
│                      *time.Location → VTIMEZONE
│   └── internal/gen   generator for the CLDR Windows zone table
├── internal/icaltime  basic-format DATE / DATE-TIME text, shared by the above
├── docs/              this document, the API survey, the RFC texts
└── testdata/          RFC fixtures; testdata/thirdparty is a nested module
```

Dependencies point one way, so each layer is usable alone:

```
             ical
           /  |   \
         tz   |    contentline
        /  \  |
    recur   \ |
        \    \|
         civil   (+ internal/icaltime)
```

- `civil` and `contentline` use only the standard library.
- `recur` computes in wall-clock time, as RFC 5545 defines recurrence. It
  touches `*time.Location` only to compare a UTC `UNTIL` and to resolve
  instances to instants.
- `tz` uses `recur` to expand VTIMEZONE observances, from a plain-struct
  description of a VTIMEZONE, so it doesn't import the model.
- `ical` turns components into `tz.Definition` and `recur.Set` values.

## Layer 1: content lines (`contentline`)

```go
r := contentline.NewReader(src)    // fields: Strict, Warn, MaxLineLength
line, err := r.ReadLine()          // Name (upper case), Params, Value (raw), Pos
w := contentline.NewWriter(dst)
err = w.WriteLine(line); w.Flush()
```

- One left-to-right byte scan, no regexes. Names are interned so common
  ones don't allocate.
- Unfolding accepts CRLF, LF and lone CR (`Strict`: CRLF only), and SPACE
  or HTAB continuations. It works on bytes, so a fold inside a UTF-8
  sequence comes back whole.
- `MaxLineLength` (default 1 MiB, unfolded) is never recovered from.
- DQUOTE-quoted parameter values; RFC 6868 `^n ^' ^^` decoded on read and
  encoded on write.
- A UTF-8 BOM at the start is skipped. Any other junk is a syntax error.
- Lenient mode (the default) recovers and calls `Warn` with a
  `*SyntaxError`: an unparseable line is skipped; a parameter with no `=`,
  text after a closing quote, a quote inside an unquoted value, control
  characters in parameters and values, empty lines and stray continuations
  are read the way most readers read them. `Strict` makes each an error,
  and also rejects malformed UTF-8, which lenient mode keeps as it is.
- The writer folds at 75 octets on rune boundaries, always writes CRLF,
  quotes parameter values containing `,;:`, and rejects names that
  aren't tokens and any control character except HTAB, so no value can
  start a line of its own.

## Layer 2: the document model (`ical`)

A **generic tree with raw values**, with **typed views** on top. This is
the shape of ical.js, sabre/vobject and emersion/go-ical, and the only one
that round-trips unknown content losslessly.

```go
type Component struct { Name string; Props Props; Children []*Component }
type Property  struct { Name string; Params Params; Value string } // Value still escaped
type Params []Param
type Param     struct { Name string; Values []string }             // unquoted, RFC 6868-decoded
```

- **Ordered slices, not maps**: order survives a round trip and encoding
  is deterministic. `Props` and `Params` have `Get`, `All`, `Set`, `Add`
  and `Del`; names compare case-insensitively.
- **Raw values, decoded on demand**: decoding never fails over a value
  nobody reads, and unknown value types survive untouched.

  ```go
  p.Text() string                       // never fails; \: reads as :
  p.Texts() []string                    // CATEGORIES, RESOURCES
  p.DateTime(r tz.Resolver) (DateTime, error)
  p.DateTimes(r tz.Resolver) ([]DateTime, error)
  p.Periods(r tz.Resolver) ([]Period, error)
  p.Duration() (Duration, error)
  p.Recur() (recur.Rule, error)
  p.Int(), p.Float(), p.Bool(), p.UTCOffset(), p.Binary(), p.Geo()
  p.SetText, SetTexts, SetDateTime, SetDateTimes, SetDuration, SetRecur, …
  ```

  `DateTime` takes the resolver explicitly (nil means `tz.Load`), so a
  zoned value is never silently left unresolved.

- **Typed views** name the RFC's properties and apply its defaults:

  ```go
  cal.Events() iter.Seq[Event]          // also Todos, Journals, Timezones
  type Event struct{ Item }             // Item: what VEVENT/VTODO/VJOURNAL share
  ev.UID(), ev.Summary(), ev.Categories(), ev.Sequence(), ev.Alarms(), …
  ev.Start() (DateTime, error)
  ev.End() (DateTime, error)            // DTEND, else DTSTART+DURATION, else §3.6.1 defaults
  ev.Duration() (Duration, error)
  ev.RecurrenceID() (DateTime, bool, error)
  ev.Recurrence() (*recur.Set, error)
  ev.SetSummary(…), ev.SetStart(…), ev.SetRule(…), …
  ```

### Decoder and Encoder

Configuration is in exported fields, like `encoding/xml.Decoder`:

```go
dec := ical.NewDecoder(r)
dec.Strict = false                 // default: recover, and report to Warn
dec.Warn = func(err error) { … }   // each recovery, as a *SyntaxError
dec.MaxLineLength, dec.MaxDepth    // defaults 1 MiB, 32
cal, err := dec.Decode()           // io.EOF when no calendar is left
cal, err := ical.Parse(r)          // exactly one VCALENDAR
err = cal.AddTimezones(nil)        // before writing: a VTIMEZONE for each TZID used
err = ical.NewEncoder(w).Encode(cal)
b, err := cal.Bytes()
```

- A stream may hold several VCALENDARs (§3.4); `Decode` returns them in
  turn.
- **Truncation is never success.** Input that ends inside a component is
  an error wrapping `io.ErrUnexpectedEOF`, in both modes. Empty or blank
  input is `io.EOF`; input with content but no VCALENDAR at all (an HTML
  error page, a vCard, a bare VEVENT) is a `*SyntaxError` wrapping
  `io.ErrUnexpectedEOF`, as `Parse` reports empty input, so a broken feed
  doesn't read as an empty one. (A cut-off final line may be reported first as the malformed
  line it is: in strict mode any line without its CRLF, in both modes a
  bare `BEGIN:`. It is still an error.)
- Lenient recoveries at this layer: lines outside a VCALENDAR are
  ignored; an END naming an enclosing component closes the ones in
  between; an END for nothing open is ignored; parameters or space
  around a BEGIN or END name, a missing or repeated PRODID or VERSION and
  a repeated CALSCALE or METHOD are reported. Strict mode also rejects a
  VCALENDAR with no component (§3.6).
- Never recovered: truncation, a BEGIN that isn't a name, nesting over
  `MaxDepth`, a line over `MaxLineLength`.

## Layer 3: values, civil time and time zones

### Civil time (`civil`)

`Date`, `Time` and `DateTime`, in the shape of `cloud.google.com/go/civil`,
with the calendar arithmetic `recur` and `tz` share (epoch days, weekday,
year day, `AddDate`). Recurrence runs on wall-clock time and DATE and
floating values never have a zone; a `time.Time` would force one on them.

`civil.DateTime.In(loc)` turns a wall-clock reading into an instant the
way RFC 5545 §3.3.5 requires, which `time.Date` doesn't promise:

- A time that occurs twice (fall back) is the **first** occurrence.
- A time in a gap (spring forward) uses the offset **before** the gap:
  `TZID=America/New_York:20070311T023000` is 03:30 EDT. (`time.Date`
  moves it back to 01:30 EST.)

It tries the offsets in force a day either side and picks by those two
rules. Tested against both RFC examples and Lord Howe's 30-minute shift.

### DATE and DATE-TIME

```go
type DateTime struct {
	civil.DateTime                // wall clock as written
	IsDate bool                   // VALUE=DATE
	UTC    bool                   // the Z form
	TZID   string                 // as written
	Loc    *time.Location         // TZID resolved
}
v.Time() time.Time                // floating read as UTC wall clock
v.In(loc) time.Time               // floating read on loc's wall clock
v.Add(d Duration) DateTime        // §3.3.6, below
```

It keeps all three forms of §3.3.5 and the TZID text, so it re-encodes
exactly. A value that a computation produced (`Add`, `NewDateTime`, or an
occurrence from the `Expander`) also remembers its exact instant, because
the wall clock and TZID alone can't say which of two repeated 01:30s it
is: one hour after the first 01:30 of a fall-back night is the second.
Since a TZID's wall clock names the first, a setter writes such a value in
UTC. Lenient reading accepts an 8-digit value without `VALUE=DATE`, a
time without seconds (`19970701T1300Z`), and a DATE with a `Z`
(`20131210Z`), all seen in real feeds.

### Durations

`Duration{Negative, Weeks, Days, Hours, Minutes, Seconds}`. Weeks and
days are nominal (on the wall clock: P1D keeps 12:00 across a DST change);
hours, minutes and seconds are exact (PT24H doesn't). `P2W` re-encodes as
`P2W`.

### Time zones (`tz`)

A TZID resolves through the calendar (`Calendar` is a `tz.Resolver`):

1. The calendar's own VTIMEZONE with that TZID, compiled.
2. `tz.Load`, cached, failures included:
   - an IANA name, including miscased ones;
   - a Windows ID (`Eastern Standard Time`) through CLDR's
     `windowsZones` (v49, 142 IDs, deprecated IANA aliases mapped to
     current names), generated by `tz/internal/gen`;
   - a globally unique TZID (`/mozilla.org/20050126_1/America/New_York`,
     `/Europe/Stockholm`): leading segments are dropped until an IANA name
     loads;
   - a fixed offset (`UTC-04:00`, `GMT+0530`), which names its zone exactly.
3. Otherwise an error wrapping `tz.ErrUnknownZone`. `Load` never guesses,
   and refuses `Local`, `localtime` (which Debian links to the host's
   zone) and anything path-like. A value whose TZID doesn't resolve is
   never placed at a guessed instant: the accessor and the `Expander`
   report the error, and the `Expander` drops what it can't place.

**Compiling a VTIMEZONE** (`tz.Compile`) expands each STANDARD and
DAYLIGHT observance with `recur` (DTSTART, RRULEs, RDATEs, on the
TZOFFSETFROM wall clock, a UTC UNTIL compared as an instant), sorts the
onsets, encodes them as TZif v2 (RFC 9636), and loads that with
`time.LoadLocationFromTZData`. So callers get ordinary `time.Time` values
with the right offsets and abbreviations. Before the first onset the zone
keeps the first observance's TZOFFSETFROM. Transitions are explicit to
year 2200; when the open-ended rules are annual nth-weekday rules (almost
all are, including the RFC 2445 `BYMONTHDAY=8,…,14` style), they are also
written as the TZif footer, so the zone stays right after that. Results
are cached by a hash of the definition's content, so one VTIMEZONE
repeated across thousands of feeds compiles once. This also covers
Outlook's `Customized Time Zone`, which exists nowhere else. A compiled
US Eastern definition matches IANA `America/New_York` hour by hour over
2007–2030 and 2195–2210. The footer also takes rules for a weekday on or
after a given day (`BYDAY=FR;BYMONTHDAY=23,…,29`, or `BYYEARDAY` for a
week that runs into the next month), written the way the IANA database
writes them, as a weekday plus up to 167 hours (`M3.4.4/26`). Abbreviations
go into the footer as they are, so a zone keeps its TZNAMEs past the
explicit transitions; RFC 9636 would allow only 3 or more of `A-Za-z0-9+-`
there, but the time package, the only reader of this TZif data, takes
anything up to the closing `>`.

The three caches (`Load`'s, `Compile`'s and `DefinitionOf`'s) are bounded
LRUs over immutable values, and are the only process-wide state in the
module.

**Writing a VTIMEZONE.** RFC 5545 requires a VTIMEZONE for every TZID a
calendar uses (§3.6.5), valid "for all recurrence instances" of the
components that use it. A producer that writes `DTSTART;TZID=America/Chicago`
has a `*time.Location` and needs the VTIMEZONE text:

```go
def, err := tz.DefinitionOf(loc, from)  // the zone from `from` on, as STANDARD/DAYLIGHT observances
z, err := ical.NewTimezone(def)         // a VTIMEZONE component: Timezone.Definition's inverse
err = cal.AddTimezones(r)               // both, for each TZID the calendar uses and doesn't define
```

- `DefinitionOf` walks the zone's transitions from `from` to the end of
  `tz.Horizon` (2200). Transitions that recur on one yearly rule become one
  observance with an RRULE: the nth or last weekday of a month, a weekday on
  or after a day (Israel's Friday on or after March 23; Egypt's Friday on or
  after October 26, which can be November 1, needs `BYYEARDAY=-67,…,-61`),
  a fixed date, or a day of the year. A rule the zone stopped following ends
  with a UTC UNTIL equal to its last onset (§3.6.5); one it has followed for
  at least 28 years (long enough for its dates to fall on every day of the
  week, which tells apart rules that fit a few years alike) and still
  follows at 2200 is open-ended, so readers extend it as far as recurrences
  go. Other
  transitions (Ramadan breaks, abolished DST) are listed by DTSTART and
  RDATE. The rules in force at `from` are traced back to where they began,
  so from any time since 2007 `America/New_York` is the two rules calendar
  programs write, and the output doesn't change as a feed's earliest event
  moves on. A zone with no transition before `from` gets one observance that
  changes nothing, at 1970-01-01 (or `from`'s day, if earlier), so every
  time from `from` on follows an onset.
- DAYLIGHT is the higher offset of a change. The IANA database's negative
  DST (Europe/Dublin's winter, in the main rather than rearguard data)
  would otherwise make GMT the DAYLIGHT observance, and the same zone would
  be written differently on hosts with different builds of the database.
  A change is only read the other way round when the zone changes back
  within two years, so leaving DST for good, for a new and higher standard
  time, stays STANDARD.
- The time package's `ZoneBounds` can't be followed blindly. Past a TZif
  file's explicit transitions (2037 in "fat" files; sooner in "slim" ones),
  where Go computes transitions from the POSIX footer, it reports the start
  of every UTC year as a bound, and on December 31 of a leap year an end
  that doesn't advance (it takes a year to be 365 days), so a naive walk
  never ends. The walker steps over bounds where nothing changes and
  replaces one that doesn't advance with a searched day's step.
- `NewTimezone` writes DTSTART and RDATE as local times, then RRULE,
  TZOFFSETFROM, TZOFFSETTO and TZNAME, the order of the RFC's examples. It
  writes valid RFC 5545 for any definition it accepts: a rule that ends is
  always written with a UTC UNTIL at its last onset, even when the
  definition ends it with a COUNT or a floating or DATE UNTIL. It refuses
  definitions no VTIMEZONE can hold (no TZID or observance, an offset of a
  day or more, a rule that breaks §3.3.10).
- `AddTimezones(r)` finds every TZID parameter in the calendar (outside its
  VTIMEZONEs), resolves it with `r` (`tz.Load` when `r` is nil, so
  `Eastern Standard Time` gets New York's rules under its own name), and
  builds each zone from the earliest instant written with it. A producer
  whose times were computed in locations `tz.Load` might not give back for
  their names (from `time/tzdata`, or `time.LoadLocationFromTZData`)
  passes a resolver returning them, so the VTIMEZONE agrees with the
  times. The new VTIMEZONEs go before the other components, in TZID order.
  A TZID it can't resolve is reported and doesn't stop the others.
- Cost: about 0.3 ms per zone on an M-series Mac, most of it walking to
  2200 (`BenchmarkDefinitionOf`), the first time; results are cached by
  location and the onset of `from`'s period, and handed out as copies.

## Layer 4: recurrence (`recur`)

### The rule value

```go
type Rule struct {
	Freq       Frequency      // Secondly … Yearly
	Interval   int            // 0 is read as 1
	Count      int            // 0: not set
	Until      Until          // keeps DATE / floating / UTC form
	WeekStart  Weekday        // WKST; the zero value is Monday, the default
	BySecond, ByMinute, ByHour, ByMonthDay, ByYearDay, ByWeekNo, ByMonth, BySetPos []int
	ByDay      []WeekdayNum   // {N: -1, Day: recur.Friday} is -1FR
}
rule, err := recur.ParseRule("FREQ=MONTHLY;BYDAY=-1FR;COUNT=12")
rule.String()     // canonical, round-trips
rule.Validate()   // §3.3.10's MUST / MUST NOT between parts
```

`recur.Weekday` counts from Monday so its zero value is the RFC's default
WKST; `Time()` and `WeekdayOf` convert to and from `time.Weekday`.
`ParseRule` reads names and values case-insensitively and ignores an empty
part, but refuses unknown or repeated parts, out-of-range values, and a
COUNT or INTERVAL below 1 (which rrule-go reads as "endless" and "1").

### Expansion API

```go
for t := range rule.All(dtstart) { … }                 // the rule alone (dateutil semantics)
for t := range rule.Between(dtstart, from, to) { … }   // [from, to), seeking

set := &recur.Set{Start: dtstart, Location: loc, Rules: …, ExRules: …,
	RDates: …, ExDates: …,   // wall-clock times on Start's clock
	RTimes: …, ExTimes: …,   // instants, for values written in UTC or another zone
	AllDay: false, Cache: cache}
for t := range set.Between(from, to) { … }             // civil.DateTime, RFC 5545 semantics
for t := range set.Times(fromTime, toTime) { … }        // time.Time, resolved per §3.3.5
it := set.TimeIterator(fromTime, toTime)                // pull style, for merging
wall, at, ok := it.Next(); it.Close()
```

The rules run on the wall clock, but a `Set` resolves every instance to
an instant and de-duplicates, applies EXDATEs and orders by instant. That
matters at DST changes: an instance in a spring-forward gap resolves to
after the wall-clock times that follow it, an EXDATE written in UTC must
match an instance the gap moved, and an RDATE in UTC can name the second
of two identical 01:30s when clocks go back, which no wall-clock time
can. A small reorder buffer, bounded by the gap a resolution skipped,
keeps the output sorted by instant without materializing it.

### The engine

Period-based, as the RFC defines it: each FREQ × INTERVAL period is
expanded by the BYxxx parts the §3.3.10 table marks *Expand*, filtered by
those marked *Limit*, de-duplicated, then BYSETPOS picks within the
period; then instances before DTSTART are dropped and COUNT and UNTIL
applied. Day-level parts compile to bit masks and per-year tables
(month, month day, year day, weekday, ISO week with any WKST, nth
weekday), so a period costs about its number of candidate days. A
period's candidates are the product of its matching days and its times of
day, walked without being materialized, so a dense period (every second
of a year) costs no memory; BYSETPOS indexes into the product directly.

Every BYxxx part means something for every frequency, so rules that break
a MUST NOT still expand predictably: an N/A day-level part limits the
period's days and replaces the DTSTART default it would otherwise get
(`FREQ=WEEKLY;BYMONTHDAY=23` is every 23rd, not Sundays that are the
23rd), and a numbered BYDAY outside MONTHLY/YEARLY loses its number (as in
dateutil).

### Semantics settled by the vectors

The RFC is explicit on most of these; where it isn't, the choice follows
libical, ical.js and ical.net, which agree with each other:

- **DTSTART is the first instance** of a `Set` and **counts toward
  COUNT** even when the rule doesn't generate it (§3.8.5.3: "always counts
  as the first occurrence"; sabre/vobject agrees). `Rule.All` is the rule
  alone and doesn't add it, like dateutil.
- **Missing parts come from DTSTART** (§3.3.10). With no day-level part,
  YEARLY takes DTSTART's month and day, MONTHLY its day, WEEKLY its
  weekday. **YEARLY with BYMONTHDAY but no BYMONTH takes DTSTART's
  month** (unless BYWEEKNO or BYYEARDAY already places the day in the
  year), and **YEARLY with BYWEEKNO but no BYDAY takes DTSTART's
  weekday** (unless BYMONTHDAY or BYYEARDAY already picks the day)
  (libical, ical.js, ical.net; dateutil expands to every month and every
  day of the week).
- **With BYWEEKNO, YEARLY periods are week-numbering years**, so INTERVAL
  counts the years the weeks belong to (libical, ical.net).
- **A numbered BYDAY in YEARLY counts within each month when BYMONTH is
  present, within the year otherwise** (errata 1913, 3779).
- **Invalid dates are skipped and not counted** (Feb 30, the 31st of a
  30-day month). **Times in a DST gap are kept**, resolved forward past
  the gap (erratum 4271). Sub-daily rules step on the wall clock.
- **A DATE DTSTART ignores BYHOUR, BYMINUTE and BYSECOND** (§3.3.10
  "MUST be ignored").
- **UNTIL is inclusive and compared in its own form**: DATE against the
  date (a DATE UNTIL on a timed series includes that whole day), floating
  against the wall clock, UTC against the instant. A gap time past a UTC
  UNTIL doesn't end the rule, since the wall-clock times after it can
  still resolve to before UNTIL.
- **Duplicates count once**; a `0MO` BYDAY entry is dropped (ical.net).
- **An EXRULE is the rule alone**: DTSTART neither counts toward its COUNT
  nor is excluded unless the EXRULE generates it (dateutil). An EXRULE
  excludes RDATEs too.
- **RDATEs before DTSTART are kept**, as dateutil keeps them: an explicit
  date isn't dropped silently.
- **Iteration ends at year 9999**, the last year iCalendar can write.

The 28 vectors where another library differs on purpose are listed, with
reasons, in `knownDifferences` in `recur/vectors_test.go`; each is checked
to still differ, so the list can't go stale. Most are dateutil's
month/week defaults; the rest are bugs in the other library (for example
sabre/vobject puts week −20 of 2012 at ISO week 32).

### Seeking

Every period expands independently (BYSETPOS is per period) and its
instances lie inside its own span. So for a rule without COUNT, the first
period that can hold an instance at or after `from` is computed
arithmetically, and expansion starts there. One month of a daily series
begun in 1990 costs ~1 µs. A COUNT rule must know how many instances came
before; without a cache it counts from DTSTART (~260 µs for the same
query), with one it doesn't (below). A differential test checks `Between`
against a full walk on 7,000+ windows from every vector rule, with and
without a cache, and `FuzzRule` does the same on random rules (87 million
executions without a failure).

### Termination

1. **Static checks** catch rules that can never match: month days no
   listed month has, numbered weekdays that can't exist (6MO in a
   month), time-of-day phases an INTERVAL never reaches (`FREQ=HOURLY;
   INTERVAL=24;BYHOUR=11` from 09:00, by a gcd against the day), and
   DAILY intervals of whole weeks that never reach BYDAY.
2. **A proven bound.** The Gregorian calendar repeats every 400 years
   (146,097 days, exactly 20,871 weeks). A period's candidates depend only
   on where it falls in that cycle, and periods step by INTERVAL, so
   positions repeat after `cycle / gcd(INTERVAL, cycle)` periods. That many
   consecutive empty periods prove there will never be another candidate.
3. **Coarse skipping**: a failed month or day jumps a daily or sub-daily
   rule to the next aligned period in the next candidate month, day, hour
   or minute.
4. **Year 9999** is the hard end.

### Caching

A `recur.Cache` is explicit, bounded (LRU by rule) and safe for
concurrent use. Nothing is cached unless one is passed in, via
`Set.Cache` or `ical.Expander.Cache`.

1. **COUNT checkpoints**: as a COUNT rule is walked, sparse (period,
   instances before it) checkpoints are recorded, the stride doubling as
   the table grows so memory stays bounded (the scheme of the consuming
   app's `restart.go`, moved inside the engine and keyed on periods). A
   later query resumes from the nearest checkpoint.
2. **Rule ends**: once a walk reaches a rule's end (its COUNT-th instance,
   UNTIL, the cycle bound, year 9999) the last instance is stored. From
   then on a COUNT rule is treated as the same rule ending there: it seeks
   arithmetically with no counting at all, and a query past the end
   returns at once. For the COUNT query above: ~1.9 µs cached against
   ~260 µs uncached.
3. **Expansion blocks** *(planned)*: materialized instances per calendar
   year, so sliding windows reuse each other's work. Seeking already makes
   uncached windows cheap, so this matters mainly for very dense rules.

Keys are the canonical RRULE text, DTSTART, and (only for a UTC UNTIL) the
`*time.Location` identity, so an IANA zone and a calendar's own VTIMEZONE
of the same name never share an entry.

## Layer 5: occurrences

```go
x := ical.Expander{Cache: cache, Floating: loc, Components: []string{"VEVENT"}}
for occ, err := range x.Occurrences(cal, from, to) {
	occ.Start, occ.End       // DateTime
	occ.RecurrenceID         // which instance this is
	occ.Component            // the master, or the override that replaced it
}
for occ, err := range cal.Occurrences(from, to) { … } // the zero Expander
```

- **Overlap**: an occurrence is in `[from, to)` if it starts before `to`
  and ends after `from`; one with no duration if it starts in the window
  (CalDAV time-range, RFC 4791 §9.9).
- **Overrides** are grouped by UID and indexed by RECURRENCE-ID instant.
  An override replaces its instance and is yielded where its own DTSTART
  puts it, so an instance moved into the window from outside appears and
  one moved out doesn't. The highest SEQUENCE wins among overrides of one
  instance, and among masters when a feed keeps several revisions. An
  override whose instance a newer master EXDATEs is dropped
  (python-recurring-ical-events reads feeds this way; its
  calendar-level expectations all pass).
- **RANGE=THISANDFUTURE** *(planned)*: such overrides currently apply to
  their own instance only.
- **End times**: DTEND gives every instance the same exact duration,
  DURATION the same nominal one (§3.8.5.3); an RDATE PERIOD has its own,
  unless it doesn't end after it starts (§3.3.9), which is reported and
  falls back to the series' length. With neither, a DATE start lasts one
  day and a DATE-TIME start zero (§3.6.1).
- **EXDATE as a DATE on a timed series** (forbidden by the RFC, written by
  some producers) excludes every instance on that day.
- **RDATE and EXDATE in another zone** are matched as instants
  (`RTimes`, `ExTimes`), and for a floating series read in
  `Expander.Floating`; occurrences are reported on the series' clock.
- **Order**: a heap merges per-component pull iterators
  (`recur.Iterator`, no goroutines, already in instant order), so the
  stream is sorted and can stop anywhere.
- **Errors** for one component (an unparseable RRULE, an unknown TZID)
  are yielded with that component and don't stop the others. Empty RRULEs
  are ignored.

## Errors, strictness and limits

- `*ical.SyntaxError{Line, Msg, Err}`; `errors.Is(err, io.ErrUnexpectedEOF)`
  for truncation; `errors.Is(err, contentline.ErrLineTooLong)`.
- `*ical.ValueError{Property, Value, Err}` from value accessors;
  `tz.ErrUnknownZone`, `tz.ErrInvalidDefinition`, `ical.ErrNoProperty`
  wrapped where they apply.
- Lenient by default, strict on request. Lenient never means silent: every
  recovery goes to `Warn`.

## Extensibility: other RFCs

| Extension kind | Example | Seam |
|---|---|---|
| New properties, parameters | RFC 7986 COLOR, IMAGE, CONFERENCE; RFC 9073 STRUCTURED-DATA | Raw values already round-trip; a package adds typed accessors and *(planned)* schema entries |
| New components | RFC 7953 VAVAILABILITY; RFC 9073 VLOCATION, VRESOURCE; RFC 9074 VALARM extensions | Generic `Component`; a package adds a typed view |
| Changed encodings | RFC 6868 | Done, in `contentline` |
| Recurrence extensions | RFC 7529 RSCALE, SKIP, leap months | `Rule` gains `RScale`/`Skip`; the per-year masks come from a calendar-system interface with Gregorian built in. 34 libical RSCALE vectors are waiting |
| Other representations | RFC 7265 jCal, RFC 6321 xCal | New codecs over the same tree (`ical/jcal`) |
| Protocol semantics | RFC 5546 iTIP | Validators and helpers over the model |

A **schema** *(planned)* will be a value, not a global registry:
`ical.RFC5545.With(rfc7986.Schema)`. It will drive defaults for typed
decoding (the VALUE a property has when none is given, whether it is a
list) and validation (required, at most once, allowed where). Decoding
doesn't need it to round-trip, which is why it can come later.

## Testing

| Suite | What it checks | Status |
|---|---|---|
| RFC recurrence examples | All 43 §3.8.5.3 cases, independently brute-force checked when transcribed | 43 / 43 |
| Third-party recurrence vectors | 1,217 from dateutil, libical, ical.js, ical.net, sabre/vobject, rrule.js, rust-rrule | 992 match; 28 documented differences; 197 skipped: 73 use extensions (BYEASTER, RSCALE, SKIP), 124 combine parts RFC 5545 forbids |
| Seek ≡ walk | `Between` vs. a full walk on 7,000+ windows, with and without a cache | pass |
| Corpus | 2,139 `.ics` files from 9 projects and the RFC: no panic or hang; decode → encode → decode identical; valid folding; every event expands without panicking | 2,045 decode; the 94 rejected have no VCALENDAR (fragments, vCards, fuzz junk) or are truncated. 618 per-event expansion errors, all bad input: 512 events lack the DTSTART §3.6.1 requires, the rest are malformed values, unknown TZIDs with no VTIMEZONE, or RSCALE |
| RFC examples | 222 property lines and 26 components from `examples.json` | pass |
| RFC conformance | 143 tests in `conformance_*_test.go`, each case citing the RFC 5545 / 6868 sentence it checks, with expectations worked out from the RFC text rather than the code: content lines and framing, every value type, components, VTIMEZONE (the RFC's own zones, onset by onset), VTIMEZONEs this library writes, TZID resolution under three host zones, occurrences, RRULE grammar and every §3.3.10 MUST | pass; 131 skipped as `KNOWN GAP`, almost all §3.6/§3.8 property rules that only the planned schema validation will report |
| Independent RRULE reference | A naive §3.3.10 implementation sharing no code with `recur`, differentially tested against `Set.Between`, `Rule.Between` (seeking, with and without a cache) and zoned `Set.Times` near real DST changes | 3,000 rules + 1,000 zoned sets per run (`-short`: a tenth); 300,000 + 100,000 with `-recur.conformance=300000`: no mismatches |
| recurring-ical-events | 11 calendar-level occurrence expectations (overrides, SEQUENCE, EXDATE) | 11 / 11 |
| VTIMEZONE | compiled zones vs. IANA, hour by hour | pass |
| VTIMEZONE writing | `DefinitionOf` → `Compile` against the source zone at every change of either (and densely around each, and weekly) from 1800, 1900, 1970, a transition instant, 2026 and 2150 to 2400, for 19 zones chosen for their history; every zone in the host's zoneinfo from 1960 and 2026; the written text read back strictly and checked against §3.6.5's MUSTs, with offsets worked out by §3.6.5's own "last onset before the time" rule, without `Compile`; a weekly event in Chicago keeps its local time to 2060 | pass; mutations of the onset clock or UNTIL are caught |
| Fuzzing | `FuzzReader`, `FuzzDecode`, `FuzzRule` | clean (4 min each: 15M, 13M, 19M executions) |
| Independent review | A second pass over the engine and expander, with a 40,000-rule randomized differential run, found 9 bugs (DST-gap ordering, instants vs. wall clock for RDATE/EXDATE, an all-day window edge, EXRULE counting, a slow-to-prove empty BYSETPOS, …) | all fixed, each with a regression test |
| Hostile input | 1 MiB parameter, 5 MiB of parameters, 1M comma values | linear |
| Comparison | `testdata/compare` (a nested module): the same RFC-derived probes, hostile inputs and corpus against golang-ical, emersion/go-ical and rrule-go, each probe in its own subprocess with a timeout | this library: no wrong value, panic or hang; golang-ical fails 12 probes (10 with its options), emersion/go-ical 19, rrule-go 7 |

The third-party fixtures are a nested Go module (`testdata/thirdparty`),
so they're excluded from the library's module zip and users never download
them or their mixed (partly copyleft: libical MPL/LGPL, ical.js MPL,
recurring-ical-events LGPL) licences. Tests skip them when absent. Each
source folder has its licence and a PROVENANCE file with the exact commit;
the scripts that built them are in `testdata/thirdparty/tools`.

## Roadmap

1. **Foundation** (done): the layers above, with the test suites.
2. **Occurrences, part 2**: RANGE=THISANDFUTURE, including stacked range
   overrides (the survey found no library that does this completely);
   flags on `Occurrence` for "overridden" and "from RDATE"; expansion blocks
   in the cache; VTODO (DUE) and VJOURNAL defaults in the Expander.
3. **Schema and validation**: per-property value types and cardinality
   for RFC 5545, `Validate` on the model, strict-mode checks such as
   UNTIL's form against DTSTART's (erratum 4414).
4. **Time zones, part 2**: writing a VTIMEZONE from a `*time.Location`
   (done: `tz.DefinitionOf`, `ical.NewTimezone`, `Calendar.AddTimezones`);
   a Windows *display name* table (`(UTC-05:00) Eastern Time (US &
   Canada)`) from sabre/vobject's data.
5. **Extensions**: RFC 7986, 9073/9074, 7529 (RSCALE, against libical's
   vectors), then jCal/xCal and iTIP helpers.

## Open questions

From the [API survey](api-survey.md)'s recommendations, where this design
differs or hasn't decided yet:

- **Unknown RRULE parts.** `ParseRule` rejects them (as dateutil and
  vobject do); python-icalendar and ical4j keep them. The raw property
  already round-trips, so this only affects expansion. Accepting
  `RSCALE=GREGORIAN` (equivalent to RFC 5545) would be an easy first step
  toward RFC 7529.
- **TZID precedence.** The calendar's VTIMEZONE wins, as RFC 5545 and
  libical/ical.js have it. python-icalendar and ical.net prefer IANA for
  names IANA knows, which protects against truncated VTIMEZONEs. Make it an
  option on `Expander` and `Calendar`?
- **Unresolved TZIDs at decode time.** Resolution is lazy, so an unknown
  TZID surfaces as an error from the accessor or the `Expander`, not
  through `Decoder.Warn`. A decode-time pass would warn earlier, at the
  cost of resolving zones nobody reads.
- **Ending vs. failing.** The survey recommends a budget on empty periods
  that returns an error. Here exhaustion is *proven* (the cycle bound), so
  a rule that ends simply ends; but a statically empty rule could also be
  reported by a validation call.

- **Floating default.** `DateTime.Time()` reads a floating value as UTC
  wall clock, and `Expander.Floating` defaults to UTC. Deterministic and
  never host-dependent, but a caller who forgets to pass a zone gets UTC
  without being told. The alternative is an error from `Time()` for
  floating values.
- **A DATE UNTIL on a timed series** includes the whole day here; dateutil
  reads it as midnight. Both are outside the RFC (erratum 4414 wants the
  forms to match).
- **Several masters with one UID** pick the highest SEQUENCE; RFC 5545
  calls the calendar invalid. Is warning about it worth an API?
