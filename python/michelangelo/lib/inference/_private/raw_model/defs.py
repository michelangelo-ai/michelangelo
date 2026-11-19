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
    """Make the modules in a raw model's ``defs/`` importable.

    ``defs/`` is appended to ``sys.path`` once, so it never shadows the standard
    library or installed packages. This changes process-wide state: a module is
    imported once per process, so two raw models whose ``defs/`` hold a module
    of the same name must be loaded in separate processes. Each Ray inference
    actor loads one model, so the predictor never mixes them.

    Args:
        model_path: The path to the packaged raw model.
    """
    defs_path = os.path.abspath(get_model_defs_path(model_path))
    if os.path.exists(defs_path) and defs_path not in sys.path:
        sys.path.append(defs_path)
