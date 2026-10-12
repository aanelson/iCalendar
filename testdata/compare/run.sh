#!/bin/sh
# Re-run the whole comparison (about 15 minutes; the hostile-input probes
# run one at a time with 60 s timeouts so their timings are clean).
#
#   ./run.sh                 everything: probes, CPU profiles, corpus
#   ./run.sh -only '^6\.'    a subset of probes by ID regexp (no corpus)
#
# Every probe runs in its own subprocess with a timeout, so a hang or a
# fatal crash in one library is recorded as such and the run continues.
# Raw results: logs/results*.jsonl, logs/corpus.jsonl.
# Summaries:   logs/matrix*.md, logs/corpus.md, logs/loadlocation-share.txt.
set -e
cd "$(dirname "$0")"
export GOPROXY=off GOFLAGS=-mod=mod
mkdir -p logs bin
command go build -o bin/probe ./probe
command go build -race -o bin/probe-race ./probe
command go build -o bin/driver ./driver
./bin/driver "$@" > logs/driver-stdout.txt 2> logs/driver-progress.txt || true
case "$*" in *-only*) exit 0;; esac

# Share of CPU in time.LoadLocation while reading DTSTART/DTEND of 20,000
# zoned events (the "LoadLocation on every property" claim).
: > logs/loadlocation-share.txt
for lib in this golang-ical+opts emersion; do
	PROBE_CPUPROFILE=logs/cpu-$lib.pprof ./bin/probe run 3.08-throughput "$lib" parse+times > /dev/null
	{
		echo "== $lib"
		command go tool pprof -top -cum bin/probe logs/cpu-$lib.pprof 2>/dev/null |
			grep -E "Duration:|Showing|flat%|time.LoadLocation|tz.Load|loadTzinfo|readFile" | head -12
	} >> logs/loadlocation-share.txt
done

./bin/driver -corpus > logs/corpus-stdout.txt 2> logs/corpus-progress.txt
echo "see logs/matrix.md, logs/corpus.md, logs/loadlocation-share.txt"
