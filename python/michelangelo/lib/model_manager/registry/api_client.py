"""ModelRegistryClient backed by ``APIClient.ModelService``.

Delegates all gRPC calls to the shared ``APIClient`` channel, which manages
connection lifecycle and automatically injects the YARPC transport headers
(``rpc-caller``, ``rpc-service``, ``rpc-encoding``) required by the
Michelangelo apiserver via ``DefaultHeaderProvider``.

Typical usage::

    from michelangelo.api.v2 import APIClient
    from michelangelo.lib.model_manager.registry.api_client import APIRegistryClient

    # One APIClient per process — it manages the shared gRPC channel.
    client = APIClient(endpoint="localhost:15566", caller="my-pipeline")
    registry = APIRegistryClient(svc=client.ModelService, namespace="default")
    registered = registry.register_model(
        name="my-classifier",
        artifact_uri="s3://bucket/models/my-classifier/abc123/raw/model.ubj",
        labels={"framework": "xgboost"},
        metadata={"rmse": 0.87},
    )
    print(registered.version, registered.registry_uri)

To target the singleton channel (``MA_API_SERVER`` env var)::

    import os
    os.environ["MA_API_SERVER"] = "localhost:15566"

    from michelangelo.api.v2 import APIClient
    APIClient.set_caller("my-pipeline")

    from michelangelo.lib.model_manager.registry.api_client import APIRegistryClient
    registry = APIRegistryClient(namespace="default")
    registered = registry.register_model("my-model", "s3://bucket/raw/model.ubj")

For testing without a real server, use
:class:`~michelangelo.lib.model_manager.registry.client.InMemoryRegistryClient`.
"""

from __future__ import annotations

import json
import logging
import re
from typing import TYPE_CHECKING, Any

import grpc

from michelangelo.gen.api.v2 import model_pb2
from michelangelo.lib.model_manager.registry.client import (
    ModelRegistryClient,
    RegisteredModel,
)

if TYPE_CHECKING:
    from collections.abc import Mapping

    from michelangelo.api.v2.services.gen.model import ModelService as _ModelServiceType

_logger = logging.getLogger(__name__)

METADATA_ANNOTATION_KEY = "michelangelo.io/metadata"
"""Annotation key under which free-form metadata is JSON-serialised."""

_MAX_REGISTER_RETRIES = 3

_K8S_LABEL_VALUE_MAX_LENGTH = 63
_K8S_LABEL_VALUE_RE = re.compile(r"^(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?$")


def _is_valid_k8s_label_value(value: str) -> bool:
    """Return ``True`` if *value* satisfies Kubernetes' label-value rules.

    At most 63 characters, and either empty or starting/ending with an
    alphanumeric character with dashes, underscores, dots, and alphanumerics
    in between (see
    https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#syntax-and-character-set).
    """
    return (
        len(value) <= _K8S_LABEL_VALUE_MAX_LENGTH
        and _K8S_LABEL_VALUE_RE.match(value) is not None
    )


__all__ = ["METADATA_ANNOTATION_KEY", "APIRegistryClient"]


