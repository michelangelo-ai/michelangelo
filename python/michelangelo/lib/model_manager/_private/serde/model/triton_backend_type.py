"""Read the Triton backend of a deployable model package."""

from __future__ import annotations

import os
import re

_CONFIG_FILE = "config.pbtxt"
_BACKEND_PATTERN = re.compile(r'^\s*backend\s*:\s*"([^"]+)"', re.MULTILINE)


def get_deployable_backend_type(model_path: str) -> str:
    """Return the backend named in a deployable package's ``config.pbtxt``.

    Args:
        model_path: The deployable model package path.

    Returns:
        The backend, such as ``"pytorch"``, ``"onnxruntime"`` or ``"python"``.
        See :class:`~michelangelo.lib.model_manager.constants.TritonBackendType`.

    Raises:
        FileNotFoundError: If the package has no ``config.pbtxt``.
        ValueError: If ``config.pbtxt`` names no backend.
    """
    config_path = os.path.join(model_path, _CONFIG_FILE)
    if not os.path.exists(config_path):
        raise FileNotFoundError(f"{_CONFIG_FILE} not found in {model_path}.")

    with open(config_path) as f:
        match = _BACKEND_PATTERN.search(f.read())
    if not match:
        raise ValueError(f"{_CONFIG_FILE} in {model_path} names no backend.")
    return match.group(1)
