---
sidebar_position: 4
sidebar_label: "Shell Scripts"
---

# Shell Scripts Reference

## Overview

Shell scripts in `tools/` automate code generation and development workflows. They are not general-purpose utilities — each has a specific, documented purpose and is designed to be run at a particular point in a development workflow.

## Script Reference

| Script | Purpose | When to run |
|--------|---------|-------------|
| `tools/gen-proto-go.sh` | Regenerates `proto-go/` from `.proto` sources | After any `.proto` file change |
| `tools/grpc-svc-gen.sh [Entity]` | Scaffolds a new gRPC service definition for a CRD type | When adding a new API resource |
| `tools/gazelle` | Updates Bazel BUILD files for Go packages and proto targets | After adding/removing Go files or proto definitions |
| `tools/test/generate-certs.sh` | Generates test certificates | For local TLS testing |

## gen-proto-go.sh

Builds `//proto/...` with Bazel, copies the generated `.pb.go` files into `proto-go/`, syncs dependency versions from `go/go.mod` into `proto-go/go.mod`, and runs `go mod tidy` in `proto-go/`.

Always run this after editing `.proto` files and check in both the proto change and the generated output together:

```bash
tools/gen-proto-go.sh
```

See [Protocol Buffers](protobuf.md) for the full code generation workflow.

## grpc-svc-gen.sh

Takes an entity name (e.g., `Pipeline`) and generates the scaffolding for a new gRPC service definition. This is the starting point when adding a new API resource type.

```bash
tools/grpc-svc-gen.sh [EntityName]
```

Example:

```bash
tools/grpc-svc-gen.sh Pipeline
```

Run without arguments to see the full usage message.

## gazelle

A symlink to the Bazel-managed Gazelle binary. Scans Go packages and proto directories and updates BUILD targets. Run from the repo root:

```bash
tools/gazelle
```

See [Bazel Build System](bazel.md) for context on when to run Gazelle.

## Conventions

- Scripts use bash.
- Each script includes a usage message — run any script without arguments to see it.
- Scripts are self-contained: there is no shared shell function library. Each script carries everything it needs.
- Do not add shared utilities across scripts; keep them independent.

## Related

- [How to Write APIs](../how-to-write-apis.md)
- [Bazel Build System](bazel.md)
- [Protocol Buffers](protobuf.md)
