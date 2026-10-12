# golang-ical (arran4)

| | |
|---|---|
| Upstream | https://github.com/arran4/golang-ical |
| Commit | `33ff6fba2f0c9f5d996e5ab202ea10c67b8b8207` (branch `master`, committed 2026-10-10) |
| Cloned | 2026-10-11 (`git clone --depth 1`) |
| License | `Apache-2.0` (see `LICENSE`) |

## Copied (`ics/`, upstream paths preserved)

* `testdata/rfc5545sec4/input1-6.ics` — the RFC 5545 section 4 examples.
* `testdata/serialization/input*.ics` (7) with `testdata/serialization/expected/input*.ics` (7)
  — golang-ical's expected re-serialisation of each input (golden files for round-trip tests).
* `testdata/issue52/issue52_working.ics`, `issue52_notworking.ics`, `testdata/issue97/google.ics`,
  `testdata/issue97/thunderbird.ics_disabled` (an iCalendar file disabled upstream; kept with its
  name), `testdata/timeparsing.ics`.

## Converted

Nothing (no recurrence vectors).

## Skipped

* `testdata/fuzz/FuzzParseCalendar/*` (3) — Go native-fuzzing corpus files in the
  `go test fuzz v1` encoding, not plain iCalendar.
