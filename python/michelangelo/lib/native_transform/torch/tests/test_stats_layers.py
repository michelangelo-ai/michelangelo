"""Tests for :mod:`michelangelo.lib.native_transform.torch.stats_layers`.

Covers the forward semantics of the fitted-statistics transform layers and,
for every layer, a ``torch.jit.script`` round-trip: native transform layers
must be TorchScript-exportable so the exact transform runs at serve time, and
the scripted module (including after save/load) must reproduce the eager
output.
"""

from __future__ import annotations

import pytest

# These layers operate on real torch tensors/modules. Skip cleanly if torch is
# unavailable in a lightweight environment.
torch = pytest.importorskip("torch")

from michelangelo.lib.constants.sentinel import INT32_SENTINEL  # noqa: E402
from michelangelo.lib.native_transform.torch.stats_layers import (  # noqa: E402
    Bucketization,
    MinMax,
    Normalization,
)


class TestMinMax:
    """Min-max scaling, including dim handling and safe division."""

    def test_multi_output_cols_raises(self) -> None:
        """More than one output column raises ``ValueError``."""
        with pytest.raises(ValueError, match="exactly one column"):
            MinMax(input_cols=["x"], output_cols=["a", "b"], min=[0.0], max=[1.0])

    def test_forward_basic(self) -> None:
        """Values scale linearly into [0, 1] given min/max."""
        layer = MinMax(input_cols=["x"], output_cols=["o"], min=[0.0], max=[10.0])
        out = layer({"x": torch.tensor([[0.0], [5.0], [10.0]])})["o"]
        torch.testing.assert_close(out, torch.tensor([[0.0], [0.5], [1.0]]))

    def test_default_dim_is_minus_one(self) -> None:
        """The default dim concatenates along the last dimension."""
        layer = MinMax(
            input_cols=["a", "b"], output_cols=["o"], min=[0.0, 0.0], max=[10.0, 20.0]
        )
        out = layer({"a": torch.tensor([[5.0]]), "b": torch.tensor([[10.0]])})["o"]
        torch.testing.assert_close(out, torch.tensor([[0.5, 0.5]]))

    def test_1d_input_is_unsqueezed_when_dim_nonzero(self) -> None:
        """A 1D column is unsqueezed to [batch, 1] before concatenation."""
        layer = MinMax(input_cols=["x"], output_cols=["o"], min=[0.0], max=[10.0])
        out = layer({"x": torch.tensor([0.0, 5.0, 10.0])})["o"]
        torch.testing.assert_close(out, torch.tensor([[0.0], [0.5], [1.0]]))

    def test_dim_zero_does_not_unsqueeze(self) -> None:
        """dim=0 concatenates along the batch dimension without unsqueezing."""
        layer = MinMax(
            input_cols=["a", "b"], output_cols=["o"], min=0.0, max=10.0, dim=0
        )
        out = layer({"a": torch.tensor([0.0, 5.0]), "b": torch.tensor([10.0])})["o"]
        torch.testing.assert_close(out, torch.tensor([0.0, 0.5, 1.0]))

    def test_tensor_typed_min_max(self) -> None:
        """min/max may be passed as tensors, not just lists."""
        layer = MinMax(
            input_cols=["x"],
            output_cols=["o"],
            min=torch.tensor([0.0]),
            max=torch.tensor([4.0]),
        )
        out = layer({"x": torch.tensor([[2.0]])})["o"]
        torch.testing.assert_close(out, torch.tensor([[0.5]]))

    def test_min_equals_max_uses_eps_not_divide_by_zero(self) -> None:
        """When min == max, the eps-clamped denominator avoids NaN/inf."""
        layer = MinMax(input_cols=["x"], output_cols=["o"], min=[5.0], max=[5.0])
        out = layer({"x": torch.tensor([[5.0]])})["o"]
        assert torch.isfinite(out).all()

    def test_custom_epsilon(self) -> None:
        """A custom eps is used in place of the default."""
        layer = MinMax(
            input_cols=["x"], output_cols=["o"], min=[1.0], max=[1.0], eps=1.0
        )
        out = layer({"x": torch.tensor([[1.0]])})["o"]
        torch.testing.assert_close(out, torch.tensor([[0.0]]))

    def test_negative_values(self) -> None:
        """Negative inputs and bounds scale correctly."""
        layer = MinMax(input_cols=["x"], output_cols=["o"], min=[-10.0], max=[10.0])
        out = layer({"x": torch.tensor([[-10.0], [0.0], [10.0]])})["o"]
        torch.testing.assert_close(out, torch.tensor([[0.0], [0.5], [1.0]]))


