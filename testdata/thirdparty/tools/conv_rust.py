#!/usr/bin/env python3
"""Statically extract recurrence vectors from fmeringdal/rust-rrule tests
(rrule/src/tests/{rfc_tests,regression,daylight_saving,rrule}.rs).

Usage: python3 -I conv_rust.py TESTS_DIR OUT.json COMMIT
"""
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import vec_common as vc  # noqa: E402

SEM = (
    "rust-rrule (a Rust port of python-dateutil's rrule). Two test styles are converted: (1) string tests "
    "(rfc_tests.rs, regression.rs, daylight_saving.rs): \"DTSTART...\\nRRULE...\".parse::<RRuleSet>() then "
    ".all(N) (at most N instances; u16::MAX = effectively all) or .all_unchecked(), compared with "
    "common::check_occurrences(&dates, &[RFC 3339 strings]); the RFC 3339 values are converted to wall-clock "
    "basic form in the DTSTART zone (tzid) - the UTC offsets in the source are consistent with that zone. "
    "(2) struct tests (rrule.rs): RRule { freq, count, interval, until, week_start, by_* .. } + "
    "test_recurring_rrule(rrule, limited, dt_start, &[expected]) where dt_start/expected are ymd_hms(...) in UTC "
    "(written with a Z suffix); the struct fields were mapped to RRULE text (by_n_month_day merged into BYMONTHDAY; "
    "NWeekday::Nth(n, D) -> nDD); the explicit by_hour/by_minute/by_second/by_month/by_month_day values that the "
    "tests copy from dt_start are kept verbatim. As in dateutil, DTSTART is NOT emitted unless it matches the rule "
    "(include_dtstart=false). 'limit' = N for .all(N) with N != u16::MAX. Tests gated by #[cfg(feature = \"by-easter\")] "
    "carry extensions [\"BYEASTER\"]. Tests that build rules with the builder API, use RRuleSet builders (rruleset.rs), "
    "assert something other than the instance list, or are not mechanically extractable were skipped.")

WD = {"Mon": "MO", "Tue": "TU", "Wed": "WE", "Thu": "TH", "Fri": "FR", "Sat": "SA", "Sun": "SU"}


class Skip(Exception):
    pass


def rust_string(src, i):
    """Parse a Rust string literal starting at src[i] (") or r"/r#". Return (value, end)."""
    if src[i] == "r":
        j = i + 1
        hashes = 0
        while src[j] == "#":
            hashes += 1
            j += 1
        assert src[j] == '"'
        end = src.index('"' + "#" * hashes, j + 1)
        return src[j + 1:end], end + 1 + hashes
    assert src[i] == '"'
    j = i + 1
    out = []
    while True:
        c = src[j]
        if c == '"':
            return "".join(out), j + 1
        if c == "\\":
            nx = src[j + 1]
            if nx == "\n":
                j += 2
                while src[j] in " \t\r\n":
                    j += 1
                continue
            out.append({"n": "\n", "t": "\t", "r": "\r", "\\": "\\", '"': '"', "'": "'", "0": "\0"}.get(nx, "\\" + nx))
            j += 2
            continue
        out.append(c)
        j += 1


def blank_comments(src):
    out = list(src)
    i = 0
    n = len(src)
    while i < n:
        if src[i] == '"' or (src[i] == "r" and src[i + 1:i + 2] in ('"', "#") and not src[i - 1].isalnum()):
            try:
                _, e = rust_string(src, i)
                i = e
                continue
            except Exception:  # noqa: BLE001
                i += 1
                continue
        if src.startswith("//", i):
            j = src.find("\n", i)
            j = n if j < 0 else j
            for k in range(i, j):
                out[k] = " "
            i = j
            continue
        if src.startswith("/*", i):
            j = src.find("*/", i) + 2
            for k in range(i, j):
                if out[k] != "\n":
                    out[k] = " "
            i = j
            continue
        i += 1
    return "".join(out)


