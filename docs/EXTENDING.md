# Extending Michelangelo Protos

Michelangelo provides a built-in extensibility framework that allows organizations to add their own fields to CRDs without modifying the OSS proto definitions.

## Overview

The extension system works by:
1. Organizations define **extension protos** with additional fields they need
2. The **patch compiler** merges extension fields into base OSS protos
3. Generated **patched protos** include both base and extension fields
4. All existing **code generation** (validation, CRD, RPC) works on patched protos
5. Services import and use the patched protos with all fields available

## Quick Start

### Step 1: Create Extension Proto

Create a proto file defining your organization's additional fields:

```protobuf
// your-org/extensions/project_ext.proto
syntax = "proto3";

package yourorg.michelangelo.extensions;

import "michelangelo/api/options.proto";

// Extension fields for ProjectSpec
message ProjectSpecExtension {
  string owner_id = 1 [(michelangelo.api.validation) = {
    uuid: true,
    required: true
  }];

  string cost_center = 2 [(michelangelo.api.validation) = {
    pattern: "[A-Z]+-[0-9]+",
    required: true
  }];

  repeated string compliance_tags = 3 [(michelangelo.api.validation) = {
    min_items: "1"
  }];
}

// Extension fields for ProjectStatus
message ProjectStatusExtension {
  string internal_state = 1;
  int64 last_audit_timestamp = 2;
}
```

### Step 2: Use Patching Rule in BUILD File

In your repository, add a BUILD file that uses the `patched_proto_library` macro:

```python
# your-org/BUILD.bazel
load("@michelangelo//bazel/rules/proto:patched_proto.bzl", "patched_proto_library")

patched_proto_library(
    name = "michelangelo_extended",
    # Base protos from OSS Michelangelo
    base_protos = "@michelangelo//proto/api/v2:v2_proto",

    # Your extension protos
    extension_protos = glob(["extensions/*.proto"]),

    # Prefix for your extension fields (will be YOUR_ORG_owner_id)
    field_prefix = "YOUR_ORG_",

    # Starting tag number for extension fields
    tag_start = 999,

    # Go import path for generated code
    importpath = "your-org.com/michelangelo/proto/api/v2",

    visibility = ["//visibility:public"],
)
```

### Step 3: Import OSS Michelangelo

In your WORKSPACE or MODULE.bazel file:

```python
git_repository(
    name = "michelangelo",
    remote = "https://github.com/michelangelo-ai/michelangelo.git",
    commit = "COMMIT_SHA",  # Pin to specific version
)
```

### Step 4: Use in Your Code

```go
package main

import (
    v2 "your-org.com/michelangelo/proto/api/v2"
)

func createProject(req *CreateProjectRequest) (*v2.Project, error) {
    project := &v2.Project{
        Metadata: &metav1.ObjectMeta{
            Name:      req.Name,
            Namespace: req.Namespace,
        },
        Spec: &v2.ProjectSpec{
            // Base OSS fields
            Description: req.Description,
            Tier:        req.Tier,

            // Your extension fields (with prefix)
            YOUR_ORG_owner_id:        req.OwnerID,
            YOUR_ORG_cost_center:     req.CostCenter,
            YOUR_ORG_compliance_tags: req.ComplianceTags,
        },
    }

    // Validation includes your extension fields
    if err := project.Validate(""); err != nil {
        return nil, fmt.Errorf("validation failed: %w", err)
    }

    return project, nil
}
```

### Step 5: Build

```bash
bazel build //your-org:michelangelo_extended
```

Bazel automatically:
- Fetches OSS Michelangelo
- Runs the patch compiler
- Generates patched protos
- Runs all code generators
- Produces a usable Go library

## How It Works

### Extension Field Naming

Extension fields are added with a configurable prefix (e.g., `YOUR_ORG_`) to:
- Clearly identify which fields are extensions vs base fields
- Avoid naming conflicts with future OSS fields
- Support multiple organizations extending independently

Example:
```protobuf
// Your extension proto
message ProjectSpecExtension {
  string owner_id = 1;
}

// Generated patched proto
message ProjectSpec {
  // ... base OSS fields ...

  string YOUR_ORG_owner_id = 999;  // Extension field with prefix
}
```

