"""Utilities for finding the root import path of a package."""

import functools
import os
import sys
from pathlib import PurePath
from typing import Optional


def get_root_import_path(file_path: Optional[str] = None) -> str:
    """Get the root import path of the current package.

    Omits the paths of third-party packages.

    Args:
        file_path: The path of the file to start searching from.
            If not specified, use the path of the current file.

    Returns:
        The root import path of the current package
    """
    file = file_path or __file__
    script_path = os.path.abspath(file)
    parents = PurePath(script_path).parents
    paths = [path for path in sys.path if PurePath(path) in parents]
    return find_deepest_path(paths) if len(paths) > 0 else None


def find_deepest_path(paths):
    """Find the deepest path among a list of paths.

    Args:
        paths: A list of file paths.

    Returns:
        The deepest path.
    """
    return functools.reduce(
        lambda x, y: x if len(PurePath(x).parts) > len(PurePath(y).parts) else y,
        paths,
    )
