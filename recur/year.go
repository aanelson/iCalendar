package recur

import (
	"time"

	"github.com/aanelson/iCalendar/civil"
)

// A yearInfo is what the day-level filters need to know about one year,
// indexed by day of the year from 0 (January 1). The masks that depend on
// the rule (week numbers, numbered weekdays) are built only when the rule
// uses them.
type yearInfo struct {
	year     int
	length   int   // 365 or 366
	firstDay int64 // epoch day of January 1
	leap     bool
	weekNo   []bool // day i is in a BYWEEKNO week
	nth      []bool // day i is one of the numbered BYDAY days
}

// Day-of-year tables for common (0) and leap (1) years.
var (
	monthOfDay    [2][366]uint8 // 1-12
	monthDayOfDay [2][366]uint8 // 1-31
	monthStart    [2][13]int    // day index each month starts at; [12] is the length
)

func init() {
	for leap := range 2 {
		year := 2001
		if leap == 1 {
			year = 2000
		}
		i := 0
		for m := 1; m <= 12; m++ {
			monthStart[leap][m-1] = i
			for d := 1; d <= civil.DaysIn(year, monthOf(m)); d++ {
				monthOfDay[leap][i] = uint8(m)
				monthDayOfDay[leap][i] = uint8(d)
				i++
			}
		}
		monthStart[leap][12] = i
	}
}

func monthOf(m int) time.Month { return time.Month(m) }

func newYearInfo(p *plan, year int) *yearInfo {
	y := &yearInfo{
		year:     year,
		length:   civil.DaysInYear(year),
		firstDay: civil.Date{Year: year, Month: time.January, Day: 1}.EpochDay(),
		leap:     civil.IsLeap(year),
	}
	if p.hasWeekNo {
		y.weekNo = weekNoMask(p, year, y.length)
	}
	if len(p.nth) > 0 {
		y.nth = make([]bool, y.length)
		if p.nthWithinYear {
			y.markNth(p, 0, y.length-1)
		} else {
			l := y.leapIndex()
			for m := range 12 {
				y.markNth(p, monthStart[l][m], monthStart[l][m+1]-1)
			}
		}
	}
	return y
}

func (y *yearInfo) leapIndex() int {
	if y.leap {
		return 1
	}
	return 0
}

func (y *yearInfo) weekday(i int) Weekday {
	return weekdayOfDay(y.firstDay + int64(i))
}

// markNth marks the numbered BYDAY days within days first to last.
func (y *yearInfo) markNth(p *plan, first, last int) {
	for _, w := range p.nth {
		var i int
		if w.N > 0 {
			i = first + (w.N-1)*7 + int(floorMod(int64(w.Day-y.weekday(first)), 7))
		} else {
			i = last + (w.N+1)*7 - int(floorMod(int64(y.weekday(last)-w.Day), 7))
		}
		if i >= first && i <= last {
			y.nth[i] = true
		}
	}
}

// week1Start returns the day index, relative to January 1 of year, that
// week 1 starts on when weeks start on wkst: the first week with at least
// four days in the year (RFC 5545 §3.3.10, BYWEEKNO). It is negative when
// week 1 starts in the previous December.
func week1Start(year int, wkst Weekday) int {
	jan1 := weekdayOfDay(civil.Date{Year: year, Month: time.January, Day: 1}.EpochDay())
	before := int(floorMod(int64(jan1-wkst), 7)) // days of Jan 1's week in December
	if before <= 3 {
		return -before
	}
	return 7 - before
}

// weeksIn returns how many weeks year has under wkst: 52 or 53.
func weeksIn(year int, wkst Weekday) int {
	return (civil.DaysInYear(year) - week1Start(year, wkst) + week1Start(year+1, wkst)) / 7
}

// weekNoMask marks the days of year whose week, numbered in whichever year
// the week belongs to, is in BYWEEKNO. A day at the edge of the year can
// belong to the previous year's last week or the next year's week 1, as
// python-dateutil also reads it.
func weekNoMask(p *plan, year, length int) []bool {
	mask := make([]bool, length)
	start := week1Start(year, p.wkst)
	weeks := weeksIn(year, p.wkst)
	prevWeeks := weeksIn(year-1, p.wkst)
	prevStart := week1Start(year-1, p.wkst) - civil.DaysInYear(year-1) // relative to this Jan 1
	nextWeeks := weeksIn(year+1, p.wkst)
	match := func(w, n int) bool {
		return p.weekNoPos&(1<<w) != 0 || p.weekNoNeg&(1<<(n-w+1)) != 0
	}
	for i := range length {
		var w, n int
		switch {
		case i < start:
			w, n = (i-prevStart)/7+1, prevWeeks
		case (i-start)/7+1 > weeks:
			w, n = 1, nextWeeks
		default:
			w, n = (i-start)/7+1, weeks
		}
		mask[i] = match(w, n)
	}
	return mask
}

// dayOK reports whether day i of y passes every day-level part of p.
func (p *plan) dayOK(y *yearInfo, i int) bool {
	l := y.leapIndex()
	if p.monthMask != 0 && p.monthMask&(1<<monthOfDay[l][i]) == 0 {
		return false
	}
	if p.hasWeekNo && !y.weekNo[i] {
		return false
	}
	if p.hasYearDay {
		pos, neg := i+1, y.length-i
		if p.yearDayPos[pos/64]&(1<<(pos%64)) == 0 && p.yearDayNeg[neg/64]&(1<<(neg%64)) == 0 {
			return false
		}
	}
	if p.hasMonthDay {
		m := monthOfDay[l][i]
		d := int(monthDayOfDay[l][i])
		neg := monthStart[l][m] - monthStart[l][m-1] - d + 1
		if p.monthDayPos&(1<<d) == 0 && p.monthDayNeg&(1<<neg) == 0 {
			return false
		}
	}
	if p.hasByDay {
		if p.weekdayMask&(1<<y.weekday(i)) == 0 && (y.nth == nil || !y.nth[i]) {
			return false
		}
	}
	return true
}

// weekYearStart returns the epoch day week 1 of year starts on.
func weekYearStart(year int, wkst Weekday) int64 {
	return civil.Date{Year: year, Month: time.January, Day: 1}.EpochDay() + int64(week1Start(year, wkst))
}

// weekYearOf returns the week-numbering year epoch day d belongs to.
func weekYearOf(d int64, wkst Weekday) int {
	y := civil.DateOfEpochDay(d).Year
	switch {
	case d < weekYearStart(y, wkst):
		return y - 1
	case d >= weekYearStart(y+1, wkst):
		return y + 1
	}
	return y
}
