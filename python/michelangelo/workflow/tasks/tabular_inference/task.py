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
        assembled_model: Assembled model.
        storage_backend: Downloads model packages that are not local directories.
        registry_client: Looks up ``local_models[i].model`` in the model registry.
            Required only when a ``local_models`` entry has a ``model``
            and no ``assembled_model`` is given.

    Returns:
        A dict of dataset variables containing the prediction results along with
        the input datasets.
        The keys are dataset names and the values are DatasetVariable objects.
        The keys are the same as the input datasets.
        Prediction results are stored in the "prediction" column.
    """
    return tabular_inference_impl(
        config,
        datasets,
        assembled_model,
        storage_backend=storage_backend,
        registry_client=registry_client,
    )
