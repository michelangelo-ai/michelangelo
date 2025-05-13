from __future__ import annotations
from typing import Callable
from collections.abc import Iterator  # noqa: TC003
from .pickle_definition import find_pickle_definitions
from .pickled_file import find_pickled_files


def walk_pickle_definitions_in_dir(
    directory: str,
    match: Callable[[str, str, str], bool] | None = None,
) -> Iterator[tuple[str, str, str]]:
    """
    Walk through all pickle definitions for any pickled file in the directory.

    Args:
        directory: the directory
        match: a function to filter the pickle definitions. The function should take
            three arguments: module_def, attr_name, file_path, and return True if the
            definition should be included, False otherwise.

    Returns:
        A iterator of tuples of (module_def, attr_name, file_path)
    """
    for pickled_file in find_pickled_files(directory):
        pickle_defs = find_pickle_definitions(pickled_file)
        for pickle_def in pickle_defs:
            module_def, _, attr_name = pickle_def.rpartition(".")
            if match is None or match(module_def, attr_name, pickled_file):
                yield module_def, attr_name, pickled_file
