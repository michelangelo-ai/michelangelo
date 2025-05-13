from uber.ai.michelangelo.sdk.model_manager.schema import DataType as SchemaDataType
from uber.gen.code_uber_internal.uberai.michelangelo.api.v2beta1.schema_proto import DataType


def convert_data_type(data_type: SchemaDataType) -> DataType:
    """
    Convert the data type from the model schema to the proto data type in Unified API.

    Args:
        data_type: Data type from the model schema.

    Returns:
        Proto data type in Unified API.
    """
    data_type_name = f"DATA_TYPE_{data_type.name}"
    proto_data_type = getattr(DataType, data_type_name, DataType.DATA_TYPE_INVALID)
    return proto_data_type
