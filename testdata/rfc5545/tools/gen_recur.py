"""Transcribe the RFC 5545 section 3.8.5.3 recurrence examples into recur.json.

The expected instance lists below are hand transcriptions of the RFC's prose
("==> (1997 9:00 AM EDT) September 2-11"), with every range and ellipsis
expanded explicitly.  The RFC's EDT/EST labels are kept next to each group and
checked against zoneinfo as a transcription sanity check.  Independent
verification is done separately by check_recur.py.

Usage: python3 -I gen_recur.py <out.json>
"""
import json
import sys
from datetime import datetime
from zoneinfo import ZoneInfo

NY = ZoneInfo("America/New_York")


def R(a, b, step=1):
    return list(range(a, b + 1, step))


def days(label, year, month, ds, t="090000"):
    """A prose group: label (EDT/EST), year, month, list of days."""
    return [(label, "%04d%02d%02dT%s" % (year, month, d, t)) for d in ds]


def times(label, year, month, day, ts):
    return [(label, "%04d%02d%02dT%s" % (year, month, day, t)) for t in ts]


def flatten(*groups):
    out = []
    for g in groups:
        out.extend(g)
    return out


NYC = "America/New_York"

SEMANTICS = (
    "RFC 5545 section 3.8.5.3 semantics. DTSTART is always the first instance and counts "
    "toward COUNT (include_dtstart=true), even if it does not match the RRULE; RFC 5545 "
    "calls such a recurrence set undefined, and the only example here where DTSTART does "
    "not match (case 28, Friday the 13th) removes it with EXDATE. Instances from RRULE and "
    "RDATE are unioned (duplicates collapse), then EXDATE values are removed; EXDATE does "
    "not change how COUNT is applied. UNTIL is inclusive. A UTC UNTIL is compared with "
    "each instance's absolute time, computed in tzid. When tzid is set, dtstart, rdate, "
    "exdate, between and expected are wall-clock local times in that zone, so 9:00 AM EDT "
    "and 9:00 AM EST are both T090000. When tzid is empty, values are floating, or UTC if "
    "they end in Z. Generated dates that do not exist (e.g. February 30) are skipped and "
    "not counted. limit=N: compare only the first N instances of the final set (used for "
    "unbounded rules; N is how many instances the RFC enumerates). limit=0: compare the "
    "complete set. between=null: no window. Each case has exactly one RRULE. Where the "
    "RFC gives two alternative RRULEs joined by 'or', they are separate cases with a/b "
    "id suffixes and the same expected list."
)

cases = []


def case(num, name, dtstart, rrule, groups, comment, limit=0, exdate=None, rdate=None,
         suffix=""):
    expected = [v for (_, v) in groups]
    # transcription sanity check: RFC zone labels agree with tzdata
    for label, v in groups:
        dt = datetime.strptime(v, "%Y%m%dT%H%M%S").replace(tzinfo=NY)
        if dt.tzname() != label:
            raise SystemExit("label mismatch %s %s %s" % (num, v, label))
    if limit:
        assert limit == len(expected), (num, limit, len(expected))
    cases.append({
        "id": "rfc5545-3.8.5.3-%02d%s" % (num, suffix),
        "name": name,
        "dtstart": dtstart,
        "tzid": NYC,
        "rrule": [rrule],
        "exrule": [],
        "rdate": rdate or [],
        "exdate": exdate or [],
        "include_dtstart": True,
        "between": None,
        "limit": limit,
        "expected": expected,
        "error": False,
        "extensions": [],
        "comment": comment,
    })


UNBOUNDED = ("Unbounded rule: limit is the number of instances the RFC enumerates. "
             "Comparison is by limit, not by a between window.")

case(1, "Daily for 10 occurrences", "19970902T090000", "FREQ=DAILY;COUNT=10",
     days("EDT", 1997, 9, R(2, 11)),
     "RFC prose: ==> (1997 9:00 AM EDT) September 2-11")

