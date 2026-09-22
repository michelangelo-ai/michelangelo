"""Generic conversion of a Parquet-read config object into ``read_parquet`` kwargs.

Not specific to any single workflow task — any task-level "parquet read
config" dataclass (or pydantic model) can be converted, which is why this
accepts a loosely-typed object rather than a single concrete config class.
"""

from __future__ import annotations

import dataclasses
import logging
from typing import Any

import ray
from packaging import version

_logger = logging.getLogger(__name__)

__all__ = ["parquet_read_config_to_kwargs"]


def _config_to_dict(prc: Any) -> dict[str, Any]:
    """Flatten a parquet-read config object into a plain dict, dropping ``None``.

    Supports plain ``dataclasses.dataclass`` instances (the convention used
    by this repo's own workflow config schemas) and pydantic models (via
    ``model_dump``), so callers are not forced onto one particular config
    style.

    Raises:
        TypeError: If ``prc`` is neither a dataclass instance nor an object
            exposing ``model_dump``.
    """
    if dataclasses.is_dataclass(prc) and not isinstance(prc, type):
        return {k: v for k, v in dataclasses.asdict(prc).items() if v is not None}
    if hasattr(prc, "model_dump"):
        return prc.model_dump(exclude_none=True)
    raise TypeError(
        f"Cannot convert {type(prc).__name__} to read_parquet kwargs: expected a "
        "dataclass instance or a pydantic model with model_dump()."
    )


def parquet_read_config_to_kwargs(
    prc: Any | None, dataset_name: str | None = None
) -> dict[str, Any]:
    """Convert a parquet-read config object to ``ray.data.read_parquet`` kwargs.

    Handles the Ray 2.50 API change where ``num_cpus``, ``num_gpus``, and
    ``memory`` moved from ``ray_remote_args`` to top-level ``read_parquet``
    kwargs, and flattens an ``arrow_parquet_args`` mapping into top-level keys
    for the PyArrow reader.

    Args:
        prc: A parquet-read config object (dataclass instance or pydantic
            model). ``None`` preserves Ray's defaults (an empty kwargs dict
            is returned).
        dataset_name: Selects the entry to use from an optional
            ``override_num_blocks_per_dataset`` mapping field on ``prc``, if
            present. Ignored when ``prc`` has no such field.

    Returns:
        A kwargs dict ready to splat into ``ray.data.read_parquet(**kwargs)``.
    """
    read_kwargs = _config_to_dict(prc) if prc is not None else {}

    per_dataset_overrides = read_kwargs.pop("override_num_blocks_per_dataset", None)
    if per_dataset_overrides is not None:
        override = (
            per_dataset_overrides.get(dataset_name)
            if dataset_name is not None
            else None
        )
        if override is not None:
            read_kwargs["override_num_blocks"] = override
        else:
            read_kwargs.pop("override_num_blocks", None)

    if version.parse(ray.__version__) < version.parse("2.50"):
        ray_remote_args = read_kwargs.pop("ray_remote_args", None) or {}
        for key in ("num_cpus", "num_gpus", "memory"):
            if key in read_kwargs:
                ray_remote_args[key] = read_kwargs.pop(key)
        if ray_remote_args:
            read_kwargs["ray_remote_args"] = ray_remote_args

    read_kwargs.update(read_kwargs.pop("arrow_parquet_args", None) or {})
    return read_kwargs
