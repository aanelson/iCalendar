#!/bin/bash
S="${WORK:?set WORK to an empty scratch directory}/untrusted"
while read -r name url; do
  [ -z "$name" ] && continue
  mkdir -p "$S/$name"
  ( GIT_TERMINAL_PROMPT=0 git -c core.hooksPath=/dev/null clone --quiet --depth 1 --no-recurse-submodules "$url" "$S/$name/repo" 2>&1 | tail -2; echo "$name $(git -C "$S/$name/repo" rev-parse HEAD 2>/dev/null)" ) &
done <<LIST
libical https://github.com/libical/libical
icaljs https://github.com/kewisch/ical.js
python-icalendar https://github.com/collective/icalendar
recurring-ical-events https://github.com/niccokunzmann/python-recurring-ical-events
dateutil https://github.com/dateutil/dateutil
ical4j https://github.com/ical4j/ical4j
sabre-vobject https://github.com/sabre-io/vobject
icalnet https://github.com/ical-org/ical.net
golang-ical https://github.com/arran4/golang-ical
go-ical https://github.com/emersion/go-ical
rrulejs https://github.com/jkbrzt/rrule
rust-rrule https://github.com/fmeringdal/rust-rrule
LIST
wait