case(2, "Daily until December 24, 1997", "19970902T090000",
     "FREQ=DAILY;UNTIL=19971224T000000Z",
     flatten(days("EDT", 1997, 9, R(2, 30)), days("EDT", 1997, 10, R(1, 25)),
             days("EST", 1997, 10, R(26, 31)), days("EST", 1997, 11, R(1, 30)),
             days("EST", 1997, 12, R(1, 23))),
     "RFC prose: ==> (1997 9:00 AM EDT) September 2-30;October 1-25 (1997 9:00 AM EST) "
     "October 26-31;November 1-30;December 1-23. UNTIL 19971224T000000Z is 1997-12-23 "
     "19:00 EST, so December 24 is excluded. Erratum 6212 proposed changing this and "
     "was rejected; the example is unchanged.")

case(3, "Every other day - forever", "19970902T090000", "FREQ=DAILY;INTERVAL=2",
     flatten(days("EDT", 1997, 9, R(2, 30, 2)), days("EDT", 1997, 10, R(2, 24, 2)),
             days("EST", 1997, 10, [26, 28, 30]), days("EST", 1997, 11, R(1, 29, 2)),
             days("EST", 1997, 12, [1, 3])),
     "RFC prose: ==> (1997 9:00 AM EDT) September 2,4,6,8...24,26,28,30; October "
     "2,4,6...20,22,24 (1997 9:00 AM EST) October 26,28,30; November 1,3,5,7...25,27,29; "
     "December 1,3,... . The ellipses inside each month are expanded; the trailing "
     "'December 1,3,...' ends the enumeration. " + UNBOUNDED,
     limit=47)

case(4, "Every 10 days, 5 occurrences", "19970902T090000", "FREQ=DAILY;INTERVAL=10;COUNT=5",
     flatten(days("EDT", 1997, 9, [2, 12, 22]), days("EDT", 1997, 10, [2, 12])),
     "RFC prose: ==> (1997 9:00 AM EDT) September 2,12,22; October 2,12")

jan3 = flatten(days("EST", 1998, 1, R(1, 31)), days("EST", 1999, 1, R(1, 31)),
               days("EST", 2000, 1, R(1, 31)))
JAN_PROSE = ("RFC prose: ==> (1998 9:00 AM EST)January 1-31 (1999 9:00 AM EST)January 1-31 "
             "(2000 9:00 AM EST)January 1-31. UNTIL 20000131T140000Z is 2000-01-31 09:00 EST, "
             "which is the last instance (UNTIL is inclusive).")
case(5, "Every day in January, for 3 years (YEARLY form)", "19980101T090000",
     "FREQ=YEARLY;UNTIL=20000131T140000Z;BYMONTH=1;BYDAY=SU,MO,TU,WE,TH,FR,SA", jan3,
     JAN_PROSE + " First of the two RRULEs the RFC gives as alternatives (the RFC folds "
     "it across two lines; unfolded here). The other is case 05b.", suffix="a")
case(5, "Every day in January, for 3 years (DAILY form)", "19980101T090000",
     "FREQ=DAILY;UNTIL=20000131T140000Z;BYMONTH=1", jan3,
     JAN_PROSE + " Second of the two RRULEs the RFC gives as alternatives ('or'); "
     "the other is case 05a.", suffix="b")

case(6, "Weekly for 10 occurrences", "19970902T090000", "FREQ=WEEKLY;COUNT=10",
     flatten(days("EDT", 1997, 9, [2, 9, 16, 23, 30]), days("EDT", 1997, 10, [7, 14, 21]),
             days("EST", 1997, 10, [28]), days("EST", 1997, 11, [4])),
     "RFC prose: ==> (1997 9:00 AM EDT) September 2,9,16,23,30;October 7,14,21 "
     "(1997 9:00 AM EST) October 28;November 4")

case(7, "Weekly until December 24, 1997", "19970902T090000",
     "FREQ=WEEKLY;UNTIL=19971224T000000Z",
     flatten(days("EDT", 1997, 9, [2, 9, 16, 23, 30]), days("EDT", 1997, 10, [7, 14, 21]),
             days("EST", 1997, 10, [28]), days("EST", 1997, 11, [4, 11, 18, 25]),
             days("EST", 1997, 12, [2, 9, 16, 23])),
     "RFC prose: ==> (1997 9:00 AM EDT) September 2,9,16,23,30; October 7,14,21 "
     "(1997 9:00 AM EST) October 28; November 4,11,18,25; December 2,9,16,23")