class APIRegistryClient(ModelRegistryClient):
    """ModelRegistryClient that delegates to ``APIClient.ModelService``.

    Reuses the ``APIClient`` channel already managed by the calling process —
    no additional gRPC channel is opened or closed. ``APIClient`` injects the
    required YARPC transport headers (``rpc-caller``, ``rpc-service``,
    ``rpc-encoding``) on every call via ``DefaultHeaderProvider``, eliminating
    the need for a manual interceptor.

    **Create vs. update (with retry):** :meth:`register_model` attempts
    ``create_model`` first. On ``ALREADY_EXISTS`` it fetches ``resourceVersion``
    via ``get_model`` and retries as ``update_model``. On ``FAILED_PRECONDITION``
    (concurrent write) the whole sequence retries up to
    :data:`_MAX_REGISTER_RETRIES` times.

    **registry_uri format:** ``models:/{namespace}/{name}/{version}`` —
    Michelangelo's three-segment format, not the two-segment MLflow format.

    Args:
        svc: Pre-built ``ModelService`` instance (e.g. ``client.ModelService``).
            When ``None``, the singleton ``APIClient.ModelService`` is used —
            requires ``MA_API_SERVER`` to be set in the environment.
        namespace: Kubernetes namespace for model resources. Defaults to
            ``"default"``.

    Raises:
        RuntimeError: If ``svc`` is ``None`` and ``APIClient.ModelService`` has
            not been initialised (``MA_API_SERVER`` not set).

    Example::

        from michelangelo.api.v2 import APIClient
        from michelangelo.lib.model_manager.registry.api_client import APIRegistryClient

        client = APIClient(endpoint="apiserver:15566", caller="push-step")
        registry = APIRegistryClient(svc=client.ModelService, namespace="my-project")
        reg = registry.register_model(
            name="california-housing-xgb",
            artifact_uri="s3://bucket/models/california-housing-xgb/abc/raw/model.ubj",
            labels={"framework": "xgboost"},
            metadata={"validation-rmse": 0.876},
        )
        print(reg.registry_uri)  # "models:/my-project/california-housing-xgb/1"
    """

    def __init__(
        self,
        svc: _ModelServiceType | None = None,
        namespace: str = "default",
    ) -> None:
        """Bind to a ``ModelService`` instance.

        Args:
            svc: Optional pre-built ``ModelService``. When ``None``, taken from
                ``APIClient.ModelService`` (requires ``MA_API_SERVER``).
            namespace: Kubernetes namespace for model resources.

        Raises:
            RuntimeError: If ``svc`` is ``None`` and ``APIClient.ModelService``
                is not initialised.
        """
        if svc is not None:
            self._svc = svc
        else:
            from michelangelo.api.v2 import APIClient

            self._svc = APIClient.ModelService
            if self._svc is None:
                raise RuntimeError(
                    "APIClient.ModelService is not initialised. "
                    "Set MA_API_SERVER in the environment before constructing "
                    "APIRegistryClient, or pass an explicit svc= argument."
                )
        self._namespace = namespace

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
        """Register a model via ``create_model``, falling back to ``update_model``.

        Args:
            name: Model name. Used as ``model.metadata.name``.
            artifact_uri: URI of the raw model artifact.
            deployable_artifact_uri: Optional URI of the serving-ready bundle.
            description: Optional human-readable description.
            schema: Ignored — ``ModelService`` has no dedicated schema field.
            labels: String key-value pairs stored in ``model.metadata.labels``.
            metadata: Arbitrary JSON-serializable key-value pairs stored under
                the annotation ``michelangelo.io/metadata``.

        Returns:
            :class:`~michelangelo.lib.model_manager.registry.client.RegisteredModel`

        Raises:
            grpc.RpcError: On gRPC errors other than ``ALREADY_EXISTS`` /
                ``FAILED_PRECONDITION`` handled by the retry loop.
            RuntimeError: After exhausting :data:`_MAX_REGISTER_RETRIES` retries.
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
                _logger.info("Calling create_model for '%s'.", name)
                created = self._svc.create_model(model)
                return self._to_registered_model(created)
            except grpc.RpcError as exc:
                if exc.code() != grpc.StatusCode.ALREADY_EXISTS:
                    raise

            _logger.warning(
                "Model '%s' already exists — fetching resourceVersion and "
                "updating (attempt %d/%d).",
                name,
                attempt,
                _MAX_REGISTER_RETRIES,
            )
            existing = self._svc.get_model(self._namespace, name)
            model.metadata.resourceVersion = existing.metadata.resourceVersion
            try:
                updated = self._svc.update_model(model)
                return self._to_registered_model(updated)
            except grpc.RpcError as upd_exc:
                if upd_exc.code() == grpc.StatusCode.FAILED_PRECONDITION:
                    if attempt < _MAX_REGISTER_RETRIES:
                        _logger.warning(
                            "update_model for '%s' hit FAILED_PRECONDITION — "
                            "concurrent write detected, retrying (%d/%d).",
                            name,
                            attempt,
                            _MAX_REGISTER_RETRIES,
                        )
                        continue
                    raise RuntimeError(
                        f"register_model: exhausted {_MAX_REGISTER_RETRIES} retries "
                        f"for {name!r} due to repeated concurrent writes."
                    ) from upd_exc
                raise

    def get_model(self, name: str, version: str | None = None) -> RegisteredModel:
        """Retrieve the latest model registration by name.

        Args:
            name: Model name to look up.
            version: Emits a warning if provided — ``ModelService`` always
                returns the latest revision.

        Returns:
            :class:`~michelangelo.lib.model_manager.registry.client.RegisteredModel`

        Raises:
            grpc.RpcError: If the model is not found or the call fails.
        """
        if version is not None:
            _logger.warning(
                "APIRegistryClient.get_model() does not support per-revision "
                "lookup (version=%r for '%s'). Returning the latest revision.",
                version,
                name,
            )
        _logger.info("Calling get_model for '%s'.", name)
        model = self._svc.get_model(self._namespace, name)
        return self._to_registered_model(model)

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
        model = model_pb2.Model()
        model.metadata.name = name
        if self._namespace:
            model.metadata.namespace = self._namespace
        model.spec.model_artifact_uri.append(artifact_uri)
        if deployable_artifact_uri:
            model.spec.deployable_artifact_uri.append(deployable_artifact_uri)
        if description:
            model.spec.description = description

        # Kubernetes label values are capped at 63 characters and restricted
        # to alphanumerics/dashes/underscores/dots. Callers built on top of
        # ModelMetadata.to_registry_dict() (e.g. `model_class`, a fully
        # qualified Python import path) commonly exceed this, and
        # InMemoryRegistryClient never enforces it, so the violation is only
        # ever caught here, against a real apiserver. Demote any offending
        # value into the metadata annotation instead of erroring — the
        # information is preserved (JSON has no such length limit), it's
        # just no longer filterable as a label.
        extra_metadata: dict[str, Any] = dict(metadata or {})
        for k, v in (labels or {}).items():
            if _is_valid_k8s_label_value(v):
                model.metadata.labels[k] = v
            else:
                _logger.warning(
                    "register_model(%r): label %r=%r is not a valid Kubernetes "
                    "label value (max %d chars, alphanumeric/-/_/. only) — "
                    "storing under the %r annotation instead.",
                    name,
                    k,
                    v,
                    _K8S_LABEL_VALUE_MAX_LENGTH,
                    METADATA_ANNOTATION_KEY,
                )
                extra_metadata.setdefault(k, v)
        metadata = extra_metadata
        if metadata:
            model.metadata.annotations[METADATA_ANNOTATION_KEY] = json.dumps(
                dict(metadata)
            )
        return model

    def _to_registered_model(self, model: model_pb2.Model) -> RegisteredModel:
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
        return RegisteredModel(
            name=name,
            version=version,
            registry_uri=f"models:/{namespace}/{name}/{version}",
            artifact_uri=artifact_uri,
            deployable_artifact_uri=deployable_artifact_uri,
            labels=labels,
            metadata=metadata,
        )
