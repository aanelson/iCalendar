# RFC 5545 fixture tools

How `../recur.json`, `../examples.json` and `../ics/` were made from `docs/rfc/rfc5545.txt`.

- `gen_recur.py` transcribes the §3.8.5.3 recurrence examples, with verified errata applied, into
  explicit instance lists.
- `check_recur.py` re-derives every one of those lists independently, by brute force over the
  calendar with a hand-written condition per example, and reports any disagreement.
- `gen_examples.py` (with `findblocks.py`, `rfcclean.py` and `filt.py`) extracts every example
  block.
- `validate.py` checks the JSON files' format.

Run them with `python3 -I`; they use only the standard library.
