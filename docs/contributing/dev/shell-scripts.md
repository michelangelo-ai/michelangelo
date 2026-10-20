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
| `tools/gen-transcoder-services.sh` | Regenerates `helm/michelangelo/files/transcoder-services.json` from `services.ts`'s import paths | After changing `javascript/packages/rpc/services.ts` |
| `tools/check-transcoder-services.sh` | Checks that `transcoder-services.json` matches `services.ts` | For a `services.ts`-only change; run locally to reproduce a CI failure |
| `tools/gen-grpc-client.sh` | Generates gRPC client code (Python and JavaScript) from protobuf files | After proto changes that affect client stubs |
| `tools/grpc-svc-gen.sh [Entity]` | Scaffolds a new gRPC service definition for a CRD type | When adding a new API resource |
| `tools/gazelle` | Updates Bazel BUILD files for Go packages and proto targets | After adding/removing Go files or proto definitions |
| `tools/goimports` | Bazel wrapper that runs goimports for Go import formatting | When reformatting Go imports |
| `tools/mamockgen` | Generates mocks for specified Go interfaces (invoked via `go generate`) | When adding or updating interface mocks |
| `tools/test/generate-certs.sh` | Generates test certificates | For local TLS testing |

## gen-proto-go.sh

```bash
tools/gen-proto-go.sh
```

Builds `//proto/...` with Bazel, copies the generated `.pb.go` files into `proto-go/`, generates alias `BUILD.bazel` files under `proto-go/`, syncs dependency versions from `go/go.mod` into `proto-go/go.mod`, sets `proto-go/go.mod`'s `go` toolchain directive from `MODULE.bazel`'s `go_sdk.download(version = ...)` pin (failing loudly if that pin can't be found), and runs `go mod tidy` in `proto-go/`.

Check in both the proto change and the generated output together.

See [Protocol Buffers](protobuf.md) for the full code generation workflow.

## gen-transcoder-services.sh

```bash
tools/gen-transcoder-services.sh [output-dir]
```

Reads `javascript/packages/rpc/services.ts` to find every service the JS client imports and derives each one's fully-qualified proto service name directly from its import path — `./gen/michelangelo/api/v2/deployment_svc_pb` importing `DeploymentService` becomes `michelangelo.api.v2.DeploymentService`, since `gen-grpc-client.sh`'s generated directory layout mirrors the proto package by construction. Writes the result to `helm/michelangelo/files/transcoder-services.json`.

This list is deliberately narrower than every service under `proto/api` — Envoy's `grpc_json_transcoder` filter exposes whatever is on it over plain JSON/HTTP, so a Go-only service shouldn't become web-reachable just because its proto compiles. The script fails if `services.ts` references no services at all. It does not validate each derived name against a compiled descriptor set — Envoy does that at startup, refusing to come up if an entry doesn't match, so a wrong FQN fails loudly rather than routing silently.

`transcoder-services.json` is committed, not build output — `helm install` never runs this script. As a last-resort check, the Envoy ConfigMap template fails at render time if the file is missing, empty, or malformed.

## check-transcoder-services.sh

```bash
tools/check-transcoder-services.sh
```

A narrow backstop: it runs `gen-transcoder-services.sh` into a scratch directory and diffs the result against the committed `transcoder-services.json`. Run it locally to reproduce a CI failure from the "Transcoder services check" workflow.

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
