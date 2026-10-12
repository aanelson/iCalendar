#!/usr/bin/env python3
"""Parse libical-style recurrence vector files (RRULE:/DTSTART:/START-AT:/
INSTANCES: blocks separated by blank lines, '#' comments) as used by libical
(src/test/icalrecur_test*.txt) and ical.net (RecurrenceTestCases.txt, which
adds EXCEPTION:, EXCEPTION-STEP:, DURATION:).

Usage: python3 -I conv_libical_fmt.py OUT.json CONFIG.json
CONFIG = {source, upstream, commit, license, semantics, include_dtstart,
          files: [{path, file, prefix, comment}]}
"""
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import vec_common as vc  # noqa: E402

UNIMPL = "*** UNIMPLEMENTED"


def key(s):
    """Unspecified-kind comparison key: DATE -> midnight, drop Z."""
    s = s.strip().rstrip("Z")
    if len(s) == 8:
        s += "T000000"
    return s


def parse_blocks(text):
    blocks = []
    cur = None
    comments = []
    lineno = 0
    for raw in text.splitlines():
        lineno += 1
        line = raw.rstrip("\r")
        if line.strip() == "":
            if cur is not None:
                blocks.append(cur)
                cur = None
            comments = [] if cur is None else comments
            continue
        if line.startswith("#"):
            if cur is None:
                comments.append(line[1:].strip())
            continue
        if cur is None:
            cur = {"comments": comments, "line": lineno, "fields": {}}
            comments = []
        if ":" not in line:
            cur.setdefault("unparsed", []).append(line)
            continue
        k, v = line.split(":", 1)
        cur["fields"][k.strip()] = v
        if k.strip() == "RRULE":
            cur["line"] = lineno
    if cur is not None:
        blocks.append(cur)
    return blocks


def main():
    out_path, cfg_path = sys.argv[1], sys.argv[2]
    cfg = json.load(open(cfg_path))
    cases = []
    stats = {}
    for f in cfg["files"]:
        text = open(f["file"], encoding="utf-8").read()
        n = 0
        skipped = []
        for b in parse_blocks(text):
            fld = b["fields"]
            if "RRULE" not in fld:
                skipped.append((b["line"], "no RRULE"))
                continue
            if b.get("unparsed"):
                skipped.append((b["line"], "unparsed lines %r" % b["unparsed"]))
                continue
            n += 1
            cid = "%s-%04d" % (f["prefix"], n)
            rrule = fld["RRULE"]
            dtstart = fld.get("DTSTART", "").strip()
            inst = fld.get("INSTANCES")
            error = False
            comment_bits = []
            expected = []
            if "EXCEPTION" in fld:
                error = True
                step = fld.get("EXCEPTION-STEP", "Construction")
                comment_bits.append("source expects exception %s at step %s" % (fld["EXCEPTION"].strip(), step.strip()))
            if inst is not None:
                if inst.strip().startswith(UNIMPL):
                    error = True
                    comment_bits.append("source expects the iterator to be rejected: %s" % inst.strip())
                elif inst.strip():
                    expected = [x.strip() for x in inst.split(",")]
            if "START-AT" in fld:
                sa = fld["START-AT"].strip()
                comment_bits.append(f["start_at_note"].replace("{X}", sa))
            if "DURATION" in fld:
                comment_bits.append("event DURATION:%s (affects whether the end of the last instance overflows)" % fld["DURATION"].strip())
            extra = set(fld) - {"RRULE", "DTSTART", "INSTANCES", "START-AT", "EXCEPTION", "EXCEPTION-STEP", "DURATION"}
            if extra:
                comment_bits.append("unconverted fields: %s" % sorted(extra))
            if f.get("comment"):
                comment_bits.append(f["comment"])
            if str(b["line"]) in f.get("notes", {}):
                comment_bits.append(f["notes"][str(b["line"])])
            name = " / ".join(x for x in b["comments"] if x)
            kw = {}
            if "START-AT" in fld and f.get("start_at_between"):
                sa = fld["START-AT"].strip()
                kw["between"] = {"start": sa, "inclusive": True}
                # ical.net filters the expected list to >= START-AT itself
                expected = [x for x in expected if key(x) >= key(sa)]
            if not dtstart:
                dtstart = "19700101T000000"
                comment_bits.append("the source block has no DTSTART (the rule alone must be rejected); dtstart here is a placeholder")
            cases.append(vc.make_case(
                cid, dtstart, [rrule], expected, cfg["include_dtstart"],
                name=name, error=error,
                comment="; ".join(comment_bits) + ("" if not b["line"] else " [source line %d]" % b["line"]),
                **kw))
        stats[f["path"]] = {"cases": n, "skipped": skipped}
    vc.write(out_path, cfg["source"], cfg["upstream"], cfg["commit"], cfg["license"],
             ", ".join(f["path"] for f in cfg["files"]), cfg["semantics"], cases)
    print(json.dumps(stats, indent=1))


if __name__ == "__main__":
    main()
