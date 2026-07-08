"""ModelPusherPlugin — uploads an AssembledModel and registers it in a model registry.

The plugin is infrastructure-agnostic: callers supply any ``StorageBackend``
and ``ModelRegistryClient`` implementation. Subclass this plugin to add
organization-specific behavior (custom storage protocols, telemetry,
post-registration hooks) without modifying the core upload/register flow.

Typical usage::

    import tempfile

    from michelangelo.lib.artifact_manager.storage_backend import LocalStorageBackend
    from michelangelo.lib.model_manager.registry.client import InMemoryRegistryClient
    from michelangelo.workflow.schema.pusher import ModelPluginConfig
    from michelangelo.workflow.tasks.pusher.plugins.model_plugin import (
        ModelPusherPlugin,
    )
    from michelangelo.workflow.variables.types import AssembledModel, ModelArtifact

    backend = LocalStorageBackend(tempfile.mkdtemp())
    registry = InMemoryRegistryClient()  # replace with your registry in production
    result = ModelPusherPlugin(
        config=ModelPluginConfig(model_name="my-classifier"),
        artifact=AssembledModel(
            raw_model=ModelArtifact(path="/tmp/raw"),
            deployable_model=ModelArtifact(path="/tmp/deployable"),
        ),
        storage_backend=backend,
        registry_client=registry,
    ).execute()
    print(result["model_name"], result["version"])

Multi-registry fan-out — register in two registries simultaneously::

    import tempfile

    from michelangelo.lib.artifact_manager.storage_backend import LocalStorageBackend
    from michelangelo.lib.model_manager.registry.client import InMemoryRegistryClient
    from michelangelo.workflow.schema.pusher import ModelPluginConfig
    from michelangelo.workflow.tasks.pusher.plugins.model_plugin import (
        ModelPusherPlugin,
    )
    from michelangelo.workflow.variables.types import AssembledModel, ModelArtifact

    backend = LocalStorageBackend(tempfile.mkdtemp())
    primary = InMemoryRegistryClient()
    catalog = InMemoryRegistryClient()
    result = ModelPusherPlugin(
        config=ModelPluginConfig(
            model_name="clf",
            registry_clients=[primary, catalog],
        ),
        artifact=AssembledModel(
            raw_model=ModelArtifact(path="/tmp/raw"),
            deployable_model=ModelArtifact(path="/tmp/deployable"),
        ),
        storage_backend=backend,
    ).execute()
    # Inspect per-registry versions — they are assigned independently:
    for reg in result["registrations"]:
        print(reg["registry_uri"], reg["version"])

To implement a custom registry, subclass ``ModelRegistryClient``::

    from michelangelo.lib.model_manager.registry.client import (
        RegisteredModel, ModelRegistryClient,
    )

    class MyRegistryClient(ModelRegistryClient):
        def __init__(self, api):
            self._api = api  # e.g. MlflowClient(tracking_uri=...) or requests.Session()

        def register_model(self, name, artifact_uri, **kwargs) -> RegisteredModel:
            run_id = (kwargs.get("metadata") or {}).get("run_id")
            version = self._api.create_version(name, artifact_uri, run_id=run_id)
            return RegisteredModel(
                name=name,
                version=version.id,
                registry_uri=f"myregistry://{name}/{version.id}",
                labels=dict(kwargs.get("labels") or {}),
                metadata=dict(kwargs.get("metadata") or {}),
            )

        def get_model(self, name, version=None) -> RegisteredModel:
            v = self._api.get_version(name, version)
            return RegisteredModel(name=name, version=v.id, registry_uri=v.uri)
"""

from __future__ import annotations

import logging
import uuid
from pathlib import Path
from typing import TYPE_CHECKING, Any, TypedDict

from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.tasks.pusher.plugins.base import PusherPluginBase