class TestNormalization:
    """Feature-wise standardization, including dim handling and safe division."""

    def test_multi_output_cols_raises(self) -> None:
        """More than one output column raises ``ValueError``."""
        with pytest.raises(ValueError, match="exactly one column"):
            Normalization(
                input_cols=["x"], output_cols=["a", "b"], mean=[0.0], std=[1.0]
            )

    def test_forward_basic(self) -> None:
        """Values standardize to (x - mean) / std."""
        layer = Normalization(
            input_cols=["x"], output_cols=["o"], mean=[5.0], std=[5.0]
        )
        out = layer({"x": torch.tensor([[0.0], [5.0], [10.0]])})["o"]
        torch.testing.assert_close(out, torch.tensor([[-1.0], [0.0], [1.0]]))

    def test_default_dim_is_minus_one(self) -> None:
        """The default dim concatenates along the last dimension."""
        layer = Normalization(
            input_cols=["a", "b"], output_cols=["o"], mean=[0.0, 0.0], std=[1.0, 2.0]
        )
        out = layer({"a": torch.tensor([[1.0]]), "b": torch.tensor([[4.0]])})["o"]
        torch.testing.assert_close(out, torch.tensor([[1.0, 2.0]]))

    def test_1d_input_is_unsqueezed_when_dim_nonzero(self) -> None:
        """A 1D column is unsqueezed to [batch, 1] before concatenation."""
        layer = Normalization(
            input_cols=["x"], output_cols=["o"], mean=[0.0], std=[2.0]
        )
        out = layer({"x": torch.tensor([0.0, 2.0, 4.0])})["o"]
        torch.testing.assert_close(out, torch.tensor([[0.0], [1.0], [2.0]]))

    def test_dim_zero_does_not_unsqueeze(self) -> None:
        """dim=0 concatenates along the batch dimension without unsqueezing."""
        layer = Normalization(
            input_cols=["a", "b"], output_cols=["o"], mean=0.0, std=1.0, dim=0
        )
        out = layer({"a": torch.tensor([0.0, 1.0]), "b": torch.tensor([2.0])})["o"]
        torch.testing.assert_close(out, torch.tensor([0.0, 1.0, 2.0]))

    def test_tensor_typed_mean_std(self) -> None:
        """mean/std may be passed as tensors, not just lists."""
        layer = Normalization(
            input_cols=["x"],
            output_cols=["o"],
            mean=torch.tensor([1.0]),
            std=torch.tensor([2.0]),
        )
        out = layer({"x": torch.tensor([[3.0]])})["o"]
        torch.testing.assert_close(out, torch.tensor([[1.0]]))

    def test_std_zero_uses_eps_not_divide_by_zero(self) -> None:
        """When std == 0, the eps-clamped denominator avoids NaN/inf."""
        layer = Normalization(
            input_cols=["x"], output_cols=["o"], mean=[0.0], std=[0.0]
        )
        out = layer({"x": torch.tensor([[1.0]])})["o"]
        assert torch.isfinite(out).all()

    def test_custom_epsilon(self) -> None:
        """A custom eps is used in place of the default."""
        layer = Normalization(
            input_cols=["x"], output_cols=["o"], mean=[0.0], std=[0.0], eps=1.0
        )
        out = layer({"x": torch.tensor([[1.0]])})["o"]
        torch.testing.assert_close(out, torch.tensor([[1.0]]))

    def test_negative_values(self) -> None:
        """Negative inputs and stats standardize correctly."""
        layer = Normalization(
            input_cols=["x"], output_cols=["o"], mean=[-5.0], std=[5.0]
        )
        out = layer({"x": torch.tensor([[-10.0], [-5.0], [0.0]])})["o"]
        torch.testing.assert_close(out, torch.tensor([[-1.0], [0.0], [1.0]]))


