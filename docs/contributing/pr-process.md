# PR Process

Contributing to Michelangelo follows a standard fork-and-PR workflow. This guide covers everything from branch creation to merge.

## Before You Open a PR

**For small changes** (typos, obvious single-line bug fixes): open a PR directly.

**For larger changes** (new features, API changes, significant refactors): open a GitHub issue first to discuss the approach. This prevents wasted work if the direction needs to change.

Ensure your branch is current before pushing:

```bash
git fetch origin
git rebase origin/main
```

## Branch Naming

Use descriptive, kebab-case branch names with a short prefix indicating the type of change:

| Prefix | Use for |
|--------|---------|
| `feat/` | New features |
| `fix/` | Bug fixes |
| `docs/` | Documentation |
| `refactor/` | Refactors with no behavior change |
| `test/` | Test additions or fixes |

**Examples**: `feat/spark-dynamic-allocation`, `fix/ray-job-timeout`, `docs/mlflow-integration`

## PR Description

A good PR description lets reviewers understand the change without asking questions. Include:

1. **What** — what does this change do?
2. **Why** — motivation, or link to the GitHub issue (e.g., `Closes #123`)
3. **How to test** — steps to verify the change works
4. **Breaking changes** — any migration steps required (API changes, config field renames, etc.)

## Checks That Must Pass

All of the following must pass before a PR is ready for merge:

```bash
# Go: build, vet, and test
bazel build //go/...
bazel test //go/...

# Python: lint and pre-commit hooks
cd python
poetry run pre-commit
poetry run ruff check .
poetry run ruff format --check .
poetry run pytest
```

All CI checks (GitHub Actions) must also be green.

## Review Process

- **At least one approving review** is required before merge
- **Address all comments** — either implement the suggestion or explain why you disagree. Don't leave comments unresolved
- **Re-request review** after addressing feedback — don't assume the reviewer will re-check on their own
- **Resolve conversations** once addressed — use the "Resolve" button rather than just replying

Reviewers will check for:
- Correctness and test coverage
- Adherence to the [error handling patterns](dev/go/error-handling.md) for Go code
- Adherence to [Python coding guidelines](dev/python/mactl/coding_guidelines.md) for Python code
- Documentation for any new user-facing behavior

## Merging

Maintainers merge approved PRs. Preferred strategies:

- **Squash merge** for feature and fix branches — keeps `main` history clean with one commit per logical change
- **Merge commit** for branches with meaningful commit history that should be preserved

After merge, delete your branch.

## Stacked PRs

If your change is large, consider splitting it into sequential PRs where each builds on the previous. Open them in order, set the base branch of PR 2 to PR 1's branch, and update the base to `main` after each merge. This makes review faster and reduces merge conflicts.

## Code Review Guarantee for Approved RFCs

If you are submitting code Pull Requests that implement a formally approved Track 2 RFC, your PR is protected by our **Code Review Guarantee**:

* **Focused Evaluations:** Core maintainers will evaluate your code PR strictly for code quality, architectural adherence, style guides, and test coverage.
* **No Reopened Debates:** Core design, framework choices, and structural layout debates will not be reopened during the code review stage because they were already settled during the RFC design phase.

### Exception: Unavoidable Post-Acceptance Design Blocks

In rare instances where an insurmountable architectural blocker is discovered *during* code implementation (like an unforeseen security risk or an unexpected engine performance degradation), the assigned Shepherd is empowered to pause implementation. The Shepherd will coordinate with the Core Architecture Panel to execute one of three explicit paths:

1. **Minor Amendment:** A quick, documented tweak to the existing RFC text so implementation can safely resume.
2. **Major Withdrawal:** The flaw is critical; the RFC is marked "Withdrawn," code PRs are paused, and a new design cycle must start.
3. **Deferment:** The RFC remains accepted, but implementation is blocked until an underlying platform dependency is resolved elsewhere.