case(8, "Every other week - forever", "19970902T090000", "FREQ=WEEKLY;INTERVAL=2;WKST=SU",
     flatten(days("EDT", 1997, 9, [2, 16, 30]), days("EDT", 1997, 10, [14]),
             days("EST", 1997, 10, [28]), days("EST", 1997, 11, [11, 25]),
             days("EST", 1997, 12, [9, 23]), days("EST", 1998, 1, [6, 20]),
             days("EST", 1998, 2, [3, 17])),
     "RFC prose: ==> (1997 9:00 AM EDT) September 2,16,30; October 14 (1997 9:00 AM EST) "
     "October 28; November 11,25; December 9,23 (1998 9:00 AM EST) January 6,20; "
     "February 3, 17 ... . " + UNBOUNDED,
     limit=13)

tt5 = flatten(days("EDT", 1997, 9, [2, 4, 9, 11, 16, 18, 23, 25, 30]),
              days("EDT", 1997, 10, [2]))
TT5_PROSE = ("RFC prose: ==> (1997 9:00 AM EDT) September 2,4,9,11,16,18,23,25,30; October 2.")
case(9, "Weekly on Tuesday and Thursday for five weeks (UNTIL form)", "19970902T090000",
     "FREQ=WEEKLY;UNTIL=19971007T000000Z;WKST=SU;BYDAY=TU,TH", tt5,
     TT5_PROSE + " UNTIL 19971007T000000Z is 1997-10-06 20:00 EDT, so Tuesday "
     "October 7 at 09:00 is excluded. Erratum 5872 proposed changing UNTIL and was "
     "rejected. Alternative form; see case 09b.", suffix="a")
case(9, "Weekly on Tuesday and Thursday for five weeks (COUNT form)", "19970902T090000",
     "FREQ=WEEKLY;COUNT=10;WKST=SU;BYDAY=TU,TH", tt5,
     TT5_PROSE + " Alternative RRULE given by the RFC ('or'); see case 09a.", suffix="b")

case(10, "Every other week on Monday, Wednesday, and Friday until December 24, 1997, "
         "starting on Monday, September 1, 1997", "19970901T090000",
     "FREQ=WEEKLY;INTERVAL=2;UNTIL=19971224T000000Z;WKST=SU;BYDAY=MO,WE,FR",
     flatten(days("EDT", 1997, 9, [1, 3, 5, 15, 17, 19, 29]),
             days("EDT", 1997, 10, [1, 3, 13, 15, 17]),
             days("EST", 1997, 10, [27, 29, 31]),
             days("EST", 1997, 11, [10, 12, 14, 24, 26, 28]),
             days("EST", 1997, 12, [8, 10, 12, 22])),
     "RFC prose: ==> (1997 9:00 AM EDT) September 1,3,5,15,17,19,29; October 1,3,13,15,17 "
     "(1997 9:00 AM EST) October 27,29,31; November 10,12,14,24,26,28; December 8,10,12,22. "
     "The RFC folds the RRULE across two lines; unfolded here.")

case(11, "Every other week on Tuesday and Thursday, for 8 occurrences", "19970902T090000",
     "FREQ=WEEKLY;INTERVAL=2;COUNT=8;WKST=SU;BYDAY=TU,TH",
     flatten(days("EDT", 1997, 9, [2, 4, 16, 18, 30]), days("EDT", 1997, 10, [2, 14, 16])),
     "RFC prose: ==> (1997 9:00 AM EDT) September 2,4,16,18,30; October 2,14,16")

