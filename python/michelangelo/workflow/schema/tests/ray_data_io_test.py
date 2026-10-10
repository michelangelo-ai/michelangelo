"""Tests for michelangelo.workflow.schema.ray_data_io dataclasses."""

from __future__ import annotations

import dataclasses
from unittest import TestCase

from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.schema.ray_data_io import (
    BatchIterConfig,
    DataloadingConfig,
    ParquetReadConfig,
    RayDataContextConfig,
    WriteConfig,
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
            "override_num_blocks_per_dataset",
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

    def test_override_num_blocks_per_dataset_stored(self):
        """It stores a per-dataset block-count mapping."""
        cfg = ParquetReadConfig(
            override_num_blocks_per_dataset={"train": 64, "validation": 8}
        )
        self.assertEqual(
            cfg.override_num_blocks_per_dataset, {"train": 64, "validation": 8}
        )
        self.assertIsNone(cfg.override_num_blocks)

    def test_override_num_blocks_and_per_dataset_are_exclusive(self):
        """Setting both the global and per-dataset override is rejected."""
        with self.assertRaises(ConfigurationError):
            ParquetReadConfig(
                override_num_blocks=4, override_num_blocks_per_dataset={"train": 8}
            )

    def test_invalid_block_counts_rejected(self):
        """Non-positive or non-integer block counts are rejected."""
        for bad in (0, -1, True, 1.5, "8"):
            with self.subTest(bad=bad):
                with self.assertRaises(ConfigurationError):
                    ParquetReadConfig(override_num_blocks=bad)
                with self.assertRaises(ConfigurationError):
                    ParquetReadConfig(override_num_blocks_per_dataset={"train": bad})

    def test_invalid_per_dataset_shapes_rejected(self):
        """Non-dict, empty and non-str-keyed per-dataset values are rejected."""
        for bad in ([("train", 4)], {}, {1: 4}, "train"):
            with self.subTest(bad=bad), self.assertRaises(ConfigurationError):
                ParquetReadConfig(override_num_blocks_per_dataset=bad)

    def test_codec_roundtrip_new_fields(self):
        """The new fields round-trip through the UniFlow DataclassCodec."""
        from michelangelo.uniflow.core.codec import DataclassCodec

        codec = DataclassCodec()
        for cfg in (
            ParquetReadConfig(override_num_blocks_per_dataset={"train": 3, "val": 1}),
            WriteConfig(max_rows_per_file=10, concurrency=4),
            RayDataContextConfig(max_blocks_in_streaming_gen_buffer=1),
        ):
            with self.subTest(cfg=type(cfg).__name__):
                self.assertEqual(codec.decode(codec.encode(cfg)), cfg)

    def test_per_dataset_asdict_roundtrip(self):
        """dataclasses.asdict()/cls(**dct) round-trips the per-dataset field."""
        cfg = ParquetReadConfig(override_num_blocks_per_dataset={"train": 3})
        self.assertEqual(ParquetReadConfig(**dataclasses.asdict(cfg)), cfg)


# ---------------------------------------------------------------------------
# WriteConfig / RayDataContextConfig
# ---------------------------------------------------------------------------


class TestWriteConfig(TestCase):
    """Tests for the WriteConfig dataclass."""

    def test_concurrency_defaults_to_none(self):
        """Concurrency is unset by default, leaving writes uncapped."""
        self.assertIsNone(WriteConfig().concurrency)

    def test_concurrency_stored_and_roundtrips(self):
        """It stores concurrency and round-trips through asdict."""
        cfg = WriteConfig(max_rows_per_file=10, concurrency=4)
        self.assertEqual(cfg.concurrency, 4)
        self.assertEqual(WriteConfig(**dataclasses.asdict(cfg)), cfg)

    def test_invalid_concurrency_rejected(self):
        """Non-positive or non-integer concurrency is rejected."""
        for bad in (0, -2, True, 2.5):
            with self.subTest(bad=bad), self.assertRaises(ConfigurationError):
                WriteConfig(concurrency=bad)


class TestRayDataContextConfig(TestCase):
    """Tests for the RayDataContextConfig dataclass."""

    def test_buffer_setting_defaults_to_none(self):
        """The streaming buffer setting is unset by default."""
        self.assertIsNone(RayDataContextConfig().max_blocks_in_streaming_gen_buffer)

    def test_buffer_setting_stored_and_roundtrips(self):
        """It stores the buffer setting and round-trips through asdict."""
        cfg = RayDataContextConfig(max_blocks_in_streaming_gen_buffer=1)
        self.assertEqual(cfg.max_blocks_in_streaming_gen_buffer, 1)
        self.assertEqual(RayDataContextConfig(**dataclasses.asdict(cfg)), cfg)

    def test_invalid_buffer_setting_rejected(self):
        """Non-positive or non-integer values are rejected."""
        for bad in (0, -1, False, 1.0):
            with self.subTest(bad=bad), self.assertRaises(ConfigurationError):
                RayDataContextConfig(max_blocks_in_streaming_gen_buffer=bad)


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
