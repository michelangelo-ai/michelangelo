from __future__ import annotations

import importlib
import inspect
import os
import pkgutil

from michelangelo.lib.model_manager._private.utils.module_finder.import_parser import (
    get_imports,
)


def find_dependency_files(
    module_name: str,
    prefixes: list[str] | None = None,
    max_depth: int | None = 100,
) -> dict[str, str]:
    """Recursively find the files of the imported modules.

    Args:
        module_name: the module name
        prefixes: the prefixes of the module import path to be included in the search
        max_depth: the maximum depth to search

    Returns:
        The dictionary whose keys are the full module names
        and values are the module file paths
        The __init__.py file is also included,
        with the key being the package name with __init__ appended

        e.g. {
            "foo.bar.baz": "/path/to/foo/bar/baz.py",
            "foo.bar": "/path/to/foo/bar.py",
            "foo": "/path/to/foo.py",
            "foo.__init__": "/path/to/foo/__init__.py",
        }
    """
    files = {}

    if prefixes and module_name not in prefixes:
        prefixes.append(module_name)

    find_dependency_files_internal(
        module_name,
        files,
        0,
        prefixes,
        max_depth,
    )

    return files


def find_dependency_files_internal(
    module_name: str,
    files: set[str],
    depth: int,
    prefixes: list[str] | None = None,
    max_depth: int | None = None,
):
    """Recursively find dependency files helper.

    Args:
        module_name: The name of the module to search.
        files: A set to collect found file paths.
        depth: The current recursion depth.
        prefixes: Optional list of module prefixes to filter imports.
        max_depth: Optional maximum recursion depth.
    """
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
                files[full_name] = inspect.getfile(sub_module)
            except (ImportError, TypeError):
                pass

            init_file = os.path.join(importer.path, "__init__.py")
            if os.path.exists(init_file):
                files[f"{module_name}.__init__"] = init_file

            find_dependency_files_internal(
                full_name,
                files,
                depth + 1,
                prefixes,
                max_depth,
            )
    # if the module is a file
    elif hasattr(package, "__file__"):
        files[module_name] = package.__file__
        modules = get_imports(package)
        for module in modules:
            find_dependency_files_internal(
                module,
                files,
                depth + 1,
                prefixes,
                max_depth,
            )
