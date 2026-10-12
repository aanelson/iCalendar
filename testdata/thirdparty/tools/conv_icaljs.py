#!/usr/bin/env python3
"""Extract testRRULE(rule, {dtStart, dates, byCount, until, max, noInstance})
cases from ical.js test/recur_iterator_test.js (static parsing only).

Usage: python3 -I conv_icaljs.py TESTFILE OUT.json COMMIT
"""
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import jslit  # noqa: E402
import vec_common as vc  # noqa: E402

SEM = (
    "ical.js test/recur_iterator_test.js testRRULE(rule, options): start = ICAL.Time.fromString(options.dtStart "
    "or options.dates[0]); iterator = ICAL.Recur.fromString(rule).iterator(start); collects iterator.next().toString() "
    "up to max values, where max = options.max if given, else dates.length+1 for finite (COUNT/UNTIL) rules - so "
    "'expected' is the complete instance list and the iterator must stop - else dates.length for infinite rules "
    "('limit' = number of expected values). ICAL.RecurIterator does NOT emit a DTSTART that does not match the rule "
    "(e.g. MONTHLY;COUNT=10;BYDAY=1FR from 2012-01-07 starts 2012-02-03, and COUNT does not count it), so "
    "include_dtstart=false. All times are floating (no zone) or DATE values; noInstance:true cases expect the "
    "iterator to yield nothing (expected=[] with error=false). Commented-out tests in the source are not included. "
    "Note: ICAL.RecurExpansion (component-level expansion, not covered here) does include DTSTART.")


def conv_time(s):
    return vc.iso_to_basic(s)


def suites_at(src, pos):
    """Names of suite('...') blocks enclosing pos (approximate, by brace depth)."""
    names = []
    for m in re.finditer(r"suite\(\s*(['\"])(.*?)\1\s*,\s*function\s*\(\)\s*\{", src):
        if m.start() > pos:
            break
        # find matching close brace of this suite body
        depth = 0
        i = m.end() - 1
        while i < len(src):
            if src[i] == "{":
                depth += 1
            elif src[i] == "}":
                depth -= 1
                if depth == 0:
                    break
            i += 1
        if m.start() < pos < i:
            names.append(m.group(2))
    return names


def main():
    path, out, commit = sys.argv[1], sys.argv[2], sys.argv[3]
    orig = open(path, encoding="utf-8").read()
    src = jslit.blank_comments(orig)
    cases = []
    skipped = []
    n = 0
    for m in re.finditer(r"\btestRRULE(?:\.only)?\(", src):
        # skip the helper definition itself
        before = src[max(0, m.start() - 9):m.start()]
        if "function" in before:
            continue
        p = jslit.Parser(src, m.end() - 1)
        try:
            args = p.args()
        except jslit.ParseError as e:
            skipped.append((orig.count("\n", 0, m.start()) + 1, "parse error: %s" % e))
            continue
        line = orig.count("\n", 0, m.start()) + 1
        if len(args) != 2 or not isinstance(args[0], str) or not isinstance(args[1], dict):
            skipped.append((line, "unexpected argument shape"))
            continue
        rule, opt = args
        unknown = set(opt) - {"dtStart", "dates", "byCount", "until", "max", "noInstance", "only"}
        if unknown:
            skipped.append((line, "unknown options %s" % sorted(unknown)))
            continue
        dates = opt.get("dates", [])
        if not isinstance(dates, list) or not all(isinstance(d, str) for d in dates):
            skipped.append((line, "non-literal dates"))
            continue
        dt = opt.get("dtStart") or (dates[0] if dates else None)
        if not isinstance(dt, str):
            skipped.append((line, "no dtStart"))
            continue
        finite = bool(opt.get("byCount") or opt.get("until"))
        limit = None
        comment = []
        if "max" in opt:
            limit = opt["max"]
        elif not finite and not opt.get("noInstance"):
            limit = len(dates)
        if opt.get("noInstance"):
            expected = []
            comment.append("noInstance: iterator.next() must return null and iterator.completed must be true")
        else:
            expected = [conv_time(d) for d in dates]
        # preceding // comment lines in the original source
        pre = orig[:m.start()].rstrip().splitlines()
        cmts = []
        for ln in reversed(pre[-4:]):
            t = ln.strip()
            if t.startswith("//"):
                cmts.insert(0, t.lstrip("/").strip())
            else:
                break
        n += 1
        name = " > ".join(suites_at(src, m.start()) + [rule])
        if cmts:
            name += " (" + " ".join(cmts) + ")"
        comment.append("isFinite=%s per source options [source line %d]" % (finite, line))
        cases.append(vc.make_case("icaljs-%04d" % n, conv_time(dt), [rule], expected, False,
                                  name=name, limit=limit, comment="; ".join(comment)))
    vc.write(out, "icaljs", "https://github.com/kewisch/ical.js", commit, "MPL-2.0",
             "test/recur_iterator_test.js", SEM, cases)
    for s in skipped:
        print("skipped line %d: %s" % s)


if __name__ == "__main__":
    main()
