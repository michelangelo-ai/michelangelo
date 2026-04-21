# Local michelangelo-controllermgr with Cluster Affinity

End-to-end test plan for the `BatchRayCluster.GetAffinity()` change that drives cluster
assignment from the `michelangelo/cluster-affinity` label.

- **Code under test**: `go/components/jobs/scheduler/framework/job.go:53-69`
- **Strategy**: `ClusterOnlyAssignmentStrategy` (engine that consumes `GetAffinity()`)
- **Label**: `michelangelo/cluster-affinity` (`go/components/jobs/common/constants/constants.go:284`)
- **Expected reasons**: `cluster_matched_by_affinity`, `cluster_default_selected`, `no_clusters_found`

The plan exercises the full path: client → `michelangelo-apiserver` (gRPC/YARPC) →
controllermgr → assignment engine → RayCluster status. No `kubectl apply` for ray
clusters or cluster CRs — the apiserver handles entity creation so we exercise the
real user-facing flow.

---

## Phase 0 — Prerequisites

```bash
brew install grpcurl jq                      # one-time
which bazel k3d kubectl docker grpcurl jq    # confirm all on PATH
```

Repo root: `/Users/sidharth.padhee/Code/michelangelo-ai/worktrees/cluster-assignment-logic-for-ray-clusters`
(this worktree).

---

## Phase 1 — Build local controllermgr image

The canonical recipe lives in `.github/workflows/dev-release.yml` (which runs on
`ubuntu-24.04`, so its `bazel build` produces a Linux binary natively). On a
darwin/arm64 host you CANNOT just run `bazel build //go/cmd/controllermgr` and
ship the binary into a Linux container — it'll be a Mach-O binary and the
container will crash with `exec /app: exec format error`.

The BUILD.bazel for controllermgr forces `-linkmode external -extldflags -static`
on Linux targets (see `go/cmd/controllermgr/BUILD.bazel:57-67`), which requires
a Linux C cross-toolchain that isn't shipped with the repo. So
`--platforms=@io_bazel_rules_go//go/toolchain:linux_arm64` from darwin fails with
`cannot open ... stdlib_/pkg/linux_arm64/runtime/cgo.a`.

The reliable path is to run the bazel build inside a Linux container that
matches the host arch (linux/arm64 on M-series Mac, linux/amd64 on Intel),
exactly like CI does on `ubuntu-24.04`.

> **Why not `gcr.io/bazel-public/bazel:7.4.1`?** That image is published for
> linux/amd64 only — pulling with `--platform linux/arm64` on an M-series Mac
> fails with `image ... was found but does not provide the specified platform`.
> So we install bazelisk inside an arm64 Ubuntu image instead. (On Intel hosts
> you can use `gcr.io/bazel-public/bazel:7.4.1` directly with
> `--platform linux/amd64` and skip the bazelisk install dance.)

```bash
# 1) Run bazel inside a Linux container to produce a Linux binary.
#    Apple Silicon (linux/arm64) — install bazelisk inside ubuntu:24.04.
#    First run is ~3 min for apt + bazel download; subsequent runs hit the
#    bazel cache mounted at ~/.cache/bazel-linux and are fast.
mkdir -p "$HOME/.cache/bazel-linux"
docker run --rm \
  --platform linux/arm64 \
  -v "$PWD":/workspace \
  -v "$HOME/.cache/bazel-linux":/root/.cache/bazel \
  -w /workspace \
  ubuntu:24.04 \
  bash -c '
    set -euo pipefail
    apt-get update -qq
    apt-get install -y -qq curl python3 build-essential ca-certificates >/dev/null
    curl -sSL -o /usr/local/bin/bazel \
      https://github.com/bazelbuild/bazelisk/releases/download/v1.22.0/bazelisk-linux-arm64
    chmod +x /usr/local/bin/bazel
    bazel build //go/cmd/controllermgr
  '

# Intel Mac / Linux amd64 alternative — official bazel image works directly:
# docker run --rm --platform linux/amd64 \
#   -v "$PWD":/workspace \
#   -v "$HOME/.cache/bazel-linux":/root/.cache/bazel \
#   -w /workspace \
#   gcr.io/bazel-public/bazel:7.4.1 \
#   build //go/cmd/controllermgr

# 2) Copy the binary into the build context (repo root).
#    Required because `bazel-bin` is a symlink that lives outside the repo,
#    and Docker COPY cannot reach paths outside the build context.
cp bazel-bin/go/cmd/controllermgr/controllermgr_/controllermgr controllermgr

# 3) Sanity-check the architecture BEFORE building the image.
file controllermgr
# Expected: ELF 64-bit LSB executable, ARM aarch64 (or x86-64 for amd64)
# WRONG:    Mach-O 64-bit executable arm64  ← step 1 ran natively, not in Docker

# 4) Build the image with the OSS Dockerfile.
#    --platform forces Docker to tag the image for the matching arch so k3d
#    selects the right manifest on multi-arch hosts.
docker build \
  --platform linux/arm64 \
  -f docker/service.Dockerfile \
  --build-arg BINARY_PATH=controllermgr \
  --build-arg CONFIG_PATH=go/cmd/controllermgr/config \
  -t ghcr.io/michelangelo-ai/controllermgr:local \
  .
```

