"""Exception hierarchy for the pusher module."""

from __future__ import annotations


class PusherError(Exception):
    """Base exception class for all pusher errors.

    All exceptions raised by the pusher module inherit from this class,
    allowing callers to catch the full family with a single
    ``except PusherError`` clause.
    """


class ArtifactNotFoundError(PusherError):
    """Raised when an artifact named in config is absent from the artifacts dict.

    Args:
        name: The artifact name that was expected.
        available: The artifact names that are actually present in the dict.

    Example:
        >>> err = ArtifactNotFoundError("model", ["dataset", "report"])
        >>> "model" in str(err)
        True
    """

    def __init__(self, name: str, available: list[str]) -> None:
        """Initialize with the missing artifact name and available names."""
        super().__init__(f"Artifact '{name}' not found. Available: {available}")


class ConfigurationError(PusherError):
    """Raised when a ``PusherConfig`` or ``PusherPluginConfig`` is invalid.

    Args:
        message: Human-readable description of the configuration problem.

    Example:
        >>> err = ConfigurationError("No plugin specified for artifact 'model'.")
        >>> str(err)
        "No plugin specified for artifact 'model'."
    """

    def __init__(self, message: str) -> None:
        """Initialize with a human-readable configuration error message."""
        super().__init__(message)


class PusherPluginError(PusherError):
    """Raised when a plugin's ``execute()`` raises an unexpected exception.

    This exception is raised by ``push()`` when ``fail_fast=True`` and a
    plugin raises. The original exception is chained via the ``__cause__``
    attribute so the full stack trace is preserved.

    Args:
        artifact_name: The artifact name the plugin was processing.
        plugin_name: The name of the plugin that raised.

    Example:
        >>> err = PusherPluginError("model", "model_plugin")
        >>> "model_plugin" in str(err)
        True
    """

    def __init__(self, artifact_name: str, plugin_name: str) -> None:
        """Initialize with the artifact name and plugin name that failed."""
        super().__init__(
            f"Plugin '{plugin_name}' failed for artifact '{artifact_name}'."
        )
