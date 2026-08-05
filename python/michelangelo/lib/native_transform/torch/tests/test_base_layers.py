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

from michelangelo.lib.constants.sentinel import INT32_SENTINEL  # noqa: E402
from michelangelo.lib.native_transform.torch.base_layers import (  # noqa: E402
    CaseWhen,
    Cast,
    Ceil,
    Clip,
    Compare,
    Concatenate,
    Constant,
    Divide,
    Floor,
    IdentityTransform,
    IDHashTokenizer,
    LogTransform,
    PadOrCrop1D,
    Scale,
    Stack,
    Subtract,
    TensorColFillNone,
    Tile,
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


class TestTensorColFillNone:
    """Filling missing positions detected from the runtime dtype."""

    def test_mismatched_columns_raises(self) -> None:
        """Unequal input/output column counts raise ``ValueError``."""
        with pytest.raises(ValueError, match="same length"):
            TensorColFillNone(
                input_cols=["a", "b"], output_cols=["out"], default_value=0.0
            )

    def test_fill_nan_float(self) -> None:
        """NaN positions in a float tensor are filled with the default."""
        layer = TensorColFillNone(
            input_cols=["feature"], output_cols=["filled"], default_value=0.0
        )
        inputs = {"feature": torch.tensor([1.0, float("nan"), 3.0, float("nan")])}
        torch.testing.assert_close(
            layer(inputs)["filled"], torch.tensor([1.0, 0.0, 3.0, 0.0])
        )

    def test_no_missing_values_passthrough(self) -> None:
        """A tensor with no missing values is unchanged."""
        layer = TensorColFillNone(
            input_cols=["feature"], output_cols=["filled"], default_value=0.0
        )
        inputs = {"feature": torch.tensor([1.0, 2.0, 3.0])}
        torch.testing.assert_close(
            layer(inputs)["filled"], torch.tensor([1.0, 2.0, 3.0])
        )

    def test_custom_fill_value(self) -> None:
        """A non-default fill value is used for NaN positions."""
        layer = TensorColFillNone(
            input_cols=["feature"], output_cols=["filled"], default_value=-1.0
        )
        inputs = {"feature": torch.tensor([1.0, float("nan"), 3.0])}
        torch.testing.assert_close(
            layer(inputs)["filled"], torch.tensor([1.0, -1.0, 3.0])
        )

    def test_detect_int32_min_as_missing(self) -> None:
        """The int32 minimum is detected as missing and filled."""
        layer = TensorColFillNone(
            input_cols=["feature"], output_cols=["filled"], default_value=999
        )
        min_val = torch.iinfo(torch.int32).min
        inputs = {"feature": torch.tensor([1, min_val, 3, min_val], dtype=torch.int32)}
        outputs = layer(inputs)
        assert outputs["filled"].dtype == torch.int32
        torch.testing.assert_close(
            outputs["filled"], torch.tensor([1, 999, 3, 999], dtype=torch.int32)
        )

    def test_detect_int64_min_as_missing(self) -> None:
        """The int64 minimum is detected as missing and filled."""
        layer = TensorColFillNone(
            input_cols=["feature"], output_cols=["filled"], default_value=888
        )
        min_val = torch.iinfo(torch.int64).min
        inputs = {"feature": torch.tensor([100, min_val, 200], dtype=torch.int64)}
        outputs = layer(inputs)
        assert outputs["filled"].dtype == torch.int64
        torch.testing.assert_close(
            outputs["filled"], torch.tensor([100, 888, 200], dtype=torch.int64)
        )

    def test_int32_all_missing(self) -> None:
        """An all-missing int32 tensor is fully replaced."""
        layer = TensorColFillNone(
            input_cols=["feature"], output_cols=["filled"], default_value=0
        )
        min_val = torch.iinfo(torch.int32).min
        inputs = {"feature": torch.tensor([min_val, min_val], dtype=torch.int32)}
        torch.testing.assert_close(
            layer(inputs)["filled"], torch.tensor([0, 0], dtype=torch.int32)
        )

    def test_int64_no_false_positive_on_real_values(self) -> None:
        """Genuine int64 values (not the minimum) are left untouched."""
        layer = TensorColFillNone(
            input_cols=["feature"], output_cols=["filled"], default_value=0
        )
        inputs = {"feature": torch.tensor([10, 20, 30], dtype=torch.int64)}
        torch.testing.assert_close(layer(inputs)["filled"], inputs["feature"])

    def test_int64_no_false_positive_near_minimum(self) -> None:
        """Values close to (but not equal to) the int64 minimum are not missing.

        Detection must be an exact comparison. A float-mediated check loses
        precision at this magnitude and flags a wide band around the sentinel.
        """
        min_val = torch.iinfo(torch.int64).min
        near_min = [min_val + 1, min_val + 1_000_000_000, min_val + 10**15]
        layer = TensorColFillNone(
            input_cols=["feature"], output_cols=["filled"], default_value=0
        )
        inputs = {"feature": torch.tensor(near_min, dtype=torch.int64)}
        torch.testing.assert_close(layer(inputs)["filled"], inputs["feature"])

    def test_int64_minimum_still_detected_alongside_near_values(self) -> None:
        """Only the exact int64 minimum is filled, even next to near-min values."""
        min_val = torch.iinfo(torch.int64).min
        layer = TensorColFillNone(
            input_cols=["feature"], output_cols=["filled"], default_value=7
        )
        inputs = {
            "feature": torch.tensor(
                [min_val, min_val + 1_000_000_000], dtype=torch.int64
            )
        }
        torch.testing.assert_close(
            layer(inputs)["filled"],
            torch.tensor([7, min_val + 1_000_000_000], dtype=torch.int64),
        )

    def test_multiple_columns(self) -> None:
        """Each column is filled independently."""
        layer = TensorColFillNone(
            input_cols=["f1", "f2"], output_cols=["o1", "o2"], default_value=0.0
        )
        inputs = {
            "f1": torch.tensor([1.0, float("nan")]),
            "f2": torch.tensor([float("nan"), 2.0]),
        }
        outputs = layer(inputs)
        torch.testing.assert_close(outputs["o1"], torch.tensor([1.0, 0.0]))
        torch.testing.assert_close(outputs["o2"], torch.tensor([0.0, 2.0]))


class TestCaseWhen:
    """SQL-like conditional selection over (condition, value) pairs."""

    def test_odd_input_columns_raises(self) -> None:
        """An odd number of input columns raises ``ValueError``."""
        with pytest.raises(ValueError, match="even number"):
            CaseWhen(
                input_cols=["cond1", "val1", "cond2"],
                output_cols=["result"],
                default_value=0,
            )

    def test_first_matching_condition_wins(self) -> None:
        """Earlier condition-value pairs take priority over later ones."""
        layer = CaseWhen(
            input_cols=["cond1", "val1", "cond2", "val2"],
            output_cols=["result"],
            default_value=-1,
        )
        inputs = {
            "cond1": torch.tensor([True, False, True, False]),
            "val1": torch.tensor([10, 0, 30, 0]),
            "cond2": torch.tensor([True, True, False, False]),
            "val2": torch.tensor([100, 200, 0, 0]),
        }
        torch.testing.assert_close(
            layer(inputs)["result"], torch.tensor([10, 200, 30, -1])
        )

    def test_default_used_when_no_condition_matches(self) -> None:
        """The scalar default fills positions where no condition is true."""
        layer = CaseWhen(
            input_cols=["cond1", "val1", "cond2", "val2"],
            output_cols=["result"],
            default_value=99,
        )
        inputs = {
            "cond1": torch.tensor([False, False, False]),
            "val1": torch.tensor([1, 2, 3]),
            "cond2": torch.tensor([False, False, False]),
            "val2": torch.tensor([10, 20, 30]),
        }
        torch.testing.assert_close(layer(inputs)["result"], torch.tensor([99, 99, 99]))

    def test_three_condition_pairs(self) -> None:
        """Three pairs resolve in priority order, falling back to the default."""
        layer = CaseWhen(
            input_cols=["c1", "v1", "c2", "v2", "c3", "v3"],
            output_cols=["result"],
            default_value=0,
        )
        inputs = {
            "c1": torch.tensor([True, False, False, False]),
            "v1": torch.tensor([10, 10, 10, 10]),
            "c2": torch.tensor([False, True, False, False]),
            "v2": torch.tensor([20, 20, 20, 20]),
            "c3": torch.tensor([False, False, True, False]),
            "v3": torch.tensor([30, 30, 30, 30]),
        }
        torch.testing.assert_close(
            layer(inputs)["result"], torch.tensor([10, 20, 30, 0])
        )

    def test_scalar_tensor_default(self) -> None:
        """A 0-dim tensor default broadcasts across the value shape."""
        layer = CaseWhen(
            input_cols=["cond1", "val1"],
            output_cols=["result"],
            default_value=torch.tensor(-1),
        )
        inputs = {
            "cond1": torch.tensor([True, False, True]),
            "val1": torch.tensor([100, 200, 300]),
        }
        torch.testing.assert_close(
            layer(inputs)["result"], torch.tensor([100, -1, 300])
        )

    def test_list_default(self) -> None:
        """A list default is materialized element-wise."""
        layer = CaseWhen(
            input_cols=["cond1", "val1"],
            output_cols=["result"],
            default_value=[99, 99],
        )
        inputs = {
            "cond1": torch.tensor([True, False]),
            "val1": torch.tensor([10, 20]),
        }
        torch.testing.assert_close(layer(inputs)["result"], torch.tensor([10, 99]))

    def test_all_conditions_true_uses_first(self) -> None:
        """When every condition is true, the first pair's values are chosen."""
        layer = CaseWhen(
            input_cols=["cond1", "val1", "cond2", "val2"],
            output_cols=["result"],
            default_value=0,
        )
        inputs = {
            "cond1": torch.tensor([True, True, True]),
            "val1": torch.tensor([10, 20, 30]),
            "cond2": torch.tensor([True, True, True]),
            "val2": torch.tensor([100, 200, 300]),
        }
        torch.testing.assert_close(layer(inputs)["result"], torch.tensor([10, 20, 30]))


class TestCompare:
    """Pairwise element-wise comparisons."""

    def test_odd_input_columns_raises(self) -> None:
        """An odd number of input columns raises ``ValueError``."""
        with pytest.raises(ValueError, match="even"):
            Compare(input_cols=["l", "r", "x"], output_cols=["o"], compare_op="equal")

    def test_output_not_half_raises(self) -> None:
        """Output count not equal to half the input count raises ``ValueError``."""
        with pytest.raises(ValueError, match="half"):
            Compare(input_cols=["l", "r"], output_cols=["o1", "o2"], compare_op="equal")

    def test_invalid_op_raises(self) -> None:
        """An unsupported operator raises ``ValueError``."""
        with pytest.raises(ValueError, match="not supported"):
            Compare(input_cols=["l", "r"], output_cols=["o"], compare_op="between")

    @pytest.mark.parametrize(
        ("op", "expected"),
        [
            ("equal", [True, False, True]),
            ("not_equal", [False, True, False]),
        ],
    )
    def test_equality_ops(self, op: str, expected: list[bool]) -> None:
        """``equal`` and ``not_equal`` produce boolean masks."""
        layer = Compare(input_cols=["left", "right"], output_cols=["c"], compare_op=op)
        inputs = {"left": torch.tensor([1, 2, 3]), "right": torch.tensor([1, 3, 3])}
        outputs = layer(inputs)
        assert outputs["c"].dtype == torch.bool
        torch.testing.assert_close(outputs["c"], torch.tensor(expected))

    @pytest.mark.parametrize(
        ("op", "expected"),
        [
            ("greater", [False, False, True]),
            ("less", [True, False, False]),
            ("greater_equal", [False, True, True]),
            ("less_equal", [True, True, False]),
        ],
    )
    def test_ordering_ops_broadcast_scalar(self, op: str, expected: list[bool]) -> None:
        """Ordering operators broadcast a scalar right operand."""
        layer = Compare(input_cols=["left", "right"], output_cols=["c"], compare_op=op)
        inputs = {"left": torch.tensor([1, 2, 3]), "right": torch.tensor(2)}
        torch.testing.assert_close(layer(inputs)["c"], torch.tensor(expected))

    def test_multiple_pairs(self) -> None:
        """Multiple pairs compare independently."""
        layer = Compare(
            input_cols=["l1", "r1", "l2", "r2"],
            output_cols=["o1", "o2"],
            compare_op="equal",
        )
        inputs = {
            "l1": torch.tensor([1, 2]),
            "r1": torch.tensor([1, 3]),
            "l2": torch.tensor([4, 5]),
            "r2": torch.tensor([4, 5]),
        }
        outputs = layer(inputs)
        torch.testing.assert_close(outputs["o1"], torch.tensor([True, False]))
        torch.testing.assert_close(outputs["o2"], torch.tensor([True, True]))


class TestTile:
    """Repeating tensors along an axis by count or inferred from a target."""

    def test_default_axis_is_zero(self) -> None:
        """The default tiling axis is ``0``."""
        assert Tile(input_cols=["s"], output_cols=["o"], count=2).axis == 0

    def test_forward_with_count(self) -> None:
        """An explicit count repeats a 2D tensor along axis 0."""
        layer = Tile(input_cols=["source"], output_cols=["tiled"], axis=0, count=3)
        inputs = {"source": torch.tensor([[1, 2], [3, 4]])}
        expected = torch.tensor([[1, 2], [3, 4], [1, 2], [3, 4], [1, 2], [3, 4]])
        torch.testing.assert_close(layer(inputs)["tiled"], expected)

    def test_forward_1d_axis_0(self) -> None:
        """A 1D tensor tiled along axis 0 stays 1D."""
        layer = Tile(input_cols=["source"], output_cols=["tiled"], axis=0, count=4)
        inputs = {"source": torch.tensor([1, 2, 3])}
        outputs = layer(inputs)
        assert outputs["tiled"].dim() == 1
        torch.testing.assert_close(
            outputs["tiled"], torch.tensor([1, 2, 3, 1, 2, 3, 1, 2, 3, 1, 2, 3])
        )

    def test_forward_with_target(self) -> None:
        """The count is inferred from the trailing target tensor's shape."""
        layer = Tile(
            input_cols=["source", "target"],
            output_cols=["tiled"],
            axis=0,
            target_tensor_provided=True,
        )
        inputs = {
            "source": torch.tensor([[1, 2]]),
            "target": torch.tensor([[0], [0], [0]]),
        }
        torch.testing.assert_close(
            layer(inputs)["tiled"], torch.tensor([[1, 2], [1, 2], [1, 2]])
        )

    def test_missing_count_and_target_raises(self) -> None:
        """Construction without a count or target tensor raises ``ValueError``."""
        with pytest.raises(ValueError, match="count must be specified"):
            Tile(input_cols=["source"], output_cols=["tiled"], axis=0)

    def test_axis_1_autounsqueeze_1d(self) -> None:
        """A 1D input with axis=1 is unsqueezed to produce a 2D result."""
        layer = Tile(input_cols=["source"], output_cols=["tiled"], axis=1, count=3)
        inputs = {"source": torch.tensor([10, 20])}
        outputs = layer(inputs)
        assert outputs["tiled"].shape == torch.Size([2, 3])
        torch.testing.assert_close(
            outputs["tiled"], torch.tensor([[10, 10, 10], [20, 20, 20]])
        )

    def test_axis_1_autounsqueeze_with_target(self) -> None:
        """Auto-unsqueeze also applies when the count comes from a target."""
        layer = Tile(
            input_cols=["source", "target"],
            output_cols=["tiled"],
            axis=1,
            target_tensor_provided=True,
        )
        inputs = {
            "source": torch.tensor([5, 10, 15]),
            "target": torch.tensor([[0, 0, 0, 0]]),
        }
        outputs = layer(inputs)
        assert outputs["tiled"].shape == torch.Size([3, 4])
        torch.testing.assert_close(
            outputs["tiled"],
            torch.tensor([[5, 5, 5, 5], [10, 10, 10, 10], [15, 15, 15, 15]]),
        )

    def test_axis_negative_1(self) -> None:
        """A negative axis tiles along the last dimension."""
        layer = Tile(input_cols=["source"], output_cols=["tiled"], axis=-1, count=3)
        inputs = {"source": torch.tensor([[1, 2], [3, 4]])}
        torch.testing.assert_close(
            layer(inputs)["tiled"],
            torch.tensor([[1, 2, 1, 2, 1, 2], [3, 4, 3, 4, 3, 4]]),
        )

    def test_2d_axis_1_no_unsqueeze(self) -> None:
        """A 2D input with axis=1 tiles without unsqueezing."""
        layer = Tile(input_cols=["source"], output_cols=["tiled"], axis=1, count=2)
        inputs = {"source": torch.tensor([[1], [2]])}
        outputs = layer(inputs)
        assert outputs["tiled"].dim() == 2
        torch.testing.assert_close(outputs["tiled"], torch.tensor([[1, 1], [2, 2]]))

    def test_multiple_columns_with_count(self) -> None:
        """Multiple source columns are each tiled by the count."""
        layer = Tile(input_cols=["s1", "s2"], output_cols=["o1", "o2"], count=2)
        inputs = {"s1": torch.tensor([1, 2]), "s2": torch.tensor([3, 4])}
        outputs = layer(inputs)
        torch.testing.assert_close(outputs["o1"], torch.tensor([1, 2, 1, 2]))
        torch.testing.assert_close(outputs["o2"], torch.tensor([3, 4, 3, 4]))


class TestPadOrCrop1D:
    """Padding/cropping to a fixed length, including sentinel handling."""

    def test_invalid_align_raises(self) -> None:
        """An align other than left/right raises ``ValueError``."""
        with pytest.raises(ValueError, match="align"):
            PadOrCrop1D(
                input_cols=["x"], output_cols=["y"], max_length=3, align="center"
            )

    def test_non_positive_max_length_raises(self) -> None:
        """A non-positive max_length raises ``ValueError``."""
        with pytest.raises(ValueError, match="positive"):
            PadOrCrop1D(input_cols=["x"], output_cols=["y"], max_length=0)

    def test_mismatched_columns_raises(self) -> None:
        """Unequal input/output column counts raise ``ValueError``."""
        with pytest.raises(ValueError, match="same length"):
            PadOrCrop1D(input_cols=["a", "b"], output_cols=["y"], max_length=3)

    def test_left_padding_pads_right(self) -> None:
        """align='left' pads on the right to reach max_length."""
        layer = PadOrCrop1D(
            input_cols=["f"], output_cols=["o"], max_length=5, pad_value=-1
        )
        out = layer({"f": torch.tensor([1, 2, 3])})["o"]
        torch.testing.assert_close(out, torch.tensor([1, 2, 3, -1, -1]))

    def test_left_cropping_keeps_first_n(self) -> None:
        """align='left' crops from the right, keeping the first N."""
        layer = PadOrCrop1D(
            input_cols=["f"], output_cols=["o"], max_length=3, pad_value=-1
        )
        out = layer({"f": torch.tensor([1, 2, 3, 4, 5])})["o"]
        torch.testing.assert_close(out, torch.tensor([1, 2, 3]))

    def test_right_padding_pads_left(self) -> None:
        """align='right' pads on the left."""
        layer = PadOrCrop1D(
            input_cols=["f"],
            output_cols=["o"],
            max_length=5,
            pad_value=-1,
            align="right",
        )
        out = layer({"f": torch.tensor([1, 2, 3])})["o"]
        torch.testing.assert_close(out, torch.tensor([-1, -1, 1, 2, 3]))

    def test_right_cropping_keeps_last_n(self) -> None:
        """align='right' crops from the left, keeping the last N."""
        layer = PadOrCrop1D(
            input_cols=["f"],
            output_cols=["o"],
            max_length=3,
            pad_value=-1,
            align="right",
        )
        out = layer({"f": torch.tensor([1, 2, 3, 4, 5])})["o"]
        torch.testing.assert_close(out, torch.tensor([3, 4, 5]))

    def test_right_align_keeps_real_prefix_over_trailing_sentinels(self) -> None:
        """align='right' crops to real content, not to sentinel padding.

        A short sequence collated into a longer tensor carries trailing NaN
        padding. Cropping to the last ``max_length`` raw positions would return
        that padding; the real prefix must be kept instead.
        """
        layer = PadOrCrop1D(
            input_cols=["f"],
            output_cols=["o"],
            max_length=3,
            pad_value=-1.0,
            align="right",
        )
        out = layer({"f": torch.tensor([1.0, 2.0, 3.0, float("nan"), float("nan")])})[
            "o"
        ]
        torch.testing.assert_close(out, torch.tensor([1.0, 2.0, 3.0]))

    def test_right_align_int_sentinel_stripped_before_crop(self) -> None:
        """The integer collation sentinel is stripped before an align='right' crop."""
        layer = PadOrCrop1D(
            input_cols=["f"],
            output_cols=["o"],
            max_length=2,
            pad_value=-1,
            align="right",
        )
        out = layer(
            {"f": torch.tensor([10, 20, 30, INT32_SENTINEL], dtype=torch.int32)}
        )["o"]
        torch.testing.assert_close(out, torch.tensor([20, 30], dtype=torch.int32))

    def test_right_align_pads_when_content_shorter_than_max_length(self) -> None:
        """Sentinel-trimmed content shorter than max_length is left-padded."""
        layer = PadOrCrop1D(
            input_cols=["f"],
            output_cols=["o"],
            max_length=4,
            pad_value=-1.0,
            align="right",
        )
        out = layer({"f": torch.tensor([1.0, 2.0, float("nan")])})["o"]
        torch.testing.assert_close(out, torch.tensor([-1.0, -1.0, 1.0, 2.0]))

    def test_right_align_all_sentinel_returns_all_pad(self) -> None:
        """An input that is entirely sentinel yields only ``pad_value``."""
        layer = PadOrCrop1D(
            input_cols=["f"],
            output_cols=["o"],
            max_length=3,
            pad_value=-1.0,
            align="right",
        )
        out = layer({"f": torch.tensor([float("nan"), float("nan")])})["o"]
        torch.testing.assert_close(out, torch.tensor([-1.0, -1.0, -1.0]))

    def test_right_align_content_length_is_per_row(self) -> None:
        """Each batch row is cropped against its own real-content length."""
        layer = PadOrCrop1D(
            input_cols=["f"],
            output_cols=["o"],
            max_length=3,
            pad_value=-1.0,
            align="right",
        )
        inputs = {
            "f": torch.tensor(
                [
                    [1.0, 2.0, 3.0, float("nan"), float("nan")],
                    [1.0, 2.0, 3.0, 4.0, 5.0],
                    [1.0, float("nan"), float("nan"), float("nan"), float("nan")],
                ]
            )
        }
        torch.testing.assert_close(
            layer(inputs)["o"],
            torch.tensor([[1.0, 2.0, 3.0], [3.0, 4.0, 5.0], [-1.0, -1.0, 1.0]]),
        )

    def test_left_align_unaffected_by_trailing_sentinels(self) -> None:
        """align='left' keeps the real prefix, as it always has."""
        layer = PadOrCrop1D(
            input_cols=["f"],
            output_cols=["o"],
            max_length=3,
            pad_value=-1.0,
            align="left",
        )
        out = layer({"f": torch.tensor([1.0, 2.0, 3.0, float("nan"), float("nan")])})[
            "o"
        ]
        torch.testing.assert_close(out, torch.tensor([1.0, 2.0, 3.0]))

    def test_exact_length_unchanged(self) -> None:
        """An input already at max_length is returned unchanged."""
        layer = PadOrCrop1D(input_cols=["f"], output_cols=["o"], max_length=3)
        out = layer({"f": torch.tensor([1, 2, 3])})["o"]
        torch.testing.assert_close(out, torch.tensor([1, 2, 3]))

    def test_preserve_input_dtype_when_none(self) -> None:
        """With dtype=None the input dtype is preserved on pad and crop."""
        pad_layer = PadOrCrop1D(
            input_cols=["f"], output_cols=["o"], max_length=5, pad_value=0
        )
        pad_out = pad_layer({"f": torch.tensor([1, 2, 3], dtype=torch.int32)})["o"]
        assert pad_out.dtype == torch.int32
        torch.testing.assert_close(
            pad_out, torch.tensor([1, 2, 3, 0, 0], dtype=torch.int32)
        )

    def test_explicit_dtype_overrides_input(self) -> None:
        """An explicit dtype converts the output."""
        layer = PadOrCrop1D(
            input_cols=["f"],
            output_cols=["o"],
            max_length=4,
            dtype=torch.float32,
            pad_value=0,
        )
        out = layer({"f": torch.tensor([1, 2, 3], dtype=torch.int64)})["o"]
        assert out.dtype == torch.float32
        torch.testing.assert_close(out, torch.tensor([1.0, 2.0, 3.0, 0.0]))

    def test_string_dtype_alias(self) -> None:
        """A string dtype alias resolves to the target dtype."""
        layer = PadOrCrop1D(
            input_cols=["f"],
            output_cols=["o"],
            max_length=3,
            dtype="float64",
            pad_value=0.0,
        )
        out = layer({"f": torch.tensor([1, 2, 3, 4], dtype=torch.int32)})["o"]
        assert out.dtype == torch.float64
        torch.testing.assert_close(
            out, torch.tensor([1.0, 2.0, 3.0], dtype=torch.float64)
        )

    def test_empty_last_dim_returns_full_pad(self) -> None:
        """A zero-length input becomes a full pad_value tensor of max_length."""
        layer = PadOrCrop1D(
            input_cols=["f"],
            output_cols=["o"],
            max_length=5,
            dtype=torch.float32,
            pad_value=-1.0,
        )
        out = layer({"f": torch.tensor([], dtype=torch.float32)})["o"]
        torch.testing.assert_close(out, torch.full((5,), -1.0))

    def test_empty_last_dim_preserves_batch_dim(self) -> None:
        """An empty last dim keeps the batch dimension when padding a 2D input."""
        layer = PadOrCrop1D(
            input_cols=["f"],
            output_cols=["o"],
            max_length=3,
            pad_value=7,
            dtype=torch.int64,
        )
        out = layer({"f": torch.empty((2, 0), dtype=torch.int64)})["o"]
        assert out.shape == torch.Size([2, 3])
        torch.testing.assert_close(out, torch.full((2, 3), 7, dtype=torch.int64))

    def test_empty_last_dim_preserves_dtype(self) -> None:
        """An empty int64 input keeps its dtype in the full-pad output."""
        layer = PadOrCrop1D(
            input_cols=["f"], output_cols=["o"], max_length=3, pad_value=-1
        )
        out = layer({"f": torch.tensor([], dtype=torch.int64)})["o"]
        assert out.dtype == torch.int64
        torch.testing.assert_close(out, torch.tensor([-1, -1, -1], dtype=torch.int64))

    def test_nan_collation_sentinel_uses_pad_value(self) -> None:
        """NaN collation sentinels become pad_value."""
        layer = PadOrCrop1D(
            input_cols=["f"], output_cols=["o"], max_length=4, pad_value=4.61
        )
        out = layer({"f": torch.tensor([[1.0, float("nan"), 3.0]])})["o"]
        assert out[0, 1].item() == pytest.approx(4.61, abs=1e-4)
        assert out[0, 3].item() == pytest.approx(4.61, abs=1e-4)

    def test_int_sentinel_uses_pad_value(self) -> None:
        """INT32_SENTINEL collation positions become pad_value."""
        layer = PadOrCrop1D(
            input_cols=["f"], output_cols=["o"], max_length=4, pad_value=-1
        )
        out = layer({"f": torch.tensor([[10, INT32_SENTINEL, 30]], dtype=torch.int32)})[
            "o"
        ]
        torch.testing.assert_close(
            out, torch.tensor([[10, -1, 30, -1]], dtype=torch.int32)
        )

    def test_non_standard_sentinel_is_left_as_data(self) -> None:
        """A non-standard numeric value (not NaN/INT32_SENTINEL) is left as data."""
        layer = PadOrCrop1D(
            input_cols=["f"],
            output_cols=["o"],
            max_length=4,
            dtype=torch.int32,
            pad_value=-1,
        )
        out = layer({"f": torch.tensor([10, -2_000_000_000, 30], dtype=torch.int32)})[
            "o"
        ]
        torch.testing.assert_close(
            out, torch.tensor([10, -2_000_000_000, 30, -1], dtype=torch.int32)
        )

    def test_over_padding_short_input(self) -> None:
        """A single element pads out to the full max_length."""
        layer = PadOrCrop1D(
            input_cols=["f"],
            output_cols=["o"],
            max_length=4,
            pad_value=9,
            dtype=torch.int64,
        )
        out = layer({"f": torch.tensor([7])})["o"]
        torch.testing.assert_close(out, torch.tensor([7, 9, 9, 9], dtype=torch.int64))

    def test_multiple_columns(self) -> None:
        """Multiple columns are padded/cropped independently."""
        layer = PadOrCrop1D(
            input_cols=["f1", "f2"],
            output_cols=["o1", "o2"],
            max_length=4,
            dtype=torch.int64,
            pad_value=-1,
        )
        inputs = {
            "f1": torch.tensor([1, 2, 3, 4, 5]),
            "f2": torch.tensor([6, 7, 8, 9, 10]),
        }
        outputs = layer(inputs)
        torch.testing.assert_close(
            outputs["o1"], torch.tensor([1, 2, 3, 4], dtype=torch.int64)
        )
        torch.testing.assert_close(
            outputs["o2"], torch.tensor([6, 7, 8, 9], dtype=torch.int64)
        )


class TestScale:
    """Scalar multiplication."""

    def test_mismatched_columns_raises(self) -> None:
        """Unequal input/output column counts raise ``ValueError``."""
        with pytest.raises(ValueError, match="same length"):
            Scale(input_cols=["a", "b"], output_cols=["out"], factor=2.0)

    def test_basic_scaling(self) -> None:
        """Values are multiplied by the factor."""
        layer = Scale(input_cols=["val"], output_cols=["scaled"], factor=2.0)
        inputs = {"val": torch.tensor([1.0, 2.0, 3.0])}
        torch.testing.assert_close(
            layer(inputs)["scaled"], torch.tensor([2.0, 4.0, 6.0])
        )

    def test_negative_factor(self) -> None:
        """A negative factor flips signs and scales."""
        layer = Scale(input_cols=["val"], output_cols=["scaled"], factor=-0.5)
        inputs = {"val": torch.tensor([10.0, -20.0])}
        torch.testing.assert_close(layer(inputs)["scaled"], torch.tensor([-5.0, 10.0]))

    def test_multiple_columns(self) -> None:
        """Each column is scaled by the same factor."""
        layer = Scale(input_cols=["v1", "v2"], output_cols=["s1", "s2"], factor=10.0)
        inputs = {"v1": torch.tensor([1.0, 2.0]), "v2": torch.tensor([3.0, 4.0])}
        outputs = layer(inputs)
        torch.testing.assert_close(outputs["s1"], torch.tensor([10.0, 20.0]))
        torch.testing.assert_close(outputs["s2"], torch.tensor([30.0, 40.0]))


class TestClip:
    """Clamping to a range with optional value exemption."""

    def test_mismatched_columns_raises(self) -> None:
        """Unequal input/output column counts raise ``ValueError``."""
        with pytest.raises(ValueError, match="same length"):
            Clip(input_cols=["a", "b"], output_cols=["out"], min_value=0.0)

    def test_no_bounds_raises(self) -> None:
        """Omitting both bounds raises ``ValueError`` at construction time."""
        with pytest.raises(ValueError, match="min_value or max_value"):
            Clip(input_cols=["val"], output_cols=["clipped"])

    def test_basic_clip(self) -> None:
        """Values are clamped to ``[min_value, max_value]``."""
        layer = Clip(
            input_cols=["val"], output_cols=["clipped"], min_value=0.0, max_value=10.0
        )
        inputs = {"val": torch.tensor([-5.0, 5.0, 15.0])}
        torch.testing.assert_close(
            layer(inputs)["clipped"], torch.tensor([0.0, 5.0, 10.0])
        )

    def test_only_min_bound(self) -> None:
        """A missing upper bound leaves large values untouched."""
        layer = Clip(input_cols=["val"], output_cols=["clipped"], min_value=0.0)
        inputs = {"val": torch.tensor([-5.0, 5.0, 1000.0])}
        torch.testing.assert_close(
            layer(inputs)["clipped"], torch.tensor([0.0, 5.0, 1000.0])
        )

    def test_only_max_bound(self) -> None:
        """A missing lower bound leaves small values untouched."""
        layer = Clip(input_cols=["val"], output_cols=["clipped"], max_value=10.0)
        inputs = {"val": torch.tensor([-500.0, 5.0, 15.0])}
        torch.testing.assert_close(
            layer(inputs)["clipped"], torch.tensor([-500.0, 5.0, 10.0])
        )

    def test_ignore_value_preserved(self) -> None:
        """Positions equal to ignore_value are preserved even if out of range."""
        layer = Clip(
            input_cols=["val"],
            output_cols=["clipped"],
            min_value=0.0,
            max_value=10.0,
            ignore_value=-1.0,
        )
        inputs = {"val": torch.tensor([-5.0, 5.0, 15.0, -1.0])}
        torch.testing.assert_close(
            layer(inputs)["clipped"], torch.tensor([0.0, 5.0, 10.0, -1.0])
        )

    def test_multiple_columns(self) -> None:
        """Each column is clamped independently."""
        layer = Clip(
            input_cols=["v1", "v2"],
            output_cols=["o1", "o2"],
            min_value=0.0,
            max_value=1.0,
        )
        inputs = {"v1": torch.tensor([-1.0, 2.0]), "v2": torch.tensor([0.5, 5.0])}
        outputs = layer(inputs)
        torch.testing.assert_close(outputs["o1"], torch.tensor([0.0, 1.0]))
        torch.testing.assert_close(outputs["o2"], torch.tensor([0.5, 1.0]))


def _b2_layer_cases() -> list[tuple[str, TorchTransformBaseLayer, dict]]:
    """Build (id, layer, inputs) cases covering every B2 structural layer.

    Each entry exercises the ``forward`` path that a served model relies on so
    the TorchScript round-trip test can confirm the scripted module (and a
    save/load cycle) reproduces the eager output.
    """
    return [
        (
            "tensor_col_fill_none_float",
            TensorColFillNone(input_cols=["f"], output_cols=["o"], default_value=0.0),
            {"f": torch.tensor([1.0, float("nan"), 3.0])},
        ),
        (
            "tensor_col_fill_none_int32",
            TensorColFillNone(input_cols=["f"], output_cols=["o"], default_value=9),
            {
                "f": torch.tensor(
                    [1, torch.iinfo(torch.int32).min, 3], dtype=torch.int32
                )
            },
        ),
        (
            "case_when_scalar_default",
            CaseWhen(
                input_cols=["c1", "v1", "c2", "v2"],
                output_cols=["o"],
                default_value=-1,
            ),
            {
                "c1": torch.tensor([True, False, False]),
                "v1": torch.tensor([10, 20, 30]),
                "c2": torch.tensor([False, True, False]),
                "v2": torch.tensor([100, 200, 300]),
            },
        ),
        (
            "case_when_tensor_default",
            CaseWhen(
                input_cols=["c1", "v1"],
                output_cols=["o"],
                default_value=torch.tensor(-1),
            ),
            {
                "c1": torch.tensor([True, False, True]),
                "v1": torch.tensor([100, 200, 300]),
            },
        ),
        (
            "compare_equal",
            Compare(input_cols=["l", "r"], output_cols=["o"], compare_op="equal"),
            {"l": torch.tensor([1, 2, 3]), "r": torch.tensor([1, 3, 3])},
        ),
        (
            "compare_greater_equal",
            Compare(
                input_cols=["l", "r"], output_cols=["o"], compare_op="greater_equal"
            ),
            {"l": torch.tensor([1, 2, 3]), "r": torch.tensor(2)},
        ),
        (
            "tile_count",
            Tile(input_cols=["s"], output_cols=["o"], count=2),
            {"s": torch.tensor([1, 2])},
        ),
        (
            "tile_axis1_unsqueeze",
            Tile(input_cols=["s"], output_cols=["o"], axis=1, count=3),
            {"s": torch.tensor([10, 20])},
        ),
        (
            "pad_or_crop_explicit_dtype",
            PadOrCrop1D(
                input_cols=["f"],
                output_cols=["o"],
                max_length=5,
                dtype=torch.int64,
                pad_value=0,
            ),
            {"f": torch.tensor([1, 2, 3])},
        ),
        (
            "pad_or_crop_preserve_dtype",
            PadOrCrop1D(input_cols=["f"], output_cols=["o"], max_length=5, pad_value=0),
            {"f": torch.tensor([1, 2, 3], dtype=torch.int32)},
        ),
        (
            "pad_or_crop_align_right",
            PadOrCrop1D(
                input_cols=["f"],
                output_cols=["o"],
                max_length=5,
                pad_value=-1,
                align="right",
            ),
            {"f": torch.tensor([1, 2, 3, 4, 5, 6, 7])},
        ),
        (
            "pad_or_crop_ragged_nan",
            PadOrCrop1D(
                input_cols=["f"],
                output_cols=["o"],
                max_length=5,
                pad_value=0.0,
            ),
            {"f": torch.tensor([1.0, float("nan"), 3.0])},
        ),
        (
            "pad_or_crop_ragged_int",
            PadOrCrop1D(
                input_cols=["f"],
                output_cols=["o"],
                max_length=5,
                pad_value=-1,
            ),
            {"f": torch.tensor([10, INT32_SENTINEL, 30], dtype=torch.int32)},
        ),
        (
            "scale",
            Scale(input_cols=["v"], output_cols=["o"], factor=2.0),
            {"v": torch.tensor([1.0, 2.0])},
        ),
        (
            "clip",
            Clip(input_cols=["v"], output_cols=["o"], min_value=0.0, max_value=10.0),
            {"v": torch.tensor([-5.0, 5.0, 15.0])},
        ),
        (
            "clip_ignore_value",
            Clip(
                input_cols=["v"],
                output_cols=["o"],
                min_value=0.0,
                max_value=10.0,
                ignore_value=-1.0,
            ),
            {"v": torch.tensor([-5.0, 5.0, 15.0, -1.0])},
        ),
    ]


class TestIDHashTokenizer:
    """The IDHashTokenizer wrapper layer: lookup plus sentinel handling."""

    def test_mismatched_columns_raises(self) -> None:
        """Unequal input/output column counts raise ``ValueError``."""
        with pytest.raises(ValueError, match="same length"):
            IDHashTokenizer(
                input_cols=["a", "b"], output_cols=["o"], vocabulary=[1, 2, 3]
            )

    def test_forward_basic(self) -> None:
        """Known values map to their vocabulary position."""
        layer = IDHashTokenizer(
            input_cols=["x"], output_cols=["o"], vocabulary=[10, 20, 30]
        )
        out = layer({"x": torch.tensor([10, 30, 20], dtype=torch.int32)})["o"]
        torch.testing.assert_close(out, torch.tensor([0, 2, 1], dtype=torch.int32))

    def test_unknown_value_maps_to_unk_index(self) -> None:
        """A value outside the vocabulary maps to unk_index."""
        layer = IDHashTokenizer(
            input_cols=["x"], output_cols=["o"], vocabulary=[10, 20]
        )
        out = layer({"x": torch.tensor([10, 99], dtype=torch.int32)})["o"]
        torch.testing.assert_close(
            out, torch.tensor([0, layer.unk_index], dtype=torch.int32)
        )

    def test_multi_column(self) -> None:
        """Multiple columns tokenize independently."""
        layer = IDHashTokenizer(
            input_cols=["a", "b"], output_cols=["oa", "ob"], vocabulary=[1, 2]
        )
        out = layer(
            {
                "a": torch.tensor([1], dtype=torch.int32),
                "b": torch.tensor([2], dtype=torch.int32),
            }
        )
        torch.testing.assert_close(out["oa"], torch.tensor([0], dtype=torch.int32))
        torch.testing.assert_close(out["ob"], torch.tensor([1], dtype=torch.int32))

    def test_exposed_attributes(self) -> None:
        """vocabulary/unk_index/output_vocab_size are forwarded from the core."""
        layer = IDHashTokenizer(
            input_cols=["x"], output_cols=["o"], vocabulary=[5, 6, 7]
        )
        assert layer.vocabulary == [5, 6, 7]
        assert layer.unk_index == 3
        assert layer.output_vocab_size == 3

    def test_nan_sentinel_is_preserved(self) -> None:
        """A NaN sentinel in float input is preserved as INT32_SENTINEL output.

        The mask is captured before the float->int32 cast the core requires,
        since NaN does not survive that cast.
        """
        layer = IDHashTokenizer(
            input_cols=["x"], output_cols=["o"], vocabulary=[1, 2, 3]
        )
        out = layer({"x": torch.tensor([1.0, float("nan"), 3.0])})["o"]
        torch.testing.assert_close(
            out, torch.tensor([0, INT32_SENTINEL, 2], dtype=torch.int32)
        )

    def test_int_sentinel_is_preserved(self) -> None:
        """An INT32_SENTINEL position in integer input is preserved."""
        layer = IDHashTokenizer(
            input_cols=["x"], output_cols=["o"], vocabulary=[1, 2, 3]
        )
        out = layer({"x": torch.tensor([1, INT32_SENTINEL, 3], dtype=torch.int32)})["o"]
        torch.testing.assert_close(
            out, torch.tensor([0, INT32_SENTINEL, 2], dtype=torch.int32)
        )


def _b4_layer_cases() -> list[tuple[str, TorchTransformBaseLayer, dict]]:
    """Layer/input pairs shared by the B4 TorchScript round-trip test."""
    return [
        (
            "id_hash_tokenizer",
            IDHashTokenizer(
                input_cols=["x"], output_cols=["o"], vocabulary=[10, 20, 30]
            ),
            {"x": torch.tensor([10, 30, 99], dtype=torch.int32)},
        ),
        (
            "id_hash_tokenizer_nan_sentinel",
            IDHashTokenizer(input_cols=["x"], output_cols=["o"], vocabulary=[1, 2, 3]),
            {"x": torch.tensor([1.0, float("nan"), 3.0])},
        ),
    ]


class TestB4TorchScriptRoundTrip:
    """The B4 IDHashTokenizer wrapper must script, save/load, and match eager."""

    @pytest.mark.parametrize(
        ("layer", "inputs"),
        [(layer, inputs) for _, layer, inputs in _b4_layer_cases()],
        ids=[case_id for case_id, _, _ in _b4_layer_cases()],
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

        model_path = tmp_path / "scripted_b4_layer.pt"
        scripted.save(str(model_path))
        loaded = torch.jit.load(str(model_path))
        loaded_out = loaded(inputs)
        for key in eager:
            torch.testing.assert_close(loaded_out[key], eager[key])


class TestB2TorchScriptRoundTrip:
    """Every B2 structural layer must script, save/load, and match eager output."""

    @pytest.mark.parametrize(
        ("layer", "inputs"),
        [(layer, inputs) for _, layer, inputs in _b2_layer_cases()],
        ids=[case_id for case_id, _, _ in _b2_layer_cases()],
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

        model_path = tmp_path / "scripted_b2_layer.pt"
        scripted.save(str(model_path))
        loaded = torch.jit.load(str(model_path))
        loaded_out = loaded(inputs)
        for key in eager:
            torch.testing.assert_close(loaded_out[key], eager[key])
