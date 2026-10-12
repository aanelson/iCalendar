# RFC texts

This directory has the specifications the library implements, as plain
text from `https://www.rfc-editor.org/rfc/rfcNNNN.txt`. They were fetched on
2026-10-11 and are byte-for-byte unmodified. RFCs 9073, 9074, 9253 and 9636
start with a UTF-8 byte order mark because the RFC Editor publishes them that
way.

RFC 5545 is the primary specification; the others extend or reference it.
The "Maps to" column uses the package names from
[`docs/architecture.md`](../architecture.md). Names marked *planned* don't
exist yet.

| RFC | Title | Status | What it adds or changes relative to RFC 5545 | Maps to |
|---|---|---|---|---|
| [5545](rfc5545.txt) | Internet Calendaring and Scheduling Core Object Specification (iCalendar) | Proposed Standard (obsoletes 2445) | The core spec: content lines, parameters, value types, components, properties, RRULE, VTIMEZONE. Updated by 5546, 6868, 7529, 7953, 7986, 9073, 9074 and 9253. Errata are in [rfc5545-errata.md](rfc5545-errata.md). | `contentline` (§3.1), `civil` and value codecs (§3.3), `recur` (§3.3.10, §3.8.5), `tz` (§3.6.5), `ical` (everything else) |
| [5234](rfc5234.txt) | Augmented BNF for Syntax Specifications: ABNF | Internet Standard (STD 68) | Defines the ABNF notation and core rules (ALPHA, DIGIT, WSP, DQUOTE, CRLF) used by every grammar here. Adds nothing to iCalendar. RFC 7405 later added case-sensitive strings, which iCalendar doesn't use. | Reference only: how to read the grammars in `contentline` and the value codecs |
| [6868](rfc6868.txt) | Parameter Value Encoding in iCalendar and vCard | Proposed Standard | Adds the `^` escape in parameter values: `^n` (newline), `^^` (caret) and `^'` (double quote). RFC 5545 had no way to put those characters in a parameter. | `contentline` (on by default, per the architecture doc) |
| [5546](rfc5546.txt) | iCalendar Transport-Independent Interoperability Protocol (iTIP) | Proposed Standard (obsoletes 2446) | Scheduling semantics: the METHOD values (PUBLISH, REQUEST, REPLY, ADD, CANCEL, REFRESH, COUNTER, DECLINECOUNTER), the properties allowed or required per method and component, and REQUEST-STATUS codes. Formally updates 5545. | `itip` (*planned*): validators and helpers over the `ical` model |
| [6047](rfc6047.txt) | iCalendar Message-Based Interoperability Protocol (iMIP) | Proposed Standard (obsoletes 2447) | Carries iTIP over email: `text/calendar` MIME parts with `method` and `charset` parameters, plus security considerations. No change to the data format. | `imip` (*planned*), or helpers next to `itip`. Out of scope for the core |
| [7529](rfc7529.txt) | Non-Gregorian Recurrence Rules in iCalendar | Proposed Standard | Adds RRULE parts `RSCALE` (calendar system, e.g. Hebrew or Chinese) and `SKIP` (OMIT/BACKWARD/FORWARD for invalid dates), and a leap-month suffix `L` for BYMONTH (e.g. `5L`). Also updates xCal and jCal. | `recur` (`Rule.RScale`/`Skip`; a calendar-system interface with Gregorian built in) |
| [7986](rfc7986.txt) | New Properties for iCalendar | Proposed Standard | Calendar-level NAME, DESCRIPTION, UID, URL, LAST-MODIFIED, CATEGORIES, REFRESH-INTERVAL, SOURCE and COLOR. New IMAGE and CONFERENCE properties. New DISPLAY, EMAIL, FEATURE and LABEL parameters. | `rfc7986` schema package (*planned*), used as `ical.RFC5545.With(rfc7986.Schema)` |
| [9073](rfc9073.txt) | Event Publishing Extensions to iCalendar | Proposed Standard | New PARTICIPANT, VLOCATION and VRESOURCE components. New STYLED-DESCRIPTION, STRUCTURED-DATA, LOCATION-TYPE, PARTICIPANT-TYPE, RESOURCE-TYPE and CALENDAR-ADDRESS properties. New ORDER, SCHEMA and DERIVED parameters. | `rfc9073` schema and typed views (*planned*) |
| [9074](rfc9074.txt) | "VALARM" Extensions for iCalendar | Proposed Standard | Lets VALARM carry UID and RELATED-TO. Adds the ACKNOWLEDGED property, snoozing via `RELTYPE=SNOOZE`, and location-based PROXIMITY triggers. | `rfc9074` schema (*planned*); alarm helpers in `ical` |
| [9253](rfc9253.txt) | Support for iCalendar Relationships | Proposed Standard | Extends RELATED-TO: URI and UID values, and new RELTYPE values (temporal FINISHTOSTART and others, FIRST, NEXT, DEPENDS-ON, REFID, CONCEPT). Adds LINK, CONCEPT and REFID properties, LINKREL and GAP parameters, and the XML-REFERENCE and UID value types. | `rfc9253` schema (*planned*) |
| [7953](rfc7953.txt) | Calendar Availability | Proposed Standard | New VAVAILABILITY component and AVAILABLE subcomponent, and the BUSYTYPE property, for publishing when someone is available. Defines how they combine with VFREEBUSY. Also updates CalDAV (4791, 6638). | `rfc7953` schema (*planned*); free/busy computation later |
| [7265](rfc7265.txt) | jCal: The JSON Format for iCalendar | Proposed Standard | A lossless JSON mapping of the iCalendar data model. No change to the data model itself. | `ical/jcal` codec (*planned*) over the same `Component` tree |
| [6321](rfc6321.txt) | xCal: The XML Format for iCalendar | Proposed Standard | A lossless XML mapping of the iCalendar data model. Updated by 6868 and 7529. | `ical/xcal` codec (*planned*) |
| [8536](rfc8536.txt) | The Time Zone Information Format (TZif) | Proposed Standard, **obsoleted by RFC 9636** | The binary `TZif` zone format (versions 1 to 3) used by tzdata and `/usr/share/zoneinfo`. Not part of iCalendar. It's relevant because VTIMEZONE rules are compiled to, or checked against, TZif. | `tz` (VTIMEZONE to TZif / `*time.Location`) |
| [9636](rfc9636.txt) | The Time Zone Information Format (TZif) | Proposed Standard (obsoletes 8536) | **Extra; not in the original request.** The current TZif spec: it adds version 4 (leap-second table truncation and expiration) and clarifies earlier versions. Fetched because 8536 is obsolete; implement against this one. | `tz` |

## Related files

- [`rfc5545-errata.md`](rfc5545-errata.md): RFC 5545 errata, summarized,
  with the implementation impact of each.
- [`../../testdata/rfc5545/recur.json`](../../testdata/rfc5545/recur.json):
  every §3.8.5.3 recurrence example as a fully listed instance list.
- [`../../testdata/rfc5545/examples.json`](../../testdata/rfc5545/examples.json)
  and [`../../testdata/rfc5545/ics/`](../../testdata/rfc5545/ics/): every
  example block of iCalendar text in RFC 5545. Where a Verified erratum
  corrects an example, there's an extra `-corrected` entry.

## Licence

The RFC texts are reproduced under the IETF Trust Legal Provisions (TLP,
<https://trustee.ietf.org/license-info>). The TLP lets IETF Contributions,
including published RFCs, be copied and distributed in full, without
modification. They are kept here exactly as published, including the
copyright notice and boilerplate in each document. Code Components in them
(such as ABNF and examples) are also available under the Revised BSD
License, as the TLP describes. The test fixtures derived from RFC 5545
(`testdata/rfc5545/`) are excerpts of Code Components, credited with
`"license": "IETF Trust Legal Provisions"`.
