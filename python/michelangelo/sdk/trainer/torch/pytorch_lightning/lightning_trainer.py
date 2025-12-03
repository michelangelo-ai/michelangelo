"""Lightning trainer - compatible with internal SDK API."""

import logging
from dataclasses import dataclass
from typing import Any, Callable, Optional

import pytorch_lightning as pl
from ray.data import Dataset
from ray.train import RunConfig, ScalingConfig
from ray.train.lightning import RayDDPStrategy, RayLightningEnvironment
from ray.train.torch import TorchTrainer

log = logging.getLogger(__name__)


@dataclass
class LightningTrainerParam:
    """Parameters for LightningTrainer - matches internal API exactly."""
    create_model: Callable[..., pl.LightningModule]
    model_kwargs: dict[str, Any]
    train_data: Dataset
    validation_data: Dataset
    batch_size: int
    num_epochs: int
    lightning_trainer_kwargs: Optional[dict[str, Any]] = None

    def __post_init__(self):
        """Initialize lightning_trainer_kwargs if not provided."""
        if self.lightning_trainer_kwargs is None:
            self.lightning_trainer_kwargs = {}


class LightningTrainer:
    """Lightning trainer that wraps Ray Train.

    Compatible with internal
    uber.ai.michelangelo.sdk.trainer.torch.pytorch_lightning.lightning_trainer.LightningTrainer.
    """

    def __init__(self, param: LightningTrainerParam):
        """Initialize the Lightning trainer with parameters."""
        self.param = param
        self._setup_trainer()

    def _setup_trainer(self):
        """Setup the Ray TorchTrainer with Lightning."""

        def train_loop_per_worker(config):
            """Training loop that runs on each worker."""
            import os

            import torch
            from ray import train
            from ray.train import get_context

            # Get Ray Train context
            get_context()

            # Setup Lightning environment
            RayLightningEnvironment()

            # Create model
            model = self.param.create_model(**self.param.model_kwargs)

            # Get distributed datasets
            train_dataset = train.get_dataset_shard("train")
            val_dataset = train.get_dataset_shard("validation")

            # Convert to PyTorch datasets
            def ray_dataset_to_torch(ray_ds, batch_size):
                """Convert Ray dataset to PyTorch DataLoader."""
                from torch.utils.data import DataLoader
                from torch.utils.data import Dataset as TorchDataset

                class RayTorchDataset(TorchDataset):
                    def __init__(self, ray_dataset_iter):
                        # Convert the iterator to list for indexing
                        self.data = []
                        for batch in ray_dataset_iter.iter_batches():
                            # batch is already a dict with lists
                            if isinstance(batch, dict):
                                # Convert columnar format to row format
                                num_rows = len(next(iter(batch.values())))
                                for i in range(num_rows):
                                    item = {k: v[i] for k, v in batch.items()}
                                    self.data.append(item)
                            else:
                                # If not dict, try pandas conversion
                                for item in batch.to_pandas().to_dict('records'):
                                    self.data.append(item)

                    def __len__(self):
                        return len(self.data)

                    def __getitem__(self, idx):
                        item = self.data[idx]
                        return {
                            k: (torch.tensor(v, dtype=torch.long)
                                if isinstance(v, list) else v)
                            for k, v in item.items()
                        }

                torch_dataset = RayTorchDataset(ray_ds)
                return DataLoader(
                    torch_dataset,
                    batch_size=batch_size,
                    shuffle=True,
                    num_workers=0,  # Ray handles the parallelism
                )

            train_dataloader = ray_dataset_to_torch(
                train_dataset, self.param.batch_size
            )
            val_dataloader = ray_dataset_to_torch(
                val_dataset, self.param.batch_size
            )

            # Setup trainer kwargs - let Ray handle MLflow logging
            trainer_kwargs = {
                "max_epochs": self.param.num_epochs,
                "enable_checkpointing": True,
                "logger": False,  # Ray MLflow callback will handle logging
                **self.param.lightning_trainer_kwargs
            }

            # Use Ray strategy if specified
            if "strategy" in trainer_kwargs:
                # Keep the strategy as-is (e.g., RayFSDPStrategy)
                pass
            else:
                # Default to Ray DDP strategy
                trainer_kwargs["strategy"] = RayDDPStrategy()

            # Create Lightning trainer
            trainer = pl.Trainer(**trainer_kwargs)

            # Train the model
            trainer.fit(
                model,
                train_dataloaders=train_dataloader,
                val_dataloaders=val_dataloader,
            )

            # Save model checkpoint for Ray to capture
            import tempfile

            from ray import train as ray_train

            # Create checkpoint in temporary directory for Ray to capture
            checkpoint_dir = tempfile.mkdtemp()

            checkpoint_path = os.path.join(checkpoint_dir, "model_checkpoint.ckpt")
            trainer.save_checkpoint(checkpoint_path)

            # Also save the model state directly
            model_path = os.path.join(checkpoint_dir, "model_state.pt")
            torch.save(model.state_dict(), model_path)

            # Report final metrics to Ray for MLflow logging
            final_metrics = {}
            if hasattr(trainer, 'logged_metrics') and trainer.logged_metrics:
                for key, value in trainer.logged_metrics.items():
                    if isinstance(value, torch.Tensor):
                        final_metrics[key] = value.item()
                    else:
                        final_metrics[key] = value

            # Report checkpoint and metrics to Ray Train
            # (this gets picked up by MLflow callback)
            ray_train.report(
                final_metrics,
                checkpoint=ray_train.Checkpoint.from_directory(checkpoint_dir)
            )

            return {"metrics": final_metrics}

        # Create TorchTrainer with Lightning
        self.torch_trainer = TorchTrainer(
            train_loop_per_worker=train_loop_per_worker,
            datasets={
                "train": self.param.train_data,
                "validation": self.param.validation_data,
            },
            train_loop_config={},
        )

    def train(self, run_config: RunConfig, scaling_config: ScalingConfig):
        """Train the model using Ray.

        Returns Ray Result object compatible with internal API.
        """
        log.info("Starting distributed Lightning training...")

        # Configure the trainer with scaling and run configs
        self.torch_trainer._scaling_config = scaling_config
        self.torch_trainer._run_config = run_config

        # Train and return result
        result = self.torch_trainer.fit()

        log.info("Distributed Lightning training completed")
        return result
