# Calibration Notes

Lessons from live calibration runs — cases where test coverage looked reasonable but missed the mark. Use these to correct recurring misjudgments.

Review this file quarterly: if a Rule below has recurred a second time in an unrelated PR, promote it into `javascript/CLAUDE.md` directly and remove it from here. A single occurrence stays parked — it isn't yet a pattern.

---

## 1. Don't test a derived lookup table against the object it was derived from

**Source:** PR #1673, `config/entities/model/__tests__/constants.test.ts` (`MODEL_KIND_TEXT_MAP`)

A test asserted `Object.entries(MODEL_KIND).forEach(...)` produces a string in `MODEL_KIND_TEXT_MAP`. Both objects live in the same file, and the map's keys are literally `MODEL_KIND.X` — so the test could never fail without the source file itself failing to compile. It also breaks under a refactor (e.g. swapping the map for a function) that wouldn't change any observable behavior.

**Rule:** When a value is produced by a translation/lookup step keyed off a local constant, don't write a unit test whose expected values are derived from that same constant — it can only prove internal consistency, not correctness, and it couples the test to the specific mechanism (map vs. function vs. switch) rather than the outcome. Test the translation through the boundary that consumes it (e.g. render the component/column with a raw input value and assert the displayed output), so the test survives a change to the mechanism as long as the observable behavior is unchanged.