### Tag Number Management

Extension fields use tag numbers starting from 999 (configurable) to avoid conflicts with base fields. This range is documented as reserved for extensions.

### Validation Integration

Validation annotations in your extension protos are preserved and code is generated automatically:

```protobuf
string owner_id = 1 [(michelangelo.api.validation) = {uuid: true, required: true}];
```

Generates:
```go
if spec.YOUR_ORG_owner_id == "" {
    return errors.New("YOUR_ORG_owner_id is required")
}
if _, err := uuid.Parse(spec.YOUR_ORG_owner_id); err != nil {
    return errors.New("YOUR_ORG_owner_id must be valid UUID")
}
```

### CRD Schema Generation

Extension fields are included in Kubernetes CRD schemas automatically:

```yaml
openAPIV3Schema:
  properties:
    spec:
      properties:
        YOUR_ORG_owner_id:
          type: string
          format: uuid
        YOUR_ORG_cost_center:
          type: string
          pattern: ^[A-Z]+-[0-9]+$
```

## Advanced Usage

### Custom Configuration

For fine-grained control, provide a custom patch configuration:

```yaml
# config/patches.yaml
patches:
  - target_proto: "michelangelo/api/v2/project.proto"
    target_message: "ProjectSpec"
    extension_proto: "extensions/project_ext.proto"
    extension_message: "ProjectSpecExtension"

validation_overrides:
  - target_proto: "michelangelo/api/v2/project.proto"
    target_message: "ProjectSpec"
    field: "tier"
    new_validation:
      min: "1"
      max: "10"  # Override OSS max of 4
```

Use in BUILD file:
```python
patched_proto_library(
    name = "michelangelo_extended",
    base_protos = "@michelangelo//proto/api/v2:v2_proto",
    extension_protos = glob(["extensions/*.proto"]),
    extension_config = "config/patches.yaml",
)
```

### Validation Override

You can override validation rules on existing base fields:

```yaml
validation_overrides:
  - target_proto: "michelangelo/api/v2/project.proto"
    target_message: "ProjectSpec"
    field: "tier"
    new_validation:
      min: "1"
      max: "10"  # Increase from OSS max of 4
```

This allows organizations to relax or tighten constraints based on their needs.

## Best Practices

### Naming Conventions

- Use descriptive extension message names like `{OrgName}ProjectSpecExtension`
- Use consistent field prefixes like `YOUR_ORG_` across all extensions
- Document what each extension field means

### Version Management

- Pin to specific OSS Michelangelo commit SHAs
- Test extension compatibility before updating OSS version
- Use semantic versioning for your patched proto library

### Field Design

- Add validation annotations to extension fields
- Use appropriate proto types (int32, string, bool, etc.)
- Avoid deeply nested extension structures
- Document field semantics in proto comments

### Testing

- Write unit tests for services using extension fields
- Validate that patched protos compile correctly
- Test CRD creation with extension fields in Kubernetes
- Verify validation logic for extension fields

## Updating OSS Version

When updating to a new Michelangelo version:

1. Update commit SHA in WORKSPACE/MODULE.bazel
2. Run build to regenerate patched protos
3. Check for conflicts (compiler will error if found)
4. Resolve any conflicts by updating extensions or configuration
5. Test thoroughly before deploying

## Troubleshooting

### Build Fails with "tag number collision"

OSS added a field with a tag number that conflicts with your extensions. Update your `tag_start` to a higher number or use custom configuration to reassign tag numbers.

### Field Not Found in Generated Code

Check that:
- Extension proto is listed in `extension_protos`
- Message names follow conventions (e.g., `ProjectSpecExtension` extends `ProjectSpec`)
- BUILD target built successfully

### Validation Not Working

Ensure:
- Validation annotations use correct syntax
- Validation plugin is in the `compilers` list
- Generated `.pb.validation.go` file exists

## Examples

See the `examples/extensions/` directory for complete working examples.

## Support

For questions or issues with the extension framework:
- Open an issue on GitHub
- Check existing documentation
- Review example implementations


