"""Independent brute-force check of testdata/rfc5545/recur.json.

This is NOT an RRULE engine. For each case it hard-codes, from the RFC's English
description, a predicate over calendar days (or minutes), plus the COUNT/UNTIL
bound copied by hand from the RFC text. It then scans every candidate day or
minute from DTSTART forward and compares the result with the transcription.

Usage: python3 -I check_recur.py <recur.json>
"""
import calendar
import json
import sys
from datetime import date, datetime, timedelta, timezone
from zoneinfo import ZoneInfo

NY = ZoneInfo("America/New_York")
MO, TU, WE, TH, FR, SA, SU = range(7)


def utc(s):
    return datetime.strptime(s, "%Y%m%dT%H%M%SZ").replace(tzinfo=timezone.utc)


def abs_time(local):
    return local.replace(tzinfo=NY).astimezone(timezone.utc)


def mlen(d):
    return calendar.monthrange(d.year, d.month)[1]


def months_since(d0, d):
    return (d.year - d0.year) * 12 + (d.month - d0.month)


def week_start(d, wkst):
    return d - timedelta(days=(d.weekday() - wkst) % 7)


def weeks_since(d0, d, wkst):
    return (week_start(d, wkst) - week_start(d0, wkst)).days // 7


def nth_from_start(d):
    return (d.day - 1) // 7 + 1


def nth_from_end(d):
    return (mlen(d) - d.day) // 7 + 1


def month_days(d, weekdays):
    return [date(d.year, d.month, i) for i in range(1, mlen(d) + 1)
            if date(d.year, d.month, i).weekday() in weekdays]


# Each spec: dict(start=local datetime of DTSTART, step="day"|"minute",
#   pred=fn(local datetime)->bool, count=N | until=UTC string | limit=N,
#   exdate=[local datetimes], horizon=max datetime to scan)
def daily(y, m, d, h=9, mi=0):
    return datetime(y, m, d, h, mi)


SPECS = {}


def spec(cid, **kw):
    SPECS[cid] = kw


d0 = daily(1997, 9, 2)
# 01 Daily for 10 occurrences
spec("01", start=d0, pred=lambda t: True, count=10)
# 02 Daily until Dec 24 1997 (UNTIL=19971224T000000Z)
spec("02", start=d0, pred=lambda t: True, until="19971224T000000Z")
# 03 Every other day forever: listed through 1997-12-03
spec("03", start=d0, pred=lambda t: (t.date() - d0.date()).days % 2 == 0, limit=47)
# 04 Every 10 days, 5 occurrences
spec("04", start=d0, pred=lambda t: (t.date() - d0.date()).days % 10 == 0, count=5)
# 05 Every day in January for 3 years, UNTIL=20000131T140000Z
for s in ("05a", "05b"):
    spec(s, start=daily(1998, 1, 1), pred=lambda t: t.month == 1, until="20000131T140000Z")
# 06 Weekly for 10 occurrences (Tuesdays)
spec("06", start=d0, pred=lambda t: t.weekday() == TU, count=10)
# 07 Weekly until Dec 24 1997
spec("07", start=d0, pred=lambda t: t.weekday() == TU, until="19971224T000000Z")
# 08 Every other week forever, WKST=SU (13 listed)
spec("08", start=d0, pred=lambda t: t.weekday() == TU and weeks_since(d0, t, SU) % 2 == 0,
     limit=13)
# 09 Tue+Thu for five weeks
spec("09a", start=d0, pred=lambda t: t.weekday() in (TU, TH), until="19971007T000000Z")
spec("09b", start=d0, pred=lambda t: t.weekday() in (TU, TH), count=10)
# 10 Every other week Mon/Wed/Fri until Dec 24 1997, starting Mon Sep 1, WKST=SU
d10 = daily(1997, 9, 1)
spec("10", start=d10,
     pred=lambda t: t.weekday() in (MO, WE, FR) and weeks_since(d10, t, SU) % 2 == 0,
     until="19971224T000000Z")
# 11 Every other week Tue/Thu, 8 occurrences, WKST=SU
spec("11", start=d0,
     pred=lambda t: t.weekday() in (TU, TH) and weeks_since(d0, t, SU) % 2 == 0, count=8)
# 12/13 First Friday of the month
first_fri = lambda t: t.weekday() == FR and nth_from_start(t) == 1
spec("12", start=daily(1997, 9, 5), pred=first_fri, count=10)
spec("13", start=daily(1997, 9, 5), pred=first_fri, until="19971224T000000Z")
# 14 Every other month, first and last Sunday, 10 occurrences
d14 = daily(1997, 9, 7)
spec("14", start=d14,
     pred=lambda t: months_since(d14, t) % 2 == 0 and t.weekday() == SU
     and (nth_from_start(t) == 1 or nth_from_end(t) == 1), count=10)