Verify:

```bash
docker images ghcr.io/michelangelo-ai/controllermgr:local
docker inspect ghcr.io/michelangelo-ai/controllermgr:local \
  --format '{{.Os}}/{{.Architecture}}'
# Expected: linux/arm64 (or linux/amd64 on Intel)
```

> **Symptom → fix**: if `kubectl logs michelangelo-controllermgr` shows
> `exec /app: exec format error`, the binary is for the wrong OS/arch.
> Re-run step 1 inside a Linux container, then steps 2–4, then re-import via
> `k3d image import` and `ma sandbox sync`.

> **Re-running after a failed attempt**: clear stale artifacts so the next
> build doesn't reuse the bad binary or image:
>
> ```bash
> rm -f controllermgr                                                # stale Mach-O binary
> docker rmi -f ghcr.io/michelangelo-ai/controllermgr:local || true  # stale image
> kubectl delete pod michelangelo-controllermgr --ignore-not-found   # force pull next sync
> ```

---

## Phase 2 — Wire the sandbox to use the local image

Edit `python/michelangelo/cli/sandbox/resources/michelangelo-controllermgr.yaml`:

```yaml
spec:
  template:
    spec:
      containers:
      - name: michelangelo-controllermgr
        image: ghcr.io/michelangelo-ai/controllermgr:local   # was :main
        imagePullPolicy: Never                                # added
```

`Never` forces k3d to use the locally-imported image instead of pulling from ghcr.io.

---

## Phase 3 — Create sandbox + load image

```bash
ma sandbox create
k3d image import ghcr.io/michelangelo-ai/controllermgr:local \
  -c michelangelo-sandbox
ma sandbox sync   # re-rolls the controllermgr deployment with :local
```

Confirm the new pod is running with the local image:

```bash
kubectl --context k3d-michelangelo-sandbox -n default get pods -l app=michelangelo-controllermgr -o wide
kubectl --context k3d-michelangelo-sandbox -n default describe pod -l app=michelangelo-controllermgr | grep -E 'Image:|Image ID:'
```

---

## Phase 4 — Smoke-test the apiserver

The apiserver is YARPC over gRPC with reflection. **Required headers** on every call:
`rpc-caller`, `rpc-service: ma-apiserver`, `rpc-encoding: proto`, plus `-max-time` for
the TTL (YARPC rejects without these — see `go/cmd/apiserver/yarpc.go` and
`go/cmd/apiserver/main.go: const serverName = "ma-apiserver"`).

```bash
grpcurl -plaintext \
  -H 'rpc-caller: grpcurl' \
  -H 'rpc-service: ma-apiserver' \
  -H 'rpc-encoding: proto' \
  127.0.0.1:15566 list
```

Expected: `michelangelo.api.v2.RayClusterService`, `ClusterService`, etc.

---

## Phase 5 — Register a second compute cluster (optional but recommended)

The sandbox auto-registers exactly one cluster (`michelangelo-compute-0` via
`sandbox.py:1406-1469`). To exercise the affinity-vs-fallback distinction, register a
second one via apiserver:

1. Spin up a second k3d cluster (`k3d cluster create michelangelo-compute-2 ...`)
   on the shared sandbox network.
2. Create the matching `cluster-michelangelo-compute-2-{client-token,ca-data}` Secrets
   in the sandbox's `ma-system` namespace (`sandbox.py:1472` is the reference).
3. Call `ClusterService/CreateCluster` with the new cluster's host/port:

```bash
grpcurl -plaintext -max-time 30 \
  -H 'rpc-caller: grpcurl' \
  -H 'rpc-service: ma-apiserver' \
  -H 'rpc-encoding: proto' \
  -d @ 127.0.0.1:15566 \
  michelangelo.api.v2.ClusterService/CreateCluster <<'EOF'
{
  "cluster": {
    "metadata": {"name": "michelangelo-compute-2", "namespace": "ma-system"},
    "spec": {
      "kubernetes": {
        "rest": {
          "host": "https://host.docker.internal",
          "port": "<port-from-k3d-kubeconfig>",
          "tokenTag":  "cluster-michelangelo-compute-2-client-token",
          "caDataTag": "cluster-michelangelo-compute-2-ca-data"
        },
        "skus": []
      }
    }
  }
}
EOF
```

