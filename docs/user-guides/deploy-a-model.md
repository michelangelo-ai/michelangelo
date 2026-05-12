# Deploy a Model

Serve a trained model from Michelangelo so applications can call it for predictions.

This guide walks through the end-user deploy workflow: pointing a registered model at an inference server, applying the deployment, and confirming the model is serving requests. This guide assumes your platform team has already set up the cluster and serving infrastructure — see [Next Steps](#next-steps) for the operator-facing setup docs.

:::note
**Michelangelo Studio** is the browser-based UI for Michelangelo. Its **Deploy & Predict** phase will provide a UI for the workflow described in this guide — creating deployments, managing rollouts, and sending test predictions from the browser. This phase is currently in development; until it ships, use the CLI flow below. The same `Deployment` and `InferenceServer` resources you create with `ma` will appear in Studio once the UI is enabled.
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
- **A registered model revision.** Deployments target a specific `Revision` of a `Model`. The Model Registry guide covers registration. To list available revisions in your namespace, run `ma revision get -n <your-namespace>`.
- **Access to a cluster with serving installed.** Either a local sandbox (covered below) or a remote cluster set up by your operator team.
- **The `ma` CLI on your PATH.** From the `python/` directory, run `poetry install && source .venv/bin/activate`. See the [CLI Reference](./cli.md) for details. Run all `ma` commands below from the `python/` directory.

## Try It on the Sandbox

The fastest way to see a working deployment end-to-end is the built-in sandbox demo. From a clean sandbox:

```bash
ma sandbox create
ma sandbox demo inference
```

`ma sandbox demo inference` provisions an `InferenceServer` named `inference-server-example` in the `default` namespace, plus the model config and gateway routes needed to serve predictions. From there you can apply your own `Deployment` against it and start sending inference requests.

For the full local walkthrough — including uploading a model artifact and sending a sample request — see [Sandbox Setup](../getting-started/sandbox-setup.md).

## The Two Resources You Need

A working deployment requires two Michelangelo resources:

| Resource | Purpose | Who Creates It |
|----------|---------|----------------|
| **`InferenceServer`** | A long-lived runtime that hosts one or more models. Defines the backend (Triton, vLLM, etc.), CPU/memory/GPU, and how many replicas to run. | Often shared across a team — created once and reused for many models |
| **`Deployment`** | A model-to-server binding. Points a specific model `Revision` at an `InferenceServer` and controls rollout strategy. | Created per model (and updated per new revision) |

If a suitable `InferenceServer` already exists in your project, you only need to create a `Deployment`. Run `ma inference_server get -n <your-namespace>` to find it — ask your platform operator if you're unsure.

## Step 1: Define an InferenceServer (if one doesn't exist)

Create `inferenceserver.yaml`:

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
    name: "your-username"  # your Michelangelo username or team identifier
  clusterTargets:
  - clusterId: <cluster-id>  # cluster identifier provided by your platform team
    kubernetes:
      host: https://kubernetes.default.svc
      port: "443"
      tokenTag: <token-secret-tag>
      caDataTag: <ca-data-secret-tag>
```

Key fields:

- **`backendType`** — the serving framework. `BACKEND_TYPE_TRITON` is the default for most use cases. Other backends (`BACKEND_TYPE_LLM_D`, `BACKEND_TYPE_DYNAMO`, `BACKEND_TYPE_TORCHSERVE`) are available depending on your platform setup.
- **`initSpec.resourceSpec`** — CPU and memory per replica. Size based on your model's needs.
- **`initSpec.numInstances`** — how many replicas to run for availability and throughput.
- **`owner.name`** — your Michelangelo username or team identifier. Ask your platform operator if you don't know what value to use.
- **`clusterTargets`** — required. Specifies which cluster(s) the server is provisioned on. The `clusterId` and secret tags come from your platform team. See the [Michelangelo Serving overview](../operator-guides/serving/index.md) for how operators configure cluster targets.

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
```

Key fields:

- **`inferenceServer`** — the `InferenceServer` to load the model on. Must already exist.
- **`desiredRevision`** — the model `Revision` you want to serve. Update this field and re-apply to roll out a new model version.

For a more complete example with explicit rollout strategy and target type, see the [`Deployment` reference in the operator serving guide](../operator-guides/serving/index.md).

## Step 3: Apply Both Resources

Use the `ma` CLI to create or update each resource.

:::tip
`apply` works as an upsert — it creates the resource if it doesn't exist, or updates it if it does.
:::

```bash
ma inference_server apply -f inferenceserver.yaml
ma deployment apply -f deployment.yaml
```

Check status:

```bash
ma inference_server get -n <your-namespace> --name my-inference-server
ma deployment get -n <your-namespace> --name my-model-deployment
```

A healthy deployment progresses through validation, asset preparation, resource acquisition, model traffic routing, and finally `Rollout Complete`. The [Deployment Lifecycle section in the serving overview](../operator-guides/serving/index.md#deployment-lifecycle) explains each stage.

## Step 4: Send a Prediction Request

Once the deployment is complete, the model is reachable through the gateway. The exact URL depends on your environment — on the sandbox the gateway is exposed at `localhost:8080`. The path follows this pattern:

```
http://<gateway-host>/<inference-server-name>/<deployment-name>/infer
```

Send a request with the input shape your model expects:

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
The request and response payloads follow the [Triton inference protocol](https://github.com/triton-inference-server/server/blob/main/docs/protocol/extension_binary_data.md). Each input/output corresponds to a `ModelSchemaItem` in your packaged model.
:::

## Updating a Deployment

To roll out a new model version, update `desiredRevision.name` in `deployment.yaml` and re-apply:

```bash
ma deployment apply -f deployment.yaml
```

The Deployment controller handles the rollout, including rollback if the new revision fails health checks.

## Deleting a Deployment

```bash
ma deployment delete -n <your-namespace> --name my-model-deployment
```

Deleting the `Deployment` removes the model from the inference server but leaves the `InferenceServer` and the underlying model artifacts intact. To tear down the server too:

```bash
ma inference_server delete -n <your-namespace> --name my-inference-server
```

## Troubleshooting

### Deployment stuck in `Validation`

The model `Revision` or target `InferenceServer` couldn't be resolved. Confirm both exist with `ma revision get` and `ma inference_server get`, and that the namespace fields in the Deployment spec match.

### Deployment reaches `Rollout Complete` but predictions return 404

The gateway route may not be ready yet, or the URL path is incorrect. The path is `/<inference-server-name>/<deployment-name>/infer` — both names must match exactly (case-sensitive) and use the metadata `name` field, not the model name.

### Predictions return a schema validation error

The request payload doesn't match the model's `ModelSchema`. Re-check the input names, shapes, and dtypes against the schema you defined when packaging the model. See [Schema validation errors in the Model Registry guide](./model-registry-guide.md#schema-validation-errors) for details.

### Model artifact not found

The Deployment can't locate the packaged model in storage. Verify the artifact was uploaded to the configured bucket (`deploy-models` on the sandbox) and that the `Revision` references the correct path.

## Next Steps

This guide covers the end-user workflow assuming serving infrastructure is already in place. For platform-level concerns:

- **[Michelangelo Serving overview](../operator-guides/serving/index.md)** — architecture, controller lifecycles, and core concepts
- **[Sandbox Setup](../getting-started/sandbox-setup.md)** — full walkthrough with model upload and inference on a local sandbox
- **[Integrate with a Custom Backend](../operator-guides/serving/integrate-custom-backend.md)** — add support for new serving frameworks
- **[CLI Reference](./cli.md)** — every `ma` command, including the full list of supported flags
