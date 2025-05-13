from __future__ import annotations
import importlib


def get_module(module_attr: str) -> any:
    """
    Load the attribute from the module

    Args:
        module_attr: the full attribute definition,
            for example, 'module.submodule.attribute'

    Returns:
        The module
    """
    module_def, _, _ = module_attr.rpartition(".")
    module = importlib.import_module(module_def)
    return module
