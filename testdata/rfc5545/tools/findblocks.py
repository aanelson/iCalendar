"""List candidate indented example blocks in RFC 5545 for manual review.

Usage: python3 -I findblocks.py <rfc5545.txt> <out.txt>
A block = maximal run of non-blank lines indented >= 5 spaces deeper than the
prose (prose is 3 or 6 spaces) -- i.e. indent >= 7, or indent 5 lines that
are followed by continuation lines. Page-break markers inside a run are kept.
"""
import sys
import os

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from rfcclean import clean, section_of, PAGEBREAK  # noqa: E402


def indent(t):
    return len(t) - len(t.lstrip(" "))


def main():
    lines = list(section_of(clean(sys.argv[1])))
    # skip table of contents region
    blocks = []
    cur = []
    for ln, t, sec in lines:
        if ln < 230:
            continue
        code = t.strip() != "" and t != PAGEBREAK and indent(t) >= 7
        code5 = t.strip() != "" and t != PAGEBREAK and indent(t) == 5
        if code or code5 or (t == PAGEBREAK and cur):
            cur.append((ln, t, sec))
        else:
            if cur:
                blocks.append(cur)
            cur = []
    if cur:
        blocks.append(cur)
    with open(sys.argv[2], "w") as f:
        for b in blocks:
            while b and b[-1][1] == PAGEBREAK:
                b.pop()
            if not b:
                continue
            f.write("=== sec %s lines %d-%d\n" % (b[0][2], b[0][0], b[-1][0]))
            for ln, t, sec in b:
                f.write("%5d|%s\n" % (ln, t))
    print(len(blocks))


main()
