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
| `tools/gen-descriptors.sh` | Builds `helm/michelangelo/files/descriptors.pb` (via `bazel build //proto/api/v2:v2_proto`) + `transcoder-services.json` (its intersection with `services.ts`) | After any `.proto` file change — same trigger point as `gen-proto-go.sh`, CI-enforced |
| `tools/check-transcoder-services.sh` | Narrow backstop: verifies `transcoder-services.json` matches `services.ts` for edits that don't touch `proto/**` | CI backstop on `javascript/packages/rpc/services.ts` or `helm/michelangelo/files/**` changes; run locally to reproduce a CI failure |
| `tools/gen-grpc-client.sh` | Generates gRPC **client language bindings** (Python and JavaScript classes) from protobuf files | After proto changes that affect client stubs — but you rarely run this directly, see below |
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

Check in both the proto change and the generated output together. Also run `tools/gen-descriptors.sh` (below) — it's the same trigger point, just a different Bazel output group of the same compile.

See [Protocol Buffers](protobuf.md) for the full code generation workflow.

## gen-descriptors.sh

```bash
tools/gen-descriptors.sh [output-dir]
```

Builds `helm/michelangelo/files/descriptors.pb` — the compiled proto `FileDescriptorSet` Envoy's `grpc_json_transcoder` filter reads — via `bazel build //proto/api/v2:v2_proto`, which materializes Bazel's native `proto_library` implicit descriptor-set output (`bazel-bin/proto/api/v2/v2_proto-descriptor-set.proto.bin`). This is the **same underlying compilation** `gen-proto-go.sh` already triggers with `bazel build //proto/...` — no separate `buf` compile, no BSR dependency resolution, no network. Confirmed deterministic: running it twice back-to-back produces a byte-identical `descriptors.pb`.

It then also writes `transcoder-services.json` — the subset of that descriptor set's services that `javascript/packages/rpc/services.ts` actually imports, resolved to fully-qualified names (`buf build <path>#format=binpb` is used here purely as a local binary→JSON decoder of the file just built, not to compile anything, so this step is also network-free). This is **not** every service that exists in `proto/api` — Envoy's `grpc_json_transcoder` exposes whatever is on this list over plain JSON/HTTP, so dumping the full descriptor set here would make any new Go-only service web-reachable the moment its proto compiles, whether or not the browser client can call it. Fails loudly if a `services.ts` import doesn't resolve to any proto service (a likely typo/rename), or if no services are referenced at all (refuses to write an empty allowlist).

**Run this on every proto change**, same as `gen-proto-go.sh` — even a change that looks Go-only or JS-only. `descriptors.pb` is one deduplicated descriptor set for the whole tree, so a change to a shared/imported type can affect how an already-JS-consumed service's types resolve, even with no proto file `services.ts` cares about touched directly. CI enforces this unconditionally via the "Check transcoder descriptor artifacts are up to date" step in `main.yml`'s `dirty-check` job, which runs on every `proto/**`/`go/**` change regardless of whether `javascript/` also changed.

Both `descriptors.pb` and `transcoder-services.json` are committed generated artifacts, not build-time output — `helm install` never invokes Bazel or buf, so the chart works from a plain checkout without either toolchain. As a last-resort safety net independent of both CI checks, the Envoy ConfigMap template itself fails fast (`fail` in the template) at `helm template`/`lint`/`install` time if `transcoder-services.json` is missing, empty, or malformed.

## check-transcoder-services.sh

```bash
tools/check-transcoder-services.sh
```

A **narrow** backstop: runs `gen-descriptors.sh` into a scratch directory and diffs both outputs against the committed ones. It exists specifically because `main.yml`'s `dirty-check` job (the primary enforcement described above) doesn't run on `javascript/**`-only changes — so a `services.ts` edit with no accompanying proto change would otherwise slip through uncaught. Run it locally to reproduce a CI failure.

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

Generates gRPC **client language bindings** — the actual generated TS/Python classes under `javascript/packages/rpc/gen/` and `python/michelangelo/gen/` — for Python and JavaScript from the compiled proto definitions, via buf's remote codegen plugins (needs network access to buf.build). This is a separate concern from `gen-descriptors.sh` above and deliberately does **not** touch `descriptors.pb` or `transcoder-services.json` — those regenerate on the proto-build trigger described above, not on this one, so that a plain `yarn build`/`yarn setup` doesn't pick up a new hard dependency on Bazel.

**You rarely need to run this directly.** `javascript/package.json` wires it in as `yarn generate`, which runs automatically as a `prebuild` and `setup` script — so `yarn build`, `yarn dev` (via prebuild on first install), and `yarn setup` all regenerate the client class files as a side effect whenever you have a JS dev loop running.

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
