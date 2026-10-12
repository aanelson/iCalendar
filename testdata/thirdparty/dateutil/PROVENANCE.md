# python-dateutil

| | |
|---|---|
| Upstream | https://github.com/dateutil/dateutil |
| Commit | `2642afacc33fb839c404b75b230fff58b79793f2` (branch `master`, committed 2026-09-26) |
| Cloned | 2026-10-11 (`git clone --depth 1`) |
| License | `Apache-2.0 AND BSD-3-Clause` — see `LICENSE`: contributions after 2017-12-01 (and re-licensed code) are Apache-2.0; the BSD-3-Clause license applies to all code |

## Copied

No fixture files (only `tests/test_rrule.py` was used, as conversion input).

## Converted (`recur.json`, 297 cases from 295 test methods)

`tests/test_rrule.py` was parsed with Python's `ast` module — **never imported or executed**.
Extracted shapes:

* `self.assertEqual(list(rrule(FREQ, dtstart=..., **kw)), [datetime(...), ...])` and
  `assert list(x) == [...]`, including `rr = rrule(...)` / `exp = [...]` local assignments and
  `rule[:]` / `rule[:N]` slices (→ `limit`).
* `rrulestr("DTSTART...\nRRULE...\nEXRULE...\nRDATE...\nEXDATE...", dtstart=..., unfold=...,
  forceset=..., compatible=...)` — the RRULE/EXRULE text is kept verbatim (e.g. lower-case
  `testStrCase`, whitespace-separated `testStrSpaces`); `compatible=True` adds DTSTART
  (`include_dtstart: true`).
* `rruleset()` built with `.rrule()`, `.exrule()`, `.rdate()`, `.exdate()` when all member rules
  share one `dtstart` (8 cases).
* `.between(a, b[, inc])` → `between` (4 cases); `.after(dt[, inc])` → `between.start` + `limit: 1`
  and `.xafter(dt, count=N[, inc])` → `between.start` + `limit: N` (4 cases).
* `assertRaises(ValueError, rrule, ...)`, `with self.assertRaises(ValueError)` /
  `pytest.raises(ValueError)` → `error: true` (13 cases; 6 are dateutil-specific "BYxxx can never
  match with this INTERVAL" errors and are commented as such).

kwarg mapping: freq→FREQ, interval, count, until (naive → floating `YYYYMMDDTHHMMSS`, `date` →
`YYYYMMDD`, UTC-aware → `Z`), wkst, bysetpos, bymonth, bymonthday, byyearday, byweekno,
byweekday (`MO..SU`, ints 0–6, `TU(+1)` → `1TU`, `TH(-1)` → `-1TH`), byhour, byminute, bysecond;
`byeaster` → `BYEASTER=n` with `extensions: ["BYEASTER"]` (21 cases). `dtstart=date(...)` is
written as midnight because dateutil promotes it. `tzinfo=tz.gettz(NAME)` → `tzid` (6 cases).
dateutil does not add DTSTART unless it matches the rule → `include_dtstart: false`.
59 cases use rule-part combinations RFC 5545 forbids (BYWEEKNO/BYYEARDAY/BYMONTHDAY/BYDAY-ordinal
with the wrong FREQ, BYSETPOS alone); dateutil defines behaviour for them and they carry
`NONSTANDARD:*` markers in `extensions`.

Cross-check: the 229 rrule.js cases whose test names match dateutil's agree with these vectors
except where rrule.js deliberately uses different rules (e.g. `13TU` in
`testYearlyByNWeekDayLarge`).

Script: `scripts/conv_dateutil.py` (scratch, not committed).

## Not extracted: 267 of 562 test functions

| Count | Reason |
|---:|---|
| 227 | `testToStr*` — `str()`/`rrulestr()` round-trip tests with no expected instance list |
| 14 | assert something other than an instance list: `count()`, `before()`, `in`, `isinstance`, `str()` of the rule, `testStrAppendRRULEToken`, cache internals |
| 6 | `WeekdayTest` — weekday-object helper tests, not recurrence expansion |
| 5 | `RRuleSetTest.*Count` / cache tests (count only, built in loops) |
| 4 | `__getitem__` index / non-prefix slice (`rr[0]`, `rr[-1]`, `rr[1:2]`, `rr[::-2]`) |
| 2 | Python-2-only code (`long()`) |
| 2 | non-IANA zones: `tz.tzstr('UTC+04')` / `tzids=` mapping |
| 2 | no explicit dtstart (`freeze_time` + `datetime.now()` default) |
| 1 | warning-only test (`COUNT`+`UNTIL` deprecation, no assertion on instances) |
| 1 | sub-second (microsecond) dtstart |
| 1 | `tzids=` callable raising a custom exception |
| 1 | `DTSTART:19970101T000000,19970202T000000` (two DTSTART values) |
| 1 | `rruleset` with RDATE/EXDATE only and no rule (no DTSTART to anchor) |

Additionally 3 functions were only partially extracted (`testReplaceIfSet`,
`testReplaceIfNotSet`: `rr.replace(...)` assertions skipped; `testToStrMonthlyByWeekDay`: the
round-trip part skipped, its `between()` assertion kept).
