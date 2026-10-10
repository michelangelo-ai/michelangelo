"""Tests for michelangelo.workflow.schema.ray_data_io dataclasses."""

from __future__ import annotations

import dataclasses
from unittest import TestCase

from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.schema.ray_data_io import (
    BatchIterConfig,
    DataloadingConfig,
    ParquetReadConfig,
)

# ---------------------------------------------------------------------------
# ParquetReadConfig
# ---------------------------------------------------------------------------


class TestParquetReadConfig(TestCase):
    """Tests for ParquetReadConfig dataclass."""

    def test_all_none_by_default(self):
        """All optional fields default to None."""
        cfg = ParquetReadConfig()
        for attr in (
            "num_cpus",
            "num_gpus",
            "memory",
            "concurrency",
            "override_num_blocks",
            "shuffle",
            "tensor_column_schema",
            "arrow_parquet_args",
        ):
            self.assertIsNone(getattr(cfg, attr), msg=f"{attr} should be None")

    def test_fields_stored(self):
        """It stores provided values."""
        cfg = ParquetReadConfig(num_cpus=2.0, shuffle="files", concurrency=4)
        self.assertEqual(cfg.num_cpus, 2.0)
        self.assertEqual(cfg.shuffle, "files")
        self.assertEqual(cfg.concurrency, 4)


# ---------------------------------------------------------------------------
# BatchIterConfig
# ---------------------------------------------------------------------------


class TestBatchIterConfig(TestCase):
    """Tests for BatchIterConfig dataclass."""

    def test_required_batch_size(self):
        """batch_size is required; rest default."""
        cfg = BatchIterConfig(batch_size=64)
        self.assertEqual(cfg.batch_size, 64)
        self.assertEqual(cfg.num_shuffle_batches, 0)
        self.assertIsNone(cfg.collate_fn)

    def test_all_fields(self):
        """It stores all fields."""
        cfg = BatchIterConfig(
            batch_size=32,
            num_shuffle_batches=4,
            collate_fn="myproject.collate.fn",
        )
        self.assertEqual(cfg.num_shuffle_batches, 4)
        self.assertEqual(cfg.collate_fn, "myproject.collate.fn")

    def test_prefetch_batches_defaults_to_ray_default(self):
        """prefetch_batches defaults to 1, the Ray Data default."""
        self.assertEqual(BatchIterConfig(batch_size=8).prefetch_batches, 1)

    def test_prefetch_batches_stored(self):
        """It stores an explicit prefetch_batches, including 0 (disabled)."""
        self.assertEqual(
            BatchIterConfig(batch_size=8, prefetch_batches=12).prefetch_batches, 12
        )
        self.assertEqual(
            BatchIterConfig(batch_size=8, prefetch_batches=0).prefetch_batches, 0
        )

    def test_prefetch_batches_invalid_raises(self):
        """Negative, non-integer and bool prefetch_batches raise ConfigurationError."""
        for bad in (-1, 1.5, "4", True, None):
            with self.subTest(value=bad), self.assertRaises(ConfigurationError):
                BatchIterConfig(batch_size=8, prefetch_batches=bad)

    def test_asdict_roundtrip(self):
        """dataclasses.asdict()/cls(**dct) round-trips BatchIterConfig."""
        cfg = BatchIterConfig(batch_size=8, num_shuffle_batches=2, prefetch_batches=5)
        self.assertEqual(BatchIterConfig(**dataclasses.asdict(cfg)), cfg)

    def test_codec_roundtrip(self):
        """BatchIterConfig round-trips through the UniFlow DataclassCodec."""
        from michelangelo.uniflow.core.codec import DataclassCodec

        codec = DataclassCodec()
        cfg = BatchIterConfig(
            batch_size=8, collate_fn="pkg.collate", prefetch_batches=5
        )
        self.assertEqual(codec.decode(codec.encode(cfg)), cfg)

    def test_nested_in_dataloading_config_codec_roundtrip(self):
        """A nested BatchIterConfig survives an outer DataloadingConfig round-trip."""
        from michelangelo.uniflow.core.codec import DataclassCodec

        codec = DataclassCodec()
        cfg = DataloadingConfig(
            batch_iter_config=BatchIterConfig(batch_size=8, prefetch_batches=3)
        )
        decoded = codec.decode(codec.encode(cfg))
        self.assertEqual(decoded.batch_iter_config.prefetch_batches, 3)


# ---------------------------------------------------------------------------
# DataloadingConfig
# ---------------------------------------------------------------------------


class TestDataloadingConfig(TestCase):
    """Tests for DataloadingConfig dataclass."""

    def test_all_none_by_default(self):
        """Both optional fields default to None."""
        cfg = DataloadingConfig()
        self.assertIsNone(cfg.parquet_read_config)
        self.assertIsNone(cfg.batch_iter_config)

    def test_fields_stored(self):
        """It stores provided sub-configs."""
        pr = ParquetReadConfig(shuffle="files")
        bi = BatchIterConfig(batch_size=32)
        cfg = DataloadingConfig(parquet_read_config=pr, batch_iter_config=bi)
        self.assertIs(cfg.parquet_read_config, pr)
        self.assertIs(cfg.batch_iter_config, bi)
