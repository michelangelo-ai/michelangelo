from __future__ import annotations
import os
import sys
import importlib


def get_module_attr(module_attr: str) -> any:
    """
    Load the attribute from the module

    Args:
        module_attr: the full attribute definition,
            for example, 'module.submodule.attribute'

    Returns:
        The attribute
    """
    module_def, _, attr_def = module_attr.rpartition(".")
    module = importlib.import_module(module_def)
    attr = getattr(module, attr_def)
    return attr


def find_attr_from_sys_modules(attr_name: str) -> list[any]:
    """
    Search through available modules in sys.modules to find where the attribute is defined.

    Args:
        attr_name: The name of the attribute to search for.

    Returns:
        A list of module attributes whose name matches the input.
    """
    return [getattr(module, attr_name) for module_name, module in sys.modules.items() if hasattr(module, attr_name)]


def find_attr_from_dir(attr_name: str, path: str) -> list[any]:
    """
    Search through available modules in the given path to find module attributes whose name matches the input.
    Assuming all the python modules in the path are included in sys.path

    Args:
        attr_name: The name of the attribute to search for.
        path: The path to search for modules.

    Returns:
        A list of module attributes whose name matches the input.
    """
    res = []
    for dirpath, _, filenames in os.walk(path):
        for filename in filenames:
            if filename.endswith(".py"):
                module_name = filename[:-3]
                module_path = os.path.relpath(os.path.join(dirpath, module_name), path)
                module_import_path = module_path.replace("/", ".").replace("\\", ".")

                try:
                    module = importlib.import_module(module_import_path)
                except Exception:
                    continue

                if hasattr(module, attr_name):
                    res.append(getattr(module, attr_name))
    return res
