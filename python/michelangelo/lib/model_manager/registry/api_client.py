"""Michelangelo gRPC model service registry client.

Implements :class:`~michelangelo.lib.model_manager.registry.client.ModelRegistryClient`
by calling Michelangelo's ``ModelService`` gRPC API. Works against any running
``ModelService`` endpoint — a local sandbox API server (``insecure=True``) or
a production cluster (``insecure=False`` with TLS).

The ``grpcio`` package is a required dependency and is always available.

Typical usage::

    from michelangelo.lib.model_manager.registry.api_client import APIRegistryClient
    from michelangelo.lib.model_manager.registry.schema.api import APIRegistryConfig

    client = APIRegistryClient(APIRegistryConfig(
        endpoint="localhost:50051",
        namespace="sandbox",
    ))
    registered = client.register_model(
        name="my-classifier",
        artifact_uri="s3://bucket/models/my-classifier/abc123/raw",
        labels={"training_framework": "xgboost"},
        metadata={"run_id": "mlflow-run-abc"},
    )
    print(registered.version, registered.registry_uri)
"""

from __future__ import annotations

import json
import logging
from typing import TYPE_CHECKING, Any

import grpc

from michelangelo.gen.api.v2 import model_pb2, model_svc_pb2
from michelangelo.gen.api.v2.model_svc_pb2_grpc import ModelServiceStub
from michelangelo.lib.model_manager.registry.client import (
    ModelRegistryClient,
    RegisteredModel,
)

if TYPE_CHECKING:
    from collections.abc import Mapping

    from michelangelo.lib.model_manager.registry.schema.api import APIRegistryConfig

_logger = logging.getLogger(__name__)
_METADATA_ANNOTATION_KEY = "michelangelo.io/metadata"

__all__ = ["APIRegistryClient"]


