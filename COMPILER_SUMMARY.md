# Michelangelo Proto Compiler Summary

## All Available Compilers

### 1. **go_proto** (Standard Go)
- **Location**: `@io_bazel_rules_go//proto:go_proto`
- **Output**: `.pb.go`
- **Purpose**: Standard Protocol Buffers Go bindings
- **Example Generated Code**:
```go
type DemoMessage struct {
    Id    string `protobuf:"bytes,1,opt,name=id,proto3"`
    Name  string `protobuf:"bytes,2,opt,name=name,proto3"`
    Score int32  `protobuf:"varint,3,opt,name=score,proto3"`
}
```

### 2. **gogoslick_proto** (Gogo Optimized)
- **Location**: `@io_bazel_rules_go//proto:gogoslick_proto`
- **Output**: `.pb.go` (optimized)
- **Purpose**: High-performance Protocol Buffers with additional features
- **Features**: Faster marshaling, additional helper methods

### 3. **go_validation** ✅
- **Location**: `//bazel/rules/proto:go_validation`
- **Output**: `.pb.validation.go`
- **Purpose**: Field validation based on proto annotations
- **Example Generated Code**:
```go
func (this *DemoMessage) Validate(prefix string) error {
    if this.Id == "" {
        return status.Error(codes.InvalidArgument, prefix+"id is required")
    }
    if !pattern0.MatchString(this.Id) {
        return status.Error(codes.InvalidArgument, prefix+"id must match pattern")
    }
    return nil
}
```

### 4. **go_ext** ✅ (NEW - Created by us)
- **Location**: `//bazel/rules/proto:go_ext`
- **Output**: `.ext.go`
- **Purpose**: Extension validation with field verification and direct integration
- **Key Features**:
  - Field comparison with original proto
  - Direct validation using unsafe pointer conversion
  - Integration with original proto validation system
- **Example Generated Code**:
```go
func (this *DemoMessage) Validate(prefix string) error {
    if this.Id == "" {
        return status.Error(codes.InvalidArgument, prefix+"id is required")
    }
    if !pattern0.MatchString(this.Id) {
        return status.Error(codes.InvalidArgument, prefix+"id must match pattern")
    }
    return nil
}

func init() {
    v2.RegisterDemoMessageValidateExt(func(orig *v2.DemoMessage, prefix string) error {
        // Call ext validation directly on original type using unsafe pointer conversion
        // Types have identical structure, so this is safe
        extMsg := (*DemoMessage)(unsafe.Pointer(orig))
        return extMsg.Validate(prefix)
    })
}
```

### 5. **go_yarpc**
- **Location**: `//bazel/rules/proto:go_yarpc`
- **Output**: `.pb.yarpc.go`
- **Purpose**: YARPC RPC framework integration
- **Example Generated Code**:
```go
type DemoServiceClient interface {
    GetDemo(ctx context.Context, request *DemoRequest, opts ...yarpc.CallOption) (*DemoResponse, error)
}
```

### 6. **go_kubeyarpc**
- **Location**: `//bazel/rules/proto:go_kubeyarpc`
- **Output**: `.pb.kubeyarpc.go`
- **Purpose**: Kubernetes-aware YARPC services
- **Features**: Resource-aware handlers, Kubernetes integration

### 7. **go_kubeproto**
- **Location**: `//bazel/rules/proto:go_kubeproto`
- **Output**: Embedded in `.pb.go`
- **Purpose**: Kubernetes CRD support
- **Features**: DeepCopy methods, resource interfaces

### 8. **kube_proto_sql**
- **Location**: `//bazel/rules/proto:kube_proto_sql.bzl`
- **Output**: `.sql`
- **Purpose**: SQL schema generation
- **Example Generated Code**:
```sql
CREATE TABLE demo_message (
    id VARCHAR(64) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    score INT,
    email VARCHAR(255)
);
```

### 9. **kube_proto_yaml**
- **Location**: `//bazel/rules/proto:kube_proto_yaml.bzl`
- **Output**: `.yaml`
- **Purpose**: Kubernetes YAML manifests
- **Example Generated Code**:
```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: demomessages.michelangelo.ai
spec:
  group: michelangelo.ai
  versions:
  - name: v1
    served: true
    storage: true
```

## Using Multiple Compilers Together

Example BUILD.bazel configuration using all compilers:

```python
go_proto_library(
    name = "my_proto_all",
    compilers = [
        "@io_bazel_rules_go//proto:go_proto",      # Standard .pb.go
        "//bazel/rules/proto:go_validation",       # .pb.validation.go
        "//bazel/rules/proto:go_ext",              # .ext.go
        "//bazel/rules/proto:go_yarpc",            # .pb.yarpc.go
        "//bazel/rules/proto:go_kubeyarpc",        # .pb.kubeyarpc.go
        "//bazel/rules/proto:go_kubeproto",        # Kubernetes support
    ],
    proto = ":my_proto",
    # ... other config
)
```

## Files Generated for a Single Proto

For a proto file `example.proto`, the compilers generate:

1. `example.pb.go` - Core protobuf code (go_proto/gogoslick)
2. `example.pb.validation.go` - Validation functions (go_validation)
3. `example.ext.go` - Extension validation with direct integration (go_ext)
4. `example.pb.yarpc.go` - YARPC RPC code (go_yarpc)
5. `example.pb.kubeyarpc.go` - Kubernetes YARPC (go_kubeyarpc)
6. `example.sql` - SQL schema (kube_proto_sql)
7. `example.yaml` - Kubernetes YAML (kube_proto_yaml)

## The ext Compiler Advantage

The **go_ext** compiler we created adds unique capabilities:

1. **Field Verification**: Ensures ext protos match original proto structure
2. **Direct Validation**: High-performance validation using unsafe pointer conversion
3. **Integration Hooks**: Automatic registration with original proto validation system
4. **Separate Package Support**: Can generate in different packages
5. **Template Sharing**: Uses centralized templates from `/go/kubeproto/templates/`

This allows teams to:
- Add validation to existing protos without modifying them
- Create stricter validation rules for specific use cases
- Maintain backward compatibility
- Integrate seamlessly with existing validation infrastructure

## Build Examples

```bash
# Build with validation only
bazel build //proto/api/v2:v2_go_proto

# Build with ext validation
bazel build //proto/test/kubeproto/ext:ext_go_proto

# Build with validation and ext compilers
bazel build //proto/api/v2_ext:v2_ext_go_proto

# Generate SQL schemas
bazel build //proto/api/v2:v2_kube_proto_sql

# Generate Kubernetes YAML
bazel build //proto/api/v2:v2_kube_proto_yaml
```
