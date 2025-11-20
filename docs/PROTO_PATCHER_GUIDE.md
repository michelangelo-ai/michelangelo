# Proto Patcher - Complete Usage Guide

## Overview

The Proto Patcher tool extends base Protocol Buffer files with organization-specific fields while maintaining clean separation between OSS and internal code.

## Quick Start

### Build the Tool

```bash
cd /home/user/Uber/michelangelo
bazel build //tools/proto-patcher
```

### Basic Usage

```bash
bazel run //tools/proto-patcher -- \
  --base_proto_dir=<path-to-base-protos> \
  --ext_proto_dir=<path-to-extension-protos> \
  --output_dir=<output-directory> \
  --config=<config-file> \
  --verbose
```

## Command Line Options

| Option | Required | Default | Description |
|--------|----------|---------|-------------|
| `--base_proto_dir` | Yes | - | Directory containing base/OSS proto files |
| `--ext_proto_dir` | Yes | - | Directory containing extension proto files |
| `--output_dir` | Yes | - | Output directory for patched proto files |
| `--config` | Conditional | - | Path to JSON config file (required if not using --generate_config) |
| `--generate_config` | Conditional | false | Auto-generate config from extension files |
| `--field_prefix` | No | `EXT_` | Prefix for extension fields (used with --generate_config) |
| `--tag_start` | No | 999 | Starting tag number for extension fields |
| `--import_paths` | No | - | Comma-separated list of proto import paths |
| `--verbose` | No | false | Enable verbose logging |

## Usage Modes

### Mode 1: Manual Configuration (Recommended for Production)

Create a JSON configuration file specifying exactly which messages to patch:

**Example: patch_config.json**
```json
{
  "field_prefix": "UBER_INTERNAL_",
  "tag_start": 999,
  "patches": [
    {
      "target_proto": "project.proto",
      "target_message": "ProjectSpec",
      "extension_proto": "project_ext.proto",
      "extension_message": "ProjectSpecExtension",
      "patch_mode": "merge"
    },
    {
      "target_proto": "project.proto",
      "target_message": "ProjectStatus",
      "extension_proto": "project_ext.proto",
      "extension_message": "ProjectStatusExtension",
      "patch_mode": "merge"
    }
  ],
  "validation_overrides": []
}
```

**Run with config:**
```bash
bazel run //tools/proto-patcher -- \
  --base_proto_dir=/tmp/proto-demo/base \
  --ext_proto_dir=/tmp/proto-demo/ext \
  --output_dir=/tmp/proto-demo/output \
  --config=/tmp/proto-demo/patch_config.json \
  --verbose
```

### Mode 2: Auto-Generated Configuration (Quick Testing)

Automatically generate configuration based on naming conventions:

**Naming Convention:**
- Extension files must end with `_ext.proto`
- Extension messages must end with `Extension`
- Example: `project_ext.proto` → `project.proto`
- Example: `ProjectSpecExtension` → `ProjectSpec`

**Run with auto-config:**
```bash
bazel run //tools/proto-patcher -- \
  --base_proto_dir=/path/to/base \
  --ext_proto_dir=/path/to/ext \
  --output_dir=/path/to/output \
  --generate_config \
  --field_prefix=UBER_INTERNAL_ \
  --tag_start=999 \
  --verbose
```

## Patch Modes

### Merge Mode (`patch_mode: "merge"`)

Merges extension fields into an existing message in the base proto.

**Before:**
```protobuf
message ProjectSpec {
  string description = 1;
  string owner = 2;
}
```

**Extension:**
```protobuf
message ProjectSpecExtension {
  string cost_center = 1;
  int64 budget_usd = 2;
}
```

**After (merged):**
```protobuf
message ProjectSpec {
  string description = 1;
  string owner = 2;
  string UBER_INTERNAL_cost_center = 999;
  int64 UBER_INTERNAL_budget_usd = 1000;
}
```

### Copy Mode (`patch_mode: "copy"`)

Copies the entire extension message as a new top-level message.

**Extension:**
```protobuf
message OrganizationConfig {
  string org_id = 1;
  repeated string tags = 2;
}
```

**After (copied):**
```protobuf
// Original messages unchanged
...

// New message added as-is
message OrganizationConfig {
  string org_id = 1;
  repeated string tags = 2;
}
```

