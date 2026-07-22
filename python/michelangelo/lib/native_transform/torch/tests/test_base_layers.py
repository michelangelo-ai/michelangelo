"""Tests for :mod:`michelangelo.lib.native_transform.torch.base_layers`.

Covers the forward semantics of the foundation transform layers and, for every
layer, a ``torch.jit.script`` round-trip: native transform layers must be
TorchScript-exportable so the exact transform runs at serve time, and the
scripted module (including after save/load) must reproduce the eager output.
"""

from __future__ import annotations

import pytest

# These layers operate on real torch tensors/modules. Skip cleanly if torch is
# unavailable in a lightweight environment.
torch = pytest.importorskip("torch")

from michelangelo.lib.native_transform.torch.base_layers import (  # noqa: E402
    Cast,
    Ceil,
    Concatenate,
    Constant,
    Divide,
    Floor,
    IdentityTransform,
    LogTransform,
    Stack,
    Subtract,
    TorchTransformBaseLayer,
)


class _BaseTestLayer(TorchTransformBaseLayer):
    """Minimal concrete subclass used to exercise the abstract base."""

    def forward(self, inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]:
        """Return the first input column under the first output column."""
        return {self.output_cols[0]: inputs[self.input_cols[0]]}


class TestTorchTransformBaseLayer:
    """Base-layer construction and abstractness."""

    def test_init_with_kwargs(self) -> None:
        """``name`` and columns are stored from constructor arguments."""
        layer = _BaseTestLayer(
            input_cols=["col1", "col2"], output_cols=["output"], name="test_layer"
        )
        assert layer.name == "test_layer"
        assert layer.input_cols == ["col1", "col2"]
        assert layer.output_cols == ["output"]

    def test_init_without_kwargs(self) -> None:
        """``name`` defaults to a generated snake_case name from the class."""
        layer = _BaseTestLayer(input_cols=[], output_cols=[])
        # ``_BaseTestLayer`` is a private (leading-underscore) class name, so the
        # generated name is prefixed with "private" per ``to_snake_case``.
        assert layer.name.startswith("private__base_test_layer_")

    def test_default_names_are_unique(self) -> None:
        """Two default-constructed layers of the same class get distinct names."""
        first = _BaseTestLayer(input_cols=[], output_cols=[])
        second = _BaseTestLayer(input_cols=[], output_cols=[])
        assert first.name != second.name

    def test_explicit_name_overrides_generation(self) -> None:
        """An explicit ``name`` is used verbatim, not auto-generated."""
        layer = _BaseTestLayer(input_cols=[], output_cols=[], name="explicit")
        assert layer.name == "explicit"

    def test_abstract_class_cannot_be_instantiated(self) -> None:
        """The abstract base cannot be instantiated directly."""
        with pytest.raises(TypeError):
            TorchTransformBaseLayer(input_cols=["test"], output_cols=["output"])


