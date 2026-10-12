# ical.js

| | |
|---|---|
| Upstream | https://github.com/kewisch/ical.js |
| Commit | `33ba266282fac7c721d792e04124513df73fe140` (branch `main`, committed 2026-09-17) |
| Cloned | 2026-10-11 (`git clone --depth 1`) |
| License | `MPL-2.0` (see `LICENSE`) — file-level weak copyleft |

## Copied (`ics/`, upstream paths preserved)

* `samples/*.ics` (18) and `samples/timezones/**` (7 VTIMEZONEs incl. the fictional
  `Makebelieve/*` zones) — real-world-ish samples: Google birthday calendar, blank lines in the
  middle/end, `DURATION` instead of `DTEND`, forced value types, multiple RRULEs,
  RDATE/EXDATE mixes, `recur_instances.ics` (RECURRENCE-ID overrides), `utc_negative_zero.ics`
  (`-0000` offset).
* `test/parser/*.ics` (21) **and** the matching `test/parser/*.json` (21): ical.js's expected
  jCal (RFC 7265) parse result for each input — usable as golden parse output.

## Converted (`recur.json`, 115 cases)

All active `testRRULE(rule, {dtStart, dates, byCount, until, noInstance})` calls in
`test/recur_iterator_test.js`, parsed statically (comments stripped, JS literals parsed, never
executed). `dtStart` defaults to `dates[0]`; finite rules compare the complete list, infinite
rules compare `dates.length` values (`limit`). `noInstance: true` → `expected: []`.
ICAL.RecurIterator does not emit a non-matching DTSTART (`include_dtstart: false`).

Script: `scripts/conv_icaljs.py` (scratch, not committed).

## Skipped

* 7 `testRRULE` calls that are commented out upstream (BYWEEKNO TODOs at lines 354, 1003, 1013,
  1022, 1040, 1059 and the Feb-29 yearly case at line 1600).
* `test/recur_expansion_test.js` (component-level expansion over `samples/recur_instances.ics`
  with expectations spread over several API calls) and `test/recur_test.js` (API/parsing tests)
  — not mechanically extractable as vectors.
* `.vcf` vCard fixtures (out of scope).
