#!/usr/bin/env python3
"""Extract $this->parse(rule, start, expected, fastForward, tz, runTillTheEnd)
cases (including #[DataProvider] data) from sabre/vobject
tests/VObject/Recur/RRuleIteratorTest.php - static text parsing only.

Usage: python3 -I conv_sabre.py TESTFILE OUT.json COMMIT
"""
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import jslit  # noqa: E402
import vec_common as vc  # noqa: E402

PARAMS = ["rule", "start", "expected", "fastForward", "tz", "runTillTheEnd"]
DEFAULTS = {"fastForward": None, "tz": "UTC", "runTillTheEnd": False}

SEM = (
    "sabre/vobject tests/VObject/Recur/RRuleIteratorTest.php, helper parse($rule, $start, $expected, $fastForward=null, "
    "$tz='UTC', $runTillTheEnd=false): new RRuleIterator($rule, new DateTime($start, new DateTimeZone($tz))); optional "
    "fastForward(new DateTime($fastForward)) (PHP default zone is UTC in the test bootstrap); then collects "
    "current()->format('Y-m-d H:i:s') while valid(); for infinite rules (no COUNT/UNTIL) it stops once as many values "
    "as expected were collected ('limit'), unless runTillTheEnd. RRuleIterator::rewind() sets the current date to "
    "DTSTART, so DTSTART is ALWAYS the first instance even when it does not match the rule, and it counts toward COUNT "
    "(include_dtstart=true, RFC 5545 semantics). Values are wall-clock times in $tz: tzid is set for non-UTC zones; "
    "for the default 'UTC' zone dtstart/expected carry a Z suffix. A date-only $start means midnight. fastForward(dt) "
    "skips instances < dt, so it is recorded as between.start (inclusive, no end; UTC because the PHP default zone is "
    "UTC). error=true: the test expects InvalidDataException. Several cases exercise DST gaps (Europe/Zurich, "
    "America/New_York): sabre keeps the nominal wall-clock sequence and moves non-existent local times forward by the "
    "gap; these expectations are sabre's behaviour where RFC 5545 is not explicit.")


def find_functions(src):
    """Yield (name, params_text, body_start, body_end, attr_text) for every method."""
    for m in re.finditer(r"((?:#\[[^\]]*\]\s*)*)(?:public|protected|private)\s+(static\s+)?function\s+(\w+)\s*\(([^)]*)\)\s*(?::\s*[\w?\\]+)?\s*\{", src):
        start = m.end() - 1
        depth = 0
        i = start
        while i < len(src):
            c = src[i]
            if c in "'\"":
                q = c
                i += 1
                while i < len(src) and src[i] != q:
                    if src[i] == "\\":
                        i += 1
                    i += 1
            elif c == "{":
                depth += 1
            elif c == "}":
                depth -= 1
                if depth == 0:
                    break
            i += 1
        yield m.group(3), m.group(4), start, i, m.group(1)


def assoc_to_dict(v):
    if isinstance(v, tuple) and v[0] == "assoc":
        return {k: x for k, x in v[1]}
    return None


def provider_rows(src, body_start, body_end):
    """Return list of (label, row) where row is dict or list."""
    body = src[body_start:body_end]
    rows = []
    for m in re.finditer(r"\byield\b", body):
        p = jslit.Parser(src, body_start + m.end(), php=True)
        first = p.expr()
        if p.peek(2) == "=>":
            p.eat("=>")
            row = p.expr()
            label = first
        else:
            row, label = first, None
        rows.append((label, row))
    if not rows:
        m = re.search(r"\breturn\b", body)
        if m:
            p = jslit.Parser(src, body_start + m.end(), php=True)
            arr = p.expr()
            if isinstance(arr, list):
                rows = [(None, r) for r in arr]
            elif isinstance(arr, tuple) and arr[0] == "assoc":
                rows = list(arr[1])
    return rows


def php_dt(s):
    return vc.iso_to_basic(s)


