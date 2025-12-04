"""Sample data validation utilities."""

from typing import Optional

from numpy import ndarray

from michelangelo.lib.model_manager._private.utils.data_utils.numpy_data import (
    validate_numpy_data,
    validate_numpy_data_with_model_schema,
)
from michelangelo.lib.model_manager.schema import ModelSchema


def validate_sample_data(
    sample_data: list[dict[str, ndarray]],
) -> tuple[bool, Optional[Exception]]:
    """Validate the sample data.

    Args:
        sample_data: The sample data to validate

    Returns:
        Tuple containing a boolean indicating whether the sample data is valid and
        an exception if the sample data is invalid
    """
    if not sample_data:
        return False, ValueError("Sample data is required")

    is_valid, err = validate_numpy_data(sample_data)

    if not is_valid:
        error_type = type(err)
        return False, error_type(f"Error validating sample data, {err}")

    return True, None


def validate_sample_data_with_model_schema(
    sample_data: list[dict[str, ndarray]],
    model_schema: ModelSchema,
    batch_inference: Optional[bool] = False,
) -> tuple[bool, Optional[Exception]]:
    """Validate the sample data with the model schema.

    Args:
        sample_data: The sample data to validate
        model_schema: The model schema to validate against
        batch_inference: Optional flag for batch inference validation

    Returns:
        Tuple containing a boolean indicating whether the sample data is valid and
        an exception if the sample data is invalid
    """
    is_valid, err = validate_numpy_data_with_model_schema(
        sample_data, model_schema.input_schema, batch_inference
    )

    if not is_valid:
        error_type = type(err)
        return False, error_type(
            f"Error validating sample data with model input schema. {err}"
        )

    return True, None
