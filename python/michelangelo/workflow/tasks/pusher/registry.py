"""Plugin registry for mapping plugin names to their implementations."""

from __future__ import annotations

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from michelangelo.workflow.tasks.pusher.plugins.base import PusherPluginBase

from michelangelo.workflow.schema.exceptions import ConfigurationError

__all__ = ["PluginRegistry", "default_registry"]


class PluginRegistry:
    """Registry mapping plugin names to their implementation class and artifact type.

    The open source library ships a ``default_registry`` pre-populated with
    the three built-in plugins (populated by ``plugins/__init__.py``). Downstream
    packages call ``extend()`` to create a child registry and register their own
    plugins without mutating the shared default.

    Lookups fall through to the parent registry when a name is not found
    locally, forming a chain: child registry → ``default_registry``.

    Args:
        parent: Optional parent registry to inherit registrations from. When
            a name is absent locally, lookup continues in the parent.

    Example::

        from michelangelo.workflow.tasks.pusher.registry import default_registry

        custom_registry = default_registry.extend()
        custom_registry.register(
            "my_plugin",
            MyPlugin,
            MyArtifactType,
        )
        plugin_class, artifact_type = custom_registry.get("model_plugin")
    """

    def __init__(self, parent: PluginRegistry | None = None) -> None:
        """Initialise with an optional parent registry."""
        self._registry: dict[
            str, tuple[type[PusherPluginBase], type | tuple[type, ...] | None]
        ] = {}
        self._parent = parent

    def register(
        self,
        name: str,
        plugin_class: type[PusherPluginBase],
        artifact_type: type | tuple[type, ...] | None = None,
    ) -> None:
        """Register a plugin under a given name.

        Registering an already-registered name in the same instance raises an
        error. To override a parent-registered plugin, register the same name
        in a child registry created via ``extend()``.

        Args:
            name: Plugin identifier used in ``PusherPluginConfig`` as the
                typed field name or as the ``plugin_name`` extension value.
            plugin_class: Concrete subclass of ``PusherPluginBase``.
            artifact_type: Expected Python type (or tuple of types) of the
                artifact value. When provided, ``push()`` validates
                ``isinstance(artifact, artifact_type)`` before invoking the
                plugin. Pass ``None`` for config-only plugins or when the
                plugin accepts any artifact type.

        Raises:
            ValueError: If ``name`` is already registered in this instance.
                Overrides must go through a child registry via ``extend()``.

        Example:
            >>> registry = PluginRegistry()
            >>> # registry.register("model_plugin", ModelPusherPlugin, AssembledModel)
        """
        if name in self._registry:
            raise ValueError(
                f"Plugin '{name}' is already registered in this registry. "
                "To override a parent registration, create a child registry "
                "via extend() and register the override there."
            )
        self._registry[name] = (plugin_class, artifact_type)

    def get(
        self, name: str
    ) -> tuple[type[PusherPluginBase], type | tuple[type, ...] | None]:
        """Look up a plugin by name, falling through to the parent if needed.

        Args:
            name: Plugin name to look up.

        Returns:
            A tuple of ``(plugin_class, artifact_type)``. ``artifact_type``
            may be ``None`` for config-only plugins.

        Raises:
            ConfigurationError: If ``name`` is not found in this registry or
                any ancestor registry.

        Example:
            >>> registry = PluginRegistry()
            >>> registry.get("unknown")  # doctest: +IGNORE_EXCEPTION_DETAIL
            Traceback (most recent call last):
                ...
            michelangelo.workflow.schema.exceptions.ConfigurationError: ...
        """
        if name in self._registry:
            return self._registry[name]
        if self._parent is not None:
            return self._parent.get(name)
        raise ConfigurationError(
            f"No plugin registered under name '{name}'. "
            f"Registered names: {self.registered_names()}."
        )

    def registered_names(self) -> list[str]:
        """Return all plugin names visible from this registry, including parents.

        Returns:
            Sorted list of plugin name strings from this instance and all
            ancestor registries.

        Example:
            >>> registry = PluginRegistry()
            >>> registry.registered_names()
            []
        """
        names: set[str] = set(self._registry.keys())
        if self._parent is not None:
            names.update(self._parent.registered_names())
        return sorted(names)

    def extend(self) -> PluginRegistry:
        """Create a child registry that inherits all registrations from this one.

        Lookups on the child fall through to this registry for any name not
        registered locally. Registering the same name in the child overrides
        the parent's registration without mutating it.

        Returns:
            A new ``PluginRegistry`` whose parent is this registry.

        Example::

            from michelangelo.workflow.tasks.pusher.registry import default_registry

            provider_registry = default_registry.extend()
            provider_registry.register(
                "model_plugin",
                ProviderModelPusherPlugin,
                AssembledModel,
            )
        """
        return PluginRegistry(parent=self)


# Declared here as an empty registry; populated by plugins/__init__.py to
# avoid circular imports between registry and plugins.base.
default_registry: PluginRegistry = PluginRegistry()
