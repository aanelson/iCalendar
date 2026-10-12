## 7 corpus robustness

2200 files under testdata/thirdparty/*/ics and testdata/rfc5545/ics; per-file subprocess, 20 s timeout.

### all files (2200)

| lib | ok (≥1 calendar) | empty success (no error, no calendar) | error | panic | crash | hang |
|---|---|---|---|---|---|---|
| this | 2049 | 1 | 150 | 0 | 0 | 0 |
| this(strict) | 281 | 1 | 1918 | 0 | 0 | 0 |
| golang-ical | 1994 | 6 | 200 | 0 | 0 | 0 |
| golang-ical+opts | 2016 | 16 | 168 | 0 | 0 | 0 |
| emersion | 2024 | 11 | 158 | 7 | 0 | 0 |

### iCalendar-text files (.ics/.ical/.ifb, ical4j .txt) (2139)

| lib | ok (≥1 calendar) | empty success (no error, no calendar) | error | panic | crash | hang |
|---|---|---|---|---|---|---|
| this | 2045 | 1 | 93 | 0 | 0 | 0 |
| this(strict) | 281 | 1 | 1857 | 0 | 0 | 0 |
| golang-ical | 1993 | 6 | 140 | 0 | 0 | 0 |
| golang-ical+opts | 2014 | 6 | 119 | 0 | 0 | 0 |
| emersion | 2023 | 7 | 106 | 3 | 0 | 0 |

### other files (jCal .json, extensionless libical fuzz inputs, …) (61)

| lib | ok (≥1 calendar) | empty success (no error, no calendar) | error | panic | crash | hang |
|---|---|---|---|---|---|---|
| this | 4 | 0 | 57 | 0 | 0 | 0 |
| this(strict) | 0 | 0 | 61 | 0 | 0 | 0 |
| golang-ical | 1 | 0 | 60 | 0 | 0 | 0 |
| golang-ical+opts | 2 | 10 | 49 | 0 | 0 | 0 |
| emersion | 1 | 4 | 52 | 4 | 0 | 0 |

Files that this, golang-ical (default) and emersion all read without error: 1990. VEVENT counts differ on 0 of them.

### Panics, crashes and hangs

- testdata/thirdparty/libical/ics/test-data/fuzz42517512 — emersion: panic runtime error: index out of range [0] with length 0 
- testdata/thirdparty/libical/ics/test-data/fuzz52787 — emersion: panic runtime error: index out of range [0] with length 0 
- testdata/thirdparty/libical/ics/test-data/fuzz69344 — emersion: panic runtime error: index out of range [0] with length 0 
- testdata/thirdparty/libical/ics/test-data/fuzzubsan — emersion: panic runtime error: index out of range [0] with length 0 
- testdata/thirdparty/python-icalendar/ics/src/icalendar/tests/calendars/issue_348_exception_parsing_value.ics — emersion: panic runtime error: index out of range [0] with length 0 
- testdata/thirdparty/sabre-vobject/ics/extracted/tests/VObject/ITip/BrokerAttendeeReplyTest.php/191.ics — emersion: panic runtime error: index out of range [0] with length 0 
- testdata/thirdparty/sabre-vobject/ics/extracted/tests/VObject/ITip/BrokerAttendeeReplyTest.php/337.ics — emersion: panic runtime error: index out of range [0] with length 0 
