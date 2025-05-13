from __future__ import annotations
from uber.ai.michelangelo.sdk.model_manager.schema import DataType


DTYPE_MAPPING: dict[str, DataType] = {
    """
    Mapping from Spark DataFrame dtypes to ModelSchema data types.
    """
    "unknown": DataType.UNKNOWN,
    "bigint": DataType.NUMERIC,
    "boolean": DataType.BOOLEAN,
    "date": DataType.DATE,
    "double": DataType.NUMERIC,
    "float": DataType.NUMERIC,
    "long": DataType.NUMERIC,
    "int": DataType.NUMERIC,
    "null": DataType.NULL,
    "smallint": DataType.NUMERIC,
    "string": DataType.STRING,
    "timestamp": DataType.TIMESTAMP,
    "tinyint": DataType.NUMERIC,
    "vector": DataType.VECTOR,
}
