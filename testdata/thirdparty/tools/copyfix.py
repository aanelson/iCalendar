#!/usr/bin/env python3
"""Copy fixture files from an untrusted upstream checkout into a destination
directory, preserving relative paths.

Usage:
  python3 -I copyfix.py REPO_ROOT DEST_DIR MANIFEST_JSON SPEC [SPEC...]

SPEC is  glob[@mode]  where glob is relative to REPO_ROOT (supports **),
and mode is one of:
  all       copy every matching text file (default)
  ical      copy only if the file text contains "BEGIN:V" (case-insensitive)

Files > 2 MB, files containing NUL bytes, symlinks and anything outside the
repo are skipped and recorded in the manifest.
"""
import glob
import json
import os
import shutil
import sys

MAX = 2 * 1024 * 1024


def main():
    repo, dest, manifest_path = sys.argv[1], sys.argv[2], sys.argv[3]
    specs = sys.argv[4:]
    repo = os.path.realpath(repo)
    copied, skipped = [], []
    seen = set()
    for spec in specs:
        if "@" in spec:
            pattern, mode = spec.rsplit("@", 1)
        else:
            pattern, mode = spec, "all"
        for path in sorted(glob.glob(os.path.join(repo, pattern), recursive=True)):
            rel = os.path.relpath(path, repo)
            if rel in seen:
                continue
            if "/.git/" in "/" + rel + "/":
                continue
            if os.path.islink(path):
                skipped.append({"path": rel, "reason": "symlink"})
                seen.add(rel)
                continue
            if not os.path.isfile(path):
                continue
            real = os.path.realpath(path)
            if not real.startswith(repo + os.sep):
                skipped.append({"path": rel, "reason": "outside repo"})
                seen.add(rel)
                continue
            seen.add(rel)
            size = os.path.getsize(path)
            if size > MAX:
                skipped.append({"path": rel, "reason": "larger than 2 MB (%d bytes)" % size})
                continue
            with open(path, "rb") as fh:
                data = fh.read()
            if b"\x00" in data:
                skipped.append({"path": rel, "reason": "binary (contains NUL bytes)"})
                continue
            if mode == "ical" and b"begin:v" not in data.lower():
                skipped.append({"path": rel, "reason": "no BEGIN:V... component"})
                continue
            out = os.path.join(dest, rel)
            os.makedirs(os.path.dirname(out), exist_ok=True)
            with open(out, "wb") as fh:
                fh.write(data)
            copied.append({"path": rel, "bytes": size})
    with open(manifest_path, "w") as fh:
        json.dump({"copied": copied, "skipped": skipped}, fh, indent=1)
    print("copied %d files (%d bytes), skipped %d" % (
        len(copied), sum(c["bytes"] for c in copied), len(skipped)))
    for s in skipped:
        print("  skip:", s["path"], "-", s["reason"])


if __name__ == "__main__":
    main()
