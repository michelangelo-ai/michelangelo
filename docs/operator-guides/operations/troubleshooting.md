---
last_verified: 2026-10-14
sidebar_position: 3
---

# Troubleshooting

This guide is for platform operators diagnosing issues with a Michelangelo AI deployment. All `kubectl` commands assume access to the control plane cluster. Commands use `<release-namespace>` for the namespace the Helm chart is installed in and resource names for a Helm release named `michelangelo` (workload names are `<release>-<component>`). Cluster resources are stored in the `ma-system` namespace.

---

## Jobs not being scheduled

**Symptoms**: Jobs are submitted but remain in a pending state with no cluster assignment.

**Diagnostics**:
```bash
# Check for scheduler errors in the controller manager
kubectl -n <release-namespace> logs deployment/michelangelo-controllermgr | grep -i "scheduler\|assign\|enqueue"

# List registered compute clusters and their status
kubectl -n ma-system get clusters

# Inspect a specific cluster's status conditions
kubectl -n ma-system describe cluster <cluster-name>
```

**Likely causes**:
- No compute clusters are registered — complete the [cluster registration steps](../setup/register-a-compute-cluster-to-michelangelo-control-plane.md)
- The Cluster CRD has the wrong `host` or `port` — the control plane cannot reach the compute cluster's API server
- The `ray-manager` token Secret in the control plane is missing or has expired
- The job requested resources (GPU, CPU) that no registered cluster can satisfy

---

## Compute cluster registration failures

**Symptoms**: The Cluster CRD is created but the cluster status shows unhealthy or unknown.

**Diagnostics**:
```bash
# Inspect cluster conditions
kubectl -n ma-system describe cluster <cluster-name>

# Verify the token and CA secrets exist in the control plane
kubectl -n default get secret cluster-<cluster-name>-client-token
kubectl -n default get secret cluster-<cluster-name>-ca-data

# Confirm the token secret is populated (output should be > 0)
kubectl -n default get secret cluster-<cluster-name>-client-token \
  -o jsonpath='{.data.token}' | wc -c

# Test network connectivity from the control plane to the compute API server
kubectl -n <release-namespace> run connectivity-test --rm -it --restart=Never \
  --image=curlimages/curl -- curl -k https://<compute-host>:<port>/healthz
```

**Likely causes**:
- Network policy or firewall is blocking the control plane from reaching the compute cluster's API server
- The token Secret is missing the `token` key or was not populated (check the `kubernetes.io/service-account.name` annotation on the Secret)
- The CA data does not match the compute cluster's TLS certificate (CA data mismatch)

---

## Ray pods not starting on the compute cluster

**Symptoms**: A RayCluster or RayJob resource is created on the compute cluster, but head or worker pods remain Pending or enter CrashLoopBackOff.

**Diagnostics**:
```bash
# Check on the compute cluster (use its kubectl context)
kubectl --context <compute-context> get rayclusters,rayjobs
kubectl --context <compute-context> describe raycluster <name>

# List pods for the cluster
kubectl --context <compute-context> get pods -l ray.io/cluster=<cluster-name>

# Check head pod logs
kubectl --context <compute-context> logs <head-pod-name>

# Verify storage config is present on the compute cluster
kubectl --context <compute-context> get configmap michelangelo-config
kubectl --context <compute-context> get secret aws-credentials
```

**Likely causes**:
- The `michelangelo-config` ConfigMap is missing or has the wrong `AWS_ENDPOINT_URL`
- The container image cannot be pulled (wrong registry, missing imagePullSecret)
- Insufficient CPU or memory quota on the compute cluster — check `kubectl --context <compute-context> describe nodes`

---

## Worker cannot connect to the API server

**Symptoms**: Worker pods crash-loop or restart repeatedly. Logs show connection refused, TLS errors, or authentication failures connecting to the API server.

**Diagnostics**:
```bash
# Check recent worker logs
kubectl -n <release-namespace> logs deployment/michelangelo-worker --tail=100

# Verify the worker's configured API server address
kubectl -n <release-namespace> get configmap michelangelo-worker-config -o yaml | grep -A3 "worker:"

# Confirm the API server deployment is running
kubectl -n <release-namespace> get deployment michelangelo-apiserver
kubectl -n <release-namespace> get pods -l app.kubernetes.io/component=apiserver
```

**Likely causes**:
- `worker.address` in the worker ConfigMap (rendered from the API server Service name and `apiserver.port`) points to the wrong hostname or port — it must resolve to the API server from within the release namespace
- `worker.useTLS: true` is set but the API server's certificate is not trusted — ensure the CA bundle is mounted into the worker pod
- The API server is not yet ready (check its pod status and readiness probe)

