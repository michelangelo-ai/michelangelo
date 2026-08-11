"""Bridges a fitted ``TransformSpec`` DAG to a ``ModelSchema``.

Once a :class:`~michelangelo.lib.native_transform.torch.transform_spec.TransformSpec`
has been fitted and materialized into a
:class:`~michelangelo.lib.native_transform.torch.base_transform_module.TorchTransformModule`,
:func:`derive_native_transform_schema` describes the module's input/output
contract as a :class:`~michelangelo.lib.model_manager.schema.ModelSchema` —
the shape/dtype metadata Triton config generation and served-model validation
consume.

Shapes and dtypes are resolved from two sources, in priority order:

* A forward pass over ``sample_data`` (when given), which runs the module and
  reads the resulting tensors' shapes and dtypes directly.
* The spec's own declarations: a level-0 layer's ``input_dtype`` (populated
  from the dataset's Arrow schema), a layer's ``output_dtype`` (user-set), and
  a layer's ``input_shape`` override (needed for columns a forward pass alone
  cannot pin, such as a ragged array's fixed max length or an all-null sample
  column).

Spec declarations win over the forward pass wherever both are present, since
they encode an explicit contract rather than an inference from one sample row.
"""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING, Any

import torch

from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem

if TYPE_CHECKING:
    from michelangelo.lib.native_transform.torch.base_transform_module import (
        TorchTransformModule,
    )
    from michelangelo.lib.native_transform.torch.transform_spec import TransformSpec

__all__ = ["derive_native_transform_schema"]

_logger = logging.getLogger(__name__)

_TORCH_DTYPE_TO_DATA_TYPE: dict[torch.dtype, DataType] = {
    torch.float32: DataType.FLOAT,
    torch.float64: DataType.DOUBLE,
    torch.int32: DataType.INT,
    torch.int64: DataType.LONG,
    torch.int16: DataType.SHORT,
    torch.int8: DataType.BYTE,
    torch.bool: DataType.BOOLEAN,
}


def _to_batched_tensor(
    value: Any, dtype_hint: torch.dtype | None = None
) -> tuple[torch.Tensor, list[int] | None]:
    """Convert a sample value into a batched tensor for shape/dtype inference.

    Args:
        value: A single column's sample value (scalar, list, numpy array, or
            tensor).
        dtype_hint: Dtype to fall back on when ``value`` is ``None`` or is
            not convertible to a tensor. Typically the column's dtype from
            the dataset's Arrow schema.

    Returns:
        A tuple of ``(batched_tensor, shape)``, where ``shape`` excludes the
        batch dimension. Returns a zero-filled fallback tensor and ``None``
        shape when ``value`` is ``None`` or fails conversion.
    """
    fallback = torch.zeros([1, 1], dtype=dtype_hint or torch.float32)
    if value is None:
        return fallback, None
    try:
        tensor = torch.as_tensor(value)
    except (TypeError, ValueError):
        return fallback, None
    if tensor.dim() == 0:
        tensor = tensor.unsqueeze(0)
    tensor = tensor.unsqueeze(0)  # Add the batch dimension.
    return tensor, list(tensor.shape[1:])


def _build_schema_items(
    cols: list[str],
    dtype_map: dict[str, torch.dtype],
    shape_map: dict[str, list[int]],
) -> list[ModelSchemaItem]:
    """Build a ``ModelSchemaItem`` per column from resolved dtypes and shapes.

    Args:
        cols: Column names to build items for.
        dtype_map: Column -> resolved torch dtype. A column missing here
            defaults to ``DataType.FLOAT``.
        shape_map: Column -> resolved shape. A column missing here gets an
            unset (``None``) shape.

    Returns:
        One ``ModelSchemaItem`` per column, in ``cols`` order.
    """
    return [
        ModelSchemaItem(
            name=col,
            data_type=_TORCH_DTYPE_TO_DATA_TYPE.get(
                dtype_map.get(col, torch.float32), DataType.FLOAT
            ),
            shape=shape_map.get(col),
        )
        for col in cols
    ]


def _filter_output_cols(
    output_cols: list[str], columns_to_keep: list[str] | None
) -> list[str]:
    """Restrict ``output_cols`` to ``columns_to_keep``, preserving order.

    Args:
        output_cols: The module's full set of output columns.
        columns_to_keep: The columns to keep, or ``None`` to keep all.

    Returns:
        ``output_cols`` filtered to ``columns_to_keep`` (or unchanged, if
        ``columns_to_keep`` is ``None``).
    """
    if columns_to_keep is None:
        return output_cols
    keep = set(columns_to_keep)
    return [col for col in output_cols if col in keep]


