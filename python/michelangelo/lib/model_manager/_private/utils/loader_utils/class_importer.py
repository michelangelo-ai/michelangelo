"""Import a model class with fallback strategies to avoid name collisions."""

from __future__ import annotations

import ast
import importlib
import logging
import os
import shutil
import sys
import tempfile
import uuid

_logger = logging.getLogger(__name__)


def create_import_rewriter(defs_path: str, prefix: str) -> ast.NodeTransformer:
    """Create an import rewriter that prefixes imports to avoid name conflicts."""
    imports = os.listdir(defs_path)

    class ImportRewriter(ast.NodeTransformer):
        def visit_Import(self, node):  # noqa: N802
            for alias in node.names:
                if any(alias.name.startswith(import_name) for import_name in imports):
                    alias.name = prefix + alias.name
            return node

        def visit_ImportFrom(self, node):  # noqa: N802
            if any(node.module.startswith(import_name) for import_name in imports):
                node.module = prefix + node.module
            return node

    return ImportRewriter()


def create_alternative_defs(defs_path: str) -> tuple[str, str]:
    """Copy defs into a uniquely-named wrapper package with rewritten imports."""
    tmpdir = tempfile.mkdtemp()
    # A unique wrapper to guarantee no conflicts between the import names
    # and other packages.
    wrapper_name = f"package_{uuid.uuid4().hex}"
    wrapper_dir = os.path.join(tmpdir, wrapper_name)
    os.makedirs(wrapper_dir)
    new_defs = os.path.join(wrapper_dir, "defs")
    shutil.copytree(defs_path, new_defs)
    rewriter = create_import_rewriter(new_defs, f"{wrapper_name}.defs.")
    for root, _, files in os.walk(new_defs):
        for file in files:
            if file.endswith(".py"):
                with open(os.path.join(root, file)) as f:
                    content = f.read()
                    tree = ast.parse(content)
                    new_tree = rewriter.visit(tree)
                    modified_code = ast.unparse(new_tree)
                    with open(os.path.join(root, file), "w") as f:
                        f.write(modified_code)
    return tmpdir, wrapper_name


def import_model_class(defs_path: str, model_class_str: str) -> type:
    """Import a model class by its fully qualified name with fallback logic.

    Tries three strategies in order:
    1. Direct import from system path
    2. Append defs_path to sys.path and retry
    3. Create alternative defs with unique wrapper to avoid import conflicts

    Args:
        defs_path: Path to the directory containing model definition files.
        model_class_str: Full import path to the model class (e.g., "mymodule.MyModel").

    Returns:
        The imported model class.
    """
    module_def, _, class_name = model_class_str.rpartition(".")

    if not module_def or not class_name:
        raise ValueError(
            f"Invalid model class definition {model_class_str}. Please specify "
            "the full import path to the model class."
        )

    try:
        module = importlib.import_module(module_def)
    except (ImportError, ModuleNotFoundError):
        _logger.info(
            f"Module {module_def} not found in the system path. Trying to load "
            "from the model package."
        )
        sys.path.append(os.path.abspath(defs_path))
        try:
            module = importlib.import_module(module_def)
        except (ImportError, ModuleNotFoundError):
            _logger.info(
                f"Module {module_def} not found after appending the model package "
                "to the system path. Trying to load model after modifying the "
                "import names."
            )
            new_defs_path, wrapper_name = create_alternative_defs(defs_path)
            sys.path.append(new_defs_path)
            module = importlib.import_module(f"{wrapper_name}.defs." + module_def)

    try:
        return getattr(module, class_name)
    except AttributeError as err:
        raise AttributeError(
            f"Class {class_name} not found in module {module_def}."
        ) from err
