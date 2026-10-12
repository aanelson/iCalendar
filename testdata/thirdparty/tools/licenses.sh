#!/bin/bash
set -e
U="${WORK:?set WORK to the scratch directory clone.sh used}/untrusted"
T="$(git rev-parse --show-toplevel)/testdata/thirdparty"
cp() { /bin/mkdir -p "$(dirname "$2")"; /bin/cp "$1" "$2"; }
cp $U/libical/repo/LICENSE.txt $T/libical/LICENSE
cp $U/libical/repo/LICENSES/MPL-2.0.txt $T/libical/LICENSE-MPL-2.0
cp $U/libical/repo/LICENSES/LGPL-2.1-only.txt $T/libical/LICENSE-LGPL-2.1-only
cp $U/icaljs/repo/LICENSE $T/icaljs/LICENSE
cp $U/python-icalendar/repo/LICENSE.rst $T/python-icalendar/LICENSE
cp $U/recurring-ical-events/repo/LICENSE $T/recurring-ical-events/LICENSE
cp $U/dateutil/repo/LICENSE $T/dateutil/LICENSE
cp $U/ical4j/repo/LICENSE.txt $T/ical4j/LICENSE
cp $U/sabre-vobject/repo/LICENSE $T/sabre-vobject/LICENSE
cp $U/icalnet/repo/license.md $T/icalnet/LICENSE
cp $U/golang-ical/repo/LICENSE $T/golang-ical/LICENSE
cp $U/go-ical/repo/LICENSE $T/go-ical/LICENSE
cp $U/rrulejs/repo/LICENCE $T/rrulejs/LICENSE
cp $U/rust-rrule/repo/LICENSE-APACHE $T/rust-rrule/LICENSE-APACHE
cp $U/rust-rrule/repo/LICENSE-MIT $T/rust-rrule/LICENSE-MIT
echo ok
