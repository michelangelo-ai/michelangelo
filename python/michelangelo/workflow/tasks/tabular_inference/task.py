"""Tabular inference workflow task."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

from michelangelo.workflow.tasks.tabular_inference.offline.task_impl import (
    tabular_inference_impl,
)

if TYPE_CHECKING:
    from michelangelo.lib.artifact_manager.storage_backend import StorageBackend
    from michelangelo.lib.model_manager.registry.client import ModelRegistryClient
    from michelangelo.workflow.schema.inference import TabularInferenceConfig
    from michelangelo.workflow.variables import DatasetVariable
    from michelangelo.workflow.variables.types import AssembledModel

_logger = logging.getLogger(__name__)

__all__ = ["tabular_inference"]


def tabular_inference(
    config: TabularInferenceConfig,
    datasets: dict[str, DatasetVariable],
    assembled_model: AssembledModel | None = None,
    *,
    storage_backend: StorageBackend | None = None,
    registry_client: ModelRegistryClient | None = None,
) -> dict[str, DatasetVariable]:
    """Tabular inference task.

    Produces model predictions on train, validation, and test datasets.

    Args:
        config: Tabular inference configuration.
        datasets: A dict of datasets.
            The keys are dataset names and the values are DatasetVariable objects.
            The inference is run on each dataset.
            For a train pipeline, the datasets are train, validation, and
            (optional) test datasets.
            For an inference pipeline, the datasets are input datasets for
            inference.
        assembled_model: The model scored by the ``local_models`` entry that
            has no ``model``. At most one entry may have no ``model``; entries
            with a ``model`` are looked up in the registry even when this is
            given.
        storage_backend: Downloads model packages that are not local directories.
        registry_client: Looks up ``local_models[i].model`` in the model registry.
            Required only when a ``local_models`` entry has a ``model``.

    Returns:
        A dict of dataset variables holding the prediction results along with
        the input columns, keyed by dataset name.

        - A ``None`` input dataset is left out of the result.
        - Prediction columns are named after the model's output schema.
        - When ``columns_to_keep`` is set, only those input columns are kept,
          so label columns that are not listed are dropped.
        - An empty input dataset is passed through unscored, with no
          prediction columns.
        - With no ``local_models``, the input datasets (sampled, if
          ``sampling`` is set) are written out as new datasets.
    """
    return tabular_inference_impl(
        config,
        datasets,
        assembled_model,
        storage_backend=storage_backend,
        registry_client=registry_client,
    )
