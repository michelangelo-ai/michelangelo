"""Tests for :mod:`michelangelo.lib.native_transform.torch.schema`.

Covers shape/dtype inference from a sample forward pass, spec-declared
dtype/shape overrides, output-column filtering via ``columns_to_keep``, and
the high-level ``derive_native_transform_schema`` entry point end to end.
"""

from __future__ import annotations

import numpy as np
import pytest

# These tests build real torch layers/modules and a real TransformSpec.
torch = pytest.importorskip("torch")
pytest.importorskip("pydantic")

from michelangelo.lib.model_manager.schema import DataType  # noqa: E402
from michelangelo.lib.native_transform.torch.base_transform_module import (  # noqa: E402
    get_transform_module,
)
from michelangelo.lib.native_transform.torch.schema import (  # noqa: E402
    _build_schema_items,
    _collect_input_shape_overrides,
    _create_native_transform_schema,
    _filter_output_cols,
    _get_sample_shapes_and_dtypes,
    _to_batched_tensor,
    derive_native_transform_schema,
)
from michelangelo.lib.native_transform.torch.transform_spec import (  # noqa: E402
    TransformSpec,
)


class TestToBatchedTensor:
    """``_to_batched_tensor``'s value -> (tensor, shape) conversion."""

    def test_scalar_returns_correct_shape(self) -> None:
        """A bare scalar batches to shape [1]."""
        tensor, shape = _to_batched_tensor(1.0)
        assert shape == [1]
        assert tensor.shape[0] == 1

    def test_list_returns_correct_shape(self) -> None:
        """A 1D list's shape excludes the batch dimension."""
        _, shape = _to_batched_tensor([1.0, 2.0, 3.0])
        assert shape == [3]

    def test_2d_list_returns_correct_shape(self) -> None:
        """A 2D list's shape preserves both dimensions."""
        _, shape = _to_batched_tensor([[1.0, 2.0], [3.0, 4.0]])
        assert shape == [2, 2]

    def test_numpy_array_returns_correct_shape(self) -> None:
        """A numpy array converts to the same shape as an equivalent list."""
        _, shape = _to_batched_tensor(np.array([1.0, 2.0]))
        assert shape == [2]

    def test_torch_tensor_returns_correct_shape(self) -> None:
        """An already-a-tensor value converts without changing its shape."""
        _, shape = _to_batched_tensor(torch.tensor([1.0, 2.0, 3.0]))
        assert shape == [3]

    def test_none_returns_none_shape(self) -> None:
        """A None value falls back to a [1, 1] float32 zero tensor with no shape."""
        tensor, shape = _to_batched_tensor(None)
        assert shape is None
        assert tensor.shape == torch.Size([1, 1])
        assert tensor.dtype == torch.float32

    def test_none_with_dtype_hint_uses_hint(self) -> None:
        """A null column should use dtype_hint from the Arrow schema."""
        tensor, shape = _to_batched_tensor(None, dtype_hint=torch.int64)
        assert shape is None
        assert tensor.dtype == torch.int64

    def test_unconvertible_value_returns_none_shape(self) -> None:
        """A value torch.as_tensor can't convert falls back like a None value."""
        tensor, shape = _to_batched_tensor("string_value")
        assert shape is None
        assert tensor.shape == torch.Size([1, 1])

    def test_dtype_hint_ignored_when_value_present(self) -> None:
        """dtype_hint must not override an actual present value's dtype."""
        tensor, _ = _to_batched_tensor(
            np.array([1, 2], dtype=np.int32), dtype_hint=torch.float64
        )
        assert tensor.dtype == torch.int32

    def test_preserves_int32_dtype(self) -> None:
        """A numpy int32 array's dtype survives batching."""
        tensor, shape = _to_batched_tensor(np.array([1, 2, 3], dtype=np.int32))
        assert tensor.dtype == torch.int32
        assert shape == [3]

    def test_preserves_float64_dtype(self) -> None:
        """A numpy float64 array's dtype survives batching."""
        tensor, shape = _to_batched_tensor(np.array([1.0, 2.0], dtype=np.float64))
        assert tensor.dtype == torch.float64
        assert shape == [2]


