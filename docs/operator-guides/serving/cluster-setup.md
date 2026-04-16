# Run Inference on a Local Sandbox

Deploy a model to a Triton inference server running in a local Michelangelo sandbox cluster.

## Prerequisites
- **Repository**: Local checkout with `$REPOROOT` pointing to the repo root
- **Tooling**: `poetry`, `docker`, `k3d`

## Procedure

1) Change to the Python workspace:

```bash
cd $REPOROOT/python
```

2) Create the Michelangelo sandbox:

```bash
poetry run ma sandbox create
```

3) Initialize the inference demo environment:

```bash
poetry run ma sandbox demo inference
```

This command:
- Creates an `InferenceServer` CR named `inference-server-example` in the `default` namespace
- Deploys a Triton inference server with the model-sync sidecar
- Creates the model ConfigMap for dynamic model loading
- Sets up the Gateway and HTTPRoute infrastructure

4) Upload your Triton model to MinIO storage:

This step can be done manually or through a Uniflow Pipeline. Your model artifacts should be placed in the `deploy-models` bucket. The sandbox includes a MinIO instance accessible at `http://localhost:9000` (credentials: `ma-minio`/`ma-minio`). 

5) Apply a Deployment CR to load the model:

```yaml
apiVersion: michelangelo.api/v2
kind: Deployment
metadata:
  name: bert-cola-deployment
  namespace: default
  labels:
    app: bert-cola-example
spec:
  desiredRevision:
    name: bert-cola-example
    namespace: default
  inferenceServer:
    name: inference-server-example
    namespace: default
  selector:
    matchLabels:
      environment: production
  deletionSpec:
    deleted: false
  strategy:
    rolling:
      incrementPercentage: 20
  definition:
    type: TARGET_TYPE_INFERENCE_SERVER
    subType: realtime-serving
  modelFamily:
    name: bert-cola-family
    namespace: default
  owner:
    name: "user-1234"
```

```bash
kubectl apply -f deployment.yaml
```

6) Run inference against the deployed model:

```bash
curl -X POST http://localhost:8080/inference-server-example/bert-cola-deployment/infer \
  -H "Content-Type: application/json" \
  -d '{
  "inputs": [
    {
      "name": "input_ids",
      "shape": [1, 10],
      "datatype": "INT64",
      "data": [101, 7592, 999, 102, 0, 0, 0, 0, 0, 0]
    },
    {
      "name": "attention_mask",
      "shape": [1, 10],
      "datatype": "INT64",
      "data": [1, 1, 1, 1, 0, 0, 0, 0, 0, 0]
    }
  ]
}'
```

**Outcome**:
- Sandbox cluster is running with Michelangelo controllers
- Triton inference server is deployed and healthy
- Model is loaded and serving inference requests

---

> **Note:** A remote cluster solution where inference servers are hosted in clusters separate from the control plane is coming soon.