## Working with Nested Messages

The proto-patcher fully supports extending nested messages. You can patch messages at any nesting level without special configuration - just reference the message name directly.

### What Nested Messages Are

Nested messages are proto messages defined within another message or used as field types:

```protobuf
message DeploymentSpec {
  string model_id = 1;
  
  // ResourceConfig is a "nested" message used as a field type
  ResourceConfig resources = 2;
}

// This message can be extended even though it's nested
message ResourceConfig {
  int32 cpu_millicores = 1;
  int64 memory_mb = 2;
}
```

### How to Extend Nested Messages

**Step 1: Identify the nested message name**

Look for messages used as field types in your base proto:

```protobuf
message DeploymentSpec {
  ResourceConfig resources = 3;      // ← ResourceConfig is nested here
  AutoscalingConfig autoscaling = 4; // ← AutoscalingConfig is nested here
}
```

**Step 2: Create extension messages**

Create extension messages for each nested message you want to extend:

```protobuf
// Extension for top-level message
message DeploymentSpecExtension {
  string cost_center = 1;
  bool requires_approval = 2;
}

// Extension for nested ResourceConfig
message ResourceConfigExtension {
  string cloud_provider = 1;
  string instance_type = 2;
  bool spot_eligible = 3;
}

// Extension for nested AutoscalingConfig
message AutoscalingConfigExtension {
  int32 cooldown_seconds = 1;
  repeated string custom_metrics = 2;
}
```

**Step 3: Configure patches for each message**

Add separate patch rules for each message (top-level and nested):

```json
{
  "field_prefix": "UBER_",
  "tag_start": 1000,
  "patches": [
    {
      "target_proto": "deployment.proto",
      "target_message": "DeploymentSpec",
      "extension_proto": "deployment_ext.proto",
      "extension_message": "DeploymentSpecExtension",
      "patch_mode": "merge"
    },
    {
      "target_proto": "deployment.proto",
      "target_message": "ResourceConfig",  // ← Just use the message name
      "extension_proto": "deployment_ext.proto",
      "extension_message": "ResourceConfigExtension",
      "patch_mode": "merge"
    },
    {
      "target_proto": "deployment.proto",
      "target_message": "AutoscalingConfig",  // ← No special nesting syntax needed
      "extension_proto": "deployment_ext.proto",
      "extension_message": "AutoscalingConfigExtension",
      "patch_mode": "merge"
    }
  ]
}
```

**Step 4: Run the patcher**

```bash
bazel run //tools/proto-patcher -- \
  --base_proto_dir=/path/to/base \
  --ext_proto_dir=/path/to/ext \
  --output_dir=/path/to/output \
  --config=config.json \
  --verbose
```

### Complete Nested Message Example

**Base Proto: `deployment.proto`**
```protobuf
syntax = "proto3";

message Deployment {
  string id = 1;
  DeploymentSpec spec = 2;
}

message DeploymentSpec {
  string model_id = 1;
  ResourceConfig resources = 2;
}

message ResourceConfig {
  int32 cpu_millicores = 1;
  int64 memory_mb = 2;
}
```

**Extension Proto: `deployment_ext.proto`**
```protobuf
syntax = "proto3";

message DeploymentSpecExtension {
  string cost_center = 1;
}

message ResourceConfigExtension {
  string cloud_provider = 1;
  string instance_type = 2;
}
```

**Patched Output: `deployment_patched.proto`**
```protobuf
syntax = "proto3";

message Deployment {
  string id = 1;
  DeploymentSpec spec = 2;
}

message DeploymentSpec {
  string model_id = 1;
  ResourceConfig resources = 2;
  string UBER_cost_center = 1000;  // ← Extension field added
}

message ResourceConfig {
  int32 cpu_millicores = 1;
  int64 memory_mb = 2;
  string UBER_cloud_provider = 1000;   // ← Extension fields added
  string UBER_instance_type = 1001;    // ← to nested message
}
```

### Key Points About Nested Messages

✅ **No special syntax required** - Reference nested messages by their simple name, not a path (use `ResourceConfig`, not `DeploymentSpec.ResourceConfig`)