---

## Temporal / Cadence connectivity issues

**Symptoms**: Workflows fail to start. Worker logs contain errors like `failed to connect to temporal`, `context deadline exceeded`, or `domain not found`.

**Diagnostics**:
```bash
# Check worker logs for workflow engine errors
kubectl -n <release-namespace> logs deployment/michelangelo-worker | grep -i "temporal\|cadence\|workflow"

# Inspect the configured workflow engine endpoint
kubectl -n <release-namespace> get configmap michelangelo-worker-config -o yaml \
  | grep -A8 "workflow-engine:"

# Test TCP connectivity to Temporal from a worker pod
kubectl -n <release-namespace> exec deployment/michelangelo-worker -- \
  nc -zv temporal.your-domain.com 7233
```

**Likely causes**:
- `workflow-engine.host` has the wrong hostname or port (Temporal default is `7233`)
- The Temporal domain (`uniflow`, `default`) has not been created — create it with the Temporal CLI or admin tools
- Network policy in the release namespace is blocking egress to the Temporal endpoint

---

## InferenceServer not becoming healthy

**Symptoms**: An InferenceServer resource is created but stays in a non-Ready state. The Deployment controller cannot deploy models to it because the server is not healthy.

**Diagnostics**:
```bash
# Check InferenceServer status and conditions
kubectl get inferenceservers
kubectl describe inferenceserver <name>

# Check the underlying Kubernetes Deployment
kubectl get deployment triton-<inferenceserver-name>   # Triton backend
kubectl describe deployment triton-<inferenceserver-name>

# Check the model-sync logs (in the sandbox, model-sync runs as a DaemonSet)
kubectl logs -l app=model-sync
```

