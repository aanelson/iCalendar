# iCal4j

| | |
|---|---|
| Upstream | https://github.com/ical4j/ical4j |
| Commit | `bd09358860598b07235a91061117d1cf46ba7638` (default branch `develop`, committed 2026-10-11) |
| Cloned | 2026-10-11 (`git clone --depth 1`) |
| License | `BSD-3-Clause` (see `LICENSE`, upstream `LICENSE.txt`) |

## Copied (`ics/`, upstream paths preserved — the valid/invalid split is kept)

* `src/test/resources/samples/valid/*.ics` (81) — files iCal4j expects to parse and validate:
  RFC 5545 section examples (`rfc5545-sec3.*`, `rfc5545-sec4.*`), CalConnect interop samples
  (`calconnect*.ics`), exports from Evolution (`evolution2.ics`, Windows-style TZIDs),
  KOrganizer, Sunbird, Google (`google_aus_holidays.ics`), Oracle, holiday feeds,
  `blankTzid.ics`, `dst-only-vtimezone.ics`, `EXDATE-IN-UTC.ics`, `multiple_calendars.ics`,
  `custom_component.ics`, `zidestoreical4jbomb.ics`. Note one filename contains a space
  (`New Years Day.ics`).
* `src/test/resources/samples/invalid/*.ics` (22) — files iCal4j expects to **fail** strict
  parsing/validation (`korganizer-lowercase.ics`, `sogo-geo-escaped-semicolon.ics`,
  `phpicalendar_sample.ics`, `13-MoonPhase.ics`, `0.ics`, ...). Some of these are accepted by
  lenient parsers; treat "invalid" as "iCal4j strict mode rejects it".
* `src/test/resources/samples/hcalendar/*.ics` (1), `src/test/resources/net/fortuna/ical4j/data/*.ics`
  (2), `.../model/*.ics` (1).
* `src/test/resources/net/fortuna/ical4j/transform/**/*.txt` (12) — real-world iCalendar
  payloads stored with a `.txt` extension and used by iCal4j's "transformer" (compatibility
  fix-up) tests: `outlook1/2.txt`, `timezones/outlook1/2.txt` with `*_expected.txt` (Outlook /
  Windows TZID → Olson rewriting), `outlook/TZ-no-description.txt`, `outlook/TZ-set-to-utc.txt`,
  `yahoo1/2.txt`, `apple.txt`, `dtstamp/invalid.txt`. Only `.txt` files that contain a
  `BEGIN:V...` component were copied.

## Converted

Nothing (iCal4j's recurrence tests are Groovy/Java code with in-code expectations).

## Skipped

* `src/main/resources/zoneinfo/**` and `zoneinfo-global/**` (iCal4j's bundled VTIMEZONE
  database — library data, not test fixtures; libical's `zoneinfo2025c` sets cover this).
* `samples/hcalendar/example1.html`, `logback.xml`, `log4j2.properties`, `ical4j.properties`,
  `tz.alias` (not iCalendar).
