"""Collect the RFC 5545 example blocks into examples.json and .ics files.

Every block is identified by its original line numbers in docs/rfc/rfc5545.txt.
Page headers/footers are dropped, the block's own leading indentation (the
indentation of its first line) is removed from every line, and folded
continuation lines keep their extra leading whitespace.

Usage: python3 -I gen_examples.py <rfc5545.txt> <examples.json> <ics_dir>
"""
import json
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from rfcclean import clean, PAGEBREAK  # noqa: E402

RFC, OUT_JSON, ICS_DIR = sys.argv[1], sys.argv[2], sys.argv[3]
CLEAN = clean(RFC)
BY_LINE = {ln: t for ln, t in CLEAN if t != PAGEBREAK}
CRLF = "\r\n"


def grab(ranges, strip_annotation=False):
    if isinstance(ranges, int):
        ranges = [(ranges, ranges)]
    elif isinstance(ranges, tuple):
        ranges = [ranges]
    out = []
    for r in ranges:
        if isinstance(r, int):
            r = (r, r)
        a, b = r
        for ln in range(a, b + 1):
            if ln not in BY_LINE:
                continue  # page header/footer/padding line
            t = BY_LINE[ln]
            if t.strip() == "":
                raise SystemExit("blank line %d inside block %r" % (ln, ranges))
            out.append(t)
    base = len(out[0]) - len(out[0].lstrip(" "))
    res = []
    for t in out:
        ind = len(t) - len(t.lstrip(" "))
        if ind < base:
            raise SystemExit("line less indented than block start: %r" % t)
        t = t[base:]
        if strip_annotation:
            t = re.sub(r"\s{2,};.*$", "", t)
        res.append(t.rstrip())
    return CRLF.join(res)


def crosses_page(src):
    lns = []
    for r in (src if isinstance(src, list) else [src]):
        if isinstance(r, int):
            r = (r, r)
        lns += [r[0], r[1]]
    lo, hi = min(lns), max(lns)
    return any(t == PAGEBREAK and lo <= ln <= hi for ln, t in CLEAN)


entries = []
counters = {}


def letter(n):
    s = ""
    n += 1
    while n:
        n, r = divmod(n - 1, 26)
        s = chr(97 + r) + s
    return s


def ex(section, title, kind, src, valid=True, errata="", comment="", text=None,
       strip_annotation=False, corrected=None, ics=False, wrap=False):
    i = counters.get(section, 0)
    counters[section] = i + 1
    eid = "rfc5545-%s-%s" % (section, letter(i))
    if text is None:
        text = grab(src, strip_annotation)
        if crosses_page(src):
            comment = (comment + " " if comment else "") + \
                "Crosses a page break in the RFC (page footer/header removed)."
    e = {"id": eid, "section": section, "title": title, "kind": kind, "text": text,
         "valid": valid, "errata": errata, "comment": comment}
    entries.append(e)
    if ics:
        write_ics(eid, text, wrap)
    if corrected:
        new = text
        for old, rep in corrected["replace"]:
            if new.count(old) != 1:
                raise SystemExit("correction anchor not unique in %s: %r" % (eid, old))
            new = new.replace(old, rep)
        c = {"id": eid + "-corrected", "section": section, "title": title, "kind": kind,
             "text": new, "valid": True, "errata": errata,
             "comment": ("Text of %s with Verified erratum %s applied. %s" % (
                 eid, errata, corrected.get("comment", ""))).strip()}
        entries.append(c)
        if ics:
            write_ics(eid + "-corrected", new, wrap)
    return eid


def write_ics(eid, text, wrap):
    lines = text.split(CRLF)
    if wrap:
        lines = ["BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//rfc5545 examples//EN"] + \
            lines + ["END:VCALENDAR"]
    with open(os.path.join(ICS_DIR, eid + ".ics"), "w", encoding="ascii", newline="") as f:
        f.write(CRLF.join(lines) + CRLF)


R5 = "See testdata/rfc5545/recur.json case "

