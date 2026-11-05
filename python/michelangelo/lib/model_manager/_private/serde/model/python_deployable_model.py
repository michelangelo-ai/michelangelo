"""Load the model of a ``python`` backend deployable package.

A ``python`` backend package holds one of two kinds of model:

- a PyTorch ``nn.Module`` packaged by the torch packager with the ``python``
  backend, stored as a state dict, or
- a custom :class:`~michelangelo.lib.model_manager.interface.custom_model.Model`
  packaged by the custom packager, stored as pickled files.
"""

from __future__ import annotations

import os
from typing import TYPE_CHECKING

from michelangelo.lib.model_manager._private.serde.loader.custom_model_loader import (
    load_custom_model,
)
from michelangelo.lib.model_manager._private.serde.loader.torch_deployable_model_loader import (  # noqa: E501
    _load_torch_python_deployable_model,
)
from michelangelo.lib.model_manager._private.utils.loader_utils import (
    import_model_class,
)
from michelangelo.lib.model_manager.interface.custom_model import Model

if TYPE_CHECKING:
    import torch


def _import_packaged_model_class(model_path: str) -> type:
    """Import the class named in the package's ``0/model_class.txt``."""
    version_dir = os.path.join(model_path, "0")
    model_class_path = os.path.join(version_dir, "model_class.txt")
    if not os.path.exists(model_class_path):
        raise ValueError(f"Missing model_class.txt in {version_dir}")

    with open(model_class_path) as f:
        model_class_str = f.read().strip()
    if not model_class_str:
        raise ValueError(f"model_class.txt is empty in {version_dir}")

    return import_model_class(version_dir, model_class_str)


def _is_torch_python_backend(model_path: str) -> bool:
    """Whether a ``python`` backend package holds a PyTorch model.

    The model class decides, not the presence of ``model.pt``: a custom model
    can contain a ``model.pt`` too.

    Args:
        model_path: The deployable model package path.

    Returns:
        ``True`` for a PyTorch model, ``False`` for a custom ``Model``.

    Raises:
        ValueError: If ``0/model_class.txt`` is missing or empty.
    """
    return not issubclass(_import_packaged_model_class(model_path), Model)


def _load_custom_python_deployable_model(model_path: str) -> Model:
    """Load the custom ``Model`` of a ``python`` backend package.

    Args:
        model_path: The deployable model package path.

    Returns:
        The loaded custom model.

    Raises:
        ValueError: If ``0/model_class.txt`` is missing or empty.
    """
    version_dir = os.path.join(model_path, "0")
    model_class = _import_packaged_model_class(model_path)
    return load_custom_model(
        os.path.join(version_dir, "model"), model_class, version_dir
    )


def load_python_deployable_model(model_path: str) -> torch.nn.Module | Model:
    """Load the model of a ``python`` backend deployable package.

    Args:
        model_path: The deployable model package path.

    Returns:
        A PyTorch module in eval mode on the CPU, or a custom ``Model``.

    Raises:
        ValueError: If ``0/model_class.txt`` is missing or empty.
    """
    if _is_torch_python_backend(model_path):
        return _load_torch_python_deployable_model(model_path)
    return _load_custom_python_deployable_model(model_path)
