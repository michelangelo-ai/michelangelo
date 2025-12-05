"""Data types for model schema features."""

from enum import Enum


class DataType(Enum):
    """Enum for data types used in model schemas.

    This enum defines the supported data types for model features (inputs,
    outputs, and palette features). The values are used to specify the data
    type of each feature in a ModelSchemaItem and correspond to the data types
    defined in the Michelangelo schema protobuf specification.

    The enum values match those in the protobuf definition at:
    https://github.com/michelangelo-ai/michelangelo/blob/main/proto/api/v2/schema.proto

    Attributes:
        INVALID: Invalid or unset data type. Should not be used in valid
            schemas.
        UNKNOWN: Unknown data type. May be used as a placeholder during schema
            inference.
        BOOLEAN: Boolean values (True/False). Maps to bool in Python and
            BOOL in Triton.
        STRING: String/text data. Maps to str in Python and BYTES in Triton.
        BYTE: 8-bit signed integer. Maps to int8 in NumPy and INT8 in Triton.
        CHAR: Character data (8-bit unsigned integer). Maps to uint8 in NumPy.
        SHORT: 16-bit signed integer. Maps to int16 in NumPy and INT16 in
            Triton.
        INT: 32-bit signed integer. Maps to int32 in NumPy and INT32 in Triton.
        LONG: 64-bit signed integer. Maps to int64 in NumPy and INT64 in
            Triton.
        FLOAT: 32-bit floating point. Maps to float32 in NumPy and FP32 in
            Triton.
        DOUBLE: 64-bit floating point. Maps to float64 in NumPy and FP64 in
            Triton.
    """

    INVALID = 0
    UNKNOWN = 1
    BOOLEAN = 4
    STRING = 7
    BYTE = 15
    CHAR = 16
    SHORT = 17
    INT = 18
    LONG = 19
    FLOAT = 20
    DOUBLE = 21
