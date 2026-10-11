"""Tests for michelangelo.workflow.schema.custom_dataloader and its wiring."""

from __future__ import annotations

from unittest import TestCase

from michelangelo.workflow.schema.custom_dataloader import (
    CustomDataloaderConfig,
    CustomDataloaderKind,
)
from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.schema.ray_data_io import (
    BatchIterConfig,
    DataloadingConfig,
    ParquetReadConfig,
)


class TestCustomDataloaderConfig(TestCase):
    """Tests for the CustomDataloaderConfig dataclass."""

    def test_defaults(self):
        """It defaults to the Ray-backed kind with empty kwargs."""
        cfg = CustomDataloaderConfig(build_dataloader_fn="pkg.mod.build")
        self.assertEqual(cfg.build_dataloader_fn, "pkg.mod.build")
        self.assertEqual(cfg.build_dataloader_kwargs, {})
        self.assertIsNone(cfg.sample_data_fn)
        self.assertEqual(cfg.sample_data_fn_kwargs, {})
        self.assertEqual(cfg.kind, CustomDataloaderKind.RAY_DATA)

    def test_kwargs_not_shared_between_instances(self):
        """Mutable defaults are per-instance."""
        a = CustomDataloaderConfig(build_dataloader_fn="x.y")
        b = CustomDataloaderConfig(build_dataloader_fn="x.y")
        a.build_dataloader_kwargs["k"] = 1
        self.assertEqual(b.build_dataloader_kwargs, {})

    def test_kind_values_are_strings(self):
        """Kinds compare equal to their serialised string values."""
        self.assertEqual(CustomDataloaderKind.RAY_DATA, "ray_data")
        self.assertEqual(CustomDataloaderKind("file_backed"), "file_backed")


class TestDataloadingConfigCustom(TestCase):
    """Tests for custom_dataloader_config on DataloadingConfig."""

    def _custom(self):
        return CustomDataloaderConfig(build_dataloader_fn="pkg.mod.build")

    def test_custom_alone_is_valid(self):
        """It accepts a custom dataloader on its own."""
        cfg = DataloadingConfig(custom_dataloader_config=self._custom())
        self.assertIsNotNone(cfg.custom_dataloader_config)

    def test_default_has_no_custom_dataloader(self):
        """Existing configs are unaffected."""
        cfg = DataloadingConfig(batch_iter_config=BatchIterConfig(batch_size=4))
        self.assertIsNone(cfg.custom_dataloader_config)

    def test_combined_with_batch_iter_config_raises(self):
        """It rejects batch_iter_config alongside a custom dataloader."""
        with self.assertRaises(ConfigurationError):
            DataloadingConfig(
                batch_iter_config=BatchIterConfig(batch_size=4),
                custom_dataloader_config=self._custom(),
            )

    def test_combined_with_parquet_read_config_raises(self):
        """It rejects parquet_read_config alongside a custom dataloader."""
        with self.assertRaises(ConfigurationError):
            DataloadingConfig(
                parquet_read_config=ParquetReadConfig(num_cpus=1),
                custom_dataloader_config=self._custom(),
            )
