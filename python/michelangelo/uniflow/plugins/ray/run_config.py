"""Ray Train ``RunConfig`` helper defaulted to UniFlow-managed storage.

Any Ray-based workflow task (trainer, and future tasks with their own Ray
Train steps) can call :func:`create_run_config` instead of hand-rolling its
own ``storage_path``/``storage_filesystem`` defaulting from a task-specific
``storage_backend`` parameter. Centralizing this here keeps "where does Ray
Train write checkpoints" in one shared place as the task catalog grows,
rather than each task re-deriving it independently.
"""

from __future__ import annotations

import logging
import os
import tempfile
from urllib.parse import urlparse

import ray.train

from michelangelo.uniflow.plugins.ray.io import _fs_path

_logger = logging.getLogger(__name__)

__all__ = ["create_run_config"]


def create_run_config(**kwargs) -> ray.train.RunConfig:
    """Build a ``ray.train.RunConfig`` defaulted to UniFlow-managed storage.

    Resolves ``storage_path``/``storage_filesystem`` from the same
    ``UF_STORAGE_URL`` environment variable that ``DatasetVariable`` and
    ``ModelVariable`` already use for their own storage location, via the
    existing :func:`michelangelo.uniflow.plugins.ray.io._fs_path` filesystem
    resolver (native PyArrow S3, or fsspec when
    ``UF_PLUGIN_RAY_USE_FSSPEC=1``). Falls back to a local temp directory
    when ``UF_STORAGE_URL`` is unset, so local/sandbox runs keep working
    without extra configuration.

    Args:
        **kwargs: Any ``ray.train.RunConfig`` keyword argument. Explicitly
            passing ``storage_path`` and/or ``storage_filesystem`` overrides
            the ``UF_STORAGE_URL``-derived default for that field.

    Returns:
        A ``ray.train.RunConfig`` with ``storage_path``/``storage_filesystem``
        defaulted as described above.
    """
    storage_url = os.environ.get("UF_STORAGE_URL")
    if storage_url:
        storage_filesystem, storage_path = _fs_path(storage_url)
        if storage_filesystem is not None:
            # Ray Train expects a filesystem-relative path (no scheme) when an
            # explicit storage_filesystem is set; _fs_path()'s native-PyArrow
            # branch returns the raw URL, so strip the scheme here.
            parsed = urlparse(storage_path)
            storage_path = parsed.netloc + parsed.path
    else:
        storage_filesystem = None
        storage_path = tempfile.mkdtemp(prefix="michelangelo_train_")

    kwargs.setdefault("storage_path", storage_path)
    kwargs.setdefault("storage_filesystem", storage_filesystem)

    _logger.info(
        "create_run_config: storage_path=%r storage_filesystem=%r",
        kwargs["storage_path"],
        kwargs["storage_filesystem"],
    )
    return ray.train.RunConfig(**kwargs)
