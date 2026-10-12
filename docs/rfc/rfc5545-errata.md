# RFC 5545 errata

This is a summary of the errata list at <https://www.rfc-editor.org/errata/rfc5545>,
as of 2026-10-11. The list has 22 Verified, 4 Held for Document Update,
2 Reported and 11 Rejected errata. The summaries are paraphrased; see the
errata page for the official text. Each entry has an **Impact** line naming
the layers it affects: parser, writer, recurrence, timezone, or none.
"Test data" means the erratum corrects an RFC example that we use as a
fixture.

Where a Verified erratum corrects an example, `testdata/rfc5545/examples.json`
keeps the RFC's original text, names the erratum in `errata`, and adds a
`<id>-corrected` entry with the correction applied. `testdata/rfc5545/recur.json`
applies erratum 3883 directly, and also keeps the uncorrected rule as a
separate derived case.

## Verified errata that matter for implementation

| ID | Section | Impact | One-line effect |
|---|---|---|---|
| 1913, 3779 | 3.3.10 | recurrence, parser | In a YEARLY rule, a numeric BYDAY counts within the month if BYMONTH is present, otherwise within the year. A numeric BYDAY together with BYWEEKNO is not allowed. |
| 4271 | 3.3.10 | recurrence, timezone | Only invalid dates (Feb 30) are dropped. Instances in a DST gap are kept and shifted, as §3.3.5 describes. |
| 4414 | 3.3.10 | parser, writer, recurrence | UNTIL takes the same form as DTSTART. With a floating DTSTART, UNTIL is floating, not UTC. |
| 7691 | 3.8.4.2 | parser, writer | Backslash escapes don't apply inside quoted parameter values. |
| 2516 | 3.8.4.1 | parser, writer | SENT-BY (like the other cal-address parameters) must be DQUOTEd. |
| 2039 | 4 | parser | A DATE-TIME TRIGGER needs `VALUE=DATE-TIME`, because the default type is DURATION. |
| 2677 | 3.8.1.10 | parser | RESOURCES may appear more than once. |
| 5602 | 3.1.3 | parser | BASE64 values carry `=` padding, as RFC 4648 requires. |
| 3883 | 3.8.5.3 | recurrence (test data) | Corrected UNTIL in the "every 3 hours" example. |
| 3740 | 4 | timezone (test data) | Corrected VTIMEZONE in the §4 group-meeting example. |
| 3747 | 3.3.10 | none (see caution) | Edits Note 2 of the BYxxx table. Don't change behaviour because of it. |

## Verified (22)

### Technical

**1911, §2.1 (character table).** The table of character names lists the
decimal code point of DQUOTE as 22, which is its hexadecimal value. The
correct decimal value is 34. The ABNF (`DQUOTE = %x22`) was always right.
**Impact:** none.

**1913, §3.3.10 (BYDAY in YEARLY rules).** One sentence says a numeric BYDAY
in a YEARLY rule is an offset within the year "when BYWEEKNO or BYMONTH are
present". A "not" is missing. Erratum 3779 refines the same sentence, so read
the two together. **Impact:** recurrence.

**1916, §3.6.4 (VFREEBUSY example).** In the last VFREEBUSY example, ORGANIZER
is written `jsmith@example.com` with no scheme, so it isn't a CAL-ADDRESS URI.
The correction is `mailto:jsmith@example.com`. **Impact:** none (test data:
`rfc5545-3.6.4-c` is `valid: false`, and `-corrected` is valid).

**2039, §4 (to-do example VALARM).** The alarm has `TRIGGER:19980403T120000Z`.
TRIGGER's default value type is DURATION, so an absolute trigger needs
`TRIGGER;VALUE=DATE-TIME:...`, which the correction adds. **Impact:** parser.
A strict parser rejects the original. A lenient one may infer DATE-TIME from
the value's shape; that is a policy choice. (Test data: `rfc5545-4-d`.)

**2677, §3.8.1.10 (RESOURCES conformance).** The conformance text says
RESOURCES "can be specified once" in VEVENT/VTODO. "Once" was a mistake; the
ABNF and RFC 2445 allow it to repeat. **Impact:** parser (cardinality
validation must allow repeats). Erratum 2676, which proposed the opposite
fix, was rejected.

**3740, §4 (group-meeting example VTIMEZONE).** The example's VTIMEZONE has
only one STANDARD onset (1998-10-25) and one DAYLIGHT onset (1999-04-04).
Both are after the 1998-03-12 meeting, so the zone data doesn't cover the
event. The correction replaces them with RRULE-based observances starting in
1967 and 1987, taken from the §3.6.5 example. **Impact:** timezone (test data:
`rfc5545-4-b` and `-corrected`). The original is still syntactically valid,
which makes it a useful fixture for how a resolver behaves before the first
onset.

