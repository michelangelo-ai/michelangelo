"""Helpers for working with classes by their fully-qualified names."""

# ruff: noqa: I001
from typing import Any

import importlib


def get_full_class_name(obj: Any) -> str:
    """Return the fully-qualified class name of ``obj``."""
    cls = obj.__class__
    return f"{cls.__module__}.{cls.__name__}"


def is_instance_of(obj: Any, full_class_name: str) -> bool:
    """Check if the object is an instance of the class with the given name.

    Also returns True for subclasses. Return False if the class cannot be
    imported.
    """
    try:
        module_name, class_name = full_class_name.rsplit(".", 1)
    except ValueError as e:
        raise ValueError(f"Invalid full class name: {full_class_name}") from e

    try:
        module = importlib.import_module(module_name)
    except ModuleNotFoundError:
        return False

    try:
        clz = getattr(module, class_name)
    except AttributeError:
        return False

    if isinstance(obj, clz):  # noqa: SIM103
        return True

    return False
