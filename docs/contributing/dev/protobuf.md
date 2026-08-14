---
sidebar_position: 1
sidebar_label: "Protocol Buffers"
---

# Protocol Buffers

## Overview

All Michelangelo AI API resources are defined in `.proto` files. The gRPC API, message types, and service contracts all live here. Proto files are the source of truth for what resources exist, what fields they have, and what operations the API server supports.

## Module Structure

- `proto/api/v2/` — source of truth for all service and message definitions
- `proto-go/` — generated Go bindings, **never edit directly**
- Proto files are organized per resource type: `pipeline_svc.proto`, `project_svc.proto`, etc.

The `proto-go/` directory is checked into the repo for convenience (so Go tools can consume the bindings without running Bazel), but it is always derived from `proto/api/v2/`. Any manual edits to `proto-go/` will be overwritten by the next code generation run.

## Code Generation Workflow

After editing any `.proto` file, regenerate the Go bindings:

1. If creating a new service: scaffold the proto file with `tools/grpc-svc-gen.sh [Entity]`, then edit the generated file. Otherwise, edit the existing `.proto` file in `proto/api/v2/` directly.
2. Run `tools/gazelle` to update BUILD targets
3. Run `bazel build //proto/...` to compile
4. Run `tools/gen-proto-go.sh` to regenerate alias `BUILD.bazel` files under `proto-go/`, sync dependency versions from `go/go.mod` into `proto-go/go.mod`, and run `go mod tidy` in `proto-go/`
5. Run `tools/gen-descriptors.sh` to regenerate `helm/michelangelo/files/descriptors.pb` — the compiled proto descriptor set Envoy's `grpc_json_transcoder` filter reads. It's built via `bazel build //proto/api/v2:v2_proto` (the same compilation `gen-proto-go.sh` already triggers, just requesting a different Bazel output group), so it needs no network access and can't drift from what the Go bindings compile against. Run this on **every** proto change, even one that looks Go-only or JS-only: it's a single deduplicated descriptor set for the whole tree, so a change to a shared/imported type can affect how an unrelated, already-JS-consumed service resolves. CI enforces this via `main.yml`'s `dirty-check` job, so a forgotten run fails the PR rather than merging stale.
6. Check in the `.proto` changes and both generated outputs (`proto-go/` and `helm/michelangelo/files/descriptors.pb`)

If the change adds, removes, or renames a service (not just a field or method) that the JS client should use, also update `javascript/packages/rpc/services.ts` and re-run `tools/gen-descriptors.sh` — the Envoy transcoder's allowlist (`helm/michelangelo/files/transcoder-services.json`) is the intersection of what `services.ts` references and what exists in the descriptor set, not every service that exists under `proto/api`, so a Go-only service never becomes web-reachable just by compiling. This half has its own CI backstop (`transcoder-services-check.yaml`) for a `services.ts` edit with no proto change. The actual JS/Python **client language bindings** (the generated TS/Python classes, as opposed to the descriptor set) are a separate concern on their own schedule — see [gen-grpc-client.sh](shell-scripts.md#gen-grpc-clientsh), wired into `javascript/`'s `yarn build`/`yarn setup`.

## Service Pattern

Each ML entity (Pipeline, InferenceServer, Model, etc.) has a corresponding `*_svc.proto` file that defines a gRPC service with standard CRUD methods. For example, `pipeline_svc.proto` defines `PipelineService` with `CreatePipeline`, `GetPipeline`, `ListPipelines`, `UpdatePipeline`, and `DeletePipeline` RPCs.

When adding a new API resource, follow this same pattern. The new entity gets its own `*_svc.proto` file in `proto/api/v2/`.

## gRPC Service Generation

Use `tools/grpc-svc-gen.sh` to scaffold a new service rather than copying and editing an existing file by hand:

```bash
tools/grpc-svc-gen.sh [EntityName]
```

Example:

```bash
tools/grpc-svc-gen.sh Pipeline
```

Run the script without arguments to see its full usage message.

## Versioning

All current APIs live under `proto/api/v2`. Breaking changes (field removals, type changes, incompatible method signature changes) require a new version directory (e.g., `proto/api/v3`). Additive changes (new fields, new RPCs) can go into the existing version.

## Related

- [How to Write APIs](../how-to-write-apis.md)
- [Building from Source](../building-michelangelo-ai-from-source.md)
- [Bazel Build System](bazel.md)
- [Shell Scripts Reference](shell-scripts.md)
