# TODO Comment Enforcement

This document describes the TODO comment enforcement system for the Go codebase.

## Purpose

Ensure all TODO comments are linked to GitHub issues for proper tracking and prioritization of technical debt.

## TODO Comment Format

All TODO comments **must** follow one of these formats:

```go
// TODO(#123): Description of what needs to be done
// TODO(https://github.com/org/repo/issues/123): Description
```

### ❌ Invalid Formats

```go
// TODO: Description  (missing issue link)
// TODO Description   (missing issue link and colon)
// TODO - Description (missing issue link)
```

### ✅ Valid Formats

```go
// TODO(#546): Fix typo in workflow name and make this configurable
// TODO(https://github.com/michelangelo-ai/michelangelo/issues/547): Make this configurable
```

## Enforcement

### CI/CD Pipeline

The `go-lint.yml` GitHub Actions workflow runs on every pull request and:

1. **Detects unlinked TODOs**: Scans all Go files (excluding `gen/` and `thirdparty/`)
2. **Comments on PR**: Lists any TODO comments without proper issue links
3. **Fails the build**: If unlinked TODOs are found

### Local Development

#### Install golangci-lint

```bash
# macOS
brew install golangci-lint

# Linux/WSL
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin
```

#### Run locally

```bash
cd go
golangci-lint run ./...
```

#### Check for unlinked TODOs

```bash
cd go
grep -rn "TODO" --include="*.go" --exclude-dir=gen --exclude-dir=thirdparty . | grep -v "TODO(#" | grep -v "TODO(https://"
```

## Workflow for Adding TODOs

1. **Create a GitHub issue** for the technical debt/future work
2. **Add TODO comment** with the issue link:
   ```go
   // TODO(#123): Description matching the issue title
   ```
3. **Commit and push** - The CI will validate the format

## Why This Matters

- **Visibility**: All technical debt is tracked in GitHub issues
- **Prioritization**: Issues can be labeled, assigned, and prioritized
- **Accountability**: Clear ownership and due dates
- **Metrics**: Track technical debt over time
- **No forgotten TODOs**: Nothing falls through the cracks

## Configuration

### golangci-lint Configuration

Located in `go/.golangci.yml`:

```yaml
linters:
  enable:
    - godox  # Detects TODO/FIXME/BUG comments

linters-settings:
  godox:
    keywords:
      - TODO
      - FIXME
      - BUG
      - HACK
      - NOTE
```

### GitHub Actions Workflow

Located in `.github/workflows/go-lint.yml`:

- Runs on every PR that touches Go code
- Uses golangci-lint with godox linter
- Checks for unlinked TODO comments
- Comments on PR with results
- Fails build if issues found

## Excluding Files

The following directories are excluded from TODO enforcement:

- `go/gen/` - Generated code
- `go/thirdparty/` - Third-party code

## Related

- [TODO Tracking PR #567](https://github.com/michelangelo-ai/michelangelo/pull/567) - Initial TODO linking
- [Issue #542](https://github.com/michelangelo-ai/michelangelo/issues/542) - TODO tracking initiative
