"""Serialize additional, dynamically-imported Python modules into a package."""

import os
from typing import Optional

from michelangelo.lib.model_manager._private.utils.module_finder import (
    find_dependency_files,
)
from michelangelo.lib.model_manager._private.utils.module_utils import (
    save_module_files,
)


def serialize_additional_imports(
    additional_import_prefixes: Optional[list[str]],
    target_dir: str,
    include_import_prefixes: Optional[list[str]] = None,
) -> None:
    """Serialize additional Python modules into the target directory.

    This complements the static import extraction performed for the model
    class by allowing users to specify module prefixes that are dynamically
    imported (e.g. via importlib) and would not be captured by the standard
    import analysis.

    Each prefix is treated as a module path. Its source files and their
    transitive dependencies are recursively resolved and copied into
    ``target_dir``, preserving the original directory structure.

    Args:
        additional_import_prefixes: Module prefixes to resolve and
            serialize, e.g. ``["mypackage.foo", "mypackage.bar.utils"]``.
            If None or empty, this function is a no-op.
        target_dir: The directory to copy the resolved source files into.
        include_import_prefixes: Optional filter passed to
            ``find_dependency_files`` so that only transitive dependencies
            whose module names start with one of these prefixes are
            included.
    """
    if not additional_import_prefixes:
        return

    os.makedirs(target_dir, exist_ok=True)

    for prefix in additional_import_prefixes:
        files = find_dependency_files(prefix, prefixes=include_import_prefixes)
        save_module_files(files, target_dir)
