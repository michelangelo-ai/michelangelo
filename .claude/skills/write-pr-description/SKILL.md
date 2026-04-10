---
name: Write PR description
description: >-
  Trigger this skill when the user asks to write a PR description, create a pull
  request, draft a commit message, or summarize changes for a PR. Produces a
  complete PR body (Summary + Test plan) ready for squash & merge.
user-invocable: true
---

# Writing PR Descriptions

Since we squash & merge, the PR body becomes the commit message on `main`.
This skill produces a complete, paste-ready PR description that works well
in both contexts: as a PR for reviewers and as a permanent commit message.

## Flow

1. **Align on scope** — Ask the developer which changes to include. Encourage
   staging the relevant files or providing git SHA(s). Check `git diff --staged`
   and `git diff` to understand what is ready.

2. **Read each changed file** — Summarize what changed in each file before
   drafting.

3. **Draft the Summary** — Follow the principles and structure below.

4. **Ask clarifying questions freely** — If the motivation behind a change is
   unclear, ask rather than guess.

5. **Gather the Test plan** — Ask the developer what they ran to verify this
   change. Prompt for specifics:
   - Which tests did you run? (unit, integration, e2e)
   - Any manual verification steps?
   - Did you test in a specific environment? (local, staging, CI)
   - Screenshots or logs to include?

6. **Output the complete PR description** — Format as:

   ```
   ## Summary
   {narrative}

   ## Test plan
   {test details}
   ```

## Summary Principles

- Focus on **why** the change was made, not how it works
- Explain what was wrong before, how it works now, and why this solution was
  chosen
- Write a short narrative, not a list of files
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

### Example Summary

> To reduce DB load during peak traffic, this adds a Redis cache for the
> User Profile service, replacing direct PostgreSQL reads on the hot path.
>
> Previous approach hit the database on every profile lookup, which caused
> p99 latency spikes during peak hours. The cache sits in front of the
> existing repository layer with a 5-minute TTL, keeping the invalidation
> window acceptable for profile data that changes infrequently.

## Test Plan Principles

- Be specific: name the tests, commands, or manual steps
- Include evidence when possible (screenshots, log output, CI links)
- Mention what environments were tested
- If no tests were added, explain why (e.g., "refactor with no behavior change,
  existing tests cover this")

## Situational Guidelines

**Bug Fixes** — Summary should explain the root cause, not just the symptoms.
Test plan should demonstrate the fix and ideally include a regression test.

**Refactoring** — Summary should explain why refactoring was necessary and
what it enables. Test plan can reference existing tests if behavior is unchanged.

**New Features** — Summary should explain the user need driving the feature.
Test plan should cover happy path and key edge cases.

**Configuration/Build Changes** — Summary should explain why the change was
needed. Test plan should confirm the build/deploy still works.

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
