// Package contentline reads and writes the content lines of RFC 5545 §3.1:
// the NAME;PARAM=VALUE:VALUE lines, folded at 75 octets, that every
// iCalendar (and vCard) object is made of.
//
// It knows nothing about components or value types. A Line's Value is
// exactly the text after the colon, still escaped, and the Writer writes it
// back unchanged. Escaping TEXT values (\, \; \n) is the model's job; this
// layer handles folding, parameter quoting, RFC 6868 parameter escapes, and
// the safety checks that keep one line from becoming two.
//
// The Reader scans each line once, left to right, with no regular
// expressions, so its time and memory are linear in the input. It never
// panics on malformed input.
package contentline

import (
	"fmt"
	"strings"
)

// A Line is one unfolded content line.
type Line struct {
	// Name is the property name, or BEGIN or END, in upper case: RFC 5545
	// §2 makes names case-insensitive.
	Name string
	// Params are the parameters in the order they appear.
	Params []Param
	// Value is everything after the first colon that is not inside a
	// quoted parameter value, exactly as written: TEXT escapes and list
	// commas are left for the caller to interpret.
	Value string
	// Pos is the 1-based physical line the content line starts on. The
	// Writer ignores it.
	Pos int
}

// A Param is one parameter of a content line.
type Param struct {
	// Name is the parameter name in upper case.
	Name string
	// Values are the parameter's comma-separated values, with quotes
	// removed and RFC 6868 escapes decoded. Most parameters have one.
	Values []string
}

// Param returns the first value of the parameter named name, compared
// case-insensitively, and whether the line has it.
func (l *Line) Param(name string) (string, bool) {
	for _, p := range l.Params {
		if strings.EqualFold(p.Name, name) {
			if len(p.Values) == 0 {
				return "", true
			}
			return p.Values[0], true
		}
	}
	return "", false
}

// A SyntaxError reports a content line that does not follow RFC 5545 §3.1.
type SyntaxError struct {
	Line int    // 1-based physical line where the content line starts
	Msg  string // what is wrong
	Err  error  // the underlying error, such as ErrLineTooLong, or nil
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("contentline: line %d: %s", e.Line, e.Msg)
}

func (e *SyntaxError) Unwrap() error { return e.Err }

// isNameByte reports whether c may appear in a name: RFC 5545 iana-token
// and x-name are ALPHA, DIGIT and "-".
func isNameByte(c byte) bool {
	return 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || '0' <= c && c <= '9' || c == '-'
}

// isControl reports whether c is a CONTROL character of RFC 5545 §3.1:
// %x00-08 / %x0A-1F / %x7F. HTAB (%x09) is not one.
func isControl(c byte) bool {
	return c < 0x20 && c != '\t' || c == 0x7f
}
