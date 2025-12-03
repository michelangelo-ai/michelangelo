from michelangelo.lib.model_manager._private.schema.triton.data_type import (
    DATA_TYPE_MAPPING,
)
from michelangelo.lib.model_manager.schema import (
    ModelSchema,
    ModelSchemaItem,
)


def validate_model_schema(model_schema: ModelSchema) -> tuple[bool, Exception]:
    """Validate the model schema for Triton models.

    Args:
        model_schema (ModelSchema): Model schema to validate.

    Returns:
        tuple[bool, Exception]: Tuple containing a boolean indicating whether the
            schema is valid and an exception if the schema is invalid.
    """
    for schema in [
        model_schema.input_schema,
        model_schema.feature_store_features_schema,
    ]:
        if schema:
            for item in schema:
                is_valid, error = validate_model_schema_item(item)
                if not is_valid:
                    return False, error

    for item in model_schema.output_schema:
        is_valid, error = validate_output_schema_item(item)
        if not is_valid:
            return False, error

    return True, None


def validate_model_schema_item(item: ModelSchemaItem) -> tuple[bool, Exception]:
    """Validate a model schema item for Triton models.

    Args:
        item (ModelSchemaItem): Schema item to validate.

    Returns:
        tuple[bool, Exception]: Tuple containing a boolean indicating whether the
            schema item is valid and an exception if the schema item is invalid.
    """
    if item.data_type not in DATA_TYPE_MAPPING:
        supported_types = [t.name for t in DATA_TYPE_MAPPING]
        return False, ValueError(
            f"Invalid data type: {item.data_type}. Supported data types for "
            f"Triton models: {supported_types}"
        )

    if not item.shape or len(item.shape) == 0:
        return False, ValueError(f"Shape must be provided for item: {item}")

    return True, None


def validate_output_schema_item(item: ModelSchemaItem) -> tuple[bool, Exception]:
    """Validate an output schema item for Triton models.

    Args:
        item (ModelSchemaItem): Schema item to validate.

    Returns:
        tuple[bool, Exception]: Tuple containing a boolean indicating whether the
            schema item is valid and an exception if the schema item is invalid.
    """
    is_valid, error = validate_model_schema_item(item)
    if not is_valid:
        return False, error

    if item.optional:
        return False, ValueError(
            f"Optional is not allowed for output schema. Please remove the "
            f"optional flag from the schema item: {item}"
        )

    return True, None