# ---- 3.1 Content Lines ---------------------------------------------------
ex("3.1", "Content line folding (unfolded form)", "property", 489,
   comment="Unfolded form of the line. It is equivalent to rfc5545-3.1-b.")
ex("3.1", "Content line folding (folded form)", "property", (493, 495),
   comment="Folded form. Unfolding (remove CRLF plus one whitespace character) gives "
           "exactly rfc5545-3.1-a. The second continuation line starts with two spaces: "
           "the fold character plus a real space.")
ex("3.1.1", "List and field separators (ATTENDEE)", "property", (631, 632))
ex("3.1.1", "List and field separators (RDATE value list)", "property", 634)
ex("3.1.3", "Binary content (ATTACH by URI)", "property", 668)
ex("3.1.3", "Binary content (inline BASE64 ATTACH)", "property", (682, 683), valid=False,
   errata="5602",
   comment="The BASE64 value has no '=' padding (59 characters, which is not a multiple of "
           "4). That is invalid under the 'binary' ABNF in 3.3.1. Erratum 5602 adds the "
           "padding. The value decodes to 'The quick brown fox jumps over the lazy dog.'",
   corrected={"replace": [("IGRvZy4", "IGRvZy4=")]})

# ---- 3.2 Property Parameters ----------------------------------------------
ex("3.2", "Quoted parameter value (ALTREP)", "parameter", (709, 710))
ex("3.2.1", "ALTREP", "parameter", (801, 804),
   comment="The RFC follows this with the referenced MIME body part (Content-Type/"
           "Content-Id headers and an HTML document). That part is not iCalendar text and "
           "is not included.")
ex("3.2.2", "CN", "parameter", 858)
ex("3.2.3", "CUTYPE", "parameter", 890)
ex("3.2.4", "DELEGATED-FROM", "parameter", (925, 926))
ex("3.2.5", "DELEGATED-TO", "parameter", (961, 962),
   comment="The fold falls inside a quoted parameter value ('jqpublic' / '@example.com').")
ex("3.2.6", "DIR", "parameter", (990, 991))
ex("3.2.7", "ENCODING", "parameter", (1034, 1044))
ex("3.2.8", "FMTTYPE", "parameter", (1077, 1078))
ex("3.2.9", "FBTYPE", "parameter", 1112)
ex("3.2.10", "LANGUAGE", "parameter", 1154)
ex("3.2.10", "LANGUAGE", "parameter", 1156)
ex("3.2.10", "LANGUAGE", "parameter", 1158)
ex("3.2.11", "MEMBER", "parameter", (1193, 1194))
ex("3.2.11", "MEMBER", "parameter", (1196, 1197))
ex("3.2.12", "PARTSTAT", "parameter", 1275)
ex("3.2.13", "RANGE", "parameter", 1313)
ex("3.2.14", "RELATED", "parameter", 1340)
ex("3.2.15", "RELTYPE", "parameter", (1385, 1386))
ex("3.2.16", "ROLE", "parameter", 1431)
ex("3.2.17", "RSVP", "parameter", 1465)
ex("3.2.18", "SENT-BY", "parameter", (1488, 1489))
ex("3.2.19", "TZID", "parameter", 1546)
ex("3.2.19", "TZID", "parameter", 1548)

# ---- 3.3 Value types ----------------------------------------------------
ex("3.3.1", "BINARY", "property", (1690, 1698),
   comment="ATTACH with an inline BASE64 icon. The fold falls between 'VALUE' and "
           "'=BINARY'.")
ex("3.3.2", "BOOLEAN", "value", 1719)
ex("3.3.3", "CAL-ADDRESS", "value", 1750)
ex("3.3.4", "DATE", "value", 1783)
ex("3.3.5", "DATE-TIME (UTC offset form)", "value", 1822, valid=False, strip_annotation=True,
   comment="The RFC presents this as invalid: a DATE-TIME with a numeric UTC offset is not "
           "allowed. The RFC annotates it with ';Invalid time format', which is removed "
           "here.")