def match_brace(src, i, open_="{", close="}"):
    depth = 0
    while i < len(src):
        c = src[i]
        if c == '"':
            _, i = rust_string(src, i)
            continue
        if c == open_:
            depth += 1
        elif c == close:
            depth -= 1
            if depth == 0:
                return i
        i += 1
    raise Skip("unbalanced")


def tests_in(src):
    for m in re.finditer(r"#\[test\]\s*((?:#\[[^\]]*\]\s*)*)fn\s+(\w+)\s*\(\)\s*\{", src):
        end = match_brace(src, m.end() - 1)
        yield m.group(2), m.group(1), src[m.end():end]


YMD = re.compile(r"ymd_hms\(\s*(-?\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*\)")
RFC3339 = re.compile(r"^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(Z|[+-]\d{2}:\d{2})$")


def ints(text):
    text = text.strip()
    if not text:
        return []
    out = []
    for t in text.split(","):
        t = t.strip()
        if not t:
            continue
        if not re.match(r"^-?\d+$", t):
            raise Skip("non-integer list element %r" % t)
        out.append(int(t))
    return out


def vec_body(v):
    v = v.strip()
    m = re.match(r"^vec!\[(.*)\]$", v, re.S)
    if not m:
        raise Skip("not a vec! literal: %r" % v[:40])
    return m.group(1)


def split_top(text, sep=","):
    parts, depth, cur, i = [], 0, [], 0
    while i < len(text):
        c = text[i]
        if c == '"':
            _, e = rust_string(text, i)
            cur.append(text[i:e])
            i = e
            continue
        if c in "([{":
            depth += 1
        elif c in ")]}":
            depth -= 1
        if c == sep and depth == 0:
            parts.append("".join(cur))
            cur = []
        else:
            cur.append(c)
        i += 1
    if "".join(cur).strip():
        parts.append("".join(cur))
    return [p.strip() for p in parts if p.strip()]


def struct_rule(body):
    m = re.search(r"RRule\s*\{", body)
    if not m:
        raise Skip("no RRule struct literal")
    end = match_brace(body, m.end() - 1)
    inner = body[m.end():end]
    if len(re.findall(r"RRule\s*\{", body)) != 1:
        raise Skip("multiple RRule literals")
    fields = {}
    for part in split_top(inner):
        if part.startswith(".."):
            continue
        k, v = part.split(":", 1)
        fields[k.strip()] = v.strip()
    parts = []
    ext = []
    if "freq" not in fields:
        raise Skip("no freq")
    fm = re.match(r"^Frequency::(\w+)$", fields.pop("freq"))
    parts.append("FREQ=" + fm.group(1).upper())
    if "interval" in fields:
        parts.append("INTERVAL=%d" % int(fields.pop("interval")))
    if "count" in fields:
        cm = re.match(r"^Some\((\d+)\)$", fields.pop("count"))
        parts.append("COUNT=%s" % cm.group(1))
    if "until" in fields:
        um = YMD.search(fields.pop("until"))
        parts.append("UNTIL=" + vc.basic(*[int(x) for x in um.groups()], utc=True))
    if "week_start" in fields:
        wm = re.match(r"^Weekday::(\w+)$", fields.pop("week_start"))
        parts.append("WKST=" + WD[wm.group(1)])
    if "by_set_pos" in fields:
        parts.append("BYSETPOS=" + ",".join(map(str, ints(vec_body(fields.pop("by_set_pos"))))))
    if "by_month" in fields:
        parts.append("BYMONTH=" + ",".join(map(str, ints(vec_body(fields.pop("by_month"))))))
    md = []
    if "by_month_day" in fields:
        md += ints(vec_body(fields.pop("by_month_day")))
    if "by_n_month_day" in fields:
        md += ints(vec_body(fields.pop("by_n_month_day")))
    if md:
        parts.append("BYMONTHDAY=" + ",".join(map(str, md)))
    if "by_year_day" in fields:
        parts.append("BYYEARDAY=" + ",".join(map(str, ints(vec_body(fields.pop("by_year_day"))))))
    if "by_week_no" in fields:
        parts.append("BYWEEKNO=" + ",".join(map(str, ints(vec_body(fields.pop("by_week_no"))))))
    if "by_weekday" in fields:
        days = []
        for w in split_top(vec_body(fields.pop("by_weekday"))):
            em = re.match(r"^NWeekday::Every\(Weekday::(\w+)\)$", w)
            nm = re.match(r"^NWeekday::Nth\((-?\d+),\s*Weekday::(\w+)\)$", w)
            if em:
                days.append(WD[em.group(1)])
            elif nm:
                days.append("%d%s" % (int(nm.group(1)), WD[nm.group(2)]))
            else:
                raise Skip("weekday form %r" % w)
        parts.append("BYDAY=" + ",".join(days))
    for k, name in (("by_hour", "BYHOUR"), ("by_minute", "BYMINUTE"), ("by_second", "BYSECOND")):
        if k in fields:
            vals = ints(vec_body(fields.pop(k)))
            if vals:
                parts.append("%s=%s" % (name, ",".join(map(str, vals))))
    if "by_easter" in fields:
        em = re.match(r"^Some\((-?\d+)\)$", fields.pop("by_easter"))
        parts.append("BYEASTER=%s" % em.group(1))
        ext.append("BYEASTER")
    if fields:
        raise Skip("unmapped struct fields %s" % sorted(fields))
    return ";".join(parts), ext


