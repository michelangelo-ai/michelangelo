# Vendored k8s.io protos

These `.proto` files are copied verbatim from the `k8s.io/apimachinery` and
`k8s.io/api` Go modules at the version pinned in `go/go.mod`
(`k8s.io/apimachinery v0.31.14` / `k8s.io/api v0.31.14`), i.e. the exact
source Bazel/Gazelle compiles for the Go backend (`@io_k8s_apimachinery`,
`@io_k8s_api`) and that `tools/gen-descriptors.sh` bundles into
`descriptors.pb`.

`tools/gen-grpc-client.sh` uses this directory (instead of an external BSR
module) to generate the JS/Python client bindings for k8s types, so the
client-side types can't drift from what the Go server actually sends on
the wire. A prior version of that script depended on
`buf.build/coscene-io/kubernetes-apis`, an unofficial third-party mirror
that had redefined `ObjectMeta.creationTimestamp` and
`ManagedFieldsEntry.time` as `google.protobuf.Timestamp` instead of
`k8s.io.apimachinery.pkg.apis.meta.v1.Time` — a divergence from the real
upstream proto that caused the generated client to expect an RFC3339
string for a field the server sends as `{seconds, nanos}`.

To update after bumping the `k8s.io/apimachinery`/`k8s.io/api` version in
`go/go.mod`, re-copy the `generated.proto` files from the corresponding
`@io_k8s_apimachinery`/`@io_k8s_api` Bazel external repos (find their path
with `bazel query --output=location @io_k8s_apimachinery//pkg/apis/meta/v1:v1_proto`)
so this vendored copy stays byte-identical to what the Go backend compiles
against.