ex("3.3.5", "DATE-TIME (form #1, floating)", "value", 1831)
ex("3.3.5", "DATE-TIME (form #2, UTC)", "value", 1874)
ex("3.3.5", "DATE-TIME (form #3, TZID reference)", "value", 1887,
   comment="A fragment, as the RFC prints it: a TZID parameter and a value. It is not a "
           "complete content line.")
ex("3.3.5", "DATE-TIME (leap second)", "value", 1914,
   comment="Second 60 is allowed only for a positive leap second. Implementations that do "
           "not support leap seconds SHOULD treat it as second 59.")
ex("3.3.5", "DATE-TIME (DTSTART floating)", "property", 1923, strip_annotation=True,
   comment="The RFC annotation '; Local time' is removed. Same instant as -g and -h for a "
           "New York user.")
ex("3.3.5", "DATE-TIME (DTSTART UTC)", "property", 1924, strip_annotation=True,
   comment="The RFC annotation '; UTC time' is removed.")
ex("3.3.5", "DATE-TIME (DTSTART with TZID)", "property", 1925,
   comment="The RFC annotation on the next two lines ('; Local time and time zone "
           "reference') is not included.")
ex("3.3.6", "DURATION", "value", 1986)
ex("3.3.6", "DURATION", "value", 1990)
for ln in (2012, 2013, 2014):
    ex("3.3.7", "FLOAT", "value", ln,
       comment="The RFC lists three FLOAT values in one block; each is a separate entry.")
for ln in (2045, 2046, 2047, 2048):
    ex("3.3.8", "INTEGER", "value", ln,
       comment="The RFC lists four INTEGER values in one block; each is a separate entry.")
ex("3.3.9", "PERIOD (explicit)", "value", 2096)
ex("3.3.9", "PERIOD (start and duration)", "value", 2101, errata="7332",
   comment="Valid as printed, but the prose does not say UTC. Erratum 7332 removes the "
           "'Z' so the value matches the description.",
   corrected={"replace": [("19970101T180000Z/", "19970101T180000/")]})
ex("3.3.10", "RECUR (last work day of the month)", "value", 2375)
ex("3.3.10", "RECUR (multiple BYxxx rule parts)", "property", (2473, 2475),
   comment="Every Sunday in January at 08:30 and 09:30, every other year. The RFC gives no "
           "instance list for this one.")
ex("3.3.10", "RECUR", "value", 2504,
   comment="10 occurrences, every other day.")
ex("3.3.11", "TEXT", "value", 2571,
   comment="Escaped form of the three-line value 'Project XYZ Final Review' / 'Conference "
           "Room - 3B' / 'Come Prepared.' (RFC lines 2565-2567, which show the decoded "
           "text, not iCalendar text).")
ex("3.3.12", "TIME (UTC offset form)", "value", 2622, valid=False, strip_annotation=True,
   comment="The RFC presents this as invalid: a TIME with a numeric UTC offset is not "
           "allowed. The RFC annotation ';Invalid time format' is removed.")
ex("3.3.12", "TIME (form #1, floating)", "value", 2630)
ex("3.3.12", "TIME (form #2, UTC)", "value", 2668)
ex("3.3.12", "TIME (8:30 AM New York, floating)", "value", 2683,
   comment="The RFC lists three TIME values in one block; each is a separate entry.")
ex("3.3.12", "TIME (8:30 AM New York, UTC)", "value", 2684)
ex("3.3.12", "TIME (8:30 AM New York, TZID reference)", "value", 2685,
   comment="A fragment: a TZID parameter and a value, not a complete content line.")
ex("3.3.13", "URI", "value", 2723)
ex("3.3.14", "UTC-OFFSET", "value", 2761)
ex("3.3.14", "UTC-OFFSET", "value", 2763)

