from unittest import TestCase
from michelangelo._internal.gateways.terrablob_gateway.common import (
    TerrablobOptions,
    validate_kwargs,
)


class OptionTest(TestCase):
    def test_validate_kwargs(self):
        validate_kwargs({})
        validate_kwargs(
            {
                "timeout": "2h",
                "source_entity": "source_entity",
                "is_staging": True,
                "auth_mode": "auto",
            }
        )
        with self.assertRaises(TypeError):
            validate_kwargs({"unknown": "unknown"})

    def test_terrablob_options(self):
        options = TerrablobOptions()
        self.assertEqual(options.timeout, None)
        self.assertEqual(options.keepalive, False)
        self.assertEqual(options.source_entity, None)
        self.assertEqual(options.is_staging, False)
        self.assertEqual(options.auth_mode, None)

        options = TerrablobOptions(
            timeout="2h",
            keepalive=True,
            source_entity="source_entity",
            is_staging=True,
            auth_mode="auto",
        )
        self.assertEqual(options.timeout, "2h")
        self.assertEqual(options.source_entity, "source_entity")
        self.assertEqual(options.is_staging, True)
        self.assertEqual(options.auth_mode, "auto")
        self.assertEqual(options.keepalive, True)
