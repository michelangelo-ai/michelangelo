"""The Custom Raw Model Loader."""

import ast
import importlib
import logging
import os
import shutil
import sys
import tempfile
import uuid

from michelangelo.lib.model_manager._private.serde.loader.custom_model_loader import (
    load_custom_model,
)
from michelangelo.lib.model_manager.interface.custom_model import Model

_logger = logging.getLogger(__name__)


def load_custom_raw_model(model_path: str) -> Model:
    """Load a custom raw model from the given model path.

    Args:
        model_path: The path to the model.

    Returns:
        The loaded custom raw model in the Model wrapper.
    """
    model_bin_path = os.path.join(model_path, "model")
    defs_path = os.path.join(model_path, "defs")

    if not os.path.exists(os.path.join(defs_path, "model_class.txt")):
        raise ValueError("Missing defs/model_class.txt in the model package.")

    model_class = None
    with open(os.path.join(defs_path, "model_class.txt")) as f:
        model_class = f.read().strip()

    if not model_class:
        raise ValueError("defs/model_class.txt is empty in the model package.")

    module_def, _, class_name = model_class.rpartition(".")

    if not module_def or not class_name:
        raise ValueError(
            f"Invalid model class definition {model_class}. Please specify the full import path to the model class."
        )

    try:
        module = importlib.import_module(module_def)
    except (ImportError, ModuleNotFoundError):
        _logger.info(
            f"Module {module_def} not found in the system path. Trying to load from the model package."
        )
        sys.path.append(os.path.abspath(defs_path))
        try:
            module = importlib.import_module(module_def)
        except (ImportError, ModuleNotFoundError):
            _logger.info(
                f"Module {module_def} not found after appending the model package to the system path. "
                "Trying to load model after modifying the import names."
            )
            new_defs_path, wrapper_name = create_alternative_defs(defs_path)
            sys.path.append(new_defs_path)
            module = importlib.import_module(f"{wrapper_name}.defs." + module_def)

    try:
        ModelClass = getattr(module, class_name)
    except AttributeError as err:
        raise AttributeError(
            f"Class {class_name} not found in module {module_def}."
        ) from err

    return load_custom_model(model_bin_path, ModelClass, defs_path)


def create_alternative_defs(defs_path: str) -> tuple[str, str]:
    """Create an alternative defs path.

    Alternative defs path is created with a unique wrapper
    to guarentee no conflicts between the import names and other packages.

    Args:
        defs_path: The path to the defs directory.

    Returns:
        A tuple containing the alternative defs path and the wrapper name.
    """
    tmpdir = tempfile.mkdtemp()
    # A unique wrapper to guarentee no conflicts between the import names and other packages
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


def create_import_rewriter(defs_path: str, prefix: str) -> ast.NodeTransformer:
    """Create an import rewriter.

    The import writer is to modify the import names
    to avoid conflicts with other packages.

    Args:
        defs_path: The path to the defs directory.
        prefix: The prefix to add to the import names.

    Returns:
        An import rewriter.
    """
    imports = os.listdir(defs_path)

    class ImportRewriter(ast.NodeTransformer):
        def visit_Import(self, node):
            for alias in node.names:
                if any(alias.name.startswith(import_name) for import_name in imports):
                    alias.name = prefix + alias.name
            return node

        def visit_ImportFrom(self, node):
            if any(node.module.startswith(import_name) for import_name in imports):
                node.module = prefix + node.module
            return node

    return ImportRewriter()