class TestConcatenate:
    """Concatenation along the last dimension with dtype handling."""

    def test_forward_basic(self) -> None:
        """Columns concatenate along the last dim, preserving float32."""
        layer = Concatenate(
            input_cols=["col1", "col2", "col3"], output_cols=["concatenated"]
        )
        inputs = {
            "col1": torch.tensor([[1.0, 2.0], [3.0, 4.0]]),
            "col2": torch.tensor([[5.0], [6.0]]),
            "col3": torch.tensor([[7.0, 8.0, 9.0], [10.0, 11.0, 12.0]]),
        }
        outputs = layer(inputs)
        expected = torch.tensor(
            [[1.0, 2.0, 5.0, 7.0, 8.0, 9.0], [3.0, 4.0, 6.0, 10.0, 11.0, 12.0]]
        )
        torch.testing.assert_close(outputs["concatenated"], expected)
        assert outputs["concatenated"].dtype == torch.float32

    def test_forward_type_promotion(self) -> None:
        """Mixed dtypes promote to the widest type when dtype is None."""
        layer = Concatenate(
            input_cols=["col1", "col2", "col3"], output_cols=["concatenated"]
        )
        inputs = {
            "col1": torch.tensor([[1, 2]], dtype=torch.int32),
            "col2": torch.tensor([[3.5]], dtype=torch.float64),
            "col3": torch.tensor([[4]], dtype=torch.int64),
        }
        outputs = layer(inputs)
        assert outputs["concatenated"].dtype == torch.float64
        expected = torch.tensor([[1.0, 2.0, 3.5, 4.0]], dtype=torch.float64)
        torch.testing.assert_close(outputs["concatenated"], expected)

    def test_forward_single_tensor(self) -> None:
        """A single input column passes through unchanged."""
        layer = Concatenate(input_cols=["col1"], output_cols=["output"])
        inputs = {"col1": torch.tensor([[1.0, 2.0, 3.0]])}
        torch.testing.assert_close(layer(inputs)["output"], inputs["col1"])

    def test_forward_preserve_int_dtype(self) -> None:
        """Integer inputs keep their dtype when no dtype is given."""
        layer = Concatenate(input_cols=["col1", "col2"], output_cols=["concatenated"])
        inputs = {
            "col1": torch.tensor([[1, 2]], dtype=torch.int32),
            "col2": torch.tensor([[3, 4]], dtype=torch.int32),
        }
        outputs = layer(inputs)
        assert outputs["concatenated"].dtype == torch.int32
        torch.testing.assert_close(
            outputs["concatenated"], torch.tensor([[1, 2, 3, 4]], dtype=torch.int32)
        )

    def test_forward_explicit_dtype(self) -> None:
        """An explicit dtype forces conversion of the output."""
        layer = Concatenate(
            input_cols=["col1", "col2"],
            output_cols=["concatenated"],
            dtype=torch.float32,
        )
        inputs = {
            "col1": torch.tensor([[1, 2]], dtype=torch.int32),
            "col2": torch.tensor([[3, 4]], dtype=torch.int32),
        }
        outputs = layer(inputs)
        assert outputs["concatenated"].dtype == torch.float32
        torch.testing.assert_close(
            outputs["concatenated"],
            torch.tensor([[1.0, 2.0, 3.0, 4.0]], dtype=torch.float32),
        )


class TestStack:
    """Stacking along a new dimension (inputs cast to float32)."""

    def test_default_dim(self) -> None:
        """Default ``dim=-1`` and the layer stores it."""
        layer = Stack(input_cols=["a", "b"], output_cols=["out"])
        assert layer.dim == -1

    def test_custom_dim(self) -> None:
        """A custom ``dim`` kwarg is stored."""
        assert Stack(input_cols=["a", "b"], output_cols=["out"], dim=1).dim == 1

    def test_forward_default_dim(self) -> None:
        """2D inputs stack along the last dim to shape ``(B, L, N)``."""
        layer = Stack(input_cols=["col1", "col2", "col3"], output_cols=["stacked"])
        inputs = {
            "col1": torch.tensor([[1.0, 2.0], [3.0, 4.0]]),
            "col2": torch.tensor([[5.0, 6.0], [7.0, 8.0]]),
            "col3": torch.tensor([[9.0, 10.0], [11.0, 12.0]]),
        }
        outputs = layer(inputs)
        assert outputs["stacked"].shape == torch.Size([2, 2, 3])
        expected = torch.stack([inputs["col1"], inputs["col2"], inputs["col3"]], dim=-1)
        torch.testing.assert_close(outputs["stacked"], expected)

    def test_forward_dim_1(self) -> None:
        """Stacking along ``dim=1`` inserts the new axis in the middle."""
        layer = Stack(input_cols=["a", "b"], output_cols=["out"], dim=1)
        inputs = {
            "a": torch.tensor([[1.0, 2.0], [3.0, 4.0]]),
            "b": torch.tensor([[5.0, 6.0], [7.0, 8.0]]),
        }
        outputs = layer(inputs)
        assert outputs["out"].shape == torch.Size([2, 2, 2])
        torch.testing.assert_close(
            outputs["out"], torch.stack([inputs["a"], inputs["b"]], dim=1)
        )

    def test_forward_dim_0(self) -> None:
        """Stacking 1D inputs along ``dim=0`` yields shape ``(N, L)``."""
        layer = Stack(input_cols=["a", "b"], output_cols=["out"], dim=0)
        inputs = {"a": torch.tensor([1.0, 2.0]), "b": torch.tensor([3.0, 4.0])}
        outputs = layer(inputs)
        assert outputs["out"].shape == torch.Size([2, 2])

    def test_forward_casts_to_float32(self) -> None:
        """Mixed-dtype inputs are stacked as float32."""
        layer = Stack(input_cols=["a", "b"], output_cols=["out"])
        inputs = {
            "a": torch.tensor([1, 2], dtype=torch.int32),
            "b": torch.tensor([3.5, 4.5], dtype=torch.float64),
        }
        assert layer(inputs)["out"].dtype == torch.float32


