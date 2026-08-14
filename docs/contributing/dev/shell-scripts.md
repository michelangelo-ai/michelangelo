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
| `tools/gen-grpc-client.sh` | Generates gRPC client code (Python and JavaScript) from protobuf files | After proto changes that affect client stubs |
| `tools/check-transcoder-services.sh` | Verifies `helm/michelangelo/files/transcoder-services.json` matches the services declared under `proto/api` | Run in CI on any `proto/**` or `helm/michelangelo/files/**` change; run locally to debug a CI failure |
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

Generates gRPC client stubs for Python and JavaScript from the compiled proto definitions. Run this after proto changes when client-side stubs need to be regenerated.

It also regenerates `helm/michelangelo/files/descriptors.pb` and `helm/michelangelo/files/transcoder-services.json` (via `tools/gen-descriptors.sh`), which the Envoy `grpc_json_transcoder` filter's ConfigMap templates read at `helm template`/`helm install` time. Commit both alongside the client stub changes so the chart's transcoder allowlist stays in sync with the services the client is built against.

Both files are committed generated artifacts, not build-time output — `helm install` never invokes buf, so the chart works from a plain checkout without the buf toolchain. The Envoy ConfigMap template fails fast (`fail` in the template) if `transcoder-services.json` is missing, empty, or malformed, and the "Transcoder services check" CI workflow re-derives the expected service list directly from `proto/api/**/*.proto` and fails the PR if it doesn't match the committed file — so a proto change without a `gen-grpc-client.sh` re-run can't merge silently.

## check-transcoder-services.sh

```bash
tools/check-transcoder-services.sh
```

Parses `package`/`service` declarations out of `proto/api/**/*.proto` (no buf invocation, no network) and diffs the result against `helm/michelangelo/files/transcoder-services.json`. This is what CI runs on every `proto/**` or `helm/michelangelo/files/**` change; run it locally to reproduce a CI failure before re-running `gen-grpc-client.sh`.

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
