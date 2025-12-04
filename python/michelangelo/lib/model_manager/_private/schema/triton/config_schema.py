"""Config schema conversion module."""

from michelangelo.lib.model_manager._private.schema.triton.data_type import (
    convert_data_type,
)
from michelangelo.lib.model_manager.schema import ModelSchema, ModelSchemaItem


def convert_model_schema(
    model_schema: ModelSchema,
) -> tuple[dict, dict]:
    """Convert a ModelSchema to input and output schema dictionaries for Triton models.

    Args:
        model_schema (ModelSchema): Model schema to convert.

    Returns:
        tuple[dict, dict]: Tuple containing the input and output schema dictionaries.
    """
    input_schema = convert_schema_to_dict(
        model_schema.input_schema + model_schema.feature_store_features_schema
    )
    output_schema = convert_schema_to_dict(model_schema.output_schema)
    return input_schema, output_schema


def convert_schema_to_dict(schema: list[ModelSchemaItem]):
    """Convert a list of ModelSchemaItems to a dictionary.

    Args:
        schema (list[ModelSchemaItem]): List of ModelSchemaItems to convert.

    Returns:
        dict: Dictionary representation of the schema.
    """
    return {
        item.name: {
            "data_type": convert_data_type(item.data_type),
            "shape": convert_shape(item.shape),
            "optional": item.optional,
        }
        for item in schema
    }


def convert_shape(shape: list[int]) -> str:
    """Convert a shape to a string.

    Args:
        shape (list[int]): Shape to convert.

    Returns:
        str: String representation of the shape.
    """
    return (
        f"[ {', '.join(str(dim) for dim in shape)} ]"
        if shape and len(shape) > 0
        else "[ -1 ]"
    )
