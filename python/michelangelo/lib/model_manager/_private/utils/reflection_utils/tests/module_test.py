from unittest import TestCase
from michelangelo.lib.model_manager._private.utils.reflection_utils import get_module

# enable metabuild to build bazel dependencies
import michelangelo.lib.model_manager._private.utils.reflection_utils.tests.fixtures.simple_module  # noqa:F401


class ModuleTest(TestCase):
    def test_get_module_attr(self):
        module = get_module("michelangelo.lib.model_manager._private.utils.reflection_utils.tests.fixtures.simple_module.module_attr")

        self.assertIsNotNone(module)
        self.assertEqual(
            module.__name__,
            "michelangelo.lib.model_manager._private.utils.reflection_utils.tests.fixtures.simple_module",
        )
