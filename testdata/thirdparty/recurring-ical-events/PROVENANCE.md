# python-recurring-ical-events

| | |
|---|---|
| Upstream | https://github.com/niccokunzmann/python-recurring-ical-events |
| Commit | `b054fcbc9a1deba0a108663c51975216b0f293c2` (branch `main`, committed 2026-09-23) |
| Cloned | 2026-10-11 (`git clone --depth 1`) |
| License | **`LGPL-3.0-or-later` (copyleft)** — see `LICENSE` (GNU LGPL v3 text); `pyproject.toml` declares `LGPL-3.0-or-later` |

> **Copyleft notice.** These fixtures come from an LGPL-3.0 project. They are redistributed
> unmodified, with the license, inside this separate nested Go module that is excluded from the
> library's module zip and is never linked into library code. If that is not acceptable for
> your distribution, delete this directory; tests that use it must skip when it is absent.

## Copied (`ics/`, upstream paths preserved)

`recurring_ical_events/test/calendars/*` — 96 `.ics` files plus `date_exclude.txt` (an
iCalendar file with a `.txt` name). This is the richest recurrence-edge-case corpus here:
RECURRENCE-ID overrides and moved/deleted instances (`issue_62_moved_event*`,
`recurring_events_moved.ics`, `issue_163_deleted_modification.ics`, `issue_253_*`), SEQUENCE
interplay (`issue_148_*`, `issue_223_*`, ~60 files use SEQUENCE), `RANGE=THISANDFUTURE`
(`issue_75_range_parameter.ics`), RDATE with PERIOD (`issue_113_*`), UNTIL before DTSTART
(`issue_117_*`), DST (`daylight_saving_time.ics`, `issue_48_*`), Windows TZIDs
(`issue_27_t1/t2.ics`, `issue_28_rrule_with_UTC_endinginZ.ics`, `issue_107_*.ics`),
`X-WR-TIMEZONE` (`x_wr_timezone_simple_events_issue_59.ics`, `issue_86_*`), mixed DATE/DATE-TIME
(`issue_201_*`), alarms (`alarm_*`), bad RRULEs (`bad_rrule_missing_until_event.ics`),
`end_before_start_event.ics`, zero-length events, and large real calendars (`Germany.ics`,
`fablab_cottbus.ics`, `issue_173_only_modifications_error.ics` 212 KB).

## Converted (`expected.json`, 11 entries)

Hand-transcribed (each one checked against the test source) from tests that pin down the
**complete** set of occurrences for a query: `test_issue_4.py` (2), `test_issue_20_exdate_ignored.py`
(1), `test_issue_62_moved_event.py` (2), `test_issue_148_ignored_exdate_in_higher_sequence.py`
(2), `test_issue_197_rfc5545_examples.py` (4). Format: a JSON list of
`{"calendar", "start", "end", "expected_starts", "comment"}` with one optional extra key,
`summary_contains`, for the issue-197 calendar that holds four RFC examples in one file.

Semantics: `of(calendar).between(start, end)` / `.at(...)` return occurrences that **overlap**
the half-open window `[start, end)`; `at(day)` = `[day, day+1)`, `at((y, m))` = that month;
`start`/`end` = `null` means `.all()`. `expected_starts` are the occurrences' DTSTART values in
chronological order: `YYYYMMDD` where the test only asserts the date (or the event is all-day),
otherwise wall-clock `YYYYMMDDTHHMMSS` in the event's TZID (named in `comment`).

## Skipped

* Tests asserting only counts (`len(events) == n`), membership of a few dates, alarms, durations,
  property values or API behaviour — the large majority. Converting them would need running the
  library, which was not done.
* `benchmark/` calendars (not tests).