class TestBucketization:
    """Bucketization, including dtype handling and sentinel preservation."""

    def test_mismatched_columns_raises(self) -> None:
        """Unequal input/output column counts raise ``ValueError``."""
        with pytest.raises(ValueError, match="same length"):
            Bucketization(
                input_cols=["a", "b"], output_cols=["o"], boundaries=[0.0, 1.0]
            )

    def test_forward_basic(self) -> None:
        """Values bucketize with right=True (left boundary closed) semantics."""
        layer = Bucketization(
            input_cols=["x"], output_cols=["o"], boundaries=[0.0, 5.0, 10.0]
        )
        out = layer({"x": torch.tensor([-1.0, 0.0, 3.0, 5.0, 12.0])})["o"]
        torch.testing.assert_close(
            out, torch.tensor([0, 1, 1, 2, 3], dtype=torch.int32)
        )

    def test_multi_column(self) -> None:
        """Multiple columns bucketize independently."""
        layer = Bucketization(
            input_cols=["a", "b"], output_cols=["oa", "ob"], boundaries=[0.0, 5.0]
        )
        out = layer({"a": torch.tensor([-1.0]), "b": torch.tensor([6.0])})
        torch.testing.assert_close(out["oa"], torch.tensor([0], dtype=torch.int32))
        torch.testing.assert_close(out["ob"], torch.tensor([2], dtype=torch.int32))

    def test_explicit_dtype(self) -> None:
        """An explicit dtype converts the output."""
        layer = Bucketization(
            input_cols=["x"],
            output_cols=["o"],
            boundaries=[0.0, 5.0],
            dtype=torch.int64,
        )
        out = layer({"x": torch.tensor([1.0])})["o"]
        assert out.dtype == torch.int64

    def test_string_dtype(self) -> None:
        """A string dtype alias resolves the same as the torch.dtype."""
        layer = Bucketization(
            input_cols=["x"],
            output_cols=["o"],
            boundaries=[0.0, 5.0],
            dtype="int64",
        )
        out = layer({"x": torch.tensor([1.0])})["o"]
        assert out.dtype == torch.int64

    def test_nan_sentinel_is_preserved(self) -> None:
        """NaN sentinel positions in float input are preserved as INT32_SENTINEL."""
        layer = Bucketization(
            input_cols=["x"], output_cols=["o"], boundaries=[0.0, 5.0]
        )
        out = layer({"x": torch.tensor([1.0, float("nan"), 6.0])})["o"]
        torch.testing.assert_close(
            out, torch.tensor([1, INT32_SENTINEL, 2], dtype=torch.int32)
        )

    def test_int_sentinel_is_preserved(self) -> None:
        """INT32_SENTINEL positions in integer input are preserved."""
        layer = Bucketization(input_cols=["x"], output_cols=["o"], boundaries=[0, 5])
        out = layer({"x": torch.tensor([1, INT32_SENTINEL, 6], dtype=torch.int32)})["o"]
        torch.testing.assert_close(
            out, torch.tensor([1, INT32_SENTINEL, 2], dtype=torch.int32)
        )

    def test_nan_sentinel_preserved_as_nan_with_float_output_dtype(self) -> None:
        """A float output dtype preserves the sentinel as NaN, not INT32_SENTINEL.

        Regression test: the sentinel written back must match the *output*
        dtype, not always fall back to the integer sentinel.
        """
        layer = Bucketization(
            input_cols=["x"],
            output_cols=["o"],
            boundaries=[0.0, 5.0],
            dtype=torch.float32,
        )
        out = layer({"x": torch.tensor([1.0, float("nan"), 6.0])})["o"]
        assert out.dtype == torch.float32
        assert torch.isnan(out[1])
        torch.testing.assert_close(out[0], torch.tensor(1.0))
        torch.testing.assert_close(out[2], torch.tensor(2.0))


def _stats_layer_cases() -> list[tuple[str, object, dict]]:
    """Layer/input pairs shared by the parametrized TorchScript round-trip test."""
    return [
        (
            "min_max",
            MinMax(input_cols=["x"], output_cols=["o"], min=[0.0], max=[10.0]),
            {"x": torch.tensor([[0.0], [5.0], [10.0]])},
        ),
        (
            "normalization",
            Normalization(input_cols=["x"], output_cols=["o"], mean=[5.0], std=[5.0]),
            {"x": torch.tensor([[0.0], [5.0], [10.0]])},
        ),
        (
            "bucketization",
            Bucketization(input_cols=["x"], output_cols=["o"], boundaries=[0.0, 5.0]),
            {"x": torch.tensor([-1.0, 1.0, 6.0])},
        ),
        (
            "bucketization_nan_sentinel",
            Bucketization(input_cols=["x"], output_cols=["o"], boundaries=[0.0, 5.0]),
            {"x": torch.tensor([1.0, float("nan"), 6.0])},
        ),
        (
            "bucketization_int_sentinel",
            Bucketization(input_cols=["x"], output_cols=["o"], boundaries=[0, 5]),
            {"x": torch.tensor([1, INT32_SENTINEL, 6], dtype=torch.int32)},
        ),
    ]


class TestB3TorchScriptRoundTrip:
    """Every B3 fitted-statistics layer must script, save/load, and match eager."""

    @pytest.mark.parametrize(
        ("layer", "inputs"),
        [(layer, inputs) for _, layer, inputs in _stats_layer_cases()],
        ids=[case_id for case_id, _, _ in _stats_layer_cases()],
    )
    def test_scripted_matches_eager(
        self,
        layer,
        inputs: dict[str, torch.Tensor],
        tmp_path,
    ) -> None:
        """Scripting (and reloading) reproduces the eager forward output."""
        layer.eval()
        eager = layer(inputs)

        scripted = torch.jit.script(layer)
        scripted_out = scripted(inputs)
        assert set(scripted_out) == set(eager)
        for key in eager:
            torch.testing.assert_close(scripted_out[key], eager[key])

        model_path = tmp_path / "scripted_b3_layer.pt"
        scripted.save(str(model_path))
        loaded = torch.jit.load(str(model_path))
        loaded_out = loaded(inputs)
        for key in eager:
            torch.testing.assert_close(loaded_out[key], eager[key])
