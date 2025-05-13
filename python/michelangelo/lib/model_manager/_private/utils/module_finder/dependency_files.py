from __future__ import annotations
import inspect
import importlib
import pkgutil
import os
from uber.ai.michelangelo.sdk.model_manager._private.utils.module_finder.import_parser import get_imports


def find_dependency_files(
    module_name: str,
    prefixes: list[str] | None = None,
    max_depth: int | None = 100,
) -> list[str]:
    """
    Recursively find the files of the imported modules

    Args:
        module_name: the module name
        prefixes: the prefixes of the module import path to be included in the search
        max_depth: the maximum depth to search

    Returns:
        The list of files
    """
    files = set()

    if prefixes and module_name not in prefixes:
        prefixes.append(module_name)

    find_dependency_files_internal(
        module_name,
        files,
        0,
        prefixes,
        max_depth,
    )

    return list(files)


def find_dependency_files_internal(
    module_name: str,
    files: set[str],
    depth: int,
    prefixes: list[str] | None = None,
    max_depth: int | None = None,
):
    if prefixes and not any(module_name.startswith(prefix) for prefix in prefixes):
        return None

    if max_depth is not None and depth > max_depth:
        return None

    try:
        package = importlib.import_module(module_name)
    except (ImportError, TypeError):
        return None

    # if the module is a package
    if hasattr(package, "__path__"):
        for importer, name, _ in pkgutil.walk_packages(package.__path__):
            full_name = f"{module_name}.{name}"

            try:
                sub_module = importlib.import_module(full_name)
                files.add(inspect.getfile(sub_module))
            except (ImportError, TypeError):
                pass

            init_file = os.path.join(importer.path, "__init__.py")
            if os.path.exists(init_file):
                files.add(init_file)

            find_dependency_files_internal(
                full_name,
                files,
                depth + 1,
                prefixes,
                max_depth,
            )
    # if the module is a file
    elif hasattr(package, "__file__"):
        files.add(package.__file__)
        modules = get_imports(package)
        for module in modules:
            find_dependency_files_internal(
                module,
                files,
                depth + 1,
                prefixes,
                max_depth,
            )
