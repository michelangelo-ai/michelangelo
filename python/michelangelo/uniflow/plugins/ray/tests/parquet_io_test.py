"""Tests for michelangelo.uniflow.plugins.ray.parquet_io."""

from __future__ import annotations

import dataclasses
from unittest import TestCase
from unittest.mock import patch

from michelangelo.uniflow.plugins.ray.parquet_io import (
    _config_to_dict,
    parquet_read_config_to_kwargs,
)


@dataclasses.dataclass
class _FakeParquetReadConfig:
    """A minimal dataclass-based stand-in for a parquet-read config."""

    concurrency: int | None = None
    num_cpus: int | None = None
    num_gpus: int | None = None
    memory: int | None = None
    arrow_parquet_args: dict | None = None
    override_num_blocks_per_dataset: dict | None = None


class _FakePydanticParquetReadConfig:
    """A minimal pydantic-like stand-in exposing only ``model_dump``."""

    def __init__(self, **kwargs):
        """Store the given fields for later ``model_dump``."""
        self._fields = kwargs

    def model_dump(self, exclude_none: bool = False):
        """Return the stored fields, dropping ``None`` values if requested."""
        if exclude_none:
            return {k: v for k, v in self._fields.items() if v is not None}
        return dict(self._fields)


class ConfigToDictTest(TestCase):
    """Tests for _config_to_dict."""

    def test_dataclass_instance_drops_none_fields(self):
        """A dataclass instance is flattened, dropping None-valued fields."""
        result = _config_to_dict(_FakeParquetReadConfig(concurrency=4))
        self.assertEqual(result, {"concurrency": 4})

    def test_pydantic_like_object_uses_model_dump(self):
        """An object with model_dump() is converted via that method."""
        result = _config_to_dict(_FakePydanticParquetReadConfig(concurrency=2))
        self.assertEqual(result, {"concurrency": 2})

    def test_unsupported_type_raises_type_error(self):
        """An object with neither dataclass fields nor model_dump() raises."""
        with self.assertRaises(TypeError):
            _config_to_dict(object())


class ParquetReadConfigToKwargsTest(TestCase):
    """Tests for parquet_read_config_to_kwargs."""

    def test_none_config_returns_empty_kwargs(self):
        """A None config returns an empty kwargs dict (Ray's own defaults)."""
        self.assertEqual(parquet_read_config_to_kwargs(None), {})

    def test_basic_fields_pass_through(self):
        """Plain scalar fields pass through unchanged."""
        result = parquet_read_config_to_kwargs(_FakeParquetReadConfig(concurrency=4))
        self.assertEqual(result, {"concurrency": 4})

    def test_arrow_parquet_args_flattened_to_top_level(self):
        """arrow_parquet_args entries are merged into the top-level kwargs."""
        config = _FakeParquetReadConfig(
            concurrency=4, arrow_parquet_args={"batch_size": 100}
        )
        result = parquet_read_config_to_kwargs(config)
        self.assertEqual(result, {"concurrency": 4, "batch_size": 100})

    def test_override_num_blocks_per_dataset_selected_by_name(self):
        """The entry matching dataset_name becomes override_num_blocks."""
        config = _FakeParquetReadConfig(
            override_num_blocks_per_dataset={"train": 8, "validation": 2}
        )
        result = parquet_read_config_to_kwargs(config, dataset_name="train")
        self.assertEqual(result, {"override_num_blocks": 8})

    def test_override_num_blocks_per_dataset_no_match_drops_key(self):
        """No matching entry for dataset_name means override_num_blocks is absent."""
        config = _FakeParquetReadConfig(override_num_blocks_per_dataset={"train": 8})
        result = parquet_read_config_to_kwargs(config, dataset_name="validation")
        self.assertEqual(result, {})

    def test_override_num_blocks_per_dataset_no_dataset_name_drops_key(self):
        """No dataset_name given means override_num_blocks is absent."""
        config = _FakeParquetReadConfig(override_num_blocks_per_dataset={"train": 8})
        result = parquet_read_config_to_kwargs(config)
        self.assertEqual(result, {})

    @patch("michelangelo.uniflow.plugins.ray.parquet_io.ray")
    def test_pre_2_50_moves_resource_args_into_ray_remote_args(self, mock_ray):
        """Before Ray 2.50, num_cpus/num_gpus/memory move under ray_remote_args."""
        mock_ray.__version__ = "2.40.0"
        config = _FakeParquetReadConfig(num_cpus=2, num_gpus=1, memory=1024)
        result = parquet_read_config_to_kwargs(config)
        self.assertEqual(
            result, {"ray_remote_args": {"num_cpus": 2, "num_gpus": 1, "memory": 1024}}
        )

    @patch("michelangelo.uniflow.plugins.ray.parquet_io.ray")
    def test_post_2_50_keeps_resource_args_top_level(self, mock_ray):
        """From Ray 2.50 onward, num_cpus/num_gpus/memory stay top-level kwargs."""
        mock_ray.__version__ = "2.50.0"
        config = _FakeParquetReadConfig(num_cpus=2, num_gpus=1, memory=1024)
        result = parquet_read_config_to_kwargs(config)
        self.assertEqual(result, {"num_cpus": 2, "num_gpus": 1, "memory": 1024})
