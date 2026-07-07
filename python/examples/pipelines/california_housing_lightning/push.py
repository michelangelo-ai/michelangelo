"""Pusher step for the California Housing Lightning workflow.

Pushes the trained model and preprocessed train/validation datasets to
storage and registry in a single Spark task. ``train_tabular()`` hands off
its trained model as an intra-pipeline ``ModelVariable`` (there is no OSS
"assembler" task yet to package it into a registry-ready ``ModelArtifact``),
so ``push_step`` does that conversion itself (download the state-dict file,
wrap it as a local ``ModelArtifact``) before handing off to the shared
``ModelPusherPlugin``.

Unlike xgb's ``TrainResult``, ``train_tabular()`` does not return training
metrics (no eval-metrics dict), so this pusher omits the ``eval_report``
plugin that the xgb example pushes.
"""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

import michelangelo.uniflow.core as uniflow
from examples.pipelines.california_housing_lightning._backend import (
    resolve_storage_backend,
)
from michelangelo.uniflow.plugins.spark import SparkTask
from michelangelo.workflow.schema.pusher import (
    DatasetPluginConfig,
    ModelPluginConfig,
    PusherConfig,
    PusherPluginConfig,
)
from michelangelo.workflow.tasks.pusher import push
from michelangelo.workflow.variables.types import (
    AssembledModel,
    ModelArtifact,
    PusherResult,
)

if TYPE_CHECKING:
    from examples.pipelines.california_housing_lightning.preprocess import (
        PreprocessResult,
    )
    from michelangelo.workflow.variables import ModelVariable

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
    model_variable: ModelVariable,
) -> list[PusherResult]:
    """Push the trained model and preprocessed datasets in a single Spark step.

    Pushes three artifacts using a single storage backend selected at runtime:

    - **model** -- the Lightning checkpoint, converted from the
      intra-pipeline ``ModelVariable`` returned by ``train_tabular()`` into a
      local ``ModelArtifact``, via ``ModelPusherPlugin``.
    - **train_data** / **validation_data** -- preprocessed datasets via
      ``DatasetPusherPlugin`` + ``S3Sink`` (remote) or ``LocalFileSink`` (local/CI).

    Args:
        pr: Result of the ``preprocess`` task, holding preprocessed training
            and validation ``DatasetVariable`` handles.
        model_variable: Result of the ``train`` task -- a ``ModelVariable``
            wrapping the trained Lightning model, persisted under
            ``UF_STORAGE_URL``.

    Returns:
        List of ``PusherResult``, one per artifact pushed.
    """
    import os
    import tempfile

    import fsspec

    storage_backend, is_remote = resolve_storage_backend("california_lightning_push_")

    _run_id = os.path.basename(model_variable.path.rstrip("/"))

    # train_tabular() no longer packages/uploads the model itself -- it hands
    # off an intra-pipeline ModelVariable persisted under UF_STORAGE_URL, and
    # there is no OSS "assembler" task yet to turn that into a registry-ready
    # ModelArtifact. Pull the state-dict file it already wrote (via
    # save_lightning_model()) down to local disk with the same fsspec
    # mechanism ModelVariable itself uses, then wrap it as a ModelArtifact --
    # the local-file contract ModelPusherPlugin expects.
    local_model_dir = tempfile.mkdtemp(prefix="california_lightning_push_model_")
    local_model_path = os.path.join(local_model_dir, "model.pt")
    fs, remote_model_path = fsspec.core.url_to_fs(model_variable.path)
    fs.get(remote_model_path, local_model_path)
    model_artifact = ModelArtifact(
        path=local_model_path, metadata=model_variable.metadata
    )

    pr.train_data.load_pandas_dataframe()
    pr.validation_data.load_pandas_dataframe()

    if is_remote:
        from michelangelo.workflow.schema.sinks.s3 import S3SinkConfig
        from michelangelo.workflow.tasks.functions.sinks import S3Sink

        def _dataset_config(key: str) -> DatasetPluginConfig:
            return DatasetPluginConfig(
                sinks=[S3Sink(S3SinkConfig(key, storage_backend=storage_backend))]
            )
    else:
        from michelangelo.workflow.schema.sinks.local import LocalFileSinkConfig
        from michelangelo.workflow.tasks.functions.sinks import LocalFileSink

        _local_dir = storage_backend.get_storage_location()

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
