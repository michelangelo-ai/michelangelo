"""Namespace package module (no __init__.py in ns_pkg/)."""

from __future__ import annotations

import numpy as np

from examples.model_manager.simple_custom.lib.ns_pkg.nested_ns.ops import add_zero


def echo_int32(x: np.ndarray) -> np.ndarray:
    # Calls into nested namespace package module.
    return add_zero(x.astype(np.int32))


