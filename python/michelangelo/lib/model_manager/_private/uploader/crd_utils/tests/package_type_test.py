from unittest import TestCase
from michelangelo.lib.model_manager.constants import PackageType
from michelangelo.lib.model_manager._private.uploader.crd_utils import convert_package_type
from uber.gen.code_uber_internal.uberai.michelangelo.api.v2beta1.model_proto import (
    DEPLOYABLE_MODEL_PACKAGE_TYPE_INVALID,
    DEPLOYABLE_MODEL_PACKAGE_TYPE_SPARK_PIPELINE,
    DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON,
    DEPLOYABLE_MODEL_PACKAGE_TYPE_RAW,
    DEPLOYABLE_MODEL_PACKAGE_TYPE_MOBILE,
)


class PackageTypeTest(TestCase):
    def test_convert_package_type(self):
        package_type = convert_package_type(PackageType.SPARK)
        self.assertEqual(package_type, DEPLOYABLE_MODEL_PACKAGE_TYPE_SPARK_PIPELINE)

        package_type = convert_package_type(PackageType.TRITON)
        self.assertEqual(package_type, DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON)

        package_type = convert_package_type(PackageType.RAW)
        self.assertEqual(package_type, DEPLOYABLE_MODEL_PACKAGE_TYPE_RAW)

        package_type = convert_package_type(PackageType.MOBILE)
        self.assertEqual(package_type, DEPLOYABLE_MODEL_PACKAGE_TYPE_MOBILE)

        package_type = convert_package_type("invalid")
        self.assertEqual(package_type, DEPLOYABLE_MODEL_PACKAGE_TYPE_INVALID)