def _collect_input_shape_overrides(
    transform_spec: TransformSpec,
) -> dict[str, list[int]]:
    """Collect manual ``input_shape`` overrides declared on the spec's layers.

    These pin the per-sample shape (batch dimension excluded) for columns a
    forward pass cannot reliably derive on its own — for example a ragged
    array whose sampled length varies row to row, where ``input_shape``
    instead names the fixed max length used to package the served model.

    Args:
        transform_spec: The fitted spec DAG to scan.

    Returns:
        Mapping from input column name to its overridden shape.
    """
    overrides: dict[str, list[int]] = {}
    for layer_spec in transform_spec.transform_specs.values():
        if layer_spec.input_shape is None:
            continue
        for col in layer_spec.input_cols:
            overrides[col] = list(layer_spec.input_shape)
    return overrides


def _validate_input_shape_overrides(
    override_shapes: dict[str, list[int]], input_cols: list[str]
) -> None:
    """Reject ``input_shape`` overrides declared on non-input columns.

    An override on an intermediate column is silently unused (only the
    module's raw ``input_cols`` are considered when building the schema), so
    it is rejected here rather than left to fail silently.

    Args:
        override_shapes: Column -> overridden shape, from
            :func:`_collect_input_shape_overrides`.
        input_cols: The module's raw input columns.

    Raises:
        ValueError: If ``override_shapes`` names a column not in
            ``input_cols``.
    """
    misplaced = sorted(set(override_shapes) - set(input_cols))
    if misplaced:
        raise ValueError(
            f"input_shape set on non-input column(s) {misplaced}: move it to "
            "the layer spec that reads the raw input column."
        )


def _get_sample_shapes_and_dtypes(
    transform_module: TorchTransformModule,
    sample_data: dict[str, Any],
    input_dtype_map: dict[str, torch.dtype] | None = None,
) -> tuple[
    dict[str, list[int]],
    dict[str, list[int]],
    dict[str, torch.dtype],
    dict[str, torch.dtype],
]:
    """Derive input/output shapes and dtypes by running one sample through the module.

    Args:
        transform_module: The module to run in inference mode.
        sample_data: A single sample row, keyed by column name.
        input_dtype_map: Column -> dtype from the dataset's Arrow schema,
            used as the fallback dtype for columns whose sample value is
            ``None``.

    Returns:
        A tuple of ``(input_shapes, output_shapes, input_dtypes,
        output_dtypes)``. Shapes exclude the batch dimension. Output shapes
        and dtypes are empty if the forward pass raises.
    """
    input_dtype_map = input_dtype_map or {}
    input_shapes: dict[str, list[int]] = {}
    input_dtypes: dict[str, torch.dtype] = {}
    input_tensors: dict[str, torch.Tensor] = {}

    for col in transform_module.input_cols:
        tensor, shape = _to_batched_tensor(
            sample_data.get(col), dtype_hint=input_dtype_map.get(col)
        )
        input_tensors[col] = tensor
        input_dtypes[col] = tensor.dtype
        if shape is not None:
            input_shapes[col] = shape

    output_shapes: dict[str, list[int]] = {}
    output_dtypes: dict[str, torch.dtype] = {}
    try:
        transform_module.eval()
        with torch.no_grad():
            output_tensors = transform_module(input_tensors)
        output_shapes = {
            col: list(tensor.shape[1:]) for col, tensor in output_tensors.items()
        }
        output_dtypes = {col: tensor.dtype for col, tensor in output_tensors.items()}
    except Exception as e:
        _logger.warning(f"Failed to derive native transform output shapes/dtypes: {e}")

    return input_shapes, output_shapes, input_dtypes, output_dtypes