✅ **Independent tag numbering** - Each message gets its own tag numbering starting at `tag_start`. This means both `DeploymentSpec` and `ResourceConfig` can have fields starting at tag 1000.

✅ **Works at any depth** - Messages can be nested multiple levels deep and still be patched normally

✅ **Multiple patches to same file** - You can patch multiple messages (top-level and nested) in the same proto file, and all patches will be applied to a single output file

✅ **All field types supported** - Nested messages can have extension fields with any type:
  - Primitives: `string`, `int32`, `int64`, `bool`, `double`, etc.
  - Complex types: `repeated string`, `map<string, string>`
  - Enums (as field types)
  - Other message types

### Real-World Use Case

**Scenario:** You want to add cloud provider metadata to a Kubernetes-style deployment spec that has nested resource and autoscaling configurations.

**Base (OSS):**
```protobuf
message DeploymentSpec {
  int32 replicas = 1;
  ResourceConfig resources = 2;
  AutoscalingConfig autoscaling = 3;
}

message ResourceConfig {
  int32 cpu = 1;
  int64 memory = 2;
}

message AutoscalingConfig {
  bool enabled = 1;
  int32 min = 2;
  int32 max = 3;
}
```

**Extensions (Uber-specific):**
```protobuf
message ResourceConfigExtension {
  string cloud_provider = 1;     // "AWS", "GCP", "Azure"
  string instance_type = 2;       // "m5.xlarge", "n1-standard-4"
  repeated string availability_zones = 3;
}

message AutoscalingConfigExtension {
  bool predictive_scaling = 1;    // Uber-specific ML-based scaling
  repeated string custom_metrics = 2;
}
```

**Result:** Your internal deployment specs now have cloud-provider-specific fields in the nested messages, while the base OSS proto remains clean and cloud-agnostic.

### Try It Yourself

A complete nested message example is available at:

```bash
# View the example
ls /tmp/proto-demo-nested/

# Base proto with nested messages
cat /tmp/proto-demo-nested/base/deployment.proto

# Extensions for nested messages
cat /tmp/proto-demo-nested/ext/deployment_ext.proto

# Configuration
cat /tmp/proto-demo-nested/patch_config.json

# Run the patcher
cd /home/user/Uber/michelangelo
bazel run //tools/proto-patcher -- \
  --base_proto_dir=/tmp/proto-demo-nested/base \
  --ext_proto_dir=/tmp/proto-demo-nested/ext \
  --output_dir=/tmp/proto-demo-nested/output \
  --config=/tmp/proto-demo-nested/patch_config.json \
  --verbose

# View the result
cat /tmp/proto-demo-nested/output/deployment_patched.proto
```

## Complete Working Example

### Step 1: Create Base Proto

**File: `/tmp/demo/base/service.proto`**
```protobuf
syntax = "proto3";
package example;

message ServiceConfig {
  string name = 1;
  int32 replicas = 2;
}
```

### Step 2: Create Extension Proto

**File: `/tmp/demo/ext/service_ext.proto`**
```protobuf
syntax = "proto3";
package example.ext;

message ServiceConfigExtension {
  string cost_center = 1;
  string owner_email = 2;
  bool requires_approval = 3;
}
```

### Step 3: Create Configuration

**File: `/tmp/demo/config.json`**
```json
{
  "field_prefix": "ORG_",
  "tag_start": 1000,
  "patches": [
    {
      "target_proto": "service.proto",
      "target_message": "ServiceConfig",
      "extension_proto": "service_ext.proto",
      "extension_message": "ServiceConfigExtension",
      "patch_mode": "merge"
    }
  ]
}
```

### Step 4: Run Patcher

```bash
cd /home/user/Uber/michelangelo

bazel run //tools/proto-patcher -- \
  --base_proto_dir=/tmp/demo/base \
  --ext_proto_dir=/tmp/demo/ext \
  --output_dir=/tmp/demo/output \
  --config=/tmp/demo/config.json \
  --verbose
```

### Step 5: View Output

**File: `/tmp/demo/output/service_patched.proto`**
```protobuf
syntax = "proto3";
package example;

message ServiceConfig {
  string name = 1;
  int32 replicas = 2;
  string ORG_cost_center = 1000;
  string ORG_owner_email = 1001;
  bool ORG_requires_approval = 1002;
}
```

