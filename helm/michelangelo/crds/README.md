# CRDs

This directory is intentionally empty.

Michelangelo AI's CRDs are registered (and updated) by the apiserver at startup
through its `crdSync.enableCRDUpdate: true` config. The apiserver calls
`crd.SyncCRDs()` (`go/api/crd/sync.go`) which generates each CustomResource
Definition from the registered protobuf types in `proto-go/api/v2/*` and
applies them via the Kubernetes API.

If a future change ships static CRD YAML, drop the manifests in this directory
and Helm will install them before any chart templates render.
