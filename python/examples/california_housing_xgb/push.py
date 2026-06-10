"""Pusher step for the California Housing XGBoost workflow.

Pushes all pipeline artifacts in a single Spark task: trained XGBoost model,
evaluation report, and preprocessed train/validation datasets. Datasets are
written to Hive tables via HiveSink (Spark-native, no toPandas() collection).
Storage backends for model and eval report are selected at runtime from
environment variables.
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
from michelangelo.workflow.schema.sinks import HiveSinkConfig
from michelangelo.workflow.tasks.functions.sinks import HiveSink
from michelangelo.workflow.tasks.pusher import push
from michelangelo.workflow.variables.types import (
    AssembledModel,
    ModelArtifact,
    PusherResult,
)

if TYPE_CHECKING:
    from examples.california_housing_xgb.preprocess import PreprocessResult
    from examples.california_housing_xgb.train import TrainResult

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

    Pushes four artifacts:

    - **model** — trained XGBoost checkpoint via ``ModelPusherPlugin``.
    - **eval_report** — training metrics via ``EvalReportPusherPlugin``.
    - **train_data** — preprocessed training dataset via ``DatasetPusherPlugin``
      + ``HiveSink`` (Spark-native ``saveAsTable``, no ``toPandas()`` collection).
    - **validation_data** — preprocessed validation dataset via ``DatasetPusherPlugin``
      + ``HiveSink``.

    All infrastructure (storage backend, registry client, sinks) is constructed
    inside the task body — required by the UniFlow codec boundary. Stateful
    objects cannot be serialised across the workflow→task boundary.

    Hive database is configured via ``HIVE_DATABASE`` (default:
    ``california_housing``). Storage backend for model / eval report:

    - **Remote** (``MINIO_ENDPOINT`` set): ``MinioStorageBackend``.
    - **Local** (default): ``LocalStorageBackend`` writing to a temp directory.

    Args:
        pr: Result of the ``preprocess`` task, holding preprocessed training
            and validation ``DatasetVariable`` handles (Spark DataFrames).
        train_result: Result of the ``train`` task, holding the XGBoost
            checkpoint path and training metrics.

    Returns:
        List of ``PusherResult``, one per artifact pushed.
    """
    import glob
    import os
    import tempfile

    # ── Locate XGBoost checkpoint ────────────────────────────────────────────
    checkpoint_glob = os.path.join(train_result.path, "**", "model.ubj")
    matches = glob.glob(checkpoint_glob, recursive=True)
    if not matches:
        # model.ubj is the default XGBoost binary checkpoint written by
        # XGBoostTrainer. Fall back to any non-directory file under the
        # checkpoint dir if Ray writes to a different name in future versions.
        matches = [
            p
            for p in glob.glob(
                os.path.join(train_result.path, "**", "*"), recursive=True
            )
            if os.path.isfile(p)
        ]
    if not matches:
        raise FileNotFoundError(f"No model checkpoint found under {train_result.path}")
    checkpoint_path = matches[0]
    log.info("Found model checkpoint: %s", checkpoint_path)

    # ── Load datasets as Spark DataFrames ─────────────────────────────────────
    # HiveSink requires pyspark.sql.DataFrame — load_spark_dataframe() reads
    # the Parquet written by the preprocess task back into Spark.
    pr.train_data.load_spark_dataframe()
    pr.validation_data.load_spark_dataframe()

    # ── Hive sinks ────────────────────────────────────────────────────────────
    # Write preprocessed datasets to Hive tables via Spark saveAsTable.
    # Database is configurable via HIVE_DATABASE (default: california_housing).
    hive_db = os.environ.get("HIVE_DATABASE", "california_housing")
    log.info("push_step: writing datasets to Hive database '%s'", hive_db)

    def _dataset_config(table: str) -> DatasetPluginConfig:
        return DatasetPluginConfig(
            sinks=[HiveSink(HiveSinkConfig(database=hive_db, table=table))]
        )

    # ── Storage backend (model + eval report) ─────────────────────────────────
    # MINIO_* env vars → MinIO / S3-compatible (remote runs).
    # Unset → local temp directory (development and CI runs).
    # Separate from the UniFlow checkpoint store (configured via --storage-url).
    #
    # To use a different backend (GCS, Azure Blob, HDFS, …), subclass
    # StorageBackend and implement upload() / download():
    #
    #   from michelangelo.lib.artifact_manager.storage_backend import StorageBackend
    #
    #   class GCSStorageBackend(StorageBackend):
    #       def upload(self, local_path: str, destination_key: str) -> str: ...
    #       def download(self, uri: str, local_path: str) -> None: ...
    endpoint = os.environ.get("MINIO_ENDPOINT")
    if endpoint:
        _required_minio = ("MINIO_BUCKET", "MINIO_ACCESS_KEY", "MINIO_SECRET_KEY")
        missing = [k for k in _required_minio if k not in os.environ]
        if missing:
            raise OSError(
                f"MINIO_ENDPOINT is set but required variables are missing: {missing}. "
                "Set all MINIO_* variables or unset MINIO_ENDPOINT "
                "to use local storage."
            )
        from michelangelo.lib.artifact_manager.minio_backend import MinioStorageBackend

        bucket = os.environ["MINIO_BUCKET"]
        storage_backend = MinioStorageBackend(
            endpoint=endpoint,
            bucket=bucket,
            access_key=os.environ["MINIO_ACCESS_KEY"],
            secret_key=os.environ["MINIO_SECRET_KEY"],
            secure=os.environ.get("MINIO_SECURE", "true").lower() != "false",
            create_bucket_if_missing=True,
        )
        log.info(
            "push_step: using MinioStorageBackend (remote) → %s",
            storage_backend.get_storage_location(),
        )
    else:
        from michelangelo.lib.artifact_manager.storage_backend import (
            LocalStorageBackend,
        )

        _local_dir = tempfile.mkdtemp(prefix="california_push_")
        storage_backend = LocalStorageBackend(_local_dir)
        log.info(
            "push_step: using LocalStorageBackend (local/CI) → %s",
            storage_backend.get_storage_location(),
        )

    # ── Registry client ───────────────────────────────────────────────────────
    # REGISTRY_ENDPOINT → APIRegistryClient (remote); else InMemoryRegistryClient.
    registry_endpoint = os.environ.get("REGISTRY_ENDPOINT")
    if registry_endpoint:
        from michelangelo.lib.model_manager.registry.api_client import (
            APIRegistryClient,
        )

        registry_client = APIRegistryClient(
            endpoint=registry_endpoint,
            namespace=os.environ.get("REGISTRY_NAMESPACE", ""),
            insecure=os.environ.get("REGISTRY_INSECURE", "true").lower() != "false",
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
                dataset_plugin=_dataset_config("train_data"),
            ),
            PusherPluginConfig(
                name="validation_data",
                dataset_plugin=_dataset_config("validation_data"),
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
