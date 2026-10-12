package contentline

// knownNames interns the component, property and parameter names RFC 5545
// defines, and the X- names common producers emit, so reading them doesn't
// allocate. Any other name is still read; it just costs a string.
var knownNames = func() map[string]string {
	names := []string{
		"BEGIN", "END",
		// Components (§3.4, §3.6).
		"VCALENDAR", "VEVENT", "VTODO", "VJOURNAL", "VFREEBUSY", "VTIMEZONE",
		"VALARM", "STANDARD", "DAYLIGHT",
		// Calendar properties (§3.7).
		"CALSCALE", "METHOD", "PRODID", "VERSION",
		// Descriptive (§3.8.1).
		"ATTACH", "CATEGORIES", "CLASS", "COMMENT", "DESCRIPTION", "GEO",
		"LOCATION", "PERCENT-COMPLETE", "PRIORITY", "RESOURCES", "STATUS",
		"SUMMARY",
		// Date and time (§3.8.2).
		"COMPLETED", "DTEND", "DUE", "DTSTART", "DURATION", "FREEBUSY", "TRANSP",
		// Time zone (§3.8.3).
		"TZID", "TZNAME", "TZOFFSETFROM", "TZOFFSETTO", "TZURL",
		// Relationship (§3.8.4).
		"ATTENDEE", "CONTACT", "ORGANIZER", "RECURRENCE-ID", "RELATED-TO",
		"URL", "UID",
		// Recurrence (§3.8.5).
		"EXDATE", "EXRULE", "RDATE", "RRULE",
		// Alarm (§3.8.6).
		"ACTION", "REPEAT", "TRIGGER",
		// Change management (§3.8.7).
		"CREATED", "DTSTAMP", "LAST-MODIFIED", "SEQUENCE",
		// Miscellaneous (§3.8.8).
		"REQUEST-STATUS",
		// RFC 7986.
		"NAME", "REFRESH-INTERVAL", "SOURCE", "COLOR", "IMAGE", "CONFERENCE",
		// Parameters (§3.2), less TZID which is above.
		"ALTREP", "CN", "CUTYPE", "DELEGATED-FROM", "DELEGATED-TO", "DIR",
		"ENCODING", "FMTTYPE", "FBTYPE", "LANGUAGE", "MEMBER", "PARTSTAT",
		"RANGE", "RELATED", "RELTYPE", "ROLE", "RSVP", "SENT-BY", "VALUE",
		"DISPLAY", "EMAIL", "FEATURE", "LABEL",
		// Common extensions.
		"X-WR-CALNAME", "X-WR-CALDESC", "X-WR-TIMEZONE", "X-WR-RELCALID",
		"X-PUBLISHED-TTL", "X-LIC-LOCATION", "X-ALT-DESC",
		"X-MICROSOFT-CDO-BUSYSTATUS", "X-MICROSOFT-CDO-INTENDEDSTATUS",
		"X-MICROSOFT-CDO-ALLDAYEVENT", "X-MICROSOFT-CDO-IMPORTANCE",
		"X-MICROSOFT-CDO-INSTTYPE", "X-MICROSOFT-DONOTFORWARDMEETING",
		"X-MICROSOFT-DISALLOW-COUNTER", "X-MICROSOFT-CDO-APPT-SEQUENCE",
		"X-MICROSOFT-CDO-OWNERAPPTID", "X-MICROSOFT-CDO-REPLYTIME",
		"X-MS-OLK-CONFTYPE", "X-MS-OLK-AUTOFILLLOCATION",
		"X-APPLE-STRUCTURED-LOCATION", "X-APPLE-TRAVEL-ADVISORY-BEHAVIOR",
		"X-APPLE-CALENDAR-COLOR", "X-APPLE-DEFAULT-ALARM",
		"X-GOOGLE-CONFERENCE", "X-MOZ-GENERATION", "X-MOZ-LASTACK",
		"X-MOZ-SNOOZE-TIME",
		"X-LIC-ERROR", "X-ALT-DESC", "X-WR-ALARMUID", "X-APPLE-RADIUS",
		"X-TITLE", "X-ADDRESS",
	}
	m := make(map[string]string, len(names))
	for _, n := range names {
		m[n] = n
	}
	return m
}()