class TestCast:
    """Casting to a target dtype."""

    def test_forward_to_float(self) -> None:
        """Int input casts to float32."""
        layer = Cast(
            input_cols=["feature"], output_cols=["casted"], dtype=torch.float32
        )
        inputs = {"feature": torch.tensor([1, 2, 3], dtype=torch.int32)}
        outputs = layer(inputs)
        assert outputs["casted"].dtype == torch.float32
        torch.testing.assert_close(outputs["casted"], torch.tensor([1.0, 2.0, 3.0]))

    def test_forward_to_int(self) -> None:
        """Float input truncates to int64."""
        layer = Cast(input_cols=["feature"], output_cols=["casted"], dtype=torch.int64)
        inputs = {"feature": torch.tensor([1.1, 2.9, 3.5], dtype=torch.float32)}
        outputs = layer(inputs)
        assert outputs["casted"].dtype == torch.int64
        torch.testing.assert_close(outputs["casted"], torch.tensor([1, 2, 3]))

    def test_mismatched_columns_raises(self) -> None:
        """Unequal input/output column counts raise ``ValueError``."""
        with pytest.raises(ValueError, match="same length"):
            Cast(input_cols=["a", "b"], output_cols=["out"])

    @pytest.mark.parametrize("dtype", ["float32", "torch.float32"])
    def test_string_dtype_aliases_behave_identically(self, dtype: str) -> None:
        """Bare and ``torch.``-prefixed string aliases both cast correctly."""
        layer = Cast(input_cols=["feature"], output_cols=["casted"], dtype=dtype)
        assert layer.dtype == torch.float32
        inputs = {"feature": torch.tensor([1, 2, 3], dtype=torch.int32)}
        assert layer(inputs)["casted"].dtype == torch.float32

    def test_unrecognized_dtype_string_raises(self) -> None:
        """A typo'd dtype string raises instead of silently no-op'ing."""
        with pytest.raises(ValueError, match="Unsupported dtype specification"):
            Cast(input_cols=["feature"], output_cols=["casted"], dtype="flaot32")


class TestConstant:
    """Constant tensor generation shaped like the input."""

    def test_forward_scalar(self) -> None:
        """A scalar constant fills a tensor matching the input shape."""
        layer = Constant(
            input_cols=["target"],
            output_cols=["const"],
            constant=3.14,
            dtype=torch.float32,
        )
        inputs = {"target": torch.tensor([1, 2, 3])}
        outputs = layer(inputs)
        assert outputs["const"].dtype == torch.float32
        assert outputs["const"].shape == inputs["target"].shape
        torch.testing.assert_close(outputs["const"], torch.tensor([3.14, 3.14, 3.14]))

    def test_no_input_column_raises(self) -> None:
        """Empty (but length-matched) columns raise for the missing shape ref."""
        # input_cols and output_cols are both empty, so the length check passes
        # and the empty-shape-reference guard is what fires.
        with pytest.raises(ValueError, match="at least one input column"):
            Constant(input_cols=[], output_cols=[], constant=42, dtype=torch.int32)

    def test_forward_multiple_columns(self) -> None:
        """Each output column gets its own constant-filled tensor."""
        layer = Constant(
            input_cols=["in1", "in2"], output_cols=["out1", "out2"], constant=1.0
        )
        inputs = {"in1": torch.tensor([1, 2]), "in2": torch.tensor([3, 4])}
        outputs = layer(inputs)
        expected = torch.tensor([1.0, 1.0])
        torch.testing.assert_close(outputs["out1"], expected)
        torch.testing.assert_close(outputs["out2"], expected)

    def test_forward_multi_dimensional(self) -> None:
        """A 2D reference produces a matching 2D constant tensor."""
        layer = Constant(
            input_cols=["matrix"],
            output_cols=["const"],
            constant=7.0,
            dtype=torch.float32,
        )
        inputs = {"matrix": torch.tensor([[1, 2], [3, 4]])}
        torch.testing.assert_close(
            layer(inputs)["const"], torch.tensor([[7.0, 7.0], [7.0, 7.0]])
        )

    def test_forward_bool_constant(self) -> None:
        """A boolean constant infers a bool output dtype."""
        layer = Constant(input_cols=["ref"], output_cols=["const"], constant=True)
        inputs = {"ref": torch.tensor([1, 2, 3])}
        torch.testing.assert_close(
            layer(inputs)["const"], torch.tensor([True, True, True])
        )

    def test_forward_infer_dtype(self) -> None:
        """An int constant infers an int64 output dtype."""
        layer = Constant(input_cols=["ref"], output_cols=["const"], constant=42)
        inputs = {"ref": torch.tensor([1.0, 2.0])}
        assert layer(inputs)["const"].dtype == torch.int64

    def test_mismatched_columns_raises(self) -> None:
        """Unequal input/output column counts raise ``ValueError``."""
        with pytest.raises(ValueError, match="same length"):
            Constant(input_cols=["a", "b"], output_cols=["out"], constant=1.0)