class TestBuildSchemaItems:
    """``_build_schema_items``'s column -> ``ModelSchemaItem`` assembly."""

    def test_creates_items_with_dtypes_and_shapes(self) -> None:
        """Each column gets its own resolved data_type and shape."""
        items = _build_schema_items(
            cols=["col1", "col2"],
            dtype_map={"col1": torch.float64, "col2": torch.int64},
            shape_map={"col1": [10], "col2": [5]},
        )
        assert len(items) == 2
        assert items[0].name == "col1"
        assert items[0].data_type == DataType.DOUBLE
        assert items[0].shape == [10]
        assert items[1].name == "col2"
        assert items[1].data_type == DataType.LONG
        assert items[1].shape == [5]

    def test_defaults_to_float_for_missing_dtype(self) -> None:
        """A column absent from dtype_map defaults to DataType.FLOAT."""
        items = _build_schema_items(["col1"], {}, {})
        assert items[0].data_type == DataType.FLOAT
        assert items[0].shape is None


class TestFilterOutputCols:
    """``_filter_output_cols``'s ``columns_to_keep`` filtering."""

    def test_returns_all_when_columns_to_keep_none(self) -> None:
        """A None columns_to_keep keeps every output column."""
        assert _filter_output_cols(["a", "b", "c"], None) == ["a", "b", "c"]

    def test_filters_to_kept_columns(self) -> None:
        """Only columns named in columns_to_keep survive."""
        assert _filter_output_cols(["a", "b", "c", "d"], ["b", "d"]) == ["b", "d"]

    def test_preserves_output_cols_order(self) -> None:
        """Filtering preserves output_cols' original order, not columns_to_keep's."""
        assert _filter_output_cols(["c", "a", "b"], ["a", "b", "c"]) == ["c", "a", "b"]


