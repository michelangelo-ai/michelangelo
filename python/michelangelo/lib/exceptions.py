"""Exceptions raised by Michelangelo library components."""

from __future__ import annotations

__all__ = ["ConfigurationError"]


class ConfigurationError(Exception):
    """Raised when a library component is misconfigured.

    Raised by ``__post_init__`` on config dataclasses when a required field
    is missing or invalid (e.g. empty ``endpoint``, empty ``bucket``).

    Args:
        message: Human-readable description of the configuration problem.

    Example:
        >>> err = ConfigurationError("endpoint must be non-empty.")
        >>> str(err)
        'endpoint must be non-empty.'
    """

    def __init__(self, message: str) -> None:
        """Initialize with a human-readable configuration error message."""
        super().__init__(message)