if TYPE_CHECKING:
    from michelangelo.lib.artifact_manager.storage_backend import StorageBackend
    from michelangelo.lib.model_manager.registry.client import (
        ModelRegistryClient,
    )
    from michelangelo.workflow.schema.pusher import ModelPluginConfig
    from michelangelo.workflow.variables.types import AssembledModel

_logger = logging.getLogger(__name__)

__all__ = [
    "ModelPushResult",
    "ModelPusherPlugin",
    "PartialRegistrationError",
    "RegistrationResult",
]


class PartialRegistrationError(Exception):
    """Raised when fan-out registration fails after one or more registries succeeded.

    In multi-registry fan-out, registration calls proceed in order. If registry
    N raises, registries 0…N-1 already hold a record. This exception carries
    the completed registrations so callers can compensate (retry the failed
    registry, roll back the successful ones, or alert).

    Attributes:
        registrations_completed: Per-registry results for every registry that
            successfully registered before the failure.
        failed_registry_type: Class name of the registry that raised.

    Example::

        try:
            result = plugin.execute()
        except PartialRegistrationError as exc:
            for reg in exc.registrations_completed:
                print(f"Already registered: {reg['registry_uri']}")
            print(f"Failed at: {exc.failed_registry_type}")
            raise
    """

    def __init__(
        self,
        registrations_completed: list[RegistrationResult],
        failed_registry_type: str,
        cause: Exception,
    ) -> None:
        """Initialize with completed registrations, failed registry type, and cause."""
        self.registrations_completed = registrations_completed
        self.failed_registry_type = failed_registry_type
        super().__init__(
            f"{failed_registry_type} failed after "
            f"{len(registrations_completed)} successful registration(s): {cause}"
        )
        self.__cause__ = cause


class RegistrationResult(TypedDict):
    """Per-registry result produced by a single ``ModelRegistryClient`` call."""

    version: str
    registry_uri: str


class ModelPushResult(TypedDict):
    """Typed return value of ``ModelPusherPlugin.execute()``.

    Attributes:
        model_name: Name under which the model was registered (shared across
            all registries).
        version: Registry-assigned version from the **first** registry in the
            list. Provided as a convenience for the common single-registry
            case.

            .. warning::
                When using multi-registry fan-out, each registry assigns
                versions independently — the first registry's version is
                **not** representative of the others. Always inspect
                ``registrations`` for per-registry version strings when
                ``len(registrations) > 1``.

        raw_artifact_uri: URI of the uploaded raw model artifact. The same
            URI is sent to every registry.
        deployable_artifact_uri: URI of the uploaded deployable artifact, or
            ``None`` when ``artifact.deployable_model`` is ``None``. The same
            URI is sent to every registry.
        push_id: 16-character hex token identifying this specific upload.
            Appears in storage keys (``models/{name}/{push_id}/raw``).

            .. note::
                ``push_id`` is a storage-layer correlation token only — it
                does not appear in the registry. Use
                ``registrations[*].version`` and
                ``registrations[*].registry_uri`` to identify the model in
                the registry. To store the push_id in the registry, pass it
                via ``config.labels`` (e.g.
                ``labels={"push_id": result["push_id"]}`` on the next call).

        registrations: Per-registry result list — one entry per
            ``ModelRegistryClient`` used. Each entry contains ``version``
            and ``registry_uri`` for that registry. Inspect this list for
            per-registry details when using multi-registry fan-out.
    """

    model_name: str
    version: str
    raw_artifact_uri: str
    deployable_artifact_uri: str | None
    push_id: str
    registrations: list[RegistrationResult]


