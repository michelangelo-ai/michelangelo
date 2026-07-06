"""Shared fixtures for tabular_trainer task tests."""

from __future__ import annotations

from unittest.mock import MagicMock, Mock

import numpy as np

from michelangelo.lib.artifact_manager.storage_backend import StorageBackend
from michelangelo.workflow.schema.tabular_trainer import (
    ColumnConfig,
    LightningTrainerConfig,
    TabularTrainerConfig,
)
from michelangelo.workflow.variables.metadata import ModelMetadata
from michelangelo.workflow.variables.types import ModelArtifact


def make_lightning_config(**overrides) -> LightningTrainerConfig:
    """Return a minimal valid ``LightningTrainerConfig``."""
    defaults = {
        "model_class": "tests.fixtures.DummyModel",
        "input_columns": {"x": ColumnConfig("torch.float32")},
        "output_columns": {"y": ColumnConfig("torch.float32")},
        "labels": {"label": ColumnConfig("torch.long")},
        "metadata_columns": [],
    }
    defaults.update(overrides)
    return LightningTrainerConfig(**defaults)


def make_tabular_config(**lightning_overrides) -> TabularTrainerConfig:
    """Return a minimal valid ``TabularTrainerConfig`` with a lightning backend."""
    return TabularTrainerConfig(lightning=make_lightning_config(**lightning_overrides))


def mock_train_dataset(sample_row: dict | None = None) -> Mock:
    """Return a Mock ``DatasetVariable`` with a sensible ``value``."""
    if sample_row is None:
        sample_row = {"x": np.array([1.0]), "label": np.array([0])}
    ds_mock = Mock()
    ds_mock.value.take.return_value = [sample_row]
    ds_mock.value.select_columns.return_value = ds_mock.value
    return ds_mock


def mock_validation_dataset() -> Mock:
    """Return a Mock validation ``DatasetVariable``."""
    ds_mock = Mock()
    ds_mock.value.select_columns.return_value = ds_mock.value
    return ds_mock


def mock_storage_backend() -> Mock:
    """Return a Mock ``StorageBackend`` that records calls."""
    backend = MagicMock(spec=StorageBackend)
    backend.upload.return_value = "s3://bucket/models/abc123"
    return backend


def make_model_artifact(
    path: str = "s3://bucket/models/base",
    *,
    is_incremental_training: bool = False,
    baseline_model_identifier: str | None = None,
) -> ModelArtifact:
    """Return a ``ModelArtifact`` for use as an ``initial_model``."""
    meta = ModelMetadata(
        training_framework="lightning",
        is_incremental_training=is_incremental_training,
        baseline_model_identifier=baseline_model_identifier,
    )
    return ModelArtifact(path=path, metadata=meta)