def main():
    path, out, commit = sys.argv[1], sys.argv[2], sys.argv[3]
    orig = open(path, encoding="utf-8").read()
    src = jslit.blank_comments(orig, php=True)
    funcs = list(find_functions(src))
    by_name = {f[0]: f for f in funcs}
    cases = []
    skipped = []
    n = 0
    for name, params, bs, be, attrs in funcs:
        if not name.startswith("test"):
            continue
        body = src[bs:be]
        calls = [m for m in re.finditer(r"\$this->parse\(", body)]
        if not calls:
            skipped.append((name, "does not use the parse() helper"))
            continue
        expect_exc = "expectException(" in body
        pnames = re.findall(r"\$(\w+)", params)
        prov = re.search(r"DataProvider\(\s*'(\w+)'\s*\)", attrs or "")
        rows = [(None, None)]
        if prov:
            pf = by_name.get(prov.group(1))
            if not pf:
                skipped.append((name, "provider not found"))
                continue
            rows = provider_rows(src, pf[2], pf[3])
        for call in calls:
            p = jslit.Parser(src, bs + call.end() - 1, php=True)
            try:
                args = p.args()
            except jslit.ParseError as e:
                skipped.append((name, "parse error %s" % e))
                continue
            for label, row in rows:
                bind = {}
                if row is not None:
                    d = assoc_to_dict(row)
                    if d is not None:
                        bind = {("$" + k): v for k, v in d.items()}
                    elif isinstance(row, list):
                        bind = {("$" + k): v for k, v in zip(pnames, row)}
                    else:
                        skipped.append((name, "provider row shape"))
                        continue
                vals = dict(DEFAULTS)
                ok = True
                pos = 0
                for a in args:
                    if isinstance(a, tuple) and a[0] == "named":
                        key, v = a[1], a[2]
                    else:
                        key, v = PARAMS[pos], a
                        pos += 1
                    if isinstance(v, tuple) and v[0] == "ident":
                        if v[1] not in bind:
                            ok = False
                            skipped.append((name, "unbound variable %s" % v[1]))
                            break
                        v = bind[v[1]]
                    vals[key] = v
                if not ok:
                    continue
                rule, start, expected = vals.get("rule"), vals.get("start"), vals.get("expected")
                if not isinstance(rule, str) or not isinstance(start, str) or not isinstance(expected, list) \
                        or not all(isinstance(x, str) for x in expected):
                    skipped.append((name, "non-literal arguments (rule=%r)" % (rule,)))
                    continue
                tz = vals["tz"]
                utc = tz == "UTC"
                ff = vals["fastForward"]
                run_till_end = bool(vals["runTillTheEnd"])

                def conv(s, z=utc):
                    b = php_dt(s)
                    if len(b) == 8:
                        b += "T000000"
                    return b + ("Z" if z and not b.endswith("Z") else "")

                parts = vc.rule_parts(rule)
                infinite = "COUNT" not in parts and "UNTIL" not in parts
                limit = None
                comment = []
                if infinite and not run_till_end and not expect_exc:
                    if expected:
                        limit = len(expected)
                    else:
                        comment.append("infinite rule but the iterator yields nothing (after fastForward): it must give up on its own rather than loop forever")
                if run_till_end:
                    comment.append("runTillTheEnd=true: the iterator must terminate on its own after these instances")
                between = None
                if ff:
                    between = {"start": conv(ff, True), "inclusive": True}
                    comment.append("fastForward(%s UTC)" % ff)
                if len(php_dt(start)) == 8:
                    comment.append("date-only start '%s' = midnight" % start)
                n += 1
                nm = name + (" [%s]" % label if label else "")
                line = orig.count("\n", 0, bs) + 1
                comment.append("source line %d" % line)
                if expect_exc:
                    comment.insert(0, "source expects InvalidDataException")
                cases.append(vc.make_case(
                    "sabre-%04d" % n, conv(start), [rule], [] if expect_exc else [conv(x) for x in expected], True,
                    name=nm, tzid=None if utc else tz, between=between, limit=limit, error=expect_exc,
                    comment="; ".join(comment)))
    vc.write(out, "sabre-vobject", "https://github.com/sabre-io/vobject", commit, "BSD-3-Clause",
             "tests/VObject/Recur/RRuleIteratorTest.php", SEM, cases)
    for s in skipped:
        print("skipped %s: %s" % s)


if __name__ == "__main__":
    main()