def main():
    tdir, out, commit = sys.argv[1], sys.argv[2], sys.argv[3]
    cases = []
    skipped = []
    n = 0
    for fname in ("rfc_tests.rs", "regression.rs", "daylight_saving.rs", "rrule.rs"):
        path = os.path.join(tdir, fname)
        orig = open(path, encoding="utf-8").read()
        src = blank_comments(orig)
        for name, attrs, body in tests_in(src):
            line = orig.count("\n", 0, orig.find("fn %s(" % name)) + 1
            try:
                ext = []
                comment = []
                if "by-easter" in attrs:
                    ext.append("BYEASTER")
                if "check_occurrences" in body:
                    # string style
                    sm = None
                    for m in re.finditer(r'(?<![A-Za-z0-9_])(r#*")|"', body):
                        st = m.start()
                        val, e = rust_string(body, st)
                        if "RRULE" in val.upper() or "DTSTART" in val.upper():
                            sm = val
                            break
                    if sm is None:
                        raise Skip("no rule string")
                    am = re.search(r"\.all\(([^)]*)\)", body)
                    unchecked = ".all_unchecked()" in body
                    limit = None
                    if am:
                        a = am.group(1).strip()
                        if a != "u16::MAX":
                            limit = int(a)
                    elif not unchecked:
                        raise Skip("no .all()")
                    co = body.index("check_occurrences(")
                    arr_start = body.index("&[", co)
                    arr_end = match_brace(body, arr_start + 1, "[", "]")
                    exp_raw = [rust_string(p, 0)[0] for p in split_top(body[arr_start + 2:arr_end])]
                    if body.count("check_occurrences(") != 1:
                        raise Skip("multiple check_occurrences")
                    # parse rule text
                    dtstart, tzid, rrules, exdates, rdates = None, None, [], [], []
                    for ln in re.split(r"\s*\n\s*", sm.strip()):
                        if ":" not in ln:
                            raise Skip("line without ':' %r" % ln)
                        nm, val = ln.split(":", 1)
                        params = nm.split(";")[1:]
                        nm = nm.split(";")[0].upper()
                        ptz = None
                        for p in params:
                            if p.upper().startswith("TZID="):
                                ptz = p.split("=", 1)[1]
                            elif p.upper() not in ("VALUE=DATE-TIME", "VALUE=DATE"):
                                raise Skip("param %s" % p)
                        if nm == "DTSTART":
                            dtstart, tzid = val.strip(), ptz
                        elif nm == "RRULE":
                            rrules.append(val.strip())
                        elif nm in ("EXDATE", "RDATE"):
                            if ptz != tzid:
                                raise Skip("EXDATE/RDATE zone differs")
                            (exdates if nm == "EXDATE" else rdates).extend(v.strip() for v in val.split(","))
                        else:
                            raise Skip("property %s" % nm)
                    if not dtstart or not vc.is_basic(dtstart):
                        raise Skip("dtstart %r" % dtstart)
                    utc = dtstart.endswith("Z")
                    if not tzid and not utc:
                        raise Skip("floating DTSTART (rust-rrule uses the local zone)")
                    expected = []
                    for e in exp_raw:
                        mm = RFC3339.match(e)
                        if not mm:
                            raise Skip("expected value %r" % e)
                        y, mo, d, h, mi, s, off = mm.groups()
                        if utc and off not in ("Z", "+00:00"):
                            raise Skip("non-UTC offset for UTC dtstart")
                        expected.append("%s%s%sT%s%s%s%s" % (y, mo, d, h, mi, s, "Z" if utc else ""))
                    if unchecked and not am:
                        comment.append("all_unchecked()")
                    n += 1
                    comment.append("%s:%d" % (fname, line))
                    cases.append(vc.make_case("rust-rrule-%04d" % n, dtstart, rrules, expected, False,
                                              name="%s::%s" % (fname[:-3], name), tzid=tzid, exdate=exdates,
                                              rdate=rdates, limit=limit, extensions=ext, comment="; ".join(comment)))
                elif "test_recurring_rrule(" in body:
                    if body.count("test_recurring_rrule(") != 1:
                        raise Skip("multiple test_recurring_rrule calls")
                    rule, e2 = struct_rule(body)
                    for e in e2:
                        if e not in ext:
                            ext.append(e)
                    call = body.index("test_recurring_rrule(")
                    cend = match_brace(body, call + len("test_recurring_rrule"), "(", ")")
                    args = split_top(body[call + len("test_recurring_rrule("):cend])
                    if len(args) != 4:
                        raise Skip("test_recurring_rrule arity")
                    limited = args[1] == "true"
                    dm = YMD.fullmatch(args[2].strip())
                    if not dm:
                        raise Skip("dt_start form")
                    dts = vc.basic(*[int(x) for x in dm.groups()], utc=True)
                    arr = args[3].strip()
                    if not arr.startswith("&["):
                        raise Skip("expected form")
                    items = split_top(arr[2:-1])
                    expected = []
                    for it in items:
                        mm = YMD.fullmatch(it)
                        if not mm:
                            raise Skip("expected element form %r" % it)
                        expected.append(vc.basic(*[int(x) for x in mm.groups()], utc=True))
                    if not limited:
                        comment.append("all_unchecked()")
                    n += 1
                    comment.append("%s:%d" % (fname, line))
                    cases.append(vc.make_case("rust-rrule-%04d" % n, dts, [rule], expected, False,
                                              name="%s::%s" % (fname[:-3], name), extensions=ext,
                                              comment="; ".join(comment)))
                else:
                    raise Skip("no recognised assertion helper")
            except Skip as e:
                skipped.append(("%s::%s" % (fname, name), str(e)))
            except (ValueError, AttributeError, IndexError, KeyError) as e:
                skipped.append(("%s::%s" % (fname, name), "parse failure: %r" % e))
    vc.write(out, "rust-rrule", "https://github.com/fmeringdal/rust-rrule", commit, "MIT OR Apache-2.0",
             "rrule/src/tests/{rfc_tests,regression,daylight_saving,rrule}.rs", SEM, cases)
    for s in skipped:
        print("skipped %s: %s" % s)


if __name__ == "__main__":
    main()
