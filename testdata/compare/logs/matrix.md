
## 1 parsing

### 1.01-lowercase-names — lowercase names (begin:vcalendar, dtstart:…)

_RFC basis: §3.1 / §2: names of properties, parameters and enumerated values are case-insensitive_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
|  | pass: 2026-01-01T10:00:00Z | error: line 1: expected begin | error: line 1: expected begin | pass: 2026-01-01T10:00:00Z |

### 1.02-fold-space — folding with SPACE

_RFC basis: §3.1 unfolding: CRLF + one SPACE or HTAB removed_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
|  | pass: "HelloWorld" | pass: "HelloWorld" | pass: "HelloWorld" | pass: "HelloWorld" |

### 1.03-fold-htab — folding with HTAB

_RFC basis: §3.1 unfolding: CRLF + one SPACE or HTAB removed_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
|  | pass: "HelloWorld" | pass: "HelloWorld" | pass: "HelloWorld" | pass: "HelloWorld" |

### 1.04-fold-in-utf8 — fold inside a multi-byte UTF-8 character

_RFC basis: §3.1 note: unfold so a split multi-octet sequence is restored_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
|  | pass: "café €😀" | pass: "café €😀" | pass: "café €😀" | pass: "café €😀" |

### 1.05-quoted-param — quoted parameter values containing : ; ,

_RFC basis: §3.1 quoted-string / §3.2: values with COLON, SEMICOLON or COMMA MUST be quoted_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
|  | pass: CN=["Doe, John; Jr: III"] X-URL=["http://x.example/a:b;c,d"] value="mailto:j@example.com" | pass: CN=["Doe, John; Jr: III"] X-URL=["http://x.example/a:b;c,d"] value="mailto:j@example.com" | pass: CN=["Doe, John; Jr: III"] X-URL=["http://x.example/a:b;c,d"] value="mailto:j@example.com" | pass: CN=["Doe, John; Jr: III"] X-URL=["http://x.example/a:b;c,d"] value="mailto:j@example.com" |

### 1.06-rfc6868 — RFC 6868 parameter escapes ^' ^n ^^

_RFC basis: RFC 6868 §3 (an extension RFC, not part of 5545)_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
|  | pass: ["George \"Babe\" Ruth ^ \nX"] | **WRONG**: ["George ^'Babe^' Ruth ^^ ^nX"] | **WRONG**: ["George ^'Babe^' Ruth ^^ ^nX"] | **WRONG**: ["George ^'Babe^' Ruth ^^ ^nX"] |

### 1.07-text-unescape — TEXT unescaping \, \; \n \\ (and \\n is backslash+n)

_RFC basis: §3.3.11 TEXT: ESCAPED-CHAR_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
|  | pass: "a,b;c\nd\\e\\nf" | pass: "a,b;c\nd\\e\\nf" | pass: "a,b;c\nd\\e\\nf" | pass: "a,b;c\nd\\e\\nf" |

### 1.08-summary-comma — SUMMARY:Dinner, drinks is one value

_RFC basis: §3.8.1.12: SUMMARY is a single TEXT (the unescaped comma is itself non-conforming per §3.3.11, but SUMMARY is not a list)_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
|  | pass: "Dinner, drinks" | pass: "Dinner, drinks" | pass: "Dinner, drinks" | **WRONG**: "Dinner" |

### 1.09-two-calendars — two VCALENDARs in one stream

_RFC basis: §3.4: an iCalendar stream holds one or more iCalendar objects (icalstream = 1*icalobject)_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
|  | pass: 2 calendars, 2 events | error: 0 calendar(s), 0 event(s), error after them: line 10: unexpected end | error: 0 calendar(s), 0 event(s), error after them: line 10: unexpected end | pass: 2 calendars, 2 events |

### 1.10-lf-only — LF-only line endings

_RFC basis: §3.1 requires CRLF; LF-only is non-conforming but common. Pass = read the same event_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
|  | pass: HelloWorld | pass: HelloWorld | pass: HelloWorld | pass: HelloWorld |

### 1.11a-missing-end-vcalendar — feed missing only END:VCALENDAR

_RFC basis: §3.4/§3.6: every BEGIN needs its END. Pass = an error, not a silent shorter calendar_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
|  | pass: error: ical: line 9: input ends inside VCALENDAR (BEGIN on line 1) | **WRONG**: silent success, 1 event(s) | **WRONG**: silent success, 1 event(s) | **WRONG**: bare io.EOF (a Decode loop reads it as end of stream: 0 calendars, no error) |

### 1.11b-cut-inside-vevent — feed cut inside a VEVENT (after a whole line)

_RFC basis: §3.4/§3.6: every BEGIN needs its END. Pass = an error, not a silent shorter calendar_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
|  | pass: error: ical: line 7: input ends inside VEVENT (BEGIN on line 1) | pass: error: line 7: ran out of lines | pass: error: line 7: ran out of lines | **WRONG**: bare io.EOF (a Decode loop reads it as end of stream: 0 calendars, no error) |

### 1.11c-cut-mid-line — feed cut mid-line inside a parameter (…DTSTART;TZID=Amer)

_RFC basis: §3.4/§3.6: every BEGIN needs its END. Pass = an error, not a silent shorter calendar_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
|  | pass: error: ical: line 6: input ends inside VEVENT (BEGIN on line 1) | pass: error: line 7: parsing property DTSTART: unexpected end of property DTSTART | pass: error: line 6: ran out of lines | **PANIC** runtime error: index out of range [0] with length 0 |

### 1.12-empty — empty input