**3779, §3.3.10 (BYDAY in YEARLY rules, again).** Even with 1913 applied, the
paragraph contradicts itself about BYWEEKNO. It says the offset is within the
year when BYWEEKNO is present, and later forbids numeric BYDAY with BYWEEKNO.
The resolution: with FREQ=YEARLY, a numeric BYDAY is an offset within the
month when BYMONTH is present, and within the year when BYMONTH is absent.
Numeric BYDAY with BYWEEKNO stays forbidden. **Impact:** recurrence (offset
scope), parser (reject or flag `FREQ=YEARLY;BYWEEKNO=...;BYDAY=1MO`).

**3883, §3.8.5.3 ("every 3 hours from 9 to 5").** The rule has
`UNTIL=19970902T170000Z`, which is 13:00 EDT. That gives only 09:00 and 12:00,
not the stated 09:00, 12:00 and 15:00. The correction is
`UNTIL=19970902T210000Z` (17:00 EDT). **Impact:** recurrence (test data:
`recur.json` case `33` is corrected, case `33-uncorrected` is the printed
rule).

**4149, §4 (published busy-time example).** The VFREEBUSY has no UID or
DTSTAMP, which §3.6.4 makes REQUIRED. The correction adds both. **Impact:**
none for code. Validators should flag the original (test data: `rfc5545-4-f`
is `valid: false`).

**4271, §3.3.10 (nonexistent local times).** §3.3.10 says instances with an
invalid date or a nonexistent local time "MUST be ignored". A later paragraph
of the same section says a nonexistent or repeated local time is interpreted
like an explicit DATE-TIME (§3.3.5). The correction splits the rule. Invalid
dates such as February 30 are still dropped and don't count. An instance at a
nonexistent wall-clock time (in a spring-forward gap) is kept, and resolved by
§3.3.5: use the offset in effect before the gap, so 02:30 becomes 03:30 EDT.
Major clients behave this way. **Impact:** recurrence, timezone. This is the
most significant behavioural erratum.

**6109, §3.6.1 (multi-day anniversaries).** The text says an all-day
("anniversary") VEVENT spans more than one date when DTEND is any date after
DTSTART. Because DATE DTEND is exclusive, DTEND = DTSTART + 1 day is still a
single day. The correction says "at least two days after". **Impact:** none
(descriptive; consistent with exclusive DTEND).

**7691, §3.8.4.2 (CONTACT ALTREP example).** The LDAP URI in a quoted ALTREP
parameter contains `\,`. Backslash escaping is a TEXT value rule and doesn't
apply in parameter values, where a comma inside quotes needs no escaping. So
the URI would contain a literal backslash. The correction uses a plain comma.
**Impact:** parser (don't unescape parameter values), writer (don't escape
commas in quoted parameter values). (Test data: `rfc5545-3.8.4.2-b`.)

**7945, §3.8.8.3 (REQUEST-STATUS ABNF).** The `rstatus` rule leaves out the
final `CRLF` that every other property rule has; the correction appends it:
`... [";" extdata] CRLF`. **Impact:** none in practice, because the
content-line layer ends every line with CRLF anyway.

### Editorial

**2038, Appendix A.1.** The list of new restrictions since RFC 2445 is
missing one: the `charset` Content-Type parameter is now mandatory in MIME
transports. **Impact:** none for this library (MIME/iMIP layer).

**2497, §2.1.** Reorders the character table by code point and repeats the
DQUOTE fix from 1911. **Impact:** none.

**2516, §3.8.4.1 (ATTENDEE SENT-BY example).** The example writes
`SENT-BY=mailto:jan_doe@example.com` without quotes. The ABNF is
`sentbyparam = "SENT-BY" "=" DQUOTE cal-address DQUOTE`, so the quotes are
required. **Impact:** parser (strict mode needs the DQUOTEs; lenient mode may
accept a bare value), writer (always quote SENT-BY, DELEGATED-FROM/TO, MEMBER,
DIR and ALTREP). (Test data: `rfc5545-3.8.4.1-e`.)

**2527, §3.8.5.2 (RDATE example).** A comma is missing at the fold in the
`RDATE;VALUE=DATE:` list, so unfolding produces the bad date `1997042119970526`.
**Impact:** none for code. The original is a good negative parser test
(`rfc5545-3.8.5.2-c`).

**3747, §3.3.10 (BYxxx table, Note 2).** Note 2, which governs BYDAY in YEARLY
rules, used to say: limit if BYYEARDAY or BYMONTHDAY is present; otherwise
special expand "for WEEKLY if BYWEEKNO present", "for MONTHLY if BYMONTH
present", otherwise "for YEARLY". The erratum deletes the WEEKLY and MONTHLY
clauses, on the theory that Note 2 only applies to FREQ=YEARLY. **Impact:**
none, with a caution. The deleted clauses described *how* BYDAY expands inside
a YEARLY rule: within each week chosen by BYWEEKNO, or within each month
chosen by BYMONTH. That behaviour is still required. The §3.8.5.3 examples
"Every Thursday in March" (`BYMONTH=3;BYDAY=TH`) and "Monday of week number
20" (`BYWEEKNO=20;BYDAY=MO`) only produce the RFC's stated results with it.
Implement the original semantics.

