# Third-party iCalendar test corpus

Test inputs and expected results collected from other iCalendar / RRULE implementations, used
to exercise this library's parser, serializer and recurrence engine against real-world and
adversarial data. Nothing here is code from those projects: the directory holds data files
copied verbatim, plus recurrence vectors converted into one JSON format (`recur.json`).

Collected on 2026-10-11 from shallow clones. Each library folder has the upstream license
text(s) (`LICENSE*`) and a `PROVENANCE.md` with the exact commit, original paths, how each file
was converted, and what was skipped and why.

## Why a nested module

`go.mod` here declares `github.com/aanelson/iCalendar/testdata/thirdparty`. A directory with
its own `go.mod` is excluded from the parent module's zip, so `go get` users of the library never
download this ~11 MB corpus or its mixed (partly copyleft) licenses. The module has no Go
packages. Tests in the main module read the files by relative path (for example
`../testdata/thirdparty/...` from a package directory). They must call `t.Skip` when the
directory is missing, which is the case when the module is tested from the module cache.

## Sources

| Folder | Upstream | Commit | License | Fixture files (`ics/`) | Vectors |
|---|---|---|---|---:|---:|
| `libical` | [libical/libical](https://github.com/libical/libical) | `7dba02b3ab4a9536eafe3b9bbedb57e28d83775e` | **MPL-2.0 OR LGPL-2.1-only** | 1260 (1227 `.ics`, incl. 2×597 VTIMEZONEs; 32 fuzz inputs; `recur.txt`) | 185 |
| `icaljs` | [kewisch/ical.js](https://github.com/kewisch/ical.js) | `33ba266282fac7c721d792e04124513df73fe140` | **MPL-2.0** | 67 (46 `.ics` + 21 jCal `.json`) | 115 |
| `python-icalendar` | [collective/icalendar](https://github.com/collective/icalendar) | `ec09f7e9f444e3d2a48b5b8cedb086729659e358` | BSD-2-Clause | 201 (196 `.ics` + 5 `.jcal`) | – |
| `recurring-ical-events` | [niccokunzmann/python-recurring-ical-events](https://github.com/niccokunzmann/python-recurring-ical-events) | `b054fcbc9a1deba0a108663c51975216b0f293c2` | **LGPL-3.0-or-later** | 97 | 11 (`expected.json`) |
| `dateutil` | [dateutil/dateutil](https://github.com/dateutil/dateutil) | `2642afacc33fb839c404b75b230fff58b79793f2` | Apache-2.0 AND BSD-3-Clause | – | 297 |
| `ical4j` | [ical4j/ical4j](https://github.com/ical4j/ical4j) | `bd09358860598b07235a91061117d1cf46ba7638` | BSD-3-Clause | 119 (`samples/valid` 81, `samples/invalid` 22, 12 Outlook/Yahoo/Apple `.txt`) | – |
| `sabre-vobject` | [sabre-io/vobject](https://github.com/sabre-io/vobject) | `5e2f338811916c03927398bb24e527989de63194` | BSD-3-Clause | 265 (1 `.ics` + 264 extracted from PHP tests) | 93 |
| `icalnet` | [ical-org/ical.net](https://github.com/ical-org/ical.net) | `7a5264f2e4c0a983fce56b42044d656abd1f8d8e` | MIT | 150 | 92 |
| `golang-ical` | [arran4/golang-ical](https://github.com/arran4/golang-ical) | `33ff6fba2f0c9f5d996e5ab202ea10c67b8b8207` | Apache-2.0 | 25 (incl. 7 golden re-serialisations) | – |
| `go-ical` | [emersion/go-ical](https://github.com/emersion/go-ical) | `439c63cef608771fe1e93407b1b861e8c9a441ae` | MIT | 1 (extracted from a Go test) | – |
| `rrulejs` | [jkbrzt/rrule](https://github.com/jkbrzt/rrule) | `9f2061febeeb363d03352efe33d30c33073a0242` | BSD-3-Clause | – | 232 |
| `rust-rrule` | [fmeringdal/rust-rrule](https://github.com/fmeringdal/rust-rrule) | `1c3420ede20e6190040804121e1b5e1eddac6493` | MIT OR Apache-2.0 | – | 203 |

Totals: 2185 fixture files, 1217 recurrence vectors plus 11 calendar-level expectations,
about 11 MB.

### Copyleft content

* `libical/` — MPL-2.0 or LGPL-2.1-only (your choice). Both are copyleft; MPL-2.0 is
  file-level.
* `icaljs/` — MPL-2.0 (file-level copyleft).
* `recurring-ical-events/` — **LGPL-3.0-or-later**.

These files are redistributed unmodified, except for `recur.json`, which is a mechanical
re-encoding of upstream test data. Each folder includes its license. Nothing here is compiled
into or linked with the library, and the nested module keeps it out of the library's module
zip. If a distribution must not contain copyleft files, delete those three folders; the tests
must skip when a folder is missing.

## Layout

```
<lib>/LICENSE*        upstream license text(s), verbatim
<lib>/PROVENANCE.md   upstream URL, commit, original paths, conversion notes, skips
<lib>/ics/...         fixtures; the path below ics/ is the upstream path
<lib>/recur.json      unified recurrence vectors (if the source has recurrence tests)
recurring-ical-events/expected.json   calendar-level occurrence expectations
```

The fixture trees contain `.ics` files plus a few other kinds: iCalendar in `.txt` files
(ical4j `transform/`, libical `recur.txt`, recurring-ical-events `date_exclude.txt`), libical
fuzz inputs with no extension, golang-ical `thunderbird.ics_disabled`, ical.js
`test/parser/*.json` (the expected jCal for the `.ics` file with the same name) and
python-icalendar `*.jcal`. Files may use LF or CRLF, may have a BOM, and may be deliberately
broken, truncated or non-UTF-8. Anything containing NUL bytes was left out. One ical4j
filename contains a space (`New Years Day.ics`).

Robustness tests should at least parse every file under `*/ics/` without panicking or hanging.
Stricter assertions need per-source knowledge. For example, `ical4j/ics/.../samples/invalid/`
are files that iCal4j's strict mode rejects, and golang-ical's
`testdata/serialization/expected/` holds golden re-serialisations.

## `recur.json` format

```json
{
  "source": "libical", "upstream": "...", "commit": "<sha>", "license": "...",
  "path": "<upstream file(s) the vectors came from>",
  "semantics": "how this source treats DTSTART, UNTIL, zones, limits, errors (read this)",
  "cases": [{
    "id": "libical-0001", "name": "...",
    "dtstart": "19970902T090000", "tzid": "America/New_York",
    "rrule": ["FREQ=DAILY;COUNT=10"], "exrule": [], "rdate": [], "exdate": [],
    "include_dtstart": false,
    "between": {"start": "...", "end": "...", "inclusive": true},
    "limit": 10,
    "expected": ["19970902T090000", "..."],
    "error": false, "extensions": [], "comment": ""
  }]
}
```

* Every date string is `YYYYMMDD`, `YYYYMMDDTHHMMSS` (floating, or wall-clock in `tzid`) or
  `YYYYMMDDTHHMMSSZ`. `tzid` is absent for floating and UTC values.
* `id`, `dtstart`, `rrule`, `include_dtstart` and `expected` are always present. Optional
  fields are omitted when empty. `rrule` can be `[]` for an RDATE-only set (dateutil).
* `include_dtstart: true` means the source emits DTSTART as the first instance even when it
  does not match the rule, and counts it toward COUNT (RFC 5545 semantics; sabre). `false`
  means only instances the rule generates are emitted (libical, ical.js, ical.net, dateutil,
  rrule.js, rust-rrule).
* `limit: N` means the source compared only the first N instances, usually of an unbounded rule.
* `between` is the query window. It is exclusive unless `inclusive` is true, and `end` may be
  absent (open-ended "from start": libical-style START-AT, sabre `fastForward`, dateutil
  `after`/`xafter`).
* `error: true` means the source expects the rule (or its evaluation) to be rejected, and
  `expected` is `[]`. Some errors are implementation-specific; the case `comment` says which.
* `extensions` flags anything that is not plain RFC 5545: `RSCALE` and `SKIP` (RFC 7529),
  `BYEASTER` (dateutil), and `NONSTANDARD:<detail>` for rule-part combinations that RFC 5545
  §3.3.10 forbids but the source defines anyway (for example `NONSTANDARD:BYWEEKNO-WITH-MONTHLY`).
  A strict RFC 5545 implementation may skip these or expect an error.
* Expected values are copied from the source unchanged. Where a source's expectation is
  questionable, the case `comment` says so. Read each file's `semantics` before using its
  vectors. The sources differ on DTSTART inclusion, on DST-gap handling (sabre moves
  non-existent local times forward), and on whether floating values are treated as UTC
  (ical.net).

All `recur.json` files were validated: they parse, every `id` is unique across all files, and
every date string uses one of the three forms above.

## `recurring-ical-events/expected.json`

A JSON list of `{"calendar", "start", "end", "expected_starts", "comment"}`, with an optional
`summary_contains` key. Each entry gives the DTSTART values of the occurrences that
`recurring_ical_events.of(calendar).between(start, end)` returns. These are the occurrences that
**overlap** the half-open window. `null` for `start` and `end` means all occurrences. See that
folder's `PROVENANCE.md` for details.
