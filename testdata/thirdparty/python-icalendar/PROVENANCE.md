# python icalendar (collective/icalendar)

| | |
|---|---|
| Upstream | https://github.com/collective/icalendar |
| Commit | `ec09f7e9f444e3d2a48b5b8cedb086729659e358` (branch `main`, committed 2026-10-11) |
| Cloned | 2026-10-11 (`git clone --depth 1`) |
| License | `BSD-2-Clause` (see `LICENSE`, upstream `LICENSE.rst`) |

## Copied (`ics/`, upstream paths preserved)

* `src/icalendar/tests/**/*.ics` — 162 files: `calendars/` (115 .ics), `events/` (34),
  `timezones/` (4), `alarms/` (4), `availabilities/` (2), `todos/`, `journals/`, `freebusy/`.
  Many are real-world or deliberately broken inputs: Outlook/Exchange Windows TZIDs
  (`issue_836_do_not_quote_tzid.ics`, `timezone_same_start*.ics`,
  `issue_156_RDATE_with_PERIOD_TZID_khal*.ics`), `broken_ical.ics`, `broken_dtstart.ics`,
  `big_bad_calendar.ics`, `small_bad_calendar.ics`, `parsing_error*.ics`, `invalid_duration.ics`,
  `bom_calendar.ics` (UTF-8 BOM), `fuzz_testcase_vtimezone_lone_cr.ics`, non-ASCII TZIDs
  (`issue_237_*`), DST ambiguity (`issue_722_timezone_transition_ambiguity.ics`),
  Thunderbird/Google/Etar alarm states (`alarm_*`), RFC 7529/7953/7986/9253/6868/7265 examples.
* `src/icalendar/tests/calendars/*.jcal` (5) — jCal counterparts of the RFC 7265 examples.
* `src/icalendar/fuzzing/corpus/*.ics` (34) — fuzzing seed corpus (outside `tests/`, included
  because it is iCalendar input of the same kind).

## Converted

Nothing (no recurrence vectors in this project's tests).

## Skipped

* `src/icalendar/tests/calendars/fuzz_testcase_0_char_in_component_name.ics` — contains NUL
  bytes (binary).
* `docs/tutorials/*.ics` (documentation, not tests); `src/icalendar/tests/fuzzed/` (Python
  test cases, not data files).
