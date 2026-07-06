"""Shared fixtures for tabular_trainer task tests."""

from __future__ import annotations

from unittest.mock import Mock

import numpy as np

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


def make_model_artifact(
    path: str = "/tmp/models/base/model.pt",
    *,
    is_incremental_training: bool = False,
    baseline_model_identifier: str | None = None,
) -> ModelArtifact:
    """Return a ``ModelArtifact`` for use as an ``initial_model``.

    ``path`` points directly to a local state-dict file (matching what
    ``LightningTrainerParam.initial_weights_path`` expects and what
    ``ModelVariable.save_lightning_model()`` writes) — not a directory.
    The default path does not exist on disk; tests exercising the
    ``os.path.isfile`` guard in ``_train_lightning`` should mock it or
    pass a real file path.
    """
    meta = ModelMetadata(
        training_framework="lightning",
        is_incremental_training=is_incremental_training,
        baseline_model_identifier=baseline_model_identifier,
    )
    return ModelArtifact(path=path, metadata=meta)
