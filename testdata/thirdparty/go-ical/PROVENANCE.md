# go-ical (emersion)

| | |
|---|---|
| Upstream | https://github.com/emersion/go-ical |
| Commit | `439c63cef608771fe1e93407b1b861e8c9a441ae` (branch `master`, committed 2025-06-09) |
| Cloned | 2026-10-11 (`git clone --depth 1`) |
| License | `MIT` (see `LICENSE`) |

## Copied (`ics/`)

go-ical has **no** `testdata/` directory or `.ics` files; its tests embed calendars in Go raw
string literals.

* `ics/extracted/ical_test.go/17.ics` — `exampleCalendarStr` from `ical_test.go` (line 17),
  converted to CRLF exactly as the test's `toCRLF()` does. It exercises a quoted `ALTREP`
  parameter, a parameter value list with a quoted value containing `:` (`FOO=bar,"b:az"`),
  escaped text (`\n`, `\,`) and an RRULE. The expected decode result is the `exampleCalendar`
  Go value in the same file.

## Converted

Nothing (go-ical delegates recurrence to `teambition/rrule-go`).

## Skipped

* The `TestDecoderLongLine` template in `decoder_test.go` (a `fmt` template whose DESCRIPTION is
  generated at run time with `strings.Repeat`).
