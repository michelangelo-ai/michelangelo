"""Starlark plugin bindings exposed under the uniflow `lib` namespace."""

from .deployment import (
    create_or_update_deployment,
    model_deployment,
    wait_for_deployment,
)

__all__ = [
    "create_or_update_deployment",
    "model_deployment",
    "wait_for_deployment",
]
