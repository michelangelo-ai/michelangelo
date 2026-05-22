"""Abstract base class for all pusher plugins."""

from __future__ import annotations

import logging
from abc import ABC, abstractmethod
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from michelangelo.lib.artifact_manager.storage_backend import StorageBackend
    from michelangelo.lib.model_manager.registry.client import ModelRegistryClient

_logger = logging.getLogger(__name__)

__all__ = ["PusherPluginBase"]


class PusherPluginBase(ABC):
    """Abstract base class that all pusher plugins must implement.

    Plugins receive infrastructure dependencies via constructor injection,
    keeping plugin logic free of SDK-specific client initialization.
    A new instance is created per artifact per ``push()`` invocation.

    Args:
        config: Plugin-specific configuration dataclass or dict.
        artifact: The artifact value to push. ``None`` for config-only plugins.
        storage_backend: Storage backend for uploading files. Subclasses that
            require it validate this in ``__init__`` via ``ConfigurationError``.
        registry_client: Model registry client. ``None`` for non-model plugins.

    Example::

        class MyPlugin(PusherPluginBase):
            def execute(self) -> dict[str, Any]:
                uri = self._storage_backend.upload(self._artifact.path, "key/v1")
                return {"uri": uri}
    """

    def __init__(
        self,
        config: Any,
        artifact: Any = None,
        storage_backend: StorageBackend | None = None,
        registry_client: ModelRegistryClient | None = None,
    ) -> None:
        """Store injected dependencies as protected instance attributes."""
        self._config = config
        self._artifact = artifact
        self._storage_backend = storage_backend
        self._registry_client = registry_client

    @abstractmethod
    def execute(self) -> dict[str, Any]:
        """Execute the push operation for this plugin.

        Returns:
            A dict of plugin-specific result values stored in
            ``PusherResult.value``. Use an empty dict for plugins that
            produce no structured output.

        Raises:
            Any exception is surfaced to ``push()`` and wrapped in
            ``PusherPluginError`` when ``fail_fast=True``.
        """
