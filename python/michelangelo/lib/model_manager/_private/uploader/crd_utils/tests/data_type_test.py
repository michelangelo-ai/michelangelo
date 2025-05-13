from unittest import TestCase
from michelangelo.lib.model_manager.schema import DataType as SchemaDataType
from michelangelo.lib.model_manager._private.uploader.crd_utils import convert_data_type
from uber.gen.code_uber_internal.uberai.michelangelo.api.v2beta1.schema_proto import DataType


class DataTypeTest(TestCase):
    def test_convert_data_type(self):
        for data_type in SchemaDataType:
            proto_data_type = convert_data_type(data_type)
            expected_proto_data_type = getattr(DataType, f"DATA_TYPE_{data_type.name}", DataType.DATA_TYPE_INVALID)
            self.assertEqual(proto_data_type, expected_proto_data_type)
