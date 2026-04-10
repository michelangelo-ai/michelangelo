---
name: Write pull request (PR) description
description: >-
  Trigger this skill when the user asks to write a pull request (PR)
  description, create a pull request, or summarize changes for a PR.
  Produces a complete PR title and body.
user-invocable: true
---

## Flow

1. **Align on scope** — Run `git log --oneline main..HEAD` and `git diff main`
   to see what's on the branch. Present a brief summary and confirm with the
   developer: "These are the changes I see — should I cover all of them, or
   a subset?"

2. **Read each changed file** — Internally summarize what changed in each file
   to build context. Do not output this to the developer.

3. **Proactively ask clarifying questions** — If the motivation behind a change
   is unclear, ask rather than guess.

4. **Draft the Summary** — Follow the principles and structure below.

5. **Draft the subject line** — Derive from the Summary. Follow the subject
   line rules below.

6. **Gather the Test plan** — Check the diff for new or modified test files and
   note what you observe. Then ask the developer: "How did you verify this
   works?" Combine their answer with any test signals from the diff.

7. **Output the complete commit message** — The first line is the subject
   (which becomes the PR title), followed by a blank line, then the body:

   ```
   Fix race condition in concurrent form submissions

   ## Summary
   {narrative}

   ## Test plan
   {test details}
   ```

## Subject Line Rules

The subject line completes the sentence: "If applied, this commit will \_\_\_."

- Aim for around 50 characters — not a hard limit, but a rule of thumb
  that forces concise thinking
- Capitalize the first word
- Use the imperative mood: "Add", "Fix", "Replace", "Remove"
- Describe the _outcome_, not the mechanism
- Do not end with a period

## Summary Core Principles

- Explain what problem the code solves, not how the code works
- Explain what was wrong before, how it works now, and why this solution was chosen
- When renaming or restructuring, explain what motivated the change
- Summarize the approach at the decision level, not step-by-step
- Be concise, not persuasive
- Wrap body lines at 72 characters
- If the branch name or commit messages reference a ticket (e.g., PROJ-123,
  #456), include it. Ask the developer if unsure.

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

This structure is a ceiling, not a minimum. A one-line config change needs
one sentence, not three paragraphs.

### Example: Bug fix

```
Fix race condition in concurrent form submissions

## Summary
Concurrent form submissions could corrupt the session store
because the save handler read and wrote session state without
a lock. Under load testing, ~2% of submissions produced a 500.

This adds a per-session mutex around the read-modify-write
cycle. A lock-free approach using compare-and-swap was
considered but rejected because the session store doesn't
support atomic updates.

## Test plan
Added `TestConcurrentFormSubmission` — spawns 50 goroutines
submitting simultaneously and asserts zero errors. Ran the
existing integration suite locally; all passing.
```

### Example: Trivial change

```
Fix typo in validation error message

## Summary
"Ivalid email" → "Invalid email" in the signup form error text.

## Test plan
Visual verification in the browser. No logic change.
```

## Test Plan Principles

- Be specific: name the tests, commands, or manual steps
- Include evidence when possible (screenshots, log output)
- If no tests were added, explain why (e.g., "refactor with no behavior change,
  existing tests cover this")

## Situational Guidelines

**Bug Fixes** — Summary should explain the root cause, not just the symptoms.
Test plan should demonstrate the fix and ideally include a regression test.

**Refactoring** — Summary should explain why refactoring was necessary, what
architectural problem(s) it solves, and what it enables. Test plan can reference
existing tests if behavior is unchanged.

**Breaking Changes** — Call out the breaking change prominently in the Summary.
Name the affected API or behavior, what consumers need to change, and why the
break was necessary.

**New Features** — Summary should explain the user need driving the feature.
Test plan should cover happy path and key edge cases.

**Configuration/Build Changes** — Summary should explain why the change was
needed. Test plan should confirm the build/deploy still works.

## Tone

Write in a direct, technical tone. Use short sentences. Avoid hedging ("might",
"could potentially", "it seems"). The audience is busy engineers scanning PRs —
every sentence should earn its place.

## Using Chat History

Reference earlier conversation context when it explains the architectural
motivations behind a change. Not all changes will have been discussed — assess
those in context without assuming a connection exists.

Verify what final changes were accepted by the developer, since many changes
may have been explored during a session.
