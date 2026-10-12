#!/usr/bin/env python3
"""Extract testRecurring(msg, new RRule({...}) | {rrule, method, args}, expected)
cases from rrule.js test/rrule.test.ts (static parsing only).

Usage: python3 -I conv_rrulejs.py TESTFILE OUT.json COMMIT
"""
import os
import re
import sys
from collections import Counter

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import jslit  # noqa: E402
import vec_common as vc  # noqa: E402

FREQ = {"RRule.YEARLY", "RRule.MONTHLY", "RRule.WEEKLY", "RRule.DAILY", "RRule.HOURLY",
        "RRule.MINUTELY", "RRule.SECONDLY", "Frequency.YEARLY", "Frequency.MONTHLY", "Frequency.WEEKLY",
        "Frequency.DAILY", "Frequency.HOURLY", "Frequency.MINUTELY", "Frequency.SECONDLY"}
WD = ["MO", "TU", "WE", "TH", "FR", "SA", "SU"]
ORDER = ["freq", "interval", "count", "until", "wkst", "bysetpos", "bymonth", "bymonthday",
         "byyearday", "byweekno", "byweekday", "byhour", "byminute", "bysecond", "byeaster"]
NAME = {"freq": "FREQ", "interval": "INTERVAL", "count": "COUNT", "until": "UNTIL", "wkst": "WKST",
        "bysetpos": "BYSETPOS", "bymonth": "BYMONTH", "bymonthday": "BYMONTHDAY", "byyearday": "BYYEARDAY",
        "byweekno": "BYWEEKNO", "byweekday": "BYDAY", "byhour": "BYHOUR", "byminute": "BYMINUTE",
        "bysecond": "BYSECOND", "byeaster": "BYEASTER"}

SEM = (
    "rrule.js (jkbrzt/rrule, a JavaScript port of python-dateutil) test/rrule.test.ts testRecurring(msg, rule | "
    "{rrule, method, args}, expected). new RRule({...}) options were mapped to RRULE text like the dateutil source "
    "(freq, interval, count, until, wkst, bysetpos, bymonth, bymonthday, byyearday, byweekno, byweekday "
    "(RRule.TU.nth(-1) -> -1TU), byhour, byminute, bysecond, byeaster -> BYEASTER + extensions). rrule.js "
    "datetime(y,m,d,h,mi,s) and parse('YYYYMMDDTHHMMSS') build UTC JS Dates, so every value here carries a Z suffix. "
    "Like dateutil, DTSTART is NOT emitted unless it matches the rule (include_dtstart=false). method 'all' -> full "
    "list; 'between'(a, b, inc) -> between (exclusive unless inc); 'after'(dt, inc) -> between.start + limit 1. "
    "The upstream assertDatesEqual() only checks the length when more than one date is expected, so cases with a "
    "single expected date and method 'all' get limit=1 (only the first instance is compared), and cases with no "
    "expected dates are skipped (nothing is asserted). Most cases duplicate dateutil's suite.")


class Skip(Exception):
    pass


def dt(v):
    if isinstance(v, tuple) and v[0] == "call" and v[1] == "datetime":
        a = v[2]
        if not all(isinstance(x, int) for x in a) or not 3 <= len(a) <= 6:
            raise Skip("datetime args")
        a = (list(a) + [0, 0, 0])[:6]
        return vc.basic(*a, utc=True)
    if isinstance(v, tuple) and v[0] == "call" and v[1] == "parse" and len(v[2]) == 1 and isinstance(v[2][0], str):
        s = v[2][0]
        if not re.match(r"^\d{8}T\d{6}$", s):
            raise Skip("parse() format")
        return s + "Z"
    raise Skip("unsupported date expression %r" % (v,))


def wd(v):
    if isinstance(v, int):
        return WD[v]
    if isinstance(v, tuple) and v[0] == "ident" and v[1].startswith("RRule.") and v[1][6:] in WD:
        return v[1][6:]
    if isinstance(v, tuple) and v[0] == "method" and v[2] == "nth" and isinstance(v[1], tuple) \
            and v[1][0] == "ident" and v[1][1][6:] in WD and len(v[3]) == 1 and isinstance(v[3][0], int):
        return "%d%s" % (v[3][0], v[1][1][6:])
    raise Skip("weekday %r" % (v,))


