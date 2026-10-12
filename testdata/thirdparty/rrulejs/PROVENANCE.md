# rrule.js

| | |
|---|---|
| Upstream | https://github.com/jkbrzt/rrule |
| Commit | `9f2061febeeb363d03352efe33d30c33073a0242` (branch `master`, committed 2023-11-10) |
| Cloned | 2026-10-11 (`git clone --depth 1`) |
| License | `BSD-3-Clause` (see `LICENSE`, upstream `LICENCE`; the test suite derives from python-dateutil, whose BSD-3-Clause notice is included in the same file) |

## Copied

No fixture files (vectors only).

## Converted (`recur.json`, 232 cases)

`test/rrule.test.ts`: every `testRecurring(msg, new RRule({...}) | {rrule, method, args},
expected)` call whose rule is an object literal, parsed statically (TypeScript literals,
nothing executed). Options are mapped to RRULE text as for dateutil. rrule.js `datetime()` /
`parse()` build UTC JS Dates, so all values carry `Z`. DTSTART is not emitted unless it
matches (`include_dtstart: false`). `between` / `after` methods are mapped to `between`
(+ `limit: 1` for `after`). Because the upstream `assertDatesEqual()` checks the length only
when more than one date is expected, single-date `all` cases get `limit: 1`. When `freq` is
omitted the rrule.js default `YEARLY` is written; rrule.js also forces `YEARLY` whenever
`byeaster` is set (noted per case).

Script: `scripts/conv_rrulejs.py` (scratch, not committed).

## Skipped (19 calls)

* 9 `testRecurring.skip(...)` (disabled upstream).
* 3 using `parse()` with strings that are not plain `YYYYMMDDTHHMMSS`.
* 2 with `new Date(ms)` sub-second values.
* 2 `before()` cases (last instance before a date — not expressible in the vector format).
* 2 with an empty expected list (upstream asserts nothing in that case).
* 1 using the `tzid` option.
* `test/rruleset.test.ts`, `test/rrulestr.test.ts`, `test/datewithzone.test.ts` (not converted;
  mostly duplicates of dateutil's set/str tests or JS time-zone specifics).
