# BERT CoLA Multi-Cluster KServe Demo

End-to-end demo: a real BERT model (fine-tuned on CoLA, linguistic
acceptability) served on CPU via Michelangelo's KServe backend
(`go/components/inferenceserver/backends/kserve.go`), running
simultaneously on two separate k3d clusters — `michelangelo-compute-1` and
`michelangelo-sandbox`.

## Architecture

```
                    k3d-michelangelo-sandbox (control plane + target)
                    ┌─────────────────────────────────────────────┐
   controllermgr    │  InferenceServer CR (this repo's CRD)        │
   (pod in sandbox ─┼──►  clusterTargets:                         │
   not in a pod)    │      - michelangelo-compute-1 ───────────────┼──┐
                    │      - michelangelo-sandbox                  │  │
                    │                                               │  │
                    │  KServe InferenceService + Triton predictor  │  │
                    │  pod (real inference, CPU-only)              │  │
                    │  MinIO (s3://deploy-models/...) ◄─────────────┼──┤ model repo
                    └─────────────────────────────────────────────┘  │
                                                                       │
                    k3d-michelangelo-compute-1 (target only)          │
                    ┌─────────────────────────────────────────────┐  │
                    │  KServe InferenceService + Triton predictor  │◄─┘
                    │  pod (real inference, CPU-only)              │
                    └─────────────────────────────────────────────┘
```

One `InferenceServer` CR (control-plane object, lives on sandbox) fans out
to a real KServe `InferenceService` + Triton predictor pod on *each*
cluster listed in `spec.clusterTargets`. Both predictor pods serve the same
model independently.

## Prerequisites

- Docker Desktop running with at least 60 GB of free disk space (Triton
  images are ~20 GB each — see "Known issues" below if Docker Desktop won't
  start at all).
- `k3d`, `kubectl`, `helm`, and `ma` (the Michelangelo CLI) installed.

## Step 0 — Stand up two k3d clusters with the Michelangelo sandbox

Starting point: a machine with Docker Desktop running. Everything below creates
the clusters, installs the sandbox, and gets `controllermgr`/`apiserver` running
as pods so the rest of the demo can proceed.

**1. Create the two clusters:**

```bash
# Control plane + serving target
python -m michelangelo.cli.sandbox.sandbox create-cluster michelangelo-sandbox

# Compute-only serving target (no Michelangelo control-plane components)
python -m michelangelo.cli.sandbox.sandbox create-cluster michelangelo-compute-1
```

**2. Install the Michelangelo sandbox on the control-plane cluster:**

```bash
helm upgrade --install michelangelo helm/michelangelo \
  --kube-context k3d-michelangelo-sandbox \
  -n default --create-namespace \
  -f helm/michelangelo/values-k3d.yaml \
  --set workflow.engine=temporal \
  --set workflow.endpoint=michelangelo-temporal-frontend:7233
```

Wait for all pods to be ready:

```bash
kubectl --context k3d-michelangelo-sandbox rollout status \
  deployment/michelangelo-apiserver \
  deployment/michelangelo-controllermgr \
  deployment/michelangelo-worker \
  -n default --timeout=180s
```

**3. Verify `controllermgr` and `apiserver` are running:**

```bash
kubectl --context k3d-michelangelo-sandbox get pod -n default \
  -l 'app in (michelangelo-controllermgr,michelangelo-apiserver)'
```

> **Temporal schema note:** Temporal's MySQL backend has no persistent volume
> in the sandbox. If MySQL ever restarts (e.g. after a machine reboot), the
> schema is wiped and `controllermgr`/`worker` will crash-loop with
> `Table 'temporal.schema_version' doesn't exist`. Re-run the schema setup
> from the runbook at the bottom of this section, then restart the affected
> deployments.

**Running controllermgr locally instead (advanced):**

If you prefer to run `controllermgr` on the host (e.g. to test unreleased code),
you need two things:

- `RUNTIME_ENVIRONMENT=local` so `go/cmd/controllermgr/config/local.yaml`
  is loaded on top of `base.yaml` (overrides the Cadence host with Temporal)
