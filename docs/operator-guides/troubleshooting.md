# Troubleshooting

This guide is for platform operators diagnosing issues with a Michelangelo deployment. All `kubectl` commands assume access to the control plane cluster.

---

## Jobs not being scheduled

**Symptoms**: Jobs are submitted but remain in a pending state with no cluster assignment.

**Diagnostics**:
```bash
# Check for scheduler errors in the controller manager
kubectl -n ma-system logs deployment/michelangelo-controllermgr | grep -i "scheduler\|assign\|enqueue"

# List registered compute clusters and their status
kubectl -n ma-system get clusters

# Inspect a specific cluster's status conditions
kubectl -n ma-system describe cluster <cluster-name>
```

**Likely causes**:
- No compute clusters are registered — complete the [cluster registration steps](jobs/register-a-compute-cluster-to-michelangelo-control-plane.md)
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
kubectl -n ma-system run connectivity-test --rm -it --restart=Never \
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
kubectl -n ma-system logs deployment/michelangelo-worker --tail=100

# Verify the worker's configured API server address
kubectl -n ma-system get configmap michelangelo-worker-config -o yaml | grep -A3 "worker:"

# Confirm the API server deployment is running
kubectl -n ma-system get deployment michelangelo-apiserver
kubectl -n ma-system get pods -l app=michelangelo-apiserver
```

**Likely causes**:
- `worker.address` in the worker ConfigMap points to the wrong hostname or port — it must resolve to the API server from within the `ma-system` namespace
- `worker.useTLS: true` is set but the API server's certificate is not trusted — ensure the CA bundle is mounted into the worker pod
- The API server is not yet ready (check its pod status and readiness probe)

---

## Temporal / Cadence connectivity issues

**Symptoms**: Workflows fail to start. Worker logs contain errors like `failed to connect to temporal`, `context deadline exceeded`, or `domain not found`.

**Diagnostics**:
```bash
# Check worker logs for workflow engine errors
kubectl -n ma-system logs deployment/michelangelo-worker | grep -i "temporal\|cadence\|workflow"

# Inspect the configured workflow engine endpoint
kubectl -n ma-system get configmap michelangelo-worker-config -o yaml \
  | grep -A8 "workflow-engine:"

# Test TCP connectivity to Temporal from a worker pod
kubectl -n ma-system exec deployment/michelangelo-worker -- \
  nc -zv temporal.your-domain.com 7233
```

**Likely causes**:
- `workflow-engine.host` has the wrong hostname or port (Temporal default is `7233`)
- The Temporal domain (`uniflow`, `default`) has not been created — create it with the Temporal CLI or admin tools
- Network policy in `ma-system` is blocking egress to the Temporal endpoint

---

## InferenceServer not becoming healthy

**Symptoms**: An InferenceServer resource is created but stays in a non-Ready state. The Deployment controller cannot deploy models to it because the server is not healthy.

**Diagnostics**:
```bash
# Check InferenceServer status and conditions
kubectl get inferenceservers
kubectl describe inferenceserver <name>

# Check the underlying Kubernetes Deployment
kubectl get deployment -l app=<inferenceserver-name>
kubectl describe deployment <inferenceserver-deployment>

# Check model-sync sidecar logs
kubectl logs <inferenceserver-pod-name> -c model-sync
```

**Likely causes**:
- The backend type is not registered in the controller manager — check controller manager logs for `unknown backend type`
- The inference server container image cannot be pulled
- The model-sync sidecar cannot connect to S3 to download models (see [S3 errors](#s3--object-store-errors) below)
- Insufficient GPU resources on the node — check `kubectl describe node` for allocatable GPU count

---

## Model not loading (Deployment stuck in Asset Preparation)

**Symptoms**: A Deployment resource is created but remains in the `AssetPreparation` or `ResourceAcquisition` stage indefinitely.

**Diagnostics**:
```bash
# Check Deployment status
kubectl get deployments.michelangelo.api
kubectl describe deployment.michelangelo.api <name>

# Check model-sync sidecar for download errors
kubectl logs <inferenceserver-pod> -c model-sync

# Verify the model config ConfigMap was created
kubectl get configmap <inferenceserver-name>-model-config -o yaml
```

**Likely causes**:
- The model artifact is not at the expected S3 path — verify the registered model's `artifactUri` matches what is actually in S3
- S3 credentials in the inference pod do not have `s3:GetObject` permission on the model bucket
- The inference server has reached its maximum number of loaded models — check the serving framework's capacity limits

---

## S3 / object store errors

**Symptoms**: Jobs fail with access denied or endpoint unreachable errors. Model downloads fail in the model-sync sidecar.

**Diagnostics**:
```bash
# Check controller manager storage config
kubectl -n ma-system get configmap michelangelo-controllermgr-config -o yaml \
  | grep -A5 "minio:"

# Test S3 access from a worker pod
kubectl -n ma-system exec deployment/michelangelo-worker -- \
  aws s3 ls s3://your-bucket/ --endpoint-url http://your-minio-endpoint

# Check for IAM role annotation on the relevant ServiceAccount
kubectl -n ma-system get serviceaccount michelangelo-controllermgr -o yaml \
  | grep -i iam
```

**Likely causes**:
- `useIam: true` is set but the pod's ServiceAccount does not have an IAM role annotation, so no credentials are injected
- `awsEndpointUrl` is missing the URL scheme (`http://` or `https://`) or has the wrong port
- The S3 bucket does not exist or is in a different region than `awsRegion` specifies
- Pod-level network policy is blocking outbound traffic to the S3 endpoint

---

## UI not loading or API calls failing

**Symptoms**: The Michelangelo UI shows a blank page, a CORS error in the browser console, or API calls return 502/504.

**Diagnostics**:
```bash
# Check Envoy and UI pod status
kubectl get pods | grep -E "envoy|ui|apiserver"
kubectl logs deployment/michelangelo-ui

# Check Envoy configuration
kubectl get configmap envoy-config -o yaml
```

**Likely causes**:
- `apiBaseUrl` in the UI's `config.json` does not match the actual Envoy ingress hostname — they must match exactly
- The Envoy cluster's `socket_address.address` for `michelangelo-apiserver` is wrong — it must be the Kubernetes service name for the API server within the cluster
- CORS allowed origins in the Envoy config do not include the origin from which users are accessing the UI
- The Ingress resource for the UI or API server is misconfigured (wrong hostname, missing TLS secret)

## What's next?

- **Monitoring**: Set up proactive alerting so issues surface before users report them in the [Monitoring guide](./monitoring.md)
- **Network & Ingress**: Resolve Ingress and CORS issues at the source with the [Network guide](./network.md)
- **Authentication**: Fix RBAC and OIDC configuration issues with the [Authentication guide](./authentication.md)
