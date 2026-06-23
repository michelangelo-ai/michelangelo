"""Utilities for nested object-specification dicts.

A specification dict describes how to construct an object: a reserved
``_target_`` key holds the import path of the class to instantiate, and the
remaining keys are constructor arguments, which may themselves be nested
specifications.
"""

from __future__ import annotations

from typing import Any

_TARGET_KEY = "_target_"


def collect_nested_class_paths(val: Any) -> set[str]:
    """Recursively collect every ``_target_`` class path in a nested spec.

    Args:
        val: A specification value, which may be a dict, a list, or a scalar.
            Dicts and lists are traversed recursively.

    Returns:
        The set of class import paths referenced by any ``_target_`` key found
        anywhere within the value.
    """
    found: set[str] = set()
    if isinstance(val, dict):
        if _TARGET_KEY in val:
            found.add(val[_TARGET_KEY])
        for key, value in val.items():
            if key != _TARGET_KEY:
                found |= collect_nested_class_paths(value)
    elif isinstance(val, list):
        for value in val:
            found |= collect_nested_class_paths(value)
    return found
