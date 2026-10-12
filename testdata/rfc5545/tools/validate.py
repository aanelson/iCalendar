"""Validate testdata/rfc5545/{recur,examples}.json and ics/*.ics.

Usage: python3 -I validate.py <worktree root>
"""
import base64
import binascii
import json
import os
import re
import sys
from datetime import datetime

ROOT = sys.argv[1]
TD = os.path.join(ROOT, "testdata", "rfc5545")
RFC = os.path.join(ROOT, "docs", "rfc", "rfc5545.txt")
problems = []
notes = []

DATE_RE = re.compile(r"^(\d{8})$|^(\d{8}T\d{6})$|^(\d{8}T\d{6}Z)$")


def check_date(s, where):
    if not isinstance(s, str) or not DATE_RE.match(s):
        problems.append("bad date format %r at %s" % (s, where))
        return
    fmt = {8: "%Y%m%d", 15: "%Y%m%dT%H%M%S", 16: "%Y%m%dT%H%M%SZ"}[len(s)]
    try:
        datetime.strptime(s, fmt)
    except ValueError:
        if not (len(s) >= 15 and s[13:15] == "60"):  # leap second
            problems.append("not a real date/time %r at %s" % (s, where))


# ---------------- recur.json ----------------
recur = json.load(open(os.path.join(TD, "recur.json"), encoding="utf-8"))
KEYS = ["id", "name", "dtstart", "tzid", "rrule", "exrule", "rdate", "exdate",
        "include_dtstart", "between", "limit", "expected", "error", "extensions", "comment"]
for k in ("source", "upstream", "commit", "license", "path", "semantics", "cases"):
    if k not in recur:
        problems.append("recur.json missing top-level key " + k)
ids = [c["id"] for c in recur["cases"]]
if len(ids) != len(set(ids)):
    problems.append("duplicate recur ids")
for c in recur["cases"]:
    if list(c.keys()) != KEYS:
        problems.append("recur case %s keys %s" % (c["id"], list(c.keys())))
    check_date(c["dtstart"], c["id"] + ".dtstart")
    for f in ("rdate", "exdate", "expected"):
        for i, v in enumerate(c[f]):
            check_date(v, "%s.%s[%d]" % (c["id"], f, i))
    if c["between"] is not None:
        check_date(c["between"]["start"], c["id"] + ".between.start")
        check_date(c["between"]["end"], c["id"] + ".between.end")
    for r in c["rrule"] + c["exrule"]:
        m = re.search(r"UNTIL=([^;]+)", r)
        if m:
            check_date(m.group(1), c["id"] + ".UNTIL")
    if c["expected"] != sorted(c["expected"]):
        problems.append("expected not sorted " + c["id"])
    if len(set(c["expected"])) != len(c["expected"]):
        problems.append("duplicate expected " + c["id"])
    if c["limit"] and c["limit"] != len(c["expected"]):
        problems.append("limit != len(expected) " + c["id"])
    if c["include_dtstart"] is not True:
        problems.append("include_dtstart not true " + c["id"])
    if not c["exdate"] and c["expected"][0] != c["dtstart"]:
        problems.append("first instance is not DTSTART " + c["id"])

# ---------------- examples.json ----------------
exs = json.load(open(os.path.join(TD, "examples.json"), encoding="utf-8"))
EKEYS = ["id", "section", "title", "kind", "text", "valid", "errata", "comment"]
eids = [e["id"] for e in exs]
if len(eids) != len(set(eids)):
    problems.append("duplicate example ids")
if set(eids) & set(ids):
    problems.append("ids shared between files")
rfc_lines = set(l.rstrip() for l in open(RFC, encoding="ascii").read().split("\n"))
rfc_text = open(RFC, encoding="ascii").read()
for e in exs:
    if list(e.keys()) != EKEYS:
        problems.append("example %s keys" % e["id"])
    if e["kind"] not in ("property", "parameter", "component", "calendar", "value"):
        problems.append("bad kind " + e["id"])
    t = e["text"]
    if "\n" in t.replace("\r\n", "") or "\r" in t.replace("\r\n", ""):
        problems.append("bare CR/LF in " + e["id"])
    if t.endswith("\r\n") or t.startswith(" "):
        problems.append("leading space / trailing CRLF in " + e["id"])
    if any(not (32 <= ord(ch) < 127) for ch in t.replace("\r\n", "")):
        problems.append("non-ASCII/control char in " + e["id"])
    # exact-copy check: every line must be a line of the RFC (after re-indenting)
    if not e["id"].endswith("-corrected") and e["id"] not in ("rfc5545-3.5-b", "rfc5545-3.5-c"):
        for ln in t.split("\r\n"):
            exact = any((" " * k + ln) in rfc_lines for k in range(3, 9))
            # entries whose RFC annotation (e.g. ";Invalid time format") was stripped
            annotated = any(re.match(r"^ {3,8}" + re.escape(ln) + r"\s{2,};", l)
                            for l in rfc_lines)
            if not exact and not annotated:
                problems.append("line not found in RFC (%s): %r" % (e["id"], ln))
    else:
        if e["id"].startswith("rfc5545-3.5-"):
            if t not in rfc_text:
                problems.append("inline example not in RFC text " + e["id"])
    for ln in t.split("\r\n"):
        if len(ln.encode()) > 75:
            notes.append("%s: line longer than 75 octets (%d)" % (e["id"], len(ln)))


