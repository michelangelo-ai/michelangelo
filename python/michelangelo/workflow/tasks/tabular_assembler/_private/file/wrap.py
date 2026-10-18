"""Normalize a downloaded raw model to a directory layout."""

from __future__ import annotations

import os
import shutil
import tempfile

__all__ = ["ensure_model_directory"]


def ensure_model_directory(local_model_path: str, file_name: str | None = None) -> str:
    """Return a directory holding the model, wrapping a single file if needed.

    A trainer may save one checkpoint file instead of a directory. After
    ``StorageBackend.download`` that leaves a *file* at ``local_model_path``,
    but the packagers expect a model directory. A file is moved into a new
    directory that takes over ``local_model_path``; a directory is returned
    untouched.

    Args:
        local_model_path: Local path produced by ``StorageBackend.download``.
        file_name: Name the wrapped file gets inside the new directory.
            Defaults to the basename of ``local_model_path``. Only the final
            path component is used; an empty name, ``.`` or ``..`` is
            ignored.

    Returns:
        ``local_model_path``, which is guaranteed to be a directory on return.

    Raises:
        FileNotFoundError: If ``local_model_path`` does not exist.
    """
    if os.path.isdir(local_model_path):
        return local_model_path
    if not os.path.isfile(local_model_path):
        raise FileNotFoundError(
            f"Downloaded raw model not found at '{local_model_path}'."
        )

    name = os.path.basename(file_name or "")
    if name in ("", ".", ".."):
        name = os.path.basename(local_model_path)
    holder = tempfile.mkdtemp(dir=os.path.dirname(os.path.abspath(local_model_path)))
    try:
        shutil.move(local_model_path, os.path.join(holder, name))
        os.rename(holder, local_model_path)
    except BaseException:
        shutil.rmtree(holder, ignore_errors=True)
        raise
    return local_model_path
