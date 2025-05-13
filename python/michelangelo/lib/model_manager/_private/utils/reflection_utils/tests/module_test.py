from unittest import TestCase
from uber.ai.michelangelo.sdk.model_manager._private.utils.reflection_utils import get_module

# enable metabuild to build bazel dependencies
import uber.ai.michelangelo.sdk.model_manager._private.utils.reflection_utils.tests.fixtures.simple_module  # noqa:F401


class ModuleTest(TestCase):
    def test_get_module_attr(self):
        module = get_module("uber.ai.michelangelo.sdk.model_manager._private.utils.reflection_utils.tests.fixtures.simple_module.module_attr")

        self.assertIsNotNone(module)
        self.assertEqual(
            module.__name__,
            "uber.ai.michelangelo.sdk.model_manager._private.utils.reflection_utils.tests.fixtures.simple_module",
        )
