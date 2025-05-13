from unittest import TestCase
from uber.ai.michelangelo.sdk.model_manager._private.schema.spark import DTYPE_MAPPING
from uber.ai.michelangelo.sdk.model_manager.schema.data_type import DataType


class DTypeMappingTest(TestCase):
    def test_dtype_mapping(self):
        # just to test that there is no malice in the DTYPE_MAPPING
        for dtype, data_type in DTYPE_MAPPING.items():
            self.assertIsInstance(dtype, str)
            self.assertIsInstance(data_type, DataType)
