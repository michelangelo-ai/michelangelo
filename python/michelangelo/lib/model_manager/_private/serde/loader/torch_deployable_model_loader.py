"""Loader for torch Python-backend Triton deployable model packages.

A torch Python-backend deployable package stores a state dict under
``0/model/model.pt``, the model class import path in ``0/model_class.txt``,
and optional constructor arguments in ``0/skeleton.yaml``.
"""

from __future__ import annotations

import contextlib
import json
import os
import sys

import torch
import yaml

from michelangelo.lib.model_manager._private.utils.spec_utils.spec import instantiate


@contextlib.contextmanager
def _sys_path(directory: str):
    """Temporarily prepend *directory* to ``sys.path``."""
    sys.path.insert(0, directory)
    try:
        yield
    finally:
        with contextlib.suppress(ValueError):
            sys.path.remove(directory)


def _import_model_class(version_dir: str, model_class_str: str) -> type:
    """Import *model_class_str* with *version_dir* on sys.path.

    The version directory contains the serialized model class source and must
    be on ``sys.path`` so the import succeeds inside the Triton serving
    environment.

    Args:
        version_dir: Directory containing the serialized model source.
        model_class_str: Fully qualified class name, e.g. ``my_pkg.MyModel``.

    Returns:
        The imported class.

    Raises:
        ValueError: If *model_class_str* is empty.
        ImportError: If the class cannot be imported.
    """
    if not model_class_str:
        raise ValueError("model_class.txt is empty")
    module_def, _, class_name = model_class_str.rpartition(".")
    with _sys_path(version_dir):
        import importlib

        module = importlib.import_module(module_def)
    return getattr(module, class_name)


def _load_skeleton(version_dir: str) -> dict:
    """Load constructor kwargs from skeleton.yaml or hyperparameters.json.

    Args:
        version_dir: The ``0/`` versioned directory inside the package root.

    Returns:
        The loaded skeleton dict, or an empty dict when no file is present.
    """
    for path, loader in [
        (os.path.join(version_dir, "skeleton.yaml"), lambda f: yaml.safe_load(f) or {}),
        (
            os.path.join(version_dir, "hyperparameters.json"),
            lambda f: json.load(f) or {},
        ),
    ]:
        if os.path.exists(path):
            with open(path) as f:
                return loader(f)
    return {}


def _load_torch_python_deployable_model(
    model_path: str,
    device: str | torch.device = "cpu",
) -> torch.nn.Module:
    """Load a torch Python-backend deployable model from a Triton package.

    Reads ``0/model_class.txt`` to resolve the model class, instantiates it
    from the constructor skeleton (using recursive ``_target_`` instantiation
    when present), loads the state dict from ``0/model/model.pt``, and
    returns the model in eval mode on *device*.

    Args:
        model_path: Root directory of the Triton package (contains
            ``config.pbtxt`` and the ``0/`` version directory).
        device: Device to load the model on, e.g. ``"cpu"`` or ``"cuda"``.
            Defaults to ``"cpu"``.

    Returns:
        The loaded PyTorch model in eval mode.

    Raises:
        ValueError: If ``model_class.txt`` is missing or empty.
        FileNotFoundError: If the state dict file does not exist.
        RuntimeError: If the state dict cannot be loaded into the model.
    """
    version_dir = os.path.join(model_path, "0")

    model_class_path = os.path.join(version_dir, "model_class.txt")
    if not os.path.exists(model_class_path):
        raise ValueError(f"Missing model_class.txt in {version_dir}")

    with open(model_class_path) as f:
        model_class_str = f.read().strip()

    model_cls = _import_model_class(version_dir, model_class_str)
    skeleton = _load_skeleton(version_dir)

    if "_target_" in skeleton:
        model = instantiate(skeleton)
    else:
        model = model_cls(**(skeleton or {}))

    model_file = os.path.join(version_dir, "model", "model.pt")
    if not os.path.exists(model_file):
        raise FileNotFoundError(f"No model weights found at {model_file}")

    try:
        state_dict = torch.load(model_file, map_location=device, weights_only=True)
        model.load_state_dict(state_dict)
    except Exception as err:
        raise RuntimeError(
            f"Failed to load state dict into {model_cls.__name__}: {err}"
        ) from err

    model.to(device)
    model.eval()
    return model
