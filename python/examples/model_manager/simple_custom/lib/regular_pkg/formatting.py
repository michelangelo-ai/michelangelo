"""Formatting helpers (regular package module) for dependency extraction tests."""

from __future__ import annotations

from examples.model_manager.simple_custom.lib.regular_pkg.nested.pathing import join_parts


def format_artifact(prefix: str, model_name: str) -> str:
    # Calls into a *nested regular package* (dependency extraction test).
    return join_parts(prefix, "model", model_name)


