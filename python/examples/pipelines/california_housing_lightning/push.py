"""Pusher step for the California Housing Lightning workflow.

Pushes the trained model and preprocessed train/validation datasets to
storage and registry in a single Spark task. Simpler than the sibling xgb
example's ``push.py``: ``train_tabular()`` already returns a ``ModelArtifact``
(the final artifact type the pusher expects), so there is no raw-checkpoint
glob/MinIO-listing step needed to locate it.

Unlike xgb's ``TrainResult``, ``train_tabular()`` does not return training
metrics on its ``ModelArtifact`` (no eval-metrics dict), so this pusher omits
the ``eval_report`` plugin that the xgb example pushes.
"""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.spark import SparkTask
from michelangelo.workflow.schema.pusher import (
    DatasetPluginConfig,
    ModelPluginConfig,
    PusherConfig,
    PusherPluginConfig,
)
from michelangelo.workflow.tasks.pusher import push
from michelangelo.workflow.variables.types import AssembledModel, PusherResult

if TYPE_CHECKING:
    from examples.pipelines.california_housing_lightning.preprocess import (
        PreprocessResult,
    )
    from michelangelo.workflow.variables.types import ModelArtifact

log = logging.getLogger(__name__)

__all__ = ["push_step"]


@uniflow.task(
    config=SparkTask(
        driver_cpu=1,
        driver_memory="4G",
        executor_cpu=1,
        executor_memory="2G",
        executor_instances=1,
    ),
)
def push_step(
    pr: PreprocessResult,
    model_artifact: ModelArtifact,
) -> list[PusherResult]:
    """Push the trained model and preprocessed datasets in a single Spark step.

    Pushes three artifacts using a single storage backend selected at runtime:

    - **model** -- the Lightning checkpoint, already an assembled
      ``ModelArtifact`` from ``train_tabular()``, via ``ModelPusherPlugin``.
    - **train_data** / **validation_data** -- preprocessed datasets via
      ``DatasetPusherPlugin`` + ``S3Sink`` (remote) or ``LocalFileSink`` (local/CI).

    Args:
        pr: Result of the ``preprocess`` task, holding preprocessed training
            and validation ``DatasetVariable`` handles.
        model_artifact: Result of the ``train`` task -- an already-assembled
            ``ModelArtifact`` pointing at the uploaded Lightning checkpoint.

    Returns:
        List of ``PusherResult``, one per artifact pushed.
    """
    import os
    import tempfile

    s3_endpoint = os.environ.get("AWS_ENDPOINT_URL", "")
    from urllib.parse import urlparse

    parsed = urlparse(s3_endpoint) if s3_endpoint else None
    endpoint = parsed.netloc if parsed else None
    if s3_endpoint and not endpoint:
        raise ValueError(
            f"AWS_ENDPOINT_URL={s3_endpoint!r} is missing a scheme. "
            "Use a full URL like http://minio:9091"
        )
    secure = parsed.scheme == "https" if parsed else False
    access_key = os.environ.get("AWS_ACCESS_KEY_ID", "")
    secret_key = os.environ.get("AWS_SECRET_ACCESS_KEY", "")

    _run_id = os.path.basename(model_artifact.path.rstrip("/"))

    pr.train_data.load_pandas_dataframe()
    pr.validation_data.load_pandas_dataframe()

    if endpoint:
        bucket = (
            os.environ.get("AWS_S3_BUCKET")
            or (
                os.environ.get("MA_FILE_SYSTEM")
                or os.environ.get("UF_STORAGE_URL", "s3://default")
            )
            .removeprefix("s3://")
            .split("/")[0]
        )
        if not bucket:
            raise OSError(
                "Could not determine storage bucket. "
                "Set AWS_S3_BUCKET or MA_FILE_SYSTEM."
            )
        from michelangelo.lib.artifact_manager.minio_backend import MinioStorageBackend
        from michelangelo.workflow.schema.sinks.s3 import S3SinkConfig
        from michelangelo.workflow.tasks.functions.sinks import S3Sink

        storage_backend = MinioStorageBackend(
            endpoint=endpoint,
            bucket=bucket,
            access_key=access_key,
            secret_key=secret_key,
            secure=secure,
            create_bucket_if_missing=True,
        )
        log.info(
            "push_step: using MinioStorageBackend (remote) -> %s",
            storage_backend.get_storage_location(),
        )

        def _dataset_config(key: str) -> DatasetPluginConfig:
            return DatasetPluginConfig(
                sinks=[S3Sink(S3SinkConfig(key, storage_backend=storage_backend))]
            )
    else:
        from michelangelo.lib.artifact_manager.storage_backend import (
            LocalStorageBackend,
        )
        from michelangelo.workflow.schema.sinks.local import LocalFileSinkConfig
        from michelangelo.workflow.tasks.functions.sinks import LocalFileSink

        _local_dir = tempfile.mkdtemp(prefix="california_lightning_push_")
        storage_backend = LocalStorageBackend(_local_dir)
        log.info(
            "push_step: using LocalStorageBackend (local/CI) -> %s",
            storage_backend.get_storage_location(),
        )

        def _dataset_config(key: str) -> DatasetPluginConfig:  # type: ignore[misc]
            return DatasetPluginConfig(
                sinks=[
                    LocalFileSink(
                        LocalFileSinkConfig(
                            destination_path=os.path.join(_local_dir, key)
                        )
                    )
                ]
            )

    registry_endpoint = os.environ.get("REGISTRY_ENDPOINT")
    if registry_endpoint:
        import grpc as _grpc

        from michelangelo.api.v2 import APIClient
        from michelangelo.lib.model_manager.registry.api_client import APIRegistryClient

        _insecure = os.environ.get("REGISTRY_INSECURE", "true").lower() != "false"
        _credentials = None if _insecure else _grpc.ssl_channel_credentials()
        _channel = (
            _grpc.insecure_channel(registry_endpoint)
            if _insecure
            else _grpc.secure_channel(registry_endpoint, _credentials)
        )
        _api_client = APIClient(
            caller="california-housing-lightning-push-step",
            channel=_channel,
        )
        registry_client = APIRegistryClient(
            svc=_api_client.ModelService,
            namespace=os.environ.get("REGISTRY_NAMESPACE", "default"),
        )
        log.info("push_step: using APIRegistryClient at %s", registry_endpoint)
    else:
        from michelangelo.lib.model_manager.registry.client import (
            InMemoryRegistryClient,
        )

        registry_client = InMemoryRegistryClient()
        log.warning(
            "REGISTRY_ENDPOINT not set -- using InMemoryRegistryClient. "
            "Model registration will not be persisted."
        )

    config = PusherConfig(
        items=[
            PusherPluginConfig(
                name="model",
                model_plugin=ModelPluginConfig(
                    model_name="california-housing-lightning",
                    description=(
                        "PyTorch Lightning regression on California Housing dataset"
                    ),
                    labels={"framework": "pytorch_lightning"},
                ),
            ),
            PusherPluginConfig(
                name="train_data",
                dataset_plugin=_dataset_config(
                    f"datasets/california-housing-lightning/{_run_id}/train"
                ),
            ),
            PusherPluginConfig(
                name="validation_data",
                dataset_plugin=_dataset_config(
                    f"datasets/california-housing-lightning/{_run_id}/validation"
                ),
            ),
        ]
    )

    assembled = AssembledModel(raw_model=model_artifact)

    results = push(
        config=config,
        artifacts={
            "model": assembled,
            "train_data": pr.train_data,
            "validation_data": pr.validation_data,
        },
        storage_backend=storage_backend,
        registry_client=registry_client,
    )

    for r in results:
        log.info(
            "push %s (%s): success=%s value=%s error=%s",
            r.name,
            r.plugin,
            r.success,
            r.value,
            r.error,
        )

    return results
