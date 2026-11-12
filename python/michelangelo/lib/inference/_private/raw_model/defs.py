"""Locate and import the user-defined modules shipped with a raw model."""

from __future__ import annotations

import os
import sys


def get_model_defs_path(model_path: str) -> str:
    """Get the path to the model defs directory for a given model path.

    Args:
        model_path: The path to the packaged raw model.

    Returns:
        The path to the model defs.
    """
    return os.path.join(model_path, "defs")


def inject_model_defs(model_path: str) -> None:
    """Inject the model defs into ``sys.path``.

    Args:
        model_path: The path to the packaged raw model.
    """
    defs_path = get_model_defs_path(model_path)
    if os.path.exists(defs_path):
        sys.path.insert(0, defs_path)
