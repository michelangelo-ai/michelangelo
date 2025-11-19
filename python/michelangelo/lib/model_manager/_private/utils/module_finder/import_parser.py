import ast
import inspect
from types import ModuleType
from typing import Optional


def get_imports(module: ModuleType) -> list[str]:
    """
    Extract the imported modules from the python module
    Only the modules/packages right after the import/from statements
    are considered, because the alias are not guaranteed to be a module

    Args:
        module: the module object

    Returns:
        The list of imported module names in absolute form
    """
    filepath = inspect.getfile(module)

    with open(filepath) as file:
        tree = ast.parse(file.read(), filename=filepath)

    modules = []
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            modules.extend([alias.name for alias in node.names])
        elif isinstance(node, ast.ImportFrom):
            module_name = get_node_module(node, module)
            if module_name is not None:
                modules.append(module_name)

    return modules


def get_node_module(
    node: ast.ImportFrom,
    module: ModuleType,
) -> Optional[str]:
    if not node.module:
        return None

    if node.level == 0:
        return node.module

    path_splits = module.__name__.split(".")
    node_module_name = ".".join([*path_splits[: -node.level], node.module]) if len(path_splits) > 0 else node.module
    return node_module_name
