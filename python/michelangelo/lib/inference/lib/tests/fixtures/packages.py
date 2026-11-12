from __future__ import annotations

import torch

_FIX = __name__
_PREFIX = __name__.rpartition(".")[0]


class PositionalModel(torch.nn.Module):
    """Returns ``2 * a + b`` from one argument per input."""

    def forward(self, a: torch.Tensor, b: torch.Tensor) -> torch.Tensor:
        return 2 * a + b


class DictModel(torch.nn.Module):
    """Returns ``2 * a + b`` from a dict of named inputs."""

    def forward(self, inputs: dict[str, torch.Tensor]) -> torch.Tensor:
        return 2 * inputs["a"] + inputs["b"]


def torch_deployable(root, model: torch.nn.Module, schema, backend=None) -> str:
    """Package ``model`` with the torch packager.

    Args:
        root: A ``pathlib.Path`` to an empty directory.
        model: A module defined in this file, so the packager can import it.
        schema: The model schema.
        backend: ``None`` (TorchScript), ``"onnxruntime"`` or ``"python"``.

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
        backend=backend,
        dest_model_path=str(root / "package"),
        sample_data=[{item.name: torch.ones(1, 1) for item in schema.input_schema}],
        include_import_prefixes=[_PREFIX],
    )
