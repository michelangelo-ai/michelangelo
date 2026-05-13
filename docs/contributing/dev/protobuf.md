---
sidebar_position: 1
sidebar_label: "Protocol Buffers"
---

# Protocol Buffers in Michelangelo

## Role

All Michelangelo API resources are defined in `.proto` files. The gRPC API, message types, and service contracts all live here. Proto files are the source of truth for what resources exist, what fields they have, and what operations the API server supports.

## Module Structure

- `proto/api/v2/` — source of truth for all service and message definitions
- `proto-go/` — generated Go bindings, **never edit directly**
- Proto files are organized per resource type: `pipeline_svc.proto`, `project_svc.proto`, etc.

The `proto-go/` directory is checked into the repo for convenience (so Go tools can consume the bindings without running Bazel), but it is always derived from `proto/api/v2/`. Any manual edits to `proto-go/` will be overwritten by the next code generation run.

## Code Generation Workflow

After editing any `.proto` file, regenerate the Go bindings:

1. Edit `.proto` files in `proto/api/v2/`
2. Run `tools/gazelle` to update BUILD targets
3. Run `bazel build //proto/...` to compile
4. Run `tools/gen-proto-go.sh` to regenerate `proto-go/` and keep it in sync
5. Check in both the `.proto` changes and the generated `proto-go/` changes

Both the proto source and the generated output must be committed together so that the repo is always in a consistent state.

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