- A port-forward so Temporal is reachable on `localhost:7233`:

```bash
kubectl --context k3d-michelangelo-sandbox port-forward \
  svc/michelangelo-temporal-frontend 7233:7233 &

CONFIG_DIR=go/cmd/controllermgr/config \
RUNTIME_ENVIRONMENT=local \
tools/bazel run //go/cmd/controllermgr
```

> **Note:** The Temporal schema is stored in MySQL, which has no persistent
> volume in the sandbox. If MySQL restarts, the schema is wiped and you must
> re-run the schema setup before restarting `controllermgr`:
>
> ```bash
> ADMIN=$(kubectl get pod -n default -l app=michelangelo-temporal-admintools -o name | head -1)
> kubectl exec $ADMIN -- temporal-sql-tool --ep mysql --port 3306 \
>   --user root --password root --db temporal --pl mysql8 \
>   setup-schema --version 0.0
> kubectl exec $ADMIN -- temporal-sql-tool --ep mysql --port 3306 \
>   --user root --password root --db temporal_visibility --pl mysql8 \
>   create-database
> kubectl exec $ADMIN -- temporal-sql-tool --ep mysql --port 3306 \
>   --user root --password root --db temporal_visibility --pl mysql8 \
>   setup-schema --version 0.0
> kubectl exec $ADMIN -- temporal-sql-tool --ep mysql --port 3306 \
>   --user root --password root --db temporal --pl mysql8 \
>   update-schema --schema-dir /etc/temporal/schema/mysql/v8/temporal/versioned
> kubectl exec $ADMIN -- temporal-sql-tool --ep mysql --port 3306 \
>   --user root --password root --db temporal_visibility --pl mysql8 \
>   update-schema --schema-dir /etc/temporal/schema/mysql/v8/visibility/versioned
> kubectl exec $ADMIN -- temporal operator namespace create default
> kubectl rollout restart deployment michelangelo-temporal-frontend \
>   michelangelo-temporal-history michelangelo-temporal-matching \
>   michelangelo-temporal-worker michelangelo-worker -n default
> ```

## Step 2 — Install KServe on every target cluster

Check first — you likely only need to do this for clusters that don't have
it yet:

```bash
kubectl --context k3d-<cluster> get crd | grep kserve
```