class ModelPusherPlugin(PusherPluginBase):
    """Plugin that uploads a trained model and registers it in a model registry.

    Uploads the raw model artifact first, then the deployable artifact, then
    calls ``register_model()`` on each configured registry with the resolved
    name, both URIs, the description, and split labels/metadata. Both artifacts
    must already be packaged — packaging is an assembler-time concern outside
    the pusher's scope.

    The two artifact formats serve different consumers:

    - ``raw_model`` — training-format checkpoint (weights + optimizer state).
      Used to resume training, fine-tune, or run offline validation.
    - ``deployable_model`` — serving-optimized bundle (e.g. ONNX, TorchScript,
      or a Triton config + weights directory). Loaded directly by an inference
      runtime.

    **Single registry** (default): pass ``registry_client=`` to the constructor
    or leave ``config.registry_clients`` empty.

    **Multi-registry fan-out**: populate ``config.registry_clients`` with two
    or more ``ModelRegistryClient`` instances. The same artifact URIs are sent
    to every registry. When ``config.registry_clients`` is non-empty it takes
    precedence over the ``registry_client=`` constructor argument; passing both
    raises ``ConfigurationError``.

    .. note::
        Model input/output schema (``ModelMetadata._schema``) is not forwarded
        to the registry in this implementation. Subclasses may override
        ``execute()`` to pass ``schema=`` to ``register_model()`` when the
        target registry supports native schema storage (e.g. MLflow
        ``ModelSignature``, Vertex AI predict schema).

    .. warning::
        **Upload semantics**: artifact uploads and registry registrations use
        at-least-once semantics. A failure after the first upload but before
        registration may leave orphan artifact blobs in storage. Implement
        idempotent storage key patterns or clean up via
        ``storage_backend.delete()`` in error handlers.

        **Multi-registry fan-out**: registration calls proceed in order and
        fail-fast on the first exception. If registry N fails, registries
        0…N-1 already hold a record while N…end do not. Callers must handle
        partial registration and compensate as needed.

    Args:
        config: ``ModelPluginConfig`` specifying the optional
            ``model_name``, ``description``, ``labels``, ``run_id``, and
            ``registry_clients`` list for multi-registry fan-out.
        artifact: An ``AssembledModel`` with a pre-packaged ``raw_model``.
            ``deployable_model`` is optional — when ``None``, the deployable
            upload is skipped and ``deployable_artifact_uri`` is ``None`` in
            the result. Required.
        storage_backend: Backend used to upload both artifact paths. Required.
            ``upload()`` must return a fully-qualified URI consumable without
            additional context (e.g. ``s3://…``, ``gs://…``, absolute path).
        registry_client: Primary registry client. Required when
            ``config.registry_clients`` is empty. Has no effect when
            ``config.registry_clients`` is non-empty (passing both raises
            ``ConfigurationError``).

    Raises:
        ConfigurationError: If ``artifact`` or ``storage_backend`` is ``None``;
            if no registry is configured (both ``registry_client`` is ``None``
            and ``config.registry_clients`` is empty); or if both
            ``registry_client`` and a non-empty ``config.registry_clients``
            are supplied simultaneously.

    Example::

        import tempfile

        from michelangelo.lib.artifact_manager.storage_backend import (
            LocalStorageBackend,
        )
        from michelangelo.lib.model_manager.registry.client import (
            InMemoryRegistryClient,
        )
        from michelangelo.workflow.schema.pusher import ModelPluginConfig
        from michelangelo.workflow.tasks.pusher.plugins.model_plugin import (
            ModelPusherPlugin,
        )
        from michelangelo.workflow.variables.types import AssembledModel, ModelArtifact

        backend = LocalStorageBackend(tempfile.mkdtemp())
        registry = InMemoryRegistryClient()  # replace with your registry in production
        plugin = ModelPusherPlugin(
            config=ModelPluginConfig(
                model_name="my-classifier",
                description="Boston housing XGBoost model",
                labels={"owner": "ml-platform"},
                run_id="mlflow-run-abc123",
            ),
            artifact=AssembledModel(
                raw_model=ModelArtifact(path="/tmp/raw"),
                deployable_model=ModelArtifact(path="/tmp/deployable"),
            ),
            storage_backend=backend,
            registry_client=registry,
        )
        result = plugin.execute()
        # result == {
        #     "model_name": "my-classifier",
        #     "version": "1",
        #     "push_id": "a1b2c3d4e5f6a7b8",
        #     "raw_artifact_uri": "/store/models/my-classifier/<push_id>/raw",
        #     "deployable_artifact_uri":
        #         "/store/models/my-classifier/<push_id>/deployable",
        #     "registrations": [
        #         {"version": "1", "registry_uri": "memory://my-classifier/1"}
        #     ],
        # }
    """

    def __init__(
        self,
        config: ModelPluginConfig,
        artifact: AssembledModel | None = None,
        storage_backend: StorageBackend | None = None,
        registry_client: ModelRegistryClient | None = None,
    ) -> None:
        """Initialize the plugin. See class docstring for arguments and exceptions."""
        super().__init__(config, artifact, storage_backend, registry_client)
        if artifact is None:
            raise ConfigurationError(
                "ModelPusherPlugin requires an AssembledModel artifact. "
                "Pass the assembled model via the artifact= argument."
            )
        if storage_backend is None:
            raise ConfigurationError(
                "ModelPusherPlugin requires a storage_backend. "
                "Pass a StorageBackend implementation (e.g. LocalStorageBackend) "
                "via the storage_backend= argument."
            )
        if config.model_name == "":
            raise ConfigurationError(
                "ModelPusherPlugin: model_name must be a non-empty string or None "
                "(None auto-generates a unique name, e.g. 'model-a1b2c3d4')."
            )
        # Resolve the effective registry list at init time.
        has_injected = registry_client is not None
        has_config_list = bool(config.registry_clients)
        if has_injected and has_config_list:
            raise ConfigurationError(
                "Provide either registry_client= or config.registry_clients, not both. "
                "When config.registry_clients is non-empty it defines the full list; "
                "the constructor registry_client= argument is ignored."
            )
        if has_config_list:
            effective: list[ModelRegistryClient] = list(config.registry_clients)
        elif has_injected:
            effective = [registry_client]  # type: ignore[list-item]
        else:
            raise ConfigurationError(
                "ModelPusherPlugin requires at least one registry client. "
                "Either pass registry_client= or set config.registry_clients."
            )
        self._registries: list[ModelRegistryClient] = effective
        # Keep base-class attribute coherent so subclass overrides that read
        # self._registry_client still get a valid client.
        self._registry_client = self._registries[0]

    def execute(self) -> ModelPushResult:
        """Upload both model artifacts and register the model in all registries.

        Resolves the model name (``config.model_name`` → auto-generated when
        ``None``), generates a unique 16-hex-character push ID to avoid storage
        key collisions across versions, uploads the raw artifact first, then
        the deployable artifact, and calls ``register_model()`` on each
        configured registry with both URIs, description, labels, and metadata.

        Returns:
            A :class:`ModelPushResult` dict with:

            - ``"model_name"``: name under which the model was registered.
            - ``"version"``: version string from the **first** registry.
              Use ``registrations`` for per-registry versions in fan-out mode.
            - ``"raw_artifact_uri"``: URI of the uploaded raw model artifact.
            - ``"deployable_artifact_uri"``: URI of the uploaded deployable
              artifact.
            - ``"push_id"``: 16-character hex token embedded in the
              storage keys for this upload.
            - ``"registrations"``: list of per-registry dicts, each containing
              ``"version"`` and ``"registry_uri"``. One entry per registry.

        Raises:
            IOError: If an upload fails.
            PartialRegistrationError: If a registry call fails after one or
                more registries have already registered successfully. Carries
                ``registrations_completed`` and ``failed_registry_type`` for
                compensation. In the single-registry case, the underlying
                exception propagates directly (no partial state exists).
            ValueError: If the model name is invalid per a registry's rules.
        """
        if self._config.model_name is None:
            _logger.warning(
                "No model_name set in config — auto-generating a UUID-based name. "
                "Set config.model_name explicitly for reproducible model identities."
            )
            model_name = _generate_name()
        else:
            model_name = self._config.model_name
        push_id = uuid.uuid4().hex[:16]
        base_labels = self._build_labels()
        base_metadata = self._build_metadata()

        _logger.info(
            "Uploading raw model artifact for '%s' (push %s).", model_name, push_id
        )
        raw_uri = self._storage_backend.upload(
            self._artifact.raw_model.path,
            f"models/{model_name}/{push_id}/raw/{Path(self._artifact.raw_model.path).name}",
        )

        deployable_uri: str | None = None
        if self._artifact.deployable_model is not None:
            _logger.info(
                "Uploading deployable artifact for '%s' (push %s).", model_name, push_id
            )
            deployable_uri = self._storage_backend.upload(
                self._artifact.deployable_model.path,
                f"models/{model_name}/{push_id}/deployable/{Path(self._artifact.deployable_model.path).name}",
            )

        registrations: list[RegistrationResult] = []
        for registry in self._registries:
            _logger.info("Registering '%s' in %s.", model_name, type(registry).__name__)
            try:
                registered = registry.register_model(
                    name=model_name,
                    artifact_uri=raw_uri,
                    deployable_artifact_uri=deployable_uri,
                    description=self._config.description,
                    labels=dict(
                        base_labels
                    ),  # shallow copy — prevents cross-registry mutation
                    metadata=dict(
                        base_metadata
                    ),  # shallow copy — prevents cross-registry mutation
                )
            except Exception as exc:
                if registrations:
                    raise PartialRegistrationError(
                        registrations_completed=list(registrations),
                        failed_registry_type=type(registry).__name__,
                        cause=exc,
                    ) from exc
                raise
            _logger.info(
                "Registered '%s' v%s at %s.",
                registered.name,
                registered.version,
                registered.registry_uri,
            )
            registrations.append(
                RegistrationResult(
                    version=registered.version,
                    registry_uri=registered.registry_uri,
                )
            )

        return ModelPushResult(
            model_name=model_name,
            version=registrations[0]["version"],
            raw_artifact_uri=raw_uri,
            deployable_artifact_uri=deployable_uri,
            push_id=push_id,
            registrations=registrations,
        )

    def _build_labels(self) -> dict[str, str]:
        """Build the indexed labels dict from artifact metadata and config labels.

        Calls ``ModelMetadata.to_registry_dict()`` on the raw artifact's
        metadata, then merges with ``config.labels``. Caller-supplied
        ``labels`` take precedence on key conflicts.

        Returns:
            A ``dict[str, str]`` suitable for ``ModelRegistryClient.register_model(
            labels=...)``. A fresh dict is returned on each call.
        """
        result: dict[str, str] = dict(
            self._artifact.raw_model.metadata.to_registry_dict()
        )
        result.update(self._config.labels)
        return result

    def _build_metadata(self) -> dict[str, Any]:
        """Build the supplementary metadata dict.

        Merges ``config.metadata`` with ``run_id`` injected under the key
        ``"run_id"`` when set. ``run_id`` takes precedence over any ``"run_id"``
        key in ``config.metadata``. Registries with native run linkage (e.g.
        MLflow) should extract ``metadata["run_id"]`` in their
        ``register_model()`` implementation and pass it to the registry-native
        API.

        Returns:
            A ``dict[str, Any]`` suitable for ``ModelRegistryClient.register_model(
            metadata=...)``. Returns an empty dict when no supplementary
            metadata is configured.
        """
        result: dict[str, Any] = dict(self._config.metadata)
        if self._config.run_id is not None:
            result["run_id"] = self._config.run_id  # run_id wins on collision
        return result


def _generate_name() -> str:
    """Generate a unique model name with an 8-character hex UUID suffix."""
    return f"model-{uuid.uuid4().hex[:8]}"
