"""The executable ``TorchTransformModule`` DAG runner.

Converts a fitted
:class:`~michelangelo.lib.native_transform.torch.transform_spec.TransformSpec`
into a single ``torch.nn.Module`` that runs its layers in topological order.
The module is TorchScript-exportable, so the exact same transform graph runs
at training time (batched, ahead of the model) and at serving time (embedded
in the model artifact).
"""

from __future__ import annotations

from typing import TYPE_CHECKING

import torch

from michelangelo.lib.native_transform.torch.base_layers import TorchTransformBaseLayer
from michelangelo.lib.native_transform.torch.utils import generate_layer_name

if TYPE_CHECKING:
    from michelangelo.lib.native_transform.torch.transform_spec import TransformSpec

__all__ = ["TorchTransformModule", "get_transform_module"]


class TorchTransformModule(torch.nn.Module):
    """Executes a DAG of transform layers, in topological order.

    Args:
        name: The module's name.
        input_cols: Column names the module expects as input.
        output_cols: Column names the module returns as output.
        layers: The transform layers to run, already in topological order.
            Every element must be a
            :class:`~michelangelo.lib.native_transform.torch.base_layers.TorchTransformBaseLayer`.

    Raises:
        AssertionError: If any element of ``layers`` is not a
            ``TorchTransformBaseLayer``.
    """

    def __init__(
        self,
        name: str,
        input_cols: list[str],
        output_cols: list[str],
        layers: torch.nn.ModuleList,
    ) -> None:
        """Initialize the TorchTransformModule.

        Args:
            name: The module's name.
            input_cols: Column names the module expects as input.
            output_cols: Column names the module returns as output.
            layers: The transform layers to run, already in topological
                order. Every element must be a ``TorchTransformBaseLayer``.

        Raises:
            AssertionError: If any element of ``layers`` is not a
                ``TorchTransformBaseLayer``.
        """
        super().__init__()
        assert all(isinstance(m, TorchTransformBaseLayer) for m in layers), (
            "All modules must be instances of TorchTransformBaseLayer"
        )
        self.name = name
        self.input_cols = input_cols
        self.output_cols = output_cols
        self.layers = layers

    def forward(self, inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]:
        """Run every layer in order, threading outputs into later layers' inputs.

        Args:
            inputs: Mapping from column name to tensor; must contain every
                column in ``self.input_cols``.

        Returns:
            A mapping from each column in ``self.output_cols`` to its
            computed tensor.

        Raises:
            ValueError: If ``inputs`` is missing a declared input column, or
                a layer's declared input column is not yet available (i.e.
                ``layers`` is not in a valid topological order).
        """
        # Node registry stores inputs and intermediate outputs from each layer.
        nodes: dict[str, torch.Tensor] = {}
        for name in self.input_cols:
            if name not in inputs:
                raise ValueError(f"Missing input name {name}. Inputs={inputs}")
            nodes[name] = inputs[name]

        for layer in self.layers:
            layer_inputs: dict[str, torch.Tensor] = {}
            for col in layer.input_cols:
                if col not in nodes:
                    raise ValueError(
                        f"Missing input name {col} for layer {layer.name}."
                    )
                layer_inputs[col] = nodes[col]
            layer_outputs = layer(layer_inputs)
            nodes.update(layer_outputs)

        results: dict[str, torch.Tensor] = {}
        for output_col in self.output_cols:
            results[output_col] = nodes[output_col]
        return results


def get_transform_module(
    transform_spec: TransformSpec,
    start_level: int,
    end_level: int | None = None,
    output_cols: set[str] | None = None,
) -> TorchTransformModule | None:
    """Materialize a level range of a ``TransformSpec`` into a ``TorchTransformModule``.

    Args:
        transform_spec: The fitted spec DAG to materialize.
        start_level: The first transform level to include (inclusive).
        end_level: The last transform level to include (inclusive). Defaults
            to the spec's maximum level; clamped to it if given a larger
            value.
        output_cols: The columns the module should return. Defaults to every
            output column produced across the included levels.

    Returns:
        The materialized module, or ``None`` if the level range contains no
        layers.
    """
    layers = []
    transform_input_cols: set[str] = set()
    transform_output_cols: set[str] = set()
    end_level = (
        transform_spec.get_max_transform_level()
        if end_level is None
        else min(end_level, transform_spec.get_max_transform_level())
    )
    for level in range(start_level, end_level + 1):
        layers.extend(transform_spec.to_transform_layers(level))
        transform_input_cols.update(transform_spec.get_transform_input_cols(level))
        transform_output_cols.update(transform_spec.get_transform_output_cols(level))
    if len(layers) == 0:
        return None
    input_cols = transform_input_cols - transform_output_cols
    return TorchTransformModule(
        name=generate_layer_name(TorchTransformModule.__name__.lower()),
        input_cols=sorted(input_cols),
        output_cols=sorted(transform_output_cols)
        if output_cols is None
        else sorted(output_cols),
        layers=torch.nn.ModuleList(layers),
    )
