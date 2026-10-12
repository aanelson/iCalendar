#!/bin/bash
S="${WORK:?set WORK to the scratch directory clone.sh used}"
U=$S/untrusted
T="$(git rev-parse --show-toplevel)/testdata/thirdparty"
mkdir -p "$T" "$S/manifests"
cd "$(dirname "$0")"
C() { python3 -I "$(dirname "$0")/copyfix.py" "$@"; }
echo "== libical"; C $U/libical/repo $T/libical/ics $S/manifests/libical.json 'test-data/**/*.ics' 'src/test/**/*.ics' 'test-data/fuzz*' 'test-data/poc-*' 'test-data/timefuzz*'
echo "== icaljs"; C $U/icaljs/repo $T/icaljs/ics $S/manifests/icaljs.json 'samples/**/*.ics' 'test/**/*.ics'
echo "== python-icalendar"; C $U/python-icalendar/repo $T/python-icalendar/ics $S/manifests/python-icalendar.json 'src/icalendar/tests/**/*.ics' 'src/icalendar/tests/**/*.jcal' 'src/icalendar/fuzzing/corpus/**/*.ics'
echo "== recurring-ical-events"; C $U/recurring-ical-events/repo $T/recurring-ical-events/ics $S/manifests/recurring-ical-events.json 'recurring_ical_events/test/calendars/**/*'
echo "== ical4j"; C $U/ical4j/repo $T/ical4j/ics $S/manifests/ical4j.json 'src/test/resources/**/*.ics' 'src/test/resources/**/*.txt@ical'
echo "== sabre-vobject"; C $U/sabre-vobject/repo $T/sabre-vobject/ics $S/manifests/sabre-vobject.json 'tests/**/*.ics'
echo "== icalnet"; C $U/icalnet/repo $T/icalnet/ics $S/manifests/icalnet.json 'Ical.Net.Tests/**/*.ics'
echo "== golang-ical"; C $U/golang-ical/repo $T/golang-ical/ics $S/manifests/golang-ical.json 'testdata/**/*.ics' 'testdata/**/*.ics_disabled'
