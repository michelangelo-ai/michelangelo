---
name: pr-template
description: Create or update a GitHub PR using the repo's pull_request_template.md format. Use when creating a new PR or updating an existing PR body.
user-invocable: true
---

Create or update a GitHub PR following the repo's pull request template.

## Template Format

Always use this exact structure for the PR body, sourced from `.github/pull_request_template.md`:

```
**What type of PR is this? (check all applicable)**
- [ ] Refactor
- [ ] Feature
- [ ] Bug Fix
- [ ] Optimization
- [ ] Documentation Update

**What changed?**

<description of what changed>

**Why?**

<reason for the change>

**How did you test it?**

<testing approach>

**Potential risks**

<risks or "None">

**Release notes**

<release notes or "N/A">

**Documentation Changes**

<doc changes or "N/A">
```

## Your Task

1. Run `git diff main...HEAD` (or `git diff HEAD~1` if already on main) to understand the changes
2. Infer the PR type(s) from the diff and check the applicable boxes
3. Fill in all sections — keep each answer concise (1-3 sentences or bullet points)
4. **If creating a new PR:**
   - Check if already on a non-main branch; if on main, ask the user for a branch name
   - Push the branch if not yet pushed: `git push -u origin <branch>`
   - Create the PR: `gh pr create --title "<title>" --body "<body>"`
5. **If updating an existing PR:**
   - Get the current PR number: `gh pr view --json number -q '.number'`
   - Update the body: `gh pr edit <number> --body "<body>"`

## PR Title

- Use conventional commit format: `<type>: <short description>`
- Types: `feat`, `fix`, `docs`, `refactor`, `chore`, `test`, `perf`
- Keep under 70 characters
- Example: `docs: add PR template skill`

## Filling in the Template

- **What changed?** — What the code does differently now. Be concrete.
- **Why?** — The motivation: bug, user need, cleanup, requirement.
- **How did you test it?** — Commands run, manual steps, or "no functional changes".
- **Potential risks** — Anything that could break in production. Default to "None" for docs/chore changes.
- **Release notes** — Only notable if it's a schema change, migration, or config change. Otherwise "N/A".
- **Documentation Changes** — Note any doc updates or "N/A".