case(12, "Monthly on the first Friday for 10 occurrences", "19970905T090000",
     "FREQ=MONTHLY;COUNT=10;BYDAY=1FR",
     flatten(days("EDT", 1997, 9, [5]), days("EDT", 1997, 10, [3]),
             days("EST", 1997, 11, [7]), days("EST", 1997, 12, [5]),
             days("EST", 1998, 1, [2]), days("EST", 1998, 2, [6]), days("EST", 1998, 3, [6]),
             days("EST", 1998, 4, [3]), days("EDT", 1998, 5, [1]), days("EDT", 1998, 6, [5])),
     "RFC prose: ==> (1997 9:00 AM EDT) September 5;October 3 (1997 9:00 AM EST) November "
     "7;December 5 (1998 9:00 AM EST) January 2;February 6;March 6;April 3 (1998 9:00 AM "
     "EDT) May 1;June 5. Note: the RFC labels April 3, 1998 as EST. That is correct, because "
     "US DST began on 1998-04-05.")

case(13, "Monthly on the first Friday until December 24, 1997", "19970905T090000",
     "FREQ=MONTHLY;UNTIL=19971224T000000Z;BYDAY=1FR",
     flatten(days("EDT", 1997, 9, [5]), days("EDT", 1997, 10, [3]),
             days("EST", 1997, 11, [7]), days("EST", 1997, 12, [5])),
     "RFC prose: ==> (1997 9:00 AM EDT) September 5; October 3 (1997 9:00 AM EST) "
     "November 7; December 5")

case(14, "Every other month on the first and last Sunday of the month for 10 occurrences",
     "19970907T090000", "FREQ=MONTHLY;INTERVAL=2;COUNT=10;BYDAY=1SU,-1SU",
     flatten(days("EDT", 1997, 9, [7, 28]), days("EST", 1997, 11, [2, 30]),
             days("EST", 1998, 1, [4, 25]), days("EST", 1998, 3, [1, 29]),
             days("EDT", 1998, 5, [3, 31])),
     "RFC prose: ==> (1997 9:00 AM EDT) September 7,28 (1997 9:00 AM EST) November 2,30 "
     "(1998 9:00 AM EST) January 4,25;March 1,29 (1998 9:00 AM EDT) May 3,31")

case(15, "Monthly on the second-to-last Monday of the month for 6 months",
     "19970922T090000", "FREQ=MONTHLY;COUNT=6;BYDAY=-2MO",
     flatten(days("EDT", 1997, 9, [22]), days("EDT", 1997, 10, [20]),
             days("EST", 1997, 11, [17]), days("EST", 1997, 12, [22]),
             days("EST", 1998, 1, [19]), days("EST", 1998, 2, [16])),
     "RFC prose: ==> (1997 9:00 AM EDT) September 22;October 20 (1997 9:00 AM EST) "
     "November 17;December 22 (1998 9:00 AM EST) January 19;February 16")

case(16, "Monthly on the third-to-the-last day of the month, forever", "19970928T090000",
     "FREQ=MONTHLY;BYMONTHDAY=-3",
     flatten(days("EDT", 1997, 9, [28]), days("EST", 1997, 10, [29]),
             days("EST", 1997, 11, [28]), days("EST", 1997, 12, [29]),
             days("EST", 1998, 1, [29]), days("EST", 1998, 2, [26])),
     "RFC prose: ==> (1997 9:00 AM EDT) September 28 (1997 9:00 AM EST) October "
     "29;November 28;December 29 (1998 9:00 AM EST) January 29;February 26 ... . "
     + UNBOUNDED,
     limit=6)

case(17, "Monthly on the 2nd and 15th of the month for 10 occurrences", "19970902T090000",
     "FREQ=MONTHLY;COUNT=10;BYMONTHDAY=2,15",
     flatten(days("EDT", 1997, 9, [2, 15]), days("EDT", 1997, 10, [2, 15]),
             days("EST", 1997, 11, [2, 15]), days("EST", 1997, 12, [2, 15]),
             days("EST", 1998, 1, [2, 15])),
     "RFC prose: ==> (1997 9:00 AM EDT) September 2,15;October 2,15 (1997 9:00 AM EST) "
     "November 2,15;December 2,15 (1998 9:00 AM EST) January 2,15")

