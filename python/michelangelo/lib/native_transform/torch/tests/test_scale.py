"""Tests for :mod:`michelangelo.lib.native_transform.torch.scale`.

Covers ``ClipAndScale`` forward semantics (clipping, scaling, dtype casting,
1D unsqueeze, multi-column concatenation, invalid dtype) and its
``torch.jit.script``/``torch.jit.trace`` round trips.
"""

from __future__ import annotations

import pytest

# These layers operate on real torch tensors/modules. Skip cleanly if torch is
# unavailable in a lightweight environment.
torch = pytest.importorskip("torch")

from michelangelo.lib.native_transform.torch.scale import ClipAndScale  # noqa: E402


class TestClipAndScale:
    """Forward semantics of the ClipAndScale layer."""

    def test_forward_with_min_max_clip(self) -> None:
        """Values outside [min_value, max_value] are clamped into range."""
        layer = ClipAndScale(
            min_value=12,
            max_value=18,
            scale_factor=1,
            output_type=None,
            input_cols=["input"],
            output_cols=["output"],
        )
        inputs = {"input": torch.tensor([[0.0], [15.0], [20.0]], dtype=torch.float32)}
        outputs = layer(inputs)
        expected = torch.tensor([[12.0], [15.0], [18.0]], dtype=torch.float32)
        torch.testing.assert_close(outputs["output"], expected)

    def test_forward_with_scale(self) -> None:
        """The clipped tensor is multiplied by scale_factor."""
        layer = ClipAndScale(
            min_value=12,
            max_value=18,
            scale_factor=1 / 2,
            output_type=None,
            input_cols=["input"],
            output_cols=["output"],
        )
        inputs = {"input": torch.tensor([[0.0], [16.0], [20.0]], dtype=torch.float32)}
        outputs = layer(inputs)
        expected = torch.tensor([[6.0], [8.0], [9.0]], dtype=torch.float32)
        torch.testing.assert_close(outputs["output"], expected)

    def test_forward_with_scale_int_output(self) -> None:
        """output_type casts the scaled result to the requested dtype."""
        layer = ClipAndScale(
            min_value=12,
            max_value=18,
            scale_factor=1 / 3,
            output_type="int32",
            input_cols=["input"],
            output_cols=["output"],
        )
        inputs = {"input": torch.tensor([[0.0], [16.0], [20.0]], dtype=torch.float32)}
        outputs = layer(inputs)
        expected = torch.tensor([[4], [5], [6]], dtype=torch.int32)
        torch.testing.assert_close(outputs["output"], expected)

    def test_multiple_input_cols(self) -> None:
        """Multiple input columns are concatenated before clip/scale."""
        layer = ClipAndScale(
            min_value=0,
            max_value=10,
            scale_factor=0.1,
            output_type=None,
            input_cols=["col1", "col2"],
            output_cols=["output"],
        )
        inputs = {
            "col1": torch.tensor([[5.0], [15.0]]),
            "col2": torch.tensor([[-5.0], [8.0]]),
        }
        outputs = layer(inputs)
        expected = torch.tensor([[0.5, 0.0], [1.0, 0.8]], dtype=torch.float32)
        torch.testing.assert_close(outputs["output"], expected)

    def test_1d_tensor_unsqueeze(self) -> None:
        """A 1D input column is unsqueezed to a column vector before clip/scale."""
        layer = ClipAndScale(
            min_value=0,
            max_value=5,
            scale_factor=2,
            output_type=None,
            input_cols=["input"],
            output_cols=["output"],
        )
        inputs = {"input": torch.tensor([1.0, 3.0, 10.0])}
        outputs = layer(inputs)
        expected = torch.tensor([[2.0], [6.0], [10.0]], dtype=torch.float32)
        torch.testing.assert_close(outputs["output"], expected)

    @pytest.mark.parametrize(
        ("output_type", "expected_dtype"),
        [
            ("float32", torch.float32),
            ("float64", torch.float64),
            ("int32", torch.int32),
            ("int64", torch.int64),
            ("bool", torch.bool),
            (torch.float16, torch.float16),
        ],
    )
    def test_dtype_resolution(self, output_type, expected_dtype) -> None:
        """Both string aliases and torch.dtype values resolve correctly."""
        layer = ClipAndScale(
            min_value=0,
            max_value=1,
            scale_factor=1,
            output_type=output_type,
            input_cols=["input"],
            output_cols=["output"],
        )
        inputs = {"input": torch.tensor([[0.5]], dtype=torch.float32)}
        outputs = layer(inputs)
        assert outputs["output"].dtype == expected_dtype

    def test_invalid_dtype_raises_error(self) -> None:
        """An unrecognized output_type string raises ValueError."""
        with pytest.raises(ValueError, match="Unsupported dtype"):
            layer = ClipAndScale(
                min_value=0,
                max_value=1,
                scale_factor=1,
                output_type="invalid_type",
                input_cols=["input"],
                output_cols=["output"],
            )
            layer({"input": torch.tensor([[0.5]])})


class TestClipAndScaleTorchScriptRoundTrip:
    """ClipAndScale must script/trace, save/load, and match eager output."""

    def test_scripted_matches_eager(self, tmp_path) -> None:
        """Scripting (and reloading) reproduces the eager forward output."""
        layer = ClipAndScale(
            min_value=12,
            max_value=18,
            scale_factor=1,
            output_type=None,
            input_cols=["input"],
            output_cols=["output"],
        )
        layer.eval()
        inputs = {"input": torch.tensor([[0.0], [15.0], [20.0]], dtype=torch.float32)}
        eager = layer(inputs)

        scripted = torch.jit.script(layer)
        scripted_out = scripted(inputs)
        torch.testing.assert_close(scripted_out["output"], eager["output"])

        model_path = tmp_path / "scripted_clip_and_scale.pt"
        scripted.save(str(model_path))
        loaded = torch.jit.load(str(model_path))
        loaded_out = loaded(inputs)
        torch.testing.assert_close(loaded_out["output"], eager["output"])

    @pytest.mark.parametrize(
        ("output_type", "expected_dtype"),
        [
            ("float32", torch.float32),
            ("float64", torch.float64),
            ("int32", torch.int32),
            ("int64", torch.int64),
        ],
    )
    def test_traced_matches_eager_across_dtypes(
        self, output_type, expected_dtype
    ) -> None:
        """Tracing reproduces the eager output for every supported dtype."""
        layer = ClipAndScale(
            min_value=0,
            max_value=1,
            scale_factor=1,
            output_type=output_type,
            input_cols=["input"],
            output_cols=["output"],
        )
        inputs = {"input": torch.tensor([[0.5]], dtype=torch.float32)}
        eager = layer(inputs)
        traced = torch.jit.trace(layer, (inputs,), strict=False)
        traced_out = traced(inputs)
        torch.testing.assert_close(traced_out["output"], eager["output"])
        assert traced_out["output"].dtype == expected_dtype
