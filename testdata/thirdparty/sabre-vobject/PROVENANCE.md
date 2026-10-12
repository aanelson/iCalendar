# sabre/vobject

| | |
|---|---|
| Upstream | https://github.com/sabre-io/vobject |
| Commit | `5e2f338811916c03927398bb24e527989de63194` (branch `master`, committed 2026-10-05) |
| Cloned | 2026-10-11 (`git clone --depth 1`) |
| License | `BSD-3-Clause` (see `LICENSE`) |

## Copied (`ics/`)

* `tests/VObject/RecurrenceIterator/UntilRespectsTimezoneTest.ics` — the only `.ics` file in
  `tests/`.
* `ics/extracted/<test file>/<line>.ics` (264) — iCalendar documents embedded in the PHP test
  sources as heredoc/nowdoc blocks containing `BEGIN:VCALENDAR` … `END:VCALENDAR`, extracted by
  text processing (nothing executed). `<line>` is the line of the `<<<` opener in the test file.
  Indentation of PHP 7.3 flexible heredocs is removed as PHP does; line endings are LF as in the
  source. Mostly iTIP scheduling scenarios (`ITip/*`, 138: REQUEST/REPLY/CANCEL with
  RECURRENCE-ID exceptions, EXDATE, attendee changes), component and free-busy tests
  (`Component/*`, `FreeBusyGeneratorTest.php`), recurrence-iterator calendars (`Recur/*`, 23,
  with RECURRENCE-ID overrides, infinite rules, broken RRULEs), `TimeZoneUtilTest.php` (Windows
  display-name TZIDs such as `"Sarajevo, Skopje, Sofija, Vilnius, Warsaw, Zagreb"`,
  `(GMT+01.00) Sarajevo/Warsaw/Zagreb`, `X-MICROSOFT-CDO-TZID`, Evolution-style
  `/freeassociation.sourceforge.net/Tzfile/...`, `"UTC-04:00"`, unknown `foo`), birthday generator inputs, splitter inputs.

## Converted (`recur.json`, 93 cases)

Every `$this->parse(...)` call in `tests/VObject/Recur/RRuleIteratorTest.php`, including
`#[DataProvider]` rows (yield-with-label and `return [[...]]` forms), parsed as PHP literals.
Helper semantics (read from the helper at the end of the test file and from
`lib/Recur/RRuleIterator.php`): `RRuleIterator::rewind()` starts at DTSTART, so DTSTART is
always the first instance and counts toward COUNT (`include_dtstart: true`); default zone is
`UTC` (values written with `Z`), otherwise wall-clock in `tzid` (Europe/Zurich DST gap cases,
America/New_York); `fastForward(dt)` → `between.start` (inclusive, UTC); infinite rules compare
only `count(expected)` values (`limit`); `runTillTheEnd` means the iterator must terminate by
itself; `expectException(InvalidDataException)` → `error: true`.

Script: `scripts/conv_sabre.py`, `scripts/extract_inline.py` (scratch, not committed).

## Skipped

* `testYearlyByYearDayImmutable` and `testIteratorFunctions` (call the iterator API directly,
  no `parse()` helper).
* `RDateIteratorTest.php`, `EventIterator/*` tests (not RRULE-vector shaped).
* 61 embedded calendars: 53 heredocs that interpolate PHP variables (`$...`) and 8 heredocs
  containing backslash escapes (PHP would rewrite them, so a verbatim copy would differ from what
  the test feeds the parser).
* `.vcf` vCard fixtures.
