"""Michelangelo gRPC model service registry client.

.. note::
    **Internal adapter — requires a running Michelangelo ModelService.**
    This client imports ``michelangelo.gen.api.v2`` gRPC stubs that are generated
    from Michelangelo's internal protobuf definitions. It is intended for use
    within a Michelangelo deployment. External adopters using a different model
    registry should implement
    :class:`~michelangelo.lib.model_manager.registry.client.ModelRegistryClient`
    directly (e.g.
    :class:`~michelangelo.lib.model_manager.registry.client.InMemoryRegistryClient`
    for testing).

Implements :class:`~michelangelo.lib.model_manager.registry.client.ModelRegistryClient`
by calling Michelangelo's ``ModelService`` gRPC API. Works against any running
``ModelService`` endpoint — a local sandbox API server (``insecure=True``) or
a production cluster (``insecure=False`` with TLS).

The ``grpcio`` package is a required dependency and is always available.

Typical usage::

    from michelangelo.lib.model_manager.registry.api_client import APIRegistryClient

    with APIRegistryClient(endpoint="localhost:50051", namespace="sandbox") as client:
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
from michelangelo.lib.exceptions import ConfigurationError
from michelangelo.lib.model_manager.registry.client import (
    ModelRegistryClient,
    RegisteredModel,
)

if TYPE_CHECKING:
    from collections.abc import Mapping

_logger = logging.getLogger(__name__)

METADATA_ANNOTATION_KEY = "michelangelo.io/metadata"
"""Annotation key under which free-form metadata is JSON-serialised
in the ModelService API."""

_MAX_REGISTER_RETRIES = 3

__all__ = ["METADATA_ANNOTATION_KEY", "APIRegistryClient"]


class APIRegistryClient(ModelRegistryClient):
    """ModelRegistryClient backed by Michelangelo's gRPC ``ModelService`` API.

    .. note::
        This client depends on ``michelangelo.gen.api.v2`` gRPC stubs generated
        from Michelangelo's internal protobuf definitions. It requires a running
        ``ModelService`` endpoint. External adopters should implement
        :class:`ModelRegistryClient` directly rather than using this class.

    Connects to any running ``ModelService`` endpoint. For local sandbox use,
    point ``endpoint`` at a locally running API server with ``insecure=True``.
    For production, set ``insecure=False`` and provide a TLS-enabled endpoint.

    **Create vs. update (with retry):** :meth:`register_model` attempts
    ``CreateModel`` first. If the server responds with ``ALREADY_EXISTS``, the
    client fetches the current ``resourceVersion`` and calls ``UpdateModel``
    instead. If ``UpdateModel`` fails with ``FAILED_PRECONDITION`` (a concurrent
    writer updated the resource between our ``GetModel`` and ``UpdateModel``),
    the whole sequence is retried up to :data:`_MAX_REGISTER_RETRIES` times.

    **registry_uri format:** The ``registry_uri`` field of the returned
    :class:`RegisteredModel` uses the Michelangelo-internal format
    ``models:/{namespace}/{name}/{version}`` — *not* the MLflow two-segment
    format ``models:/{name}/{version}``. Do not pass this URI to MLflow
    client libraries.

    **Labels** are stored in ``model.metadata.labels`` (indexed, filterable
    string key-value pairs). **Metadata** is JSON-serialised and stored under
    the annotation key ``michelangelo.io/metadata``.

    **Channel lifecycle:** The underlying gRPC channel holds native threads and
    TCP connections. Call :meth:`close` when done, or use the client as a
    context manager (``with APIRegistryClient(...) as client:``).

    Args:
        endpoint: gRPC server address without the scheme
            (e.g. ``"localhost:50051"`` or ``"api.michelangelo.io:443"``).
        namespace: Kubernetes namespace used for model resources. Leave empty
            to use the server's default namespace.
        insecure: Use a plaintext gRPC channel (no TLS). Set ``True`` for a
            local sandbox API server, ``False`` for any TLS-protected endpoint.
        timeout_seconds: Per-call deadline in seconds.

    Raises:
        ConfigurationError: If ``endpoint`` is empty.

    Example::

        from michelangelo.lib.model_manager.registry.api_client import APIRegistryClient

        with APIRegistryClient(
            endpoint="localhost:50051", namespace="sandbox"
        ) as client:
            reg = client.register_model(
                name="boston-xgb",
                artifact_uri="s3://bucket/models/boston-xgb/abc123/raw",
                deployable_artifact_uri=(
                    "s3://bucket/models/boston-xgb/abc123/deployable"
                ),
                description="XGBoost model trained on Boston housing data",
                labels={"training_framework": "xgboost"},
                metadata={"run_id": "mlflow-run-abc", "rmse": 2.41},
            )
            print(reg.version)       # "1"
            print(reg.registry_uri)  # "models:/sandbox/boston-xgb/1"
    """

    def __init__(
        self,
        endpoint: str,
        namespace: str = "",
        insecure: bool = True,
        timeout_seconds: int = 30,
    ) -> None:
        """Open a gRPC channel and create the ModelService stub.

        Args:
            endpoint: gRPC server address without the scheme.
            namespace: Kubernetes namespace for model resources.
            insecure: Use plaintext gRPC (no TLS). Defaults to ``True``.
            timeout_seconds: Per-call deadline in seconds. Defaults to ``30``.

        Raises:
            ConfigurationError: If ``endpoint`` is empty.
        """
        if not endpoint:
            raise ConfigurationError(
                "APIRegistryClient endpoint must be non-empty. "
                "Provide the gRPC server address, e.g. 'localhost:50051'."
            )
        self._namespace = namespace
        self._timeout_seconds = timeout_seconds
        if insecure:
            channel = grpc.insecure_channel(endpoint)
        else:
            credentials = grpc.ssl_channel_credentials()
            channel = grpc.secure_channel(endpoint, credentials)
        self._channel = channel
        self._stub = ModelServiceStub(channel)

    def close(self) -> None:
        """Close the underlying gRPC channel, releasing threads and connections."""
        self._channel.close()

    def __enter__(self) -> APIRegistryClient:
        """Enter the context manager, returning self."""
        return self

    def __exit__(self, *_: object) -> None:
        """Exit the context manager, closing the gRPC channel."""
        self.close()

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
        as ``UpdateModel``. If a concurrent writer causes ``UpdateModel`` to
        return ``FAILED_PRECONDITION``, the whole sequence is retried up to
        :data:`_MAX_REGISTER_RETRIES` times.

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
                ``ALREADY_EXISTS`` / ``FAILED_PRECONDITION`` handled by the
                retry loop.
            RuntimeError: If all :data:`_MAX_REGISTER_RETRIES` attempts are
                exhausted due to repeated concurrent writes.
        """
        for attempt in range(1, _MAX_REGISTER_RETRIES + 1):
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
                    timeout=self._timeout_seconds,
                )
                return self._to_registered_model(resp.model)
            except grpc.RpcError as exc:
                if exc.code() != grpc.StatusCode.ALREADY_EXISTS:
                    raise

            # ALREADY_EXISTS — fetch resourceVersion and update instead
            _logger.warning(
                "Model '%s' already exists — fetching resourceVersion and "
                "updating (attempt %d/%d).",
                name,
                attempt,
                _MAX_REGISTER_RETRIES,
            )
            get_resp = self._stub.GetModel(
                model_svc_pb2.GetModelRequest(
                    name=name,
                    namespace=self._namespace,
                ),
                timeout=self._timeout_seconds,
            )
            model.metadata.resourceVersion = get_resp.model.metadata.resourceVersion
            try:
                upd_resp = self._stub.UpdateModel(
                    model_svc_pb2.UpdateModelRequest(model=model),
                    timeout=self._timeout_seconds,
                )
                return self._to_registered_model(upd_resp.model)
            except grpc.RpcError as upd_exc:
                if upd_exc.code() == grpc.StatusCode.FAILED_PRECONDITION:
                    if attempt < _MAX_REGISTER_RETRIES:
                        _logger.warning(
                            "UpdateModel for '%s' hit FAILED_PRECONDITION — "
                            "concurrent write detected, retrying (%d/%d).",
                            name,
                            attempt,
                            _MAX_REGISTER_RETRIES,
                        )
                        continue
                    raise RuntimeError(
                        f"register_model: exhausted {_MAX_REGISTER_RETRIES} retries "
                        f"for {name!r} due to repeated concurrent writes. "
                        "Call register_model() again to retry."
                    ) from upd_exc
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
                namespace=self._namespace,
            ),
            timeout=self._timeout_seconds,
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
        if self._namespace:
            model.metadata.namespace = self._namespace

        model.spec.model_artifact_uri.append(artifact_uri)
        if deployable_artifact_uri:
            model.spec.deployable_artifact_uri.append(deployable_artifact_uri)
        if description:
            model.spec.description = description
        for k, v in (labels or {}).items():
            model.metadata.labels[k] = v
        if metadata:
            model.metadata.annotations[METADATA_ANNOTATION_KEY] = json.dumps(
                dict(metadata)
            )
        return model

    def _to_registered_model(self, model: model_pb2.Model) -> RegisteredModel:
        """Map a ``Model`` proto response to a :class:`RegisteredModel`."""
        name = model.metadata.name
        namespace = model.metadata.namespace or self._namespace
        version = str(model.spec.revision_id)

        artifact_uri = (
            model.spec.model_artifact_uri[0] if model.spec.model_artifact_uri else None
        )
        deployable_artifact_uri = (
            model.spec.deployable_artifact_uri[0]
            if model.spec.deployable_artifact_uri
            else None
        )
        labels = dict(model.metadata.labels)

        metadata_str = dict(model.metadata.annotations).get(METADATA_ANNOTATION_KEY)
        if metadata_str:
            try:
                metadata: dict[str, Any] = json.loads(metadata_str)
            except json.JSONDecodeError as exc:
                raise ValueError(
                    f"Model {name!r}: annotation {METADATA_ANNOTATION_KEY!r} "
                    f"contains invalid JSON: {exc}"
                ) from exc
        else:
            metadata = {}

        # registry_uri uses Michelangelo's internal three-segment format
        # "models:/{namespace}/{name}/{version}" — not the two-segment MLflow format.
        return RegisteredModel(
            name=name,
            version=version,
            registry_uri=f"models:/{namespace}/{name}/{version}",
            artifact_uri=artifact_uri,
            deployable_artifact_uri=deployable_artifact_uri,
            labels=labels,
            metadata=metadata,
        )
