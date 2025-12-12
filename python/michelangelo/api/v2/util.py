"""Utility functions for Michelangelo API operations.

This module provides helper functions for common API operations, including
name generation following Kubernetes and Michelangelo conventions.
"""

import uuid
from datetime import datetime, timezone


def generate_random_name(prefix):
    """Generate a unique object name following Kubernetes naming conventions.

    Creates a name with format: {prefix}-{timestamp}-{random_chars}
    where prefix is normalized to lowercase with underscores replaced by hyphens.

    Args:
        prefix: Name prefix for the generated name. Must be 1-128 characters.

    Returns:
        A unique name string combining the prefix, UTC timestamp (YYYYMMDD-HHMMSS),
        and the first segment of a random UUID.

    Raises:
        RuntimeError: If prefix is empty or exceeds 128 characters.
    """
    if len(prefix) == 0:
        raise RuntimeError("Prefix cannot be empty.")
    if len(prefix) > 128:
        raise RuntimeError("Prefix cannot have more than 128 characters.")

    prefix = prefix.lower().replace("_", "-")
    t = datetime.now(timezone.utc)
    return "{prefix}-{ts}-{rand_char}".format(
        prefix=prefix,
        ts=datetime.strftime(t, "%Y%m%d-%H%M%S"),
        rand_char=str(uuid.uuid4()).split("-")[0],
    )
