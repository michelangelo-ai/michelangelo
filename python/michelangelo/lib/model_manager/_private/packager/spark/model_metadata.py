import yaml
from michelangelo.lib.model_manager.schema import (
    ModelSchema,
    DataType,
)
from michelangelo.lib.model_manager.utils.model import SparkModelMetadata


def generate_model_metadata_content(
    model_metadata: SparkModelMetadata,
    model_schema: ModelSchema,
) -> dict:
    """
    Generate the content for each file in the model metadata

    Args:
        model_metadata: The SparkModelMetadata instance.

    Returns:
        The content of the model metadata.
    """
    if model_metadata.column_stats is None:
        model_metadata.column_stats = {
            "mu": {},
            "sigma": {},
            "min": {},
            "max": {},
        }

    if model_metadata.basis_columns_type is None:
        model_metadata.basis_columns_type = {
            "numeric": get_numeric_input_features(model_schema),
            "non-numeric": get_non_numeric_input_features(model_schema),
            "vector": get_vector_input_features(model_schema),
        }

    return {file_name: yaml.dump(content, sort_keys=False) for content, file_name in model_metadata.list_files() if content is not None}


# TODO: think about moving these to public method in ModelSchema
def get_numeric_input_features(model_schema: ModelSchema) -> list:
    """
    Get the numeric input features from the model schema

    Args:
        model_schema: The model schema.

    Returns:
        The list of numeric input features.
    """
    return [item.name for item in model_schema.input_schema if item.data_type == DataType.NUMERIC]


def get_non_numeric_input_features(model_schema: ModelSchema) -> list:
    """
    Get the non-numeric input features from the model schema

    Args:
        model_schema: The model schema.

    Returns:
        The list of non-numeric input features.
    """
    return [
        item.name
        for item in model_schema.input_schema
        if item.data_type
        not in {
            DataType.NUMERIC,
            DataType.VECTOR,
        }
    ]


def get_vector_input_features(model_schema: ModelSchema) -> list:
    """
    Get the vector input features from the model schema

    Args:
        model_schema: The model schema.

    Returns:
        The list of vector input features.
    """
    return [item.name for item in model_schema.input_schema if item.data_type == DataType.VECTOR]