case(18, "Monthly on the first and last day of the month for 10 occurrences",
     "19970930T090000", "FREQ=MONTHLY;COUNT=10;BYMONTHDAY=1,-1",
     flatten(days("EDT", 1997, 9, [30]), days("EDT", 1997, 10, [1]),
             days("EST", 1997, 10, [31]), days("EST", 1997, 11, [1, 30]),
             days("EST", 1997, 12, [1, 31]), days("EST", 1998, 1, [1, 31]),
             days("EST", 1998, 2, [1])),
     "RFC prose: ==> (1997 9:00 AM EDT) September 30;October 1 (1997 9:00 AM EST) "
     "October 31;November 1,30;December 1,31 (1998 9:00 AM EST) January 1,31;February 1")

case(19, "Every 18 months on the 10th thru 15th of the month for 10 occurrences",
     "19970910T090000", "FREQ=MONTHLY;INTERVAL=18;COUNT=10;BYMONTHDAY=10,11,12,13,14,15",
     flatten(days("EDT", 1997, 9, R(10, 15)), days("EST", 1999, 3, [10, 11, 12, 13])),
     "RFC prose: ==> (1997 9:00 AM EDT) September 10,11,12,13,14,15 (1999 9:00 AM EST) "
     "March 10,11,12,13. The RFC folds the RRULE across two lines; unfolded here.")

case(20, "Every Tuesday, every other month", "19970902T090000",
     "FREQ=MONTHLY;INTERVAL=2;BYDAY=TU",
     flatten(days("EDT", 1997, 9, [2, 9, 16, 23, 30]), days("EST", 1997, 11, [4, 11, 18, 25]),
             days("EST", 1998, 1, [6, 13, 20, 27]), days("EST", 1998, 3, [3, 10, 17, 24, 31])),
     "RFC prose: ==> (1997 9:00 AM EDT) September 2,9,16,23,30 (1997 9:00 AM EST) November "
     "4,11,18,25 (1998 9:00 AM EST) January 6,13,20,27;March 3,10,17,24,31 ... . "
     + UNBOUNDED,
     limit=18)

case(21, "Yearly in June and July for 10 occurrences", "19970610T090000",
     "FREQ=YEARLY;COUNT=10;BYMONTH=6,7",
     flatten(*[days("EDT", y, m, [10]) for y in R(1997, 2001) for m in (6, 7)]),
     "RFC prose: ==> (1997 9:00 AM EDT) June 10;July 10 (1998 ...) ... (2001 9:00 AM EDT) "
     "June 10;July 10. The day of month comes from DTSTART, as the RFC's note says.")

case(22, "Every other year on January, February, and March for 10 occurrences",
     "19970310T090000", "FREQ=YEARLY;INTERVAL=2;COUNT=10;BYMONTH=1,2,3",
     flatten(days("EST", 1997, 3, [10]),
             *[days("EST", y, m, [10]) for y in (1999, 2001, 2003) for m in (1, 2, 3)]),
     "RFC prose: ==> (1997 9:00 AM EST) March 10 (1999 9:00 AM EST) January 10;February "
     "10;March 10 (2001 ...) (2003 9:00 AM EST) January 10;February 10;March 10")

case(23, "Every third year on the 1st, 100th, and 200th day for 10 occurrences",
     "19970101T090000", "FREQ=YEARLY;INTERVAL=3;COUNT=10;BYYEARDAY=1,100,200",
     flatten(days("EST", 1997, 1, [1]), days("EDT", 1997, 4, [10]), days("EDT", 1997, 7, [19]),
             days("EST", 2000, 1, [1]), days("EDT", 2000, 4, [9]), days("EDT", 2000, 7, [18]),
             days("EST", 2003, 1, [1]), days("EDT", 2003, 4, [10]), days("EDT", 2003, 7, [19]),
             days("EST", 2006, 1, [1])),
     "RFC prose: ==> (1997 9:00 AM EST) January 1 (1997 9:00 AM EDT) April 10;July 19 "
     "(2000 9:00 AM EST) January 1 (2000 9:00 AM EDT) April 9;July 18 (2003 9:00 AM EST) "
     "January 1 (2003 9:00 AM EDT) April 10;July 19 (2006 9:00 AM EST) January 1")

case(24, "Every 20th Monday of the year, forever", "19970519T090000", "FREQ=YEARLY;BYDAY=20MO",
     flatten(days("EDT", 1997, 5, [19]), days("EDT", 1998, 5, [18]), days("EDT", 1999, 5, [17])),
     "RFC prose: ==> (1997 9:00 AM EDT) May 19 (1998 9:00 AM EDT) May 18 (1999 9:00 AM "
     "EDT) May 17 ... . " + UNBOUNDED,
     limit=3)