class TestGetSampleShapesAndDtypes:
    """``_get_sample_shapes_and_dtypes``'s forward-pass-driven inference."""

    def _concatenate_module(self):
        """Build a one-layer Concatenate module over columns a, b -> ab."""
        spec = TransformSpec(
            raw_transform_specs={
                "transform_specs": [
                    {
                        "transform_name": "Concatenate",
                        "input_cols": ["a", "b"],
                        "output_cols": ["ab"],
                    }
                ]
            }
        )
        return get_transform_module(spec, start_level=0)

    def test_derives_input_and_output_shapes(self) -> None:
        """Shapes for both inputs and the concatenated output are derived."""
        module = self._concatenate_module()
        input_shapes, output_shapes, _, _ = _get_sample_shapes_and_dtypes(
            module, {"a": [1.0, 2.0], "b": [3.0, 4.0]}
        )
        assert input_shapes == {"a": [2], "b": [2]}
        assert output_shapes["ab"] == [4]

    def test_handles_null_columns(self) -> None:
        """A None sample value is excluded from input_shapes but not output_shapes."""
        module = self._concatenate_module()
        input_shapes, output_shapes, _, _ = _get_sample_shapes_and_dtypes(
            module, {"a": [1.0, 2.0], "b": None}
        )
        assert "a" in input_shapes
        assert "b" not in input_shapes
        assert "ab" in output_shapes

    def test_preserves_int32_through_concatenate(self) -> None:
        """int32 inputs stay int32 through Concatenate when dtypes match."""
        module = self._concatenate_module()
        sample_data = {
            "a": np.array([1, 2], dtype=np.int32),
            "b": np.array([3, 4], dtype=np.int32),
        }
        _, _, input_dtypes, output_dtypes = _get_sample_shapes_and_dtypes(
            module, sample_data
        )
        assert input_dtypes["a"] == torch.int32
        assert input_dtypes["b"] == torch.int32
        assert output_dtypes["ab"] == torch.int32

    def test_promotes_mixed_dtypes_through_concatenate(self) -> None:
        """torch.cat promotes int32 + float32 to float32."""
        module = self._concatenate_module()
        sample_data = {
            "a": np.array([1, 2], dtype=np.int32),
            "b": np.array([3.0, 4.0], dtype=np.float32),
        }
        _, _, input_dtypes, output_dtypes = _get_sample_shapes_and_dtypes(
            module, sample_data
        )
        assert input_dtypes["a"] == torch.int32
        assert input_dtypes["b"] == torch.float32
        assert output_dtypes["ab"] == torch.float32

    def test_null_column_uses_dtype_hint_from_input_dtype_map(self) -> None:
        """A None sample value falls back to input_dtype_map's dtype, not float32."""
        module = self._concatenate_module()
        sample_data = {"a": np.array([1, 2], dtype=np.int32), "b": None}
        _, _, input_dtypes, output_dtypes = _get_sample_shapes_and_dtypes(
            module, sample_data, input_dtype_map={"b": torch.int32}
        )
        assert input_dtypes["a"] == torch.int32
        assert input_dtypes["b"] == torch.int32
        assert output_dtypes["ab"] == torch.int32

    def test_null_column_without_dtype_hint_defaults_to_float32(self) -> None:
        """A None sample value with no dtype hint falls back to float32."""
        module = self._concatenate_module()
        sample_data = {"a": np.array([1, 2], dtype=np.int32), "b": None}
        _, _, input_dtypes, output_dtypes = _get_sample_shapes_and_dtypes(
            module, sample_data
        )
        assert input_dtypes["a"] == torch.int32
        assert input_dtypes["b"] == torch.float32
        assert output_dtypes["ab"] == torch.float32

    def test_padorcrop1d_preserves_int32(self) -> None:
        """PadOrCrop1D with dtype=None preserves an int32 input's dtype."""
        spec = TransformSpec(
            raw_transform_specs={
                "transform_specs": [
                    {
                        "transform_name": "PadOrCrop1D",
                        "input_cols": ["a"],
                        "output_cols": ["a_padded"],
                        "max_length": 5,
                    }
                ]
            }
        )
        module = get_transform_module(spec, start_level=0)
        sample_data = {"a": np.array([1, 2, 3], dtype=np.int32)}
        _, _, input_dtypes, output_dtypes = _get_sample_shapes_and_dtypes(
            module, sample_data
        )
        assert input_dtypes["a"] == torch.int32
        assert output_dtypes["a_padded"] == torch.int32


