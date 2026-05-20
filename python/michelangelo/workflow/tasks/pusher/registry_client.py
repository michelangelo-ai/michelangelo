"""Model registry client abstraction."""

from __future__ import annotations

from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from typing import Any


@dataclass
class RegisteredModel:
    """A model record returned after successful registration.

    Attributes:
        name: Model name in the registry.
        version: Registry-assigned version string (e.g. ``"3"`` for MLflow,
            a resource ID suffix for Vertex AI).
        registry_uri: URI uniquely identifying this model version in the
            registry (e.g. ``"models:/name/3"`` for MLflow).
        metadata: Additional key-value pairs stored with the registration.

    Example:
        >>> model = RegisteredModel(
        ...     name="my-classifier",
        ...     version="1",
        ...     registry_uri="models:/my-classifier/1",
        ... )
        >>> model.version
        '1'
    """

    name: str
    version: str
    registry_uri: str
    metadata: dict[str, Any] = field(default_factory=dict)


class ModelRegistryClient(ABC):
    """Abstract base class for model registry clients.

    Implement this interface to connect the pusher to any model registry
    (MLflow, Vertex AI, custom). A single instance is shared across all
    plugin invocations within one ``push()`` call.

    Example implementation::

        class InMemoryRegistryClient(ModelRegistryClient):
            def __init__(self) -> None:
                self._store: dict[str, list[RegisteredModel]] = {}

            def register_model(
                self,
                name: str,
                artifact_uri: str,
                **kwargs: Any,
            ) -> RegisteredModel:
                version = str(len(self._store.get(name, [])) + 1)
                entry = RegisteredModel(
                    name=name,
                    version=version,
                    registry_uri=f"memory://{name}/{version}",
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
        schema: dict[str, Any] | None = None,
        metadata: dict[str, str] | None = None,
    ) -> RegisteredModel:
        """Register a model and its artifact URI in the registry.

        Args:
            name: Model name to register under. The registry creates the model
                entry if it does not already exist.
            artifact_uri: URI of the raw model artifact as returned by
                ``StorageBackend.upload()``.
            deployable_artifact_uri: Optional URI of the serving-ready bundle.
                How this is stored depends on the registry implementation.
            schema: Optional model input/output schema. Registry
                implementations that do not support a native schema field may
                ignore this argument.
            metadata: Optional string key-value pairs stored alongside the
                registration (e.g. ``{"framework": "xgboost"}``).

        Returns:
            A ``RegisteredModel`` describing the created registration,
            including the registry-assigned ``version`` and ``registry_uri``.

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