## Advanced Features

### Multiple Patches to Same File

The patcher correctly handles multiple patches to the same base proto file:

```json
{
  "patches": [
    {
      "target_proto": "project.proto",
      "target_message": "ProjectSpec",
      "extension_proto": "project_ext.proto",
      "extension_message": "ProjectSpecExtension",
      "patch_mode": "merge"
    },
    {
      "target_proto": "project.proto",
      "target_message": "ProjectStatus",
      "extension_proto": "project_ext.proto",
      "extension_message": "ProjectStatusExtension",
      "patch_mode": "merge"
    }
  ]
}
```

Both patches will be applied to the same output file (`project_patched.proto`).

### Validation Overrides (Future Feature)

The `validation_overrides` section allows modifying validation rules on existing base fields:

```json
{
  "validation_overrides": [
    {
      "target_proto": "project.proto",
      "target_message": "ProjectSpec",
      "field": "name",
      "new_validation": {
        "min_length": "10",
        "max_length": "100"
      }
    }
  ]
}
```

### Import Paths

If your protos have complex import dependencies, specify additional import paths:

```bash
bazel run //tools/proto-patcher -- \
  --base_proto_dir=/path/to/base \
  --ext_proto_dir=/path/to/ext \
  --output_dir=/path/to/output \
  --import_paths=/path/to/proto/deps,/another/path \
  --config=config.json
```

## Integration with Bazel

For automated patching in your build pipeline, use the `patched_proto_library` Bazel rule:

```python
load("//bazel/rules/proto:patched_proto.bzl", "patched_proto_library")

patched_proto_library(
    name = "project_patched_proto",
    base_proto = "//proto/api/v2:project_proto",
    extension_proto = "//extensions:project_ext_proto",
    config = "//config:patch_config.json",
)
```

See `bazel/rules/proto/patched_proto.bzl` for details.

## Troubleshooting

### Error: "base proto not found"

**Solution:** Ensure the `target_proto` in config matches the actual filename (e.g., `project.proto`, not just `project`).

### Error: "target message not found"

**Solution:**
1. Check message name spelling in config
2. Ensure the message exists in the base proto
3. For nested messages, use the full path (e.g., `Project.Spec`)

### Error: "tag number collision"

**Solution:** Increase `tag_start` to a higher number (e.g., 2000) to avoid conflicts with existing field tags.

### Error: "field name collision"

**Solution:**
1. Use a different `field_prefix`
2. Rename the extension field to avoid conflicts

### Import Errors

**Solution:** Add missing import paths using `--import_paths`:

```bash
--import_paths=/path/to/deps,/path/to/k8s/protos
```

## Testing

Run the full test suite:

```bash
cd /home/user/Uber/michelangelo
bazel test //tools/proto-patcher/...
```

Run specific test:

```bash
bazel test //tools/proto-patcher/patcher:patcher_test --test_output=all
```

## Best Practices

1. **Use Manual Config in Production:** Auto-generation is great for testing, but manual config gives you full control.

2. **Field Prefix Convention:** Use a clear prefix like `UBER_INTERNAL_` or `ORG_` to distinguish extension fields.

3. **Tag Numbering:** Start at 999 or higher to leave room for future OSS fields.

4. **Merge vs Copy:**
   - Use **merge** for extending existing CRD messages (Spec, Status)
   - Use **copy** for adding completely new message types

5. **Validation:** Extension fields support the same validation annotations as base fields.

6. **Version Control:** Keep your extension protos and configs in version control alongside your main codebase.

7. **Documentation:** Document why each extension field exists and its purpose.

## Example Output Structure

```
output/
├── project_patched.proto       # Base + extensions merged
├── deployment_patched.proto    # Another patched proto
└── cluster_patched.proto       # Yet another patched proto
```

## Next Steps

- Read `docs/EXTENDING.md` for architectural overview
- Check `examples/extensions/` for real-world examples
- See `bazel/rules/proto/patched_proto.bzl` for Bazel integration
- Review `PROTO_EXTENSIONS.md` for design decisions

## Support

For issues or questions:
1. Check the troubleshooting section above
2. Review test cases in `tools/proto-patcher/*/test.go`
3. Run with `--verbose` for detailed logs
