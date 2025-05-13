from unittest import TestCase
from michelangelo._internal.utils.reflection_utils import get_module_attr

# enable metabuild to build bazel dependencies
import michelangelo._internal.utils.reflection_utils.tests.fixtures.simple_module  # noqa:F401


class ModuleAttrTest(TestCase):
    def test_get_module_attr(self):
        module_attr = get_module_attr("michelangelo._internal.utils.reflection_utils.tests.fixtures.simple_module.module_attr")

        self.assertIsNotNone(module_attr)
        self.assertEqual(module_attr.__name__, "module_attr")

    def test_get_module_attr_with_invalid_module(self):
        with self.assertRaises(ValueError):
            get_module_attr("module_attr_invalid")
