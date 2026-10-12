package recur

import (
	"container/list"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/aanelson/iCalendar/civil"
)

// A Cache remembers what expanding a rule has taught the engine, so later
// expansions of the same rule from the same DTSTART skip work:
//
//   - For a COUNT rule, sparse checkpoints of how many instances came
//     before a period. A query far from DTSTART starts at the nearest
//     checkpoint instead of counting from DTSTART. The stride between
//     checkpoints doubles as a rule's table grows, so each rule's memory
//     stays bounded however far it is walked.
//   - Where a rule ends: its last instance once a walk reaches COUNT,
//     UNTIL or the end of what it can ever produce. A query past the end
//     returns at once.
//
// A rule without COUNT needs no cache to seek; see Rule.Between.
//
// A Cache is safe for concurrent use. It holds at most its MaxRules rules,
// dropping the least recently used. The zero value is not usable; call
// NewCache. Nothing is cached unless a Cache is passed in: there is no
// package-level cache.
type Cache struct {
	mu      sync.Mutex
	max     int
	byKey   map[string]*list.Element
	lru     *list.List // of *ruleState, least recently used first
	hits    int64
	misses  int64
	stores  int64
	evicted int64
}

// DefaultCacheRules is the number of rules a Cache keeps when NewCache is
// given zero.
const DefaultCacheRules = 10_000

// NewCache returns a Cache keeping at most maxRules rules; zero means
// DefaultCacheRules.
func NewCache(maxRules int) *Cache {
	if maxRules <= 0 {
		maxRules = DefaultCacheRules
	}
	return &Cache{max: maxRules, byKey: map[string]*list.Element{}, lru: list.New()}
}

// CacheStats are counters a Cache keeps, for monitoring.
type CacheStats struct {
	Rules   int   // rules held now
	Hits    int64 // lookups that found the rule
	Misses  int64 // lookups that didn't
	Stores  int64 // walks that added to the cache
	Evicted int64 // rules dropped to stay under the limit
}

// Stats returns the cache's counters.
func (c *Cache) Stats() CacheStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return CacheStats{Rules: c.lru.Len(), Hits: c.hits, Misses: c.misses, Stores: c.stores, Evicted: c.evicted}
}

const (
	checkpointEvery    = 64  // the initial stride, in periods
	maxCheckpointsRule = 256 // the stride doubles past this many
)

// A checkpoint records that before is the number of instances in the
// periods before period.
type checkpoint struct {
	period int64
	before int64
}

type ruleState struct {
	key string
	// loc is the zone the key names by address. Holding it keeps the
	// address from being reused by another zone while the entry lives.
	loc    *time.Location
	stride int64
	points []checkpoint // ascending period
	ended  bool
	end    int64 // with ended: no instance is after this wall-clock second
}

// cacheKey identifies a rule anchored at start. The zone matters only for
// a UTC UNTIL, and is identified by the *time.Location itself: two
// locations with one name but different rules (an IANA zone and a
// calendar's own VTIMEZONE) must not share entries.
func cacheKey(r Rule, start civil.DateTime, o iterOpts) string {
	start.Nanosecond = 0
	key := r.String() + "@" + strconv.FormatInt(start.EpochSecond(), 10)
	if r.Until.UTC && o.loc != nil {
		key += fmt.Sprintf("@%p", o.loc)
	}
	if o.countStart {
		key += "@c"
	}
	if o.allDay {
		key += "@d"
	}
	return key
}

// lookup returns the state for key, marking it recently used. The caller
// holds the lock.
func (c *Cache) lookup(key string) *ruleState {
	e, ok := c.byKey[key]
	if !ok {
		c.misses++
		return nil
	}
	c.hits++
	c.lru.MoveToBack(e)
	return e.Value.(*ruleState)
}

// end reports where the rule is known to end.
func (c *Cache) end(key string) (int64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if s := c.lookup(key); s != nil && s.ended {
		return s.end, true
	}
	return 0, false
}

// checkpoint returns the latest checkpoint at or before period, and where
// the rule ends if that is known.
func (c *Cache) checkpoint(key string, period int64) (cp checkpoint, end int64, ended bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.lookup(key)
	if s == nil {
		return checkpoint{}, 0, false
	}
	i := sort.Search(len(s.points), func(i int) bool { return s.points[i].period > period })
	if i > 0 {
		cp = s.points[i-1]
	}
	return cp, s.end, s.ended
}

// store adds what a walk found: checkpoints, and the rule's end when the
// walk reached it.
func (c *Cache) store(key string, loc *time.Location, found []checkpoint, ended bool, end int64) {
	if len(found) == 0 && !ended {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stores++
	var s *ruleState
	if e, ok := c.byKey[key]; ok {
		s = e.Value.(*ruleState)
		c.lru.MoveToBack(e)
	} else {
		for c.lru.Len() >= c.max {
			old := c.lru.Front()
			c.lru.Remove(old)
			delete(c.byKey, old.Value.(*ruleState).key)
			c.evicted++
		}
		s = &ruleState{key: key, loc: loc, stride: checkpointEvery}
		c.byKey[key] = c.lru.PushBack(s)
	}
	if ended && (!s.ended || end < s.end) {
		s.ended, s.end = true, end
	}
	for _, cp := range found {
		s.add(cp)
	}
}

func (s *ruleState) add(cp checkpoint) {
	if cp.period%s.stride != 0 {
		return
	}
	i := sort.Search(len(s.points), func(i int) bool { return s.points[i].period >= cp.period })
	if i < len(s.points) && s.points[i].period == cp.period {
		return
	}
	s.points = append(s.points, checkpoint{})
	copy(s.points[i+1:], s.points[i:])
	s.points[i] = cp
	if len(s.points) > maxCheckpointsRule {
		s.stride *= 2
		kept := s.points[:0]
		for _, p := range s.points {
			if p.period%s.stride == 0 {
				kept = append(kept, p)
			}
		}
		s.points = kept
	}
}