# 15 Second-to-last Monday, 6 occurrences
spec("15", start=daily(1997, 9, 22), pred=lambda t: t.weekday() == MO and nth_from_end(t) == 2,
     count=6)
# 16 Third-to-last day of month forever (6 listed)
spec("16", start=daily(1997, 9, 28), pred=lambda t: t.day == mlen(t) - 2, limit=6)
# 17 2nd and 15th, 10 occurrences
spec("17", start=d0, pred=lambda t: t.day in (2, 15), count=10)
# 18 First and last day, 10 occurrences
spec("18", start=daily(1997, 9, 30), pred=lambda t: t.day == 1 or t.day == mlen(t), count=10)
# 19 Every 18 months, 10th-15th, 10 occurrences
d19 = daily(1997, 9, 10)
spec("19", start=d19, pred=lambda t: months_since(d19, t) % 18 == 0 and 10 <= t.day <= 15,
     count=10)
# 20 Every Tuesday, every other month (18 listed)
spec("20", start=d0, pred=lambda t: months_since(d0, t) % 2 == 0 and t.weekday() == TU,
     limit=18)
# 21 Yearly in June and July on the 10th, 10 occurrences
spec("21", start=daily(1997, 6, 10), pred=lambda t: t.month in (6, 7) and t.day == 10, count=10)
# 22 Every other year Jan/Feb/Mar on the 10th, 10 occurrences
spec("22", start=daily(1997, 3, 10),
     pred=lambda t: (t.year - 1997) % 2 == 0 and t.month in (1, 2, 3) and t.day == 10, count=10)
# 23 Every third year, yearday 1/100/200, 10 occurrences
spec("23", start=daily(1997, 1, 1),
     pred=lambda t: (t.year - 1997) % 3 == 0 and t.timetuple().tm_yday in (1, 100, 200),
     count=10)
