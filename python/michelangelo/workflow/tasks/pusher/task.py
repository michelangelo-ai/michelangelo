"""Top-level push() dispatch function for the Michelangelo pusher."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING, Any, Callable

from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.tasks.pusher.exceptions import (
    ArtifactNotFoundError,
    PusherPluginError,
)
from michelangelo.workflow.variables.types import PusherResult

if TYPE_CHECKING:
    from michelangelo.lib.artifact_manager.storage_backend import StorageBackend
    from michelangelo.lib.model_manager.registry.client import ModelRegistryClient
    from michelangelo.workflow.schema.pusher import PusherConfig
    from michelangelo.workflow.tasks.pusher.registry import PluginRegistry

_logger = logging.getLogger(__name__)

__all__ = ["push"]


def push(
    config: PusherConfig,
    artifacts: dict[str, Any],
    *,
    storage_backend: StorageBackend | None = None,
    registry_client: ModelRegistryClient | None = None,
    registry: PluginRegistry | None = None,
    fail_fast: bool = True,
    on_error: Callable[[str, str, Exception], None] | None = None,
) -> list[PusherResult]:
    """Push one or more artifacts using their configured plugins.

    Iterates ``config.items`` in order, resolves each artifact from
    ``artifacts`` by name, instantiates the matching plugin, and calls
    ``execute()``. Infrastructure dependencies (``storage_backend``,
    ``registry_client``) are injected into every plugin.

    Args:
        config: Top-level pusher configuration listing artifact/plugin pairs.
        artifacts: Mapping from artifact name to artifact value. Keys must
            match ``PusherPluginConfig.name`` for each item in config.
        storage_backend: Backend used for artifact uploads. Required —
            pass a ``LocalStorageBackend``, ``MinioStorageBackend``, or any
            :class:`~michelangelo.lib.artifact_manager.storage_backend.StorageBackend`
            subclass. Raises :class:`ConfigurationError` when ``None``.
        registry_client: Registry client injected into plugins that require
            one (e.g. ``ModelPusherPlugin``). Pass ``None`` for plugins that
            don't need a registry, or when registry clients are specified
            directly on ``ModelPluginConfig.registry_clients``.
        registry: Plugin registry to resolve plugin names against. Defaults
            to ``default_registry`` when ``None``.
        fail_fast: When ``True`` (default), the first plugin failure raises
            ``PusherPluginError`` and subsequent items are not processed.
            When ``False``, all items run and failures are recorded in
            ``PusherResult.error``.
        on_error: Optional callback invoked on every plugin failure, regardless
            of ``fail_fast``. Signature: ``(artifact_name, plugin_name, exc)``.
            Exceptions raised by the callback are logged and suppressed.

    Returns:
        List of :class:`~michelangelo.workflow.variables.types.PusherResult`,
        one per ``config.items`` entry processed. In ``fail_fast=True`` mode
        the list is shorter than ``config.items`` when a failure occurs.

    Raises:
        ArtifactNotFoundError: If a name in ``config.items`` is absent from
            ``artifacts``.
        ConfigurationError: If a plugin name is not registered, or the
            artifact type does not match the registered expected type.
        PusherPluginError: If a plugin's ``execute()`` raises and
            ``fail_fast=True``.

    Example::

        from michelangelo.lib.model_manager.registry.client import (
            InMemoryRegistryClient,
        )
        from michelangelo.workflow.schema.pusher import (
            ModelPluginConfig, PusherConfig, PusherPluginConfig,
        )
        from michelangelo.workflow.tasks.pusher import push
        from michelangelo.workflow.variables.types import AssembledModel, ModelArtifact

        result = push(
            config=PusherConfig(items=[
                PusherPluginConfig(
                    name="clf",
                    model_plugin=ModelPluginConfig(model_name="my-classifier"),
                ),
            ]),
            artifacts={
                "clf": AssembledModel(raw_model=ModelArtifact(path="/tmp/raw"))
            },
            registry_client=InMemoryRegistryClient(),
        )
        assert result[0].success
    """
    # Importing pusher.plugins here guarantees default_registry is populated
    # with the three built-in plugins regardless of how push() was imported
    # (package __init__ or direct module import).
    import michelangelo.workflow.tasks.pusher.plugins  # noqa: F401
    from michelangelo.workflow.tasks.pusher.registry import (
        default_registry,
    )

    effective_registry = registry if registry is not None else default_registry

    if storage_backend is None:
        raise ConfigurationError(
            "storage_backend is required. Pass a LocalStorageBackend, "
            "MinioStorageBackend, or any StorageBackend subclass."
        )

    results: list[PusherResult] = []

    for item in config.items:
        artifact_name = item.name

        if artifact_name not in artifacts:
            raise ArtifactNotFoundError(artifact_name, list(artifacts.keys()))

        artifact = artifacts[artifact_name]
        plugin_name = item.resolved_plugin_name()
        plugin_class, expected_type = effective_registry.get(plugin_name)

        if expected_type is not None and not isinstance(artifact, expected_type):
            if isinstance(expected_type, tuple):
                expected_name = " | ".join(t.__name__ for t in expected_type)
            else:
                expected_name = expected_type.__name__
            raise ConfigurationError(
                f"Artifact '{artifact_name}' has type "
                f"{type(artifact).__name__!r} but plugin '{plugin_name}' "
                f"expects {expected_name!r}."
            )

        plugin_cfg = item.resolved_plugin_config()
        plugin = plugin_class(
            config=plugin_cfg,
            artifact=artifact,
            storage_backend=storage_backend,
            registry_client=registry_client,
        )

        _logger.info("Pushing artifact '%s' via '%s'.", artifact_name, plugin_name)
        try:
            value = plugin.execute()
            results.append(
                PusherResult(
                    name=artifact_name,
                    plugin=plugin_name,
                    success=True,
                    value=value,
                )
            )
        except Exception as exc:
            _logger.error(
                "Plugin '%s' failed for artifact '%s': %s",
                plugin_name,
                artifact_name,
                exc,
            )
            if on_error is not None:
                try:
                    on_error(artifact_name, plugin_name, exc)
                except Exception as cb_exc:
                    _logger.warning(
                        "on_error callback raised for artifact '%s' plugin '%s': %s",
                        artifact_name,
                        plugin_name,
                        cb_exc,
                    )

            if fail_fast:
                raise PusherPluginError(artifact_name, plugin_name) from exc

            results.append(
                PusherResult(
                    name=artifact_name,
                    plugin=plugin_name,
                    success=False,
                    value={},
                    error=str(exc),
                )
            )

    return results
