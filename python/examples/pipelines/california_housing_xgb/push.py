"""Pusher step for the California Housing XGBoost workflow.

Pushes all pipeline artifacts in a single Spark task: trained XGBoost model,
evaluation report, and preprocessed train/validation datasets. All four artifacts
share the same storage backend — MinIO / S3-compatible for remote runs,
local filesystem for development and CI.
"""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.spark import SparkTask
from michelangelo.workflow.schema.pusher import (
    DatasetPluginConfig,
    EvalReportPluginConfig,
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
    from examples.pipelines.california_housing_xgb.preprocess import PreprocessResult
    from examples.pipelines.california_housing_xgb.train import TrainResult

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
    train_result: TrainResult,
) -> list[PusherResult]:
    """Push all pipeline artifacts to storage and registry in a single Spark step.

    Pushes four artifacts using a single storage backend selected at runtime:

    - **model** — trained XGBoost checkpoint via ``ModelPusherPlugin``.
    - **eval_report** — training metrics via ``EvalReportPusherPlugin``.
    - **train_data** — preprocessed training dataset via ``DatasetPusherPlugin``
      + ``S3Sink`` (remote) or ``LocalFileSink`` (local/CI).
    - **validation_data** — preprocessed validation dataset via
      ``DatasetPusherPlugin`` + ``S3Sink`` (remote) or ``LocalFileSink`` (local/CI).

    All four artifacts share the same storage backend:

    - **Remote** (``AWS_ENDPOINT_URL`` set): ``MinioStorageBackend`` — model and
      eval report are uploaded directly; datasets are serialised to Parquet and
      uploaded via ``S3Sink``.
    - **Local** (``AWS_ENDPOINT_URL`` unset): ``LocalStorageBackend`` —
      model and eval report are
      copied to a temp directory; datasets are written as Parquet via
      ``LocalFileSink``.

    All infrastructure (storage backend, registry client, sinks) is constructed
    inside the task body — required by the UniFlow codec boundary. Stateful
    objects cannot be serialised across the workflow→task boundary.

    Args:
        pr: Result of the ``preprocess`` task, holding preprocessed training
            and validation ``DatasetVariable`` handles.
        train_result: Result of the ``train`` task, holding the XGBoost
            checkpoint path and training metrics.

    Returns:
        List of ``PusherResult``, one per artifact pushed.
    """
    import glob
    import os
    import tempfile
    from urllib.parse import urlparse

    s3_endpoint = os.environ.get("AWS_ENDPOINT_URL", "")
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

    # ── Locate XGBoost checkpoint ────────────────────────────────────────────
    # In a remote run, train_result.path is an S3 path (e.g.
    # "michelangelo/workflows/ray_results/ray_train_run-...") and glob.glob
    # cannot traverse S3. Use the MinIO client to list and download model.ubj.
    # In a local run, train_result.path is a local filesystem path.
    raw_path = train_result.path
    if endpoint:
        from minio import Minio

        _mc = Minio(
            endpoint,
            access_key=access_key,
            secret_key=secret_key,
            secure=secure,
        )
        # Normalize: strip s3:// scheme if present
        s3_path = raw_path.removeprefix("s3://")
        bucket, _, prefix = s3_path.partition("/")

        objects = list(_mc.list_objects(bucket, prefix=prefix, recursive=True))
        ubj_objects = [o for o in objects if o.object_name.endswith("model.ubj")]
        if not ubj_objects:
            ubj_objects = [o for o in objects if not o.is_dir]
        if not ubj_objects:
            raise FileNotFoundError(
                f"No model checkpoint found in s3://{bucket}/{prefix}"
            )
        tmp_ckpt_dir = tempfile.mkdtemp(prefix="checkpoint_")
        checkpoint_path = os.path.join(tmp_ckpt_dir, "model.ubj")
        _mc.fget_object(bucket, ubj_objects[0].object_name, checkpoint_path)
    else:
        checkpoint_glob = os.path.join(raw_path, "**", "model.ubj")
        matches = glob.glob(checkpoint_glob, recursive=True)
        if not matches:
            # model.ubj is the default XGBoost binary checkpoint written by
            # XGBoostTrainer. Fall back to any non-directory file under the
            # checkpoint dir if Ray writes to a different name in future versions.
            matches = [
                p
                for p in glob.glob(os.path.join(raw_path, "**", "*"), recursive=True)
                if os.path.isfile(p)
            ]
        if not matches:
            raise FileNotFoundError(f"No model checkpoint found under {raw_path}")
        checkpoint_path = matches[0]
    log.info("Found model checkpoint: %s", checkpoint_path)

    # ── Per-run path prefix ───────────────────────────────────────────────────
    # Derive a unique run ID from the Ray training run directory name so that
    # dataset and model outputs from different runs never share the same path.
    # Using the train_result path basename keeps dataset keys correlated with
    # the model checkpoint — both live under the same run identifier.
    # Example: "ray_train_run-2026-06-09_19-42-48"
    _run_id = os.path.basename(train_result.path)

    # ── Load datasets as pandas DataFrames ───────────────────────────────────
    # Both S3Sink and LocalFileSink require pandas DataFrames.
    pr.train_data.load_pandas_dataframe()
    pr.validation_data.load_pandas_dataframe()

    # ── Storage backend ───────────────────────────────────────────────────────
    # AWS_ENDPOINT_URL set → MinIO / S3-compatible remote storage.
    # Unset → local temp directory (development and CI).
    #
    # To use a different backend (GCS, Azure Blob, HDFS, …), subclass
    # StorageBackend and implement upload() / download():
    #
    #   from michelangelo.lib.artifact_manager.storage_backend import StorageBackend
    #
    #   class GCSStorageBackend(StorageBackend):
    #       def upload(self, local_path: str, destination_key: str) -> str: ...
    #       def download(self, uri: str, local_path: str) -> None: ...
    if endpoint:
        # Bucket: AWS_S3_BUCKET → parsed from MA_FILE_SYSTEM/UF_STORAGE_URL.
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
            "push_step: using MinioStorageBackend (remote) → %s",
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

        _local_dir = tempfile.mkdtemp(prefix="california_push_")
        storage_backend = LocalStorageBackend(_local_dir)
        log.info(
            "push_step: using LocalStorageBackend (local/CI) → %s",
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

    # ── Registry client ───────────────────────────────────────────────────────
    # REGISTRY_ENDPOINT → APIRegistryClient (remote); else InMemoryRegistryClient.
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
            caller="california-housing-push-step",
            channel=_channel,
        )
        registry_client = APIRegistryClient(
            svc=_api_client.ModelService,
            namespace=os.environ.get(
                "REGISTRY_NAMESPACE", os.environ.get("MA_NAMESPACE", "default")
            ),
        )
        log.info("push_step: using APIRegistryClient at %s", registry_endpoint)
    else:
        from michelangelo.lib.model_manager.registry.client import (
            InMemoryRegistryClient,
        )

        registry_client = InMemoryRegistryClient()
        log.warning(
            "REGISTRY_ENDPOINT not set — using InMemoryRegistryClient. "
            "Model registration will not be persisted."
        )

    # ── Pusher config ─────────────────────────────────────────────────────────
    from michelangelo.gen.api.v2.evaluation_report_pb2 import (
        EvaluationReport,
        EvaluationReportSpec,
    )

    metrics = {k: round(v, 4) for k, v in (train_result.metrics or {}).items()}
    config = PusherConfig(
        items=[
            PusherPluginConfig(
                name="model",
                model_plugin=ModelPluginConfig(
                    model_name="california-housing-xgb",
                    description="XGBoost regression on California Housing dataset",
                    labels={"framework": "xgboost"},
                    metadata=metrics,
                ),
            ),
            PusherPluginConfig(
                name="eval_report",
                eval_report_plugin=EvalReportPluginConfig(
                    report_name="california-housing-xgb-eval",
                    extra_fields=metrics,
                ),
            ),
            PusherPluginConfig(
                name="train_data",
                dataset_plugin=_dataset_config(
                    f"datasets/california-housing/{_run_id}/train"
                ),
            ),
            PusherPluginConfig(
                name="validation_data",
                dataset_plugin=_dataset_config(
                    f"datasets/california-housing/{_run_id}/validation"
                ),
            ),
        ]
    )

    assembled = AssembledModel(raw_model=ModelArtifact(path=checkpoint_path))
    eval_report = EvaluationReport(
        spec=EvaluationReportSpec(title="California Housing XGBoost Evaluation")
    )

    results = push(
        config=config,
        artifacts={
            "model": assembled,
            "eval_report": eval_report,
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
