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
| `tools/gen-grpc-client.sh` | Generates gRPC client code (Python and JavaScript) from protobuf files; also regenerates the Envoy transcoder's descriptor set and services allowlist | After proto changes that affect client stubs — but you rarely run this directly, see below |
| `tools/gen-descriptors.sh` | Builds `helm/michelangelo/files/descriptors.pb` + `transcoder-services.json` from `proto/api` (called by `gen-grpc-client.sh`) | Not run directly; invoked by `gen-grpc-client.sh` and `check-transcoder-services.sh` |
| `tools/check-transcoder-services.sh` | Verifies `helm/michelangelo/files/transcoder-services.json` is what `gen-descriptors.sh` would produce right now | CI backstop on any `proto/**` or `helm/michelangelo/files/**` change; run locally to reproduce a CI failure |
| `tools/grpc-svc-gen.sh [Entity]` | Scaffolds a new gRPC service definition for a CRD type | When adding a new API resource |
| `tools/gazelle` | Updates Bazel BUILD files for Go packages and proto targets | After adding/removing Go files or proto definitions |
| `tools/goimports` | Bazel wrapper that runs goimports for Go import formatting | When reformatting Go imports |
| `tools/mamockgen` | Generates mocks for specified Go interfaces (invoked via `go generate`) | When adding or updating interface mocks |
| `tools/test/generate-certs.sh` | Generates test certificates | For local TLS testing |

## gen-proto-go.sh

```bash
tools/gen-proto-go.sh
```

Builds `//proto/...` with Bazel, copies the generated `.pb.go` files into `proto-go/`, generates alias `BUILD.bazel` files under `proto-go/`, syncs dependency versions from `go/go.mod` into `proto-go/go.mod`, and runs `go mod tidy` in `proto-go/`.

Check in both the proto change and the generated output together.

See [Protocol Buffers](protobuf.md) for the full code generation workflow.

## grpc-svc-gen.sh

```bash
tools/grpc-svc-gen.sh [EntityName]
```

Example:

```bash
tools/grpc-svc-gen.sh Pipeline
```

Run without arguments to see the full usage message.

## gazelle

```bash
tools/gazelle
```

See [Bazel Build System](bazel.md) for context on when to run Gazelle.

## gen-grpc-client.sh

```bash
tools/gen-grpc-client.sh
```

Generates gRPC client stubs for Python and JavaScript from the compiled proto definitions, and (via `tools/gen-descriptors.sh`) regenerates `helm/michelangelo/files/descriptors.pb` and `helm/michelangelo/files/transcoder-services.json` — the Envoy `grpc_json_transcoder` filter's proto descriptor set and services allowlist.

**You rarely need to run this directly.** `javascript/package.json` wires it in as `yarn generate`, which runs automatically as a `prebuild` and `setup` script — so `yarn build`, `yarn dev` (via prebuild on first install), and `yarn setup` all regenerate these files as a side effect whenever you have a JS dev loop running, without you needing to know they exist. If you only touched Go code and never run a JS command, run `tools/gen-grpc-client.sh` (or just `tools/gen-descriptors.sh`, which skips the Python/JS stub codegen) manually and commit the result alongside the proto change.

Both `descriptors.pb` and `transcoder-services.json` are committed generated artifacts, not build-time output — `helm install` never invokes buf, so the chart works from a plain checkout without the buf toolchain. Two independent backstops guard against them going stale or missing:

- The Envoy ConfigMap template fails fast (`fail` in the template) at `helm template`/`lint`/`install` time if `transcoder-services.json` is missing, empty, or malformed.
- The "Transcoder services check" CI workflow runs `tools/check-transcoder-services.sh` on every `proto/**` or `helm/michelangelo/files/**` change, for proto edits that never touch `javascript/` (so `yarn generate` never runs) or a hand-edited generated file.

## gen-descriptors.sh

```bash
tools/gen-descriptors.sh [output-dir]
```

Builds `descriptors.pb` + `transcoder-services.json` from `proto/api` into `output-dir` (default `helm/michelangelo/files`). This is the shared implementation `gen-grpc-client.sh` and `check-transcoder-services.sh` both call — you normally don't invoke it directly, but it's useful standalone if you only need the Helm chart files regenerated without the Python/JS client stub codegen.

## check-transcoder-services.sh

```bash
tools/check-transcoder-services.sh
```

Runs `gen-descriptors.sh` into a scratch directory and diffs the resulting `transcoder-services.json` against the committed one — the real generator, not a proto-parsing approximation. (It deliberately skips comparing `descriptors.pb` byte-for-byte: rebuilding it re-resolves an unpinned `buf.build/coscene-io/kubernetes-apis` BSR dependency that can drift between runs even with no proto/api change, which would make a raw binary diff flaky.) This is the CI backstop described above; run it locally to reproduce a CI failure before re-running `gen-grpc-client.sh`.

## goimports

```bash
tools/goimports [flags] [files]
```

A Bazel wrapper that runs `goimports` (`@org_golang_x_tools//cmd/goimports`) on Go files. Use it to format Go imports consistently without requiring a separate goimports installation.

## mamockgen

```bash
go generate ./...
```

`mamockgen` is invoked via `go generate` directives. It reads the `GOPACKAGE` and `GOFILE` environment variables set by `go generate` and produces mock implementations for each interface listed as an argument. Generated mocks are written to a `<package>mocks/` directory alongside the source file.

## Conventions

- Scripts use bash.
- Each script includes a usage message — run any script without arguments to see it.
- Scripts are self-contained: there is no shared shell function library. Each script carries everything it needs.
- Do not add shared utilities across scripts; keep them independent.

## Related

- [How to Write APIs](../how-to-write-apis.md)
- [Bazel Build System](bazel.md)
- [Protocol Buffers](protobuf.md)