# ---- 3.4 / 3.5 -----------------------------------------------------------
ex("3.4", "iCalendar object", "calendar", (2783, 2793), ics=True)
ex("3.5", "Property", "property", 2815)
ex("3.5", "Property (case-insensitive names)", "property", None,
   text="DTSTART;TZID=America/New_York:19980714T120000",
   comment="Inline in the prose on RFC lines 2822-2823, not a separate block. "
           "Equivalent to rfc5545-3.5-c.")
ex("3.5", "Property (case-insensitive names)", "property", None,
   text="DtStart;TzID=America/New_York:19980714T120000",
   comment="Inline in the prose on RFC line 2823. Property and parameter names are "
           "case-insensitive, so this is equivalent to rfc5545-3.5-b.")

# ---- 3.6 Components ------------------------------------------------------
ex("3.6.1", "VEVENT (opaque meeting)", "component", (3009, 3017))
ex("3.6.1", "VEVENT (transparent reminder)", "component", (3031, 3040),
   )
ex("3.6.1", "VEVENT (annual anniversary)", "component", (3045, 3054))
ex("3.6.1", "VEVENT (multi-day all-day event)", "component", (3062, 3069),
   comment="DTEND is exclusive: the event covers 2007-06-28 through 2007-07-08.")
ex("3.6.2", "VTODO (due date)", "component", (3159, 3167))
ex("3.6.2", "VTODO (completed)", "component", (3173, 3182),
   comment="Has COMPLETED but STATUS:NEEDS-ACTION. That is unusual, but the RFC does not "
           "forbid it.")
ex("3.6.3", "VJOURNAL", "component", (3268, 3282))
ex("3.6.4", "VFREEBUSY (request)", "component", (3395, 3402))
ex("3.6.4", "VFREEBUSY (reply)", "component", (3426, 3436))
ex("3.6.4", "VFREEBUSY (published)", "component", (3441, 3451), valid=False, errata="1916",
   comment="ORGANIZER has no 'mailto:' scheme, so it is not a valid CAL-ADDRESS URI. "
           "Erratum 1916 adds the scheme.",
   corrected={"replace": [("ORGANIZER:jsmith@example.com", "ORGANIZER:mailto:jsmith@example.com")]})
ex("3.6.5", "STANDARD onset (DTSTART)", "property", 3687,
   comment="Shown with -b: the 1967 New York onset of Standard Time.")
ex("3.6.5", "STANDARD onset (TZOFFSETFROM)", "property", 3689)
ex("3.6.5", "VTIMEZONE (America/New_York, full history since 1967)", "component",
   (3740, 3800), ics=True, wrap=True, )
ex("3.6.5", "VTIMEZONE (America/New_York, DTSTART only, 2007 rules)", "component",
   (3820, 3835), ics=True, wrap=True,
   comment="Only valid for recurring events from 2007-03-11 03:00 EDT through 2008-03-09 "
           "01:59:59 EST.")
ex("3.6.5", "VTIMEZONE (America/New_York, current RRULE rules, TZURL)", "component",
   (3843, 3861), ics=True, wrap=True)
ex("3.6.5", "VTIMEZONE (Fictitious, daylight rule with end date)", "component",
   (3875, 3892), ics=True, wrap=True,
   comment="Possible RFC defect (no erratum filed). Section 3.6.5 says UNTIL must equal "
           "the last onset. Here the DAYLIGHT UNTIL 19980404T070000Z is a Saturday, not a "
           "generated onset: the 1998 first-Sunday-of-April onset would be 1998-04-05 "
           "07:00Z, which is after UNTIL. So the last DAYLIGHT onset is 1997-04-06. Kept "
           "valid=true because the syntax is valid.")
ex("3.6.5", "VTIMEZONE (Fictitious, two daylight rules)", "component", (3932, 3956),
   ics=True, wrap=True,
   comment="Possible RFC defects (no erratum filed). (1) The first DAYLIGHT has the same "
           "unsynchronized UNTIL 19980404T070000Z (a Saturday) as rfc5545-3.6.5-f. (2) The "
           "second DAYLIGHT's DTSTART 19990424T020000 is a Saturday, but its RRULE is "
           "BYDAY=-1SU;BYMONTH=4, whose 1999 instance is 1999-04-25. DTSTART is not "
           "synchronized with the RRULE, so the first onset is unclear. Kept valid=true "
           "because the syntax is valid.")
