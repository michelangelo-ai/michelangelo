"""Validation for custom Triton packages."""

import os
import tempfile
from typing import Union, Optional
from numpy import ndarray
from michelangelo.lib.model_manager.schema import ModelSchema
from michelangelo.lib.model_manager.serde.model import load_raw_model
from michelangelo.lib.model_manager._private.utils.data_utils import (
    validate_output_data,
    validate_output_data_with_model_schema,
)
from michelangelo._internal.utils.reflection_utils import get_module_attr


def validate_model_files(model_path: str):
    """Validate that the model does not contain reserved filenames that conflict with Triton.

    Args:
        model_path: The path to the model directory

    Raises:
        ValueError: If the model contains a reserved file '__init__.py' in the model assets folder
    """
    model_py_path = os.path.join(model_path, "__init__.py")
    if os.path.exists(model_py_path):
        raise ValueError(
            "Custom model contains the file'__init__.py' in the model assets folder. "
            "This file conflicts with Triton's reserved model.py file and will break deployments. "
            "Please remove your __init__.py file from model_path."
        )


def validate_raw_model_package(
    model_path: str,
    sample_data: Union[list[dict[str, ndarray]], dict[str, ndarray]],
    model_schema: ModelSchema,
    batch_inference: Optional[bool] = False,
):
    """Validate the raw model package.

    Args:
        model_path: The path to the model package
        sample_data: The sample data of the model
        model_schema: The model schema of the model
        batch_inference: Optional flag for batch inference validation
    """
    try:
        validate_raw_model_package_internal(model_path, sample_data, model_schema, batch_inference)
    except Exception as e:
        raise RuntimeError(f"Error when validating the raw model package. {e}") from e


def validate_raw_model_package_internal(
    model_path: str,
    sample_data: Union[list[dict[str, ndarray]], dict[str, ndarray]],
    model_schema: ModelSchema,
    batch_inference: Optional[bool] = False,
):
    """Validate the raw model package.

    This is an internal function. Use validate_raw_model_package instead.

    Args:
        model_path: The path to the model package
        sample_data: The sample data of the model
        model_schema: The model schema of the model
        batch_inference: Optional flag for batch inference validation
    """
    model = load_raw_model(model_path)

    with open(os.path.join(model_path, "defs", "model_class.txt")) as f:
        model_class = f.read().strip()
        ModelClass = get_module_attr(model_class)

    if not isinstance(model, ModelClass):
        raise TypeError(f"The loaded model is not an instance of {ModelClass}")

    # test predict
    if sample_data:
        data = sample_data[0] if isinstance(sample_data, list) else sample_data

        try:
            output = model.predict(data)
        except Exception as e:
            raise RuntimeError(f"Error when test prediction with the model. Error: {e}") from e

        is_valid, err = validate_output_data(output)

        if not is_valid:
            raise err

        is_valid, err = validate_output_data_with_model_schema(output, model_schema, batch_inference)

        if not is_valid:
            raise err

    # test save and load
    with tempfile.TemporaryDirectory() as temp_dir:
        try:
            model.save(temp_dir)
        except Exception as e:
            raise RuntimeError(f"Error when test saving the model. Error: {e}") from e

        try:
            ModelClass.load(temp_dir)
        except Exception as e:
            raise RuntimeError(f"Error when test reloading the saved model, please double check the save function. Error: {e}") from e
