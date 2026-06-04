"""Model registry client abstraction for the model manager."""

from __future__ import annotations

from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from collections.abc import Mapping


@dataclass
class RegisteredModel:
    """A single model registration record returned after a successful push.

    Carries both the registry handle (``registry_uri``) and the storage
    locations of the underlying artifacts, so a caller can download the model
    for inference or a fine-tuning run without making a second registry call.

    Attributes:
        name: Model name in the registry.
        version: Registry-assigned version string (e.g. ``"3"`` for MLflow,
            a resource ID suffix for Vertex AI).
        registry_uri: URI uniquely identifying this model version in the
            registry (e.g. ``"models:/name/3"`` for MLflow). Use this to
            reference, compare, or promote versions via the registry API.
        artifact_uri: Storage URI of the raw model package (weights + sample
            data), as returned by ``StorageBackend.upload()``. Use this to
            download the model for offline validation or fine-tuning.
            ``None`` when the registry implementation does not expose the
            storage location (e.g. read-only registry views).
        deployable_artifact_uri: Storage URI of the serving-ready bundle
            (e.g. Triton config + weights). Use this to load the model onto
            a model server for inference. ``None`` when not applicable or
            not exposed by the registry.
        labels: Indexed, filterable string key-value pairs stored with the
            registration (e.g. ``{"training_framework": "xgboost"}``).
            Mirrors the ``labels`` parameter passed to ``register_model()``.
        metadata: Supplementary key-value pairs stored with the registration.
            Values may be any JSON-serializable type. Mirrors the ``metadata``
            parameter passed to ``register_model()``.

    Example:
        >>> model = RegisteredModel(
        ...     name="my-classifier",
        ...     version="1",
        ...     registry_uri="models:/my-classifier/1",
        ...     artifact_uri="s3://bucket/models/my-classifier/raw",
        ...     labels={"training_framework": "xgboost"},
        ...     metadata={"run_id": "mlflow-run-abc123"},
        ... )
        >>> model.labels["training_framework"]
        'xgboost'
    """

    name: str
    version: str
    registry_uri: str
    artifact_uri: str | None = None
    deployable_artifact_uri: str | None = None
    labels: dict[str, str] = field(default_factory=dict)
    metadata: dict[str, Any] = field(default_factory=dict)