class TestCreateNativeTransformSchema:
    """``_create_native_transform_schema``'s dtype/shape merge priority."""

    def test_builds_schema_from_derived_shapes(self) -> None:
        """Derived input/output shapes populate the resulting schema items."""
        spec = TransformSpec(
            raw_transform_specs={
                "transform_specs": [
                    {
                        "transform_name": "Concatenate",
                        "input_cols": ["a"],
                        "output_cols": ["out"],
                    }
                ]
            }
        )
        schema = _create_native_transform_schema(
            transform_spec=spec,
            input_cols=["a"],
            output_cols=["out"],
            derived_input_shapes={"a": [10]},
            derived_output_shapes={"out": [10]},
        )
        assert len(schema.input_schema) == 1
        assert schema.input_schema[0].name == "a"
        assert schema.input_schema[0].shape == [10]
        assert len(schema.output_schema) == 1
        assert schema.output_schema[0].name == "out"
        assert schema.output_schema[0].shape == [10]

    def test_handles_missing_shapes(self) -> None:
        """No derived or overridden shapes leaves schema item shapes unset."""
        spec = TransformSpec(
            raw_transform_specs={
                "transform_specs": [
                    {
                        "transform_name": "Concatenate",
                        "input_cols": ["a"],
                        "output_cols": ["out"],
                    }
                ]
            }
        )
        schema = _create_native_transform_schema(
            transform_spec=spec, input_cols=["a"], output_cols=["out"]
        )
        assert schema.input_schema[0].shape is None
        assert schema.output_schema[0].shape is None

    def test_derived_output_dtype_used_absent_spec_override(self) -> None:
        """A derived output dtype is used when the spec sets no output_dtype."""
        spec = TransformSpec(
            raw_transform_specs={
                "transform_specs": [
                    {
                        "transform_name": "Concatenate",
                        "input_cols": ["a", "b"],
                        "output_cols": ["out"],
                    }
                ]
            }
        )
        schema = _create_native_transform_schema(
            transform_spec=spec,
            input_cols=["a", "b"],
            output_cols=["out"],
            derived_output_dtypes={"out": torch.int32},
        )
        assert schema.output_schema[0].data_type == DataType.INT

    def test_spec_output_dtype_overrides_derived(self) -> None:
        """A spec-declared output_dtype takes priority over a derived one."""
        spec = TransformSpec(
            raw_transform_specs={
                "transform_specs": [
                    {
                        "transform_name": "NumericalStandardTransform",
                        "input_cols": ["a"],
                        "output_cols": ["out"],
                    }
                ]
            }
        )
        schema = _create_native_transform_schema(
            transform_spec=spec,
            input_cols=["a"],
            output_cols=["out"],
            derived_output_dtypes={"out": torch.int64},
        )
        # NumericalStandardTransform's model_validator sets output_dtype to
        # its (float32) dtype field, which must win over the derived int64.
        assert schema.output_schema[0].data_type == DataType.FLOAT

    def test_derived_input_dtype_used_for_raw_inputs(self) -> None:
        """A derived input dtype is used when the spec sets no input_dtype."""
        spec = TransformSpec(
            raw_transform_specs={
                "transform_specs": [
                    {
                        "transform_name": "Concatenate",
                        "input_cols": ["a"],
                        "output_cols": ["out"],
                    }
                ]
            }
        )
        schema = _create_native_transform_schema(
            transform_spec=spec,
            input_cols=["a"],
            output_cols=["out"],
            derived_input_dtypes={"a": torch.float64},
        )
        assert schema.input_schema[0].data_type == DataType.DOUBLE


