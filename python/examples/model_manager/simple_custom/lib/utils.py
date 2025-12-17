"""Utilities for the simple_custom example (top-level module under lib/)."""

from __future__ import annotations

from examples.model_manager.simple_custom.lib.regular_pkg import format_artifact


def build_artifact_content(prefix: str, model_name: str) -> str:
    # Calls into a *regular package* under lib/ (dependency extraction test).
    return format_artifact(prefix=prefix, model_name=model_name)


