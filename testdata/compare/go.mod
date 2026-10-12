module github.com/aanelson/iCalendar/testdata/compare

go 1.25

require (
	github.com/aanelson/iCalendar v0.0.0
	github.com/arran4/golang-ical v0.3.7
	github.com/emersion/go-ical v0.0.0-20250609112844-439c63cef608
	github.com/teambition/rrule-go v1.8.2
)

replace github.com/aanelson/iCalendar => ../..
