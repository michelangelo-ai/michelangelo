from uber.ai.michelangelo.sdk.model_manager.schema import DataType


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
