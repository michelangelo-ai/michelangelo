"""Pusher step for the BERT CoLA fine-tuning workflow.

Registers the assembled model (produced by ``package_model``) in a model
registry via ``ModelPusherPlugin``. Constructs its own storage backend and
registry client, independently of ``package.py`` -- uniflow tasks can't
share live objects across the workflow boundary (same reasoning documented
in ``examples.pipelines.california_housing_xgb.push.push_step``).
"""

from __future__ import annotations

import logging
import os
from urllib.parse import urlparse

import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask
from michelangelo.workflow.schema.pusher import (
    ModelPluginConfig,
    PusherConfig,
    PusherPluginConfig,
)
from michelangelo.workflow.tasks.pusher import push

# Kept top-level (not TYPE_CHECKING) despite only being used in annotations --
# uniflow's @uniflow.task needs these resolvable as real objects at the
# workflow boundary, matching
# examples.pipelines.california_housing_xgb.push.push_step's same choice.
from michelangelo.workflow.variables.types import (  # noqa: TC001
    AssembledModel,
    PusherResult,
)

log = logging.getLogger(__name__)

__all__ = ["push_step"]


@uniflow.task(
    config=RayTask(head_cpu=1, head_memory="1Gi", worker_instances=0),
)
def push_step(assembled: AssembledModel) -> list[PusherResult]:
    """Push the assembled BERT CoLA model to storage and the model registry.

    Args:
        assembled: Result of ``package_model`` -- deployable and raw Triton
            packages for the fine-tuned classifier.

    Returns:
        List of ``PusherResult``, one per artifact pushed (just ``model`` here).
    """
    s3_endpoint = os.environ.get("AWS_ENDPOINT_URL", "")
    if s3_endpoint:
        parsed = urlparse(s3_endpoint)
        endpoint = parsed.netloc
        if not endpoint:
            raise ValueError(
                f"AWS_ENDPOINT_URL={s3_endpoint!r} is missing a scheme. "
                "Use a full URL like http://minio:9091"
            )
        bucket = (
            os.environ.get("AWS_S3_BUCKET")
            or os.environ.get("MA_FILE_SYSTEM", "s3://default")
            .removeprefix("s3://")
            .split("/")[0]
        )
        from michelangelo.lib.artifact_manager.minio_backend import MinioStorageBackend

        storage_backend = MinioStorageBackend(
            endpoint=endpoint,
            bucket=bucket,
            access_key=os.environ.get("AWS_ACCESS_KEY_ID", ""),
            secret_key=os.environ.get("AWS_SECRET_ACCESS_KEY", ""),
            secure=parsed.scheme == "https",
            create_bucket_if_missing=True,
        )
        log.info(
            "push_step: using MinioStorageBackend (remote) -> %s",
            storage_backend.get_storage_location(),
        )
    else:
        import tempfile

        from michelangelo.lib.artifact_manager.storage_backend import (
            LocalStorageBackend,
        )

        local_dir = tempfile.mkdtemp(prefix="bert_cola_push_")
        storage_backend = LocalStorageBackend(local_dir)
        log.info("push_step: using LocalStorageBackend (local/CI) -> %s", local_dir)

    registry_endpoint = os.environ.get("REGISTRY_ENDPOINT")
    if registry_endpoint:
        import grpc as _grpc

        from michelangelo.api.v2 import APIClient
        from michelangelo.lib.model_manager.registry.api_client import (
            APIRegistryClient,
        )

        _insecure = os.environ.get("REGISTRY_INSECURE", "true").lower() != "false"
        _credentials = None if _insecure else _grpc.ssl_channel_credentials()
        _channel = (
            _grpc.insecure_channel(registry_endpoint)
            if _insecure
            else _grpc.secure_channel(registry_endpoint, _credentials)
        )
        _api_client = APIClient(caller="bert-cola-push-step", channel=_channel)
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
            "REGISTRY_ENDPOINT not set -- using InMemoryRegistryClient. "
            "Model registration will not be persisted."
        )

    config = PusherConfig(
        items=[
            PusherPluginConfig(
                name="model",
                model_plugin=ModelPluginConfig(
                    model_name="bert-cola",
                    description="BERT fine-tuned for CoLA linguistic acceptability",
                    labels={"framework": "transformers"},
                ),
            ),
        ]
    )

    results = push(
        config=config,
        artifacts={"model": assembled},
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