Verify the apiserver sees both:

```bash
grpcurl -plaintext -max-time 30 \
  -H 'rpc-caller: grpcurl' -H 'rpc-service: ma-apiserver' -H 'rpc-encoding: proto' \
  -d '{"namespace":"ma-system"}' \
  127.0.0.1:15566 michelangelo.api.v2.ClusterService/ListCluster | jq '.clusterList.items[].metadata.name'
```

---

## Phase 6 — Test cases

Helper scripts already live at `/Users/sidharth.padhee/Code/jsons/rayclusters/oss/`:
- `create-ray-cluster.sh` — submits a RayCluster via `CreateRayCluster`
- `list-ray-cluster.sh` — lists via `ListRayCluster`

Run from `/Users/sidharth.padhee/Code/jsons/`. Edit `metadata.labels` per case.

### Case A — affinity matches an existing cluster

Set `metadata.labels."michelangelo/cluster-affinity" = "michelangelo-compute-2"` in
`create-ray-cluster.sh` and run it.

**Expect**: `assignment.cluster == "michelangelo-compute-2"`, `Scheduled` reason
`cluster_matched_by_affinity`. Mirrors `cluster_only_assignment_engine_test.go:108-114`.

### Case B — affinity points to an unknown cluster (fallback)

Change the label value to `does-not-exist` and re-create with a new `metadata.name`.

**Expect**: `assignment.cluster == "michelangelo-compute-0"` (or whichever the cache
returns first), reason `cluster_default_selected`. Mirrors
`cluster_only_assignment_engine_test.go:115-122`.

### Case C — no affinity label at all

Remove the entire `metadata.labels` block (or omit the `michelangelo/cluster-affinity`
key) and re-create.

**Expect**: first available cluster, reason `cluster_default_selected`. Mirrors
`cluster_only_assignment_engine_test.go:123-130`.

### Verification command (used for every case)

```bash
grpcurl -plaintext -max-time 30 \
  -H 'rpc-caller: grpcurl' -H 'rpc-service: ma-apiserver' -H 'rpc-encoding: proto' \
  -d '{"name":"<rc-name>","namespace":"default"}' \
  127.0.0.1:15566 michelangelo.api.v2.RayClusterService/GetRayCluster \
  | jq '.rayCluster.status | {
      cluster: .assignment.cluster,
      conditions: [.statusConditions[] | {type, reason, message}]
    }'
```

Look at:
- `.assignment.cluster` → confirms which cluster routing chose
- `Scheduled` condition `reason` → confirms the strategy's decision path

---

## Phase 7 — Tail controllermgr logs while testing

```bash
kubectl --context k3d-michelangelo-sandbox -n default logs -f \
  -l app=michelangelo-controllermgr --tail=100 \
  | grep -iE 'affinity|assignment|cluster_(matched|default|not)'
```

Look for log lines from `ClusterOnlyAssignmentStrategy.Select`. The `getResourceClassKeyFromPodSpec`
TODO at `job.go:165` means resource-aware routing is still default-bucket; only the
affinity branch matters here.

---

## Phase 8 — Cleanup

```bash
# delete RayClusters created during the test
for n in <names>; do
  grpcurl -plaintext -max-time 30 \
    -H 'rpc-caller: grpcurl' -H 'rpc-service: ma-apiserver' -H 'rpc-encoding: proto' \
    -d "{\"name\":\"$n\",\"namespace\":\"default\"}" \
    127.0.0.1:15566 michelangelo.api.v2.RayClusterService/DeleteRayCluster
done

# tear down extra compute cluster if you registered one
k3d cluster delete michelangelo-compute-2

# nuke the whole sandbox
ma sandbox delete
```

---

## Reference: useful endpoints

```
Michelangelo UI -> http://localhost:8090
Grafana -> http://localhost:3000
Michelangelo apiserver (gRPC) -> 127.0.0.1:15566
Envoy gRPC-Web proxy -> 127.0.0.1:8081
Ray Dashboard (kubectl port-forward required) -> http://localhost:8265
```

Full list in the conversation transcript or dump from `sandbox.py:27-50, 953-960, 1807-1816`.

## Reference: required YARPC headers (apiserver)

```
-H 'rpc-caller: <anything>'
-H 'rpc-service: ma-apiserver'   # dispatcher name from main.go
-H 'rpc-encoding: proto'
-max-time 30                      # TTL — YARPC rejects without it
```
