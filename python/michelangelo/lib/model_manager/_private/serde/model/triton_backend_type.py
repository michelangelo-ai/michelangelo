"""Read the Triton backend of a deployable model package."""

from __future__ import annotations

import os
import re

from michelangelo.lib.model_manager.constants.triton_backend_type import (
    TritonBackendType,
)

_BACKEND_PATTERN = re.compile(r'^\s*backend\s*:\s*"([^"]+)"', re.MULTILINE)


def get_deployable_backend_type(model_path: str) -> str:
    """Return the backend named in a deployable package's ``config.pbtxt``.

    Args:
        model_path: The deployable model package path.

    Returns:
        The backend, one of
        :class:`~michelangelo.lib.model_manager.constants.TritonBackendType`.

    Raises:
        FileNotFoundError: If the package has no ``config.pbtxt``.
        ValueError: If ``config.pbtxt`` names no backend, or one that is not
            supported.
    """
    config_path = os.path.join(model_path, "config.pbtxt")
    if not os.path.exists(config_path):
        raise FileNotFoundError(f"config.pbtxt not found in {model_path}")

    with open(config_path) as f:
        match = _BACKEND_PATTERN.search(f.read())
    if not match:
        raise ValueError(
            f"Could not find backend specification in config.pbtxt at {model_path}"
        )

    backend = match.group(1)
    supported = get_supported_deployable_backend_types()
    if backend not in supported:
        raise ValueError(
            f"Invalid backend type '{backend}' in config.pbtxt at {model_path}. "
            f"Supported backend types are {', '.join(sorted(supported))}."
        )
    return backend


def get_supported_deployable_backend_types() -> set[str]:
    """Return the backends a deployable package may name.

    Returns:
        The values of
        :class:`~michelangelo.lib.model_manager.constants.TritonBackendType`.
    """
    return {
        getattr(TritonBackendType, attr)
        for attr in dir(TritonBackendType)
        if not attr.startswith("__")
    }
