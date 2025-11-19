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

    if not module_def:
        raise ValueError(
            f"Invalid import path: {module_attr}, expecting a full import path like 'module.submodule.attribute'"
        )

    module = importlib.import_module(module_def)
    attr = getattr(module, attr_def)
    return attr
