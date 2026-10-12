#!/usr/bin/env python3
"""Extract inline iCalendar text embedded in test source files (static text
processing only; nothing is executed).

Usage:
  python3 -I extract_inline.py php REPO_ROOT DEST_DIR MANIFEST.json GLOB [GLOB...]
  python3 -I extract_inline.py go  REPO_ROOT DEST_DIR MANIFEST.json GLOB [GLOB...]

php: heredoc/nowdoc blocks (<<<ID ... ID) that contain BEGIN:VCALENDAR and
     END:VCALENDAR. Heredocs (unquoted/double-quoted) containing '$' (variable
     interpolation) or '\\' (escape processing) are skipped so that the output is
     byte-for-byte what PHP would produce. Flexible-heredoc closing-marker
     indentation is removed as PHP does.
go:  raw string literals (`...`) containing BEGIN:VCALENDAR and END:VCALENDAR
     and no fmt verbs (%v, %s, %d); written verbatim and, when the literal is
     wrapped in toCRLF(...), additionally converted to CRLF as the test does.
Output: DEST_DIR/<relative source path>/<line>.ics
"""
import glob
import json
import os
import re
import sys


def php_blocks(text):
    for m in re.finditer(r"<<<[ \t]*(['\"]?)([A-Za-z_][A-Za-z0-9_]*)\1\r?\n", text):
        quote, ident = m.group(1), m.group(2)
        start = m.end()
        cm = re.compile(r"^([ \t]*)" + re.escape(ident) + r"\b", re.M).search(text, start)
        if not cm:
            continue
        body = text[start:cm.start()]
        indent = cm.group(1)
        lines = body.split("\n")
        if lines and lines[-1] == "":
            lines = lines[:-1]
        if indent:
            lines = [ln[len(indent):] if ln.startswith(indent) else ln.lstrip(" \t") for ln in lines]
        yield (text.count("\n", 0, m.start()) + 1, quote == "'", "\n".join(lines))


def go_blocks(text):
    for m in re.finditer(r"`([^`]*)`", text):
        pre = text[max(0, m.start() - 8):m.start()]
        yield (text.count("\n", 0, m.start()) + 1, "toCRLF(" in pre, m.group(1))


def main():
    kind, repo, dest, manifest = sys.argv[1:5]
    globs = sys.argv[5:]
    repo = os.path.realpath(repo)
    out, skipped = [], []
    for g in globs:
        for path in sorted(glob.glob(os.path.join(repo, g), recursive=True)):
            if os.path.islink(path) or not os.path.isfile(path):
                continue
            rel = os.path.relpath(path, repo)
            text = open(path, encoding="utf-8", errors="replace").read()
            blocks = php_blocks(text) if kind == "php" else go_blocks(text)
            for line, flag, body in blocks:
                if "BEGIN:VCALENDAR" not in body or "END:VCALENDAR" not in body:
                    continue
                if kind == "php":
                    nowdoc = flag
                    if not nowdoc and "$" in body:
                        skipped.append({"path": rel, "line": line, "reason": "heredoc with $ interpolation"})
                        continue
                    if not nowdoc and "\\" in body:
                        skipped.append({"path": rel, "line": line, "reason": "heredoc with backslash escapes"})
                        continue
                    data = body + "\n"
                else:
                    if re.search(r"%[-+# 0-9.]*[vsdqxXfgt]", body):
                        skipped.append({"path": rel, "line": line, "reason": "fmt template"})
                        continue
                    data = body.replace("\n", "\r\n") if flag else body
                o = os.path.join(dest, rel, "%d.ics" % line)
                os.makedirs(os.path.dirname(o), exist_ok=True)
                with open(o, "w", encoding="utf-8", newline="") as fh:
                    fh.write(data)
                out.append({"path": os.path.join(rel, "%d.ics" % line), "from": rel, "line": line, "bytes": len(data.encode())})
    json.dump({"extracted": out, "skipped": skipped}, open(manifest, "w"), indent=1)
    print("extracted %d blocks, skipped %d" % (len(out), len(skipped)))
    from collections import Counter
    print(Counter(s["reason"] for s in skipped))


if __name__ == "__main__":
    main()
