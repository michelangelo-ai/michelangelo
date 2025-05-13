from typing import Optional
from numpy import ndarray
from uber.ai.michelangelo.sdk.model_manager.schema import ModelSchema
from uber.ai.michelangelo.sdk.model_manager._private.utils.data_utils.numpy_data import (
    validate_numpy_data_record,
    validate_numpy_data_record_with_model_schema,
)


def validate_output_data(output: dict[str, ndarray]) -> tuple[bool, Optional[Exception]]:
    """
    Validate the output data

    Args:
        output: The output data to validate

    Returns:
        Tuple containing a boolean indicating whether the output data is valid and an exception if the output data is invalid
    """
    if not output:
        return False, ValueError("Output of the model cannot be empty")

    is_valid, err = validate_numpy_data_record(output)

    if not is_valid:
        Error = type(err)
        return False, Error(f"Error validating model output data, {err}")

    return True, None


def validate_output_data_with_model_schema(
    output: dict[str, ndarray], model_schema: ModelSchema, batch_inference: Optional[bool] = False
) -> tuple[bool, Optional[Exception]]:
    """
    Validate the output data with the model schema

    Args:
        output: The output data to validate
        model_schema: The model schema to validate against
        batch_inference: A boolean indicating if batch inference is being used

    Returns:
        Tuple containing a boolean indicating whether the output data is valid and an exception if the output data is invalid
    """
    is_valid, err = validate_numpy_data_record_with_model_schema(output, model_schema.output_schema, batch_inference)

    if not is_valid:
        Error = type(err)
        return False, Error(f"Error validating model output data. {err}")

    return True, None
