Use scheme-based GVK resolution in ingester controller

## Summary
Replace `GetObjectKind().GroupVersionKind()` with
`Scheme.ObjectKinds()` for GVK resolution in the ingester
controller and module registration.

`GetObjectKind()` returns an empty GVK when the object has not
been through serialization, which is common for typed Go
structs created in-process. This caused silent failures in
metadata storage operations because the resulting TypeMeta had
empty Kind and APIVersion fields.

`Scheme.ObjectKinds()` resolves the GVK from the scheme
registry, which is always populated after `AddToScheme`. This
makes GVK resolution reliable regardless of how the object was
constructed. Each call site now validates the result and
returns a descriptive error on failure. A TODO referencing #943
documents that `gvks[0]` selection is non-deterministic when a
type is registered under multiple versions.

## Test plan
Added `TestSchemeGVKResolution` -- iterates all CRD objects in
the v2 scheme and asserts each resolves to a non-empty, unique
Kind via `Scheme.ObjectKinds()`. Added skeleton tests for
`handleDeletion` and `handleDeletionAnnotation` to verify
correct TypeMeta propagation. Existing test suite covers the
reconciliation paths.
