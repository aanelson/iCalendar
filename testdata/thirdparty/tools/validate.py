#!/usr/bin/env python3
"""Validate unified recur.json files.

Usage: python3 -I validate.py FILE [FILE...]
Checks: valid JSON; top-level keys; per case required keys and types; unique
ids (within a file and across all files given); every date string is
YYYYMMDD, YYYYMMDDTHHMMSS or YYYYMMDDTHHMMSSZ; error cases have empty
expected; tzid is a non-empty string when present.
"""
import json
import re
import sys

FMT = re.compile(r"^\d{8}(T\d{6}Z?)?$")
TOP = ["source", "upstream", "commit", "license", "path", "semantics", "cases"]
REQ = ["id", "dtstart", "rrule", "include_dtstart", "expected"]
KNOWN = set(REQ) | {"name", "tzid", "exrule", "rdate", "exdate", "between",
                    "limit", "error", "extensions", "comment"}


def main():
    all_ids = {}
    bad = 0
    total = 0
    for path in sys.argv[1:]:
        errs = []
        try:
            doc = json.load(open(path, encoding="utf-8"))
        except Exception as e:  # noqa: BLE001
            print("FAIL %s: invalid JSON: %s" % (path, e))
            bad += 1
            continue
        for k in TOP:
            if k not in doc:
                errs.append("missing top-level key %r" % k)
        if not re.match(r"^[0-9a-f]{40}$", str(doc.get("commit", ""))):
            errs.append("commit is not a 40-hex SHA")
        ids = set()
        for i, c in enumerate(doc.get("cases", [])):
            where = "case[%d] %s" % (i, c.get("id"))
            for k in REQ:
                if k not in c:
                    errs.append("%s: missing %r" % (where, k))
            unk = set(c) - KNOWN
            if unk:
                errs.append("%s: unknown keys %s" % (where, sorted(unk)))
            cid = c.get("id")
            if not isinstance(cid, str) or not cid:
                errs.append("%s: bad id" % where)
            elif cid in ids:
                errs.append("%s: duplicate id" % where)
            elif cid in all_ids:
                errs.append("%s: id also used in %s" % (where, all_ids[cid]))
            ids.add(cid)
            all_ids.setdefault(cid, path)

            def chk(v, what):
                if not isinstance(v, str) or not FMT.match(v):
                    errs.append("%s: %s has bad date %r" % (where, what, v))

            chk(c.get("dtstart"), "dtstart")
            for k in ("expected", "rdate", "exdate"):
                v = c.get(k, [])
                if not isinstance(v, list):
                    errs.append("%s: %s not a list" % (where, k))
                    continue
                for x in v:
                    chk(x, k)
            for k in ("rrule", "exrule"):
                v = c.get(k, [])
                if not isinstance(v, list) or not all(isinstance(x, str) for x in v):
                    errs.append("%s: %s must be a list of strings" % (where, k))
            if not isinstance(c.get("include_dtstart"), bool):
                errs.append("%s: include_dtstart not bool" % where)
            if "tzid" in c and (not isinstance(c["tzid"], str) or not c["tzid"]):
                errs.append("%s: bad tzid" % where)
            if "between" in c:
                b = c["between"]
                if not isinstance(b, dict) or "start" not in b and "end" not in b:
                    errs.append("%s: bad between" % where)
                else:
                    for k in ("start", "end"):
                        if k in b:
                            chk(b[k], "between." + k)
                    if not isinstance(b.get("inclusive"), bool):
                        errs.append("%s: between.inclusive not bool" % where)
            if "limit" in c and (not isinstance(c["limit"], int) or c["limit"] < 0):
                errs.append("%s: bad limit" % where)
            if c.get("error") and c.get("expected"):
                errs.append("%s: error case with non-empty expected" % where)
            if "extensions" in c and not isinstance(c["extensions"], list):
                errs.append("%s: extensions not a list" % where)
        total += len(doc.get("cases", []))
        if errs:
            bad += 1
            print("FAIL %s (%d problems)" % (path, len(errs)))
            for e in errs[:40]:
                print("   ", e)
        else:
            print("ok   %s: %d cases" % (path, len(doc["cases"])))
    print("total cases: %d" % total)
    sys.exit(1 if bad else 0)


if __name__ == "__main__":
    main()
