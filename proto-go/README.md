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

This script builds `//proto/...` with Bazel and copies the generated `.go`
files into `proto-go/`.
It also syncs dependency versions from `go/go.mod` into `proto-go/go.mod`
and runs `go mod tidy`.
Keeping versions aligned avoids mismatched dependency graphs between the
/go Go module and the generated /proto-go module, which can otherwise
cause build failures or subtle type conflicts. Developers should update
dependency versions only in `go/go.mod`, then run this script to sync
`proto-go/go.mod` and keep the two modules consistent.

## Note for Bazel users

Bazel builds in `go/` do **not** depend on `proto-go/`. Bazel generates
protobuf Go sources into its own output tree (`bazel-bin/...`) and compiles
against those files directly. The `proto-go/` directory is only for Go
module users.
