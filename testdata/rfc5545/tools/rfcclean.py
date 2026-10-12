"""Helpers to strip RFC page headers/footers while keeping original line numbers.

clean(path) -> list of (lineno, text); page breaks become a single
(lineno, PAGEBREAK) entry, with the padding blank lines around them dropped.
"""
import re

PAGEBREAK = "@@PAGEBREAK@@"
FOOTER = re.compile(r"^Desruisseaux\s+Standards Track\s+\[Page \d+\]\s*$")
HEADER = re.compile(r"^RFC 5545\s+iCalendar\s+September 2009\s*$")


def clean(path):
    with open(path, encoding="ascii") as f:
        raw = f.read().split("\n")
    out = []
    i = 0
    n = len(raw)
    while i < n:
        line = raw[i]
        if FOOTER.match(line):
            # drop trailing blank padding already emitted
            while out and out[-1][1].strip() == "":
                out.pop()
            j = i + 1
            if j < n and raw[j] == "\f":
                j += 1
            if j < n and HEADER.match(raw[j]):
                j += 1
            while j < n and raw[j].strip() == "":
                j += 1
            out.append((i + 1, PAGEBREAK))
            i = j
            continue
        out.append((i + 1, line.rstrip("\f")))
        i += 1
    return out


SECTION = re.compile(r"^(\d+(?:\.\d+)*)\.?\s{1,3}\S|^(Appendix [A-Z](?:\.\d+)*)\.?\s")


def section_of(lines):
    """Yield (lineno, text, section) tracking the current section heading."""
    sec = "0"
    started = False
    for ln, t in lines:
        m = re.match(r"^(\d+(?:\.\d+)*)\.\s+\S", t)
        if m and not t.startswith(" "):
            started = True
            sec = m.group(1)
        else:
            m2 = re.match(r"^Appendix ([A-Z])\.", t)
            if m2:
                sec = "A"
            m3 = re.match(r"^(A\.\d+)\.\s", t)
            if m3:
                sec = m3.group(1)
        yield ln, t, sec
