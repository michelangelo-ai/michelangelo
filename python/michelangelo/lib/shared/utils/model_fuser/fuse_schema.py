"""Schema fusion for native-transform + predictor models.

Used by the model fuser itself (to compute the fused input schema for ONNX/
TorchScript/Python export) and by the custom and PyTorch/Lightning tabular
assembler paths, when a model is preceded by a native-transform stage: the
servable package exposes a single fused input/output schema rather than two
independent ones.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

from michelangelo.lib.model_manager.schema.model_schema import ModelSchema

if TYPE_CHECKING:
    from michelangelo.lib.model_manager.schema.model_schema_item import ModelSchemaItem


def fuse_input_schema(
    tx_model_schema: ModelSchema | None,
    model_schema: ModelSchema | None,
) -> list[ModelSchemaItem]:
    """Build the fused input schema for a native-transform + predictor model.

    The fused input is the union of the transform's and the predictor's input
    schemas, minus any predictor inputs that are actually produced by the
    transform's outputs (those are computed internally by the transform and
    are not user-facing inputs to the fused model).

    Args:
        tx_model_schema: Schema of the native-transform model, or ``None`` if
            there is no transform stage.
        model_schema: Schema of the predictor model, or ``None``.

    Returns:
        A list of ``ModelSchemaItem`` in transform-then-predictor order.
        Duplicate names keep the first (transform) occurrence.
    """
    tx_output_names = (
        {item.name for item in tx_model_schema.output_schema}
        if tx_model_schema
        else set()
    )
    seen: set[str] = set()
    input_items: list[ModelSchemaItem] = []
    for schema in (tx_model_schema, model_schema):
        if not schema or not schema.input_schema:
            continue
        for item in schema.input_schema:
            if item.name in tx_output_names or item.name in seen:
                continue
            seen.add(item.name)
            input_items.append(item)
    return input_items


def fuse_model_schema(
    tx_model_schema: ModelSchema | None,
    model_schema: ModelSchema | None,
) -> ModelSchema:
    """Build the schema for a fused native-transform + predictor model.

    The input schema is :func:`fuse_input_schema`. The output schema is the
    predictor's output schema unchanged — the transform stage has no
    externally visible outputs once fused.

    Args:
        tx_model_schema: Schema of the native-transform model, or ``None``.
        model_schema: Schema of the predictor model, or ``None``.

    Returns:
        A new ``ModelSchema`` combining both inputs, with the predictor's
        output schema.
    """
    input_items = fuse_input_schema(tx_model_schema, model_schema)
    output_schema = (
        list(model_schema.output_schema)
        if model_schema and model_schema.output_schema
        else []
    )
    return ModelSchema(input_schema=input_items, output_schema=output_schema)