# 24 20th Monday of the year (3 listed)
spec("24", start=daily(1997, 5, 19),
     pred=lambda t: t.weekday() == MO and (t.timetuple().tm_yday - 1) // 7 + 1 == 20, limit=3)
# 25 Monday of ISO week 20 (WKST=MO, week 1 has >= 4 days) (3 listed)
spec("25", start=daily(1997, 5, 12),
     pred=lambda t: t.weekday() == MO and t.isocalendar()[1] == 20
     and t.isocalendar()[0] == t.year, limit=3)
# 26 Every Thursday in March (11 listed)
spec("26", start=daily(1997, 3, 13), pred=lambda t: t.month == 3 and t.weekday() == TH, limit=11)
# 27 Every Thursday in Jun/Jul/Aug (39 listed)
spec("27", start=daily(1997, 6, 5), pred=lambda t: t.month in (6, 7, 8) and t.weekday() == TH,
     limit=39)
# 28 Friday the 13th; DTSTART (a Tuesday) is the first instance but excluded by EXDATE
spec("28", start=d0, pred=lambda t: t.weekday() == FR and t.day == 13, limit=5,
     exdate=[d0], unsynced_dtstart=True)
# 29 First Saturday after the first Sunday: Saturday on day 7..13 (10 listed)
spec("29", start=daily(1997, 9, 13), pred=lambda t: t.weekday() == SA and 7 <= t.day <= 13,
     limit=10)
# 30 Election day: every 4 years, November, Tuesday on day 2..8 (3 listed)
spec("30", start=daily(1996, 11, 5),
     pred=lambda t: (t.year - 1996) % 4 == 0 and t.month == 11 and t.weekday() == TU
     and 2 <= t.day <= 8, limit=3, horizon=datetime(2005, 1, 1))
# 31 Third of the Tue/Wed/Thu days in the month, 3 occurrences
spec("31", start=daily(1997, 9, 4),
     pred=lambda t: t.date() == month_days(t, (TU, WE, TH))[2], count=3)
# 32 Second-to-last weekday of the month (7 listed)
spec("32", start=daily(1997, 9, 29),
     pred=lambda t: t.date() == month_days(t, (MO, TU, WE, TH, FR))[-2], limit=7)
# 33 Every 3 hours 9-5 (erratum 3883 UNTIL=19970902T210000Z); and the uncorrected UNTIL
three_h = lambda t: t.minute == 0 and ((t - d0).total_seconds() // 3600) % 3 == 0
spec("33", start=d0, step="minute", pred=three_h, until="19970902T210000Z",
     horizon=datetime(1997, 9, 3))
spec("33-uncorrected", start=d0, step="minute", pred=three_h, until="19970902T170000Z",
     horizon=datetime(1997, 9, 3))
# 34 Every 15 minutes, 6 occurrences
spec("34", start=d0, step="minute",
     pred=lambda t: ((t - d0).total_seconds() // 60) % 15 == 0, count=6)
# 35 Every 90 minutes, 4 occurrences
spec("35", start=d0, step="minute",
     pred=lambda t: ((t - d0).total_seconds() // 60) % 90 == 0, count=4)
# 36 Every 20 minutes 9:00-16:40 every day (2 days listed = 48)
for s in ("36a", "36b"):
    spec(s, start=d0, step="minute",
         pred=lambda t: 9 <= t.hour <= 16 and t.minute in (0, 20, 40), limit=48)
# 37 / 38 every other week Tue+Sun, 4 occurrences, WKST matters
d37 = daily(1997, 8, 5)
spec("37", start=d37,
     pred=lambda t: t.weekday() in (TU, SU) and weeks_since(d37, t, MO) % 2 == 0, count=4)
spec("38", start=d37,
     pred=lambda t: t.weekday() in (TU, SU) and weeks_since(d37, t, SU) % 2 == 0, count=4)
# 39 15th and 30th monthly, 5 occurrences (Feb 30 never exists in a real-day scan)
spec("39", start=daily(2007, 1, 15), pred=lambda t: t.day in (15, 30), count=5)


def run(s):
    start = s["start"]
    step = timedelta(minutes=1) if s.get("step") == "minute" else timedelta(days=1)
    horizon = s.get("horizon", start + timedelta(days=366 * 12))
    until = utc(s["until"]) if "until" in s else None
    count = s.get("count")
    out = [start]  # DTSTART is always the first instance
    t = start + step
    while t < horizon:
        if count is not None and len(out) >= count:
            break
        if until is not None and abs_time(t) > until:
            break
        if s["pred"](t):
            out.append(t)
        t += step
    if until is not None:
        assert abs_time(start) <= until
    ex = set(s.get("exdate", []))
    out = [x for x in out if x not in ex]
    if "limit" in s:
        if len(out) < s["limit"]:
            raise SystemExit("horizon too small")
        out = out[: s["limit"]]
    return [x.strftime("%Y%m%dT%H%M%S") for x in out]


def rrule_bound_ok(cid, s, case):
    """Hand-copied COUNT/UNTIL must appear in the transcribed RRULE."""
    r = case["rrule"][0]
    if "count" in s and ("COUNT=%d" % s["count"]) not in r:
        return "COUNT mismatch"
    if "until" in s and ("UNTIL=" + s["until"]) not in r:
        return "UNTIL mismatch"
    if "count" not in s and "until" not in s and ("COUNT=" in r or "UNTIL=" in r):
        return "rule is bounded but checker treats it as unbounded"
    if "limit" in s and s["limit"] != case["limit"]:
        return "limit mismatch"
    return None


def main():
    doc = json.load(open(sys.argv[1], encoding="utf-8"))
    bad = 0
    seen = set()
    for c in doc["cases"]:
        cid = c["id"].replace("rfc5545-3.8.5.3-", "")
        seen.add(cid)
        s = SPECS.get(cid)
        if s is None:
            print("NO SPEC", cid)
            bad += 1
            continue
        if c["dtstart"] != s["start"].strftime("%Y%m%dT%H%M%S"):
            print("DTSTART MISMATCH", cid)
            bad += 1
        if not s.get("unsynced_dtstart") and not s["pred"](s["start"]):
            print("DTSTART not synchronized with predicate", cid)
            bad += 1
        if s.get("unsynced_dtstart") and s["pred"](s["start"]):
            print("expected unsynchronized DTSTART", cid)
            bad += 1
        err = rrule_bound_ok(cid, s, c)
        if err:
            print(err, cid)
            bad += 1
        got = run(s)
        if got != c["expected"]:
            bad += 1
            print("MISMATCH", cid)
            print("  transcribed-only:", sorted(set(c["expected"]) - set(got)))
            print("  checker-only:   ", sorted(set(got) - set(c["expected"])))
        else:
            print("ok %-16s %3d instances" % (cid, len(got)))
    missing = set(SPECS) - seen
    if missing:
        print("specs without cases:", sorted(missing))
        bad += 1
    print("FAILURES:", bad)
    sys.exit(1 if bad else 0)


main()
