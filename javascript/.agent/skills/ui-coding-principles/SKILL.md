---
name: UI Coding Principles
description: "Trigger whenever any code will be written or changed — including features, refactors, and test changes. If the task ends with modified files, this skill applies. Skip only for pure read-only questions like explaining or debugging without changes."
user-invocable: false
---

# Coding Principles

## Follow Existing Patterns First

Before writing anything new:

1. **Find similar functionality** already in the codebase
2. **Follow the established approach** — same naming, structure, and conventions
3. **Establish new patterns carefully** — when nothing exists, make minimal decisions and be consistent

The right answer is almost always already in the codebase. Look before you invent.

## Minimal Complexity

The right amount of complexity is the minimum needed for the current task.

- **Don't design for hypothetical future requirements** — solve the problem in front of you
- **Don't abstract speculatively** — extract a helper when it clarifies intent or has a clear, reusable purpose; three similar lines that don't share meaningful behavior are better than a forced abstraction
- **Only add error handling for scenarios that can actually happen** — trust internal code and framework guarantees

## Avoid Premature Optimization

- Only optimize with data justifying it — profile before changing anything
- Unoptimized readable code is better than optimized unreadable code

## Code Ordering Within a File

Put the primary export first. Private helpers and implementation details should follow, not precede, the thing they support. Readers should see the main thing before the internals.
