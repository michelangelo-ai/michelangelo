---
sidebar_position: 5
---

# Deploy a Model

Serve a trained model from Michelangelo so applications can call it for predictions.

By the end, you'll have a model running on an inference server and a working `curl` request to prove it. This guide assumes your platform operator has already set up the cluster and serving infrastructure — see [Next Steps](#next-steps) for the operator-facing setup docs.

:::note
Studio's **Deploy & Predict** phase is on the roadmap. While the UI is in development, the `ma` CLI is the supported way to deploy. Everything you create here — `InferenceServer` and `Deployment` resources — will appear in Studio once the phase ships.

If you arrived from the Studio Deploy & Predict card, you're in the right place — use the CLI flow below until the UI ships.
:::

## What You'll Learn

- How a deployment ties a registered model to an inference server
- How to define an `InferenceServer` and a `Deployment` in YAML
- How to apply both with the `ma` CLI
- How to send a prediction request to verify the model is serving
- How to try the full flow on the local sandbox in a few minutes

## Prerequisites

Before deploying, you need:

- **A packaged model.** Your trained model must be packaged as a Triton-compatible artifact and uploaded to model storage (the `deploy-models` bucket on the sandbox, or your platform's configured object store). See the [Model Registry Guide](./model-registry-guide.md) for how to produce a deployable package.
- **A registered model revision.** Deployments target a specific `Revision` of a `Model`. See [Register a Revision](./model-registry-guide.md#register-a-revision) for how to create one. To list available revisions in your namespace, run `ma revision get -n <your-namespace>`.
- **Access to a cluster with serving installed.** Either a local sandbox (covered below) or a remote cluster set up by your platform operator.
- **The `ma` CLI on your PATH.** Clone the repository, then from the `python/` directory run `poetry install && source .venv/bin/activate`. Once the virtualenv is active, `ma` works from any directory. See the [CLI Reference](./cli.md) for details.

## The Two Resources You Need

A working deployment requires two Michelangelo resources:

| Resource | Purpose | Who Creates It |
|----------|---------|----------------|
| **`InferenceServer`** | A long-lived runtime that hosts one or more models. Defines the backend (Triton, LLM-D, etc.), CPU/memory/GPU, and how many replicas to run. | Often shared across a team — created once and reused for many models |
| **`Deployment`** | A model-to-server binding. Points a specific model `Revision` at an `InferenceServer` and controls rollout strategy. | Created per model (and updated per new revision) |

If a suitable `InferenceServer` already exists in your project, you only need to create a `Deployment`. Run `ma inference_server get -n <your-namespace>` to find it — ask your platform operator if you're unsure.

## Try It on the Sandbox

The fastest way to see a working deployment end-to-end is the built-in sandbox demo. From a clean sandbox:

```bash
ma sandbox create
ma sandbox demo inference
```

`ma sandbox demo inference` provisions an `InferenceServer` named `inference-server-example` in the `default` namespace, plus the model config and gateway routes needed to serve predictions. The port-forward to `localhost:8080` is active for the lifetime of that terminal session. If you used `--workflow temporal` when creating the sandbox, that port may collide with the Temporal Web UI — use a separate terminal for inference testing.

From there you can apply your own `Deployment` against the provisioned server and start sending inference requests.

:::note
If you're using the sandbox, `ma sandbox demo inference` already created an `InferenceServer` for you. Skip Step 1 and start at Step 2 using `inferenceServer.name: inference-server-example` and `inferenceServer.namespace: default`.
:::

## Step 1: Define an InferenceServer

Create `inferenceserver.yaml`:

:::note
If you set up Michelangelo on your own cluster, you are also the operator. The `clusterId`, `tokenTag`, and `caDataTag` values are the identifiers you assigned when registering the cluster with the control plane — see [the cluster setup guide](../operator-guides/serving/cluster-setup.md) for details.
:::

```yaml
apiVersion: michelangelo.api/v2
kind: InferenceServer
metadata:
  name: my-inference-server
  namespace: my-project
  labels:
    app: my-inference-server
spec:
  backendType: BACKEND_TYPE_TRITON
  initSpec:
    resourceSpec:
      cpu: 2
      memory: "4Gi"
    numInstances: 1
  owner:
    name: "<your-username>"  # your username as configured by your platform operator (used for auditing)
  clusterTargets:
  - clusterId: <cluster-id>  # cluster identifier provided by your platform operator
    kubernetes:
      host: https://kubernetes.default.svc
      port: "443"
      tokenTag: <token-secret-tag>
      caDataTag: <ca-data-secret-tag>
```

Key fields:

- **`backendType`** — required. The serving framework. `BACKEND_TYPE_TRITON` is the most common choice for general ML models. Ask your platform operator about the others: `BACKEND_TYPE_LLM_D` and `BACKEND_TYPE_DYNAMO` are optimized for LLM workloads; `BACKEND_TYPE_TORCHSERVE` is PyTorch-specific.
- **`initSpec.resourceSpec`** — CPU and memory per replica. A lightweight model typically needs 2 CPU / 4 Gi; a large deep learning model may need a GPU node — check with your platform operator if you're unsure.
- **`initSpec.numInstances`** — how many replicas to run for availability and throughput.
- **`owner.name`** — your username as configured by your platform operator, used for audit purposes. For multi-tenant deployments, your platform operator will configure `ownerSpec` (team identifiers, groups, and tier) separately — ask them if you need to set ownership for capacity attribution.
- **`clusterTargets`** — required. Specifies which cluster(s) the server is provisioned on. The `clusterId` and secret tags come from your platform operator. See the [Michelangelo Serving overview](../operator-guides/serving/index.md) for how operators configure cluster targets.

## Step 2: Define a Deployment

Create `deployment.yaml`:

```yaml
apiVersion: michelangelo.api/v2
kind: Deployment
metadata:
  name: my-model-deployment
  namespace: my-project
spec:
  inferenceServer:
    name: my-inference-server
    namespace: my-project
  desiredRevision:
    name: <your-revision-name>
    namespace: my-project
  strategy:
    blast: {}
```

Key fields:

- **`inferenceServer`** — the `InferenceServer` to load the model on. Must already exist.
- **`desiredRevision`** — the model `Revision` you want to serve. Update this field and re-apply to roll out a new model version.
- **`strategy`** — required. Controls how the rollout proceeds across server instances. `blast: {}` loads the new model on all instances simultaneously (simplest option). Other strategies — `zonal`, `rolling`, and `red_black` — let you stage rollouts progressively. See the [`Deployment` reference in the operator serving guide](../operator-guides/serving/index.md) for strategy details.

:::tip
`ma apply` sends resources to the Michelangelo API server, which manages the deployment lifecycle. This is different from `kubectl apply`, which writes directly to Kubernetes.
:::

## Step 3: Apply Both Resources

Apply each resource with `ma` — `apply` creates the resource if it doesn't exist, or updates it if it does.

```bash
ma inference_server apply -f inferenceserver.yaml
ma deployment apply -f deployment.yaml
```

Check status:

```bash
ma inference_server get -n <your-namespace> --name my-inference-server
ma deployment get -n <your-namespace> --name my-model-deployment
```

A healthy deployment progresses through these stages:

`Validation → Placement → Resource Acquisition → Rollout Complete`

The [Deployment Lifecycle section in the serving overview](../operator-guides/serving/index.md#deployment-lifecycle) explains each stage.

## Step 4: Send a Prediction Request

Once the deployment reaches `Rollout Complete`, the model is reachable through the gateway. The path follows this pattern:

```text
http://<gateway-host>/<inference-server-name>/<deployment-name>/infer
```

On the sandbox, `ma sandbox demo inference` port-forwards the gateway to `localhost:8080` for the lifetime of the terminal session. Send a request with the input shape your model expects:

```bash
curl -X POST http://localhost:8080/my-inference-server/my-model-deployment/infer \
  -H "Content-Type: application/json" \
  -d '{
    "inputs": [
      {
        "name": "input_ids",
        "shape": [1, 10],
        "datatype": "INT64",
        "data": [101, 7592, 999, 102, 0, 0, 0, 0, 0, 0]
      }
    ]
  }'
```

Replace the input names, shape, and data with whatever matches the `ModelSchema` you defined when packaging the model.

**A `200 OK` response with the predicted outputs confirms the model is serving.**

:::tip
The request and response payloads follow the [KServe v2 inference protocol](https://kserve.github.io/website/latest/modelserving/inference_api/). Each input/output corresponds to a `ModelSchemaItem` in your packaged model.
:::

## Manage Your Deployment

### Updating a Deployment

To roll out a new model version, update `desiredRevision.name` in `deployment.yaml` and re-apply:

```bash
ma deployment apply -f deployment.yaml
```

The Deployment controller handles the rollout, including rollback if the new revision fails health checks.

### Deleting a Deployment

```bash
ma deployment delete -n <your-namespace> --name my-model-deployment
```

Deleting the `Deployment` removes the model from the inference server but leaves the `InferenceServer` and the underlying model artifacts intact. To tear down the server too:

```bash
ma inference_server delete -n <your-namespace> --name my-inference-server
```

:::warning
Delete all `Deployment` resources that reference an `InferenceServer` before deleting the server itself. Deleting an `InferenceServer` while active Deployments still reference it leaves those Deployments in a stuck state.
:::

## Troubleshooting

| Symptom | Likely cause | Fix |
|---------|-------------|-----|
| Deployment stuck in `Validation` | The model `Revision` or target `InferenceServer` couldn't be resolved | Confirm both exist with `ma revision get` and `ma inference_server get`; check that namespace fields in the Deployment spec match |
| Reaches `Rollout Complete` but predictions return 404 | Gateway route not ready, or URL path is incorrect | Wait a moment and retry; confirm the path is `/<inference-server-name>/<deployment-name>/infer` (case-sensitive, uses metadata `name` fields) |
| Predictions return a schema validation error | Request payload doesn't match the model's `ModelSchema` | Re-check input names, shapes, and dtypes against the schema you defined when packaging. See [Schema validation errors in the Model Registry guide](./model-registry-guide.md#schema-validation-errors) |
| Model artifact not found | The packaged model can't be located in storage | Verify the artifact was uploaded to the configured bucket (`deploy-models` on the sandbox) and that the `Revision` references the correct path |

## Next Steps

This guide covers the end-user workflow assuming serving infrastructure is already in place. For platform-level concerns:

- **[Michelangelo Serving overview](../operator-guides/serving/index.md)** — architecture, controller lifecycles, and core concepts
- **[Model Registry Guide](./model-registry-guide.md)** — package and register a model before deploying
- **[Integrate with a Custom Backend](../operator-guides/serving/integrate-custom-backend.md)** — add support for new serving frameworks
- **[CLI Reference](./cli.md)** — every `ma` command, including the full list of supported flags
