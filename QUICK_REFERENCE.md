# Proto Extension Framework - Quick Reference

## For Users (Organizations Extending Michelangelo)

### 1. Import Michelangelo in Your Repo

```python
# WORKSPACE or MODULE.bazel
git_repository(
    name = "michelangelo",
    remote = "https://github.com/michelangelo-ai/michelangelo.git",
    commit = "COMMIT_SHA",  # Get from third_party/github.com/michelangelo-ai/repo.bzl
)
```

### 2. Create Extension Proto

```protobuf
// your-org/extensions/project_ext.proto
syntax = "proto3";
package yourorg.michelangelo.extensions;

import "michelangelo/api/options.proto";

message ProjectSpecExtension {
  string owner_id = 1 [(michelangelo.api.validation) = {
    uuid: true,
    required: true
  }];
  string cost_center = 2;
}
```

### 3. Create BUILD File

```python
# your-org/BUILD.bazel
load("@michelangelo//bazel/rules/proto:patched_proto.bzl", "patched_proto_library")

patched_proto_library(
    name = "michelangelo_extended",
    base_protos = "@michelangelo//proto/api/v2:v2_proto",
    extension_protos = glob(["extensions/*.proto"]),
    field_prefix = "YOUR_ORG_",
    tag_start = 999,
    importpath = "your-org.com/michelangelo/proto/api/v2",
    visibility = ["//visibility:public"],
)
```

### 4. Use in Services

```python
# your-service/BUILD.bazel
go_binary(
    name = "my-service",
    deps = [
        "//your-org:michelangelo_extended",  # Use extended protos
    ],
)
```

```go
// your-service/main.go
import v2 "your-org.com/michelangelo/proto/api/v2"

func main() {
    project := &v2.Project{
        Spec: &v2.ProjectSpec{
            Description: "My project",
            YOUR_ORG_owner_id: "uuid-here",
            YOUR_ORG_cost_center: "ML-123",
        },
    }

    if err := project.Validate(""); err != nil {
        log.Fatal(err)
    }
}
```

### 5. Build

```bash
bazel build //your-org:michelangelo_extended
bazel build //your-service:my-service
```

## For Developers (Implementing the Framework)

### File Structure

```
michelangelo/
├── tools/
│   └── proto-patcher/
│       ├── main.go                    # Entry point
│       ├── BUILD.bazel                # Build config
│       ├── config-generator/          # Auto-gen configs
│       │   ├── main.go
│       │   └── BUILD.bazel
│       ├── parser/                    # TODO: Proto parser
│       ├── patcher/                   # TODO: Patching logic
│       └── generator/                 # TODO: Proto generation
│
├── bazel/
│   └── rules/
│       └── proto/
│           └── patched_proto.bzl      # Main Bazel rule
│
├── docs/
│   └── EXTENDING.md                   # User guide
│
├── examples/
│   └── extensions/
│       ├── project_ext.proto          # Example extension
│       └── README.md                  # Example docs
│
├── PROTO_EXTENSIONS.md                # Framework overview
├── IMPLEMENTATION_STATUS.md           # What's done/todo
└── QUICK_REFERENCE.md                 # This file
```

### Build Commands

```bash
# Build proto-patcher tool
bazel build //tools/proto-patcher

# Build config generator
bazel build //tools/proto-patcher/config-generator

# Run proto-patcher manually
./bazel-bin/tools/proto-patcher/proto-patcher \
  --base_protos="proto1.proto proto2.proto" \
  --ext_protos="ext1.proto ext2.proto" \
  --field_prefix="ORG_" \
  --tag_start=999 \
  --output_dir=/tmp/patched

# Run config generator
./bazel-bin/tools/proto-patcher/config-generator/config-generator \
  --ext_protos="ext1.proto ext2.proto" \
  --field_prefix="ORG_" \
  --tag_start=999 \
  --output=config.yaml
```

### Implementation Checklist

Core components to implement:

