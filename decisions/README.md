# Architecture Decision Records

This directory contains Architecture Decision Records (ADRs) for cross-cutting concerns that span multiple parts of the codebase.

Language live alongside the code they govern:

```
decisions/              ← cross-cutting decisions (this directory)
go/decisions/           ← Go-specific decisions
javascript/decisions/   ← JavaScript/TypeScript-specific decisions
python/decisions/       ← Python-specific decisions
```

## What is an ADR?

An ADR is a short document that records a significant architectural decision: what was decided, why, and what the consequences are. The goal is not to document every choice, but to leave a record of decisions that are non-obvious, affect how others contribute, or represent a deliberate tradeoff.

## When to write one

Write an ADR when:

- You are establishing a convention that others must follow
- Reasonable contributors might wonder why the codebase is shaped a certain way
- You considered multiple approaches and want to record why you chose one over the others
- The decision is hard to reverse

You do not need an ADR for bug fixes, routine features, or changes that are clearly within established conventions.

## Format

ADRs are numbered sequentially within each directory. Use this structure:

```markdown
# NNNN: Title

**Status**: Accepted | Deprecated | Superseded by [NNNN](NNNN-title.md)
**Date**: YYYY-MM-DD

## Context

What situation or problem prompted this decision? What constraints existed?

## Decision

What was decided?

## Consequences

What becomes easier or harder as a result? What follow-up work does this create?
```

Keep ADRs short. The context and consequences sections are more valuable than a detailed description of the implementation — the code explains the how; the ADR explains the why.

## Process

1. Copy the format above into `NNNN-short-title.md` in the appropriate `decisions/` directory
2. Open a PR with only the ADR (no code changes required)
3. Use the PR thread for discussion and revision
4. Merge once the team has reached alignment
5. Implementation PRs can reference the ADR as rationale

If a decision is later reversed, mark its status as `Superseded by` rather than deleting it — the history of why things changed is valuable.

## Index

| ADR | Title        | Status |
| --- | ------------ | ------ |
| —   | _(none yet)_ | —      |