**Likely causes**:
- The backend type is not supported — the `Validation` condition reports `InvalidBackendType` ("unsupported backend type"); check the InferenceServer conditions and controller manager logs
- The inference server container image cannot be pulled
- The model-sync component cannot connect to S3 to download models (see [S3 errors](#s3--object-store-errors) below)
- Insufficient GPU resources on the node — check `kubectl describe node` for allocatable GPU count

---

## Model not loading (Deployment stuck in asset preparation)

**Symptoms**: A Deployment resource is created but its `AssetsPrepared` condition stays false, or it remains in the `DEPLOYMENT_STAGE_RESOURCE_ACQUISITION` stage indefinitely.

**Diagnostics**:
```bash
# Check Deployment status
kubectl get deployments.michelangelo.api
kubectl describe deployment.michelangelo.api <name>

# Check model-sync for download errors (in the sandbox, a DaemonSet)
kubectl logs -l app=model-sync

# Verify the model config ConfigMap was created
kubectl get configmap <inferenceserver-name>-model-config -o yaml   # in the InferenceServer's namespace
```

**Likely causes**:
- The model artifact is not at the expected S3 path — verify the registered model's `spec.deployableArtifactUri` matches what is actually in S3
- S3 credentials in the inference pod do not have `s3:GetObject` permission on the model bucket
- The inference server has reached its maximum number of loaded models — check the serving framework's capacity limits

---

## S3 / object store errors

**Symptoms**: Jobs fail with access denied or endpoint unreachable errors. Model downloads fail in model-sync.

**Diagnostics**:
```bash
# Check controller manager storage config
kubectl -n <release-namespace> get configmap michelangelo-controllermgr-config -o yaml \
  | grep -A6 "minio:"

# Test S3 access from a worker pod
kubectl -n <release-namespace> exec deployment/michelangelo-worker -- \
  aws s3 ls s3://your-bucket/ --endpoint-url http://your-minio-endpoint

# Check for IAM role annotation on the relevant ServiceAccount
kubectl -n <release-namespace> get serviceaccount michelangelo -o yaml \
  | grep -i iam
```

**Likely causes**:
- The object storage credentials are missing or wrong — the chart injects `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` from the object-storage Secret (`objectStorage.existingSecret`), so check that the Secret exists and holds both keys. If you rely on IAM roles instead, check that the pod's ServiceAccount (`serviceAccount.annotations`) carries the role annotation
- `awsEndpointUrl` is missing the URL scheme (`http://` or `https://`) or has the wrong port
- The S3 bucket does not exist or is in a different region than `awsRegion` specifies
- Pod-level network policy is blocking outbound traffic to the S3 endpoint

---

## UI not loading or API calls failing

**Symptoms**: The Michelangelo AI UI shows a blank page, a CORS error in the browser console, or API calls return 502/504.

**Diagnostics**:
```bash
# Check Envoy and UI pod status
kubectl -n <release-namespace> get pods | grep -E "envoy|ui|apiserver"
kubectl -n <release-namespace> logs deployment/michelangelo-ui
kubectl -n <release-namespace> logs deployment/michelangelo-envoy

# Check Envoy configuration
kubectl -n <release-namespace> get configmap michelangelo-envoy-config -o yaml
```

**Likely causes**:
- `apiBaseUrl` in the UI's `config.json` (`ui.apiBaseUrl`) does not match the actual Envoy ingress hostname — they must match exactly
- The Envoy cluster's `socket_address.address` for `michelangelo-apiserver` is wrong — it must be the Kubernetes service name for the API server within the cluster
- `envoy.corsOrigins` (a regular expression) does not match the origin from which users are accessing the UI
- The Ingress resource for the UI or API server is misconfigured (wrong hostname, missing TLS secret)

## What's Next

- **Monitoring**: Set up proactive alerting so issues surface before users report them in the [Monitoring guide](./monitoring.md)
- **Network & Ingress**: Resolve Ingress and CORS issues at the source with the [Network guide](../setup/network.md)
- **Authentication**: Fix RBAC and OIDC configuration issues with the [Authentication guide](../setup/authentication.md)

---

## Pipeline stuck in Terminating after cascade delete

**Symptoms**: A Pipeline has a `deletionTimestamp` set but does not disappear. Under `foreground` propagation (the `ma pipeline delete` default) it remains in `Terminating` state because one or more child runs are still draining.

**Diagnostics**:
```bash
# Check Pipeline finalizers and deletion timestamp
kubectl get pipeline <name> -n <namespace> -o jsonpath='{.metadata.finalizers}'
kubectl get pipeline <name> -n <namespace> -o jsonpath='{.metadata.deletionTimestamp}'

# List child PipelineRuns (and TriggerRuns — same query) that still have drain
# finalizers, with each child's own deletionTimestamp (the per-child 24h clock)
kubectl get pipelineruns -n <namespace> -o json | \
  jq '.items[] | select(.metadata.ownerReferences[]?.uid == "<pipeline-uid>") | {name: .metadata.name, deletionTimestamp: .metadata.deletionTimestamp, finalizers: .metadata.finalizers}'

# Check controller manager logs for cascade-related messages
kubectl -n <release-namespace> logs deployment/michelangelo-controllermgr | grep -i "cascade\|drain\|force"
```

The safety timeout is **per child**, keyed off each child's own `deletionTimestamp` (above) — there is no Pipeline-level cascade annotation or clock. Watch the per-kind drain metrics to see which kind is wedged: `cascade_child_drain_active{kind="pipeline_run"|"trigger_run"}` (currently draining) and `cascade_child_drain_timeout_total{kind=...}` (drains that blew past the 24h backstop).

**Likely causes**:
- Child PipelineRuns or TriggerRuns still have drain finalizers — their workflows have not finished cancelling. Wait for drains to complete (or up to 24 hours after each child's `deletionTimestamp` for that child's safety timeout to force-remove its finalizer).
- The controller manager is not running — the drain controllers need to be running to process child drain finalizers. Ensure the controller manager is healthy.
- The workflow engine (Cadence/Temporal) is unreachable — the drain controller cannot cancel the workflow. Check workflow engine connectivity.

---

## Child resources not deleted after Pipeline cascade delete

**Symptoms**: The Pipeline is gone, but PipelineRuns or TriggerRuns that belonged to it remain.

**Diagnostics** — check whether the orphaned children carry an ownerReference to the Pipeline:
```bash
kubectl get pipelineruns -n <namespace> -o json | \
  jq '.items[] | select(.metadata.ownerReferences[]?.name == "<pipeline-name>") | .metadata.name'
```

**Likely cause**: the Pipeline was deleted with a non-cascading [propagation policy](../cascade-delete.md#controlling-cascade-behavior) (`--cascade=orphan`/`background`), or the children predate the ownerReference backfill (lazy, applied on each child's next reconcile).

**Remedy**: re-run with foreground cascade — or, if the Pipeline is already gone, delete the runs directly (`ma pipeline_run delete` / `kubectl delete pipelinerun …`), which drains and removes them individually:
```bash
kubectl delete pipeline <name> -n <namespace> --cascade=foreground
```
