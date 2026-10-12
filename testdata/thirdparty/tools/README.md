# Corpus tools

The scripts that built this corpus, kept so it can be refreshed against newer upstream commits.
Upstream repositories are untrusted input: the scripts only read their files (Python `ast` and
text parsing, never executing upstream code), and `clone.sh` disables git hooks and submodules.

```sh
export WORK=$(mktemp -d)        # clones go in $WORK/untrusted/<name>/repo
./clone.sh                      # shallow clones; prints each commit
./copyall.sh                    # copies fixtures into ../<name>/ics (via copyfix.py)
./licenses.sh                   # copies each upstream license
python3 -I conv_dateutil.py …   # each conv_*.py writes ../<name>/recur.json; see its header
python3 -I validate.py ..       # checks every recur.json against the format in ../README.md
```

`vec_common.py` holds the shared recur.json writer, `jslit.py` a small JavaScript-literal reader
for the ical.js and rrule.js tests, and `extract_inline.py` pulls calendars out of test sources.
Update the commit SHAs in `../README.md` and each `PROVENANCE.md` after a refresh.