class TestInputShapeOverrides:
    """Manual ``input_shape`` overrides declared on layer specs."""

    def test_collect_shape_overrides_maps_each_col(self) -> None:
        """Every input column of an override layer gets the same overridden shape."""
        spec = TransformSpec(
            raw_transform_specs={
                "transform_specs": [
                    {
                        "transform_name": "TensorColFillNone",
                        "input_cols": ["delivery_fee"],
                        "output_cols": ["delivery_fee_filled"],
                        "default_value": 0.0,
                        "input_shape": [1],
                    }
                ]
            }
        )
        assert _collect_input_shape_overrides(spec) == {"delivery_fee": [1]}

    def test_collect_shape_overrides_empty_when_unset(self) -> None:
        """A spec with no input_shape overrides collects an empty mapping."""
        spec = TransformSpec(
            raw_transform_specs={
                "transform_specs": [
                    {
                        "transform_name": "Concatenate",
                        "input_cols": ["a"],
                        "output_cols": ["out"],
                    }
                ]
            }
        )
        assert _collect_input_shape_overrides(spec) == {}

    def test_override_backfills_a_shape_the_forward_pass_could_not_derive(
        self,
    ) -> None:
        """An override supplies a shape a null sample value could not derive."""
        spec = TransformSpec(
            raw_transform_specs={
                "transform_specs": [
                    {
                        "transform_name": "TensorColFillNone",
                        "input_cols": ["delivery_fee"],
                        "output_cols": ["delivery_fee_filled"],
                        "default_value": 0.0,
                        "input_shape": [1],
                    }
                ]
            }
        )
        schema = _create_native_transform_schema(
            transform_spec=spec,
            input_cols=["delivery_fee"],
            output_cols=["delivery_fee_filled"],
            derived_input_shapes={},
        )
        item = next(i for i in schema.input_schema if i.name == "delivery_fee")
        assert item.shape == [1]

    def test_override_wins_over_derived_shape(self) -> None:
        """A spec-declared input_shape takes priority over a derived shape."""
        spec = TransformSpec(
            raw_transform_specs={
                "transform_specs": [
                    {
                        "transform_name": "Concatenate",
                        "input_cols": ["a"],
                        "output_cols": ["out"],
                        "input_shape": [4],
                    }
                ]
            }
        )
        schema = _create_native_transform_schema(
            transform_spec=spec,
            input_cols=["a"],
            output_cols=["out"],
            derived_input_shapes={"a": [10]},
        )
        assert schema.input_schema[0].shape == [4]

    def test_misplaced_override_on_intermediate_col_raises(self) -> None:
        """An input_shape on a non-level-0 column must raise, not be dropped."""
        spec = TransformSpec(
            raw_transform_specs={
                "transform_specs": [
                    {
                        "transform_name": "TensorColFillNone",
                        "input_cols": ["delivery_fee"],
                        "output_cols": ["delivery_fee_filled"],
                        "default_value": 0.0,
                    },
                    {
                        "transform_name": "PadOrCrop1D",
                        "input_cols": ["delivery_fee_filled"],
                        "output_cols": ["delivery_fee_padded"],
                        "max_length": 32,
                        "input_shape": [32],
                    },
                ]
            }
        )
        with pytest.raises(ValueError, match="delivery_fee_filled"):
            _create_native_transform_schema(
                transform_spec=spec,
                input_cols=["delivery_fee"],
                output_cols=["delivery_fee_padded"],
                derived_input_shapes={},
            )

    def test_correctly_placed_override_passes_guard(self) -> None:
        """An input_shape on the level-0 input column passes and pins the shape."""
        spec = TransformSpec(
            raw_transform_specs={
                "transform_specs": [
                    {
                        "transform_name": "PadOrCrop1D",
                        "input_cols": ["delivery_fee"],
                        "output_cols": ["delivery_fee_padded"],
                        "max_length": 32,
                        "input_shape": [32],
                    }
                ]
            }
        )
        schema = _create_native_transform_schema(
            transform_spec=spec,
            input_cols=["delivery_fee"],
            output_cols=["delivery_fee_padded"],
            derived_input_shapes={},
        )
        item = next(i for i in schema.input_schema if i.name == "delivery_fee")
        assert item.shape == [32]


