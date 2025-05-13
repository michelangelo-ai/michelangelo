from uber.ai.michelangelo.sdk.model_manager.schema import DataType
from uber.ai.michelangelo.sdk.model_manager._private.schema.triton.data_type_mapping import DATA_TYPE_MAPPING


def convert_data_type(data_type: DataType) -> str:
    """
    Convert a ModelSchema data type to a Triton data type.

    Args:
        data_type (DataType): Data type to convert.

    Returns:
        str: Triton data type.
    """
    return DATA_TYPE_MAPPING.get(data_type)