- [x] Basic tool structure
- [x] Config generator (DONE)
- [x] Bazel rule (DONE)
- [x] Documentation (DONE)
- [x] Examples (DONE)
- [ ] Proto parser
- [ ] Patcher logic
- [ ] Proto generator
- [ ] Unit tests
- [ ] Integration tests

### Next Steps

1. **Implement Parser** (`tools/proto-patcher/parser/`)
   - Use `github.com/jhump/protoreflect/desc/protoparse`
   - Parse proto3 files into AST
   - Extract messages, fields, options

2. **Implement Patcher** (`tools/proto-patcher/patcher/`)
   - Merge extension fields into base messages
   - Assign tag numbers (start from config)
   - Add field prefixes
   - Handle validation overrides

3. **Implement Generator** (`tools/proto-patcher/generator/`)
   - Generate proto3 files from AST
   - Preserve comments
   - Format properly

4. **Add Tests**
   - Unit tests for each component
   - Integration tests with real protos
   - Test fixtures

## Validation Annotations Reference

Common validation rules for extension fields:

```protobuf
// Required field
string field = 1 [(michelangelo.api.validation) = {required: true}];

// UUID format
string id = 2 [(michelangelo.api.validation) = {uuid: true}];

// Pattern matching
string code = 3 [(michelangelo.api.validation) = {pattern: "[A-Z]+-[0-9]+"}];

// Range validation
int32 count = 4 [(michelangelo.api.validation) = {min: "1", max: "100"}];

// List validation
repeated string tags = 5 [(michelangelo.api.validation) = {
  min_items: "1",
  items: {in: ["option1", "option2", "option3"]}
}];

// Optional field
string optional = 6 [(michelangelo.api.validation) = {optional: true}];

// Custom error message
string field = 7 [(michelangelo.api.validation) = {
  required: true,
  msg: "custom error message"
}];

// Email validation
string email = 8 [(michelangelo.api.validation) = {email: true}];

// URI validation
string url = 9 [(michelangelo.api.validation) = {uri: true}];

// Length constraints
string name = 10 [(michelangelo.api.validation) = {
  min_length: "3",
  max_length: "50"
}];
```

## Naming Conventions

### Extension Proto Files
- Name: `{crd_name}_ext.proto`
- Example: `project_ext.proto`, `deployment_ext.proto`

### Extension Messages
- Name: `{MessageName}Extension`
- Example: `ProjectSpecExtension`, `ProjectStatusExtension`

### Field Prefixes
- Format: `{ORG}_` or `{ORG}_INTERNAL_`
- Example: `UBER_owner_id`, `ACME_INTERNAL_cost_center`

### Tag Numbers
- Start: 999 or higher
- Reserved: 999-1999 recommended for extensions
- Avoid: 1-998 (base proto range)

## Troubleshooting

### Build fails with "proto-patcher not found"
```bash
# Verify Michelangelo is imported
bazel query @michelangelo//...

# Build proto-patcher explicitly
bazel build @michelangelo//tools/proto-patcher
```

### "Tag number collision" error
```python
# Increase tag_start in BUILD file
patched_proto_library(
    tag_start = 1500,  # Use higher range
)
```

### Generated code doesn't include extension fields
```bash
# Check that extension protos are listed
ls your-org/extensions/

# Verify BUILD file includes them
# extension_protos = glob(["extensions/*.proto"])

# Check build outputs
bazel build //your-org:michelangelo_extended
ls bazel-bin/your-org/
```

### Validation not working
```protobuf
// Ensure import is present
import "michelangelo/api/options.proto";

// Use correct syntax
[(michelangelo.api.validation) = {required: true}]
```

## Documentation Links

- **Overview**: [PROTO_EXTENSIONS.md](PROTO_EXTENSIONS.md)
- **User Guide**: [docs/EXTENDING.md](docs/EXTENDING.md)
- **Examples**: [examples/extensions/](examples/extensions/)
- **Implementation Status**: [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md)
- **Main README**: [README.md](README.md)

## Support

- GitHub Issues: Report problems or request features
- Documentation: Check docs/ directory
- Examples: See examples/extensions/
- Wiki: Michelangelo-AI Wiki (when available)


