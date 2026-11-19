"""Read the Triton backend of a deployable model package."""

from __future__ import annotations

import os

from michelangelo.lib.model_manager._private.serde.model.pbtxt import (
    find_string_field,
)
from michelangelo.lib.model_manager.constants.triton_backend_type import (
    TritonBackendType,
)


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

    with open(config_path, encoding="utf-8") as f:
        backend = find_string_field(f.read(), "backend")
    if not backend:
        raise ValueError(
            f"Could not find backend specification in config.pbtxt at {model_path}"
        )

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
