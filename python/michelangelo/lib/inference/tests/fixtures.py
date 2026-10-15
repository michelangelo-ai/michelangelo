"""Tiny models used by the inference tests."""

from __future__ import annotations

from typing import TYPE_CHECKING, ClassVar, NamedTuple

import torch

from michelangelo.lib.model_manager.interface.custom_model import Model

if TYPE_CHECKING:
    import numpy as np


class DictModel(torch.nn.Module):
    """Returns ``2 * a + b`` as one tensor from a dict of named inputs."""

    def forward(self, inputs: dict[str, torch.Tensor]) -> torch.Tensor:
        """Score a batch."""
        return 2 * inputs["a"] + inputs["b"]


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
