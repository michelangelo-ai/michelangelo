"""Configuration for user-supplied training dataloaders.

A custom dataloader replaces the default Ray Data ``iter_torch_batches`` path
with a user-owned per-stage factory. Two modes are supported:

* ``ray_data`` (default): the factory receives a Ray Data source (``ds``) in
  addition to the dataset path, so it can reuse Ray's file sharding.
* ``file_backed``: the factory reads the dataset directly from its path or
  URI. No Ray Dataset is registered or sharded; Ray Train still launches the
  workers and manages distributed training and checkpointing.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum

__all__ = ["CustomDataloaderConfig", "CustomDataloaderKind"]


class CustomDataloaderKind(str, Enum):
    """How a custom dataloader factory obtains its data.

    Attributes:
        RAY_DATA: The factory receives a Ray Data source (``ds``) for the
            stage. This is the default.
        FILE_BACKED: The factory reads from ``dataset_path`` itself. The
            ``ds`` argument is omitted and no Ray Dataset is built.
    """

    RAY_DATA = "ray_data"
    FILE_BACKED = "file_backed"


@dataclass
class CustomDataloaderConfig:
    """Configuration for a user-owned per-stage dataloader factory.

    ``build_dataloader_fn`` is called once per stage, with ``stage="train"`` or
    ``stage="validation"``, and returns a single iterable of batches for that
    stage. The factory is called as::

        build_dataloader_fn(
            stage=...,          # "train" | "validation"
            ds=...,             # Ray Data source; "ray_data" kind only
            dataset_path=...,   # storage path or URI of the stage's dataset
            read_kwargs=...,    # column projection and other read settings
            **build_dataloader_kwargs,
        )

    Each batch must be a ``dict`` mapping column names to batched tensors.
    The factory runs on every training worker, so it is responsible for
    sharding the data across ``ray.train.get_context().get_world_size()``
    workers in ``file_backed`` mode. In ``ray_data`` mode the ``ds`` shard is
    already split per worker.

    ``dataset_path`` may be any fsspec URI (for example ``s3://``, ``gs://``,
    or a path on a filesystem registered with ``fsspec.register_implementation``),
    which is how a storage backend is plugged in without changing this config.

    ``batch_iter_config`` and ``parquet_read_config`` do not apply to custom
    dataloaders and are rejected alongside this config. ``val_batch_size``-style
    settings are not applied either: the factory owns batching for both stages.

    Attributes:
        build_dataloader_fn: Dotted import path to the factory.
        build_dataloader_kwargs: Extra keyword arguments forwarded to the
            factory on every call.
        sample_data_fn: Optional dotted import path to a function that builds
            the model ``sample_data`` metadata on the driver. It is called with
            the same ``ds`` (``ray_data`` kind only), ``dataset_path`` and
            ``read_kwargs`` as the factory, plus ``sample_data_fn_kwargs``, and
            must return a list of dicts mapping input column names to NumPy
            arrays. When set, the driver does not build the train loader.
            When unset, ``sample_data`` is taken from the first batch of the
            loader built by ``build_dataloader_fn``.
        sample_data_fn_kwargs: Extra keyword arguments forwarded to
            ``sample_data_fn``.
        kind: Whether the factory consumes a Ray Data source or reads files
            directly. Defaults to ``CustomDataloaderKind.RAY_DATA``.

    Example:
        >>> CustomDataloaderConfig(
        ...     build_dataloader_fn="my_project.data.build_loader",
        ...     kind=CustomDataloaderKind.FILE_BACKED,
        ... )
        CustomDataloaderConfig(build_dataloader_fn='my_project.data.build_loader', ...)
    """

    build_dataloader_fn: str
    build_dataloader_kwargs: dict = field(default_factory=dict)
    sample_data_fn: str | None = None
    sample_data_fn_kwargs: dict = field(default_factory=dict)
    kind: CustomDataloaderKind = CustomDataloaderKind.RAY_DATA
