# protoc-gen-kubeproto

A protoc plugin that generates Go code for Kubernetes CRDs from protobuf definitions. It extends standard `protoc-gen-go` output with CRD-specific helpers: blob storage field management, indexed field extraction, and enum unmarshalling.

## external_storage annotation

The `external_storage` field option tells the generator that a proto field is too large for etcd/MySQL and should be stored in blob storage (e.g. Terrablob). Annotate a field in your `.proto` file:

```protobuf
import "michelangelo/api/options.proto";

message PipelineRun {
  PipelineRunSpec spec = 1;
  PipelineRunStatus status = 2 [(michelangelo.api.external_storage) = true];
}
```

### Generated methods

For each CRD with at least one `external_storage` field, the generator produces three methods:

```go
// HasBlobFields returns true if this CRD has any external_storage fields.
func (m *PipelineRun) HasBlobFields() bool { return true }

// ClearBlobFields nils out every external_storage field so the object
// can be safely written to etcd without exceeding the size limit.
func (m *PipelineRun) ClearBlobFields() {
    if m.Status != nil {
        m.Status = nil
    }
}

// FillBlobFields copies Spec and Status from the blob-restored object
// back into the in-memory object, merging etcd metadata with blob payload.
func (m *PipelineRun) FillBlobFields(object k8sruntime.Object) {
    other := object.(*PipelineRun)
    m.Spec = other.Spec
    m.Status = other.Status
}
```

These methods implement the `BlobStorageObject` interface consumed by the storage layer to transparently split writes between etcd and blob storage.

## How the generator discovers blob fields

`findBlobFields` (in `main.go`) walks the proto message tree recursively and collects every field path where `external_storage = true`. It avoids infinite recursion by tracking already-visited message types. Paths are dot-separated Go field names, e.g. `Status.Conditions`.

`genClearCrdFields` then renders one nil-assignment per path, guarded by nil checks on every intermediate pointer:

```go
// for path "Spec.Conditions"
if m.Spec.Conditions != nil {
    m.Spec.Conditions = nil
}
```

`Spec` and `Status` are struct-embedded (not pointer) fields, so the generator skips the `!= nil` guard for those two path segments only.

## Repeated fields

The generator handles `external_storage` on fields nested inside one or more `repeated` message fields. When `findBlobFields` recurses into a repeated field, it appends `[]` to that segment of the path (e.g. `Status.Steps[]`). `genClearCrdFields` recognises this marker and emits a `for` loop instead of a direct field access.

### Example

```protobuf
message PipelineRunStepInfo {
  google.protobuf.Struct output = 6 [(michelangelo.api.external_storage) = true];
  repeated PipelineRunStepInfo sub_steps = 7;
  google.protobuf.Struct input  = 10 [(michelangelo.api.external_storage) = true];
}

message PipelineRunStatus {
  repeated PipelineRunStepInfo steps = 8;  // no annotation; sub-fields are annotated
}
```

Generated `ClearBlobFields` for a CRD that contains `PipelineRunStatus`:

```go
// path: Status.Steps[].Output
for _, _v0 := range m.Status.Steps {
    if _v0 != nil {
        if _v0.Output != nil {
            _v0.Output = nil
        }
    }
}
// path: Status.Steps[].Input
for _, _v0 := range m.Status.Steps {
    if _v0 != nil {
        if _v0.Input != nil {
            _v0.Input = nil
        }
    }
}
```

For doubly-nested repeated fields (e.g. `Status.Steps[].SubSteps[].Input`), the generator emits nested `for` loops, one per repeated segment.

### Self-referential types

When a repeated field's element type is self-referential (e.g. `PipelineRunStepInfo.sub_steps` has element type `PipelineRunStepInfo`), the existing cycle-detection in `findBlobFields` prevents infinite recursion by skipping already-visited message types on the current path. Only the first level of a self-referential repeated field is processed; deeper levels can be added as a future enhancement.

## FillBlobFields

`FillBlobFields` copies the entire `Spec` and `Status` from the blob-stored object back into the in-memory object. This is correct—the blob always contains the full object written before `ClearBlobFields` ran. Step metadata (name, state, etc.) is therefore restored from blob along with the large payloads. Selectively restoring only the annotated sub-fields per step element is a possible future optimisation.