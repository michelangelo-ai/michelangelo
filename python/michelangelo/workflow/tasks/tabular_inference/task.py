"""Tabular inference workflow task.

Scores Ray datasets with a trained model and returns datasets with prediction
columns added, in the form the evaluator task consumes.
"""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING, Any

from michelangelo.lib.inference import batch_model_inference
from michelangelo.uniflow.plugins.ray.data_context import set_ray_data_context
from michelangelo.uniflow.plugins.ray.parquet_io import parquet_read_config_to_kwargs
from michelangelo.workflow.tasks.functions.data.ray.write import (
    save_datasets,
    write_config_to_kwargs,
)
from michelangelo.workflow.tasks.tabular_inference._private.model_source import (
    resolve_model_source,
)
from michelangelo.workflow.variables import DatasetVariable

if TYPE_CHECKING:
    from michelangelo.lib.artifact_manager.storage_backend import StorageBackend
    from michelangelo.lib.model_manager.registry.client import ModelRegistryClient
    from michelangelo.workflow.schema.inference import TabularInferenceConfig

_logger = logging.getLogger(__name__)

__all__ = ["tabular_inference"]


def tabular_inference(
    config: TabularInferenceConfig,
    datasets: dict[str, DatasetVariable | None],
    model: Any = None,
    *,
    storage_backend: StorageBackend | None = None,
    registry_client: ModelRegistryClient | None = None,
) -> dict[str, DatasetVariable | None]:
    """Score datasets with the configured models using Ray.

    Each input column of the model is read from the dataset, the model is run in
    Ray actors, and the prediction columns are added to every row. Rows keep all
    input columns unless ``columns_to_keep`` is set.

    Args:
        config: Which models to run and how.
        datasets: Dataset name to dataset, for example ``{"train": ...,
            "validation": ...}``. ``None`` entries are passed through.
        model: The model to score with, used by every ``local_models`` entry that
            has no ``model`` reference: a ``ModelVariable`` (scored in-process) or
            an ``AssembledModel`` (scored from its raw package).
        storage_backend: Downloads model packages that are not local
            directories: registry artifacts, and the raw package of an
            ``AssembledModel`` stored remotely. Not needed for an in-process
            ``ModelVariable`` or a local package directory.
        registry_client: Looks up ``local_models[i].model``. Required only when
            a ``local_models`` entry has a ``model`` reference.

    Returns:
        A dict with the same keys as ``datasets``. Scored datasets are written
        with ``config.write_config``. ``None`` and empty datasets are returned as
        they came in. With no ``local_models``, ``datasets`` is returned
        unchanged.

    Raises:
        ValueError: If a model cannot be resolved, or models produce different
            output columns.
        ConfigurationError: If ``config.write_config`` asks for an option the
            installed Ray cannot do.
        TypeError: If ``model`` is not a supported type, or a dataset is already
            loaded as something other than a Ray Dataset.
    """
    if not config.local_models:
        _logger.warning("No local_models configured; returning datasets unchanged.")
        return datasets

    rc = config.ray_data_context
    set_ray_data_context(
        min_block_size=rc.min_block_size if rc else None,
        max_block_size=rc.max_block_size if rc else None,
        retried_io_errors=rc.retried_io_errors if rc else None,
        object_store_memory_limit=rc.object_store_memory_limit if rc else None,
        wait_for_min_actors_s=rc.wait_for_min_actors_s if rc else None,
        max_blocks_in_streaming_gen_buffer=(
            rc.max_blocks_in_streaming_gen_buffer if rc else None
        ),
    )

    # Fail on a bad write config before any scoring work is done.
    write_kwargs = write_config_to_kwargs(config.write_config)

    # Datasets are lazy: this registers the reads without loading rows.
    for name, dataset_var in datasets.items():
        if dataset_var is not None:
            dataset_var.load_ray_dataset(
                **parquet_read_config_to_kwargs(
                    config.parquet_read_config, dataset_name=name
                )
            )

    for name, dataset_var in datasets.items():
        if dataset_var is not None and not hasattr(dataset_var.value, "map_batches"):
            raise TypeError(
                f"datasets[{name!r}] holds a {type(dataset_var.value).__name__}, not a "
                "Ray Dataset. Pass a DatasetVariable that has not been loaded "
                "as another type."
            )

    sources = [
        resolve_model_source(
            model_config,
            model,
            index,
            storage_backend=storage_backend,
            registry_client=registry_client,
        )
        for index, model_config in enumerate(config.local_models)
    ]

    scored = batch_model_inference(
        {
            name: var.value if var is not None else None
            for name, var in datasets.items()
        },
        list(zip(config.local_models, sources)),
    )

    results: dict[str, DatasetVariable | None] = {}
    to_save: dict[str, DatasetVariable] = {}
    for name, dataset_var in datasets.items():
        if dataset_var is None or scored[name] is dataset_var.value:
            results[name] = dataset_var
            continue
        results[name] = to_save[name] = DatasetVariable.create(scored[name])

    save_datasets(to_save, **write_kwargs)
    _logger.info("Tabular inference completed for %d dataset(s)", len(to_save))
    return results