def rule_from(v):
    if not (isinstance(v, tuple) and v[0] == "new" and v[1] == "RRule" and len(v[2]) == 1 and isinstance(v[2][0], dict)):
        raise Skip("not new RRule({...})")
    o = dict(v[2][0])
    if "tzid" in o:
        raise Skip("tzid option")
    if "dtstart" not in o:
        raise Skip("no dtstart")
    dts = dt(o.pop("dtstart"))
    unknown = set(o) - set(ORDER)
    if unknown:
        raise Skip("unknown options %s" % sorted(unknown))
    parts, ext = [], []
    notes = []
    if "freq" not in o:
        o["freq"] = ("ident", "RRule.YEARLY")
        notes.append("freq omitted in the source; rrule.js defaults to YEARLY")
    if "byeaster" in o:
        notes.append("rrule.js parseOptions() forces freq=YEARLY whenever byeaster is set")
    for k in ORDER:
        if k not in o or o[k] is None:
            continue
        x = o[k]
        if k == "freq":
            if not (isinstance(x, tuple) and x[0] == "ident" and x[1] in FREQ):
                raise Skip("freq")
            parts.append("FREQ=" + x[1].split(".")[1])
        elif k in ("interval", "count"):
            parts.append("%s=%d" % (NAME[k], x))
        elif k == "until":
            parts.append("UNTIL=" + dt(x))
        elif k == "wkst":
            parts.append("WKST=" + wd(x))
        elif k == "byweekday":
            xs = x if isinstance(x, list) else [x]
            parts.append("BYDAY=" + ",".join(wd(y) for y in xs))
        else:
            xs = x if isinstance(x, list) else [x]
            if not all(isinstance(y, int) and not isinstance(y, bool) for y in xs):
                raise Skip("%s values" % k)
            parts.append("%s=%s" % (NAME[k], ",".join(map(str, xs))))
            if k == "byeaster":
                ext.append("BYEASTER")
    return dts, ";".join(parts), ext, notes


def main():
    path, out, commit = sys.argv[1], sys.argv[2], sys.argv[3]
    orig = open(path, encoding="utf-8").read()
    src = jslit.blank_comments(orig)
    cases, skipped = [], Counter()
    n = 0
    for m in re.finditer(r"\btestRecurring(?:\.only|\.skip)?\(", src):
        line = orig.count("\n", 0, m.start()) + 1
        try:
            if src[m.start():m.end()].endswith(".skip("):
                raise Skip("testRecurring.skip")
            p = jslit.Parser(src, m.end() - 1)
            try:
                args = p.args()
            except jslit.ParseError as e:
                raise Skip("parse error (%s)" % str(e)[:30])
            if len(args) < 3:
                raise Skip("arity")
            msg, obj, exp = args[0], args[1], args[2]
            if not isinstance(msg, str):
                msg = None
            method, margs = "all", []
            if isinstance(obj, dict):
                method = obj.get("method", "all")
                margs = obj.get("args", [])
                obj = obj.get("rrule")
            dts, rule, ext, notes = rule_from(obj)
            exp_l = exp if isinstance(exp, list) else [exp]
            expected = [dt(x) for x in exp_l]
            between, limit = None, None
            comment = notes + ["source line %d" % line]
            if method == "between":
                inc = bool(margs[2]) if len(margs) > 2 else False
                between = {"start": dt(margs[0]), "end": dt(margs[1]), "inclusive": inc}
            elif method == "after":
                inc = bool(margs[1]) if len(margs) > 1 else False
                between = {"start": dt(margs[0]), "inclusive": inc}
                limit = 1
            elif method != "all":
                raise Skip("method %s" % method)
            if not expected:
                raise Skip("no expected dates (assertDatesEqual asserts nothing)")
            if method == "all" and len(expected) == 1:
                limit = 1
                comment.append("single expected date: upstream only compares the first instance")
            n += 1
            cases.append(vc.make_case("rrulejs-%04d" % n, dts, [rule], expected, False, name=msg,
                                      between=between, limit=limit, extensions=ext, comment="; ".join(comment)))
        except Skip as e:
            skipped[str(e)] += 1
    vc.write(out, "rrulejs", "https://github.com/jkbrzt/rrule", commit, "BSD-3-Clause",
             "test/rrule.test.ts", SEM, cases)
    for k, v in skipped.most_common():
        print("skipped %d: %s" % (v, k))


if __name__ == "__main__":
    main()
