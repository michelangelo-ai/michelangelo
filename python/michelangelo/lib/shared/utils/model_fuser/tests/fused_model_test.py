"""Unit tests for ``FusedModel``.

Covers forward with dict vs. positional predictor, and merge of transform
output with input passthrough.
"""

from __future__ import annotations

import unittest

import torch
import torch.nn as nn

from michelangelo.lib.shared.utils.model_fuser import FusedModel


class _DictTransform(nn.Module):
    """Transform: dict in -> dict out. Maps input ``a`` to output ``b`` (``a + 1``)."""

    def forward(self, inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]:
        a = inputs["a"]
        return {"b": a + 1.0}


class _TwoTensorPredictor(nn.Module):
    """Predictor: two tensors in, tensor out. Sum of both."""

    def forward(self, a: torch.Tensor, b: torch.Tensor) -> torch.Tensor:
        return (a + b).sum(dim=-1, keepdim=True)


class _DictPredictor(nn.Module):
    """Predictor: dict in, tensor out. Returns the sum of the first value."""

    def forward(self, inputs: dict[str, torch.Tensor]) -> torch.Tensor:
        v = next(iter(inputs.values()))
        return v.sum(dim=-1, keepdim=True)


class FusedModelPredictorTakesDictTest(unittest.TestCase):
    """``FusedModel`` when the predictor accepts a dict."""

    def test_forward_calls_predictor_with_dict(self):
        """The predictor receives a dict merging transform output and passthrough."""
        fused = FusedModel(
            transform_module=_DictTransform(),
            predictor_module=_DictPredictor(),
            transform_input_keys=["a"],
            predictor_input_keys=["a", "b"],
            predictor_takes_dict=True,
        )
        fused.eval()
        with torch.no_grad():
            inputs = {"a": torch.tensor([[1.0, 2.0]], dtype=torch.float32)}
            out = fused(inputs)
        self.assertIsInstance(out, torch.Tensor)
        self.assertEqual(out.shape, (1, 1))
        # Transform: a -> b = a+1 = [2, 3]. Predictor gets {a: [1,2], b: [2,3]},
        # first value is a, sum = 3.
        self.assertEqual(out.item(), 3.0)

    def test_merge_transform_output_overwrites_passthrough(self):
        """Predictor input = merge(inputs, transform_output); transform keys win."""
        fused = FusedModel(
            transform_module=_DictTransform(),
            predictor_module=_DictPredictor(),
            transform_input_keys=["a"],
            predictor_input_keys=["a", "b"],
            predictor_takes_dict=True,
        )
        fused.eval()
        with torch.no_grad():
            inputs = {"a": torch.tensor([[1.0]], dtype=torch.float32)}
            out = fused(inputs)
        self.assertEqual(out.shape, (1, 1))
        # Transform: a -> b = a+1 = 2. Predictor gets {a: 1, b: 2}; first value
        # a=1, sum=1.
        self.assertEqual(out.item(), 1.0)


class FusedModelPredictorTakesTensorsTest(unittest.TestCase):
    """``FusedModel`` when the predictor accepts positional tensors."""

    def test_forward_calls_predictor_with_tensors_in_order(self):
        """Predictor receives positional tensors in ``predictor_input_keys`` order."""
        fused = FusedModel(
            transform_module=_DictTransform(),
            predictor_module=_TwoTensorPredictor(),
            transform_input_keys=["a"],
            predictor_input_keys=["a", "b"],
            predictor_takes_dict=False,
        )
        fused.eval()
        with torch.no_grad():
            inputs = {"a": torch.tensor([[1.0, 2.0]], dtype=torch.float32)}
            out = fused(inputs)
        self.assertIsInstance(out, torch.Tensor)
        self.assertEqual(out.shape, (1, 1))
        # Transform: a -> b = a+1 = [2, 3]. Predictor gets (a, b) = ([1,2],
        # [2,3]), (a+b).sum() = (3+5) = 8.
        self.assertEqual(out.item(), 8.0)

    def test_predictor_input_order_follows_predictor_input_keys(self):
        """Reordering ``predictor_input_keys`` reorders the predictor's args."""
        call_args = []

        class _CapturePredictor(nn.Module):
            def forward(
                self, first: torch.Tensor, second: torch.Tensor
            ) -> torch.Tensor:
                call_args.append((first, second))
                return first + second

        fused = FusedModel(
            transform_module=_DictTransform(),
            predictor_module=_CapturePredictor(),
            transform_input_keys=["a"],
            predictor_input_keys=["b", "a"],
            predictor_takes_dict=False,
        )
        fused.eval()
        with torch.no_grad():
            inputs = {"a": torch.tensor([[1.0]], dtype=torch.float32)}
            _ = fused(inputs)
        self.assertEqual(len(call_args), 1)
        first, second = call_args[0]
        self.assertEqual(first.shape, (1, 1))
        self.assertEqual(second.shape, (1, 1))
        self.assertEqual(first.item(), 2.0)
        self.assertEqual(second.item(), 1.0)


class FusedModelMergeTest(unittest.TestCase):
    """Merge behavior: passthrough inputs plus transform output."""

    def test_passthrough_keys_not_in_transform_input_are_in_predictor_input(self):
        """Inputs the transform doesn't consume pass through to the predictor."""

        class _Transform(nn.Module):
            def forward(
                self, inputs: dict[str, torch.Tensor]
            ) -> dict[str, torch.Tensor]:
                return {"out": inputs["x"] + 1}

        class _Pred(nn.Module):
            def forward(self, x: torch.Tensor, out: torch.Tensor) -> torch.Tensor:
                return x + out

        fused = FusedModel(
            transform_module=_Transform(),
            predictor_module=_Pred(),
            transform_input_keys=["x"],
            predictor_input_keys=["x", "out"],
            predictor_takes_dict=False,
        )
        fused.eval()
        with torch.no_grad():
            inputs = {"x": torch.tensor([[1.0]], dtype=torch.float32)}
            out = fused(inputs)
        self.assertEqual(out.item(), 1.0 + 2.0)


if __name__ == "__main__":
    unittest.main()