ex("3.6.6", "VALARM (audio, absolute trigger)", "component", (4180, 4187))
ex("3.6.6", "VALARM (display, relative trigger)", "component", (4207, 4214),
   )
ex("3.6.6", "VALARM (email)", "component", (4222, 4232))

# ---- 3.7 Calendar properties -----------------------------------------------
ex("3.7.1", "CALSCALE", "property", 4280)
ex("3.7.2", "METHOD", "property", 4334)
ex("3.7.3", "PRODID", "property", 4382)
ex("3.7.4", "VERSION", "property", 4433)

# ---- 3.8 Component properties ----------------------------------------------
ex("3.8.1.1", "ATTACH", "property", 4515)
ex("3.8.1.1", "ATTACH", "property", (4517, 4518))
ex("3.8.1.2", "CATEGORIES", "property", 4572)
ex("3.8.1.2", "CATEGORIES", "property", 4574)
ex("3.8.1.3", "CLASS", "property", 4631)
ex("3.8.1.4", "COMMENT", "property", (4683, 4686))
ex("3.8.1.5", "DESCRIPTION", "property", (4748, 4750))
ex("3.8.1.6", "GEO", "property", 4843)
ex("3.8.1.7", "LOCATION", "property", 4900)
ex("3.8.1.7", "LOCATION", "property", (4902, 4903))
ex("3.8.1.8", "PERCENT-COMPLETE", "property", 4954)
ex("3.8.1.9", "PRIORITY", "property", 5021)
ex("3.8.1.9", "PRIORITY", "property", 5026)
ex("3.8.1.9", "PRIORITY", "property", 5031,
   comment="0 means undefined priority. It is the same as omitting the property.")
ex("3.8.1.10", "RESOURCES", "property", 5087)
ex("3.8.1.10", "RESOURCES", "property", 5089)
ex("3.8.1.11", "STATUS", "property", 5167)
ex("3.8.1.11", "STATUS", "property", 5172)
ex("3.8.1.11", "STATUS", "property", 5177)
ex("3.8.1.12", "SUMMARY", "property", 5233)
ex("3.8.2.1", "COMPLETED", "property", 5277)
ex("3.8.2.2", "DTEND", "property", 5349)
ex("3.8.2.2", "DTEND", "property", 5351)
ex("3.8.2.3", "DUE", "property", 5413)
ex("3.8.2.4", "DTSTART", "property", 5484)
ex("3.8.2.5", "DURATION", "property", 5531)
ex("3.8.2.5", "DURATION", "property", 5536)
ex("3.8.2.6", "FREEBUSY", "property", 5612)
ex("3.8.2.6", "FREEBUSY", "property", 5614)
ex("3.8.2.6", "FREEBUSY", "property", (5616, 5617),
   comment="The continuation line starts with the list separator ','.")
ex("3.8.2.7", "TRANSP", "property", 5666)
ex("3.8.2.7", "TRANSP", "property", 5671)
ex("3.8.3.1", "TZID", "property", 5732)
ex("3.8.3.1", "TZID", "property", 5734)
ex("3.8.3.1", "TZID", "property", 5739,
   comment="A globally unique TZID (prefixed with '/').")
ex("3.8.3.2", "TZNAME", "property", 5793)
ex("3.8.3.2", "TZNAME", "property", 5795)
ex("3.8.3.3", "TZOFFSETFROM", "property", 5841)
ex("3.8.3.3", "TZOFFSETFROM", "property", 5843)
ex("3.8.3.4", "TZOFFSETTO", "property", 5889)
ex("3.8.3.4", "TZOFFSETTO", "property", 5891)
ex("3.8.3.5", "TZURL", "property", 5926)
ex("3.8.4.1", "ATTENDEE (to-do)", "property", (6035, 6038))
ex("3.8.4.1", "ATTENDEE (multiple attendees)", "property", (6058, 6062),
   comment="Unfolding removes the line break and the single fold space, so the first CN "
           "becomes 'HenryCabot'. The second line's fold falls inside a quoted parameter "
           "value.")