class TestDivide:
    """Pairwise element-wise division with zero-safe handling."""

    def test_forward_basic(self) -> None:
        """A single numerator/denominator pair divides in float64."""
        layer = Divide(input_cols=["numerator", "denominator"], output_cols=["divided"])
        inputs = {
            "numerator": torch.tensor([10.0, 20.0, 30.0]),
            "denominator": torch.tensor([2.0, 5.0, 10.0]),
        }
        expected = torch.tensor([5.0, 4.0, 3.0], dtype=torch.float64)
        torch.testing.assert_close(layer(inputs)["divided"], expected)

    def test_forward_safe_division_by_zero(self) -> None:
        """Zero denominators yield finite results; 0/0 becomes 0."""
        layer = Divide(input_cols=["numerator", "denominator"], output_cols=["divided"])
        inputs = {
            "numerator": torch.tensor([10.0, 0.0]),
            "denominator": torch.tensor([0.0, 0.0]),
        }
        outputs = layer(inputs)
        assert torch.isfinite(outputs["divided"]).all()
        assert outputs["divided"][0].abs() > 1e6
        assert outputs["divided"][1] == 0.0

    def test_forward_multiple_pairs(self) -> None:
        """Multiple pairs divide independently."""
        layer = Divide(
            input_cols=["n1", "d1", "n2", "d2"], output_cols=["out1", "out2"]
        )
        inputs = {
            "n1": torch.tensor([10.0, 20.0]),
            "d1": torch.tensor([2.0, 4.0]),
            "n2": torch.tensor([30.0, 40.0]),
            "d2": torch.tensor([3.0, 8.0]),
        }
        outputs = layer(inputs)
        torch.testing.assert_close(
            outputs["out1"], torch.tensor([5.0, 5.0], dtype=torch.float64)
        )
        torch.testing.assert_close(
            outputs["out2"], torch.tensor([10.0, 5.0], dtype=torch.float64)
        )

    def test_forward_add_constant_to_divisor(self) -> None:
        """The divisor constant shifts the denominator before dividing."""
        layer = Divide(
            input_cols=["num", "den"],
            output_cols=["result"],
            add_constant_to_divisor=1.0,
        )
        inputs = {
            "num": torch.tensor([10.0, 20.0]),
            "den": torch.tensor([2.0, 4.0]),
        }
        # 10 / (2 + 1) and 20 / (4 + 1).
        expected = torch.tensor([10.0 / 3.0, 4.0], dtype=torch.float64)
        torch.testing.assert_close(layer(inputs)["result"], expected)

    def test_odd_input_columns_raises(self) -> None:
        """An odd number of input columns raises ``ValueError``."""
        with pytest.raises(ValueError, match="even"):
            Divide(input_cols=["a", "b", "c"], output_cols=["out"])

    def test_explicit_eps_is_stored_and_applied(self) -> None:
        """An explicit ``eps`` is stored and substituted for a zero denominator."""
        layer = Divide(input_cols=["num", "den"], output_cols=["out"], eps=0.5)
        assert layer.eps == 0.5
        # 10 / eps when the denominator is zero.
        inputs = {"num": torch.tensor([10.0]), "den": torch.tensor([0.0])}
        torch.testing.assert_close(
            layer(inputs)["out"], torch.tensor([20.0], dtype=torch.float64)
        )


