# MovieLens-100k NCF example

Smallest viable smoke test for `michelangelo.lib.trainer.torch.pytorch_lightning.lightning_trainer.LightningTrainer`.
Trains a tiny Neural Collaborative Filtering model on MovieLens-100k on CPU with a single Ray Train worker.

## Run

From the `python/` directory:

```bash
# Install the trainer + the umbrella `example` extra (provides pandas/mlflow/etc.
# for the demos). The trainer extras are split so users only install what they
# need:
#   - `trainer`            — ray, torch, pytorch_lightning, transformers, numpy
#   - `trainer-comet`      — adds comet_ml (optional Comet tracking)
#   - `trainer-deepspeed`  — adds deepspeed (only needed for DeepSpeed training)
# This demo uses MLflow tracking (already in the `example` extra) and CPU DDP,
# so plain `trainer` is enough.
poetry install --extras "trainer example"
python -m examples.movielens.train
```

The first invocation downloads the dataset (~5 MB) to `/tmp/movielens_data/`.
Checkpoints land in `/tmp/movielens_runs/ncf_movielens100k/`.

## What it exercises

- Loading the snapshotted `LightningTrainer` and `LightningTrainerParam`.
- The trainer's per-worker training loop (`_train_loop_per_worker`) for a non-trivial
  end-to-end Lightning fit, including epoch checkpointing via `RayTrainReportCallback`.
- Default Ray Data → torch tensor collation (no custom `data_collate_fn`).
- Resolving the default `RayDDPStrategy` even when running with a single worker.

## Optional: log to Comet or MLflow

Experiment tracking is opt-in. By default the demo uses Lightning's local logger.
At most one backend is active per run; **Comet wins if both env-sets are set**.

### Comet

```bash
# Comet requires the optional comet_ml dependency:
poetry install --extras "trainer trainer-comet example"

export COMET_API_KEY=...                          # required
export COMET_WORKSPACE=<your-workspace>           # required
export COMET_PROJECT_NAME=michelangelo-demo       # optional, default michelangelo-movielens-demo
export COMET_EXPERIMENT_NAME=movielens-run-1      # optional, default ncf-movielens100k
export COMET_TAGS=demo,movielens,smoke            # optional, comma-separated
python -m examples.movielens.train
```

`train.py` reads these and resolves them into `build_comet_logger` factory
kwargs, forwarded through `LightningTrainerParam.lightning_trainer_kwargs` so
the `CometLogger` is constructed inside the Ray Train worker. With Comet
enabled you'll see a "Comet experiment URL: ..." line in the worker logs.

### MLflow

```bash
export MLFLOW_TRACKING_URI=file:///tmp/mlflow_movielens        # required (or http://...)
export MLFLOW_EXPERIMENT_NAME=ncf-movielens                    # optional, default ncf-movielens100k
export MLFLOW_RUN_NAME=run-001                                 # optional
export MLFLOW_TAGS=team=ml-platform,owner=demo                 # optional, comma-separated key=value
python -m examples.movielens.train
```

`train.py` constructs a `pytorch_lightning.loggers.MLFlowLogger` and passes it
through `lightning_trainer_kwargs["logger"]`. The trainer's `_resolve_logger`
forwards a pre-built Logger instance unchanged. View runs by pointing the
MLflow UI at the same tracking URI (`mlflow ui --backend-store-uri file:///tmp/mlflow_movielens`).

## Files

- `data.py` — downloads MovieLens-100k, builds dense user/item index, returns Ray datasets.
- `model.py` — `NCFLightningModule` (user + item embeddings → 2-layer MLP → sigmoid, MSE loss).
- `train.py` — wires up `LightningTrainerParam`, `LightningTrainer`, and Ray `RunConfig` / `ScalingConfig`.