ex("3.8.4.1", "ATTENDEE (DIR)", "property", (6067, 6069),
   comment="The fold splits the percent-encoding '%20'.")
ex("3.8.4.1", "ATTENDEE (delegation)", "property", (6074, 6081),
   comment="The last line's continuation starts with ':'.")
ex("3.8.4.1", "ATTENDEE (SENT-BY)", "property", (6086, 6087), valid=False, errata="2516",
   comment="The SENT-BY value is not quoted. The sentbyparam ABNF requires DQUOTEs. "
           "Erratum 2516 adds them.",
   corrected={"replace": [("SENT-BY=mailto:jan_doe@example.com;",
                           'SENT-BY="mailto:jan_doe@example.com";')]})
ex("3.8.4.2", "CONTACT", "property", 6142)
ex("3.8.4.2", "CONTACT (ALTREP LDAP)", "property", (6148, 6150), valid=False, errata="7691",
   comment="The quoted ALTREP value contains '\\,'. Backslash escapes do not apply inside "
           "parameter values, so the URI contains a literal backslash and is not a valid "
           "URI. Erratum 7691 removes the backslash.",
   corrected={"replace": [("ABC%20Industries\\,\r\n c=US", "ABC%20Industries,\r\n c=US")]})
ex("3.8.4.2", "CONTACT (ALTREP CID)", "property", (6157, 6158))
ex("3.8.4.2", "CONTACT (ALTREP http)", "property", (6171, 6172),
   )
ex("3.8.4.3", "ORGANIZER", "property", 6246)
ex("3.8.4.3", "ORGANIZER (DIR)", "property", (6251, 6252))
ex("3.8.4.3", "ORGANIZER (SENT-BY)", "property", (6259, 6260))
ex("3.8.4.4", "RECURRENCE-ID", "property", 6375)
ex("3.8.4.4", "RECURRENCE-ID", "property", 6377)
ex("3.8.4.5", "RELATED-TO", "property", 6468)
ex("3.8.4.5", "RELATED-TO", "property", 6470)
ex("3.8.4.6", "URL", "property", 6512)
ex("3.8.4.7", "UID", "property", 6586)
ex("3.8.5.1", "EXDATE", "property", 6694)
ex("3.8.5.2", "RDATE", "property", (6785, 6786))
ex("3.8.5.2", "RDATE (PERIOD)", "property", (6788, 6789))
ex("3.8.5.2", "RDATE (DATE list)", "property", (6791, 6792), valid=False, errata="2527",
   comment="A comma is missing at the fold, so unfolding produces the bad DATE "
           "'1997042119970526'. Erratum 2527 adds the comma.",
   corrected={"replace": [("19970421\r\n 19970526", "19970421,\r\n 19970526")]})

