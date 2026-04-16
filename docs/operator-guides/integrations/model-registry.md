# Model Registry Integration

Unlike experiment tracking — where operators connect an external server — Michelangelo includes a built-in model registry. This guide explains how operators verify the registry is healthy, configure storage and access, and integrate registered models with downstream serving and CI/CD systems.

---

## How the Model Registry Works

The registry separates operator and user responsibilities cleanly:

```
┌──────────────────────────────────────────────────────────┐
│ Operator Responsibility                                  │
│ ├─ Provision the object store bucket and IAM policy      │
│ ├─ Verify the Model CRD is installed                     │
│ └─ Configure RBAC for namespace access                   │
└──────────────────────────────────────────────────────────┘
                          ↓
┌──────────────────────────────────────────────────────────┐
│ User Responsibility (task code)                          │
│ ├─ Register models from inside @uniflow.task functions   │
│ └─ Platform creates a Model CR and writes artifacts      │
└──────────────────────────────────────────────────────────┘
                          ↓
┌──────────────────────────────────────────────────────────┐
│ Downstream Consumers                                     │
│ ├─ InferenceServer (Michelangelo serving layer)          │
│ ├─ External serving infrastructure (reads from S3)       │
│ └─ CI/CD pipelines (read artifact URIs from Model CRs)   │
└──────────────────────────────────────────────────────────┘
```

The registry is backed by a single Kubernetes Custom Resource: `Model` (CRD name `models.michelangelo.api`).

---

## Prerequisites

Before working through this guide, ensure you have completed:

- **[Platform Setup](../platform-setup.md#object-store-configuration)** — the Controller Manager's `minio.*` fields must point to a reachable S3-compatible object store.
- **Compute cluster registration** — at least one compute cluster registered with the Michelangelo control plane, so Uniflow tasks have somewhere to run.
- Sufficient cluster permissions to create Roles and RoleBindings, and to inspect Custom Resource Definitions.

---

## Step 1: Verify the Model CRD Is Installed

Confirm the `Model` CRD is present in the cluster before expecting any registration to succeed:

```bash
kubectl get crd models.michelangelo.api
```

If the CRD is missing, re-run the Michelangelo CRD installation step described in [Platform Setup](../platform-setup.md).

You can also spot-check a namespace for any existing models:

```bash
kubectl get models -n <namespace>
```

---

## Step 2: Configure S3 Permissions for Model Artifacts

The Controller Manager and task pods write model artifacts to your S3-compatible object store. The IAM role or service account bound to the Controller Manager needs the following permissions on the models bucket:

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
    "arn:aws:s3:::<your-models-bucket>",
    "arn:aws:s3:::<your-models-bucket>/*"
  ]
}
```

Task pods produce the raw model files during registration. If task pods run under a different IAM role or service account than the Controller Manager, apply equivalent write permissions to that identity as well.

### Artifact URI discovery

The exact S3 layout for a model's artifacts is set by your platform configuration — Michelangelo does not prescribe a fixed directory structure. Rather than hardcoding paths, read the actual locations from each `Model` resource after registration:

```bash
# Raw training artifact URIs (weights, checkpoints)
kubectl get model <model-name> -n <namespace> \
  -o jsonpath='{.spec.model_artifact_uri}'

# Deployable artifact URIs (packaged for serving)
kubectl get model <model-name> -n <namespace> \
  -o jsonpath='{.spec.deployable_artifact_uri}'
```

These spec fields are the authoritative source for artifact location. Use them in any automation that needs to consume artifacts.

---

## Step 3: Configure RBAC

Grant teams read access to `Model` resources in their namespace:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: model-registry-reader
  namespace: <namespace>
rules:
  - apiGroups: ["michelangelo.api"]
    resources: ["models"]
    verbs: ["get", "list", "watch"]
```

For CI/CD service accounts that need to inspect or forward model records:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: ci-model-registry-reader
  namespace: <namespace>
subjects:
  - kind: ServiceAccount
    name: ci-service-account
    namespace: ci-namespace
roleRef:
  kind: Role
  name: model-registry-reader
  apiGroup: rbac.authorization.k8s.io
```

For cluster-wide patterns and multi-tenant isolation, see [Authentication and RBAC](../authentication.md).

---

## Verification

After completing the setup steps, run this smoke test to confirm the registry is operational.

**1. Check the CRD is registered:**

```bash
kubectl get crd models.michelangelo.api
```

**2. List Model resources in a namespace** (requires at least one registered model):

```bash
# Using kubectl
kubectl get models -n <namespace>

# Using the ma CLI
ma model get -n <namespace>
```

**3. Inspect a specific model:**

```bash
kubectl describe model <model-name> -n <namespace>
```

**4. Verify object store reachability from a compute pod:**

```bash
kubectl run storage-check \
  --image=amazon/aws-cli \
  --namespace=<compute-namespace> \
  --restart=Never \
  --rm -it -- \
  s3 ls s3://<your-models-bucket>/
