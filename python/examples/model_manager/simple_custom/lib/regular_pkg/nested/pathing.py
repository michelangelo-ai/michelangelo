"""Nested module under a regular package."""

from __future__ import annotations


def join_parts(*parts: str) -> str:
    """Join non-empty string parts with `/`."""
    return "/".join([p for p in parts if p])
