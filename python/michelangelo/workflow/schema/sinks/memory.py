"""Config dataclass for InMemorySink."""

from __future__ import annotations

from dataclasses import dataclass


@dataclass
class InMemorySinkConfig:
    """Typed configuration for ``InMemorySink``.

    ``InMemorySink`` is stateless — this config carries no fields but exists
    so that all sinks follow the same config-first pattern, enabling uniform
    serialisation and introspection by the workflow engine.

    Example:
        >>> cfg = InMemorySinkConfig()
        >>> cfg
        InMemorySinkConfig()
    """