```

If any of these fail, see the [Troubleshooting](#troubleshooting) section below.

---

## The Model Custom Resource

Every registered model is a `Model` resource. Here is a representative example:

```yaml
apiVersion: michelangelo.api/v2
kind: Model
metadata:
  name: fraud-detector
  namespace: ml-team
  labels:
    algorithm: xgboost
spec:
  owner:
    name: <owner-username>
  description: "Fraud detection model trained on transaction features"
  algorithm: xgboost
  trainingFramework: sklearn
  kind: MODEL_KIND_BINARY_CLASSIFICATION
  source: TRAINING
  package_type: DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON
  revision_id: 3
  model_artifact_uri:
    - s3://<your-bucket>/<path-to-raw-weights>
  deployable_artifact_uri:
    - s3://<your-bucket>/<path-to-serving-package>
  input_schema:
    schema_items:
      - name: transaction_features
        data_type: DATA_TYPE_FLOAT
  output_schema:
    schema_items:
      - name: fraud_score
        data_type: DATA_TYPE_DOUBLE
```

Key fields:

| Field | Notes |
|---|---|
| `kind` | `Model` — there is no `ModelVersion` resource |
| `spec.kind` | ML problem type (e.g. `MODEL_KIND_REGRESSION`, `MODEL_KIND_BINARY_CLASSIFICATION`) |
| `spec.package_type` | How serving systems should interpret the deployable artifact (e.g. `DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON`) |
| `spec.revision_id` | Integer version counter; users set this when creating the resource |
| `spec.model_artifact_uri[]` | Repeated string — URIs to raw training artifacts |
| `spec.deployable_artifact_uri[]` | Repeated string — URIs to packaged artifacts ready for serving |
| `spec.input_schema` / `spec.output_schema` | `DataSchema` with `schema_items[]`; each item has `name` and `data_type` (not `dtype`) |
| `status` | Empty — the Model resource carries no status conditions, phase, or timestamps |

> **Heads-up:** `ModelStatus` is intentionally empty. Do not poll `kubectl wait --for=condition=Ready` on a `Model` — no such condition exists. If you need a readiness signal, key off the existence of the resource (and a non-empty `spec.deployable_artifact_uri[]`) or wait on a downstream resource such as an `InferenceServer`.

---

## Querying the Registry

Use either `kubectl` or the `ma` CLI to inspect registered models.

### kubectl

```bash
# List all models in a namespace
kubectl get models -n <namespace>

# Describe a specific model
kubectl describe model <model-name> -n <namespace>

# Read a single field for automation
kubectl get model <model-name> -n <namespace> \
  -o jsonpath='{.spec.deployable_artifact_uri[0]}'
```

### ma CLI

The `ma model` subcommand supports `get`, `apply`, and `delete`. To list all models in a namespace, omit `--name`:

```bash
# List models in a namespace
ma model get -n <namespace>

# Get a specific model by name
ma model get -n <namespace> --name <model-name>

# Limit results when listing
ma model get -n <namespace> --limit 20
```

`ma model` does not have a `list` subcommand or `--version` / `--output` flags. When you need structured output for scripting, use `kubectl` with `-o jsonpath` or `-o json`.

---

## Integrating with the Serving Layer

Michelangelo's `InferenceServer` resource does not reference a `Model` resource by name in its spec. The wiring from a registered model to a running server flows through `Deployment` and `Revision` resources managed by the Controller Manager, which update a `modelconfig` ConfigMap consumed by the inference backend.

A representative `InferenceServer` manifest:

```yaml
apiVersion: michelangelo.api/v2
kind: InferenceServer
metadata:
  name: fraud-detector-server
  namespace: ml-team
spec:
  tenancyType: TENANCY_TYPE_DEDICATED
  backendType: BACKEND_TYPE_TRITON
  ownerSpec:
    tier: 1
  initSpec:
    resourceSpec:
      cpu: 2
      memory: "4Gi"
    servingSpec:
      version: "latest"
    numInstances: 1
  decomSpec:
    decommission: false
  owner:
    name: <owner-username>
```

Key fields:

| Field | Notes |
|---|---|
| `backendType` | Enum — `BACKEND_TYPE_TRITON`, `BACKEND_TYPE_LLM_D`, `BACKEND_TYPE_DYNAMO`, `BACKEND_TYPE_TORCHSERVE`. Not the lowercase string `"triton"`. |
| `initSpec.numInstances` | Instance count — there is no `replicas` field |
| `tenancyType` | `TENANCY_TYPE_DEDICATED` (one project per server) or `TENANCY_TYPE_MULTI_TENANT` |
| `spec.modelVersion` | Does not exist — do not attempt to reference a Model directly from InferenceServer |

The `InferenceServer` controller emits these conditions: `Cleanup`, `HealthCheck`, `BackendProvision`, `ModelConfigProvision`, `Validation`. There is no `Ready` condition; gate readiness on `BackendProvision` and `ModelConfigProvision` instead.

For backend selection and configuration, see [Integrate a Custom Backend](../serving/integrate-custom-backend.md).

---

## CI/CD Pipeline Integration

Because `Model` resources have no status conditions, CI/CD pipelines should check for the presence of the resource and read artifact URIs directly from the spec — `kubectl wait` is not applicable.

### Example: GitHub Actions step

```yaml
- name: Check model is registered
  run: |
    kubectl get model "${{ env.MODEL_NAME }}" \
      --namespace "${{ env.NAMESPACE }}"

