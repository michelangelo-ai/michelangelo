"""Utilities for nested object-specification dicts.

A specification dict describes how to construct an object: a reserved
``_target_`` key holds the import path of the class to instantiate, and the
remaining keys are constructor arguments, which may themselves be nested
specifications.
"""

from __future__ import annotations

import importlib
from typing import Any

_TARGET_KEY = "_target_"


def instantiate(cfg: Any) -> Any:
    """Recursively instantiate a nested ``_target_`` config.

    A dict with a ``_target_`` key is treated as an object specification:
    the ``_target_`` value is a fully qualified class name, and remaining
    keys are passed as constructor arguments.  Nested dicts with their own
    ``_target_`` are recursively instantiated first.

    Args:
        cfg: A config value. A dict with a ``_target_`` key is treated as an
            object spec; anything else is returned unchanged.

    Returns:
        The instantiated object, or the original value when it is not a spec.
    """
    if not isinstance(cfg, dict) or _TARGET_KEY not in cfg:
        return cfg
    target = cfg[_TARGET_KEY]
    module_path, class_name = target.rsplit(".", 1)
    cls = getattr(importlib.import_module(module_path), class_name)
    kwargs = {}
    for key, value in cfg.items():
        if key == _TARGET_KEY:
            continue
        if isinstance(value, dict) and _TARGET_KEY in value:
            kwargs[key] = instantiate(value)
        elif isinstance(value, list):
            kwargs[key] = [
                instantiate(item)
                if isinstance(item, dict) and _TARGET_KEY in item
                else item
                for item in value
            ]
        else:
            kwargs[key] = value
    return cls(**kwargs)


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