class TestLogTransform:
    """Logarithmic transform with offset and clamping."""

    def test_forward_basic(self) -> None:
        """log(x + 1) is clamped to ``[1.0, 1e20]``."""
        layer = LogTransform(input_cols=["feature"], output_cols=["log_feature"])
        inputs = {"feature": torch.tensor([0.0, 1.0, 9.0])}
        expected = torch.clamp(
            torch.log(torch.tensor([1.0, 2.0, 10.0])), min=1.0, max=1e20
        )
        torch.testing.assert_close(layer(inputs)["log_feature"], expected)

    def test_forward_custom_add_constant(self) -> None:
        """A custom ``add_constant`` shifts the input before the log."""
        layer = LogTransform(
            input_cols=["feature"], output_cols=["log_feature"], add_constant=10.0
        )
        inputs = {"feature": torch.tensor([0.0, 90.0])}
        expected = torch.clamp(
            torch.log(torch.tensor([10.0, 100.0])), min=1.0, max=1e20
        )
        torch.testing.assert_close(layer(inputs)["log_feature"], expected)

    def test_forward_clamping_min(self) -> None:
        """Results below 1.0 are clamped up to 1.0."""
        layer = LogTransform(
            input_cols=["feature"], output_cols=["log_feature"], add_constant=0.1
        )
        inputs = {"feature": torch.tensor([0.0])}
        torch.testing.assert_close(layer(inputs)["log_feature"], torch.tensor([1.0]))

    def test_mismatched_columns_raises(self) -> None:
        """Unequal input/output column counts raise ``ValueError``."""
        with pytest.raises(ValueError, match="same length"):
            LogTransform(input_cols=["feat1", "feat2"], output_cols=["out1"])

    def test_name_kwarg_is_honored(self) -> None:
        """``name`` is forwarded to the base class rather than dropped."""
        layer = LogTransform(
            input_cols=["feature"], output_cols=["log_feature"], name="my_log"
        )
        assert layer.name == "my_log"


class TestSubtract:
    """Pairwise element-wise subtraction in float64."""

    def test_forward_basic(self) -> None:
        """A single pair subtracts element-wise in float64."""
        layer = Subtract(input_cols=["a", "b"], output_cols=["result"])
        inputs = {
            "a": torch.tensor([10.0, 20.0, 30.0]),
            "b": torch.tensor([2.0, 5.0, 10.0]),
        }
        torch.testing.assert_close(
            layer(inputs)["result"],
            torch.tensor([8.0, 15.0, 20.0], dtype=torch.float64),
        )

    def test_forward_broadcasting(self) -> None:
        """A scalar subtrahend broadcasts across the minuend."""
        layer = Subtract(input_cols=["vector", "scalar"], output_cols=["result"])
        inputs = {
            "vector": torch.tensor([10.0, 20.0, 30.0]),
            "scalar": torch.tensor([5.0]),
        }
        torch.testing.assert_close(
            layer(inputs)["result"],
            torch.tensor([5.0, 15.0, 25.0], dtype=torch.float64),
        )

    def test_forward_different_dtypes(self) -> None:
        """Mixed dtypes subtract in float64."""
        layer = Subtract(input_cols=["a", "b"], output_cols=["result"])
        inputs = {
            "a": torch.tensor([10, 20], dtype=torch.int32),
            "b": torch.tensor([2.5, 5.5], dtype=torch.float64),
        }
        outputs = layer(inputs)
        assert outputs["result"].dtype == torch.float64
        torch.testing.assert_close(
            outputs["result"], torch.tensor([7.5, 14.5], dtype=torch.float64)
        )

    def test_forward_multiple_pairs(self) -> None:
        """Multiple pairs subtract independently."""
        layer = Subtract(
            input_cols=["a1", "b1", "a2", "b2"], output_cols=["out1", "out2"]
        )
        inputs = {
            "a1": torch.tensor([10.0, 20.0]),
            "b1": torch.tensor([2.0, 5.0]),
            "a2": torch.tensor([30.0, 40.0]),
            "b2": torch.tensor([10.0, 20.0]),
        }
        outputs = layer(inputs)
        torch.testing.assert_close(
            outputs["out1"], torch.tensor([8.0, 15.0], dtype=torch.float64)
        )
        torch.testing.assert_close(
            outputs["out2"], torch.tensor([20.0, 20.0], dtype=torch.float64)
        )

    def test_odd_input_columns_raises(self) -> None:
        """An odd number of input columns raises ``ValueError``."""
        with pytest.raises(ValueError, match="even"):
            Subtract(input_cols=["a", "b", "c"], output_cols=["out"])


