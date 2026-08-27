"""Tests for michelangelo.workflow.schema.assembler config dataclasses."""

from __future__ import annotations

from dataclasses import asdict, dataclass
from unittest import TestCase

from michelangelo.workflow.schema.assembler import (
    CustomAssemblerConfig,
    TabularAssemblerConfig,
    TorchAssemblerConfig,
)


class TestCustomAssemblerConfig(TestCase):
    """Tests for CustomAssemblerConfig defaults and equality."""

    def test_defaults(self):
        """All fields default to None."""
        cfg = CustomAssemblerConfig()
        self.assertIsNone(cfg.custom_batch_processing)
        self.assertIsNone(cfg.additional_import_prefixes)
        self.assertIsNone(cfg.include_import_prefixes)

    def test_field_assignment(self):
        """Fields are set from constructor arguments."""
        cfg = CustomAssemblerConfig(
            custom_batch_processing=True,
            additional_import_prefixes=["mypkg.util"],
            include_import_prefixes=["mypkg.models"],
        )
        self.assertTrue(cfg.custom_batch_processing)
        self.assertEqual(cfg.additional_import_prefixes, ["mypkg.util"])
        self.assertEqual(cfg.include_import_prefixes, ["mypkg.models"])

    def test_equality(self):
        """Two configs with the same fields compare equal."""
        self.assertEqual(
            CustomAssemblerConfig(custom_batch_processing=True),
            CustomAssemblerConfig(custom_batch_processing=True),
        )

    def test_asdict_round_trip(self):
        """The config is serialisable via dataclasses.asdict."""
        cfg = CustomAssemblerConfig(custom_batch_processing=True)
        self.assertEqual(
            asdict(cfg),
            {
                "custom_batch_processing": True,
                "additional_import_prefixes": None,
                "include_import_prefixes": None,
                "archive_deployable_package": False,
            },
        )


class TestTorchAssemblerConfig(TestCase):
    """Tests for TorchAssemblerConfig defaults and equality."""

    def test_defaults(self):
        """The backend field defaults to None."""
        cfg = TorchAssemblerConfig()
        self.assertIsNone(cfg.backend)

    def test_field_assignment(self):
        """The backend field is set from the constructor argument."""
        cfg = TorchAssemblerConfig(backend="onnxruntime")
        self.assertEqual(cfg.backend, "onnxruntime")

    def test_equality(self):
        """Two configs with the same backend compare equal."""
        self.assertEqual(
            TorchAssemblerConfig(backend="pytorch"),
            TorchAssemblerConfig(backend="pytorch"),
        )

    def test_asdict_round_trip(self):
        """The config is serialisable via dataclasses.asdict."""
        cfg = TorchAssemblerConfig(backend="pytorch")
        self.assertEqual(
            asdict(cfg),
            {
                "backend": "pytorch",
                "include_import_prefixes": None,
                "archive_deployable_package": False,
            },
        )


class TestTabularAssemblerConfig(TestCase):
    """Tests for TabularAssemblerConfig defaults, nesting, and extensibility."""

    def test_defaults(self):
        """All fields default to None."""
        cfg = TabularAssemblerConfig()
        self.assertIsNone(cfg.model_class)
        self.assertIsNone(cfg.custom)
        self.assertIsNone(cfg.torch)

    def test_nesting_custom(self):
        """The custom sub-config is retained and accessible."""
        cfg = TabularAssemblerConfig(
            model_class="mypkg.models.MyModel",
            custom=CustomAssemblerConfig(custom_batch_processing=True),
        )
        self.assertEqual(cfg.model_class, "mypkg.models.MyModel")
        self.assertTrue(cfg.custom.custom_batch_processing)
        self.assertIsNone(cfg.torch)

    def test_nesting_torch(self):
        """The torch sub-config is retained and accessible."""
        cfg = TabularAssemblerConfig(torch=TorchAssemblerConfig(backend="pytorch"))
        self.assertEqual(cfg.torch.backend, "pytorch")
        self.assertIsNone(cfg.custom)

    def test_equality(self):
        """Two configs with equivalent nested fields compare equal."""
        self.assertEqual(
            TabularAssemblerConfig(torch=TorchAssemblerConfig(backend="pytorch")),
            TabularAssemblerConfig(torch=TorchAssemblerConfig(backend="pytorch")),
        )

    def test_repr_contains_field_values(self):
        """The generated repr reflects field values (guards field reordering)."""
        cfg = TabularAssemblerConfig(model_class="mypkg.models.MyModel")
        self.assertIn("model_class='mypkg.models.MyModel'", repr(cfg))

    def test_asdict_round_trip(self):
        """The nested config is serialisable via dataclasses.asdict."""
        cfg = TabularAssemblerConfig(
            model_class="mypkg.models.MyModel",
            custom=CustomAssemblerConfig(custom_batch_processing=True),
        )
        self.assertEqual(
            asdict(cfg),
            {
                "model_class": "mypkg.models.MyModel",
                "custom": {
                    "custom_batch_processing": True,
                    "additional_import_prefixes": None,
                    "include_import_prefixes": None,
                    "archive_deployable_package": False,
                },
                "torch": None,
            },
        )

    def test_subclassability(self):
        """Consumers can subclass to add provider-specific fields."""

        @dataclass
        class _CustomTabularAssemblerConfig(TabularAssemblerConfig):
            use_remote_storage: bool = False

        cfg = _CustomTabularAssemblerConfig(
            model_class="mypkg.models.MyModel", use_remote_storage=True
        )
        self.assertEqual(cfg.model_class, "mypkg.models.MyModel")
        self.assertTrue(cfg.use_remote_storage)
        self.assertIsInstance(cfg, TabularAssemblerConfig)
