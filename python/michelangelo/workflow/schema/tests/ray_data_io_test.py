"""Tests for michelangelo.workflow.schema.ray_data_io dataclasses."""

from __future__ import annotations

from unittest import TestCase

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
