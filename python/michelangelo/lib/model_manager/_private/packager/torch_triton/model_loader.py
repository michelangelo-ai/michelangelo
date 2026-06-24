"""Serialize the torch Python-backend serde loader into model packages.

At packaging time, the loader and its dependencies are copied into the
package directory so they are available in the Triton serving environment
without requiring the full michelangelo library to be installed there.
"""

from __future__ import annotations

from michelangelo.lib.model_manager._private.serde.loader.torch_deployable_model_loader import (  # noqa: E501
    _load_torch_python_deployable_model,
)
from michelangelo.lib.model_manager._private.utils.module_finder import (
    find_dependency_files,
)
from michelangelo.lib.model_manager._private.utils.module_utils import save_module_files


def serialize_torch_python_loader(
    target_dir: str,
    include_import_prefixes: list[str] | None = None,
) -> None:
    """Serialize the torch Python-backend loader and its dependencies.

    Copies the source of ``_load_torch_python_deployable_model`` and all
    imported modules whose names start with one of *include_import_prefixes*
    into *target_dir*, preserving the original directory structure.  The
    result is bundled into the model package so Triton can import the loader
    at serve time without a full michelangelo installation.

    Args:
        target_dir: Directory in which to write the serialized source files.
        include_import_prefixes: When set, only modules with names that start
            with one of these prefixes are serialized.  When ``None`` or
            empty, all discovered imports are included.

    Returns:
        None
    """
    module_name = _load_torch_python_deployable_model.__module__
    files = find_dependency_files(module_name, prefixes=include_import_prefixes)
    save_module_files(files, target_dir)
