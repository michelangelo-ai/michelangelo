"""Shared test helpers for custom_triton packager tests."""

import os
from pathlib import Path


def list_relative_files(root):
    """Walks a directory and returns sorted paths relative to it.

    Args:
        root: The directory to walk.

    Returns:
        A sorted list of file paths relative to ``root``.
    """
    return sorted(
        str(Path(os.path.join(dirpath, file)).relative_to(root))
        for dirpath, _, filenames in os.walk(root)
        for file in filenames
    )
