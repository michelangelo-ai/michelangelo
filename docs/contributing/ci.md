# CI Pipeline Guide

This guide explains which CI jobs run on pull requests, what each one checks, how to interpret failures, and how Bazel caching works.

---

## Jobs That Run on Pull Requests

CI is path-filtered: jobs only run when their relevant files change. Opening a PR that only modifies docs skips all Go and Python build jobs.

### Go Components

| Job | Trigger | What it does |
|-----|---------|-------------|
| **Bazel Test** (`main.yml`) | Changes to `go/`, `proto/` | Builds and tests all Go packages and proto targets: `bazel test //go/... //proto/... --build_tests_only` |
| **Dirty Check** (`main.yml`) | Changes to `go/`, `proto/` | Runs Gazelle, `go mod tidy`, `goimports`, and `gen-proto-go.sh` — fails if any of these produce a diff |
| **Go Lint** (`go-lint.yml`) | Changes to `go/**/*.go` | Runs `golangci-lint`; also checks that all `TODO` comments reference a GitHub issue (`TODO(#123): description`) |

### Python Components

| Job | Trigger | What it does |
|-----|---------|-------------|
| **Python Build** (`main.yml`) | Changes to `python/` | Runs `poetry run pytest` against the full Python test suite |
| **Python Lint** (`lint.yaml`) | Changes to `python/` | Runs `ruff format --diff` and `ruff check` on changed `.py` files only; posts results as a PR comment |

### Documentation

| Job | Trigger | What it does |
|-----|---------|-------------|
| **Docs Check** (`docs-check.yml`) | Changes to `docs/`, `website/` | Checks that relative links include `.md` extensions (required for GitHub Pages); runs the Docusaurus build and annotates broken links |

### UI

| Job | Trigger | What it does |
|-----|---------|-------------|
| **UI Pre-Land** (`ui-pre-land.yaml`) | Changes to `javascript/` | Runs UI-specific checks (linting, tests, build) |

---

## Pre-Merge Requirements

A PR can merge only after all triggered jobs pass. Jobs that were skipped (because their path filters didn't match) do not block merge.

**Required for Go changes:**
- Bazel Test
- Dirty Check
- Go Lint (including TODO format check)

**Required for Python changes:**
- Python Build
- Python Lint

**Required for doc changes:**
- Docs Check (relative links + Docusaurus build)

---

## Reading CI Failures

### Bazel Test failure

Look for lines starting with `FAILED` or `ERROR` in the job log:

```
//go/components/scheduler/...:scheduler_test FAILED in 2.3s
```

Run the same target locally to reproduce:

```bash
./tools/bazel test //go/components/scheduler/...
```

To run only the failing test:

```bash
./tools/bazel test //go/components/scheduler/... \
  --test_filter=TestAssignmentStrategy
```

### Dirty Check failure

The job prints exactly what changed. Common causes:

| Message | Fix |
|---------|-----|
| `BUILD.bazel files are not up to date` | Run `tools/gazelle` |
| `go.mod or go.sum files are not up to date` | Run `cd go && ../tools/go mod tidy` from repo root |
| `Go files are not formatted` | Run `tools/goimports -w go` |
| `proto-go is out of date` | Run `tools/gen-proto-go.sh` |

All four checks must pass before pushing. Run them together:

```bash
tools/gazelle
cd go && ../tools/go mod tidy && cd ..
tools/goimports -w go
tools/gen-proto-go.sh
```

### Go Lint failure

The job posts a comment on the PR with lint output. Two distinct checks run:

**golangci-lint** — standard Go linter. Fix the specific rule violation shown. To run locally:

```bash
cd go && golangci-lint run ./...
```

**TODO format check** — every `TODO` must link to a GitHub issue:

```go
// ❌ Fails
// TODO: fix this later
// TODO - handle edge case

// ✅ Passes
// TODO(#456): fix this later
```

Create a GitHub issue for each unlinked `TODO`, then update the comment format.

### Python Lint failure

The job posts a PR comment with ruff output. Fix locally:

```bash
cd python

# Format
poetry run ruff format <changed-file.py>

# Lint (auto-fix where possible)
poetry run ruff check --fix <changed-file.py>
```

Or run the pre-commit hook which applies both:

```bash
cd python && poetry run pre-commit
```

### Docs Check failure

Two types of failures:

**Relative link without `.md` extension:**

```
::error file=docs/foo/bar.md,line=12::Relative link without .md extension will break on GitHub Pages
```

Fix: change `[link](../other-guide)` to `[link](../other-guide.md)`.

**Broken link (Docusaurus build failure):**

```
::error file=docs/foo/bar.md,line=8::Broken link: '../non-existent.md' could not be resolved
```

Fix: verify the target file exists or update the link path.

To run the Docusaurus build locally:

```bash
cd website
bun install --frozen-lockfile
bun run build
```

---

## Bazel Caching

Bazel outputs are cached in GitHub Actions using `actions/cache` keyed on:

```
bazel-main-<os>-<hash of .bazelversion + MODULE.bazel + MODULE.bazel.lock>
```

**What this means in practice:**

- If you only change `.go` source files (not `MODULE.bazel`), the cache is restored from a previous run and only the changed targets rebuild.
- If you change `MODULE.bazel` or `MODULE.bazel.lock` (e.g., adding a new Go dependency), the cache key changes and a full rebuild runs. This is expected behavior.
- The `dirty-check` job restores the cache read-only (no redundant save) so it benefits from the cache built by the `bazel-build` job.

**If CI is slower than expected:** Check whether `MODULE.bazel.lock` has changed in your PR. If it has, a cold-cache build is expected. If it hasn't, the cache should have been restored — look for "Cache restored" in the "Cache Bazel Outputs" step.

---

## Re-Triggering Jobs

GitHub Actions jobs can be re-triggered from the PR "Checks" tab:

- Click the failed job name
- Click **Re-run failed jobs** (top right)

Re-run the full workflow to pick up any external dependency or flake:

- Click **Re-run all jobs**

There is no CI skip mechanism for production checks. Do not use `[skip ci]` commit annotations on PRs targeting `main`.

---

## Local Pre-Flight Checklist

Run these before pushing to avoid unnecessary CI cycles:

```bash
# Go
tools/gazelle
cd go && ../tools/go mod tidy && cd ..
tools/goimports -w go
./tools/bazel test //go/... //proto/... --build_tests_only

# Python (from python/)
poetry run pre-commit
poetry run pytest

# Docs (from website/)
bun install --frozen-lockfile && bun run build
```

---

## Related

- [Building from Source](building-michelangelo-ai-from-source.md)
- [PR & Review Process](pr-process.md)
- [Testing Strategy](testing.md)
- [Managing Go Dependencies](manage-go-dependencies.md)
