---
sidebar_position: 2
sidebar_label: "Bazel Build System"
---

# Bazel Build System

## Overview

Michelangelo AI uses Bazel (version 7.4.1, see `.bazelversion`) for building all Go binaries, generating proto bindings, and running tests. Bazel's hermetic, reproducible builds mean that `bazel build` produces the same output regardless of what is installed on your machine — no implicit toolchain dependencies.

## Key Files

| File | Purpose |
|------|---------|
| `MODULE.bazel` | Dependency declarations (Bazel modules) |
| `BUILD.bazel` (root) | Workspace-level rules, Gazelle config, nogo linting setup |
| `go/BUILD.bazel` | Go workspace build targets |
| `proto/BUILD.bazel` | Proto workspace build targets |

Each subdirectory that contains Go packages or proto definitions has its own `BUILD.bazel` file managed by Gazelle.

## Gazelle

Gazelle auto-generates and updates `BUILD.bazel` files for Go packages and proto targets. Run it after adding or removing Go files or proto files:

```bash
tools/gazelle
```

**Never manually edit Go or proto BUILD targets** — Gazelle owns them and will overwrite manual changes on the next run. Non-Go, non-proto targets (custom rules, macros) can be added to `BUILD.bazel` files alongside Gazelle-managed targets, but must be placed in sections that Gazelle will not touch.

## Common Commands

```bash
# Build a specific target
bazel build //go/cmd/apiserver

# Run a binary
bazel run //go/cmd/apiserver

# Build all proto targets
bazel build //proto/...

# Run all Go and proto tests (matches CI)
bazel test //go/... //proto/... --build_tests_only

# Update Bazel module dependencies
bazel mod tidy
```

## Go + Bazel Together

Go modules and Bazel modules are separate dependency systems that must be kept in sync. After changing Go dependencies with `go mod tidy`, also run:

```bash
bazel mod tidy
```

from the repo root. See [Managing Go Dependencies](../manage-go-dependencies.md) for the full workflow.

## macOS Note

If Bazel fails with C++ toolchain errors on macOS, set the following environment variables before running Bazel commands:

```bash
export CC=clang
export CXX=clang++
```

Adding these to your shell profile (`.zshrc` or `.bashrc`) avoids having to set them for each session.

## Related

- [Managing Go Dependencies](../manage-go-dependencies.md)
- [Building from Source](../building-michelangelo-ai-from-source.md)
- [How to Write APIs](../how-to-write-apis.md)
- [Protocol Buffers](protobuf.md)
- [Shell Scripts Reference](shell-scripts.md)