_RFC basis: §3.4: an iCalendar stream holds at least one object. Pass = an error, or io.EOF from a stream decoder_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
|  | pass: error: ical: line 1: no VCALENDAR in the input | **WRONG**: silent success, 0 event(s) | **WRONG**: silent success, 0 event(s) | pass: io.EOF: no calendars |

### 1.13-bom — UTF-8 BOM before BEGIN:VCALENDAR

_RFC basis: §3.1.4 (UTF-8); RFC is silent on a BOM, real feeds have one. Pass = read the event_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
|  | pass: 1 events | pass: 1 events | pass: 1 events | error: ical: malformed component: expected BEGIN property, got "\ufeffBEGIN" |

## 2 writing

### 2.01-fold-long-line — a 233-octet SUMMARY is folded to ≤75 octets with CRLF and reads back

_RFC basis: §3.1: lines SHOULD NOT exceed 75 octets; CRLF line breaks_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
|  | pass: longest line 75 octets | **WRONG**: 12 LF-only line breaks (CRLF required) | pass: longest line 75 octets | **WRONG**: line 6 is 233 octets |

### 2.02-fold-utf8 — folding never splits a UTF-8 sequence (SUMMARY of é€😀 ×20)

_RFC basis: §3.1 note and §6: fold between characters, not inside a multi-octet sequence_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
|  | pass: folded, longest line 73 octets, no split rune | pass: folded, longest line 73 octets, no split rune | pass: folded, longest line 73 octets, no split rune | **WRONG**: not folded at all (longest line 188 octets), so nothing to split |

### 2.03-text-escape — TEXT escaping of , ; \ and newline

_RFC basis: §3.3.11 TEXT: BACKSLASH, SEMICOLON, COMMA escaped; newline as \n_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
|  | pass: SUMMARY:a\,b\;c\\d\ne | pass: SUMMARY:a\,b\;c\\d\ne | pass: SUMMARY:a\,b\;c\\d\ne | pass: SUMMARY:a\,b\;c\\d\ne |

### 2.04a-inject-crlf — SUMMARY = "x\r\nATTENDEE:mailto:evil@example.com"

_RFC basis: §3.1: a value can't contain a line break; §3.3.11: newline is written \n. Pass = escaped or refused, never a new content line_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
|  | pass: escaped | **WRONG**: raw CR written inside a line (readers that accept a lone CR as a line break would split it) | **WRONG**: raw CR written inside a line (readers that accept a lone CR as a line break would split it) | pass: refused: ical: failed to encode property value: contains a CR or LF |

### 2.04b-inject-cr — SUMMARY = "x\rATTENDEE:mailto:evil@example.com" (lone CR)

_RFC basis: §3.1: a value can't contain a line break; §3.3.11: newline is written \n. Pass = escaped or refused, never a new content line_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
|  | pass: escaped | **WRONG**: INJECTED via a raw CR in the output: readers [this] take the lone CR as a line break and report an extra ATTENDEE:mailto:evil@example.com | **WRONG**: INJECTED via a raw CR in the output: readers [this] take the lone CR as a line break and report an extra ATTENDEE:mailto:evil@example.com | pass: refused: ical: failed to encode property value: contains a CR or LF |

### 2.04c-inject-lf — SUMMARY = "x\nATTENDEE:mailto:evil@example.com" (lone LF)

_RFC basis: §3.1: a value can't contain a line break; §3.3.11: newline is written \n. Pass = escaped or refused, never a new content line_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
|  | pass: escaped | pass: escaped | pass: escaped | pass: escaped |

### 2.04d-inject-param — parameter CN = "x\r\nATTENDEE:mailto:evil@example.com"

