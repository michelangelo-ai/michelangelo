"""Tiny models used by the inference tests."""

from __future__ import annotations

from typing import TYPE_CHECKING, ClassVar, Dict, NamedTuple, Union  # noqa: UP035

import torch

from michelangelo.lib.model_manager.interface.custom_model import Model

if TYPE_CHECKING:
    from pathlib import Path

    import numpy as np


class DictModel(torch.nn.Module):
    """Returns ``2 * a + b`` as one tensor from a dict of named inputs."""

    def forward(self, inputs: dict[str, torch.Tensor]) -> torch.Tensor:
        """Score a batch."""
        return 2 * inputs["a"] + inputs["b"]


class PositionalModel(torch.nn.Module):
    """Returns ``2 * a + b`` from one argument per input."""

    def forward(self, a: torch.Tensor, b: torch.Tensor) -> torch.Tensor:
        """Score a batch."""
        return 2 * a + b


class UnionDictModel(torch.nn.Module):
    """Takes a stacked tensor or a dict of named inputs.

    Models that train on stacked features and serve on named ones look like
    this. TorchScript needs ``typing.Union`` here.
    """

    def forward(
        self,
        x: Union[torch.Tensor, Dict[str, torch.Tensor]],  # noqa: UP006, UP007
    ) -> torch.Tensor:
        """Score a batch."""
        if isinstance(x, torch.Tensor):
            return x
        return 2 * x["a"] + x["b"]


class TupleModel(torch.nn.Module):
    """Returns two outputs as a plain tuple."""

    def forward(self, inputs: dict[str, torch.Tensor]) -> tuple[torch.Tensor, ...]:
        """Score a batch."""
        return inputs["a"] + 1, inputs["a"] - 1


class Pair(NamedTuple):
    """Named outputs."""

    low: torch.Tensor
    high: torch.Tensor


class NamedTupleModel(torch.nn.Module):
    """Returns a NamedTuple."""

    def forward(self, inputs: dict[str, torch.Tensor]) -> Pair:
        """Score a batch."""
        return Pair(low=inputs["a"] - 1, high=inputs["a"] + 1)


class DictOutputModel(torch.nn.Module):
    """Returns a dict of named outputs."""

    def forward(self, inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]:
        """Score a batch."""
        return {"double": inputs["a"] * 2}


class Scale(torch.nn.Module):
    """Transform: doubles ``a`` (dict in, dict out)."""

    def forward(self, inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]:
        """Transform a batch."""
        return {"a": inputs["a"] * 2}


class SumPredictor(torch.nn.Module):
    """Predictor over a dict with ``a`` and ``b``."""

    def forward(self, inputs: dict[str, torch.Tensor]) -> torch.Tensor:
        """Score a batch."""
        return inputs["a"] + inputs["b"]


class EchoModel(Model):
    """Custom model that returns its input under a new name."""

    def save(self, path: str) -> None:
        """Save nothing."""

    @classmethod
    def load(cls, path: str) -> EchoModel:
        """Load an instance."""
        return cls()

    def predict(self, inputs: dict[str, np.ndarray]) -> dict[str, np.ndarray]:
        """Echo ``a`` as ``response``."""
        return {"response": inputs["a"]}


_FIX = __name__
_PREFIX = __name__.rpartition(".")[0]


def torch_deployable(root: Path, model: torch.nn.Module, schema, backend=None) -> str:
    """Package ``model`` with the real torch packager, as the assembler does.

    Args:
        root: Empty directory for the source file and the package.
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


def custom_deployable(root: Path, schema, batch: bool = False) -> str:
    """Package :class:`EchoModel` with the real custom packager.

    Args:
        root: Empty directory for the model files and the package.
        schema: The model schema.
        batch: The packager's ``custom_batch_processing``.

    Returns:
        The package directory.
    """
    from michelangelo.lib.model_manager.packager.custom_triton.custom_triton_packager import (  # noqa: E501
        CustomTritonPackager,
    )

    (root / "model").mkdir(parents=True, exist_ok=True)
    return CustomTritonPackager(custom_batch_processing=batch).create_model_package(
        str(root / "model"),
        model_class=f"{_FIX}.{RecordModel.__name__}",
        model_schema=schema,
        dest_model_path=str(root / "package"),
        include_import_prefixes=[_PREFIX],
    )


class RecordModel(Model):
    """Custom model that records the shapes ``predict`` was called with."""

    calls: ClassVar[list[tuple[int, ...]]] = []

    def save(self, path: str) -> None:
        """Save nothing."""

    @classmethod
    def load(cls, path: str) -> RecordModel:
        """Load an instance."""
        return cls()

    def predict(self, inputs: dict[str, np.ndarray]) -> dict[str, np.ndarray]:
        """Return ``a + 1`` as ``response``."""
        RecordModel.calls.append(inputs["a"].shape)
        return {"response": inputs["a"] + 1}


def good_factory(context):
    """A conforming backend factory."""
    return GoodBackend()


class GoodBackend:
    """Minimal conforming backend."""

    input_names: ClassVar[list[str]] = ["a"]
    output_names: ClassVar[list[str]] = ["y"]

    def predict(self, inputs):
        """Return the input under another name."""
        return {"y": inputs["a"]}


def bad_factory(context):
    """A factory whose result lacks ``predict`` and ``output_names``."""
    return type("NoPredict", (), {"input_names": ["a"]})()


not_callable = 5


class EchoBackend:
    """Backend that returns chosen input columns under chosen names."""

    def __init__(self, mapping, input_names=None):
        """Map output name to input name."""
        self._mapping = mapping
        self.input_names = input_names
        self.output_names = list(mapping)
        self.seen = None

    def predict(self, inputs):
        """Echo inputs and remember what was received."""
        self.seen = inputs
        return {out: inputs[src] for out, src in self._mapping.items()}


def echo_factory(context):
    """Factory for a backend that copies ``a`` to ``y``."""
    return EchoBackend({"y": "a"}, input_names=["a"])