def unfold(t):
    return re.sub(r"\r\n[ \t]", "", t).split("\r\n")


def split_cl(line):
    """Split a content line into name, params, value honoring quoted strings."""
    i, q = 0, False
    while i < len(line):
        ch = line[i]
        if ch == '"':
            q = not q
        elif ch == ":" and not q:
            break
        i += 1
    head, value = line[:i], line[i + 1:]
    parts, cur, q = [], "", False
    for ch in head:
        if ch == '"':
            q = not q
        if ch == ";" and not q:
            parts.append(cur)
            cur = ""
        else:
            cur += ch
    parts.append(cur)
    return parts[0].upper(), parts[1:], value


DT_PROPS = {"DTSTART", "DTEND", "DUE", "DTSTAMP", "CREATED", "LAST-MODIFIED", "COMPLETED",
            "RECURRENCE-ID", "EXDATE", "RDATE"}
date_bad = []
for e in exs:
    if e["kind"] == "value":
        if re.match(r"^\d{8}(T\d{6}Z?)?$", e["text"]):
            check_date(e["text"], e["id"])
        continue
    for line in unfold(e["text"]):
        name, params, value = split_cl(line)
        pv = [p.upper() for p in params]
        if name in DT_PROPS or (name == "TRIGGER" and "VALUE=DATE-TIME" in pv):
            for v in value.split(","):
                if "VALUE=PERIOD" in pv:
                    v = v.split("/")[0]
                if not DATE_RE.match(v):
                    date_bad.append((e["id"], name, v, e["valid"]))
                else:
                    check_date(v, e["id"] + " " + name)
        if name == "FREEBUSY":
            for v in value.split(","):
                a, b = v.split("/")
                check_date(a, e["id"] + " FREEBUSY")
                if not b.startswith("P"):
                    check_date(b, e["id"] + " FREEBUSY")
        if name == "RRULE":
            m = re.search(r"UNTIL=([^;]+)", value)
            if m:
                check_date(m.group(1), e["id"] + " UNTIL")
        if name == "ATTACH" and "ENCODING=BASE64" in pv:
            try:
                base64.b64decode(value, validate=True)
                notes.append("%s: BASE64 decodes OK (%d chars)" % (e["id"], len(value)))
            except (binascii.Error, ValueError) as err:
                notes.append("%s: BASE64 invalid (%d chars): %s; valid=%s" % (
                    e["id"], len(value), err, e["valid"]))
for d in date_bad:
    notes.append("non-date value in date property: %s %s %r valid=%s" % d)

# cross-check recur.json RRULE/DTSTART/EXDATE text against the RFC example blocks
rr = [e for e in exs if e["section"] == "3.8.5.3"]
by_case = {}
for e in rr:
    m = re.search(r"case (rfc5545-3\.8\.5\.3-[0-9a-z-]+)", e["comment"])
    by_case.setdefault(m.group(1).rstrip("."), []).append(e)
for c in recur["cases"]:
    es = by_case.get(c["id"])
    if not es:
        problems.append("no RFC example block for recur case " + c["id"])
        continue
    for e in es:
        lines = unfold(e["text"])
        want = {"DTSTART;TZID=America/New_York:" + c["dtstart"], "RRULE:" + c["rrule"][0]}
        want |= {"EXDATE;TZID=America/New_York:" + x for x in c["exdate"]}
        if set(lines) != want:
            problems.append("recur case %s does not match RFC block %s: %s vs %s" % (
                c["id"], e["id"], sorted(want), sorted(lines)))

# ---------------- ics files ----------------
ICS = os.path.join(TD, "ics")
byid = {e["id"]: e for e in exs}
for fn in sorted(os.listdir(ICS)):
    raw = open(os.path.join(ICS, fn), "rb").read()
    s = raw.decode("ascii")
    if not s.endswith("\r\n") or "\n" in s.replace("\r\n", "") or "\r" in s.replace("\r\n", ""):
        problems.append("ics line endings " + fn)
    lines = s[:-2].split("\r\n")
    if lines[0] != "BEGIN:VCALENDAR" or lines[-1] != "END:VCALENDAR":
        problems.append("ics framing " + fn)
    e = byid[fn[:-4]]
    body = "\r\n".join(lines)
    if e["kind"] == "component":
        exp = "\r\n".join(["BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//rfc5545 examples//EN",
                           e["text"], "END:VCALENDAR"])
    else:
        exp = e["text"]
    if body != exp:
        problems.append("ics content differs from examples.json " + fn)

print("recur cases:", len(recur["cases"]), " examples:", len(exs),
      " (originals %d, corrected %d, invalid %d)" % (
          sum(1 for e in exs if not e["id"].endswith("-corrected")),
          sum(1 for e in exs if e["id"].endswith("-corrected")),
          sum(1 for e in exs if not e["valid"])),
      " ics:", len(os.listdir(ICS)))
for n in notes:
    print("note:", n)
for p in problems:
    print("PROBLEM:", p)
print("problems:", len(problems))
sys.exit(1 if problems else 0)