class APIRegistryClient(ModelRegistryClient):
    """ModelRegistryClient backed by Michelangelo's gRPC ``ModelService`` API.

    Connects to any running ``ModelService`` endpoint. For local sandbox use,
    point ``endpoint`` at a locally running API server with ``insecure=True``.
    For production, set ``insecure=False`` and provide a TLS-enabled endpoint.

    **Create vs. update:** :meth:`register_model` attempts ``CreateModel``
    first. If the server responds with ``ALREADY_EXISTS`` (the model name is
    already registered), the client fetches the current ``resourceVersion``
    and calls ``UpdateModel`` instead — matching the Kubernetes optimistic
    concurrency pattern used internally.

    **Labels** are stored in ``model.metadata.labels`` (indexed, filterable
    string key-value pairs). **Metadata** is JSON-serialised and stored under
    the annotation key ``michelangelo.io/metadata``.

    Args:
        config: :class:`APIRegistryConfig
            <michelangelo.lib.model_manager.registry.schema.api.APIRegistryConfig>`
            holding endpoint, namespace, TLS, and timeout settings.

    Example::

        from michelangelo.lib.model_manager.registry.api_client import APIRegistryClient
        from michelangelo.lib.model_manager.registry.schema.api import APIRegistryConfig

        client = APIRegistryClient(APIRegistryConfig(
            endpoint="localhost:50051",
            namespace="sandbox",
        ))
        reg = client.register_model(
            name="boston-xgb",
            artifact_uri="s3://bucket/models/boston-xgb/abc123/raw",
            deployable_artifact_uri="s3://bucket/models/boston-xgb/abc123/deployable",
            description="XGBoost model trained on Boston housing data",
            labels={"training_framework": "xgboost"},
            metadata={"run_id": "mlflow-run-abc", "rmse": 2.41},
        )
        print(reg.version)       # "1"
        print(reg.registry_uri)  # "models:/sandbox/boston-xgb/1"
    """

    def __init__(self, config: APIRegistryConfig) -> None:
        """Open a gRPC channel and create the ModelService stub.

        Args:
            config: Connection and namespace configuration.
        """
        self._config = config
        if config.insecure:
            channel = grpc.insecure_channel(config.endpoint)
        else:
            credentials = grpc.ssl_channel_credentials()
            channel = grpc.secure_channel(config.endpoint, credentials)
        self._stub = ModelServiceStub(channel)

    def register_model(
        self,
        name: str,
        artifact_uri: str,
        deployable_artifact_uri: str | None = None,
        description: str | None = None,
        schema: dict[str, Any] | None = None,
        labels: Mapping[str, str] | None = None,
        metadata: Mapping[str, Any] | None = None,
    ) -> RegisteredModel:
        """Register a model via ``CreateModel``, falling back to ``UpdateModel``.

        Builds a ``Model`` CRD proto from the supplied arguments and calls
        ``CreateModel``. If the server returns ``ALREADY_EXISTS``, the current
        ``resourceVersion`` is fetched via ``GetModel`` and the call is retried
        as ``UpdateModel`` (Kubernetes optimistic concurrency).

        Args:
            name: Model name to register. Used as ``model.metadata.name``.
            artifact_uri: URI of the raw model artifact (e.g. an S3 URI
                returned by ``StorageBackend.upload()``). Stored in
                ``spec.model_artifact_uri``.
            deployable_artifact_uri: Optional URI of the serving-ready bundle.
                Stored in ``spec.deployable_artifact_uri`` when provided.
            description: Optional human-readable description stored in
                ``spec.description``.
            schema: Ignored. The ``ModelService`` API does not expose a
                dedicated schema field; subclasses may override this method to
                embed schema in ``spec.input_schema`` / ``spec.output_schema``.
            labels: String key-value pairs stored in ``model.metadata.labels``
                (indexed and filterable via the API).
            metadata: Arbitrary JSON-serializable key-value pairs stored as the
                annotation ``michelangelo.io/metadata``.

        Returns:
            A :class:`~michelangelo.lib.model_manager.registry.client.RegisteredModel`
            built from the service response.

        Raises:
            grpc.RpcError: If the gRPC call fails for any reason other than
                ``ALREADY_EXISTS``.
            OSError: If the gRPC channel cannot reach the endpoint.
        """
        model = self._build_model_proto(
            name=name,
            artifact_uri=artifact_uri,
            deployable_artifact_uri=deployable_artifact_uri,
            description=description,
            labels=labels,
            metadata=metadata,
        )
        try:
            _logger.info("Calling CreateModel for '%s'.", name)
            resp = self._stub.CreateModel(
                model_svc_pb2.CreateModelRequest(model=model),
                timeout=self._config.timeout_seconds,
            )
            return self._to_registered_model(resp.model)
        except grpc.RpcError as exc:
            if exc.code() == grpc.StatusCode.ALREADY_EXISTS:
                _logger.info(
                    "Model '%s' already exists — fetching resourceVersion "
                    "and updating.",
                    name,
                )
                get_resp = self._stub.GetModel(
                    model_svc_pb2.GetModelRequest(
                        name=name,
                        namespace=self._config.namespace,
                    ),
                    timeout=self._config.timeout_seconds,
                )
                model.metadata.resourceVersion = get_resp.model.metadata.resourceVersion
                upd_resp = self._stub.UpdateModel(
                    model_svc_pb2.UpdateModelRequest(model=model),
                    timeout=self._config.timeout_seconds,
                )
                return self._to_registered_model(upd_resp.model)
            raise

    def get_model(self, name: str, version: str | None = None) -> RegisteredModel:
        """Retrieve the latest model registration by name.

        .. note::
            The ``ModelService`` API does not support per-revision lookup — it
            always returns the current (latest) model record. Passing a
            non-``None`` ``version`` emits a warning and the latest revision is
            returned regardless.

        Args:
            name: Model name to look up.
            version: If provided, a warning is emitted because per-revision
                lookup is not supported by the ``ModelService`` API. The latest
                revision is returned in all cases.

        Returns:
            A :class:`~michelangelo.lib.model_manager.registry.client.RegisteredModel`
            built from the service response.

        Raises:
            grpc.RpcError: If the model is not found or the call fails.
        """
        if version is not None:
            _logger.warning(
                "APIRegistryClient.get_model() does not support per-revision "
                "lookup (requested version=%r for model '%s'). "
                "The ModelService API always returns the latest revision.",
                version,
                name,
            )
        _logger.info("Calling GetModel for '%s'.", name)
        resp = self._stub.GetModel(
            model_svc_pb2.GetModelRequest(
                name=name,
                namespace=self._config.namespace,
            ),
            timeout=self._config.timeout_seconds,
        )
        return self._to_registered_model(resp.model)

    # ── Private helpers ──────────────────────────────────────────────────────

    def _build_model_proto(
        self,
        name: str,
        artifact_uri: str,
        deployable_artifact_uri: str | None,
        description: str | None,
        labels: Mapping[str, str] | None,
        metadata: Mapping[str, Any] | None,
    ) -> model_pb2.Model:
        """Construct a ``Model`` CRD proto from registration arguments."""
        model = model_pb2.Model()
        model.metadata.name = name
        if self._config.namespace:
            model.metadata.namespace = self._config.namespace

        model.spec.model_artifact_uri.append(artifact_uri)
        if deployable_artifact_uri:
            model.spec.deployable_artifact_uri.append(deployable_artifact_uri)
        if description:
            model.spec.description = description
        for k, v in (labels or {}).items():
            model.metadata.labels[k] = v
        if metadata:
            model.metadata.annotations[_METADATA_ANNOTATION_KEY] = json.dumps(
                dict(metadata)
            )
        return model

    def _to_registered_model(self, model: model_pb2.Model) -> RegisteredModel:
        """Map a ``Model`` proto response to a :class:`RegisteredModel`."""
        name = model.metadata.name
        namespace = model.metadata.namespace or self._config.namespace
        version = str(model.spec.revision_id)

        artifact_uri = (
            model.spec.model_artifact_uri[0]
            if model.spec.model_artifact_uri
            else None
        )
        deployable_artifact_uri = (
            model.spec.deployable_artifact_uri[0]
            if model.spec.deployable_artifact_uri
            else None
        )
        labels = dict(model.metadata.labels)

        metadata_str = dict(model.metadata.annotations).get(_METADATA_ANNOTATION_KEY)
        metadata: dict[str, Any] = json.loads(metadata_str) if metadata_str else {}

        return RegisteredModel(
            name=name,
            version=version,
            registry_uri=f"models:/{namespace}/{name}/{version}",
            artifact_uri=artifact_uri,
            deployable_artifact_uri=deployable_artifact_uri,
            labels=labels,
            metadata=metadata,
        )
