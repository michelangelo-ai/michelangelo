from __future__ import annotations
import os
import io
import pickletools


def is_pickled_file(path: str) -> bool:
    """
    Check if the file is a pickled file.

    Args:
        path: the path of the file

    Returns:
        True if the file is a pickled file, False otherwise
    """
    try:
        with open(path, "rb") as f:
            pickletools.dis(f, out=io.StringIO())
    except Exception:
        return False
    else:
        return True


def find_pickled_files(directory: str) -> list[str]:
    """
    Find all pickled files under the directory.

    Args:
        dir: the directory

    Returns:
        A list of pickled files
    """
    pickled_files = [
        os.path.join(dirpath, filename)
        for dirpath, _, filenames in os.walk(directory)
        for filename in filenames
        if is_pickled_file(os.path.join(dirpath, filename))
    ]

    return pickled_files
