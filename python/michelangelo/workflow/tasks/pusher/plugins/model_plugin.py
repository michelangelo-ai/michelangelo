"""ModelPusherPlugin — uploads an AssembledModel and registers it in a model registry.

The plugin is infrastructure-agnostic: callers supply any ``StorageBackend``
and ``ModelRegistryClient`` implementation. Uber-specific behaviour (HDFS
download, CheckpointManager, pipeline-run annotation, M3 metrics) lives in
a provider subclass and is not part of the open source contract.

Typical usage::

    import tempfile

    from michelangelo.lib.artifact_manager.storage_backend import LocalStorageBackend
    from michelangelo.workflow.schema.pusher import ModelPluginConfig
    from michelangelo.workflow.tasks.pusher.plugins.model_plugin import (
        ModelPusherPlugin,
    )
    from michelangelo.workflow.variables.types import AssembledModel, ModelArtifact

    backend = LocalStorageBackend(tempfile.mkdtemp())
    result = ModelPusherPlugin(
        config=ModelPluginConfig(model_name="my-classifier"),
        artifact=AssembledModel(
            raw_model=ModelArtifact(path="/tmp/raw"),
            deployable_model=ModelArtifact(path="/tmp/deployable"),
        ),
        storage_backend=backend,
        registry_client=my_registry,
    ).execute()
    print(result["model_name"], result["version"])
"""

from __future__ import annotations

import logging
import uuid
from typing import TYPE_CHECKING, Any

from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.tasks.pusher.plugins.base import PusherPluginBase

if TYPE_CHECKING:
    from michelangelo.lib.artifact_manager.storage_backend import StorageBackend
    from michelangelo.lib.model_manager.registry.client import ModelRegistryClient
    from michelangelo.workflow.schema.pusher import ModelPluginConfig
    from michelangelo.workflow.variables.types import AssembledModel

_logger = logging.getLogger(__name__)

__all__ = ["ModelPusherPlugin"]


class ModelPusherPlugin(PusherPluginBase):
    """Plugin that uploads a trained model and registers it in a model registry.

    Uploads the raw model artifact first, then the deployable artifact, then
    calls ``register_model()`` with the resolved name and both URIs. Both
    artifacts must already be packaged — packaging is an assembler-time
    concern (e.g. a Ray worker with GPU access) outside the pusher's scope.

    Args:
        config: ``ModelPluginConfig`` specifying the optional ``model_name``,
            ``description``, and ``extra_metadata`` registry tags.
        artifact: An ``AssembledModel`` with pre-packaged ``raw_model`` and
            ``deployable_model`` artifacts. Required.
        storage_backend: Backend used to upload both artifact paths. Required.
        registry_client: Registry used to record the model version. Required.

    Raises:
        ConfigurationError: If any of ``artifact``, ``storage_backend``, or
            ``registry_client`` is ``None``.

    Example::

        import tempfile

        from michelangelo.lib.artifact_manager.storage_backend import (
            LocalStorageBackend,
        )
        from michelangelo.workflow.schema.pusher import ModelPluginConfig
        from michelangelo.workflow.tasks.pusher.plugins.model_plugin import (
            ModelPusherPlugin,
        )
        from michelangelo.workflow.variables.types import AssembledModel, ModelArtifact

        backend = LocalStorageBackend(tempfile.mkdtemp())
        plugin = ModelPusherPlugin(
            config=ModelPluginConfig(model_name="my-classifier"),
            artifact=AssembledModel(
                raw_model=ModelArtifact(path="/tmp/raw"),
                deployable_model=ModelArtifact(path="/tmp/deployable"),
            ),
            storage_backend=backend,
            registry_client=my_registry,
        )
        result = plugin.execute()
        # result == {
        #     "model_name": "my-classifier",
        #     "version": "1",
        #     "raw_artifact_uri": "/store/models/my-classifier/raw",
        #     "deployable_artifact_uri": "/store/models/my-classifier/deployable",
        # }
    """

    def __init__(
        self,
        config: ModelPluginConfig,
        artifact: AssembledModel | None = None,
        storage_backend: StorageBackend | None = None,
        registry_client: ModelRegistryClient | None = None,
    ) -> None:
        """Validate required dependencies and store them for ``execute()``."""
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
        if registry_client is None:
            raise ConfigurationError(
                "ModelPusherPlugin requires a registry_client. "
                "Pass a ModelRegistryClient implementation (e.g. MLflowRegistryClient) "
                "via the registry_client= argument."
            )

    def execute(self) -> dict[str, Any]:
        """Upload both model artifacts and register the model in the registry.

        Resolves the model name (``config.model_name`` → auto-generated),
        uploads the raw artifact first, then the deployable artifact, and
        calls ``register_model()`` with both URIs and merged metadata.

        Returns:
            A dict with:

            - ``"model_name"``: name under which the model was registered.
            - ``"version"``: registry-assigned version string.
            - ``"raw_artifact_uri"``: URI of the uploaded raw model artifact.
            - ``"deployable_artifact_uri"``: URI of the uploaded deployable
              artifact.

        Raises:
            IOError: If an upload or registry network call fails.
        """
        model_name = self._config.model_name
        if model_name is None:
            model_name = _generate_name()

        _logger.info("Uploading raw model artifact for '%s'.", model_name)
        raw_uri = self._storage_backend.upload(
            self._artifact.raw_model.path,
            f"models/{model_name}/raw",
        )

        _logger.info("Uploading deployable artifact for '%s'.", model_name)
        deployable_uri = self._storage_backend.upload(
            self._artifact.deployable_model.path,
            f"models/{model_name}/deployable",
        )

        _logger.info("Registering '%s' in model registry.", model_name)
        registered = self._registry_client.register_model(
            name=model_name,
            artifact_uri=raw_uri,
            deployable_artifact_uri=deployable_uri,
            metadata=self._build_metadata_dict(),
        )

        return {
            "model_name": registered.name,
            "version": registered.version,
            "raw_artifact_uri": raw_uri,
            "deployable_artifact_uri": deployable_uri,
        }

    def _build_metadata_dict(self) -> dict[str, str]:
        """Build the registry metadata dict from artifact metadata and config tags.

        Extracts the public ``str`` and ``bool`` fields from the raw artifact's
        ``ModelMetadata``, skipping private ``BytesIO`` fields (``_schema``,
        ``_sample_data``, ``_hyperparameters``). ``None`` values are omitted.
        The result is merged with ``config.extra_metadata``, with
        ``extra_metadata`` taking precedence on key conflicts.

        Returns:
            A flat ``dict[str, str]`` suitable for registry label storage.
        """
        meta = self._artifact.raw_model.metadata
        result: dict[str, str] = {}

        for field_name in ("training_framework", "model_class"):
            value = getattr(meta, field_name, None)
            if value is not None:
                result[field_name] = value

        for field_name in ("assembled", "deployable"):
            result[field_name] = str(getattr(meta, field_name, False)).lower()

        result.update(getattr(self._config, "extra_metadata", {}))
        return result


def _generate_name() -> str:
    """Generate a unique model name with an 8-character hex UUID suffix."""
    return f"model-{uuid.uuid4().hex[:8]}"
