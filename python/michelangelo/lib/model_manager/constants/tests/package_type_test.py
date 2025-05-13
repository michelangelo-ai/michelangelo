from unittest import TestCase
from michelangelo.lib.model_manager.constants import PackageType


class PackageTypeTest(TestCase):
    def test_package_type(self):
        for attr_name, attr_value in vars(PackageType).items():
            if not callable(attr_value) and not attr_name.startswith("__"):
                self.assertTrue(attr_name, attr_value.upper())
