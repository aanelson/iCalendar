# iCalendar library API survey

> **Purpose.** Inform the public API of a dependency-free, idiomatic Go
> iCalendar library. Scope is RFC 5545 first, then 7986, 5546, 7529 and
> 9073/9074. The library owns recurrence expansion and can cache it.
>
> **Method.** I read shallow clones of each default branch (October 2026) as
> source only, without building or running anything, and searched each
> project's GitHub issues. Each section gives the HEAD commit and the release.
>
> **Conventions.** `#N` is an issue in that library's own tracker; issues in
> other trackers are prefixed (for example golang/go#49951). "Code reading"
> marks behaviour I inferred from source but did not test.

**Contents:** [Comparison](#comparison-table) · [Cross-cutting](#cross-cutting-findings) · [ical.js](#javascript-kewischicaljs) · [icalendar](#python-collectiveicalendar) · [dateutil](#python-dateutilrrule) · [recurring-ical-events](#python-recurring-ical-events) · [ical4j](#java-ical4j) · [vobject](#php-sabre-iovobject) · [ical.net](#net-icalnet) · [libical](#c-libical) · [rust-rrule](#rust-rrule-fmeringdalrust-rrule) · [icalendar-rs](#rust-icalendar-hoodieicalendar-rs) · [go-ical](#go-emersiongo-ical) · [golang-ical](#go-arran4golang-ical) · [rrule-go](#go-teambitionrrule-go) · [**Implications for Go**](#implications-for-an-idiomatic-go-api)

## Comparison table

| Library | Model style | Iteration style | Seek support | Cache | VTIMEZONE support | Override handling | Extension mechanism |
|---|---|---|---|---|---|---|---|
| **ical.js** 2.2.1 | jCal arrays (ordered) + lazy `Component`/`Property` wrappers | Pull `next()` (mutable result); state via `toJSON()` | None; `iterator(start)` *replaces* DTSTART (#456) | Per-zone change list | Yes, but gap and overlap resolve **against** RFC §3.3.5; unregistered TZID → floating (#582) | `Event.relateException`; THISANDFUTURE (nearest only) | Mutate global `ICAL.design` sets |
| **python icalendar** 7.3.0 | Dict-of-lists `Component` subclasses + typed values | n/a | n/a | n/a | Only for TZIDs unknown to the system (IANA wins); global first-wins cache; Windows map | n/a | Global `TypesFactory.types_map`; `Component.register` |
| **dateutil** 2.9.0 | `rrule`/`rruleset` objects | Generator + list helpers | None (walks from DTSTART) | `cache=True` (unbounded memo) | n/a (tzinfo) | n/a | n/a |
| **recurring-ical-events** 3.9.0 | Adapters over icalendar | `between/at/after/all`; `Occurrence` | Window queries, still walking | dateutil cache per rule | Via icalendar | Full: RECURRENCE-ID, THISANDFUTURE, EXDATE, RDATE PERIOD | Adapter/Series/Occurrence subclasses |
| **ical4j** 4.4.0 | Typed classes; immutable lists in mutable containers | Eager `List` + `getDatesAsStream` | Exponential/binary search to window (no COUNT) | None | `ZoneRulesBuilder` → synthetic `ZoneId` | `ComponentGroup` only; no THISANDFUTURE | `ContentHandlerContext` factory suppliers |
| **sabre/vobject** 5.0.0 | Generic tree bucketed **by name** (order lost) | PHP `Iterator` | `fastForward` (linear) | None | Rules **not** interpreted; TZID → maps/guessers, else default zone silently | Replace by RECURRENCE-ID; no THISANDFUTURE | Global static class maps |
| **ical.net** 5.2.3 / main | Typed classes over grouped property list | Lazy `IEnumerable<Occurrence>` | main: arithmetic skip (no COUNT); 5.2.3: linear | None | 5.2.3: ignored (NodaTime TZDB/Windows); main: fallback after TZDB | `Calendar.GetOccurrences` only; no THISANDFUTURE (#455) | 5.2.3 serializer factories; main per-name converters |
| **libical** 4.0.x | Generic C tree + generated typed accessors | Pull `next()`/`prev()`; callback `foreach_recurrence` | `set_start`/`set_range` arithmetic (no COUNT) | Per-zone change array | Yes; embedded first; RFC gap/overlap (4.0.6+); no Windows map | **None** in `foreach_recurrence` (#314) | Build-time codegen from CSV; X-/IANA kinds |
| **rust-rrule** 0.14.0 | Typestate `RRule<Unvalidated>` → `RRuleSet` | `IntoIterator`; `all(limit)` → `limited` | None (bounds filter after generation) | None | n/a (chrono-tz, IANA only) | n/a | None |
| **icalendar-rs** 0.17.14 | Borrowed ordered parse tree + owned `BTreeMap` builder tier | Via rrule (`recurrence` feature) | n/a | n/a | No | Accessors only | Generic `Property::new` |
| **emersion/go-ical** | Generic tree; `map[string][]Prop` (order lost) | Delegated to rrule-go | No | No | No (`time.LoadLocation`) | No | Generic names; unexported default-type map |
| **arran4/golang-ical** | Typed structs over ordered raw property slice | None (RRULE parse/print) | n/a | n/a | No (mapper → `LoadLocation`) | No | Helper subpackages |
| **rrule-go** 1.8.2 | `ROption` → `RRule`, `Set` | Closure `func() (time.Time, bool)` + slices | No | No | n/a | n/a | n/a |

## Cross-cutting findings

1. **Ordering is fragile.** Grouping properties by name loses interleaving
   and parameter order. emersion uses a map, vobject buckets children,
   python icalendar uses a dict of lists, and ical.net groups; all of these
   sort on output by default. icalendar-rs's owned `BTreeMap` also collapses
   repeats (#81). ical.js, libical, ical4j and golang-ical keep ordered lists,
   though ical.js and libical still write all properties before all
   subcomponents.
2. **VTIMEZONE rules are rarely the source of truth.** ical.js, libical and
   ical4j interpret embedded rules first. python icalendar and ical.net main
   use them only when the TZID isn't a known IANA or Windows name. vobject,
   rust-rrule, icalendar-rs and both Go libraries never read them. Go has no
   public `time.Location` constructor besides `FixedZone` and
   `LoadLocationFromTZData` (golang/go#49951 was declined), so a VTIMEZONE
   has to be compiled to TZif bytes.
3. **An unresolved TZID degrades silently.** ical.js (#582) and python
   icalendar fall back to floating, vobject to the PHP default zone, and
   golang-ical to `time.Local`. golang-ical dropped such events entirely
   before #137. Only ical.net throws.
4. **DST gap and overlap handling varies.** RFC 5545 §3.3.5 says a nonexistent
   local time uses the pre-gap offset, and a repeated local time means its
   first occurrence. Comply: libical 4.0.6+ (#1402), ical4j
   (`withEarlierOffsetAtOverlap`, #716), ical.net (NodaTime lenient resolver),
   rust-rrule. Reversed on both: ical.js (`want_daylight = false; // TODO`,
   #1051). No handling: python icalendar's zoneinfo path (#649), dateutil.
   Inherit the host: rrule-go (Go's `time.Date` "does not guarantee which"
   zone) and vobject (PHP).
5. **Real seeking exists only without COUNT.** libical
   (`icalrecur_iterator_set_start`), ical.net main (`SkipToByInterval`) and
   ical4j (binary search) jump straight to the window. With COUNT, and in
   every other library, iteration walks from DTSTART. dateutil's
   `cache=True` is the only other mitigation.
6. **Runaway-rule budgets are inconsistent.** ical4j caps consecutive empty
   increments at 1000. libical stops after 100k candidates or 50 "standing
   still" steps. ical.js gives up after 336 empty months or 28 empty years
   (`maxTries` 500). ical.net's limit defaults to *none*, and rust-rrule sets
   a `limited` flag. dateutil and rrule-go stop only at year 9999. vobject
   throws after 3500 *instances*, a count that includes `fastForward` steps,
   so a decade-old daily event fails a "today" query
   (nextcloud/server#43647).
7. **THISANDFUTURE is nearly unsupported.** Only ical.js (nearest range
   exception, no stacking) and recurring-ical-events implement it. ical.net
   (#455), ical4j (per its javadoc) and vobject (which ignores RANGE) don't.
   libical, rust-rrule and the Go libraries apply no overrides at all.
8. **EXDATE/RDATE edge cases recur across libraries.** A DATE EXDATE matches
   by day in ical.js, recurring-ical-events, ical.net (#829) and libical. It
   never matches in vobject (#555), rust-rrule (#146) or ical4j (code
   reading). An RDATE that coincides with an RRULE instance is duplicated in
   ical.js (#1026) and rust-rrule (#150). vobject drops the RRULE whenever an
   RDATE exists (#700).

---

## JavaScript: kewisch/ical.js

HEAD 33ba266, v2.2.1. ES2022 module with JSDoc; the `.d.ts` files are generated.

```js
const comp = new ICAL.Component(ICAL.parse(ics));   // jCal (RFC 7265); or Component.fromString
const ev = new ICAL.Event(comp.getFirstSubcomponent('vevent'), {strictExceptions: false});
const it = ev.iterator();                            // RecurExpansion
for (let t; (t = it.next()); ) { const {recurrenceId, item, startDate, endDate} = ev.getOccurrenceDetails(t); }
const ri = ICAL.Recur.fromString('FREQ=WEEKLY;BYDAY=MO').iterator(dtstart); // RecurIterator
ICAL.stringify(comp.toJSON()); comp.toString();
```

1. **Parse/serialize.** Reads the whole string; there is no streaming. The
   first error throws `ICAL.parse.ParserError` (or a plain `Error` for a bad
   RRULE), and there is no warnings channel. `END:` is not checked against its
   `BEGIN`. Strictness is one global flag, `ICAL.design.strict`. Values decode
   lazily, so a bad DATE-TIME throws only when it is first read (#186). Output
   folds at 75 octets with CRLF.
2. **Model.** The source of truth is jCal arrays. Lazy wrappers are created on
   access and cached. Property order is preserved, but output writes
   properties before subcomponents. Unknown and X- properties get type
   `unknown` and keep their raw text. A repeated parameter keeps only its last
   value, and quoting is normalized. Escaped parameter values are misparsed
   (#669). Unknown RRULE parts are lost if a decoded `Recur` is written back
   (code reading). There is no validation.
3. **Date/time.** `ICAL.Time` holds civil fields, `isDate`, and a zone
   (`utcTimezone`, floating `localTimezone`, or a VTIMEZONE-backed
   `Timezone`). A TZID resolves through the root VCALENDAR's VTIMEZONEs, then
   the global `TimezoneService`, and otherwise **silently becomes floating**.
   There is no Windows mapping. `Timezone` caches expanded changes up to the
   current year plus 5. A gap time gets the *post*-transition offset and an
   overlap time the *second* occurrence. The tests assert both, and both
   violate the RFC (#1051). A VTIMEZONE uses only its first RDATE value
   (#1046), and a SECONDLY observance can hang (#1054).
4. **Recurrence.** `Recur.fromString`/`fromData`. `RecurIterator.next()`
   returns the *same mutable* `Time`, so callers must clone it. `toJSON()`
   makes an iterator resumable, but there is no between or seek (#148, open
   since 2015). DTSTART is not emitted if the rule doesn't produce it (#294).
   Guards: a YEARLY scan to year 20000, 336 empty months, 28 empty years, and
   a check for repeated results. Impossible DAILY/WEEKLY rules are unguarded
   (code reading). Loop reports: #318, #509.
5. **Expansion.** `RecurExpansion` merges RRULE iterators with sorted RDATE
   and EXDATE arrays. A DATE EXDATE matches by day, and `maxTries` is 500.
   `Event` keys exceptions by **wall-time string**, so a RECURRENCE-ID in a
   different TZID misses (code reading). Siblings are related **without a UID
   check** (#1019). THISANDFUTURE shifts later instances by the nearest range
   exception's start delta. Moved and orphan overrides are not re-sorted or
   yielded (#285). End is DTEND, else DTSTART plus DURATION, else plus one day
   for a DATE.
6. **Extension.** There is no register API; you mutate global objects, for
   example `ICAL.design.icalendar.property['x-foo'] = {defaultType: 'text'}`,
   plus `param` and `value` maps with `decorate`/`undecorate` hooks. RFC 7986
   is absent, so COLOR parses as `unknown`.

## Python: collective/icalendar

HEAD ec09f7e, 7.3.0 (2026-08). zoneinfo by default; pytz is optional.

```python
cal = Calendar.from_ical(data)                    # multiple=True -> list; str|bytes|Path
for ev in cal.walk("VEVENT"): ev.start, ev.end, ev.duration, ev.rrules, ev.rdates, ev.exdates
ev = Event.new(summary="x", start=dt, end=dt2)     # auto DTSTAMP/UID; also Calendar.new(...)
cal.add_missing_timezones(); cal.to_ical(sorted=True); cal.timezones[0].to_tz()
```

1. **Parse/serialize.** In-memory parse with regex unfolding. `LazyCalendar`
   defers parsing of subcomponents. If a VTIMEZONE comes after the components
   that use it, the input is re-parsed. Leniency is a **per-class flag**, and
   only `Event` is lenient. A bad VCALENDAR or VTIMEZONE property kills the
   parse (#1051, #399). Bad values become `vBroken`, which keeps the raw text,
   and are logged in `component.errors` as `[(name, msg)]`. Exception types
   are mixed (#1448). Folding counts octets and never splits UTF-8 or a `\n`
   escape. A split escape had made Google drop events (#1501).
2. **Model.** `Component(CaselessDict)` maps names to values, with
   `subcomponents`. Typed subclasses add RFC metadata and Python properties.
   Cross-name order is lost, and `to_ical()` sorts by default. Unknown
   properties stay verbatim as `vUnknown`. Unknown components get an ad-hoc
   subclass.
3. **Date/time.** The value type is detected **from the string's shape**
   (length, `P`, `/`), not from `VALUE=`. A process-global `tzp` resolves
   TZIDs: the id, then a CLDR Windows map, then the `/vendor/Olson` suffix,
   then the raw id. An embedded VTIMEZONE is used (via `dateutil.tz.tzical`)
   only when the TZID is unknown, and it goes into a global first-wins cache.
   zoneinfo localization is `replace(tzinfo=…)`, which ignores gaps and folds
   (#649). An unresolved TZID becomes floating, silently.
   `Timezone.from_tzinfo` probes transitions and is slow (#799).
4. **Recurrence.** None. `vRecur` is a dict of lists that keeps RSCALE and
   SKIP.
5. **Durations.** Having both DTEND and DURATION raises `InvalidCalendar`, as
   does mixing a DATE with a DATE-TIME. With neither, a DATE lasts one day and
   a DATE-TIME lasts zero.
6. **Extension.** Mutate the class-level `TypesFactory.types_map`
   (`['x-foo'] = 'text'`) or subclass the factory. Components are added with
   `Component.register(Cls)`. There is no property registration API.

## Python: dateutil.rrule

HEAD 2642afa, 2.9.0.post0. `rrule.py` is essentially unchanged since 2021.

```python
r = rrule(WEEKLY, dtstart=dt0, count=10, byweekday=(MO, WE), cache=True)
r.between(a, b, inc=False); r.after(dt); r.before(dt); r.xafter(dt, count=5); r.count(); r[3]
s = rruleset(cache=True); s.rrule(r); s.rdate(d); s.exdate(x); s.exrule(r2)
rrulestr("DTSTART;TZID=Europe/Berlin:20260101T090000\nRRULE:FREQ=DAILY", forceset=True)
```

- **Iteration.** The `_iter` generator steps one period at a time from
  DTSTART using day masks, and every query walks linearly (#654). The `count`
  argument of `between()` is unused.
- **`cache=True`.** Appends 10 instances per lock acquisition into `_cache`
  from `_cache_gen`. Once complete it serves from the list. Memory grows to
  the furthest instant requested. Any `rruleset` mutation invalidates the
  cache.
- **Defaults and validation.** `dtstart=None` means *now*. `wkst=None` uses
  the process-global `calendar.firstweekday()`. `INTERVAL=0` hangs (#1441).
  An aware DTSTART with a naive UNTIL raises, which breaks DATE UNTILs
  (#1225).
- **`rrulestr`.** Rejects RSCALE, SKIP and X- parts (#1031). RDATE accepts
  `VALUE=DATE-TIME` only. DTSTART is not an instance unless the rule produces
  it, or `compatible=True` is set. `str()` drops the TZID and `Z` (#637).
- **DST.** `datetime.combine(..., tzinfo=dtstart.tzinfo)` does wall-clock
  arithmetic with no gap or fold handling, and sub-daily rules break across
  DST (#1351).
- **Limit.** Only `year > MAXYEAR` (9999) stops iteration. An exrule that
  cancels everything loops "forever" (#1393).

## Python: recurring-ical-events

HEAD b054fcb, v3.9.0. Built on icalendar, dateutil (with `cache=True`) and
x-wr-timezone.

```python
q = recurring_ical_events.of(cal, components=["VEVENT", "VTODO"], skip_bad_series=True)
q.between(start, end); q.at((2026, 10)); q.after(dt); q.all(); q.count(); q.first
for o in q.occurrences_between(start, end): o.start, o.end, o.id, o.as_component()
pages = q.paginate(page_size=50)                  # resumable via opaque OccurrenceID
```

- **Results.** Queries return shallow component copies with DTSTART
  rewritten, DTEND or DUE set, DURATION/RRULE/RDATE/EXDATE stripped,
  RECURRENCE-ID added, and SEQUENCE set to the series maximum. The v3.9
  `Occurrence` object defers that copying.
- **Overrides.** A `Series` is one UID within one component type. Its core is
  the highest-SEQUENCE non-override component. Overrides are keyed by **both**
  the UTC instant and the wall clock, which tolerates TZID mismatches.
  THISANDFUTURE applies the latest earlier range override, shifting later
  instances by `start − RECURRENCE-ID`. The resulting RECURRENCE-ID may be
  wrong (#222). An EXDATE also suppresses the override it names (#163).
- **RDATE, EXDATE and DTSTART.** RDATE accepts DATE, DATE-TIME and PERIOD; a
  PERIOD sets that instance's length. A DATE EXDATE removes the whole day.
  DTSTART is always included.
- **Time zones.** The first aware value sets the series zone, and floating
  values adopt the query's zone. UNTIL strings are rewritten when dateutil
  rejects them.
- **Limitations.** `after()` uses adaptive windows with an ever-growing
  dedup set. `between()` output is unsorted. RSCALE and SKIP are unsupported
  (#228).

## Java: ical4j

HEAD bd09358, 4.4.0 (2026-10-02). Java 11 or later; 596 VTIMEZONE files are
bundled.

```java
Calendar cal = new CalendarBuilder(CalendarParserFactory.getInstance().get(), ctx,
                                   TimeZoneRegistryFactory.getInstance().createRegistry()).build(in);
Optional<DtStart<ZonedDateTime>> s = event.getProperty(Property.DTSTART);
Recur<ZonedDateTime> r = new Recur.Builder<ZonedDateTime>().frequency(Frequency.WEEKLY).count(10).build();
List<ZonedDateTime> d = r.getDates(seed, periodStart, periodEnd);          // inclusive end
Set<Period<ZonedDateTime>> occ = event.calculateRecurrenceSet(period);     // half-open, eager
List<Period<ZonedDateTime>> all = new ComponentGroup<>(events, uid).calculateRecurrenceSet(period);
new CalendarOutputter(true).output(cal, out);
```

1. **Parse/serialize.** The parser is SAX-style (`ContentHandler` callbacks),
   but `CalendarBuilder` still builds the whole tree. A builder **can't be
   reused**: its registry holds state (#749). With several VCALENDARs in one
   stream it keeps only the **last**. `ParserException.getLineNo()` gives the
   line. There is no warnings channel: `withSuppressInvalidProperties` drops
   bad properties and logs a WARN. Leniency comes from **JVM-global**
   `CompatibilityHints` such as `ical4j.parsing.relaxed` and
   `ical4j.unfolding.relaxed`. Nesting is capped at 10 levels (#666). With the
   "validating" flag, `CalendarOutputter` **only logs**. Folding counts UTF-16
   units, at 73 by default.
2. **Model.** There are 85 property, 36 parameter and roughly 15 component
   classes. Lists are immutable but sit inside mutable containers.
   `getProperty` returns an `Optional`, and `replace()` moves the property to
   the end of the list. Unknown names become `XProperty`/`XComponent` even in
   strict mode. The children of an `XComponent` and **VALARM subcomponents are
   dropped**, which loses RFC 9074 VLOCATION (code reading).
3. **Date/time.** `DateProperty<T extends Temporal>` maps DATE to
   `LocalDate`, floating to `LocalDateTime`, TZID to `ZonedDateTime`, a
   trailing `Z` to `OffsetDateTime`, and UTC-only properties (DTSTAMP and the
   like) to `Instant`. A TZID is parsed lazily, so a VTIMEZONE that appears
   later still resolves. A VTIMEZONE becomes `ZoneRules` via
   `ZoneRulesBuilder`, registered under a **synthetic `ical4j-local-<n>`
   ZoneId** from a JVM-global pool of 1500; those ids leak into results
   (#661). Aliases come from `tz.alias` and Microsoft name tables, and TZURL
   refresh is opt-in.
4. **Recurrence.** `Recur` is immutable through its builder and supports
   RSCALE and SKIP. Parsing depends on `Locale.getDefault()`. It seeks by
   exponential then binary search, except under COUNT. There is always an end
   bound, and no cache. Budget: `net.fortuna.ical4j.recur.maxincrementcount`,
   default 1000 consecutive empty increments. Bugs: an `Instant` seed with
   WEEKLY throws (#639). INTERVAL>1 can skip the first instance (#293). 4.x is
   about 40% slower than 3.x (#715).
5. **Expansion.** Duration is taken from DURATION, otherwise DTEND minus
   DTSTART, otherwise 0 or 1 day. RDATE PERIOD values are kept. EXDATE
   matching uses `List.contains`, so a UTC EXDATE misses a zoned instance
   (code reading). Overrides apply **only** in `ComponentGroup`. Through 4.4.0
   it dropped rescheduled overrides (#510, fixed on HEAD). THISANDFUTURE is
   documented as unsupported.
6. **Extension.** Register via
   `ContentHandlerContext.withPropertyFactorySupplier(...)` and its parameter
   and component counterparts. ServiceLoader is opt-in, and its service files
   have drifted from the defaults. The supplier is invoked for every property,
   with a linear `supports()` scan, which probably explains #715. RFC 7986,
   9073, 7953 and 9253 are wired in. RFC 9074 is partial (PROXIMITY is
   unregistered).
7. **Validation.** `ValidationResult` entries carry a severity. Rules use
   `ValidationRule(One|OneOrLess|...)`. About 29 iTIP per-method validators
   are reachable through `validate(Method)`.

## PHP: sabre-io/vobject

HEAD 5e2f338, 5.0.0 (2026-07). Requires PHP 8.2 or later.

```php
$vcal = Reader::read($ics, Reader::OPTION_FORGIVING | Reader::OPTION_IGNORE_INVALID_LINES);
echo $vcal->VEVENT->SUMMARY;                     // magic: first match; foreach for all
$dt = $vcal->VEVENT->DTSTART->getDateTime($tz);  // hasTime(), isFloating()
$it = new EventIterator($vcal->VEVENT, null, $tz); $it->fastForward(new DateTimeImmutable('2026-01-01'));
while ($it->valid()) { $it->getDtStart(); $it->getEventObject(); $it->next(); }
$flat = $vcal->expand($start, $end);             // CalDAV expand; VEVENT only
$issues = $vcal->validate(Node::REPAIR);         // [['level'=>1..3,'message'=>..,'node'=>..]]
```

1. **Parse/serialize.** Readers build the whole tree, including
   `Splitter\ICalendar`, which parses everything and then yields one VCALENDAR
   per UID. The forgiving flags relax property names and *silently drop* bad
   lines. The first `ParseException` stops parsing; it includes the line, and
   properties keep `lineIndex`. `validate()` is separate from parsing and can
   repair (it adds UID and DTSTAMP).
2. **Model.** `children` are bucketed by name, so **cross-name order is
   lost**. `serialize()` reorders: VERSION first, VTIMEZONE before other
   components. A property's class comes from `VALUE=`, then `$propertyMap`,
   then `Unknown`.
3. **Date/time.** VTIMEZONE rules are **never interpreted** (#248).
   `TimeZoneUtil::getTimeZone` tries finders on the TZID (Olson, Windows,
   Lotus and Exchange maps, offsets), then guessers on the VTIMEZONE
   (`X-LIC-LOCATION`, `X-MICROSOFT-CDO-TZID`). If all fail it **silently uses
   the PHP default zone**. Both stages are pluggable. DST gaps and overlaps
   are left to PHP.
4. **Recurrence.** `RRuleIterator`'s `fastForward` is a linear loop. Unknown
   parts and BYDAY ordinals above 5 throw (#430). SECONDLY and MINUTELY never
   advance (code reading). Runaway rules stop only at year 9999 (#566, #329).
   `Settings::$maxRecurrences = 3500` throws, and its count includes the
   `fastForward` steps.
5. **Expansion.** `EventIterator` suppresses instances by RECURRENCE-ID
   timestamp and merges overrides back in by *their* DTSTART, so output stays
   in time order. RANGE is never read. **An RDATE makes it ignore the RRULE**
   (#700). RDATE ignores its own TZID and rejects PERIOD (#601). EXRULE is
   ignored. A DATE EXDATE never matches (#555). Duration is a fixed number of
   seconds, which drifts across DST. `expand()` drops non-VEVENT components
   and converts to UTC.
6. **Extension.** Global static `VCalendar::$componentMap`, `$propertyMap`
   and `$valueMap`. There is no parameter registry. Rules come from
   overriding `getValidationRules()`.
7. **iTIP.** `ITip\Broker::parseEvent()` and `processMessage()` handle
   `Message` objects for REQUEST, REPLY and CANCEL (VEVENT only). This is the
   most complete RFC 5546 prior art in the survey.

## .NET: ical.net

HEAD 7a5264f is pre-v6 and 99 commits past v5.2.3 (2026-06). NodaTime is the
only dependency. Where the two APIs differ, both are noted.

```csharp
var cal = Calendar.Load(ics);                                   // 5.2.3 ([Obsolete] on main)
var cal = CalendarSerializer.Deserialize<Calendar>(stream, opts);  // main; NET10: DeserializeAsyncEnumerable
foreach (var o in cal.GetOccurrences(startZoned).TakeWhileBefore(end.ToInstant()))  // main
    Console.WriteLine($"{o.Start} {o.End} {o.Source}");
var rule = RecurrenceRule.Parse("FREQ=WEEKLY;BYDAY=MO");        // main; 5.2.3: new RecurrencePattern(str)
```

1. **Parse/serialize.** 5.2.3 uses a regex-based `SimpleDeserializer`; its
   errors have no line numbers. Main adds a streaming UTF-8 `CalendarReader`
   with `LineNumber`, and `CalendarSerializerOptions{Converters,
   StrictParsing, OrderComponentProperties=true}`. `StrictParsing` is barely
   wired (#993). Neither version has a warnings channel.
2. **Model.** Typed components sit over a property list grouped by name, and
   output is sorted. Custom properties are added with `AddProperty(name,
   value)`. On main, an unknown property may stop at its first comma (code
   reading).
3. **Date/time.** `CalDateTime` holds a date, an optional time and an optional
   TZID (`HasTime`, `IsFloating`). Main converts through NodaTime
   (`ToZonedDateTime(provider)`). DST: a gap shifts forward and an overlap
   takes the earlier offset, using NodaTime's lenient resolver. 5.2.3 resolves
   TZIDs through a global resolver (TZDB, then Windows names, then fuzzy
   matching) and **ignores embedded VTIMEZONEs**. Main uses a per-calendar
   `TimeZoneProvider`: TZDB, aliases and Windows names first, then the
   calendar's VTIMEZONEs.
4. **Recurrence.** `GetOccurrences(start)` is lazy and may be infinite. There
   is no end parameter; callers use `TakeWhileBefore`. Main seeks
   arithmetically when there is no COUNT; 5.2.3 steps through linearly.
   `MaxUnmatchedIncrementsLimit` throws when exceeded, but its default is
   **null** (no limit). There is no cache.
5. **Expansion.** `Occurrence{Source, Start, End}` (5.2.3: `{Period, Source}`)
   refers to the master or override component without copying it. Overrides
   apply **only** in `Calendar.GetOccurrences`, keyed by (UID, RECURRENCE-ID
   instant), and the highest SEQUENCE wins. THISANDFUTURE is parsed but not
   evaluated (#455). Duplicate suppression assumes duplicates are adjacent
   (#866). Durations are the most RFC-faithful in the survey: DURATION weeks
   and days are nominal (wall clock) with the rest exact, while DTEND is
   exact. A DATE EXDATE removes the whole day (#829).
6. **Extension.** 5.2.3 has a `SerializationContext` service locator with
   `SerializerFactory`, and `DataTypeMapper` is internal. Main has
   per-property-name `CalendarPropertyConverter<T>`, a frozen internal
   component map, and no parameter or value-type registry.
7. **Pain points.** #986: an RRULE string accepted by 4.x fails in 5.x. #985:
   repeated BYMONTH values consume COUNT. DST end-time regressions: #883,
   #737, #736.

## C: libical

HEAD 7dba02b, 4.0.7-dev. The 4.x API is incompatible with 3.x; see
`docs/MigrationGuide_to_4.md`.

```c
icalcomponent *c = icalparser_add_line(p, line);           /* streaming; comp at top-level END */
icalcomponent *c = icalparser_parse_string(ics);  char *s = icalcomponent_as_ical_string_r(c);
struct icalrecurrencetype *r = icalrecurrencetype_new_from_string("FREQ=DAILY"); /* refcounted */
icalrecur_iterator *it = icalrecur_iterator_new(r, dtstart);
icalrecur_iterator_set_range(it, from, to);                /* false (MALFORMEDDATA) if COUNT */
for (struct icaltimetype t = icalrecur_iterator_next(it); !icaltime_is_null_time(t);
     t = icalrecur_iterator_next(it)) { }                  /* also _prev() */
icalcomponent_foreach_recurrence(ev, start, end, cb /* (comp, icaltime_span*, data) */, data);
```

1. **Parse/serialize.** The parser is line-oriented, incremental and lenient.
   Errors become `X-LIC-ERROR` **properties injected into the tree**, plus a
   thread-local `icalerrno`. Switches are **process-global**: unknown-token
   handling, control characters, and `icallimit_set` limits (defaults: 10k
   properties, 100 parameters, 10 MiB per value, 100k recurrence candidates,
   500 VTIMEZONE RRULE steps). Unknown IANA properties are treated as errors
   by default.
2. **Model.** A generic tree whose kinds and typed accessors are generated
   from `design-data/*.csv`. Properties and subcomponents are kept in separate
   ordered lists. The `get_first_*`/`get_next_*` cursors live **inside the
   component**, so they can't be nested; `icalcompiter` is the alternative.
3. **Date/time.** `struct icaltimetype{..., is_date, is_daylight, zone}`,
   where a NULL `zone` means floating. TZID lookup tries the ancestor
   VTIMEZONE, then builtin tzdata, then a bare Olson name. There is no Windows
   map. Each zone keeps a lazily extended, sorted change array (up to the
   current year plus 5, capped at 2582) behind a global mutex. A gap takes the
   earlier offset and an overlap the first instance; `is_daylight=1` selects
   the DST one instead (#1402, 4.0.6).
4. **Recurrence.** Refcounted rules with variable-length `by[]` arrays. RSCALE
   and SKIP are supported (non-Gregorian scales need ICU).
   `set_start`/`set_range` seek by modular arithmetic but refuse COUNT.
   `prev()` exists. Some YEARLY combinations are unsupported (#1276). There is
   no cache.
5. **Expansion.** `foreach_recurrence` uses only the first RRULE. It merges
   sorted RDATEs, matches DATE EXDATEs by date, and seeks to `start −
   duration` when there is no COUNT. EXRULE handling is O(n²). **RECURRENCE-ID
   overrides and THISANDFUTURE are not handled** (#314), and the callback gets
   only a `time_t` span.
6. **Extension.** There is no runtime registration: everything is generated at
   build time. The CSVs already cover RFC 5546, 6047, 6638, 7529, 7986, 9073,
   9074 and 9253, plus the VPOLL and VPATCH drafts. Process-global settings
   are a threading concern (#673).

## Rust: rrule (fmeringdal/rust-rrule)

HEAD 1c3420e, crate 0.14.0. Built on chrono and chrono-tz; optional features
`exrule`, `serde` and `by-easter`.

```rust
let set: RRuleSet = "DTSTART;TZID=Europe/Berlin:20260101T090000\nRRULE:FREQ=WEEKLY;COUNT=5".parse()?;
let RRuleResult { dates, limited } = set.clone().after(a).before(b).all(100);
let set = RRule::new(Frequency::Weekly).count(3).by_weekday(vec![NWeekday::Every(Weekday::Wed)]).build(dt_start)?;
for d in &set { /* DateTime<Tz> */ }
```

- **Typestate.** An `RRule<Unvalidated>` goes through `validate()`/`build()`
  against DTSTART, which also checks that UNTIL is UTC when DTSTART is zoned.
  Errors are `RRuleError{ParserError, ValidationError, IterError}`.
- **Strict parsing.** Rejects unknown parts, including RSCALE and SKIP (#95),
  and DTSTART or RDATE values in a gap or overlap. Accepts IANA zones only
  (#24).
- **Dates.** Floating values use `Tz::LOCAL`, and a DATE becomes midnight.
- **Limits.** `all(limit)` returns `limited` rather than silently truncating.
  Its 100k empty-loop cap applies only on that path.
- **No seek.** `after()`/`before()` filter after generation, and iterators
  can't be resumed (#42).
- **Round-trip.** Lossy: BYHOUR, BYMINUTE and BYSECOND are injected from
  DTSTART (#98).
- **Expansion bugs.** RDATE duplicates of RRULE instances (#150). DATE
  EXDATEs unsupported (#146). DTSTART not necessarily first (#119).

## Rust: icalendar (hoodie/icalendar-rs)

HEAD fed690e, crate 0.17.14. nom-based parser; optional features
`chrono-tz`, `recurrence` (rrule 0.14) and `strict-dates`.

```rust
let cal: Calendar = ics.parse()?;                   // parser::read_calendar(&parser::unfold(ics))
let ev = Event::new().summary("x").starts(start).ends(end)
    .append_property(Property::new("X-FOO", "bar").add_parameter("K", "v").done()).done();
if let Some(DatePerhapsTime::DateTime(CalendarDateTime::WithTimezone { date_time, tzid })) = ev.get_start() {}
```

- **Two tiers.** The borrowed parse tier (`parser::Component<'a>`) keeps
  ordered `Vec`s and is faithful. The owned builder tier uses `BTreeMap`s. A
  repeated property outside a 13-name multi list **collapses to its last
  value** (#81), parameters are sorted, and output is alphabetical. Writing
  the owned tier injects `DTSTAMP:<now>` and a random UID, so output is
  non-deterministic (#61).
- **Errors.** Parsing fails at the first error with a `String` from nom's
  `convert_error`. A fuzzer found exponential backtracking (#155, fixed).
- **Dates.** `CalendarDateTime::{Floating, Utc, WithTimezone}` plus
  `DatePerhapsTime::{DateTime, Date}` cleanly encodes the four RFC forms.
  VTIMEZONE is never interpreted, and `get_end` ignores DURATION.
- **Recurrence.** `get_recurrence()` rebuilds the text and re-parses it with
  rrule. DATE and floating values use the host's local zone (#175). There is
  no override merging.

---

## Go: emersion/go-ical

HEAD 439c63c (2025-06), `go 1.13`. Depends on `teambition/rrule-go v1.8.2`.

```go
dec := ical.NewDecoder(r); cal, err := dec.Decode()   // one VCALENDAR per call; io.EOF at end
err = ical.NewEncoder(w).Encode(cal)
type Component struct { Name string; Props Props; Children []*Component }
type Props  map[string][]Prop                          // keyed by upper-case name
type Prop   struct { Name string; Params Params; Value string } // raw value
type Params map[string][]string
func (p *Prop) DateTime(loc *time.Location) (time.Time, error)
func (c *Component) RecurrenceSet(loc *time.Location) (*rrule.Set, error)
type Event struct{ *Component }                        // typed view; cal.Events() []Event
```

1. **Parse/serialize.** `Decode()` can be called repeatedly, so concatenated
   calendars stream. It is strict: the first bad line aborts, and errors are
   `fmt.Errorf` strings without line numbers. The `Encoder` checks required
   and at-most-once properties *while encoding* and rejects CR/LF. It **does
   not fold**; the maintainer declined (#18).
2. **Model.** A generic tree of raw strings with typed accessors on demand
   (`Text`, `TextList`, `DateTime`, `Duration`, `Int`, `Bool`, `Binary`,
   `URI`). Unknown properties and components survive. **Order is lost**:
   `Props` is a map, and names are sorted on output. Raw `Props.Set` skips
   escaping while `SetText` escapes (#7).
3. **Date/time.** A floating value uses the `loc` argument, or UTC if `loc` is
   nil. A TZID goes straight to `time.LoadLocation`, and embedded VTIMEZONEs
   are ignored, so Outlook names fail (#10, open since 2021). #10 is blocked
   on golang/go#49951, which was declined. A DATE becomes midnight and loses
   its DATE-ness. `SetDateTime` writes a TZID even inside STANDARD/DAYLIGHT
   (#20).
4. **Recurrence.** Delegated: `Props.RecurrenceRule()` returns an
   `*rrule.ROption`.
5. **Expansion.** `RecurrenceSet` parses each EXDATE/RDATE property as a
   *single* value, so comma lists and `VALUE=PERIOD` fail. RECURRENCE-ID is
   ignored.
6. **Extension.** New properties are plain strings (RFC 7986 constants
   exist). Default value types live in an unexported map, so new properties
   need an explicit `VALUE=`.

## Go: arran4/golang-ical

HEAD 33ff6fb (2026-10), `go 1.20`.

```go
cal, err := ics.ParseCalendar(r)                                   // strict
cal, err := ics.ParseCalendarWithOptions(r,                         // options are ...any
    ics.WithPropertyParser(ics.FallbackParser(ics.LooseParser)),  // per-line recovery
    ics.WithUnknownPropertyHandler(ics.AcceptUnknownPropertyHandler), ics.WithWindowsTimezoneMapping())
err = cal.SerializeTo(w, ics.WithNewLineWindows, ics.WithLineLength(75))
type BaseProperty struct { IANAToken string; ICalParameters map[string][]string; Value string }
type ComponentBase struct { Properties []IANAProperty; Components []Component /* + tz mapper */ }
func ParseRecurrenceRule(s string) (*RecurrenceRule, error)        // parse/format only
```

1. **Parse/serialize.** Parses the whole calendar over an unfolding
   `CalendarStream`. Leniency is pluggable per content line through a
   `PropertyParser` function: `SkipPropertyParser`, `LooseParser`, or
   `FallbackParser(...)`. Errors are `*MalformedError{Line, Character, Err}`
   wrapping about 40 sentinels, so `errors.Is` works. Folding is rune-aware
   and prefers word boundaries. The newline defaults to the **OS** one, so
   output on Unix is LF (#116). Options are `...any`, resolved by a type
   switch, so a wrong option only fails at run time (`ErrInvalidOpArg`).
2. **Model.** Typed structs, with `GeneralComponent` for unknown components,
   over an **ordered slice** of raw properties. Parameters are a map. Tokens
   are matched with unanchored regexps, and time values with
   `^([0-9]{8})?([TZ])?([0-9]{6})?(Z)?$`.
3. **Date/time.** Floating times and DATEs become **`time.Local`** (#99,
   open). A TZID goes through an optional mapper, then `time.LoadLocation`.
   Before the Windows table existed, Outlook events were silently dropped
   (#137). VTIMEZONE is never interpreted (#74, #113).
4. **Recurrence and expansion.** None. `GetRDates`/`GetExDates` return
   `[]time.Time`.
5. **Extension.** Setters in `extensions/{microsoft,xapple,wr,wg}`. There is
   no registry.

## Go: teambition/rrule-go

HEAD e74d163 (2023-04), v1.8.2. A port of dateutil.

```go
r, err := rrule.NewRRule(rrule.ROption{Freq: rrule.WEEKLY, Dtstart: t0, Count: 10, Byweekday: []rrule.Weekday{rrule.MO}})
r, err := rrule.StrToRRule("DTSTART;TZID=America/New_York:20250101T090000\nRRULE:FREQ=DAILY")
next := r.Iterator()                 // type Next func() (value time.Time, ok bool)
ts := r.Between(after, before, inc)  // []time.Time; also All(), After(dt, inc), Before(dt, inc)
var s rrule.Set; s.RRule(r); s.RDate(t); s.ExDate(t)
```

- **Model.** The rule is an exported `ROption`. A zero `Dtstart` means
  `time.Now()`, so results depend on the wall clock (#44). There is **no
  result cache**, despite the doc comment. `Set` holds a single RRULE, since
  EXRULE was removed (#41).
- **Iteration.** Every query walks from DTSTART through a closure.
  Unmatchable rules run until year 9999.
- **DST.** Instances come from `time.Date(..., dtstart.Location())`, so gaps
  and overlaps resolve however Go's `time.Date` does. HOURLY rules produce
  duplicates or invalid instants across DST (#59, #63).
- **Other bugs.** UNTIL is always parsed as UTC (#67). Sub-second DTSTART
  values break comparisons (#56, #40). Users expect MONTHLY from Jan 30 to
  land in February (#58); RFC 5545 skips those dates, and RFC 7529 `SKIP=` is
  the fix.

---

## Implications for an idiomatic Go API

Each recommendation cites its sources. ***Current design*** notes compare it
with the library as it exists today. That design has a generic
`Component`/`Props`/`Property` tree with raw values and typed accessors;
typed views (`Event{Item}`, `Todo`, `Journal`, `Alarm`, `Timezone`); a
`Decoder` with `Strict`/`Warn`/`MaxLineLength`/`MaxDepth` fields and an
`Encoder`; `recur.Rule`/`recur.Set` with `iter.Seq` `Between`/`Times`, a pull
`recur.Iterator` and an explicit `recur.Cache`; `tz.Compile`/`tz.Load`; and
`Expander.Occurrences` returning `iter.Seq2[Occurrence, error]`.

### Parsing and serialization

1. **`Decoder` over `io.Reader` and `Encoder` over `io.Writer`, one object per
   `Decode` call.** Prior art: emersion's repeatable `Decode`, libical's
   `icalparser_add_line`, ical.net main's async enumerable. Avoid ical4j's
   builder, which keeps only the last VCALENDAR and can't be reused (#749).
   Put configuration in exported fields, as `encoding/csv.Reader` does. Avoid
   `...any` options (golang-ical) and global hints (ical4j
   `CompatibilityHints`, libical `ical_set_*`, ical.js `design.strict`).
   *Current design: matches.*
2. **Per-decoder leniency, out-of-band warnings, and positions on
   everything.** Keep the raw text of values that won't parse (python
   `vBroken`), rather than dropping lines (vobject) or injecting `X-LIC-ERROR`
   properties (libical). Line and column on every warning and error:
   golang-ical `MalformedError` and ical4j `getLineNo()` do this; most
   libraries don't. Never make leniency depend on the component type (python:
   only VEVENT is lenient, #1051).
   *Current design: matches (`Strict`/`Warn`); also route unresolved TZIDs
   to `Warn` (see 9).*
3. **Resource limits per decoder, not global.** Prior art: libical's
   `icallimit_set` defaults and ical4j's nesting cap of 10. *Current design:
   matches (`MaxLineLength`, `MaxDepth`); consider property and parameter
   counts too.*
4. **An encoder that is RFC-correct by default.** CRLF always (golang-ical
   #116). Fold at 75 **octets**, not UTF-16 units (ical4j), and never split a
   UTF-8 sequence or a `\n` escape (python #1501). emersion doesn't fold at
   all (#18). Escape in one place (emersion #7). Make validation a separate
   call that returns findings, like vobject `validate()` and ical4j
   `ValidationResult`, not something that happens inside `Encode` (emersion)
   or that only logs (ical4j).

### Data model

5. **Ordered, duplicate-preserving properties and parameters.** Store them in
   slices. Maps and name buckets lose order in emersion, vobject, python,
   ical.net and icalendar-rs, and icalendar-rs #81 also lost data. Keep raw
   value text and decode lazily (ical.js, emersion). Make typed views wrappers
   over the node (emersion `Event{*Component}`, ical.js `Event(component)`),
   not copies. Copies drift: ical4j's `replace()` reorders, and icalendar-rs's
   owned tier collapses repeated properties.
   *Current design: matches; confirm parameter order is a slice.*
6. **Value types come from `VALUE=` plus a registry, never from string shape**
   (python's `vDDDTypes` heuristic). Each registry entry carries the default
   type, the allowed types, multiplicity and cardinality. This is the hook RFC
   7986/9073/9074 plug into, as with ical.js design sets, python `types_map`,
   vobject maps and ical4j factories. Make it an **immutable value** passed on
   the `Decoder`/`Encoder` and extended with `reg.With(spec)`. Every surveyed
   registry is process-global and mutable, which is how ical4j ended up with
   drifting ServiceLoader files and a validator that depends on classpath
   order (#940). Keep unknown RRULE parts (RSCALE, SKIP, X-) as python
   `vRecur` and ical4j do, rather than rejecting them like dateutil, vobject
   and rust-rrule.

### Date/time

7. **Model the four RFC forms explicitly: DATE, floating, UTC, TZID.**
   Prior art: icalendar-rs `DatePerhapsTime`/`CalendarDateTime`, ical.net
   `CalDateTime{HasTime, IsFloating}`, ical.js `isDate` plus zone, and
   ical4j's `LocalDate`/`LocalDateTime`/`ZonedDateTime` split. Produce a
   `time.Time` only through a resolver. *Current design: matches
   (`p.DateTime(resolver)`).*
8. **Never default to `time.Local`, `time.Now()` or host locale settings.**
   Counter-examples: golang-ical #99, rust-rrule `Tz::LOCAL`, icalendar-rs
   #175, rrule-go and dateutil "DTSTART = now", ical4j
   `Locale.getDefault()`, dateutil `calendar.firstweekday()`. A floating
   time should require the caller to supply a location.
9. **Never silently fall back on an unknown TZID.** Current fallbacks:
   floating (ical.js #582, python), the PHP default zone (vobject),
   `time.Local` or a dropped event (golang-ical #137). Instead, emit a decoder
   warning and return a typed `ErrUnknownTZID` at resolve time, with an
   explicit opt-in fallback.
10. **Compile VTIMEZONE to `*time.Location` via TZif.**
    `LoadLocationFromTZData` is the only stdlib-compatible route, since
    golang/go#49951 was declined. Name the `Location` after the original TZID
    so round-trips re-emit it. ical4j's synthetic `ical4j-local-*` ids leak
    into results (#661). Cache per calendar, not globally. python's global
    first-wins cache collides across calendars, and ical4j's global id pool
    can run out. Precedence: embedded VTIMEZONE (libical, ical.js;
    RFC-faithful), then IANA, then Windows/CLDR aliases (python
    `WINDOWS_TO_OLSON`, ical4j `tz.alias`, vobject, golang-ical), then a
    caller hook. Optionally prefer IANA for known names, as python and
    ical.net main do. Bound observance expansion. A SECONDLY observance hangs
    ical.js (#1054); libical caps the search at 500.
    *Current design: matches (`tz.Compile`/`tz.Load`); document the
    precedence and make it configurable.*
11. **Implement RFC 5545 §3.3.5 in the library.** A gap takes the pre-gap
    offset, so the time moves forward. An overlap means the first
    occurrence, with an opt-in for the second like libical `is_daylight`.
    Go's `time.Date` leaves both unspecified, and rrule-go #59/#63 show the
    result. ical.js (#1051) shows the cost of reversing them. Test against
    both compiled and IANA zones.

### Recurrence

12. **`iter.Seq` for push, plus an independent pull iterator.** Return values,
    not shared mutable objects: ical.js `next()` reuses one `Time`, and
    libical cursors live inside the component. Never return a slice for an
    unbounded rule (rrule-go `All()`). The alternatives are ical4j's
    always-bounded `getDates` and rust-rrule's `all(limit)`. A serializable
    cursor (ical.js `toJSON`, recurring-ical-events `paginate`) makes paging
    possible.
    *Current design: matches.*
13. **Seek arithmetically when there is no COUNT,** as libical
    `__iterator_set_start`, ical.net `SkipToByInterval` and ical4j's binary
    search do. Never fake a seek by replacing DTSTART (ical.js #456) or by
    filtering after generation (rust-rrule, vobject `fastForward`).
14. **Let the cache solve COUNT.** Every engine walks COUNT rules from
    DTSTART. Cache (period, emitted count) checkpoints so later windows resume
    nearby. Avoid the unbounded growth of dateutil's `cache=True` and of
    recurring-ical-events' `after()` dedup set. Keep the cache explicit,
    bounded, and safe for concurrent use, keyed by rule plus DTSTART plus
    zone, and never hidden state on the rule.
    *Current design: matches (explicit `recur.Cache`).*
15. **Budget consecutive *empty* periods and fail loudly.** Count the way
    ical4j (1000) and libical do: not emitted or skipped instances, which is
    what breaks vobject's 3500 cap. Exhaustion is an error. ical.net throws,
    but its limit is off by default. rust-rrule returns a `limited` flag.
    Reject provably empty rules (BYMONTH=2;BYMONTHDAY=30) and INTERVAL=0
    (dateutil #1441) at construction. Never spin to year 9999 (dateutil,
    rrule-go, vobject).
16. **Normalize UNTIL against DTSTART's form, and keep sub-second values out
    of rule math.** rrule-go parses UNTIL as UTC unconditionally (#67) and
    breaks on sub-second DTSTARTs (#56). dateutil rejects a DATE UNTIL with a
    zoned DTSTART (#1225). recurring-ical-events rewrites UNTIL strings to
    cope.

### Occurrence expansion

17. **One expander owning overrides, EXDATE, RDATE and durations, yielding a
    small value.** The value: RecurrenceID, Start, End, a reference to the
    effective component, and flags for "overridden" and "from RDATE". Prior
    art: ical.net `Occurrence{Source, Start, End}`, ical.js
    `getOccurrenceDetails`, recurring-ical-events `Occurrence`. Don't copy
    components per instance (vobject `getEventObject`, recurring-ical-events
    `as_component`). Don't hide override handling behind a separate entry
    point: ical4j needs `ComponentGroup`, and ical.net applies overrides only
    through `Calendar`.
    *Current design: matches (`Expander.Occurrences` →
    `iter.Seq2[Occurrence, error]`).*
18. **Match overrides by UID plus instant.** Resolve floating values in the
    evaluation zone and let the highest SEQUENCE win, as ical.net main and
    recurring-ical-events do. ical.js keys on wall-time strings, which miss
    across TZIDs, and relates overrides without a UID check (#1019).
19. **Yield in start order, including moved and orphan overrides.** vobject
    merges overrides back by their own DTSTART; ical.js does neither (#285).
    This needs a look-ahead window sized by the largest override shift.
20. **Support RANGE=THISANDFUTURE from day one.** Follow ical.js and
    recurring-ical-events: use the latest earlier range override and shift
    later instances by its start delta and duration. Also handle stacked range
    overrides, which ical.js doesn't. Adding it later changes the occurrence
    type; ical.net #455 has been open since 2019.
21. **EXDATE and RDATE semantics.** A DATE EXDATE removes that local day, as
    in ical.js, recurring-ical-events, ical.net and libical. Contrast vobject
    #555, rust-rrule #146 and ical4j. Compare by instant across TZIDs and
    value kinds. Accept multi-valued and PERIOD RDATEs (emersion fails on
    both). RDATE adds to the RRULE rather than replacing it (vobject #700).
    De-duplicate coinciding instances (ical.js #1026, rust-rrule #150). Treat
    DTSTART as the first instance, as recurring-ical-events does (unlike
    dateutil and rust-rrule #119), and document it.
22. **Durations.** DURATION weeks and days are nominal (wall clock), and the
    rest is exact. DTEND − DTSTART is exact. A DATE with neither DTEND nor
    DURATION lasts one day; a DATE-TIME lasts zero. These are ical.net's
    `GetEnd` semantics, and python icalendar validates the same rules. vobject
    (fixed seconds) and ical.js (exact difference added on the wall clock)
    drift across DST.

### Errors, hygiene, later RFCs

23. **Typed errors plus sentinels.** For example `*ParseError{Line, Col,
    Err}`, `ErrUnknownTZID`, `ErrRuleBudget` and `ErrInvalidRule`, all usable
    with `errors.Is` and `errors.As`, following golang-ical's pattern. python
    #1448 and ical.net #993 show what mixed exception types cost.
24. **Avoid explicitly:**
    - *Parsing:* regexp parsing (golang-ical, python unfolding), type
      detection by string shape, maps for ordered data.
    - *Defaults:* process-global registries, settings and caches;
      `time.Local`, `time.Now()` and locale defaults; OS-dependent newlines.
    - *Silent behaviour:* falling back on an unknown TZID, dropping lines or
      events, unbounded memoization.
    - *API shape:* `...any` options, shared mutable iterator results,
      validation inside `Encode`.
    - *Tree pollution:* injected error properties (libical), default
      DTSTAMP/UID on write (icalendar-rs #61).
25. **Later RFCs.** RFC 5546 iTIP belongs in its own package over the tree.
    vobject `ITip\Broker` and ical4j's per-method validators are the prior
    art. Reserve `RSCALE`/`SKIP` fields in `recur.Rule` now for RFC 7529:
    libical and ical4j implement them, and SKIP answers rrule-go #58. Add RFC
    9073/9074 as registry entries plus typed views, and make sure the decoder
    keeps components nested under VALARM (ical4j drops them).
