# iCal.NET

| | |
|---|---|
| Upstream | https://github.com/ical-org/ical.net |
| Commit | `7a5264f2e4c0a983fce56b42044d656abd1f8d8e` (branch `main`, committed 2026-10-11) |
| Cloned | 2026-10-11 (`git clone --depth 1`) |
| License | `MIT` (see `LICENSE`, upstream `license.md`) |

## Copied (`ics/`, upstream paths preserved)

All 150 `.ics` files under `Ical.Net.Tests/Calendars/`: `Recurrence/` (71 — classic RFC
examples, BYSETPOS, EXDATE/RDATE, RECURRENCE-ID, DST, "bug" reproducers), `Serialization/`
(60 — attachments, attendees, escaping, folding, X-properties incl. Windows TZIDs in
`XProperty2.ics`, time zones, `Outlook2007*`, `Google1.ics`, ...), `Todo/` (9), `Alarm/` (7),
`Journal/` (3).

## Converted (`recur.json`, 92 cases)

`Ical.Net.Tests/Calendars/Recurrence/RecurrenceTestCases.txt` — ical.net's own vector file in
libical's `RRULE:/DTSTART:/START-AT:/INSTANCES:` format, extended with `EXCEPTION:`,
`EXCEPTION-STEP:` and `DURATION:`. Semantics were read from `Ical.Net.Tests/RecurrenceTests.cs`
(`ParseTestCaseFile`, `ExecuteRecurrenceTestCase`): DTSTART/INSTANCES/START-AT are parsed as
`CalDateTime(value, "UTC")` and evaluated in UTC (so floating values here equal UTC);
a non-matching DTSTART is not emitted (`include_dtstart: false`); START-AT → `between.start`
with `expected` filtered to `>= START-AT` exactly as the harness does; EXCEPTION blocks →
`error: true` with the .NET exception type and step in `comment` (several are ical.net-specific:
year-9999 overflow, unbounded evaluation, interval overflow). One block has no DTSTART (rule
parse error only) and uses the placeholder dtstart `19700101T000000` (see its comment). Seven
cases carry `NONSTANDARD:*` markers; ical.net rejects five of them (EvaluationException).

Script: `scripts/conv_libical_fmt.py` (scratch, not committed).

## Skipped

* `Ical.Net.Tests/contrib/libical/icalrecur_test.txt` — an older vendored copy of libical's
  vector file (MPL/LGPL); the current upstream file is in `../libical/recur.json`.
* Recurrence tests written as C# code in `RecurrenceTests.cs` etc. (expectations in code).
