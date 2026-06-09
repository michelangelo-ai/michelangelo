"""Model push task for the California Housing XGBoost workflow.

Locates the trained XGBoost checkpoint and pushes the model and an evaluation
report to storage and a model registry via the pusher plugin.
"""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask
from michelangelo.workflow.schema.pusher import (
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
    from examples.california_housing_xgb.train import TrainResult

log = logging.getLogger(__name__)

__all__ = ["push_model"]


@uniflow.task(
    config=RayTask(
        head_cpu=1,
        head_memory="4Gi",
        worker_cpu=1,
        worker_memory="4Gi",
        worker_instances=0,
    ),
)
def push_model(train_result: TrainResult) -> list[PusherResult]:
    """Push the trained XGBoost model and an eval report to storage and registry.

    All infrastructure (storage backend, registry client, pusher config) is
    constructed inside the task body. This is required by UniFlow: tasks run
    in isolated processes and the framework cannot serialize stateful objects
    (storage clients, gRPC channels) across the workflow→task boundary. Passing
    live objects as task arguments would raise a codec error at runtime.

    The XGBoost checkpoint produced by Ray under ``train_result.path`` is
    located via glob and wrapped in an ``AssembledModel``.

    Args:
        train_result: Result of the ``train`` task, holding the checkpoint
            path and training metrics.

    Returns:
        List of ``PusherResult``, one per configured artifact.
    """
    import glob
    import os
    import tempfile

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

    # Storage backend: MINIO_* env vars → MinIO artifact push destination.
    # This is separate from UF_STORAGE_URL (UniFlow's internal checkpoint
    # storage set by --storage-url). Falls back to a local temp dir for
    # local and CI runs where no object store is available.
    #
    # To add another backend (S3, GCS, Azure Blob, …), subclass StorageBackend
    # and implement upload() / download():
    #
    #   from michelangelo.lib.artifact_manager.storage_backend import StorageBackend
    #
    #   class S3StorageBackend(StorageBackend):
    #       def upload(self, local_path: str, destination_key: str) -> str: ...
    #       def download(self, uri: str, local_path: str) -> None: ...
    #
    # Then pass your instance to push() in place of MinioStorageBackend.
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
        from michelangelo.lib.artifact_manager.minio_backend import (
            MinioStorageBackend,
        )

        storage_backend = MinioStorageBackend(
            endpoint=endpoint,
            bucket=os.environ["MINIO_BUCKET"],
            access_key=os.environ["MINIO_ACCESS_KEY"],
            secret_key=os.environ["MINIO_SECRET_KEY"],
            secure=os.environ.get("MINIO_SECURE", "true").lower() != "false",
        )
    else:
        from michelangelo.lib.artifact_manager.storage_backend import (
            LocalStorageBackend,
        )

        storage_backend = LocalStorageBackend(
            tempfile.mkdtemp(prefix="california_push_")
        )

    # Registry client: REGISTRY_ENDPOINT → APIRegistryClient (remote run);
    # else InMemoryRegistryClient (local run — registrations are not persisted).
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
        log.info("Using APIRegistryClient at %s", registry_endpoint)
    else:
        from michelangelo.lib.model_manager.registry.client import (
            InMemoryRegistryClient,
        )

        registry_client = InMemoryRegistryClient()
        log.warning(
            "REGISTRY_ENDPOINT not set — using InMemoryRegistryClient. "
            "Model registration will not be persisted."
        )

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
        ]
    )

    assembled = AssembledModel(raw_model=ModelArtifact(path=checkpoint_path))
    eval_report = EvaluationReport(
        spec=EvaluationReportSpec(title="California Housing XGBoost Evaluation")
    )

    results = push(
        config=config,
        artifacts={"model": assembled, "eval_report": eval_report},
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
