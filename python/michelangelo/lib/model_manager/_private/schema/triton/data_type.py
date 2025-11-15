from michelangelo.lib.model_manager.schema import DataType


# Mapping from ModelSchema data types to Triton data types.
DATA_TYPE_MAPPING: "dict[DataType, str]" = {
    DataType.BOOLEAN: "BOOL",
    DataType.BYTE: "UINT8",
    DataType.CHAR: "INT8",
    DataType.SHORT: "INT16",
    DataType.INT: "INT32",
    DataType.LONG: "INT64",
    DataType.FLOAT: "FP32",
    DataType.DOUBLE: "FP64",
    DataType.STRING: "STRING",
    DataType.NUMERIC: "FP64",
}


def convert_data_type(data_type: DataType) -> str:
    """
    Convert a ModelSchema data type to a Triton data type.

    Args:
        data_type (DataType): Data type to convert.

    Returns:
        str: Triton data type.
    """
    return DATA_TYPE_MAPPING.get(data_type)