# 3.8.5.3 RRULE examples (instances: see recur.json)
RR = [
    ("01", (6874, 6875), ""),
    ("02", (6881, 6882), ""),
    ("03", (6897, 6898), ""),
    ("04", (6908, 6909), ""),
    ("05a", [6916, (6918, 6919)], "The RFC has a blank line between DTSTART and RRULE; it "
                                  "is dropped here. The RFC gives an alternative RRULE "
                                  "('or'); see the next entry."),
    ("05b", [6916, 6921], "The DTSTART line (RFC line 6916) is combined with the "
                          "alternative RRULE (RFC line 6921)."),
    ("06", (6929, 6930), ""),
    ("07", (6953, 6954), ""),
    ("08", (6964, 6965), ""),
    ("09a", (6978, 6979), "The RFC gives an alternative RRULE ('or'); see the next entry."),
    ("09b", [6978, 6983], "The DTSTART line (RFC line 6978) is combined with the "
                          "alternative RRULE (RFC line 6983)."),
    ("10", (6991, 6993), ""),
    ("11", (7011, 7012), ""),
    ("12", (7019, 7020), ""),
    ("13", (7029, 7030), ""),
    ("14", (7038, 7039), ""),
    ("15", (7048, 7049), ""),
    ("16", (7065, 7066), ""),
    ("17", (7075, 7076), ""),
    ("18", (7084, 7085), ""),
    ("19", (7094, 7096), ""),
    ("20", (7103, 7104), ""),
    ("21", (7121, 7122), ""),
    ("22", (7136, 7137), ""),
    ("23", (7147, 7148), ""),
    ("24", (7160, 7161), ""),
    ("25", (7178, 7179), ""),
    ("26", (7188, 7189), ""),
    ("27", (7198, 7199), ""),
    ("28", (7211, 7213), "DTSTART does not match the rule and is removed by EXDATE. The "
                         "RFC calls an unsynchronized DTSTART undefined, but the text is "
                         "syntactically valid. Erratum 5920 (rejected) objected to this."),
    ("29", (7234, 7235), ""),
    ("30", (7246, 7248), ""),
    ("31", (7258, 7259), ""),
    ("32", (7266, 7267), ""),
    ("33", (7289, 7290), "Valid syntax, but UNTIL=19970902T170000Z (13:00 EDT) does not "
                         "match '9:00 AM to 5:00 PM' or the stated output. Erratum 3883 "
                         "changes it to 19970902T210000Z."),
    ("34", (7296, 7297), ""),
    ("35", (7303, 7304), ""),
    ("36a", (7310, 7311), "The RFC gives an alternative RRULE ('or'); see the next entry."),
    ("36b", [7310, 7313], "The DTSTART line (RFC line 7310) is combined with the "
                          "alternative RRULE (RFC line 7313)."),
    ("37", (7324, 7325), ""),
    ("38", (7331, 7332), ""),
    ("39", (7345, 7346), ""),
]
for cid, src, note in RR:
    kw = {}
    if cid == "33":
        kw = dict(errata="3883", corrected={
            "replace": [("UNTIL=19970902T170000Z", "UNTIL=19970902T210000Z")],
            "comment": R5 + "rfc5545-3.8.5.3-33."})
        ref = R5 + "rfc5545-3.8.5.3-33-uncorrected (as printed)."
    else:
        ref = R5 + "rfc5545-3.8.5.3-" + cid + "."
    ex("3.8.5.3", "RRULE", "property", src, comment=(note + " " + ref).strip(), **kw)

ex("3.8.6.1", "ACTION", "property", 7399)
ex("3.8.6.1", "ACTION", "property", 7401)
ex("3.8.6.2", "REPEAT", "property", (7435, 7436),
   comment="REPEAT and DURATION shown together.")
ex("3.8.6.3", "TRIGGER", "property", 7552)
ex("3.8.6.3", "TRIGGER", "property", 7557)
ex("3.8.6.3", "TRIGGER", "property", 7569)
ex("3.8.7.1", "CREATED", "property", 7609)
ex("3.8.7.2", "DTSTAMP", "property", 7681)
ex("3.8.7.3", "LAST-MODIFIED", "property", 7714)
ex("3.8.7.4", "SEQUENCE", "property", 7771)
ex("3.8.7.4", "SEQUENCE", "property", 7777)
ex("3.8.8.1", "IANA property", "property", 7814,
   comment="A hypothetical IANA-registered property.")
ex("3.8.8.1", "IANA property", "property", 7816)
ex("3.8.8.2", "X- property", "property", (7870, 7871))
ex("3.8.8.3", "REQUEST-STATUS", "property", 7995)
ex("3.8.8.3", "REQUEST-STATUS", "property", 7997,
   comment="extdata contains an unescaped ':' (allowed in TEXT).")