class TestFloor:
    """Element-wise floor."""

    def test_forward_basic(self) -> None:
        """Floor rounds toward negative infinity."""
        layer = Floor(input_cols=["val"], output_cols=["floored"])
        inputs = {"val": torch.tensor([1.1, 2.9, -3.5])}
        torch.testing.assert_close(
            layer(inputs)["floored"], torch.tensor([1.0, 2.0, -4.0])
        )

    def test_forward_integers(self) -> None:
        """Floor is a no-op on whole numbers."""
        layer = Floor(input_cols=["val"], output_cols=["floored"])
        inputs = {"val": torch.tensor([1.0, 2.0, 3.0])}
        torch.testing.assert_close(
            layer(inputs)["floored"], torch.tensor([1.0, 2.0, 3.0])
        )

    def test_mismatched_columns_raises(self) -> None:
        """Unequal input/output column counts raise ``ValueError``."""
        with pytest.raises(ValueError, match="same length"):
            Floor(input_cols=["a", "b"], output_cols=["out"])


class TestCeil:
    """Element-wise ceiling."""

    def test_forward_basic(self) -> None:
        """Ceil rounds toward positive infinity."""
        layer = Ceil(input_cols=["val"], output_cols=["ceiled"])
        inputs = {"val": torch.tensor([1.1, 2.9, -3.5])}
        torch.testing.assert_close(
            layer(inputs)["ceiled"], torch.tensor([2.0, 3.0, -3.0])
        )

    def test_forward_integers(self) -> None:
        """Ceil is a no-op on whole numbers."""
        layer = Ceil(input_cols=["val"], output_cols=["ceiled"])
        inputs = {"val": torch.tensor([1.0, 2.0, 3.0])}
        torch.testing.assert_close(
            layer(inputs)["ceiled"], torch.tensor([1.0, 2.0, 3.0])
        )

    def test_mismatched_columns_raises(self) -> None:
        """Unequal input/output column counts raise ``ValueError``."""
        with pytest.raises(ValueError, match="same length"):
            Ceil(input_cols=["a", "b"], output_cols=["out"])


class TestIdentityTransform:
    """Pass-through transform."""

    def test_basic_identity(self) -> None:
        """Integer values pass through unchanged."""
        layer = IdentityTransform(
            input_cols=["user_id"], output_cols=["bypass_user_id"]
        )
        inputs = {"user_id": torch.tensor([123, 456, 789], dtype=torch.long)}
        torch.testing.assert_close(layer(inputs)["bypass_user_id"], inputs["user_id"])

    def test_multiple_columns(self) -> None:
        """Multiple columns each map through to their output."""
        layer = IdentityTransform(
            input_cols=["col1", "col2"], output_cols=["out1", "out2"]
        )
        inputs = {
            "col1": torch.tensor([10, 20], dtype=torch.long),
            "col2": torch.tensor([30, 40], dtype=torch.long),
        }
        outputs = layer(inputs)
        torch.testing.assert_close(outputs["out1"], inputs["col1"])
        torch.testing.assert_close(outputs["out2"], inputs["col2"])

    def test_preserves_dtype(self) -> None:
        """Input dtype is preserved on the output."""
        layer = IdentityTransform(input_cols=["data"], output_cols=["bypass_data"])
        out_int32 = layer({"data": torch.tensor([1, 2, 3], dtype=torch.int32)})
        assert out_int32["bypass_data"].dtype == torch.int32
        out_f64 = layer({"data": torch.tensor([1.0, 2.0], dtype=torch.float64)})
        assert out_f64["bypass_data"].dtype == torch.float64

    def test_preserves_shape(self) -> None:
        """A 2D tensor keeps its shape."""
        layer = IdentityTransform(input_cols=["matrix"], output_cols=["bypass_matrix"])
        inputs = {"matrix": torch.tensor([[1, 2, 3], [4, 5, 6]], dtype=torch.float32)}
        outputs = layer(inputs)
        assert outputs["bypass_matrix"].shape == inputs["matrix"].shape
        torch.testing.assert_close(outputs["bypass_matrix"], inputs["matrix"])

    def test_mismatched_columns_raises(self) -> None:
        """Unequal input/output column counts raise ``ValueError``."""
        with pytest.raises(ValueError, match="same length"):
            IdentityTransform(input_cols=["col1", "col2"], output_cols=["out1"])


