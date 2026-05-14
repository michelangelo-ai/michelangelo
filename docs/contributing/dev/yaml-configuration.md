---
sidebar_position: 5
sidebar_label: "YAML Configuration"
---

# YAML Configuration Reference

## Overview

YAML is used throughout the repo for build configuration, CI/CD pipelines, linting rules, and Kubernetes manifests. This page catalogs the key files and their purposes so contributors know where to look when modifying tooling or deployment configuration.

## Configuration Files

| File | Purpose |
|------|---------|
| `.pre-commit-config.yaml` | Pre-commit hook definitions — ruff lint/format, trailing whitespace, YAML validation |
| `go/.golangci.yml` | Go linting rules for golangci-lint: enabled linters, per-linter settings, excluded paths |
| `.github/codecov.yml` | Codecov settings for test coverage reporting |
| `.github/workflows/` | GitHub Actions CI/CD pipelines — build, test, lint, docs |
| `.bazelversion` | Pins the Bazel version for the repo (currently 7.4.1) |

## Go Linting (golangci-lint)

Go linting is configured in `go/.golangci.yml`. Key enabled linters:

- `godox` — enforces `TODO(#issue)` format (TODOs without an issue number fail CI)
- `gofmt` — formatting check
- `govet` — Go vet checks
- `errcheck` — enforces that errors are not silently discarded

To run golangci-lint locally:

```bash
cd go
golangci-lint run ./...
```

Requires golangci-lint to be installed. See the [golangci-lint installation docs](https://golangci-lint.run/welcome/install/) for options.

## Pre-commit

`.pre-commit-config.yaml` runs automatically on `git commit` if pre-commit is installed. Hooks include ruff lint and format checks for Python, trailing whitespace removal, and YAML syntax validation.

To install and activate:

```bash
pip install pre-commit
pre-commit install
```

For manual runs without committing:

```bash
cd python
poetry run pre-commit
```

See [Python Utilities](python-utilities.md) for more on the Python tooling setup.

## Kubernetes Manifests

Kubernetes resource definitions (Deployments, Services, ConfigMaps) for local sandbox setup live in `python/michelangelo/sandbox/`. These are applied by `sandbox.py` via `kubectl apply` during `ma sandbox create`. Modifying these files changes what gets deployed when a contributor creates a local sandbox.

## Helm Chart Values

The Michelangelo Helm chart lives in `helm/michelangelo/`. Key files:

| File | Purpose |
|------|---------|
| `helm/michelangelo/values.yaml` | Production defaults |
| `helm/michelangelo/values-k3d.yaml` | Local k3d overrides for development |

See the [Platform Setup guide](../../operator-guides/setup/platform-setup.md) for Helm configuration details.

## Related

- [Building from Source](../building-michelangelo-ai-from-source.md)
- [Bazel Build System](bazel.md)
- [Python Utilities](python-utilities.md)
- [Platform Setup](../../operator-guides/setup/platform-setup.md)