def _create_native_transform_schema(
    transform_spec: TransformSpec,
    input_cols: list[str],
    output_cols: list[str],
    derived_input_shapes: dict[str, list[int]] | None = None,
    derived_output_shapes: dict[str, list[int]] | None = None,
    derived_input_dtypes: dict[str, torch.dtype] | None = None,
    derived_output_dtypes: dict[str, torch.dtype] | None = None,
) -> ModelSchema:
    """Assemble a ``ModelSchema`` from a spec plus optionally-derived shapes/dtypes.

    Resolution priority (highest first):

    * Input dtype: spec's level-0 ``input_dtype`` > derived (forward pass) >
      ``DataType.FLOAT`` fallback.
    * Output dtype: spec's ``output_dtype`` > derived (forward pass) >
      ``DataType.FLOAT`` fallback.
    * Input shape: spec's ``input_shape`` override > derived (forward pass) >
      unset.
    * Output shape: derived (forward pass) > unset. Output shapes are never
      taken from the spec directly.

    Args:
        transform_spec: The fitted spec DAG providing dtype/shape overrides.
        input_cols: The module's input column names.
        output_cols: The module's (possibly filtered) output column names.
        derived_input_shapes: Input shapes from a forward pass, if run.
        derived_output_shapes: Output shapes from a forward pass, if run.
        derived_input_dtypes: Input dtypes from a forward pass, if run.
        derived_output_dtypes: Output dtypes from a forward pass, if run.

    Returns:
        The assembled ``ModelSchema``.

    Raises:
        ValueError: If the spec declares an ``input_shape`` override on a
            column that is not in ``input_cols`` (see
            :func:`_validate_input_shape_overrides`).
    """
    derived_input_shapes = derived_input_shapes or {}
    derived_output_shapes = derived_output_shapes or {}

    override_input_shapes = _collect_input_shape_overrides(transform_spec)
    _validate_input_shape_overrides(override_input_shapes, input_cols)
    input_shape_map = {**derived_input_shapes, **override_input_shapes}
    output_shape_map = derived_output_shapes

    spec_input_dtypes = transform_spec.get_input_dtype_map(target_transform_level=0)
    spec_output_dtypes: dict[str, torch.dtype] = {}
    for layer_spec in transform_spec.transform_specs.values():
        if layer_spec.output_dtype is None:
            continue
        for col in layer_spec.output_cols:
            spec_output_dtypes[col] = layer_spec.output_dtype

    input_dtype_map = {**(derived_input_dtypes or {}), **spec_input_dtypes}
    output_dtype_map = {**(derived_output_dtypes or {}), **spec_output_dtypes}

    return ModelSchema(
        input_schema=_build_schema_items(input_cols, input_dtype_map, input_shape_map),
        output_schema=_build_schema_items(
            output_cols, output_dtype_map, output_shape_map
        ),
    )


def derive_native_transform_schema(
    transform_spec: TransformSpec,
    transform_module: TorchTransformModule,
    sample_data: dict[str, Any] | None = None,
) -> ModelSchema:
    """Derive the ``ModelSchema`` for a materialized native transform module.

    This is the module's public entry point: it filters output columns by
    the spec's ``columns_to_keep``, optionally runs a sample row through the
    module to infer shapes and dtypes, and assembles the result into a
    ``ModelSchema``.

    Args:
        transform_spec: The fitted spec DAG ``transform_module`` was
            materialized from.
        transform_module: The materialized module to derive a schema for.
        sample_data: An optional sample row, keyed by column name. When
            given, a forward pass over it infers shapes and dtypes; when
            omitted, only the spec's own declarations are used.

    Returns:
        The derived ``ModelSchema``.
    """
    input_cols = transform_module.input_cols
    output_cols = _filter_output_cols(
        transform_module.output_cols, transform_spec.columns_to_keep
    )

    input_shapes: dict[str, list[int]] = {}
    output_shapes: dict[str, list[int]] = {}
    input_dtypes: dict[str, torch.dtype] = {}
    output_dtypes: dict[str, torch.dtype] = {}

    if sample_data is not None:
        input_shapes, output_shapes, input_dtypes, output_dtypes = (
            _get_sample_shapes_and_dtypes(
                transform_module,
                sample_data,
                # Only level-0 dtypes are needed: the forward pass computes
                # every intermediate dtype itself.
                input_dtype_map=transform_spec.get_input_dtype_map(
                    target_transform_level=0
                ),
            )
        )

    _logger.info(
        f"Derived native transform schema with {len(input_cols)} input(s) and "
        f"{len(output_cols)} output(s)"
    )

    return _create_native_transform_schema(
        transform_spec=transform_spec,
        input_cols=input_cols,
        output_cols=output_cols,
        derived_input_shapes=input_shapes,
        derived_output_shapes=output_shapes,
        derived_input_dtypes=input_dtypes,
        derived_output_dtypes=output_dtypes,
    )
