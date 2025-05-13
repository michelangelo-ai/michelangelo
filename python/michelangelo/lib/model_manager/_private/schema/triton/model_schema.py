from michelangelo.lib.model_manager.schema import ModelSchema
from michelangelo.lib.model_manager._private.schema.triton.schema_to_dict import convert_schema_to_dict


def convert_model_schema(
    model_schema: ModelSchema,
) -> tuple[dict, dict]:
    """
    Convert a ModelSchema to input and output schema dictionaries for Triton models.

    Args:
        model_schema (ModelSchema): Model schema to convert.

    Returns:
        tuple[dict, dict]: Tuple containing the input and output schema dictionaries.
    """
    input_schema = convert_schema_to_dict(model_schema.input_schema + model_schema.feature_store_features_schema)
    output_schema = convert_schema_to_dict(model_schema.output_schema)
    return input_schema, output_schema
