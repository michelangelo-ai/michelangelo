# MovieLens-100k NCF example

Smallest viable smoke test for `michelangelo.lib.trainer.torch.pytorch_lightning.lightning_trainer.LightningTrainer`.
Trains a tiny Neural Collaborative Filtering model on MovieLens-100k on CPU with a single Ray Train worker.

## Run

From the `python/` directory:

```bash
# One-time install. `trainer` covers ray + torch + pytorch_lightning + transformers +
# numpy + comet_ml + deepspeed. `example` adds pandas (used by data.py for the TSV load).
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

## Files

- `data.py` — downloads MovieLens-100k, builds dense user/item index, returns Ray datasets.
- `model.py` — `NCFLightningModule` (user + item embeddings → 2-layer MLP → sigmoid, MSE loss).
- `train.py` — wires up `LightningTrainerParam`, `LightningTrainer`, and Ray `RunConfig` / `ScalingConfig`.