_RFC basis: §3.1: parameter values can't hold CONTROL characters. Pass = refused or neutralized_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
|  | pass: escaped | **WRONG**: raw line break written: the output has lines that aren't content lines: "ATTENDEE\\:mailto\\:evil@example.com:mailto:a@example.com" | **WRONG**: raw line break written: the output has lines that aren't content lines: "ATTENDEE\\:mailto\\:evil@example.com:mailto:a@example.com" | **WRONG**: INJECTED: the output has a CRLF-delimited line ATTENDEE:mailto:evil@example.com; readers that report the extra ATTENDEE: [this golang-ical+o… |

### 2.05-param-colon-quoted — a parameter value containing ':' (CN="Dr: Who; Jr, III") is written as a quoted-string

_RFC basis: §3.2: values with COLON, SEMICOLON or COMMA MUST be quoted-strings (no backslash escapes in parameters)_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
|  | pass: ATTENDEE;CN="Dr: Who; Jr, III":mailto:a@example.com | **WRONG**: ATTENDEE;CN=Dr\: Who\; Jr\, III:mailto:a@example.com | **WRONG**: ATTENDEE;CN=Dr\: Who\; Jr\, III:mailto:a@example.com | pass: ATTENDEE;CN="Dr: Who; Jr, III":mailto:a@example.com |

## 3 hostile input

### 3.01-long-param-value — one parameter value of N bytes: X;A=bbbb…:v

_RFC basis: robustness (no RFC limit); pass = finishes, time should scale ~linearly_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
| 64Ki | pass: ok: 1 calendar(s), 1 event(s) [0.1 ms, RSS 12 MB] | pass: ok: 1 calendar(s), 1 event(s) [0.1 ms, RSS 12 MB] | pass: ok: 1 calendar(s), 1 event(s) [0.1 ms, RSS 12 MB] | pass: ok: 1 calendar(s), 1 event(s) [144.6 ms, RSS 20 MB] |
| 256Ki | pass: ok: 1 calendar(s), 1 event(s) [0.6 ms, RSS 15 MB] | pass: ok: 1 calendar(s), 1 event(s) [0.7 ms, RSS 15 MB] | pass: ok: 1 calendar(s), 1 event(s) [0.7 ms, RSS 15 MB] | pass: ok: 1 calendar(s), 1 event(s) [2267.4 ms, RSS 24 MB] |
| 1Mi | pass: ok: 1 calendar(s), 1 event(s) [1.6 ms, RSS 20 MB] | pass: ok: 1 calendar(s), 1 event(s) [2.0 ms, RSS 20 MB] | pass: ok: 1 calendar(s), 1 event(s) [2.0 ms, RSS 21 MB] | pass: ok: 1 calendar(s), 1 event(s) [25763.4 ms, RSS 618 MB] |
| 4Mi | pass: ok: 1 calendar(s), 1 event(s) [6.2 ms, RSS 34 MB] | pass: ok: 1 calendar(s), 1 event(s) [6.3 ms, RSS 42 MB] | pass: ok: 1 calendar(s), 1 event(s) [6.3 ms, RSS 42 MB] | **HANG** (killed after 60 s timeout) |

Scaling (time at the largest size that finished ÷ time at the size below it, for 4× the input; ~4 is linear, ~16 quadratic):

- this: 1Mi→4Mi ×4.0 (1.6 ms → 6.2 ms)
- golang-ical: 1Mi→4Mi ×3.2 (2.0 ms → 6.3 ms)
- golang-ical+opts: 1Mi→4Mi ×3.2 (2.0 ms → 6.3 ms)
- emersion: 256Ki→1Mi ×11.4 (2267.4 ms → 25763.4 ms); did not finish: 4Mi hang

### 3.02-many-params — a line of N bytes of short parameters: X;A=b;A=b;…:v

_RFC basis: robustness; pass = finishes, time/memory should scale ~linearly_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
| 64Ki | pass: ok: 1 calendar(s), 1 event(s) [1.5 ms, RSS 18 MB] | pass: ok: 1 calendar(s), 1 event(s) [144.1 ms, RSS 21 MB] | pass: ok: 1 calendar(s), 1 event(s) [141.3 ms, RSS 23 MB] | pass: ok: 1 calendar(s), 1 event(s) [0.6 ms, RSS 14 MB] |
| 256Ki | pass: ok: 1 calendar(s), 1 event(s) [6.8 ms, RSS 29 MB] | pass: ok: 1 calendar(s), 1 event(s) [3112.3 ms, RSS 34 MB] | pass: ok: 1 calendar(s), 1 event(s) [3298.0 ms, RSS 44 MB] | pass: ok: 1 calendar(s), 1 event(s) [3.5 ms, RSS 20 MB] |
| 1Mi | pass: ok: 1 calendar(s), 1 event(s) [23.2 ms, RSS 55 MB] | pass: ok: 1 calendar(s), 1 event(s) [10359.8 ms, RSS 1266 MB] | pass: ok: 1 calendar(s), 1 event(s) [9815.9 ms, RSS 1484 MB] | pass: ok: 1 calendar(s), 1 event(s) [15.0 ms, RSS 31 MB] |
| 4Mi | pass: ok: 1 calendar(s), 1 event(s) [74.5 ms, RSS 173 MB] | **HANG** (killed after 60 s timeout) | **HANG** (killed after 60 s timeout) | pass: ok: 1 calendar(s), 1 event(s) [52.1 ms, RSS 75 MB] |
| 5Mi | pass: ok: 1 calendar(s), 1 event(s) [110.9 ms, RSS 267 MB] | **HANG** (killed after 60 s timeout) | **HANG** (killed after 60 s timeout) | pass: ok: 1 calendar(s), 1 event(s) [63.4 ms, RSS 106 MB] |

Scaling (time at the largest size that finished ÷ time at the size below it, for 4× the input; ~4 is linear, ~16 quadratic):

- this: 4Mi→5Mi ×1.5 (74.5 ms → 110.9 ms)
- golang-ical: 256Ki→1Mi ×3.3 (3112.3 ms → 10359.8 ms); did not finish: 4Mi hang, 5Mi hang
- golang-ical+opts: 256Ki→1Mi ×3.0 (3298.0 ms → 9815.9 ms); did not finish: 4Mi hang, 5Mi hang
- emersion: 4Mi→5Mi ×1.2 (52.1 ms → 63.4 ms)

### 3.03-comma-param-values — N comma-separated parameter values: X;A=b,b,b,…:v

_RFC basis: robustness; pass = finishes, linear_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
| 64Ki | pass: ok: 1 calendar(s), 1 event(s) [2.2 ms, RSS 20 MB] | pass: ok: 1 calendar(s), 1 event(s) [1071.0 ms, RSS 24 MB] | pass: ok: 1 calendar(s), 1 event(s) [1074.4 ms, RSS 24 MB] | pass: ok: 1 calendar(s), 1 event(s) [2.4 ms, RSS 18 MB] |
| 256Ki | pass: ok: 1 calendar(s), 1 event(s) [6.5 ms, RSS 37 MB] | pass: ok: 1 calendar(s), 1 event(s) [4289.2 ms, RSS 1698 MB] | pass: ok: 1 calendar(s), 1 event(s) [4352.9 ms, RSS 1319 MB] | pass: ok: 1 calendar(s), 1 event(s) [9.7 ms, RSS 31 MB] |
| 1Mi | pass: ok: 1 calendar(s), 1 event(s) [25.4 ms, RSS 78 MB] | pass: ok: 1 calendar(s), 1 event(s) [20297.3 ms, RSS 1445 MB] | pass: ok: 1 calendar(s), 1 event(s) [19177.8 ms, RSS 1474 MB] | pass: ok: 1 calendar(s), 1 event(s) [32.9 ms, RSS 82 MB] |
| 4Mi | pass: ok: 1 calendar(s), 1 event(s) [94.3 ms, RSS 253 MB] | **HANG** (killed after 60 s timeout) | **HANG** (killed after 60 s timeout) | pass: ok: 1 calendar(s), 1 event(s) [114.8 ms, RSS 253 MB] |

Scaling (time at the largest size that finished ÷ time at the size below it, for 4× the input; ~4 is linear, ~16 quadratic):

- this: 1Mi→4Mi ×3.7 (25.4 ms → 94.3 ms)
- golang-ical: 256Ki→1Mi ×4.7 (4289.2 ms → 20297.3 ms); did not finish: 4Mi hang
- golang-ical+opts: 256Ki→1Mi ×4.4 (4352.9 ms → 19177.8 ms); did not finish: 4Mi hang
- emersion: 1Mi→4Mi ×3.5 (32.9 ms → 114.8 ms)

### 3.04-comma-text-values — CATEGORIES with N comma-separated values, read with the list accessor

_RFC basis: robustness; §3.8.1.2 CATEGORIES is a TEXT list_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
| 64Ki | pass: 65537 values [2.2 ms, RSS 19 MB] | pass: ok: 1 calendar(s), 1 event(s) [2.3 ms, RSS 13 MB] | pass: ok: 1 calendar(s), 1 event(s) [2.6 ms, RSS 13 MB] | pass: 65537 values [2.0 ms, RSS 18 MB] |
| 256Ki | pass: 262145 values [8.7 ms, RSS 32 MB] | pass: ok: 1 calendar(s), 1 event(s) [10.1 ms, RSS 18 MB] | pass: ok: 1 calendar(s), 1 event(s) [9.8 ms, RSS 18 MB] | pass: 262145 values [8.0 ms, RSS 28 MB] |
| 1Mi | pass: 1048577 values [29.6 ms, RSS 82 MB] | pass: ok: 1 calendar(s), 1 event(s) [38.7 ms, RSS 27 MB] | pass: ok: 1 calendar(s), 1 event(s) [36.9 ms, RSS 27 MB] | pass: 1048577 values [26.9 ms, RSS 63 MB] |
| 4Mi | pass: 4194305 values [119.4 ms, RSS 289 MB] | pass: ok: 1 calendar(s), 1 event(s) [143.0 ms, RSS 61 MB] | pass: ok: 1 calendar(s), 1 event(s) [145.0 ms, RSS 62 MB] | pass: 4194305 values [98.2 ms, RSS 185 MB] |

Scaling (time at the largest size that finished ÷ time at the size below it, for 4× the input; ~4 is linear, ~16 quadratic):

- this: 1Mi→4Mi ×4.0 (29.6 ms → 119.4 ms)
- golang-ical: 1Mi→4Mi ×3.7 (38.7 ms → 143.0 ms)
- golang-ical+opts: 1Mi→4Mi ×3.9 (36.9 ms → 145.0 ms)
- emersion: 1Mi→4Mi ×3.6 (26.9 ms → 98.2 ms)

### 3.05-deep-nesting — N nested BEGIN:X lines with no END (unterminated nesting)

_RFC basis: robustness; pass = an error without crashing (the input is truncated)_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
| 100000 | pass: error: ical: line 35: components nest deeper than 32 [0.0 ms, RSS 13 MB] | pass: error: line 1004: component nesting too deep [0.6 ms, RSS 17 MB] | pass: error: line 1004: component nesting too deep [0.7 ms, RSS 17 MB] | **WRONG**: ok: 0 calendar(s), 0 event(s) [23.1 ms, RSS 69 MB] |
| 1000000 | pass: error: ical: line 35: components nest deeper than 32 [0.0 ms, RSS 29 MB] | pass: error: line 1004: component nesting too deep [0.9 ms, RSS 32 MB] | pass: error: line 1004: component nesting too deep [0.7 ms, RSS 32 MB] | **WRONG**: ok: 0 calendar(s), 0 event(s) [215.0 ms, RSS 497 MB] |
| 10000000 | pass: error: ical: line 35: components nest deeper than 32 [0.0 ms, RSS 185 MB] | pass: error: line 1004: component nesting too deep [0.6 ms, RSS 185 MB] | pass: error: line 1004: component nesting too deep [0.5 ms, RSS 185 MB] | **CRASH** runtime: goroutine stack exceeds 1000000000-byte limit |

Scaling (time at the largest size that finished ÷ time at the size below it, for 4× the input; ~4 is linear, ~16 quadratic):

- this: 1000000→10000000 ×1.4 (0.0 ms → 0.0 ms)
- golang-ical: 1000000→10000000 ×0.7 (0.9 ms → 0.6 ms)
- golang-ical+opts: 1000000→10000000 ×0.7 (0.7 ms → 0.5 ms)
- emersion: 100000→1000000 ×9.3 (23.1 ms → 215.0 ms); did not finish: 10000000 crash

### 3.06-deep-nesting-balanced — N nested BEGIN:X … END:X (balanced)

_RFC basis: robustness; either a clean error (a depth limit) or success, never a crash_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
| 100000 | pass: clean error: ical: line 35: components nest deeper than 32 [0.0 ms, RSS 15 MB] | pass: clean error: line 1004: component nesting too deep [0.7 ms, RSS 17 MB] | pass: clean error: line 1004: component nesting too deep [1.2 ms, RSS 18 MB] | pass: ok: 1 calendar(s), 0 event(s) [31.1 ms, RSS 71 MB] |
| 1000000 | pass: clean error: ical: line 35: components nest deeper than 32 [0.0 ms, RSS 43 MB] | pass: clean error: line 1004: component nesting too deep [0.5 ms, RSS 43 MB] | pass: clean error: line 1004: component nesting too deep [0.5 ms, RSS 44 MB] | pass: ok: 1 calendar(s), 0 event(s) [310.5 ms, RSS 513 MB] |

Scaling (time at the largest size that finished ÷ time at the size below it, for 4× the input; ~4 is linear, ~16 quadratic):

- this: 100000→1000000 ×0.7 (0.0 ms → 0.0 ms)
- golang-ical: 100000→1000000 ×0.7 (0.7 ms → 0.5 ms)
- golang-ical+opts: 100000→1000000 ×0.4 (1.2 ms → 0.5 ms)
- emersion: 100000→1000000 ×10.0 (31.1 ms → 310.5 ms)

### 3.07-malformed-lines — truncated/malformed content lines (inside a VEVENT, and as the cut-off last line)

_RFC basis: §3.1 grammar; pass = error or documented recovery, no panic_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
| param-no-colon | pass: mid-feed: ok: 1 calendar(s), 1 event(s) \| cut-off feed: error: ical: line 5: input ends inside VEVENT (BEGIN on line 1) | pass: mid-feed: error: line 7: parsing property DTSTART: unexpected end of property DTSTART \| cut-off feed: error: line 6: parsing property DTSTAR… | pass: mid-feed: ok: 1 calendar(s), 1 event(s) \| cut-off feed: error: line 5: ran out of lines | **PANIC** runtime error: index out of range [0] with length 0 |
| text-after-quote | pass: mid-feed: ok: 1 calendar(s), 1 event(s) \| cut-off feed: error: ical: line 6: input ends inside VEVENT (BEGIN on line 1) | pass: mid-feed: error: line 7: parsing property X: unexpected character 'c' \| cut-off feed: error: line 6: parsing property X: unexpected characte… | pass: mid-feed: ok: 1 calendar(s), 1 event(s) \| cut-off feed: error: line 6: ran out of lines | **PANIC** ical: unexpected character 'c' after decoding param value |
| param-at-eof | pass: mid-feed: ok: 1 calendar(s), 1 event(s) \| cut-off feed: error: ical: line 5: input ends inside VEVENT (BEGIN on line 1) | pass: mid-feed: error: line 7: parsing property X: unexpected end of property X \| cut-off feed: error: line 6: parsing property X: unexpected end … | pass: mid-feed: ok: 1 calendar(s), 1 event(s) \| cut-off feed: error: line 5: ran out of lines | **PANIC** runtime error: index out of range [0] with length 0 |
| param-no-equals | pass: mid-feed: ok: 1 calendar(s), 1 event(s) \| cut-off feed: error: ical: line 6: input ends inside VEVENT (BEGIN on line 1) | pass: mid-feed: error: line 7: parsing property X: missing property value for A in X \| cut-off feed: error: line 6: parsing property X: missing pr… | pass: mid-feed: ok: 1 calendar(s), 1 event(s) \| cut-off feed: error: line 6: ran out of lines | pass: mid-feed: error: ical: malformed param: missing equal sign \| cut-off feed: error: ical: malformed param: missing equal sign |
| quote-unterminated | pass: mid-feed: ok: 1 calendar(s), 1 event(s) \| cut-off feed: error: ical: line 5: input ends inside VEVENT (BEGIN on line 1) | pass: mid-feed: error: line 7: parsing property X: unexpected end of property X \| cut-off feed: error: line 6: parsing property X: unexpected end … | pass: mid-feed: ok: 1 calendar(s), 1 event(s) \| cut-off feed: error: line 6: ran out of lines | pass: mid-feed: error: ical: malformed param value: unterminated quoted string \| cut-off feed: error: ical: malformed param value: unterminated qu… |
| empty-name | pass: mid-feed: ok: 1 calendar(s), 1 event(s) \| cut-off feed: error: ical: line 5: input ends inside VEVENT (BEGIN on line 1) | pass: mid-feed: error: line 7: unexpected end of property v \| cut-off feed: error: line 6: unexpected end of property v | pass: mid-feed: ok: 1 calendar(s), 1 event(s) \| cut-off feed: error: line 5: ran out of lines | pass: mid-feed: error: ical: malformed content line: empty property name \| cut-off feed: error: ical: malformed content line: empty property name |
| name-only | pass: mid-feed: ok: 1 calendar(s), 1 event(s) \| cut-off feed: error: ical: line 5: input ends inside VEVENT (BEGIN on line 1) | pass: mid-feed: error: line 7: unexpected end of property X \| cut-off feed: error: line 6: unexpected end of property X | pass: mid-feed: ok: 1 calendar(s), 1 event(s) \| cut-off feed: error: line 5: ran out of lines | pass: mid-feed: error: ical: malformed content line: missing colon \| cut-off feed: error: ical: malformed content line: missing colon |

### 3.08-throughput — throughput on an ordinary 20,000-event feed (~9 MB): parse only, and parse + read DTSTART/DTEND of every event (best of 5)

_RFC basis: performance (the docs claim regexes and per-property time.LoadLocation cost the other libraries)_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
| parse | pass: 396 MB/s (24.5 ms for 9.7 MB, 20000 events) [24.5 ms, RSS 92 MB] | pass: 75 MB/s (128.5 ms for 9.7 MB, 20000 events) [128.5 ms, RSS 144 MB] | pass: 77 MB/s (125.2 ms for 9.7 MB, 20000 events) [125.2 ms, RSS 151 MB] | pass: 168 MB/s (57.6 ms for 9.7 MB, 20000 events) [57.6 ms, RSS 141 MB] |
| parse+times | pass: 311 MB/s (31.1 ms for 9.7 MB, 20000 events) [31.1 ms, RSS 80 MB] | pass: 13 MB/s (741.4 ms for 9.7 MB, 20000 events) [741.4 ms, RSS 184 MB] | pass: 13 MB/s (733.5 ms for 9.7 MB, 20000 events) [733.5 ms, RSS 187 MB] | pass: 15 MB/s (642.8 ms for 9.7 MB, 20000 events) [642.8 ms, RSS 189 MB] |

## 4 time semantics

### 4.01-floating-host-tz — floating DTSTART:20260715T090000 read with the default accessor under TZ=UTC vs TZ=America/Los_Angeles

_RFC basis: §3.3.5 form #1: floating is bound to no zone; result must not silently depend on the host's zone_

| lib | TZ=UTC | TZ=America/Los_Angeles | verdict |
|---|---|---|---|
| this | info: 2026-07-15T09:00:00Z | info: 2026-07-15T09:00:00Z | host-independent (pass) |
| golang-ical | info: 2026-07-15T09:00:00Z | info: 2026-07-15T16:00:00Z | **HOST-DEPENDENT (wrong)** |
| golang-ical+opts | info: 2026-07-15T09:00:00Z | info: 2026-07-15T16:00:00Z | **HOST-DEPENDENT (wrong)** |
| emersion | info: 2026-07-15T09:00:00Z | info: 2026-07-15T09:00:00Z | host-independent (pass) |

### 4.02-date-host-tz — DATE DTSTART;VALUE=DATE:20260715 read with the default accessor under TZ=UTC vs TZ=America/Los_Angeles

_RFC basis: §3.3.4: a DATE has no zone; result must not silently depend on the host's zone_

| lib | TZ=UTC | TZ=America/Los_Angeles | verdict |
|---|---|---|---|
| this | info: 2026-07-15T00:00:00Z | info: 2026-07-15T00:00:00Z | host-independent (pass) |
| golang-ical | info: 2026-07-15T00:00:00Z | info: 2026-07-15T07:00:00Z | **HOST-DEPENDENT (wrong)** |
| golang-ical+opts | info: 2026-07-15T00:00:00Z | info: 2026-07-15T07:00:00Z | **HOST-DEPENDENT (wrong)** |
| emersion | info: 2026-07-15T00:00:00Z | info: 2026-07-15T00:00:00Z | host-independent (pass) |

### 4.03-windows-tzid-no-vtimezone — DTSTART;TZID=Eastern Standard Time:20260715T090000 with no VTIMEZONE → 13:00Z

_RFC basis: §3.2.19 requires a VTIMEZONE for each TZID, so this is outside the RFC; it is what Outlook/Exchange feeds send_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
|  | pass: 2026-07-15T13:00:00Z | error: unknown time zone Eastern Standard Time | pass: 2026-07-15T13:00:00Z | error: unknown time zone Eastern Standard Time |

### 4.04-windows-tzid-with-vtimezone — TZID=Eastern Standard Time with Outlook's VTIMEZONE for it → 13:00Z

_RFC basis: §3.6.5 / §3.2.19: the TZID refers to the VTIMEZONE in the calendar_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
|  | pass: 2026-07-15T13:00:00Z | error: unknown time zone Eastern Standard Time | pass: 2026-07-15T13:00:00Z | error: unknown time zone Eastern Standard Time |

### 4.05-custom-vtimezone — Outlook custom zone TZID:Customized Time Zone (US-Eastern rules): July 09:00 → 13:00Z, January 09:00 → 14:00Z

_RFC basis: §3.6.5: the VTIMEZONE defines the zone; no IANA name exists_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
| 20260715T090000=2026-07-15T13:00:00Z | pass: 2026-07-15T13:00:00Z | error: unknown time zone Customized Time Zone | error: unknown time zone Customized Time Zone | error: unknown time zone Customized Time Zone |
| 20260115T090000=2026-01-15T14:00:00Z | pass: 2026-01-15T14:00:00Z | error: unknown time zone Customized Time Zone | error: unknown time zone Customized Time Zone | error: unknown time zone Customized Time Zone |

### 4.06-dst-gap — DTSTART;TZID=America/New_York:20070311T023000 (spring-forward gap) → 07:30Z

_RFC basis: §3.3.5 form #3: a repeated time is the first occurrence; a time in a gap uses the offset before the gap_

| arg | this | golang-ical | golang-ical+opts | emersion | rrule-go |
|---|---|---|---|---|---|
|  | pass: 2007-03-11T07:30:00Z | **WRONG**: 2007-03-11T06:30:00Z | **WRONG**: 2007-03-11T06:30:00Z | **WRONG**: 2007-03-11T06:30:00Z | **WRONG**: 2007-03-11T06:30:00Z |

### 4.07-dst-fold — DTSTART;TZID=America/New_York:20071104T013000 (repeated hour) → 05:30Z

_RFC basis: §3.3.5 form #3: a repeated time is the first occurrence; a time in a gap uses the offset before the gap_

| arg | this | golang-ical | golang-ical+opts | emersion | rrule-go |
|---|---|---|---|---|---|
|  | pass: 2007-11-04T05:30:00Z | pass: 2007-11-04T05:30:00Z | pass: 2007-11-04T05:30:00Z | pass: 2007-11-04T05:30:00Z | pass: 2007-11-04T05:30:00Z |

## 5 durations

### 5.01-p1d-spring — DURATION:P1D from 2026-03-07 12:00 New York → 2026-03-08 12:00 EDT (23 h)

_RFC basis: §3.3.6: days and weeks are nominal (a day across a DST change is 23 or 25 h); §3.8.5.3 nominal vs exact_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
|  | pass: 2026-03-08T16:00:00Z | n/a: golang-ical has no DURATION reader: GetEndAt reads only DTEND (SetDuration exists for writing) | n/a: golang-ical has no DURATION reader: GetEndAt reads only DTEND (SetDuration exists for writing) | **WRONG**: 2026-03-08T17:00:00Z |

### 5.02-p1d-fall — DURATION:P1D from 2026-10-31 12:00 New York → 2026-11-01 12:00 EST (25 h)

_RFC basis: §3.3.6: days and weeks are nominal (a day across a DST change is 23 or 25 h); §3.8.5.3 nominal vs exact_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
|  | pass: 2026-11-01T17:00:00Z | n/a: golang-ical has no DURATION reader: GetEndAt reads only DTEND (SetDuration exists for writing) | n/a: golang-ical has no DURATION reader: GetEndAt reads only DTEND (SetDuration exists for writing) | **WRONG**: 2026-11-01T16:00:00Z |

### 5.03-pt24h-spring — DURATION:PT24H from 2026-03-07 12:00 New York → exactly 24 h later (13:00 EDT)

_RFC basis: §3.3.6: hours, minutes, seconds are exact_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
|  | pass: 2026-03-08T17:00:00Z | n/a: golang-ical has no DURATION reader: GetEndAt reads only DTEND (SetDuration exists for writing) | n/a: golang-ical has no DURATION reader: GetEndAt reads only DTEND (SetDuration exists for writing) | pass: 2026-03-08T17:00:00Z |

### 5.04-negative-duration — parse -PT15M (a typical VALARM TRIGGER) and P2W

_RFC basis: §3.3.6 dur-value = (["+"] / "-") "P" …_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
|  | pass: -15m0s, P2W (336h0m0s) | n/a: golang-ical has no DURATION parser | n/a: golang-ical has no DURATION parser | pass: -15m0s, 336h0m0s (as a time.Duration: weeks are exact) |

### 5.05-p2w-spring — DURATION:P2W from 2026-03-01 12:00 New York → 2026-03-15 12:00 EDT (335 h)

_RFC basis: §3.3.6: days and weeks are nominal (a day across a DST change is 23 or 25 h); §3.8.5.3 nominal vs exact_

| arg | this | golang-ical | golang-ical+opts | emersion |
|---|---|---|---|---|
|  | pass: 2026-03-15T16:00:00Z | n/a: golang-ical has no DURATION reader: GetEndAt reads only DTEND (SetDuration exists for writing) | n/a: golang-ical has no DURATION reader: GetEndAt reads only DTEND (SetDuration exists for writing) | **WRONG**: 2026-03-15T17:00:00Z |

## 6 recurrence

### 6.01-rfc-examples — the 43 RFC 5545 §3.8.5.3 examples (testdata/rfc5545/recur.json)

_RFC basis: §3.8.5.3 examples (erratum 3883 applied to case 33)_

| lib | exact matches of 43 | failures |
|---|---|---|
| this | 43 / 43 |  |
| rrule-go | 43 / 43 |  |
| golang-ical | 43 / 43 |  |
| emersion | 43 / 43 |  |

### 6.02-hourly-interval24-byhour — FREQ=HOURLY;INTERVAL=24;BYHOUR=11 from 09:00: ask for the first instance after DTSTART

_RFC basis: §3.3.10: 09:00 + 24h·k never has hour 11, so the rule generates nothing; must terminate_

| arg | this | rrule-go | emersion |
|---|---|---|---|
|  | pass: 0 instances [0.0 ms] | **HANG** (killed after 20 s timeout) | **HANG** (killed after 20 s timeout) |

### 6.03-duplicate-by-values — duplicate BYxxx values: FREQ=MINUTELY;COUNT=5;BYSECOND=0,0 and FREQ=DAILY;COUNT=3;BYHOUR=9,9 from 09:00

_RFC basis: §3.8.5.3: duplicate instances are ignored; COUNT counts instances_

| arg | this | rrule-go | emersion |
|---|---|---|---|
| FREQ=MINUTELY;COUNT=5;BYSECOND=0,0 | pass: 20260101T090000,20260101T090100,20260101T090200,20260101T090300,20260101T090400 | **WRONG**: 20260101T090000,20260101T090100,20260101T090200 | **WRONG**: 20260101T090000,20260101T090100,20260101T090200 |
| FREQ=DAILY;COUNT=3;BYHOUR=9,9 | pass: 20260101T090000,20260102T090000,20260103T090000 | **WRONG**: 20260101T090000,20260102T090000 | **WRONG**: 20260101T090000,20260102T090000 |

### 6.04-unmatchable — rules that can never match: time to learn there are no instances (DTSTART 2026-01-01 09:00 New York)

_RFC basis: §3.3.10: invalid dates are ignored, so these rules generate nothing; must terminate quickly_

| arg | this | rrule-go | emersion |
|---|---|---|---|
| FREQ=YEARLY;BYMONTH=2;BYMONTHDAY=30 | pass: 0 instances [0.0 ms] | pass: 0 instances [11.2 ms] | pass: 0 instances [10.9 ms] |
| FREQ=MONTHLY;BYMONTH=4,6,9,11;BYMONTHDAY=31 | pass: 0 instances [0.0 ms] | pass: 0 instances [15.5 ms] | pass: 0 instances [14.5 ms] |
| FREQ=MONTHLY;BYDAY=6MO | pass: 0 instances [0.0 ms] | pass: 0 instances [30.7 ms] | pass: 0 instances [32.2 ms] |
| FREQ=DAILY;BYMONTH=2;BYMONTHDAY=30 | pass: 0 instances [0.0 ms] | pass: 0 instances [54.2 ms] | pass: 0 instances [53.6 ms] |
| FREQ=DAILY;BYMONTH=2;BYMONTHDAY=30 @9000 | pass: 0 instances [0.0 ms] | pass: 0 instances [7.2 ms] | pass: 0 instances [6.8 ms] |

### 6.05a-iterator-mutates-rule — does iterating change the rule value? (rrule-go: unexported RRule.len; Set.Iterator sorts RDATEs in place)

_RFC basis: API property (not an RFC requirement)_

| arg | this | rrule-go |
|---|---|---|
|  | pass: rule equal after iteration: true; RDates[0] unchanged: true | **WRONG**: RRule.len 0 → 5 after All(); Set RDATE[0] 03-01 → 02-01 after All() |

### 6.05b-concurrent-iteration — 8 goroutines iterate one rule (and one set) at once, under the race detector

_RFC basis: API property: is a rule safe to share?_

| arg | this | rrule-go |
|---|---|---|
|  | pass: counts [102 102 102 102 102 102 102 102] (the driver checks stderr for DATA RACE) | **WRONG**: DATA RACE reported by the race detector |

### 6.06-month-of-1990-daily — one month (Oct 2026) of FREQ=DAILY begun 1990-01-01 09:00 New York (testing.Benchmark)

_RFC basis: performance; correctness = 31 instances_

| arg | this | rrule-go | emersion |
|---|---|---|---|
| no COUNT | pass: 31 instances; 4.96 µs/op, 17 allocs/op | pass: 31 instances; 2.19 ms/op, 10 allocs/op | pass: 31 instances; 2.38 ms/op, 13475 allocs/op |
| COUNT=20000 | pass: 31 instances; 370.98 µs/op, 53 allocs/op | pass: 31 instances; 2.19 ms/op, 10 allocs/op | pass: 31 instances; 2.40 ms/op, 13475 allocs/op |
| COUNT=20000 + cache | pass: 31 instances; 6.53 µs/op, 27 allocs/op | n/a: no cache in rrule-go | n/a: no cache in rrule-go |
| no COUNT, recur.Rule.Between (civil) | pass: 31 instances; 1.53 µs/op, 8 allocs/op | n/a: this library's API only | n/a: this library's API only |
| COUNT=20000, recur.Rule.Between (civil) | pass: 31 instances; 367.02 µs/op, 44 allocs/op | n/a: this library's API only | n/a: this library's API only |
| COUNT=20000 + cache, recur.Set.Between (civil) | pass: 31 instances; 6.28 µs/op, 27 allocs/op | n/a: no cache in rrule-go | n/a: no cache in rrule-go |

### 6.07-daily-across-dst-gap — FREQ=DAILY;COUNT=5 at 02:30 New York from 2007-03-09: the 03-11 instance is 03:30 EDT (07:30Z)

_RFC basis: §3.3.10 + erratum 4271 + §3.3.5: a gap instance is kept and uses the offset before the gap_

| arg | this | rrule-go | golang-ical | emersion |
|---|---|---|---|---|
|  | pass: 2007-03-09T07:30:00Z,2007-03-10T07:30:00Z,2007-03-11T07:30:00Z,2007-03-12T06:30:00Z,2007-03-13T06:30:00Z | **WRONG**: 2007-03-09T07:30:00Z,2007-03-10T07:30:00Z,2007-03-11T06:30:00Z,2007-03-12T06:30:00Z,2007-03-13T06:30:00Z | **WRONG**: 2007-03-09T07:30:00Z,2007-03-10T07:30:00Z,2007-03-11T06:30:00Z,2007-03-12T06:30:00Z,2007-03-13T06:30:00Z | **WRONG**: 2007-03-09T07:30:00Z,2007-03-10T07:30:00Z,2007-03-11T06:30:00Z,2007-03-12T06:30:00Z,2007-03-13T06:30:00Z |

### 6.08-unbounded-horizon — FREQ=YEARLY;BYMONTH=2;BYDAY=5MO from 2026-01-01 (5th Monday of February: leap years whose Feb 29 is a Monday), all instances

_RFC basis: §3.3.10 (numbered BYDAY counts within BYMONTH, errata 1913/3779); no UNTIL/COUNT, so it runs to year 9999. Brute force: 299 instances, 2044 … 9988_

| arg | this | rrule-go | emersion |
|---|---|---|---|
|  | pass: 299 instances, first 2044, last 9988 | **WRONG**: 11 instances, first 2044, last 2304 | **WRONG**: 11 instances, first 2044, last 2304 |