- name: Get artifact URI
  id: model
  run: |
    ARTIFACT_URI=$(kubectl get model "${{ env.MODEL_NAME }}" \
      -n "${{ env.NAMESPACE }}" \
      -o jsonpath='{.spec.deployable_artifact_uri[0]}')
    echo "deployable_uri=$ARTIFACT_URI" >> "$GITHUB_OUTPUT"

- name: Forward artifact to serving infrastructure
  run: |
    your-serving-tool deploy \
      --artifact "${{ steps.model.outputs.deployable_uri }}" \
      --target production
```

The variable is named `ARTIFACT_URI`, not `PATH` — assigning to `PATH` would overwrite the shell's executable search path and break every subsequent command in the step.

### Portable date math for retention scripts

`date -d` is GNU coreutils only and fails on macOS / BSD. Use one of the following forms depending on where the script runs:

```bash
# GNU/Linux
CUTOFF=$(date -d '90 days ago' -u +%Y-%m-%dT%H:%M:%SZ)

# macOS / BSD
CUTOFF=$(date -u -v-90d +%Y-%m-%dT%H:%M:%SZ)

# Cross-platform (Python)
CUTOFF=$(python3 -c "from datetime import datetime, timedelta, timezone; print((datetime.now(timezone.utc) - timedelta(days=90)).strftime('%Y-%m-%dT%H:%M:%SZ'))")
```

---

## Retention and Cleanup

Model artifacts in S3 are not automatically removed when a `Model` resource is deleted. Manage artifact lifecycle at the object store level using S3 lifecycle policies, or implement a periodic cleanup job that:

1. Lists `Model` resources older than your retention window. Use `metadata.creationTimestamp` (a real Kubernetes field) as the time signal — there is no `status.registeredAt`:

   ```bash
   kubectl get models -A -o json \
     | jq --arg cutoff "$CUTOFF" \
       '.items[]
        | select(.metadata.creationTimestamp < $cutoff)
        | {namespace: .metadata.namespace,
           name: .metadata.name,
           model_uris: .spec.model_artifact_uri,
           deployable_uris: .spec.deployable_artifact_uri}'
   ```

2. Reads `spec.model_artifact_uri[]` and `spec.deployable_artifact_uri[]` from each result to identify the S3 paths.
3. Deletes the S3 objects first, then deletes the `Model` CR — that ordering avoids orphaned references if cleanup is interrupted.

---

## Troubleshooting

| Symptom | Likely cause | Resolution |
|---|---|---|
| `error: the server doesn't have a resource type "models"` | CRD not installed | Re-run the Michelangelo CRD installation step (see [Platform Setup](../platform-setup.md)) |
| `kubectl get models` returns `No resources found` but CRD is present | No models registered yet, or wrong namespace | Confirm a registration task has run; check the namespace |
| `spec.model_artifact_uri` empty after registration | Controller Manager lacks S3 write permissions, or the registration task failed | Check Controller Manager logs; verify IAM policy on the bucket |
| `spec.deployable_artifact_uri` empty | Packaging step did not run or failed | Inspect the pipeline run logs for the registration task |
| RBAC error reading models (`User ... cannot get resource "models"`) | Role missing the `michelangelo.api` API group | Use `apiGroups: ["michelangelo.api"]` (not `[""]`) and apply the manifest from [Step 3](#step-3-configure-rbac) |
| `kubectl wait --for=condition=Ready` hangs on a Model | Model has no status conditions | Don't gate on Model conditions; use `spec.deployable_artifact_uri[0]` non-empty as the readiness signal, or wait on a downstream `InferenceServer` |
| `InferenceServer` does not start serving | `backendType` set to lowercase string `"triton"` instead of enum value | Use `backendType: BACKEND_TYPE_TRITON` |

For deeper diagnostic trees, see the [Troubleshooting Guide](../troubleshooting.md).

---

## What's Next

- [Integrate a Custom Backend](../serving/integrate-custom-backend.md) — configure Triton, vLLM, TensorRT-LLM, or a custom inference framework to serve registered models.
- [Authentication and RBAC](../authentication.md) — cluster-wide RBAC patterns and identity-provider setup.
- [Experiment Tracking Integration](experiment-tracking.md) — connect an external experiment tracking server to link training runs to the models they produce.
- [Object Store Configuration](../platform-setup.md#object-store-configuration) — review the full `minio.*` configuration reference.