class ModelRegistryClient(ABC):
    """Abstract base class for model registry clients.

    Implement this interface to connect the pusher to any model registry
    (MLflow, Vertex AI, W&B, Comet, or a custom store). A single instance is
    shared across all plugin invocations within one ``push()`` call.

    .. note::
        MLflow has a class also named ``RegisteredModel`` that represents a
        **model group** (all versions), not a single version record. When
        writing an ``MLflowRegistryClient``, alias one to avoid the collision::

            from michelangelo.lib.model_manager.registry.client import (
                RegisteredModel as MichelangeloModel,
            )
            from mlflow.entities.model_registry import ModelVersion

    Example implementation::

        class InMemoryRegistryClient(ModelRegistryClient):
            def __init__(self) -> None:
                self._store: dict[str, list[RegisteredModel]] = {}

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
                version = str(len(self._store.get(name, [])) + 1)
                entry = RegisteredModel(
                    name=name,
                    version=version,
                    registry_uri=f"memory://{name}/{version}",
                    artifact_uri=artifact_uri,
                    labels=dict(labels or {}),
                    metadata=dict(metadata or {}),
                )
                self._store.setdefault(name, []).append(entry)
                return entry

            def get_model(
                self,
                name: str,
                version: str | None = None,
            ) -> RegisteredModel:
                versions = self._store.get(name, [])
                if not versions:
                    raise KeyError(f"Model '{name}' not found.")
                return versions[-1] if version is None else next(
                    v for v in versions if v.version == version
                )
    """

    @abstractmethod
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
        """Register a model and its artifact URI in the registry.

        Args:
            name: Model name to register under. The registry creates the model
                entry if it does not already exist.
            artifact_uri: URI of the raw model artifact as returned by
                ``StorageBackend.upload()``. Must be a fully-qualified URI
                consumable without additional context (e.g. ``s3://…``,
                ``gs://…``, or an absolute local path).
            deployable_artifact_uri: Optional URI of the serving-ready bundle.
                How this is stored depends on the registry implementation.
                ``None`` means the raw artifact is also used for serving.
            description: Optional human-readable description stored alongside
                the model version in the registry.
            schema: Optional model input/output schema.

                Implementations that do not support a native schema field
                **must** silently accept and ignore this argument rather than
                raising — ignoring ``schema`` is the correct behaviour for any
                registry that lacks a dedicated schema API. Both
                :class:`APIRegistryClient` and :class:`InMemoryRegistryClient`
                follow this convention.  The built-in ``ModelPusherPlugin``
                does not populate this argument (it will always be ``None``
                unless called from a custom subclass).

            labels: Optional string-to-string key-value pairs stored as
                indexed, filterable labels in the registry (e.g.
                ``{"training_framework": "xgboost", "owner": "ml-platform"}``).
                Surfaced in the registry UI and searchable via filter
                expressions. Matches ``labels`` in Vertex AI and BentoML,
                ``tags`` in MLflow. Registry clients treat this as read-only.
            metadata: Optional key-value pairs for supplementary, non-indexed
                data (e.g. ``{"run_id": "mlflow-run-abc123", "accuracy": 0.94}``).
                Values may be any JSON-serializable type. Registries with
                native run linkage (e.g. MLflow) should extract
                ``metadata["run_id"]`` and pass it to their native
                version-creation API. Registry clients treat this as read-only.

        Returns:
            A ``RegisteredModel`` describing the created registration,
            including the registry-assigned ``version``, ``registry_uri``,
            ``labels``, and ``metadata``.

        Raises:
            IOError: If the registry cannot be reached.
            ValueError: If ``name`` is invalid per the registry's naming rules.
        """

    @abstractmethod
    def get_model(self, name: str, version: str | None = None) -> RegisteredModel:
        """Retrieve a model registration from the registry.

        Args:
            name: Model name to look up.
            version: Specific version string to retrieve. When ``None``, the
                registry's latest version is returned.

        Returns:
            The ``RegisteredModel`` for the requested name and version.

        Raises:
            KeyError: If the model name or version is not found.
            IOError: If the registry cannot be reached.
        """


class InMemoryRegistryClient(ModelRegistryClient):
    """Minimal in-memory registry client for testing and local development.

    .. warning::
        Not suitable for production use — all state is lost when the process
        exits. Use this client in unit tests and quick-start examples only.
        For production, implement ``ModelRegistryClient`` against your registry
        of choice (MLflow, Vertex AI, W&B, etc.).

    Example::

        registry = InMemoryRegistryClient()
        reg = registry.register_model(
            name="my-model",
            artifact_uri="s3://bucket/my-model/raw",
            labels={"training_framework": "xgboost"},
        )
        print(reg.version)   # "1"
        print(reg.labels)    # {"training_framework": "xgboost"}
    """

    def __init__(self) -> None:
        """Initialize an empty in-memory model registry store."""
        self._store: dict[str, list[RegisteredModel]] = {}

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
        """Register a model version and store it in memory."""
        version_num = str(len(self._store.get(name, [])) + 1)
        entry = RegisteredModel(
            name=name,
            version=version_num,
            registry_uri=f"memory://{name}/{version_num}",
            artifact_uri=artifact_uri,
            deployable_artifact_uri=deployable_artifact_uri,
            labels=dict(labels or {}),
            metadata=dict(metadata or {}),
        )
        self._store.setdefault(name, []).append(entry)
        return entry

    def get_model(self, name: str, version: str | None = None) -> RegisteredModel:
        """Retrieve a model registration from the in-memory store."""
        versions = self._store.get(name, [])
        if not versions:
            raise KeyError(f"Model '{name}' not found.")
        if version is None:
            return versions[-1]
        for v in versions:
            if v.version == version:
                return v
        raise KeyError(f"Model '{name}' version '{version}' not found.")
