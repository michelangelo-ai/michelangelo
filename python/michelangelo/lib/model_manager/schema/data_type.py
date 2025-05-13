from enum import Enum


class DataType(Enum):
    """
    Enum for data types.
    The enum values are used to represent the data types of each feature in the model schema.
    The enum values matches the ones in
    https://sg.uberinternal.com/code.uber.internal/uber-code/go-code/-/blob/idl/code.uber.internal/uberai/michelangelo/api/v2beta1/schema.proto
    """

    INVALID = 0
    UNKNOWN = 1
    NUMERIC = 2
    ARRAY = 3
    BOOLEAN = 4
    DATE = 5
    HIVE_STRING = 6
    STRING = 7
    TIMESTAMP = 8
    CALENDAR_INTERVAL = 9
    MAP = 10
    NULL = 11
    STRUCT = 12
    OBJECT = 13
    VECTOR = 14
    BYTE = 15
    CHAR = 16
    SHORT = 17
    INT = 18
    LONG = 19
    FLOAT = 20
    DOUBLE = 21