case(25, "Monday of week number 20 (where the default start of the week is Monday), forever",
     "19970512T090000", "FREQ=YEARLY;BYWEEKNO=20;BYDAY=MO",
     flatten(days("EDT", 1997, 5, [12]), days("EDT", 1998, 5, [11]), days("EDT", 1999, 5, [17])),
     "RFC prose: ==> (1997 9:00 AM EDT) May 12 (1998 9:00 AM EDT) May 11 (1999 9:00 AM "
     "EDT) May 17 ... . " + UNBOUNDED,
     limit=3)

case(26, "Every Thursday in March, forever", "19970313T090000",
     "FREQ=YEARLY;BYMONTH=3;BYDAY=TH",
     flatten(days("EST", 1997, 3, [13, 20, 27]), days("EST", 1998, 3, [5, 12, 19, 26]),
             days("EST", 1999, 3, [4, 11, 18, 25])),
     "RFC prose: ==> (1997 9:00 AM EST) March 13,20,27 (1998 9:00 AM EST) March 5,12,19,26 "
     "(1999 9:00 AM EST) March 4,11,18,25 ... . " + UNBOUNDED,
     limit=11)

case(27, "Every Thursday, but only during June, July, and August, forever",
     "19970605T090000", "FREQ=YEARLY;BYDAY=TH;BYMONTH=6,7,8",
     flatten(days("EDT", 1997, 6, [5, 12, 19, 26]), days("EDT", 1997, 7, [3, 10, 17, 24, 31]),
             days("EDT", 1997, 8, [7, 14, 21, 28]),
             days("EDT", 1998, 6, [4, 11, 18, 25]), days("EDT", 1998, 7, [2, 9, 16, 23, 30]),
             days("EDT", 1998, 8, [6, 13, 20, 27]),
             days("EDT", 1999, 6, [3, 10, 17, 24]), days("EDT", 1999, 7, [1, 8, 15, 22, 29]),
             days("EDT", 1999, 8, [5, 12, 19, 26])),
     "RFC prose: ==> (1997 9:00 AM EDT) June 5,12,19,26;July 3,10,17,24,31; August "
     "7,14,21,28 (1998 9:00 AM EDT) June 4,11,18,25;July 2,9,16,23,30; August 6,13,20,27 "
     "(1999 9:00 AM EDT) June 3,10,17,24;July 1,8,15,22,29; August 5,12,19,26 ... . "
     + UNBOUNDED,
     limit=39)

case(28, "Every Friday the 13th, forever", "19970902T090000",
     "FREQ=MONTHLY;BYDAY=FR;BYMONTHDAY=13",
     flatten(days("EST", 1998, 2, [13]), days("EST", 1998, 3, [13]), days("EST", 1998, 11, [13]),
             days("EDT", 1999, 8, [13]), days("EDT", 2000, 10, [13])),
     "RFC prose: ==> (1998 9:00 AM EST) February 13;March 13;November 13 (1999 9:00 AM "
     "EDT) August 13 (2000 9:00 AM EDT) October 13 ... . DTSTART (Tuesday 1997-09-02) "
     "does not match the rule. It is still the first instance (include_dtstart=true), and "
     "then the RFC's EXDATE removes it, so the RFC's stated output starts at 1998-02-13. "
     "Erratum 5920 proposed moving DTSTART and was rejected; the example is unchanged. "
     + UNBOUNDED,
     limit=5, exdate=["19970902T090000"])

