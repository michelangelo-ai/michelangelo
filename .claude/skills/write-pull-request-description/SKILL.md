---
name: Write pull request (PR) description
description: >-
  Trigger this skill when the user asks to write a pull request (PR)
  description, create a pull request, or summarize changes for a PR.
  Produces a complete PR title and body.
user-invocable: true
allowed-tools: Bash(git diff *) Bash(git log *) Bash(git merge-base *) Bash(git rev-parse *) Bash(git remote *) Bash(gh --version) Bash(gh auth status) Read(*/.github/*) Read(*/.github/**/*)
---

## Working directory rule

**Never chain `cd` with a `git` command.** `git` walks up the tree to
find the repo, so `cd <path> && git ...` is unnecessary. It also breaks
the `Bash(git log *)` / `Bash(git diff *)` allowlist patterns in this
skill's frontmatter, forcing a permission prompt on every command. Run
git commands directly from wherever the session started.

## Asking the developer questions

Use `AskUserQuestion` for any decision with a discrete answer set.
Reserve free text for narrative answers (motivation, usernames, summary
edits). Batch related structured questions into one call.

## Flow

1. **Load the diff and align on scope and intent** — run this exact command exactly once:

   ```bash
   git diff main...HEAD
   ```

   Do not run `--name-only` first or follow up with per-file `-- path/to/file` diffs.
   Everything you need is in this single call. Use the output to understand what changed,
   then confirm the intent and scope with the developer before drafting.

2. **Ask clarifying questions interactively.** Collect every clarification
   in one round before drafting. Only ask what you cannot infer from the
   diff. Common patterns:
   - Scope inclusion — [Include / Exclude / Move to separate PR]
   - Highlighting — [Mention / Brief mention / Skip]
   - Risk surfacing — [Document / Irrelevant]

   Ask narrative follow-ups (e.g. _why_ a change was made) in plain text
   after the structured questions.

3. **Draft the Summary** — Follow the principles and structure in **Summary Core Principles**. Then re-read each paragraph and ask: "Is this a mechanical list of changes the reviewer can reconstruct from the diff?" If yes, cut it.

4. **Draft the subject line** — Derive from the Summary. Follow the **Subject Line Rules**.

5. **Gather the Test plan** — Check the diff for new or modified test files.
   - Only ask how the change was verified when the diff shows no test changes
     **and** the change type warrants verification (e.g., UI, data migrations,
     infrastructure changes). Use a multi-select:
     - Manual testing in dev environment
     - Visual verification in browser
     - No testing — explain why

   For visual or manual verification, leave a placeholder like
   `[screenshot — add via GitHub UI]` so the developer can attach evidence
   after the PR is created.

6. **Assess potential risks** — Check whether the diff touches any of the
   triggers listed in the `## Potential risks` section of
   `.github/pull_request_template.md`. If any apply, ask via single-select:
   - No downstream impact
   - Yes — I'll describe it
   - Unsure — flag for reviewers

   If "Yes", follow up in plain text for the description. If no triggers
   apply, omit the section without asking.

7. **Output the complete commit message** — The first line is the subject
   (which becomes the PR title), followed by a blank line, then the body:

   ```
   Fix race condition in concurrent form submissions

   ## Summary
   {narrative}

   ## Test plan
   {test details}

   ## Potential Risks
   {optional}
   ```

8. **Offer to create the PR** — After outputting the description, ask via
   single-select:
   - Create as ready for review
   - Create as draft
   - No — just give me the description

   Stop if the developer chooses "No".

9. **Collect creation options** — Ask only for information that cannot be
   inferred:

- **Reviewers** — Ask: "Any specific reviewers to add? (Enter usernames
  or leave blank.)" Accept a comma-separated list or blank.
- **Base branch** — Infer from `git merge-base --fork-point main HEAD`
  and `git remote show origin | grep HEAD`. Use `main` as fallback.
  Do not ask unless the inferred base looks wrong (e.g., not `main`
  and not obviously a stack branch).

10. **Pre-flight checks** — Before running the command, verify:

    a. `gh` is installed: run `gh --version`. If it fails, output:
    "gh CLI is not installed. Install it from https://cli.github.com
    and authenticate with `gh auth login`, then re-run this step."
    Stop here.

    b. Authenticated: run `gh auth status`. If it fails, output:
    "gh is not authenticated. Run `gh auth login` and try again."
    Stop here.

    c. Branch is pushed: run `git rev-parse --abbrev-ref --symbolic-full-name @{u}`
    to check for an upstream. If there is no upstream, run
    `git push -u origin HEAD` and inform the developer:
    "Branch wasn't pushed yet — pushed it now."
    If push fails, output the error and stop.

11. **Run `gh pr create`** — Assemble and run the command:

    ```
    gh pr create \
      --title "<subject line from step 4>" \
      --body "<full body from step 7>" \
      --base "<inferred base branch>" \
      [--draft] \
      [--reviewer "<comma-separated usernames>"]
    ```

    Pass `--draft` only if the developer chose draft mode.
    Pass `--reviewer` only if at least one reviewer was supplied.

    After a successful run, output the PR URL:

    "PR created: <URL>"

    If the command fails, show the raw error and suggest the most likely
    fix. Do not retry automatically.

## Subject Line Rules

The subject line completes the sentence: "If applied, this commit will \_\_\_."

- Aim for around 50 characters
- Capitalize the first word
- Use the imperative mood: "Add", "Fix", "Replace", "Remove"
- Describe the _outcome_, not the mechanism
- Do not end with a period

## Summary Core Principles

- Explain what was wrong and why the approach solves it — not how the implementation works internally.
- If a sentence could be derived by reading the diff, cut it
- When renaming or restructuring, explain what motivated the change
- Be concise, not persuasive
- Wrap body lines at 72 characters

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

## Examples

### Example: Change with risks

```
Add verified_at column to users table

## Summary
The users table had no record of when email verification occurred,
making it impossible to audit which accounts verified before a
policy change. This adds a nullable `verified_at` column and
backfills it from the audit log for existing users.

## Test plan
- Ran migration against a staging snapshot;
- Verified backfill accuracy on 100 sampled accounts.

## Potential risks
- If the migration fails mid-run on production, the users table
  is left partially migrated. Recovery: run `db/rollback_verified_at.sql`
  to drop the column; no data is lost since the column is additive.
```

### Example: Trivial change

```
Fix typo in validation error message

## Summary
"Ivalid email" → "Invalid email" in the signup form error text.

## Test plan
- Visual verification in the browser. No logic change.
```

## Situational Guidelines

**Bug Fixes** — Summary should explain the root cause, not just the symptoms.
Test plan should demonstrate the fix and ideally include a regression test.

**Refactoring** — Summary should explain why refactoring was necessary, what
architectural problem(s) it solves, and what it enables.

**New Features** — Summary should explain the user need driving the feature.
Test plan should cover happy path and key edge cases.

**Creating the PR via gh CLI** — If `gh pr create` fails with "already exists",
the branch already has an open PR. Offer to run `gh pr edit` to update the title
and body instead:

    gh pr edit --title "<title>" --body "<body>"

## Using Chat History

Reference earlier conversation context when it explains the architectural
motivations behind a change. Not all changes will have been discussed — assess
those in context without assuming a connection exists.

Verify what final changes were accepted by the developer, since many changes
may have been explored during a session.
