"""Lightning training task for the California Housing Lightning workflow.

Trains a small PyTorch Lightning regression model on preprocessed California
Housing data via ``tabular_trainer``'s ``train_tabular()`` (Ray Train +
Lightning backend), the counterpart to the sibling ``california_housing_xgb``
example's bespoke XGBoost training loop.
"""

from __future__ import annotations

import logging
import os
from typing import TYPE_CHECKING

import michelangelo.uniflow.core as uniflow
from examples.pipelines.california_housing_lightning._backend import (
    resolve_storage_backend,
)
from michelangelo.uniflow.plugins.ray import RayTask
from michelangelo.workflow.schema.tabular_trainer import (
    BatchIterConfig,
    ColumnConfig,
    DataloadingConfig,
    LightningTrainerConfig,
    LightningTrainerKwargs,
    ScalingConfig,
    TabularTrainerConfig,
)
from michelangelo.workflow.tasks.tabular_trainer.task import train_tabular

if TYPE_CHECKING:
    from examples.pipelines.california_housing_lightning.preprocess import (
        PreprocessResult,
    )
    from michelangelo.workflow.variables.types import ModelArtifact

log = logging.getLogger(__name__)

__all__ = ["train"]

LABEL_COLUMN = "target"


@uniflow.task(
    config=RayTask(
        head_cpu=1,
        head_gpu=0,
        head_memory="4Gi",
        worker_cpu=1,
        worker_gpu=0,
        worker_memory="4Gi",
        worker_instances=2,
    ),
)
def train(
    pr: PreprocessResult,
    feature_columns: list[str],
) -> ModelArtifact:
    """Train a Lightning regression model using Ray Train.

    Args:
        pr: PreprocessResult containing preprocessed training and validation
            datasets.
        feature_columns: Ordered list of feature column names (excludes the
            label column).

    Returns:
        ModelArtifact pointing at the uploaded model checkpoint. Unlike the
        sibling xgb example's ``train()``, this is already the final artifact
        type expected by the pusher -- ``train_tabular()`` uploads the model
        itself and returns a ``ModelArtifact`` directly.
    """
    storage_backend, is_remote = resolve_storage_backend("california_lightning_train_")

    # train_tabular() defaults to a local-tempdir storage_path for Ray Train's
    # own distributed checkpointing when no run_config is given -- fine for a
    # single-process local run, but broken on a real multi-node cluster since
    # worker pods can't see the head pod's local filesystem. Point it at the
    # same MinIO/S3 backend used for the final model upload when running
    # remotely, namespaced by MA_PIPELINE_RUN_NAME (set by the platform on
    # every task pod) so concurrent runs don't clobber each other's checkpoints.
    run_config = None
    if is_remote:
        import ray.train

        from michelangelo.uniflow.plugins.ray.io import resolve_fs

        bucket = storage_backend.get_storage_location().removeprefix("s3://")
        run_id = os.environ.get("MA_PIPELINE_RUN_NAME", "local")
        run_config = ray.train.RunConfig(
            storage_path=f"{bucket}/ray_train_runs/{run_id}",
            storage_filesystem=resolve_fs("s3"),
        )

    config = TabularTrainerConfig(
        lightning=LightningTrainerConfig(
            model_class=(
                "examples.pipelines.california_housing_lightning.model."
                "TorchRegressionModel"
            ),
            model_kwargs={
                "feature_columns": feature_columns,
                "label_column": LABEL_COLUMN,
            },
            input_columns={
                c: ColumnConfig("torch.float32") for c in feature_columns
            },
            output_columns={"prediction": ColumnConfig("torch.float32")},
            labels={LABEL_COLUMN: ColumnConfig("torch.float32")},
            metadata_columns=[],
            scaling_config=ScalingConfig(cpu_per_worker=1),
            dataloading_config=DataloadingConfig(
                batch_iter_config=BatchIterConfig(
                    batch_size=64, num_shuffle_batches=1
                )
            ),
            # precision explicitly forced to "32" rather than relying on the
            # dispatcher's "bf16-mixed" default: verified locally that
            # bf16-mixed does not error on CPU (real torch.autocast('cpu', ...)
            # AMP, not a silent fallback), but on x86 CPUs without
            # AVX512_BF16 it runs via slower software emulation. This example
            # prioritizes fast, deterministic CI/sandbox runs over AMP.
            lightning_trainer_kwargs=LightningTrainerKwargs(
                max_epochs=5, precision="32"
            ),
        )
    )

    return train_tabular(
        config,
        pr.train_data,
        pr.validation_data,
        storage_backend=storage_backend,
        run_config=run_config,
    )