def _layer_cases() -> list[tuple[str, TorchTransformBaseLayer, dict]]:
    """Build (id, layer, inputs) cases covering every foundation layer."""
    return [
        (
            "concatenate",
            Concatenate(input_cols=["a", "b"], output_cols=["out"]),
            {"a": torch.tensor([[1.0, 2.0]]), "b": torch.tensor([[3.0]])},
        ),
        (
            "concatenate_explicit_dtype",
            Concatenate(
                input_cols=["a", "b"], output_cols=["out"], dtype=torch.float32
            ),
            {
                "a": torch.tensor([[1, 2]], dtype=torch.int32),
                "b": torch.tensor([[3, 4]], dtype=torch.int32),
            },
        ),
        (
            "stack",
            Stack(input_cols=["a", "b"], output_cols=["out"]),
            {"a": torch.tensor([1.0, 2.0]), "b": torch.tensor([3.0, 4.0])},
        ),
        (
            "cast",
            Cast(input_cols=["a"], output_cols=["out"], dtype=torch.float32),
            {"a": torch.tensor([1, 2, 3], dtype=torch.int32)},
        ),
        (
            "constant",
            Constant(
                input_cols=["a"],
                output_cols=["out"],
                constant=3.14,
                dtype=torch.float32,
            ),
            {"a": torch.tensor([1, 2, 3])},
        ),
        (
            "divide",
            Divide(input_cols=["n", "d"], output_cols=["out"]),
            {"n": torch.tensor([10.0, 0.0]), "d": torch.tensor([2.0, 0.0])},
        ),
        (
            "log_transform",
            LogTransform(input_cols=["a"], output_cols=["out"]),
            {"a": torch.tensor([0.0, 1.0, 9.0])},
        ),
        (
            "subtract",
            Subtract(input_cols=["a", "b"], output_cols=["out"]),
            {"a": torch.tensor([10.0, 20.0]), "b": torch.tensor([2.0, 5.0])},
        ),
        (
            "floor",
            Floor(input_cols=["a"], output_cols=["out"]),
            {"a": torch.tensor([1.1, 2.9, -3.5])},
        ),
        (
            "ceil",
            Ceil(input_cols=["a"], output_cols=["out"]),
            {"a": torch.tensor([1.1, 2.9, -3.5])},
        ),
        (
            "identity",
            IdentityTransform(input_cols=["a"], output_cols=["out"]),
            {"a": torch.tensor([1, 2, 3], dtype=torch.long)},
        ),
    ]


class TestTorchScriptRoundTrip:
    """Every foundation layer must script, save/load, and match eager output."""

    @pytest.mark.parametrize(
        ("layer", "inputs"),
        [(layer, inputs) for _, layer, inputs in _layer_cases()],
        ids=[case_id for case_id, _, _ in _layer_cases()],
    )
    def test_scripted_matches_eager(
        self,
        layer: TorchTransformBaseLayer,
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

        model_path = tmp_path / "scripted_layer.pt"
        scripted.save(str(model_path))
        loaded = torch.jit.load(str(model_path))
        loaded_out = loaded(inputs)
        for key in eager:
            torch.testing.assert_close(loaded_out[key], eager[key])
