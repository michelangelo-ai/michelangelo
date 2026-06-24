"""Loader for PyTorch raw model packages.

A torch raw model package stores a model as a state dict plus the metadata
needed to reconstruct the model object: a ``model_class.txt`` naming the model
class, and an optional ``skeleton.yaml`` or ``hyperparameters.yaml`` describing
its constructor arguments.  Nested dicts with a ``_target_`` key are
recursively instantiated at load time.
"""

from __future__ import annotations

import importlib
import json
import os

import torch
import yaml

from michelangelo.lib.model_manager._private.utils.spec_utils.spec import instantiate


def _load_skeleton(package_path: str) -> dict:
    """Load the constructor skeleton for the model class.

    Checks multiple locations for backwards compatibility:
    ``skeleton.yaml`` (deployable packages), ``metadata/hyperparameters.yaml``
    (raw packages), and ``hyperparameters.json`` (legacy).

    Args:
        package_path: The root path of the model package.

    Returns:
        The skeleton dict, or an empty dict if none is present.
    """
    candidates = [
        os.path.join(package_path, "skeleton.yaml"),
        os.path.join(package_path, "metadata", "hyperparameters.yaml"),
        os.path.join(package_path, "hyperparameters.json"),
    ]
    for path in candidates:
        if os.path.exists(path):
            with open(path) as f:
                if path.endswith(".json"):
                    return json.load(f) or {}
                return yaml.safe_load(f) or {}
    return {}


def load_torch_raw_model(package_path: str) -> torch.nn.Module:
    """Load a PyTorch raw model package into a model instance.

    Reads ``model_class.txt`` to resolve the model class, reconstructs the
    model from its skeleton (``_target_`` spec or plain kwargs), loads
    the saved ``state_dict``, and returns the model in eval mode.

    Args:
        package_path: The root path of the model package.

    Returns:
        The loaded PyTorch model in eval mode.

    Raises:
        ValueError: If ``model_class.txt`` is missing or empty.
        FileNotFoundError: If the model weights file is missing.
        TypeError: If the loaded weights are not a state dict.
        RuntimeError: If the state dict cannot be loaded into the model.
    """
    model_class_path = os.path.join(package_path, "defs", "model_class.txt")
    if not os.path.exists(model_class_path):
        model_class_path = os.path.join(package_path, "model_class.txt")
    if not os.path.exists(model_class_path):
        raise ValueError("Missing model_class.txt in the model package.")

    with open(model_class_path) as f:
        model_class = f.read().strip()

    if not model_class:
        raise ValueError("model_class.txt is empty in the model package.")

    module_def, _, class_name = model_class.rpartition(".")
    if not module_def or not class_name:
        raise ValueError(
            f"Invalid model class definition {model_class}. "
            "Please specify the full import path to the model class."
        )

    module = importlib.import_module(module_def)
    model_cls = getattr(module, class_name)

    skeleton = _load_skeleton(package_path)
    model = instantiate(skeleton) if "_target_" in skeleton else model_cls(**skeleton)

    model_file = os.path.join(package_path, "model", "model.pt")
    if not os.path.exists(model_file):
        raise FileNotFoundError(f"No model weights found at {model_file}")

    state_dict = torch.load(model_file, map_location="cpu", weights_only=True)
    if not isinstance(state_dict, dict):
        raise TypeError(f"Expected state_dict format, but got {type(state_dict)}")

    try:
        model.load_state_dict(state_dict)
    except Exception as err:
        raise RuntimeError(
            f"Failed to load state_dict into {class_name}: {err}"
        ) from err

    model.eval()
    return model
