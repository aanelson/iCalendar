"""Shared helpers for building unified recur.json vector files.

Imported by the per-source converters (which add this directory to sys.path
explicitly, because they are run with `python3 -I`).
"""
import json
import re

DATE_RE = re.compile(r"^\d{8}$")
DT_RE = re.compile(r"^\d{8}T\d{6}$")
DTZ_RE = re.compile(r"^\d{8}T\d{6}Z$")


def is_basic(s):
    return bool(DATE_RE.match(s) or DT_RE.match(s) or DTZ_RE.match(s))


def basic(y, mo, d, h=None, mi=0, s=0, utc=False):
    if h is None:
        return "%04d%02d%02d" % (y, mo, d)
    return "%04d%02d%02dT%02d%02d%02d%s" % (y, mo, d, h, mi, s, "Z" if utc else "")


_ISO = re.compile(
    r"^(\d{4})-?(\d{2})-?(\d{2})(?:[T ](\d{2}):?(\d{2})(?::?(\d{2}))?(Z)?)?$")


def iso_to_basic(s):
    """'2012-01-01T09:00:00' / '2012-01-01 09:00:00' / '2012-01-01' /
    '20120101T090000Z' -> RFC 5545 basic form."""
    m = _ISO.match(s.strip())
    if not m:
        raise ValueError("unrecognised date/time %r" % s)
    y, mo, d, h, mi, sec, z = m.groups()
    if h is None:
        return "%s%s%s" % (y, mo, d)
    return "%s%s%sT%s%s%s%s" % (y, mo, d, h, mi, sec or "00", "Z" if z else "")


RFC_PARTS = {
    "FREQ", "UNTIL", "COUNT", "INTERVAL", "BYSECOND", "BYMINUTE", "BYHOUR",
    "BYDAY", "BYMONTHDAY", "BYYEARDAY", "BYWEEKNO", "BYMONTH", "BYSETPOS",
    "WKST",
}


def rule_parts(rule):
    parts = {}
    for p in rule.split(";"):
        if not p:
            continue
        if "=" in p:
            k, v = p.split("=", 1)
        else:
            k, v = p, ""
        parts[k.strip().upper()] = v.strip()
    return parts


def nonstandard_features(rule):
    """Return a list of extension / non-RFC-5545 markers for an RRULE value.

    Markers:
      RSCALE, SKIP           RFC 7529 (non-Gregorian recurrence)
      BYEASTER               dateutil extension
      NONSTANDARD:<detail>   a rule-part combination RFC 5545 section 3.3.10
                             does not allow (the source nevertheless defines
                             behaviour for it)
    Unknown rule parts are NOT reported here (they are usually error cases).
    """
    out = []
    p = rule_parts(rule)
    if "RSCALE" in p:
        out.append("RSCALE")
    if "SKIP" in p:
        out.append("SKIP")
    if "BYEASTER" in p:
        out.append("BYEASTER")
    freq = p.get("FREQ", "").upper()
    if "BYWEEKNO" in p and freq and freq != "YEARLY":
        out.append("NONSTANDARD:BYWEEKNO-WITH-" + freq)
    if "BYYEARDAY" in p and freq in ("DAILY", "WEEKLY", "MONTHLY"):
        out.append("NONSTANDARD:BYYEARDAY-WITH-" + freq)
    if "BYMONTHDAY" in p and freq == "WEEKLY":
        out.append("NONSTANDARD:BYMONTHDAY-WITH-WEEKLY")
    if "BYDAY" in p and re.search(r"[+-]?\d", p["BYDAY"]):
        if freq and freq not in ("MONTHLY", "YEARLY"):
            out.append("NONSTANDARD:BYDAY-ORDINAL-WITH-" + freq)
        elif freq == "YEARLY" and "BYWEEKNO" in p:
            out.append("NONSTANDARD:BYDAY-ORDINAL-WITH-BYWEEKNO")
    if "BYSETPOS" in p:
        others = [k for k in p if k.startswith("BY") and k != "BYSETPOS"]
        if not others:
            out.append("NONSTANDARD:BYSETPOS-WITHOUT-OTHER-BYxxx")
    return out


def make_case(id, dtstart, rrule, expected, include_dtstart, **kw):
    c = {"id": id}
    if kw.get("name"):
        c["name"] = kw["name"]
    c["dtstart"] = dtstart
    if kw.get("tzid"):
        c["tzid"] = kw["tzid"]
    c["rrule"] = list(rrule)
    for k in ("exrule", "rdate", "exdate"):
        if kw.get(k):
            c[k] = list(kw[k])
    c["include_dtstart"] = include_dtstart
    if kw.get("between"):
        c["between"] = kw["between"]
    if kw.get("limit") is not None:
        c["limit"] = kw["limit"]
    c["expected"] = list(expected)
    c["error"] = bool(kw.get("error", False))
    ext = list(kw.get("extensions") or [])
    for r in list(rrule) + list(kw.get("exrule") or []):
        for e in nonstandard_features(r):
            if e not in ext:
                ext.append(e)
    c["extensions"] = ext
    c["comment"] = kw.get("comment", "") or ""
    return c


def write(path, source, upstream, commit, license, srcpath, semantics, cases):
    doc = {
        "source": source,
        "upstream": upstream,
        "commit": commit,
        "license": license,
        "path": srcpath,
        "semantics": semantics,
        "cases": cases,
    }
    with open(path, "w") as fh:
        json.dump(doc, fh, indent=1, ensure_ascii=False)
        fh.write("\n")
    print("wrote %s: %d cases" % (path, len(cases)))
