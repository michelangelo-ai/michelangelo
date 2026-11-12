"""Tiny model packages for the tabular inference tests."""

from __future__ import annotations

from typing import TYPE_CHECKING

import numpy as np
import torch

from michelangelo.lib.model_manager.interface.custom_model import Model

if TYPE_CHECKING:
    from pathlib import Path

_FIX = __name__
_PREFIX = __name__.rpartition(".")[0]


class PositionalModel(torch.nn.Module):
    """Returns ``2 * a + b`` from one argument per input."""

    def forward(self, a: torch.Tensor, b: torch.Tensor) -> torch.Tensor:
        """Score a batch."""
        return 2 * a + b


class RecordModel(Model):
    """Custom model that returns ``a + 1`` as ``response``."""

    def save(self, path: str) -> None:
        """Save nothing."""

    @classmethod
    def load(cls, path: str) -> RecordModel:
        """Load an instance."""
        return cls()

    def predict(self, inputs: dict[str, np.ndarray]) -> dict[str, np.ndarray]:
        """Return ``a + 1`` as ``response``."""
        return {"response": inputs["a"] + 1}


def torch_deployable(root: Path, model: torch.nn.Module, schema) -> str:
    """Package ``model`` with the real torch packager, as the assembler does.

    Args:
        root: Empty directory for the source file and the package.
        model: A module defined in this file, so the packager can import it.
        schema: The model schema.

    Returns:
        The package directory.
    """
    from michelangelo.lib.model_manager.packager.torch_triton.torch_triton_packager import (  # noqa: E501
        TorchTritonPackager,
    )

    root.mkdir(parents=True, exist_ok=True)
    source = root / "model.pt"
    torch.save(model, source)
    return TorchTritonPackager().create_model_package(
        str(source),
        schema,
        model_class=f"{_FIX}.{type(model).__name__}",
        dest_model_path=str(root / "package"),
        sample_data=[{item.name: torch.ones(1, 1) for item in schema.input_schema}],
        include_import_prefixes=[_PREFIX],
    )


def custom_raw(root: Path, schema) -> str:
    """Package :class:`RecordModel` as a raw package with the custom packager.

    Args:
        root: Empty directory for the model files and the package.
        schema: The model schema.

    Returns:
        The package directory.
    """
    from michelangelo.lib.model_manager.packager.custom_triton.custom_triton_packager import (  # noqa: E501
        CustomTritonPackager,
    )

    (root / "model").mkdir(parents=True, exist_ok=True)
    return CustomTritonPackager().create_raw_model_package(
        str(root / "model"),
        model_class=f"{_FIX}.{RecordModel.__name__}",
        model_schema=schema,
        sample_data=[{item.name: np.ones((1,)) for item in schema.input_schema}],
        dest_model_path=str(root / "raw_package"),
        include_import_prefixes=[_PREFIX],
    )
