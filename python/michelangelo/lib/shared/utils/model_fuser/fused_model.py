"""Fused model composing a native-transform module and a predictor module.

``forward`` accepts ``dict[str, Tensor]`` keyed by feature name. The transform
runs on its input schema; its output is merged with passthrough features from
the input (the predictor receives the transform's output where available,
else the original input for that feature). Output is the predictor's output
tensor. Designed to be TorchScript-exportable.
"""

from __future__ import annotations

import torch
import torch.nn as nn

__all__ = ["FusedModel"]


class FusedModel(nn.Module):
    """Fuses a transform module and a predictor module with schema-driven merge.

    The native transform is always dict-in, dict-out: ``forward`` is called
    with a dict and returns a dict. The predictor's input is merged from the
    transform's output and passthrough values from the fused model's input;
    the predictor is then called with either a single dict
    (``predictor_takes_dict=True``) or positional tensor arguments in
    ``predictor_input_keys`` order (``predictor_takes_dict=False``).

    Attributes:
        transform_module: Native transform module.
        predictor_module: Predictor module.
        transform_input_keys: Input feature names fed to the transform.
        predictor_input_keys: Feature names the predictor expects, in order.
        predictor_takes_dict: Whether the predictor is called with a dict.
    """

    __constants__ = [  # noqa: RUF012
        "transform_input_keys",
        "predictor_input_keys",
        "predictor_takes_dict",
    ]

    def __init__(
        self,
        transform_module: nn.Module,
        predictor_module: nn.Module,
        transform_input_keys: list[str],
        predictor_input_keys: list[str],
        predictor_takes_dict: bool = False,
    ) -> None:
        """Initialize the fused model.

        Args:
            transform_module: Native transform module. Always
                ``forward(inputs: dict) -> dict``.
            predictor_module: Predictor module. Its ``forward`` may accept a
                dict or multiple positional tensors.
            transform_input_keys: Input feature names fed to the transform
                (dict keys).
            predictor_input_keys: Feature names the predictor expects, in
                order.
            predictor_takes_dict: If ``True``, call the predictor with a
                dict; if ``False``, call it with positional tensors.
        """
        super().__init__()
        self.transform_module = transform_module
        self.predictor_module = predictor_module
        self.transform_input_keys = transform_input_keys
        self.predictor_input_keys = predictor_input_keys
        self.predictor_takes_dict = predictor_takes_dict

    def forward(self, inputs: dict[str, torch.Tensor]) -> torch.Tensor:
        """Run the transform then the predictor, merging output with passthrough.

        Args:
            inputs: Named input tensors (e.g. batched features). Must contain
                all ``transform_input_keys`` and any ``predictor_input_keys``
                not produced by the transform.

        Returns:
            The predictor's output tensor.
        """
        # The native transform is always dict-in, dict-out.
        transform_in_dict = {k: inputs[k] for k in self.transform_input_keys}
        transformed_dict = self.transform_module(transform_in_dict)

        predictor_input_dict = {}
        for k in list(inputs.keys()):
            predictor_input_dict[k] = inputs[k]
        for k in list(transformed_dict.keys()):
            predictor_input_dict[k] = transformed_dict[k]

        if self.predictor_takes_dict:
            predictor_in_dict = {
                k: predictor_input_dict[k] for k in self.predictor_input_keys
            }
            return self.predictor_module(predictor_in_dict)
        predictor_parts = [predictor_input_dict[k] for k in self.predictor_input_keys]
        return self.predictor_module(*predictor_parts)
