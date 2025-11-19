# Example Extension Protos

This directory contains example extension protos demonstrating how organizations can extend Michelangelo CRDs with their own fields.

## Files

- `project_ext.proto` - Example extensions for Project CRD
- `BUILD.bazel` - Example BUILD file showing how to use patched_proto_library

## Using These Examples

### In Your Own Repository

1. Copy the extension proto structure:
```bash
mkdir -p your-org/extensions
cp examples/extensions/project_ext.proto your-org/extensions/
```

2. Modify to your needs:
- Change package name to your organization
- Add/remove fields as needed
- Update validation rules
- Add other CRD extensions (deployment, model, etc.)

3. Create BUILD file:
```python
load("@michelangelo//bazel/rules/proto:patched_proto.bzl", "patched_proto_library")

patched_proto_library(
    name = "michelangelo_extended",
    base_protos = "@michelangelo//proto/api/v2:v2_proto",
    extension_protos = glob(["extensions/*.proto"]),
    field_prefix = "YOUR_ORG_",
    tag_start = 999,
    importpath = "your-org.com/michelangelo/proto/api/v2",
)
```

4. Import in your WORKSPACE:
```python
git_repository(
    name = "michelangelo",
    remote = "https://github.com/michelangelo-ai/michelangelo.git",
    commit = "COMMIT_SHA",
)
```

5. Use in your code:
```go
import v2 "your-org.com/michelangelo/proto/api/v2"

project := &v2.Project{
    Spec: &v2.ProjectSpec{
        Description: "My project",
        YOUR_ORG_owner_id: "uuid-here",
        YOUR_ORG_cost_center: "ML-123",
    },
}
```

## Extension Fields Explained

### ProjectSpecExtension

- **owner_id** - UUID identifying the project owner in your org's system
- **cost_center** - Budget code for accounting/billing
- **compliance_tags** - Data governance tags (PII, GDPR, etc.)
- **department** - Organizational unit
- **budget_usd** - Allocated budget
- **requires_approval** - Whether resource creation needs approval workflow
- **security_level** - Data classification level

### ProjectStatusExtension

- **internal_state** - Custom workflow state tracking
- **last_audit_timestamp** - When compliance was last checked
- **compliance_verified** - Whether compliance checks passed
- **budget_utilization_percent** - How much of budget is used

## Validation Features Demonstrated

The examples show various validation capabilities:

- **Required fields**: `required: true`
- **Format validation**: `uuid: true`, `pattern: "..."`
- **Range validation**: `min: "0"`, `max: "100"`
- **List validation**: `min_items: "1"`, `items: {in: [...]}`
- **Enum validation**: `min: "1"` (must not be INVALID value)
- **Optional fields**: `optional: true`
- **Custom error messages**: `msg: "..."`

## Adding More Extensions

To extend other CRDs:

```protobuf
// deployment_ext.proto
message DeploymentSpecExtension {
  string sla_tier = 1;
  repeated string monitoring_alerts = 2;
}

// model_ext.proto
message ModelSpecExtension {
  string model_owner = 1;
  bool production_approved = 2;
}
```

List all extension protos in BUILD file:
```python
extension_protos = glob(["extensions/*.proto"])
```

## Testing

Build the patched protos:
```bash
bazel build //:michelangelo_extended
```

Verify generated files:
```bash
ls bazel-bin/michelangelo_extended/*.pb.go
ls bazel-bin/michelangelo_extended/*.pb.validation.go
```

## Next Steps

1. Read the full documentation: `docs/EXTENDING.md`
2. Customize extensions for your organization
3. Build and test your patched protos
4. Use in your services


