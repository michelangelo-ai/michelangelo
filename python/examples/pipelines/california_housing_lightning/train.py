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


def _resolve_storage_backend():
    """Select MinIO (remote) or a local temp directory.

    Matches the ``california_housing_xgb`` example's ``push.py``
    storage-backend selection: ``AWS_ENDPOINT_URL`` set -> MinIO/S3-compatible
    remote storage; unset -> local filesystem (development and CI).
    """
    import tempfile
    from urllib.parse import urlparse

    s3_endpoint = os.environ.get("AWS_ENDPOINT_URL", "")
    if s3_endpoint:
        parsed = urlparse(s3_endpoint)
        endpoint = parsed.netloc
        if not endpoint:
            raise ValueError(
                f"AWS_ENDPOINT_URL={s3_endpoint!r} is missing a scheme. "
                "Use a full URL like http://minio:9091"
            )
        from michelangelo.lib.artifact_manager.minio_backend import MinioStorageBackend

        bucket = (
            os.environ.get("AWS_S3_BUCKET")
            or (
                os.environ.get("MA_FILE_SYSTEM")
                or os.environ.get("UF_STORAGE_URL", "s3://default")
            )
            .removeprefix("s3://")
            .split("/")[0]
        )
        return MinioStorageBackend(
            endpoint=endpoint,
            bucket=bucket,
            access_key=os.environ.get("AWS_ACCESS_KEY_ID", ""),
            secret_key=os.environ.get("AWS_SECRET_ACCESS_KEY", ""),
            secure=parsed.scheme == "https",
            create_bucket_if_missing=True,
        )

    from michelangelo.lib.artifact_manager.storage_backend import LocalStorageBackend

    local_dir = tempfile.mkdtemp(prefix="california_lightning_train_")
    log.info("train: using LocalStorageBackend (local/CI) -> %s", local_dir)
    return LocalStorageBackend(local_dir)


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
    storage_backend = _resolve_storage_backend()

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
    )
