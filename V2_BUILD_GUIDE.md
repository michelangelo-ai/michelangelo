# Building v2 Protos with All Compilers

## Current v2 Compiler Configuration

The `//proto/api/v2:v2_go_proto` target currently uses these compilers:

```python
go_proto_library(
    name = "v2_go_proto",
    compilers = [
        "//bazel/rules/proto:go_kubeproto",      # Kubernetes support
        "//bazel/rules/proto:go_yarpc",          # YARPC RPC
        "//bazel/rules/proto:go_kubeyarpc",      # Kubernetes YARPC
        "//bazel/rules/proto:go_validation",     # Validation
    ],
    ...
)
```

## Build Commands

### 1. Build v2 with Current Compilers
```bash
bazel build //proto/api/v2:v2_go_proto
```

### 2. Build v2_ext with ext Compiler
```bash
bazel build //proto/api/v2_ext:v2_ext_go_proto
```

### 3. Build SQL Schema for v2
```bash
bazel build //proto/api/v2:v2_kube_proto_sql
```

### 4. Build YAML Manifests for v2
```bash
bazel build //proto/api/v2:v2_kube_proto_yaml
```

## Generated File Locations

All generated files are placed in the bazel output directory. The exact path is:

```
bazel-out/k8-fastbuild/bin/proto/api/v2/v2_go_proto_/github.com/michelangelo-ai/michelangelo/proto/api/v2/
```

Or you can find it using:
```bash
# Get bazel output directory
bazel info bazel-bin

# The files will be in:
$(bazel info bazel-bin)/proto/api/v2/v2_go_proto_/github.com/michelangelo-ai/michelangelo/proto/api/v2/
```

## Generated Files for v2

For each proto file in v2 (e.g., `model.proto`), these files are generated:

| Compiler | Generated File | Location |
|----------|---------------|----------|
| go_kubeproto | `model.pb.go` | `bazel-out/.../proto/api/v2/v2_go_proto_/.../model.pb.go` |
| go_yarpc | `model.pb.yarpc.go` | `bazel-out/.../proto/api/v2/v2_go_proto_/.../model.pb.yarpc.go` |
| go_kubeyarpc | `model.pb.kubeyarpc.go` | `bazel-out/.../proto/api/v2/v2_go_proto_/.../model.pb.kubeyarpc.go` |
| go_validation | `model.pb.validation.go` | `bazel-out/.../proto/api/v2/v2_go_proto_/.../model.pb.validation.go` |
| kube_proto_sql | `v2.sql` | `bazel-out/.../proto/api/v2/v2_kube_proto_sql.sql` |
| kube_proto_yaml | `v2.yaml` | `bazel-out/.../proto/api/v2/v2_kube_proto_yaml.yaml` |

## Adding ext Compiler to v2

If you want to add the ext compiler to v2, you would modify the BUILD.bazel:

```python
go_proto_library(
    name = "v2_go_proto_with_ext",
    compilers = [
        "//bazel/rules/proto:go_kubeproto",
        "//bazel/rules/proto:go_yarpc",
        "//bazel/rules/proto:go_kubeyarpc",
        "//bazel/rules/proto:go_validation",
        "//bazel/rules/proto:go_ext",        # ADD THIS
    ],
    ...
)
```

This would generate additional `.ext.go` files with validation registry.

## Finding Generated Files

### Method 1: Direct Path
```bash
# After building
ls -la bazel-out/k8-fastbuild/bin/proto/api/v2/v2_go_proto_/github.com/michelangelo-ai/michelangelo/proto/api/v2/
```

### Method 2: Using find
```bash
# Find all generated v2 files
find bazel-out -path "*proto/api/v2*" -name "*.go" -o -name "*.sql" -o -name "*.yaml"
```

### Method 3: Check symlinks
```bash
# Bazel creates symlinks in the workspace root
ls -la bazel-bin/proto/api/v2/
```

## v2 Proto Files

The v2 directory contains these proto files that get compiled:
- `cached_output.proto`
- `cached_output_svc.proto`
- `git.proto`
- `model.proto`
- `pipeline.proto`
- `pipelinerun.proto`
- `project.proto`
- `project_svc.proto`
- `schema.proto`
- `transformer.proto`
- `transformer_svc.proto`

Each of these generates the corresponding output files based on the compilers used.

## Complete Build Example

```bash
# Clean build
bazel clean

# Build v2 with all its compilers
bazel build //proto/api/v2:v2_go_proto

# Build v2_ext with ext validation
bazel build //proto/api/v2_ext:v2_ext_go_proto

# Build SQL schemas
bazel build //proto/api/v2:v2_kube_proto_sql

# Build YAML manifests
bazel build //proto/api/v2:v2_kube_proto_yaml

# List all generated files
find bazel-out -path "*proto/api/v2*" \( -name "*.go" -o -name "*.sql" -o -name "*.yaml" \) | sort
```

## Output Directory Structure

```
bazel-out/k8-fastbuild/bin/proto/api/
├── v2/
│   ├── v2_go_proto_/
│   │   └── github.com/michelangelo-ai/michelangelo/proto/api/v2/
│   │       ├── cached_output.pb.go
│   │       ├── cached_output.pb.validation.go
│   │       ├── cached_output.pb.yarpc.go
│   │       ├── cached_output.pb.kubeyarpc.go
│   │       ├── model.pb.go
│   │       ├── model.pb.validation.go
│   │       ├── model.pb.yarpc.go
│   │       ├── model.pb.kubeyarpc.go
│   │       ├── schema.pb.go
│   │       ├── schema.pb.validation.go
│   │       └── ... (more files)
│   ├── v2_kube_proto_sql.sql
│   └── v2_kube_proto_yaml.yaml
└── v2_ext/
    └── v2_ext_go_proto_/
        └── github.com/michelangelo-ai/michelangelo/proto/api/v2_ext/
            ├── schema_ext.pb.go
            └── schema_ext.ext.go
```
