// Package tz turns iCalendar TZIDs into *time.Location values.
//
// A TZID names a zone in one of three ways, and Load and Compile cover
// them:
//
//   - A VTIMEZONE in the same calendar defines it (RFC 5545 §3.6.5).
//     Compile expands the definition's observances with package recur and
//     builds a real *time.Location from them, so instants, offsets and
//     abbreviations all come out right, including for zones like Outlook's
//     "Customized Time Zone" that exist nowhere else.
//   - It is an IANA name, like America/New_York, which Load resolves.
//   - It is a Windows name, like "Eastern Standard Time", or a decorated
//     IANA name, like /mozilla.org/20050126_1/America/New_York, which Load
//     maps to the IANA zone.
//
// Both functions cache what they build. The results are immutable and
// depend only on their input, so the caches are shared process-wide and
// are bounded.
//
// Turning a wall-clock time in a zone into an instant is
// civil.DateTime.In, which applies RFC 5545's rules for times that occur
// twice or not at all.
package tz

import (
	"container/list"
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"
	"time"
)

// ErrUnknownZone is wrapped by the error Load returns for a TZID it can't
// resolve.
var ErrUnknownZone = errors.New("tz: unknown time zone")

// A Resolver resolves TZIDs to locations. The model uses one per
// calendar, consulting the calendar's own VTIMEZONEs before Load.
type Resolver interface {
	Resolve(tzid string) (*time.Location, error)
}

// ResolverFunc adapts a function to the Resolver interface.
type ResolverFunc func(tzid string) (*time.Location, error)

// Resolve calls f.
func (f ResolverFunc) Resolve(tzid string) (*time.Location, error) { return f(tzid) }

// Load returns the location a TZID names when no VTIMEZONE defines it: an
// IANA name, a Windows zone ID as Outlook and Exchange write them, or an
// IANA name decorated the way some producers do it (a quoted name, or a
// "/vendor/version/" prefix). Lookups are cached.
//
// Load never guesses. A TZID it doesn't recognize, including a Windows
// display name like "(UTC-05:00) Eastern Time (US & Canada)", is an error
// wrapping ErrUnknownZone; such TZIDs normally come with a VTIMEZONE,
// which Compile handles.
func Load(tzid string) (*time.Location, error) {
	if loc, ok := loaded.get(tzid); ok {
		if loc == nil {
			return nil, fmt.Errorf("%w: %q", ErrUnknownZone, tzid)
		}
		return loc, nil
	}
	loc := load(tzid)
	loaded.put(tzid, loc)
	if loc == nil {
		return nil, fmt.Errorf("%w: %q", ErrUnknownZone, tzid)
	}
	return loc, nil
}

func load(tzid string) *time.Location {
	name := strings.TrimSpace(tzid)
	name = strings.Trim(name, `"`)
	if name == "" {
		return nil
	}
	try := func(n string) *time.Location {
		// "Local" would be the host's zone, which a calendar can't mean. So
		// would "localtime": Debian-family zoneinfo links it (and, through
		// the posix directory, posix/localtime) to /etc/localtime.
		if n == "Local" || strings.EqualFold(path.Base(n), "localtime") || strings.HasPrefix(n, "/") || strings.Contains(n, "..") {
			return nil
		}
		if loc, err := time.LoadLocation(n); err == nil {
			return loc
		}
		return nil
	}
	if loc := try(name); loc != nil {
		return loc
	}
	if iana, ok := windowsZones[name]; ok {
		return try(iana)
	}
	// A globally unique TZID (RFC 5545 §3.8.3.1 gives "/example.org/…")
	// often ends in an IANA name: drop leading segments until one loads.
	if strings.HasPrefix(name, "/") {
		parts := strings.Split(strings.Trim(name, "/"), "/")
		for i := 1; i < len(parts); i++ {
			if loc := try(strings.Join(parts[i:], "/")); loc != nil {
				return loc
			}
		}
	}
	// Case differences are common in hand-written feeds.
	if loc := try(canonicalCase(name)); loc != nil {
		return loc
	}
	// A fixed offset, like UTC-04:00 or GMT+0530, names its zone exactly.
	if off, ok := fixedOffset(name); ok {
		return time.FixedZone(name, off)
	}
	return nil
}

// fixedOffset reads UTC±H, UTC±HH:MM, UTC±HHMM and the same after GMT,
// returning seconds east of UTC.
func fixedOffset(name string) (int, bool) {
	rest, ok := strings.CutPrefix(strings.ToUpper(name), "UTC")
	if !ok {
		if rest, ok = strings.CutPrefix(strings.ToUpper(name), "GMT"); !ok {
			return 0, false
		}
	}
	if rest == "" {
		return 0, true
	}
	sign := 1
	switch rest[0] {
	case '+':
	case '-':
		sign = -1
	default:
		return 0, false
	}
	rest = strings.ReplaceAll(rest[1:], ":", "")
	var h, m int
	switch len(rest) {
	case 1, 2:
		h = atoi(rest)
	case 3, 4:
		h, m = atoi(rest[:len(rest)-2]), atoi(rest[len(rest)-2:])
	default:
		return 0, false
	}
	if h < 0 || h > 14 || m < 0 || m > 59 {
		return 0, false
	}
	return sign * (h*3600 + m*60), true
}

// atoi reads decimal digits, or returns -1.
func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// canonicalCase capitalizes each part of a zone name the way the IANA
// database spells them: america/new_york is America/New_York.
func canonicalCase(name string) string {
	b := []byte(strings.ToLower(name))
	up := true
	for i, c := range b {
		if up && 'a' <= c && c <= 'z' {
			b[i] = c - 'a' + 'A'
		}
		up = c == '/' || c == '_' || c == '-'
	}
	return string(b)
}

// lru is a small, bounded, concurrency-safe cache.
type lru[V any] struct {
	mu    sync.Mutex
	max   int
	items map[string]*list.Element
	order *list.List
}

type lruEntry[V any] struct {
	key string
	val V
}

func newLRU[V any](max int) *lru[V] {
	return &lru[V]{max: max, items: map[string]*list.Element{}, order: list.New()}
}

func (c *lru[V]) get(key string) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[key]; ok {
		c.order.MoveToBack(e)
		return e.Value.(*lruEntry[V]).val, true
	}
	var zero V
	return zero, false
}

func (c *lru[V]) put(key string, v V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[key]; ok {
		e.Value.(*lruEntry[V]).val = v
		c.order.MoveToBack(e)
		return
	}
	for c.order.Len() >= c.max {
		old := c.order.Front()
		c.order.Remove(old)
		delete(c.items, old.Value.(*lruEntry[V]).key)
	}
	c.items[key] = c.order.PushBack(&lruEntry[V]{key: key, val: v})
}

// The caches. Failed lookups are cached too (as nil), so a feed repeating
// a bad TZID doesn't reach the zoneinfo files on every property.
var (
	loaded   = newLRU[*time.Location](4096)
	compiled = newLRU[compileResult](1024)
)
