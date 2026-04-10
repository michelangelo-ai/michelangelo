---
name: Write pull request (PR) description
description: >-
  Trigger this skill when the user asks to write a pull request (PR) description, create a pull request, draft a commit message, or summarize changes for a PR. Produces a complete PR body.
user-invocable: true
---

# Write Pull Request Description

## Flow

1. **Align on scope** — Ask the developer which changes to include. Encourage staging the relevant files or providing git SHA(s). Check `git diff --staged` and `git diff` to understand what is ready.

2. **Read each changed file** — Summarize what changed in each file before drafting.

3. **Proactively ask clarifying questions** — If the motivation behind a change is unclear, ask rather than guess.

4. **Draft the PR title** — This becomes the commit subject line on `main` after squash & merge. Follow the title guidelines below.

5. **Draft the Summary** — Follow the principles and structure below.

6. **Gather the Test plan** — Ask the developer how they verified this change.
   - Which tests did you run? (unit, integration, e2e)
   - Any manual verification steps?
   - Did you test in a specific environment? (local, sandbox)
   - Screenshots or logs to include?

7. **Output the complete PR description** — Format as:

   ```
   Title: {pr title}

   ## Summary
   {narrative}

   ## Test plan
   {test details}
   ```

## PR Title Guidelines

The PR title becomes the commit subject line on `main`. It should:

- Be under ~50 characters when possible (hard limit: 72)
- Start with an action verb in imperative mood: "Add", "Fix", "Replace", "Remove"
- Describe the _outcome_, not the mechanism
- Omit articles ("a", "the") when they aren't needed for clarity
- Never end with a period

### Good titles

- `Add Redis cache for user profile reads`
- `Fix null pointer in batch job cleanup`
- `Replace polling with WebSocket notifications`
- `Remove deprecated v1 auth middleware`

### Bad titles

- `Update code` (too vague)
- `Fixed the bug where users couldn't log in when...` (too long, past tense)
- `cache.go, profile.go, handler.go changes` (lists files)

## Summary Core Principles

- Focus on **why** the change was made, not how it works
- Explain what was wrong before, how it works now, and why this solution was chosen
- Group related changes; separate distinct changes with blank lines
- Be concise, not persuasive
- Leave out implementation details — code explains the "how"

### Summary Structure

```
Action-oriented phrase describing what was done at a high level.
Use phrases like "Replace X with Y" or "Migrate from X to Y".

Explain the reasoning: what was wrong with the previous approach
and why it needed to change. Describe architectural constraints
or limitations that drove this decision.

Explain how the new approach solves the problem and why this
specific solution was chosen over alternatives. Include any
non-obvious side effects or consequences.
```

## Test Plan Principles

- Be specific: name the tests, commands, or manual steps
- Include evidence when possible (screenshots, log output, CI links)
- Mention what environments were tested
- If no tests were added, explain why (e.g., "refactor with no behavior change,
  existing tests cover this")

## Situational Guidelines

**Bug Fixes** — Summary should explain the root cause, not just the symptoms. Test plan should demonstrate the fix and ideally include a regression test.

**Refactoring** — Summary should explain why refactoring was necessary, what architectural problem(s) it solves, and what it enables. Test plan can reference existing tests if behavior is unchanged.

**Constant/Value Changes** — Don't detail the exact value. Focus on why the change was needed.

**Variable/Function Changes** — Don't detail renames or new exports. Explain why the function was needed and what problem it solves.

**New Features** — Summary should explain the user need driving the feature. Test plan should cover happy path and key edge cases.

**Configuration/Build Changes** — Summary should explain why the change was needed. Test plan should confirm the build/deploy still works.

## What NOT to Include in Summary

- How code works (code is self-explanatory)
- Variable name changes as standalone items
- Exact configuration file modifications
- Step-by-step implementation details
- Where functions are imported or exported
- Persuasive language about code quality

## Using Chat History

Reference earlier conversation context when it explains the architectural
motivations behind a change. Not all changes will have been discussed — assess
those in context without assuming a connection exists.

Verify what final changes were accepted by the developer, since many changes
may have been explored during a session.
