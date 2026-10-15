"""Tests for the shared Ray dataset write helpers."""

from __future__ import annotations

from unittest.mock import MagicMock, patch

import pytest

from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.schema.ray_data_io import WriteConfig
from michelangelo.workflow.tasks.functions.data.ray.write import (
    save_datasets,
    write_config_to_kwargs,
)

_WRITE = "michelangelo.workflow.tasks.functions.data.ray.write"


class TestWriteConfigToKwargs:
    """Tests for ``write_config_to_kwargs``."""

    def test_none_returns_empty(self):
        """No config means Ray's defaults."""
        assert write_config_to_kwargs(None) == {}

    def test_unset_fields_are_omitted(self):
        """Only options that are set reach the kwargs."""
        assert write_config_to_kwargs(WriteConfig()) == {}

    @patch(f"{_WRITE}.ray")
    def test_all_options_forwarded_on_new_ray(self, mock_ray):
        """Every set option is forwarded when Ray is new enough."""
        mock_ray.__version__ = "2.51.0"
        config = WriteConfig(max_rows_per_file=10, min_rows_per_file=2, concurrency=4)
        assert write_config_to_kwargs(config) == {
            "max_rows_per_file": 10,
            "min_rows_per_file": 2,
            "concurrency": 4,
        }

    @pytest.mark.parametrize(
        ("field", "ray_version"),
        [("max_rows_per_file", "2.47.0"), ("min_rows_per_file", "2.42.0")],
    )
    @patch(f"{_WRITE}.ray")
    def test_option_below_ray_floor_raises(self, mock_ray, field, ray_version):
        """A too-old Ray fails at config time, naming the option."""
        mock_ray.__version__ = ray_version
        with pytest.raises(ConfigurationError, match=field):
            write_config_to_kwargs(WriteConfig(**{field: 5}))


class TestSaveDatasets:
    """Tests for ``save_datasets``."""

    def test_saves_every_dataset_with_kwargs(self):
        """Each dataset is saved once, receiving the write kwargs."""
        train, test = MagicMock(), MagicMock()
        save_datasets({"train": train, "test": test}, max_rows_per_file=5)
        train.save_ray_dataset.assert_called_once_with(max_rows_per_file=5)
        test.save_ray_dataset.assert_called_once_with(max_rows_per_file=5)

    def test_saves_without_kwargs(self):
        """No kwargs means a plain save call."""
        var = MagicMock()
        save_datasets({"train": var})
        var.save_ray_dataset.assert_called_once_with()