**4414, §3.3.10 (UNTIL value form).** One sentence left over from RFC 2445
says a DATE-TIME UNTIL "MUST be specified in UTC time format". That
contradicts the 5545 rule just before it: a floating DTSTART takes a floating
UNTIL, while a UTC or TZID DTSTART takes a UTC UNTIL. The leftover sentence is
removed. **Impact:** parser (accept a floating UNTIL with a floating DTSTART),
recurrence (compare UNTIL in DTSTART's form; with TZID, convert instances to
UTC), writer (emit UNTIL in the matching form; always UTC in STANDARD/DAYLIGHT
subcomponents).

**5602, §3.1.3 (inline binary example).** The BASE64 example value has no `=`
padding (59 characters). Everywhere else RFC 5545 uses padded RFC 4648 base64.
The correction appends `=`. **Impact:** parser (decide on purpose whether
unpadded input is accepted; don't become lenient by accident). (Test data:
`rfc5545-3.1.3-b` is `valid: false`.)

**5794, Table of Contents.** Most page numbers in the ToC are wrong because
the §2.1 table was added late. **Impact:** none.

**7332, §3.3.9 (PERIOD example).** The second example ("start at 18:00:00 on
January 1, 1997 and lasting 5 hours and 30 minutes") is written
`19970101T180000Z/PT5H30M`, but the prose doesn't say UTC. The correction
drops the `Z`. **Impact:** none. Both forms are valid PERIOD values.

## Held for Document Update (4)

These are accepted as worth fixing in a future revision, but aren't official
corrections.

- **3038, §3.8.7.2 (DTSTAMP).** Reword the value type to say DTSTAMP is
  always "DATE WITH UTC TIME", as CREATED and COMPLETED already say. Impact:
  parser and writer (DTSTAMP is always UTC; this was already the rule).
- **5505, §3.2.19 (TZID parameter).** `tzidparam` uses `paramtext`, so a TZID
  can't be quoted. Real TZIDs such as `GMT-03:00` contain `:` and need
  quoting. Proposed: `tzidparam = "TZID" "=" [tzidprefix] param-value`.
  Impact: parser (accept quoted TZIDs, as existing implementations do),
  writer (quote a TZID that contains `:`, `;` or `,`).
- **2495, §3.2.7 (ENCODING).** The description cites RFC 2045 for BASE64; it
  should cite RFC 4648. Impact: none.
- **4626, Appendix A.1.** The list of new restrictions leaves out that DUE
  must be strictly later than DTSTART. Impact: parser (validation).

## Reported, not yet verified (2)

- **6316, §3.8.5.1 (EXDATE).** Proposes that EXDATE must have the same value
  type as DTSTART, and be floating exactly when DTSTART is, like
  RECURRENCE-ID. It notes that real data mixes `DTSTART` DATE-TIME with
  `EXDATE;VALUE=DATE`. Impact: recurrence and parser policy. See the
  architecture doc's open question.
- **8398, §3.6.5 (tzprop ABNF).** The EXDATE text allows EXDATE in
  STANDARD/DAYLIGHT, but `tzprop` doesn't list `exdate`. Proposes adding it.
  The verifier of rejected erratum 5215 had already said this was the real
  error. Impact: parser (accept EXDATE in observances), timezone (apply it
  when expanding onsets).

## Rejected (11)

Listed so nobody "fixes" these again. Three are about §3.8.5.3 examples,
which therefore stay exactly as printed in `recur.json`.

- **5872** (§3.8.5.3, Tue/Thu for five weeks). It proposed moving UNTIL to
  `19971002T000000Z`. Rejected: `19971007T000000Z` is before 09:00 EDT on
  Oct 7, so the original is right, and the change would drop Oct 2.
- **5920** (§3.8.5.3, Friday the 13th). It proposed moving DTSTART to a
  matching date. Rejected because the example is meant to show EXDATE
  removing a DTSTART that doesn't match the rule.
- **6212** (§3.8.5.3, "until December 24"). It proposed `UNTIL=...T140000Z`
  so that Dec 24 is included. Rejected: UNTIL doesn't have to fall on an
  instance, and the examples are correct as printed.
- **2676** (RESOURCES cardinality, the opposite of 2677). **3405**
  (weekday order in the ABNF; ABNF implies no order). **3457** (quoting
  parameter values that contain spaces; not required, since SAFE-CHAR
  includes WSP). **4150** (TZDB reference URL out of date; the reference was
  correct at publication). **4606** (REQUEST-STATUS ABNF order; the original
  is right). **5082** (forbid a future LAST-MODIFIED; out of scope).
  **5214** (VTIMEZONE ABNF lets properties and subcomponents interleave; no
  interop problem, so accept any order). **5215** (remove EXDATE from
  VTIMEZONE; the verifier said `tzprop` should gain EXDATE instead, see
  8398).