class TestDeriveNativeTransformSchema:
    """``derive_native_transform_schema``'s end-to-end public API."""

    def test_derives_schema_with_sample_data(self) -> None:
        """End to end: a sample forward pass derives both shapes and dtypes."""
        spec = TransformSpec(
            raw_transform_specs={
                "transform_specs": [
                    {
                        "transform_name": "Concatenate",
                        "input_cols": ["a", "b"],
                        "output_cols": ["ab"],
                    }
                ]
            }
        )
        module = get_transform_module(spec, start_level=0)
        sample_data = {
            "a": np.array([1.0, 2.0], dtype=np.float64),
            "b": np.array([3.0], dtype=np.float64),
        }

        schema = derive_native_transform_schema(spec, module, sample_data=sample_data)

        assert len(schema.input_schema) == 2
        by_name = {item.name: item for item in schema.input_schema}
        assert by_name["a"].data_type == DataType.DOUBLE
        assert by_name["a"].shape == [2]
        assert by_name["b"].shape == [1]
        assert len(schema.output_schema) == 1
        assert schema.output_schema[0].name == "ab"
        assert schema.output_schema[0].shape == [3]

    def test_derives_schema_without_sample_data(self) -> None:
        """Without sample_data, shapes are left unset and dtypes default to float."""
        spec = TransformSpec(
            raw_transform_specs={
                "transform_specs": [
                    {
                        "transform_name": "Concatenate",
                        "input_cols": ["a"],
                        "output_cols": ["out"],
                    }
                ]
            }
        )
        module = get_transform_module(spec, start_level=0)

        schema = derive_native_transform_schema(spec, module, sample_data=None)

        assert len(schema.input_schema) == 1
        assert schema.input_schema[0].name == "a"
        assert schema.input_schema[0].shape is None
        assert len(schema.output_schema) == 1
        assert schema.output_schema[0].name == "out"

    def test_filters_output_cols_via_columns_to_keep(self) -> None:
        """The spec's columns_to_keep filters which outputs land in the schema."""
        spec = TransformSpec(
            raw_transform_specs={
                "transform_specs": [
                    {
                        "transform_name": "Concatenate",
                        "input_cols": ["a", "b"],
                        "output_cols": ["ab"],
                    }
                ]
            }
        )
        spec.columns_to_keep = ["ab"]
        module = get_transform_module(spec, start_level=0)

        schema = derive_native_transform_schema(spec, module)

        assert [item.name for item in schema.output_schema] == ["ab"]

    def test_columns_to_keep_excludes_unlisted(self) -> None:
        """columns_to_keep excludes an unlisted output from another layer."""
        spec = TransformSpec(
            raw_transform_specs={
                "transform_specs": [
                    {
                        "transform_name": "Concatenate",
                        "input_cols": ["a"],
                        "output_cols": ["out1"],
                    },
                    {
                        "transform_name": "Concatenate",
                        "input_cols": ["b"],
                        "output_cols": ["out2"],
                    },
                ]
            }
        )
        spec.columns_to_keep = ["out1"]
        module = get_transform_module(spec, start_level=0)

        schema = derive_native_transform_schema(spec, module)

        output_names = [item.name for item in schema.output_schema]
        assert output_names == ["out1"]
        assert "out2" not in output_names

    def test_input_shape_pins_max_length_over_a_ragged_sample(self) -> None:
        """input_shape pins PadOrCrop1D's fixed max length, overriding a ragged sample.

        The sampled row has a ragged length (3) that must not leak into the
        packaged model's static input shape; input_shape (32, matching
        PadOrCrop1D's max_length) wins so Triton/ONNX/TRT get a concrete
        dimension. The output shape is derived from the forward pass (which
        PadOrCrop1D always fixes to max_length), so no output override is
        needed.
        """
        spec = TransformSpec(
            raw_transform_specs={
                "transform_specs": [
                    {
                        "transform_name": "PadOrCrop1D",
                        "input_cols": ["delivery_fee"],
                        "output_cols": ["delivery_fee_padded"],
                        "max_length": 32,
                        "input_shape": [32],
                    }
                ]
            }
        )
        module = get_transform_module(spec, start_level=0)

        schema = derive_native_transform_schema(
            spec, module, sample_data={"delivery_fee": [1.0, 2.0, 3.0]}
        )

        in_item = next(i for i in schema.input_schema if i.name == "delivery_fee")
        out_item = next(
            i for i in schema.output_schema if i.name == "delivery_fee_padded"
        )
        assert in_item.shape == [32]
        assert out_item.shape == [32]

    def test_derive_schema_uses_override_for_null_sample(self) -> None:
        """A null sample value cannot derive a shape; the override fills it in."""
        spec = TransformSpec(
            raw_transform_specs={
                "transform_specs": [
                    {
                        "transform_name": "TensorColFillNone",
                        "input_cols": ["delivery_fee"],
                        "output_cols": ["delivery_fee_filled"],
                        "default_value": 0.0,
                        "input_shape": [1],
                    }
                ]
            }
        )
        module = get_transform_module(spec, start_level=0)

        schema = derive_native_transform_schema(
            spec, module, sample_data={"delivery_fee": None}
        )

        item = next(i for i in schema.input_schema if i.name == "delivery_fee")
        assert item.shape == [1]
