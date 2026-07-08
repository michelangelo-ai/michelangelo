# Comet ML

[Comet](https://www.comet.com/) is an experiment tracking platform. It logs metrics, hyperparameters, datasets, artifacts, and system/GPU usage during training, so teams can compare runs, visualize curves, and reproduce experiments. It can be hosted at `comet.com` or self-hosted — the deployment model only affects credential/network setup, not your code.

---

## How Comet ML works with Michelangelo

Comet calls happen inside your `@uniflow.task()` function — Michelangelo doesn't intercept them; the client talks directly to Comet from the task pod. Pick the hook for your framework (see **Integrations**), and make sure `COMET_API_KEY` is available (see **Configuring Comet ML across your environment**).

```python
import comet_ml
import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask

@uniflow.task(config=RayTask(head_cpu=2, head_memory="4Gi"))
def train(train_data, config: dict):
    experiment = comet_ml.start(project_name=config.get("project_name", "my-project"))
    experiment.log_parameters(config)
    for epoch in range(config.get("max_epochs", 10)):
        loss = ...  # your training step
        experiment.log_metric("loss", loss, step=epoch)
    experiment.end()
```

---

## Configuring Comet ML across your environment

Comet reads `COMET_API_KEY` and `COMET_WORKSPACE` from the environment (self-hosted also needs `COMET_URL_OVERRIDE`). Every process in a distributed run authenticates independently, so set credentials across the whole environment — not just locally.

> Operators: see the [Experiment Tracking operator guide](https://michelangelo-ai.org/docs/operator-guides/experiment-tracking) for the network/credential boundary that applies to all third-party trackers.
> 

**Local development:**

```bash
export COMET_API_KEY=<key>
export COMET_WORKSPACE=<workspace>
export COMET_URL_OVERRIDE=https://comet.internal/clientlib/   # self-hosted only
```

**Per pipeline run** (`--environ` is injected into every task's environment, including remote workers):

```bash
poetry run python pipeline.py remote-run --image <img> --storage-url <url> \
  --environ COMET_API_KEY=<key> \
  --environ COMET_WORKSPACE=<workspace> \
  --environ COMET_PROJECT_NAME=<project> \
  --environ COMET_URL_OVERRIDE=https://comet.internal/clientlib/   # self-hosted only
```

**Cluster-wide default (operator)** — prefer a Secret for the key:

```bash
kubectl create secret generic comet-credentials -n <compute-namespace> \
  --from-literal=COMET_API_KEY=<key>

kubectl patch configmap michelangelo-config -n <compute-namespace> --type merge \
  -p '{"data":{"COMET_WORKSPACE":"<workspace>","COMET_URL_OVERRIDE":"https://comet.internal/clientlib/"}}'
```

`pipeline.yaml` (the deploy manifest used with `ma pipeline apply`/`run`) holds no Comet config of its own — credentials come from the environment as above, and the task image it references must include `comet_ml` plus your framework's extras.

Network: hosted needs HTTPS egress to `comet.com:443`; self-hosted needs the server reachable from the compute namespace. **Verify:** `kubectl exec <pod> -n <namespace> -- env | grep COMET`

---

## Integrations

Pick the hook by your training framework. All examples read `COMET_API_KEY` from the environment — do not hardcode credentials.

### PyTorch Lightning

Pass Lightning's `CometLogger` to `pl.Trainer`; everything logged via `self.log(...)` is tracked. Under DDP it logs from rank 0 (one experiment).

```python
import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask
import pytorch_lightning as pl
from pytorch_lightning.loggers import CometLogger

@uniflow.task(config=RayTask(head_cpu=2, head_memory="4Gi"))
def train(train_data, config: dict):
    comet_logger = CometLogger(project_name=config.get("project_name", "my-project"))
    trainer = pl.Trainer(max_epochs=config.get("max_epochs", 10), logger=comet_logger)
    trainer.fit(model, train_dataloader, val_dataloader)
```

[Comet PyTorch Lightning integration docs](https://www.comet.com/docs/v2/integrations/ml-frameworks/pytorch-lightning/)

### Ray Train

Distributes training across the Ray cluster. Put `CometTrainLoggerCallback` on the driver and `comet_worker_logger` in the worker, passing the **same** `config` to both — all ranks log into one experiment with per-rank metrics.

```python
from comet_ml.integration.ray import CometTrainLoggerCallback, comet_worker_logger
import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask
import ray.train
from ray.train import RunConfig, ScalingConfig
from ray.train.torch import TorchTrainer

@uniflow.task(config=RayTask(head_cpu=2, head_memory="4Gi",
                             worker_cpu=4, worker_memory="16Gi", worker_instances=2))
def train(train_data, config: dict):
    callback = CometTrainLoggerCallback(config, project_name=config.get("project_name", "my-project"))

    def train_func(loop_config):
        with comet_worker_logger(loop_config) as experiment:
            for epoch in range(loop_config.get("max_epochs", 10)):
                loss = ...  # your training step
                ray.train.report({"loss": loss})

    trainer = TorchTrainer(
        train_func,
        train_loop_config=config,
        scaling_config=ScalingConfig(num_workers=config.get("num_workers", 2)),
        run_config=RunConfig(callbacks=[callback]),
    )
    trainer.fit()
```

Every process needs `COMET_API_KEY` in its environment (see Configuring above). [Comet Ray integration docs](https://www.comet.com/docs/v2/integrations/ml-frameworks/ray/)

### HuggingFace Transformers

Set `report_to=["comet_ml"]` to enable the native Comet callback (requires `accelerate`). Single-process or DDP, it logs from the main process.

```python
import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask
from transformers import Trainer, TrainingArguments

@uniflow.task(config=RayTask(head_cpu=2, head_memory="4Gi"))
def train(train_data, config: dict):
    training_args = TrainingArguments(
        output_dir="./output",
        num_train_epochs=config.get("max_epochs", 3),
        report_to=["comet_ml"],
    )
    Trainer(model=model, args=training_args, train_dataset=train_dataset).train()
```

[Comet HuggingFace Transformers integration docs](https://www.comet.com/docs/v2/integrations/ml-frameworks/transformers/)

### Custom Training Loop

Call `comet_ml.start()` for single-process training. For custom `torch` DDP, share one `COMET_EXPERIMENT_KEY` across processes so they join one experiment, and set the per-process node identifier so metrics are namespaced by rank:

```bash
export COMET_DISTRIBUTED_NODE_IDENTIFIER=$RANK   # per process; alongside a shared COMET_EXPERIMENT_KEY
```

```python
import os
import comet_ml
import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask

@uniflow.task(config=RayTask(head_cpu=2, head_memory="4Gi"))
def train(train_data, config: dict):
    # For DDP, set this per process (e.g. from torch.distributed.get_rank()):
    os.environ["COMET_DISTRIBUTED_NODE_IDENTIFIER"] = str(config.get("rank", 0))
    experiment = comet_ml.start(project_name=config.get("project_name", "my-project"))
    experiment.log_parameters(config)
    for epoch in range(config.get("max_epochs", 10)):
        loss = ...  # your training step
        experiment.log_metric("loss", loss, step=epoch)
    experiment.end()
```

[Comet Python SDK quickstart](https://www.comet.com/docs/v2/guides/quickstart/)

---

## Distributed experiments (any framework)

When one model is trained by **multiple processes**, you want **one** experiment, not one per process. Comet supports this regardless of framework — the hook depends on how you launch:

| Launcher | Comet integration hook | Result |
| --- | --- | --- |
| Ray Train (multi-node) | `comet_ml.integration.ray` | All ranks → one experiment, per-rank metrics |
| PyTorch Lightning DDP | `CometLogger` | One experiment (logs from rank 0) |
| HuggingFace DDP | `report_to=["comet_ml"]` | One experiment (logs from the main process) |
| Custom `torch` DDP | `comet_ml.start(experiment_key=...)` | Share one `COMET_EXPERIMENT_KEY` across processes |

---

## Comet ML Model Registry vs Michelangelo Model Registry

Comet and Michelangelo each have their own model registry; the two are independent and can be used together.

- **Use Comet's registry** for model governance, lineage, versioning, and stage transitions tied to your tracked experiments.
- **Use Michelangelo's registry** when you want models deployable via Michelangelo's `InferenceServer`.
- **Use both:** register to Comet for lineage/governance, and separately register the deployable artifact to Michelangelo for serving.

Register a model to Comet from your task — log it, then register it:

```python
experiment.log_model("my-model", "./model_dir")   # log the model to the experiment
experiment.register_model("my-model")              # register it in the workspace registry
```

After the experiment has ended, register with `comet_ml.API().get_experiment_by_key(<key>).register_model("my-model")`. Registered model names must be lowercase; versions use semantic versioning (default `1.0.0`).

[Comet Model Registry docs](https://www.comet.com/docs/v2/guides/model-registry/using-model-registry/)

---

## Troubleshooting

| Symptom | Likely Cause | Fix |
| --- | --- | --- |
| `ConnectionError` or timeout | Process can't reach Comet | Hosted: allow egress to `comet.com:443`. Self-hosted: check `COMET_URL_OVERRIDE` and server reachability |
| `Invalid API key` | `COMET_API_KEY` not set or wrong | `kubectl exec <pod> -n <ns> -- env | grep COMET` |
| No experiment in dashboard | Missing/invalid key, offline mode, or the experiment was never created | Confirm `COMET_API_KEY` is set and the logger/callback/`start()` actually runs |
| N experiments instead of 1 (distributed) | Each process created its own experiment | Use the framework's distributed hook (`comet_ml.integration.ray`, `CometLogger`, `report_to`), or share one `COMET_EXPERIMENT_KEY` |
| Experiment created but no metrics | Logger/callback not wired up | Pass `CometLogger` to `pl.Trainer`, set `report_to=["comet_ml"]`, or call `experiment.log_metric(...)` |