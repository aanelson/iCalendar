// Ad-hoc check: what does each library do with a non-calendar body (an
// HTML error page) and with a bare VEVENT fragment?
package main

import (
	"bytes"
	"fmt"
	"io"

	ical "github.com/aanelson/iCalendar"
	ics "github.com/arran4/golang-ical"
	eical "github.com/emersion/go-ical"
)

func main() {
	for name, in := range map[string]string{
		"html 404 page": "<!DOCTYPE html>\r\n<html><body>404 Not Found</body></html>\r\n",
		"bare VEVENT":   "BEGIN:VEVENT\r\nUID:1\r\nDTSTART:20260101T100000Z\r\nEND:VEVENT\r\n",
	} {
		fmt.Println("==", name)
		var warns []string
		d := ical.NewDecoder(bytes.NewReader([]byte(in)))
		d.Warn = func(err error) { warns = append(warns, err.Error()) }
		cal, err := d.Decode()
		fmt.Printf("  this Decoder.Decode: cal=%v err=%v (err==io.EOF: %v) warnings=%q\n", cal != nil, err, err == io.EOF, warns)
		_, err = ical.Parse(bytes.NewReader([]byte(in)))
		fmt.Printf("  this Parse: err=%v\n", err)
		gc, err := ics.ParseCalendar(bytes.NewReader([]byte(in)))
		fmt.Printf("  golang-ical ParseCalendar: err=%v events=%v\n", err, func() int { if gc == nil { return -1 }; return len(gc.Events()) }())
		_, err = eical.NewDecoder(bytes.NewReader([]byte(in))).Decode()
		fmt.Printf("  emersion Decode: err=%v\n", err)
	}
}
