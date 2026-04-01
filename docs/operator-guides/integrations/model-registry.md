# Model Registry Integration

This guide explains how Michelangelo's model registry works from an operator perspective: what Kubernetes resources back it, how model artifacts are stored and versioned, and how downstream systems (serving infrastructure, CI/CD pipelines, external registries) consume registered models.

---

## Overview

Michelangelo includes a built-in model registry for versioning and tracking trained models produced by Uniflow pipelines. The registry is implemented as Kubernetes Custom Resources managed by the Controller Manager.

```
┌─────────────────────────────────────────────────────────────┐
│ Uniflow Task (@uniflow.task)                                │
│ └─ Registers model artifact → Michelangelo Model Registry  │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ Model Registry (Controller Manager)                         │
│ ├─ ModelVersion CRD (tracks metadata + artifact location)  │
│ └─ Writes artifact to S3-compatible object store           │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ Downstream Consumers                                        │
│ ├─ InferenceServer (Michelangelo serving)                  │
│ ├─ External serving infrastructure (reads from S3)         │
│ └─ CI/CD pipelines (promote / gate on model metadata)      │
└─────────────────────────────────────────────────────────────┘
```

---

## How Models Are Stored

### Object Store (S3 / MinIO)

All model artifacts are stored in the S3-compatible object store configured in the Controller Manager. See [Object Store Configuration](../platform-setup.md#object-store-configuration) for the `minio.*` fields.

Artifacts are written under a structured path:

```
s3://<bucket>/models/<namespace>/<model-name>/<version>/
├── raw/          # Original training output (weights, checkpoints)
└── deployable/   # Transformed for inference (e.g., Triton model repository layout)
```

Both formats are written for each registered version. The `raw` format is intended for fine-tuning or analysis workflows. The `deployable` format is what Michelangelo's InferenceServer (and compatible external serving systems) consume.

### ModelVersion Custom Resource

Each registered model version corresponds to a `ModelVersion` Custom Resource in Kubernetes:

```yaml
apiVersion: michelangelo.api/v2
kind: ModelVersion
metadata:
  name: fraud-detector-v3
  namespace: ml-team
spec:
  modelName: fraud-detector
  version: 3
  artifactPath: s3://models-bucket/models/ml-team/fraud-detector/3/
  schema:
    inputs:
      - name: transaction_features
        shape: [-1, 128]
        dtype: float32
    outputs:
      - name: fraud_score
        shape: [-1, 1]
        dtype: float32
status:
  phase: Ready
  rawArtifactPath: s3://models-bucket/models/ml-team/fraud-detector/3/raw/
  deployableArtifactPath: s3://models-bucket/models/ml-team/fraud-detector/3/deployable/
  registeredAt: "2026-03-15T09:12:00Z"
```

---

## Listing and Querying the Registry

Use `kubectl` to inspect registered models and versions:

```bash
# List all ModelVersions in a namespace
kubectl get modelversions -n ml-team

# Describe a specific version for full metadata and artifact paths
kubectl describe modelversion fraud-detector-v3 -n ml-team

# Get the artifact path for automation
kubectl get modelversion fraud-detector-v3 -n ml-team \
  -o jsonpath='{.status.deployableArtifactPath}'
```

The Michelangelo API server also exposes model registry operations over gRPC. If you are integrating with an internal tool or CI/CD pipeline, use the `ma` CLI:

```bash
# List models in a namespace
ma model list --namespace ml-team

# Get artifact location for a specific version
ma model get fraud-detector --version 3 --namespace ml-team
```

---

## Operator Configuration

### S3 Permissions for Model Artifacts

The Controller Manager writes model artifacts to S3. Ensure the IAM role or service account bound to the Controller Manager has the following permissions on the models bucket:

```json
{
  "Effect": "Allow",
  "Action": [
    "s3:PutObject",
    "s3:GetObject",
    "s3:DeleteObject",
    "s3:ListBucket"
  ],
  "Resource": [
    "arn:aws:s3:::models-bucket",
    "arn:aws:s3:::models-bucket/models/*"
  ]
}
```

Task pods (which produce the raw model files) also need write access to the same bucket path during the registration step. If task pods run under a different IAM role or service account, ensure they have equivalent permissions.

### Enabling the Model Registry

Model registry support is enabled by default when the Controller Manager is deployed. No additional configuration flag is required. To verify it is active, check that the `ModelVersion` CRD is installed:

```bash
kubectl get crd modelversions.michelangelo.api
```

If the CRD is missing, re-run the Michelangelo CRD installation step (see [Platform Setup](../platform-setup.md)).

---

## Integrating Downstream Systems

### Using Registered Models in Michelangelo's Serving Layer

Michelangelo's `InferenceServer` resource references a `ModelVersion` directly:

```yaml
apiVersion: michelangelo.api/v2
kind: InferenceServer
metadata:
  name: fraud-detector-serving
  namespace: ml-team
spec:
  modelVersion:
    name: fraud-detector-v3
    namespace: ml-team
  backend: triton
  replicas: 2
```

The Controller Manager resolves the artifact path from the `ModelVersion` status and mounts it into the serving container automatically.

### Consuming Artifacts from External Serving Infrastructure

If your organization uses a serving system outside of Michelangelo, it can read model artifacts directly from S3 using the artifact paths from the `ModelVersion` status.

Example: pulling the deployable artifact path and loading it into an external system:

```bash
ARTIFACT_PATH=$(kubectl get modelversion fraud-detector-v3 -n ml-team \
  -o jsonpath='{.status.deployableArtifactPath}')

# Pass the path to your external serving system's model loading command
your-serving-tool load-model \
  --source "$ARTIFACT_PATH" \
  --name fraud-detector \
  --version 3
```

Alternatively, access the same information via the API:

```bash
ma model get fraud-detector --version 3 --namespace ml-team --output json \
  | jq '.deployableArtifactPath'
```

### Integrating with an External Model Registry

Some organizations maintain a separate model registry for governance, compliance, or multi-platform visibility. You can sync Michelangelo model metadata to an external registry by:

1. **Listening for `ModelVersion` events** using a Kubernetes controller or a simple watch loop:

```bash
kubectl get modelversions -n ml-team --watch -o json \
  | jq 'select(.status.phase == "Ready")'
```

2. **Extracting the relevant fields** (name, version, artifact paths, schema) from the `ModelVersion` resource.

3. **Registering the entry in your external registry** using that system's API.

This pattern keeps Michelangelo as the authoritative source for artifact location, while your external registry holds the governance metadata (approval status, deployment lineage, access controls).

---

## CI/CD Pipeline Integration

### Gating Promotions on Model Version Status

A common CI/CD pattern is to wait for a model version to reach `Ready` status before promoting it to a production serving target. Use `kubectl wait`:

```bash
kubectl wait modelversion fraud-detector-v3 \
  --namespace ml-team \
  --for=condition=Ready \
  --timeout=600s
```

If this succeeds (exit code 0), the artifact is available at the path in `.status.deployableArtifactPath`. If it times out, treat the pipeline as failed.

### Example: GitHub Actions Step

```yaml
- name: Wait for model to be ready
  run: |
    kubectl wait modelversion ${{ env.MODEL_VERSION }} \
      --namespace ${{ env.NAMESPACE }} \
      --for=condition=Ready \
      --timeout=600s

- name: Get artifact path
  id: artifact
  run: |
    PATH=$(kubectl get modelversion ${{ env.MODEL_VERSION }} \
      -n ${{ env.NAMESPACE }} \
      -o jsonpath='{.status.deployableArtifactPath}')
    echo "path=$PATH" >> $GITHUB_OUTPUT

- name: Deploy to production serving
  run: |
    your-serving-tool deploy \
      --artifact ${{ steps.artifact.outputs.path }} \
      --target production
```

---

## RBAC for Model Registry Operations

Grant teams read access to `ModelVersion` resources in their namespace:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: model-registry-reader
  namespace: ml-team
rules:
  - apiGroups: ["michelangelo.api"]
    resources: ["modelversions"]
    verbs: ["get", "list", "watch"]
```

For CI/CD service accounts that need to gate on model readiness:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: ci-model-registry-reader
  namespace: ml-team
subjects:
  - kind: ServiceAccount
    name: ci-service-account
    namespace: ci-namespace
roleRef:
  kind: Role
  name: model-registry-reader
  apiGroup: rbac.authorization.k8s.io
```

---

## Retention and Cleanup

Model artifacts in S3 are not automatically deleted when a `ModelVersion` resource is removed. Implement a retention policy at the object store level (S3 lifecycle rules) or via a scheduled cleanup job that queries old `ModelVersion` resources and removes the corresponding S3 objects.

Example: list `ModelVersion` resources older than 90 days:

```bash
kubectl get modelversions -A -o json \
  | jq --arg cutoff "$(date -d '90 days ago' -u +%Y-%m-%dT%H:%M:%SZ)" \
    '.items[] | select(.status.registeredAt < $cutoff) | {name: .metadata.name, namespace: .metadata.namespace, path: .status.rawArtifactPath}'
```

---

## Related

- [Object Store Configuration](../platform-setup.md#object-store-configuration)
- [Experiment Tracking Integration](experiment-tracking.md)
- [Custom Serving Backend](../serving/integrate-custom-backend.md)
- [Authentication and RBAC](../authentication.md)
