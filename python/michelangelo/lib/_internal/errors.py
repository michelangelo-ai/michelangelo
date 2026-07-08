"""Shared exception types for the michelangelo library."""

from __future__ import annotations


class UserInputError(Exception):
    """Raised when a user-supplied input or path causes an operation to fail."""
