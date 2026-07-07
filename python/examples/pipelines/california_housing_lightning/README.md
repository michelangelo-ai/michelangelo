# California Housing Lightning

End-to-end ML pipeline for California Housing price prediction using PyTorch
Lightning via `tabular_trainer`'s `train_tabular()`. Counterpart to the
sibling [`california_housing_xgb`](../california_housing_xgb) example, which
uses a bespoke XGBoost training loop instead of `tabular_trainer`.

## Pipeline

```
feature_prep  →  preprocess  →  train  →  push_step
   (Ray)           (Spark)      (Ray)      (Spark)
```

| Step | File | Runtime | Description |
|---|---|---|---|
| `feature_prep` | `feature_prep.py` | Ray | Load dataset, train/test split, Ray Datasets |
| `preprocess` | `preprocess.py` | Spark | Cast columns to float |
| `train` | `train.py` | Ray | Distributed Lightning training via `tabular_trainer.train_tabular()` |
| `push_step` | `push.py` | Spark | Push model and preprocessed datasets to storage/registry |

`feature_prep.py` and `preprocess.py` are duplicated (not imported) from the
sibling `california_housing_xgb` example, to keep each example directory
fully self-contained per this repo's example convention.

## Prerequisites

Same as `california_housing_xgb` — see [its README](../california_housing_xgb/README.md#prerequisites)
for sandbox setup, Java/Spark requirements, and the `ma-examples` project.

## How It Works

### `train_tabular()` instead of a bespoke training loop

Unlike xgb's `train.py`, which builds its own `ScalingConfig`/`RunConfig` and
calls `xgboost.train()` directly, this example's `train.py` is a thin
`@uniflow.task` wrapper around
`michelangelo.workflow.tasks.tabular_trainer.task.train_tabular()` — the
shared Lightning + Ray Train dispatcher also covered by
`workflow/tasks/tabular_trainer/tests/`. `train_tabular()` builds its own
multi-node-safe `RunConfig` internally and returns a `ModelVariable`
pointing at the uploaded checkpoint, rather than a raw checkpoint path or an
assembled `ModelArtifact`. `push.py` downloads that checkpoint locally (via
`fsspec.core.url_to_fs()`) and wraps it in a `ModelArtifact` before handing
it to `ModelPusherPlugin`, since no OSS "assembler" task exists yet to do
that conversion automatically.

### `TorchRegressionModel` — the model to plug in

`train_tabular()` requires a `LightningModule` subclass, referenced by dotted
import path via `LightningTrainerConfig.model_class`. This example defines a
minimal MLP regressor in [`model.py`](model.py): two hidden `nn.Linear`
layers (64 → 32, ReLU) feeding a single scalar output, trained with
`MSELoss` and `Adam`.

Ray Data batches passed to `training_step`/`validation_step` are dicts of
column-name → tensor (the default `iter_torch_batches` output when no custom
collate function is configured). `TorchRegressionModel` is constructed with
`feature_columns`/`label_column` (via `LightningTrainerConfig.model_kwargs`)
so it knows which batch keys to stack into its input tensor and which key
holds the regression target.

### CPU-only precision

`train.py` explicitly forces `precision="32"` via `LightningTrainerKwargs`
rather than relying on `train_tabular()`'s dispatcher default
(`"bf16-mixed"`). Verified locally that `bf16-mixed` does not error on a
CPU-only accelerator — Lightning runs real `torch.autocast('cpu',
dtype=torch.bfloat16)` AMP, not a silent fallback — but on x86 CPUs without
`AVX512_BF16` this runs via slower software emulation. `precision="32"`
keeps this tutorial-oriented example's runs fast and deterministic on the
k3d sandbox's CPU-only nodes.

### No eval report

Unlike xgb's `push_step`, this example's pusher does not push an
`eval_report` artifact: `train_tabular()` returns a `ModelArtifact` without a
training-metrics dict (unlike xgb's custom `TrainResult.metrics`), so there
is nothing meaningful to report.

## Local Run

```bash
cd michelangelo-ai/michelangelo/python
JAVA_HOME=/opt/homebrew/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home \
PYTHONPATH=. poetry run python examples/pipelines/california_housing_lightning/california_housing_lightning.py
```

Without `AWS_ENDPOINT_URL`, `train` and `push_step` both use
`LocalStorageBackend`, writing the model checkpoint and datasets to local
temp directories (no external services required).

## Remote Run

Pass environment variables via `--environ` flags — they are serialized into the
Cadence/Temporal workflow and injected into every task's runtime environment,
reaching remote workers. Shell `export` statements before the command only
affect the local launcher and do not propagate.

```bash
cd michelangelo-ai/michelangelo/python
JAVA_HOME=/opt/homebrew/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home \
PYTHONPATH=. poetry run python examples/pipelines/california_housing_lightning/california_housing_lightning.py \
  remote-run \
  --image docker.io/library/my-workflow:latest \
  --storage-url s3://your-bucket/workflows \
  --environ AWS_ENDPOINT_URL=http://your-minio-endpoint:9000 \
  --environ AWS_ACCESS_KEY_ID=your-access-key \
  --environ AWS_SECRET_ACCESS_KEY=your-secret-key \
  --environ REGISTRY_ENDPOINT=your-apiserver-host:15566 \
  --yes
```

### k3d sandbox

```bash
cd michelangelo-ai/michelangelo/python
JAVA_HOME=/opt/homebrew/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home \
PYTHONPATH=. poetry run python examples/pipelines/california_housing_lightning/california_housing_lightning.py \
  remote-run \
  --image docker.io/library/my-workflow:latest \
  --storage-url s3://michelangelo/workflows \
  --environ AWS_ENDPOINT_URL=http://minio:9091 \
  --environ AWS_ACCESS_KEY_ID=minioadmin \
  --environ AWS_SECRET_ACCESS_KEY=minioadmin \
  --environ REGISTRY_ENDPOINT=michelangelo-apiserver:15566 \
  --yes
```

Before running, rebuild and import the image into the cluster:

```bash
docker build -t my-workflow:latest -f examples/Dockerfile .
k3d image import my-workflow:latest -c michelangelo-sandbox
kubectl delete cachedoutputs --all   # clear stale cached task outputs
```

### Environment variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `AWS_ENDPOINT_URL` | No | — | S3-compatible endpoint URL (include scheme, e.g. `http://minio:9091`). Unset → local storage |
| `AWS_ACCESS_KEY_ID` | If `AWS_ENDPOINT_URL` set | — | Access key ID |
| `AWS_SECRET_ACCESS_KEY` | If `AWS_ENDPOINT_URL` set | — | Secret access key |
| `AWS_S3_BUCKET` | No | Parsed from `MA_FILE_SYSTEM` or `UF_STORAGE_URL` | Target bucket name |
| `REGISTRY_ENDPOINT` | No | — | Model registry gRPC endpoint (`host:port`). Unset → in-memory only |
| `REGISTRY_INSECURE` | No | `true` | Set `false` to enable TLS for the registry connection |
| `REGISTRY_NAMESPACE` | No | `MA_NAMESPACE` (the pipeline's own namespace), else `default` | Model registry namespace |

> **Sandbox note:** in a k3d sandbox, `AWS_ENDPOINT_URL`, `AWS_ACCESS_KEY_ID`,
> and `AWS_SECRET_ACCESS_KEY` are automatically injected into Ray/Spark pods
> via the `michelangelo-config` ConfigMap — no `--environ` flags needed for
> `ma pipeline run`. For `remote-run`, pass them explicitly with `--environ`.
> The apiserver Service and this example's task pods run in the same
> namespace by default, so `REGISTRY_ENDPOINT` can use the short in-cluster
> DNS name (`michelangelo-apiserver:15566`) rather than the full
> `michelangelo-apiserver.<namespace>.svc.cluster.local:15566` form.
