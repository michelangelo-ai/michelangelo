"""Nested module under a regular package."""

from __future__ import annotations


def join_parts(*parts: str) -> str:
    return "/".join([p for p in parts if p])
