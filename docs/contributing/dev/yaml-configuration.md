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
| `.pre-commit-config.yaml` | Pre-commit hook definitions — ruff lint, ruff format, prettier |
| `go/.golangci.yml` | Go linting rules for golangci-lint: enabled linters, per-linter settings, excluded paths |
| `.github/codecov.yml` | Codecov settings for test coverage reporting |
| `.github/workflows/` | GitHub Actions CI/CD pipelines — build, test, lint, docs |
| `.bazelversion` | Pins the Bazel version for the repo (currently 7.4.1) |

## Go Linting (golangci-lint)

Go linting is configured in `go/.golangci.yml`. All linters are disabled except `godox`, which enforces that every TODO/FIXME comment references a GitHub issue number (`TODO(#123): description`). TODOs without an issue reference fail CI.

To run golangci-lint locally:

```bash
cd go
golangci-lint run ./...
```

Requires golangci-lint to be installed. See the [golangci-lint installation docs](https://golangci-lint.run/welcome/install/) for options.

## Pre-commit

`.pre-commit-config.yaml` runs automatically on `git commit` if pre-commit is installed. Three hooks are defined: `ruff-lint` (Python linting), `ruff-format` (Python formatting), and `prettier` (formatter for JS/TS/JSON/CSS/YAML/Markdown).

To install and activate (assumes `poetry install -E dev` has been run — see [Python Utilities](python-utilities.md)):

```bash
cd python && poetry run pre-commit install
```

For manual runs without committing:

```bash
cd python && poetry run pre-commit run --all-files
```

See [Python Utilities](python-utilities.md) for more on the Python tooling setup.

## Kubernetes Manifests

Kubernetes resource definitions (Deployments, Services, ConfigMaps) for local sandbox setup live in `python/michelangelo/cli/sandbox/resources/`. These are applied by `sandbox.py` via `kubectl apply` during `ma sandbox create`. Modifying these files changes what gets deployed when a contributor creates a local sandbox.

## Helm Chart Values

The Michelangelo Helm chart lives in `helm/michelangelo/`. Key files:

| File | Purpose |
|------|---------|
| `helm/michelangelo/values.yaml` | Production defaults |
| `helm/michelangelo/values-k3d.yaml` | Local k3d overrides for development |

See the [Platform Setup guide](../../operator-guides/platform-setup.md) for Helm configuration details.

## Related

- [Building from Source](../building-michelangelo-ai-from-source.md)
- [Bazel Build System](bazel.md)
- [Python Utilities](python-utilities.md)
- [Platform Setup](../../operator-guides/platform-setup.md)
