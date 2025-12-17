"""Artifact helpers for the simple_custom example."""

from __future__ import annotations

import os


def write_text_artifact(dir_path: str, filename: str, content: str) -> str:
    os.makedirs(dir_path, exist_ok=True)
    out = os.path.join(dir_path, filename)
    with open(out, "w", encoding="utf-8") as f:
        f.write(content)
    return out
