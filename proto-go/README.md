# proto-go

This directory contains **generated** Go protobuf sources for use by
Go module consumers that do **not** build with Bazel. It is a standalone
Go module (`github.com/michelangelo-ai/michelangelo/proto-go`) that mirrors
the protobuf packages under `proto/`.

## Regenerating files

From the repo root:

```
tools/gen-proto-go.sh
```

This script:

1. Builds `//proto/...` with Bazel (`--output_groups=go_generated_srcs`).
2. Deletes the existing `.go` files under `proto-go/` and copies the generated
   `.go` files in.
3. Regenerates an alias `BUILD.bazel` file in each package directory
   (pointing at the matching `//proto/...:go_default_library` target).
4. Sets the `go` directive in `proto-go/go.mod` to the Go SDK version pinned
   by `go_sdk.download(version = ...)` in `MODULE.bazel`.
5. Runs `go mod tidy` in `proto-go/`.

The script also contains a step meant to sync dependency versions from
`go/go.mod` into `proto-go/go.mod`, but that step currently matches nothing
(tracked in #2066), so dependency versions are not actually synced yet.
Keeping versions aligned avoids mismatched dependency graphs between the
/go Go module and the generated /proto-go module, which can otherwise
cause build failures or subtle type conflicts. Until #2066 is addressed,
check that dependency versions in `proto-go/go.mod` match `go/go.mod`
after running the script.

## Note for Bazel users

Bazel builds in `go/` do **not** depend on `proto-go/`. Bazel generates
protobuf Go sources into its own output tree (`bazel-bin/...`) and compiles
against those files directly. The `proto-go/` directory is only for Go
module users.
