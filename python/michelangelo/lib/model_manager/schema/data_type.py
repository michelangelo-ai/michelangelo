from enum import Enum


class DataType(Enum):
    """Enum for data types.

    The enum values are used to represent the data types of each feature in the
    model schema. The enum values matches the ones in
    https://github.com/michelangelo-ai/michelangelo/blob/main/proto/api/v2/schema.proto.
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
