from uber.ai.michelangelo.sdk.model_manager.schema import ModelSchemaItem
from uber.ai.michelangelo.sdk.model_manager._private.schema.triton.data_type import convert_data_type


def convert_schema_to_dict(schema: list[ModelSchemaItem]):
    """
    Convert a list of ModelSchemaItems to a dictionary.

    Args:
        schema (list[ModelSchemaItem]): List of ModelSchemaItems to convert.

    Returns:
        dict: Dictionary representation of the schema.
    """
    return {
        item.name: {
            "data_type": convert_data_type(item.data_type),
            "shape": convert_shape(item.shape),
        }
        for item in schema
    }


def convert_shape(shape: list[int]) -> str:
    """
    Convert a shape to a string.

    Args:
        shape (list[int]): Shape to convert.

    Returns:
        str: String representation of the shape.
    """
    return f"[ {', '.join(str(dim) for dim in shape)} ]" if shape and len(shape) > 0 else "[ -1 ]"
