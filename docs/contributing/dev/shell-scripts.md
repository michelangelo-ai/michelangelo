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
| `tools/gen-descriptors.sh` | Builds `helm/michelangelo/files/descriptors.pb` and `transcoder-services.json` | After any `.proto` file change — same trigger point as `gen-proto-go.sh`, CI-enforced |
| `tools/check-transcoder-services.sh` | Checks that `transcoder-services.json` matches `services.ts` | For a `services.ts`-only change; run locally to reproduce a CI failure |
| `tools/gen-grpc-client.sh` | Generates gRPC **client language bindings** (Python and JavaScript classes) from protobuf files | Runs automatically during the JS build — you rarely need to run it by hand, see below |
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

Check in both the proto change and the generated output together. Also run `tools/gen-descriptors.sh` (below) — it uses the same Bazel build, just a different output.

See [Protocol Buffers](protobuf.md) for the full code generation workflow.

## gen-descriptors.sh

```bash
tools/gen-descriptors.sh [output-dir]
```

This script builds `helm/michelangelo/files/descriptors.pb`. Envoy's `grpc_json_transcoder` filter reads this file. It is a compiled proto `FileDescriptorSet`.

The script runs `bazel build //proto/api/v2:v2_proto`. This is the same compile step `gen-proto-go.sh` uses. It needs no network access, and repeat runs produce a byte-identical file.

The script also writes `transcoder-services.json`. This file lists the services that `javascript/packages/rpc/services.ts` actually imports, matched against the descriptor set. It is not a list of every service under `proto/api`. Envoy's transcoder exposes any service on this list over plain JSON/HTTP, so the list must stay narrow: a Go-only service should not become web-reachable just because its proto compiles.

The script fails if a `services.ts` import does not resolve to a real proto service. It also fails if `services.ts` references no services at all.

**Run this script on every proto change**, even one that looks Go-only or JS-only. `descriptors.pb` covers the whole proto tree in one file. A change to a shared type can affect how an already-used service resolves, even if its own proto file wasn't touched. CI enforces this check in `main.yml`'s `dirty-check` job, on every `proto/**` or `go/**` change.

Both `descriptors.pb` and `transcoder-services.json` are committed files, not build output. `helm install` never runs Bazel or buf. As a last-resort check, the Envoy ConfigMap template fails at `helm template`/`lint`/`install` time if `transcoder-services.json` is missing, empty, or malformed.

## check-transcoder-services.sh

```bash
tools/check-transcoder-services.sh
```

This script is a narrow backstop. It runs `gen-descriptors.sh` into a scratch directory, then compares the result against the committed files. It exists because `main.yml`'s `dirty-check` job does not run on `javascript/**`-only changes. Without this check, a `services.ts` edit with no proto change could slip through uncaught. Run it locally to reproduce a CI failure.

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

This script generates gRPC **client language bindings** — the generated TS/Python classes under `javascript/packages/rpc/gen/` and `python/michelangelo/gen/`. It uses buf's remote codegen plugins, so it needs network access to buf.build.

This is separate from `gen-descriptors.sh` above. It does not touch `descriptors.pb` or `transcoder-services.json`. Those files regenerate on the proto-build trigger described above, not on this one. A plain `yarn build` or `yarn setup` stays free of a new Bazel dependency this way.

**You rarely need to run this directly.** `javascript/package.json` wires it in as `yarn generate`. This runs automatically as a `prebuild` and `setup` step, so `yarn build`, `yarn dev`, and `yarn setup` all regenerate the client classes for you.

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