If empty, install cert-manager (KServe's dependency) and KServe itself:

```bash
kubectl --context k3d-<cluster> apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.16.3/cert-manager.yaml
kubectl --context k3d-<cluster> wait --for=condition=Available --timeout=120s \
  deployment/cert-manager deployment/cert-manager-cainjector deployment/cert-manager-webhook -n cert-manager

kubectl --context k3d-<cluster> apply -f https://github.com/kserve/kserve/releases/download/v0.13.1/kserve.yaml
```

**Known issue**: KServe v0.13.1's release manifest references
`gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1`, which no longer exists
upstream — the controller pod will sit in `ImagePullBackOff` on its second
container. Remove that sidecar (the controller works fine without it,
metrics endpoint just becomes directly reachable on 8080 instead of proxied
through 8443):

```bash
kubectl --context k3d-<cluster> patch deploy kserve-controller-manager -n kserve \
  --type=json -p='[{"op":"remove","path":"/spec/template/spec/containers/1"}]'
kubectl --context k3d-<cluster> wait --for=condition=Available --timeout=90s \
  deployment/kserve-controller-manager -n kserve
```

Then apply the RBAC the Michelangelo controller needs to manage
`InferenceService` objects on this cluster, and the MinIO S3 credentials
KServe's storage-initializer needs:

```bash
kubectl --context k3d-<cluster> apply -f ../../resources/rbac-inferenceserver.yaml
kubectl --context k3d-<cluster> apply -f minio-s3-secret.yaml
```

## Step 3 — Build and import the CPU Triton+torch image

The stock `nvcr.io/nvidia/tritonserver` images don't include `torch`, and
(see "Known issues") only 24.09-py3 and later actually work CPU-only.
Build once:

```bash
docker build -t michelangelo-triton-torch:v2409-pinned triton-cpu-torch/
```

Import into **every** cluster that will run a predictor pod — this is a
separate containerd content store per cluster, so it must be repeated for
each one even though it's the same host Docker daemon:

```bash
k3d image import -c michelangelo-compute-1 michelangelo-triton-torch:v2409-pinned
k3d image import -c michelangelo-sandbox michelangelo-triton-torch:v2409-pinned
```

If `k3d image import` fails with `input/output error` on some nodes, that's
usually Docker Desktop disk pressure, not a real problem with the image —
see "Known issues" below, then just retry the import.

## Step 4 — Apply the ClusterServingRuntime

```bash
kubectl --context k3d-michelangelo-compute-1 apply -f clusterservingruntime-tritonserver-cpu.yaml
kubectl --context k3d-michelangelo-sandbox   apply -f clusterservingruntime-tritonserver-cpu.yaml
```

This creates/overwrites the runtime named `kserve-tritonserver`, which is
the one KServe actually auto-selects for `michelangelo/kserve-model-format:
triton` (see comments in the file for why the *other*
`clusterservingruntime-triton.yaml` in this directory is dead/unused).

## Step 5 — Run the bert-cola pipeline

```bash
ma pipeline dev_run --pipeline examples/bert_cola/pipeline.yaml
```

This trains the model and runs `serve.py`, which packages, registers, and
uploads everything automatically. Specifically, `serve.py` calls
`MinioStorageBackend.upload_flat()` after registration to write the deployable
directory as individual objects under the revision key —
`s3://deploy-models/<revision_name>/` — which is the flat layout KServe's
storage-initializer expects. No manual upload needed.

If you need to upload a manually-packaged directory by hand,
`upload_model_repo.py` does the same flat upload standalone:

```bash
python upload_model_repo.py \
  --local-dir /tmp/bert_cola_deployable_xxxx \
  --bucket deploy-models \
  --prefix <revision_name>
```

## Step 6 — Apply the InferenceServer and Deployment CRs

```bash
kubectl --context k3d-michelangelo-sandbox apply -f inferenceserver-bert-cola.yaml
kubectl --context k3d-michelangelo-sandbox apply -f deployment-bert-cola.yaml
```

The `InferenceServer` CR always lives on the sandbox cluster (that's where
`controllermgr`/`apiserver` run against); `spec.clusterTargets` is what
fans it out to compute-1 and sandbox as actual serving targets. See the
comments in `inferenceserver-bert-cola.yaml` for how to find the right
`host`/`port` per target — this is the part most likely to trip you up if
clusters get recreated (k3d assigns a new random host port each time).

If you're iterating and need to force a re-reconcile without changing the
spec (e.g. after patching a `ClusterServingRuntime` — RawDeployment mode
does not automatically pick up runtime changes on an already-created
`InferenceService`):

```bash
kubectl --context k3d-michelangelo-compute-1 delete inferenceservice inference-server-bert-cola-kserve -n default
kubectl --context k3d-michelangelo-sandbox annotate inferenceserver inference-server-bert-cola-kserve -n default \
  michelangelo/force-reconcile="$(date +%s)" --overwrite
```

## Configuring health-metric gates on the rollout

The `Deployment` CR supports PromQL-based health rules via `spec.healthCheckConfig`.
During a zonal rollout the `HealthCheckGate` evaluates every rule against Prometheus
after each cluster is updated; if any rule breaches its threshold the rollout is
automatically rolled back before it reaches the next cluster.

```yaml
spec:
  strategy:
    zonal:
      rolloutPeriodInSeconds: 60   # wait between clusters
  healthCheckConfig:
    # Prometheus reachable from controllermgr (running on the host).
    prometheusUrl: "http://localhost:9093"   # port-forward michelangelo-prometheus-server:80
    rules:
      # Roll back if the model's error rate exceeds 1 % over the last 2 minutes.
      - name: "error-rate"
        query: 'rate(nv_inference_request_failure{model="bert-cola"}[2m])'
        op: GT
        threshold: 0.01
      # Roll back if p99 inference latency exceeds 500 ms.
      - name: "p99-latency-us"
        query: >
          histogram_quantile(0.99,
            rate(nv_inference_request_duration_us_bucket{model="bert-cola"}[2m]))
        op: GT
        threshold: 500000
```

**Available Triton metrics** (scraped from `:8002/metrics` on each predictor pod):

| Metric | Description |
|--------|-------------|
| `nv_inference_request_success` | Cumulative successful requests |
| `nv_inference_request_failure` | Cumulative failed requests |
| `nv_inference_request_duration_us` | End-to-end latency histogram (µs) |
| `nv_inference_queue_duration_us` | Queue wait time histogram (µs) |
| `nv_inference_compute_infer_duration_us` | GPU/CPU compute time histogram (µs) |
| `nv_inference_pending_request_count` | Instantaneous queue depth |
| `nv_cpu_utilization` | CPU utilization [0–1] |

To scrape these from the sandbox Prometheus, port-forward the predictor pod's
metrics port and query it directly during development:

```bash
kubectl --context k3d-michelangelo-sandbox port-forward -n default \
  svc/inference-server-bert-cola-kserve-predictor 8002:8002 &
curl -s http://localhost:8002/metrics | grep nv_inference_request
```

The `prometheusUrl` in `healthCheckConfig` must be reachable from wherever
`controllermgr` runs (the host Mac in local dev). Port-forward sandbox Prometheus
to a local port and use that URL:

```bash
kubectl --context k3d-michelangelo-sandbox port-forward -n default \
  svc/michelangelo-prometheus-server 9093:80 &
```

## Verify

**1. Check the InferenceServer CR's status** (the source of truth for
whether both targets are actually serving — don't rely on the `Deployment`
CR's own status, see the note in `deployment-bert-cola.yaml`):

```bash
kubectl --context k3d-michelangelo-sandbox get inferenceserver inference-server-bert-cola-kserve -n default \
  -o jsonpath='{.status.state}{"\n"}{.status.clusterStatuses}'
```

Expect:

```
INFERENCE_SERVER_STATE_SERVING
[{"clusterId":"michelangelo-compute-1","state":"INFERENCE_SERVER_STATE_SERVING"},{"clusterId":"michelangelo-sandbox","state":"INFERENCE_SERVER_STATE_SERVING"}]
```

**2. Check both predictor pods are 1/1 Running with 0 (or low, stable)
restarts:**

```bash
kubectl --context k3d-michelangelo-compute-1 get pod -n default -l serving.kserve.io/inferenceservice=inference-server-bert-cola-kserve
kubectl --context k3d-michelangelo-sandbox   get pod -n default -l serving.kserve.io/inferenceservice=inference-server-bert-cola-kserve
```

**3. Send a real inference request to each cluster** — this is the only
check that actually proves the model runs, as opposed to just "the
container started":

```bash
kubectl --context k3d-michelangelo-compute-1 port-forward -n default \
  svc/inference-server-bert-cola-kserve-predictor 8097:80 &
kubectl --context k3d-michelangelo-sandbox port-forward -n default \
  svc/inference-server-bert-cola-kserve-predictor 8098:80 &
sleep 3

cat > /tmp/infer_payload.json <<'EOF'
{"inputs": [
  {"name": "input_ids", "shape": [1, 128], "datatype": "INT64", "data": [101, 7592, 2088, 102, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]},
  {"name": "attention_mask", "shape": [1, 128], "datatype": "INT64", "data": [1,1,1,1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0]},
  {"name": "token_type_ids", "shape": [1, 128], "datatype": "INT64", "data": [0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0]}
]}
EOF

curl -s -X POST http://localhost:8097/v2/models/bert-cola/infer -H "Content-Type: application/json" --data @/tmp/infer_payload.json
curl -s -X POST http://localhost:8098/v2/models/bert-cola/infer -H "Content-Type: application/json" --data @/tmp/infer_payload.json
```

Expect an HTTP 200 with real logits from both, e.g.:

```json
{"model_name":"bert-cola","model_version":"0","outputs":[{"name":"logits","datatype":"FP32","shape":[1,2],"data":[0.0745,0.4479]}]}
```

(The exact numbers are deterministic for a given checkpoint + input — both
clusters should return identical values since they're running the same
model.)

## Known issues (already fixed, documented for future reference)

1. **`GetServerStatus` bug in `kserve.go`** — used to always report
   `SERVING` regardless of the real InferenceService state. Fixed in commit
   `8e2b5092`.
2. **Stale import path in the Triton packager template** —
   `user_model.py.tmpl` imported from `uber.ai.michelangelo` instead of
   `michelangelo`, breaking every generated model package. Fixed in the
   same commit.
3. **Triton 2.33.0 (23.04-py3) SIGABRTs on CPU-only inference** — the
   python backend unconditionally calls a CUDA pointer-attribute probe
   during every inference request, and throws a fatal, unhandled exception
   if it fails (which it always does with no GPU) — `instance_group {kind:
   KIND_CPU}` in `config.pbtxt` does not prevent this. `CUDA_VISIBLE_DEVICES=-1`,
   swapping in CUDA's build-time stub `libcuda.so`, and removing
   `libcuda.so*` entirely were all tried and all failed identically —
   it's baked into that release's compiled binary. Fixed by upgrading the
   base image to 24.09-py3 (Triton 2.50.0).
4. **`not enough values to unpack` in `modeling_bert.py`** — two distinct
   causes, both fixed:
   - An unpinned `pip install transformers` pulled in `5.13.0`, whose
     `BertModel.forward()` has a different internal signature than the
     `4.46.3` the checkpoint was trained/saved against. Fixed by pinning
     the version (see `triton-cpu-torch/Dockerfile`).
   - `examples/bert_cola/model.py`'s `BertColaModel.predict()` never added
     a batch dimension before calling the HF model. Triton's generated
     per-sample batching loop (`user_model.py.tmpl`, when
     `process_batch=True`) strips the batch dim before calling `predict()`
     per the model's own documented per-sample `[128]` input contract, so
     `input_ids` arrived as a 1-D tensor and `BertModel.forward()` (which
     requires 2-D) crashed trying to unpack `input_ids.size()`. Fixed with
     `.unsqueeze(0)` on inputs / `.squeeze(0)` on the output logits.
5. **`kubernetes.default.svc` unreachable for a cluster targeting
   itself** — `spec.clusterTargets[].kubernetes.host` for a target that's
   the *same* cluster the `InferenceServer` CR lives on can't use
   `https://kubernetes.default.svc` (in-cluster DNS) unless
   `controllermgr` itself runs as a pod inside that cluster. For local dev,
   `controllermgr` runs on the host Mac, so that hostname doesn't resolve.
   Use the cluster's own externally-exposed API port instead (`docker port
   k3d-<cluster>-serverlb | grep 6443`). This also affected the pre-existing
   `inference-server-multi` demo CR, which was silently stuck in
   `INFERENCE_SERVER_STATE_CREATING` on its sandbox target until fixed the
   same way.
6. **Docker Desktop total disk exhaustion** — building/pulling many ~6-20GB
   Triton images in one session can grow `Docker.raw` (Docker Desktop's VM
   disk) enough to fill the host's shared APFS container to 0 bytes free,
   which crashes Docker Desktop entirely (`Docker Desktop is unable to
   start`) and can even cause containerd I/O errors during `k3d image
   import`. If this happens: clear `~/Library/Caches` and Trash first (this
   alone can recover 100+GB without touching any Docker/k3d state), restart
   Docker Desktop, then `docker builder prune` / `docker image prune` /
   `docker rmi` obsolete image tags. Don't reach for deleting
   `Docker.raw` directly unless genuinely out of other options — it wipes
   every image, container, volume, and both k3d clusters.
