# rust-rrule

| | |
|---|---|
| Upstream | https://github.com/fmeringdal/rust-rrule |
| Commit | `1c3420ede20e6190040804121e1b5e1eddac6493` (branch `main`, committed 2025-04-20) |
| Cloned | 2026-10-11 (`git clone --depth 1`) |
| License | `MIT OR Apache-2.0` (see `LICENSE-MIT`, `LICENSE-APACHE`) |

## Copied

No fixture files (vectors only).

## Converted (`recur.json`, 203 cases)

From `rrule/src/tests/`, parsed as text (nothing compiled or run):

* `rfc_tests.rs` (36), `regression.rs` (1), `daylight_saving.rs` (2): a
  `"DTSTART;TZID=...\nRRULE:..."` string literal parsed as `RRuleSet`, `.all(N)` /
  `.all_unchecked()`, and `common::check_occurrences(&dates, &["RFC 3339", ...])`. The RFC 3339
  values are written as wall-clock basic form in the DTSTART zone (`tzid`); `.all(N)` with
  `N != u16::MAX` → `limit: N`.
* `rrule.rs` (164): `RRule { freq, count, interval, until, week_start, by_* , ..Default::default() }`
  struct literals + `test_recurring_rrule(rrule, limited, ymd_hms(...), &[ymd_hms(...), ...])`;
  all times are UTC (`Z`). `by_n_month_day` is merged into `BYMONTHDAY`; `NWeekday::Nth(n, D)` →
  `nDD`. The tests spell out `BYHOUR/BYMINUTE/BYSECOND` (and often `BYMONTH/BYMONTHDAY`) copied
  from DTSTART; these are kept, which is equivalent to the RFC defaults. `#[cfg(feature =
  "by-easter")]` tests carry `extensions: ["BYEASTER"]`.

As a dateutil port, DTSTART is not emitted unless it matches (`include_dtstart: false`).
Cross-check: 141 of the 154 rrule.rs cases that have a same-named dateutil test produce the
same instances; the 13 differences are tests whose rules differ upstream (e.g. `INTERVAL=40`
vs `100`, extra `COUNT`).

Script: `scripts/conv_rust.py` (scratch, not committed).

## Skipped

* `rfc_tests.rs`: `every_days_in_jan_for_3_years`, `weekly_on_tue_and_thu_for_5_weeks`,
  `every_20_min_at_time_every_day` (expected lists built with loops at run time).
* `rrule.rs`: 8 `before/after/between` bound tests and `test_timezones_weekly` (assert via
  builder API / weekday checks, no literal instance list).
* `regression.rs`: `issue_49`, `issue_61`, `issue_97`, `issue_111` (no instance list).
* `rruleset.rs` (24 tests, RRuleSet builder API with RDATE/EXDATE/EXRULE), `serde.rs`,
  `datetime.rs`.