case(29, "The first Saturday that follows the first Sunday of the month, forever",
     "19970913T090000", "FREQ=MONTHLY;BYDAY=SA;BYMONTHDAY=7,8,9,10,11,12,13",
     flatten(days("EDT", 1997, 9, [13]), days("EDT", 1997, 10, [11]),
             days("EST", 1997, 11, [8]), days("EST", 1997, 12, [13]),
             days("EST", 1998, 1, [10]), days("EST", 1998, 2, [7]), days("EST", 1998, 3, [7]),
             days("EDT", 1998, 4, [11]), days("EDT", 1998, 5, [9]), days("EDT", 1998, 6, [13])),
     "RFC prose: ==> (1997 9:00 AM EDT) September 13;October 11 (1997 9:00 AM EST) November "
     "8;December 13 (1998 9:00 AM EST) January 10;February 7;March 7 (1998 9:00 AM EDT) "
     "April 11;May 9;June 13... ... . " + UNBOUNDED,
     limit=10)

case(30, "Every 4 years, the first Tuesday after a Monday in November, forever "
         "(U.S. Presidential Election day)", "19961105T090000",
     "FREQ=YEARLY;INTERVAL=4;BYMONTH=11;BYDAY=TU;BYMONTHDAY=2,3,4,5,6,7,8",
     flatten(days("EST", 1996, 11, [5]), days("EST", 2000, 11, [7]), days("EST", 2004, 11, [2])),
     "RFC prose: ==> (1996 9:00 AM EST) November 5 (2000 9:00 AM EST) November 7 (2004 "
     "9:00 AM EST) November 2 ... . The RFC folds the RRULE across two lines; unfolded here. "
     + UNBOUNDED,
     limit=3)

case(31, "The third instance into the month of one of Tuesday, Wednesday, or Thursday, "
         "for the next 3 months", "19970904T090000",
     "FREQ=MONTHLY;COUNT=3;BYDAY=TU,WE,TH;BYSETPOS=3",
     flatten(days("EDT", 1997, 9, [4]), days("EDT", 1997, 10, [7]), days("EST", 1997, 11, [6])),
     "RFC prose: ==> (1997 9:00 AM EDT) September 4;October 7 (1997 9:00 AM EST) November 6")

case(32, "The second-to-last weekday of the month", "19970929T090000",
     "FREQ=MONTHLY;BYDAY=MO,TU,WE,TH,FR;BYSETPOS=-2",
     flatten(days("EDT", 1997, 9, [29]), days("EST", 1997, 10, [30]),
             days("EST", 1997, 11, [27]), days("EST", 1997, 12, [30]),
             days("EST", 1998, 1, [29]), days("EST", 1998, 2, [26]), days("EST", 1998, 3, [30])),
     "RFC prose: ==> (1997 9:00 AM EDT) September 29 (1997 9:00 AM EST) October 30;November "
     "27;December 30 (1998 9:00 AM EST) January 29;February 26;March 30 ... . " + UNBOUNDED,
     limit=7)

case(33, "Every 3 hours from 9:00 AM to 5:00 PM on a specific day", "19970902T090000",
     "FREQ=HOURLY;INTERVAL=3;UNTIL=19970902T210000Z",
     times("EDT", 1997, 9, 2, ["090000", "120000", "150000"]),
     "RFC prose: ==> (September 2, 1997 EDT) 09:00,12:00,15:00. Verified erratum 3883 "
     "applied: the RFC prints UNTIL=19970902T170000Z, which is 13:00 EDT and does not "
     "match the description or the stated output. The erratum corrects it to "
     "19970902T210000Z (17:00 EDT). The uncorrected rule is case 33-uncorrected.")

cases.append({
    "id": "rfc5545-3.8.5.3-33-uncorrected",
    "name": "Every 3 hours from 9:00 AM (RRULE exactly as printed, before erratum 3883)",
    "dtstart": "19970902T090000",
    "tzid": NYC,
    "rrule": ["FREQ=HOURLY;INTERVAL=3;UNTIL=19970902T170000Z"],
    "exrule": [], "rdate": [], "exdate": [],
    "include_dtstart": True,
    "between": None,
    "limit": 0,
    "expected": ["19970902T090000", "19970902T120000"],
    "error": False,
    "extensions": [],
    "comment": "Derived case, not RFC output. This is the RRULE exactly as printed in RFC "
               "5545, before Verified erratum 3883. UNTIL 19970902T170000Z is 13:00 EDT, so "
               "only 09:00 and 12:00 EDT qualify. The RFC's prose result (09:00,12:00,15:00) "
               "applies only to the corrected rule, case 33. This case tests that a UTC "
               "UNTIL is compared in absolute time.",
})

