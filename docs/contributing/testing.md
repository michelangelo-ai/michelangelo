# Testing Strategy

Michelangelo uses a three-level test strategy. Understanding when to write each type of test — and how to run them — is the most important thing to know before opening a PR.

## Test Levels

### Unit Tests

Unit tests verify individual functions and types in isolation. They run fast and have no external dependencies.

**Go**: Unit tests live in the same package as the code, in `*_test.go` files.

```bash
# Run all Go unit tests
bazel test //go/...

# Run tests for a specific package
bazel test //go/components/jobs/scheduler/...

# With standard go tooling (from go/ directory)
go test ./...
go test ./components/jobs/scheduler/...
```

Use gomock to mock dependencies. See [Using Go Mocks in Unit Tests](use-go-mocks-in-unit-test.md) for patterns.

**Python**: Unit tests live in `python/tests/`.

```bash
cd python
poetry run pytest

# Run a specific test file
poetry run pytest tests/test_uniflow.py

# With verbose output
poetry run pytest -v
```

Use `unittest.mock` or `pytest-mock` for mocking Python dependencies.

### Integration Tests (Sandbox)

Integration tests validate end-to-end flows that require the Michelangelo control plane — API server, controller manager, and worker all running together.

**Setup**: The sandbox creates a local Kubernetes cluster (k3d) with all Michelangelo components:

```bash
cd python
poetry run ma sandbox create
```

This takes a few minutes on first run. See [Sandbox Setup](../getting-started/sandbox-setup.md) for full prerequisites.

**Running**: Submit a workflow or API request against the sandbox and verify the result:

```bash
# Example: run a Uniflow pipeline locally against the sandbox
poetry run python my_workflow.py

# Example: use the ma CLI against the sandbox
poetry run ma pipeline list
```

Integration tests are most important for:
- New Uniflow plugins (verify the full Go worker → Starlark → Python round-trip)
- Controller changes (verify the reconcile loop reaches the expected state)
- API changes (verify the gRPC endpoint and Kubernetes CRD interact correctly)

### End-to-End Tests

E2E tests run full production-like scenarios — typically a complete pipeline execution from submission to completion — in the sandbox or a staging environment. These run as part of CI on PRs that touch core execution paths.

## What to Test for Each Change Type

| Change type | Required tests |
|---|---|
| New Go controller | Unit tests for reconcile logic with mocked K8s client; integration test verifying CRD state transitions |
| New Uniflow plugin | Unit tests for the Starlark module builtins; integration test submitting a workflow that uses the plugin |
| New API field (proto) | Unit tests for any validation logic; verify the field round-trips through the API server |
| Python SDK change | Unit tests for the changed behavior; smoke test with `local run` mode |
| Bug fix | A regression test that fails on the code before your fix and passes after |
| Refactor | Existing tests should continue to pass unchanged; add tests for any previously untested paths you discover |

## Running Tests Before a PR

Run this locally before pushing to catch failures early:

```bash
# Go
bazel build //go/...
bazel test //go/...

# Python
cd python
poetry run pre-commit
poetry run ruff check .
poetry run pytest
```

The `pre-commit` hook runs lint, formatting, and import checks. Fix any issues it reports before committing.

## Test Coverage

There is no enforced coverage percentage, but reviewers will ask for tests if they're missing for new logic. A good rule of thumb: any code path that has business logic (not just delegation or plumbing) should have a test.

Don't add tests purely for coverage. Focus on testing behavior that could break and would be hard to catch manually.

## Flaky Tests

If a test you didn't touch starts failing intermittently on your PR, check the test's history in CI before assuming your change caused it. Flaky tests should be fixed or quarantined separately — don't work around them in your PR.