ex("3.8.8.3", "REQUEST-STATUS", "property", (7999, 8000),
   comment="statdesc has a leading space and an escaped comma. extdata has an escaped "
           "semicolon.")
ex("3.8.8.3", "REQUEST-STATUS", "property", 8002)
ex("3.8.8.3", "REQUEST-STATUS", "property", (8004, 8005))

# ---- 4 iCalendar Object Examples ------------------------------------------------
ex("4", "Three-day conference (VEVENT)", "calendar", (8024, 8040), ics=True)
ex("4", "Group-scheduled meeting with VTIMEZONE", "calendar", (8048, 8089), ics=True,
   errata="3740",
   comment="Valid syntax. The VTIMEZONE observances start on 1998-10-25 and 1999-04-04, "
           "after the 1998-03-12 meeting, so they do not cover it. Erratum 3740 replaces "
           "them with RRULE-based 1967/1987 observances.",
   corrected={"replace": [
       ("BEGIN:STANDARD\r\nDTSTART:19981025T020000\r\n",
        "BEGIN:STANDARD\r\nDTSTART:19671029T020000\r\n"
        "RRULE:FREQ=YEARLY;BYMONTH=10;BYDAY=-1SU;UNTIL=20061029T060000Z\r\n"),
       ("BEGIN:DAYLIGHT\r\nDTSTART:19990404T020000\r\n",
        "BEGIN:DAYLIGHT\r\nDTSTART:19870405T020000\r\n"
        "RRULE:FREQ=YEARLY;BYMONTH=4;BYDAY=1SU;UNTIL=20060402T070000Z\r\n")]})
ex("4", "Calendar passed in a MIME message (METHOD:xyz)", "calendar", (8101, 8130), ics=True,
   comment="In the RFC, this calendar is the body of a MIME message with these headers "
           "(not included in text): 'TO:jsmith@example.com', 'FROM:jdoe@example.com', "
           "'MIME-VERSION:1.0', 'MESSAGE-ID:<id3@example.com>', 'CONTENT-TYPE:text/calendar; "
           "method=\"xyz\"; component=\"VEVENT\"'. METHOD:xyz is a placeholder method name. "
           "It is valid as an iana-token but is not registered.")
ex("4", "To-do with audio alarm", "calendar", (8138, 8159), ics=True, valid=False,
   errata="2039",
   comment="TRIGGER defaults to DURATION, so the DATE-TIME value needs VALUE=DATE-TIME. "
           "Erratum 2039 adds it. Also (no erratum): the RFC's prose says the alarm fires "
           "'at noon, the day before' the to-do is due. TRIGGER is 1998-04-03 12:00Z, which "
           "is 12 days before DUE (1998-04-15).",
   corrected={"replace": [("TRIGGER:19980403T120000Z", "TRIGGER;VALUE=DATE-TIME:19980403T120000Z")]})
ex("4", "Journal entry", "calendar", (8163, 8193), ics=True,
   comment="A continuation line after the page break starts with two spaces ('2.' + "
           "' Definition').")
ex("4", "Published busy time", "calendar", (8199, 8211), ics=True, valid=False,
   errata="4149",
   comment="The VFREEBUSY has no UID or DTSTAMP, which are both REQUIRED. Erratum 4149 adds "
           "them.",
   corrected={"replace": [("BEGIN:VFREEBUSY\r\nORGANIZER",
                           "BEGIN:VFREEBUSY\r\nUID:19970901T115957Z-76A912@example.com\r\n"
                           "DTSTAMP:19970901T120000Z\r\nORGANIZER")]})

# ---- 8.2.6 Registration template example -----------------------------------
ex("8.2.6", "CLASS (registration template example value)", "property", 8634,
   comment="Example for registering a new CLASS value. TOP-SECRET is not registered. It "
           "is valid syntax as an iana-token classvalue.")

with open(OUT_JSON, "w", encoding="utf-8", newline="\n") as f:
    json.dump(entries, f, indent=2, ensure_ascii=False)
    f.write("\n")
print("entries:", len(entries))