case(34, "Every 15 minutes for 6 occurrences", "19970902T090000",
     "FREQ=MINUTELY;INTERVAL=15;COUNT=6",
     times("EDT", 1997, 9, 2, ["090000", "091500", "093000", "094500", "100000", "101500"]),
     "RFC prose: ==> (September 2, 1997 EDT) 09:00,09:15,09:30,09:45,10:00,10:15")

case(35, "Every hour and a half for 4 occurrences", "19970902T090000",
     "FREQ=MINUTELY;INTERVAL=90;COUNT=4",
     times("EDT", 1997, 9, 2, ["090000", "103000", "120000", "133000"]),
     "RFC prose: ==> (September 2, 1997 EDT) 09:00,10:30;12:00;13:30 (the RFC mixes ',' "
     "and ';' as separators)")

day20 = ["%02d%02d00" % (h, m) for h in range(9, 17) for m in (0, 20, 40)]
m20 = flatten(times("EDT", 1997, 9, 2, day20), times("EDT", 1997, 9, 3, day20))
M20_PROSE = ("RFC prose: ==> (September 2, 1997 EDT) 9:00,9:20,9:40,10:00,10:20, ... "
             "16:00,16:20,16:40 (September 3, 1997 EDT) 9:00,9:20,9:40,10:00,10:20, "
             "...16:00,16:20,16:40 ... . The ellipsis inside each day is expanded to every 20 "
             "minutes from 09:00 to 16:40 (24 per day). Unbounded rule: limit=48 covers the "
             "two days the RFC lists.")
case(36, "Every 20 minutes from 9:00 AM to 4:40 PM every day (DAILY form)",
     "19970902T090000", "FREQ=DAILY;BYHOUR=9,10,11,12,13,14,15,16;BYMINUTE=0,20,40", m20,
     M20_PROSE + " First of the RFC's two alternatives; see case 36b.", limit=48, suffix="a")
case(36, "Every 20 minutes from 9:00 AM to 4:40 PM every day (MINUTELY form)",
     "19970902T090000", "FREQ=MINUTELY;INTERVAL=20;BYHOUR=9,10,11,12,13,14,15,16", m20,
     M20_PROSE + " Second of the RFC's two alternatives ('or'); see case 36a.", limit=48,
     suffix="b")

case(37, "An example where the days generated makes a difference because of WKST (WKST=MO)",
     "19970805T090000", "FREQ=WEEKLY;INTERVAL=2;COUNT=4;BYDAY=TU,SU;WKST=MO",
     days("EDT", 1997, 8, [5, 10, 19, 24]),
     "RFC prose: ==> (1997 EDT) August 5,10,19,24")

case(38, "Changing only WKST from MO to SU, yields different results (WKST=SU)",
     "19970805T090000", "FREQ=WEEKLY;INTERVAL=2;COUNT=4;BYDAY=TU,SU;WKST=SU",
     days("EDT", 1997, 8, [5, 17, 19, 31]),
     "RFC prose: ==> (1997 EDT) August 5,17,19,31")

case(39, "An example where an invalid date (i.e., February 30) is ignored",
     "20070115T090000", "FREQ=MONTHLY;BYMONTHDAY=15,30;COUNT=5",
     flatten(days("EST", 2007, 1, [15, 30]), days("EST", 2007, 2, [15]),
             days("EDT", 2007, 3, [15, 30])),
     "RFC prose: ==> (2007 EST) January 15,30 (2007 EST) February 15 (2007 EDT) March 15,30. "
     "February 30 does not exist; it is skipped and not counted toward COUNT.")

doc = {
    "source": "rfc5545",
    "upstream": "https://www.rfc-editor.org/rfc/rfc5545",
    "commit": "",
    "license": "IETF Trust Legal Provisions",
    "path": "section 3.8.5.3",
    "semantics": SEMANTICS,
    "cases": cases,
}

with open(sys.argv[1], "w", encoding="utf-8", newline="\n") as f:
    json.dump(doc, f, indent=2, ensure_ascii=False)
    f.write("\n")
print("cases:", len(cases))
