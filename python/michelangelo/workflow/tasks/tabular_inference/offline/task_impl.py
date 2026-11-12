"""Implementation of the tabular inference task on Ray."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

from michelangelo.uniflow.plugins.ray.data_context import set_ray_data_context
from michelangelo.uniflow.plugins.ray.parquet_io import parquet_read_config_to_kwargs
from michelangelo.workflow.tasks.functions.data.ray.sampling import apply_sampling
from michelangelo.workflow.tasks.functions.data.ray.write import write_config_to_kwargs
from michelangelo.workflow.tasks.tabular_inference.offline.inference import (
    offline_inference,
)
from michelangelo.workflow.variables import DatasetVariable

if TYPE_CHECKING:
    from michelangelo.lib.artifact_manager.storage_backend import StorageBackend
    from michelangelo.lib.model_manager.registry.client import ModelRegistryClient
    from michelangelo.workflow.schema.inference import TabularInferenceConfig
    from michelangelo.workflow.variables.types import AssembledModel

_logger = logging.getLogger(__name__)

__all__ = ["tabular_inference_impl"]


def tabular_inference_impl(
    config: TabularInferenceConfig,
    datasets: dict[str, DatasetVariable],
    assembled_model: AssembledModel | None = None,
    *,
    storage_backend: StorageBackend | None = None,
    registry_client: ModelRegistryClient | None = None,
) -> dict[str, DatasetVariable]:
    """Run tabular inference on every dataset and save the results.

    Args:
        config: Tabular inference configuration.
        datasets: Dataset name to dataset variable.
        assembled_model: Assembled model used by every ``local_models`` entry.
        storage_backend: Downloads model packages that are not local directories.
        registry_client: Looks up ``local_models[i].model`` in the model registry.

    Returns:
        Dataset name to dataset variable holding the prediction results.

    Raises:
        ConfigurationError: If a model cannot be resolved or ``write_config`` asks
            for an option the installed Ray cannot do.
    """
    _logger.info("Starting tabular inference")
    _logger.info(f"Tabular inference config: {config}")

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
        preserve_order=rc.preserve_order if rc else None,
        max_errored_blocks=config.max_errored_blocks,
    )

    for name, dataset_var in datasets.items():
        if dataset_var is not None and dataset_var._value is None:
            dataset_var.load_ray_dataset(
                **parquet_read_config_to_kwargs(
                    config.parquet_read_config, dataset_name=name
                )
            )

    ray_datasets = {
        name: dataset.value for name, dataset in datasets.items() if dataset
    }

    if config.sampling:
        ray_datasets = {
            name: apply_sampling(ds, config.sampling, name)
            for name, ds in ray_datasets.items()
        }

    ray_results, output_column_metadata = offline_inference(
        config.local_models,
        ray_datasets,
        assembled_model,
        storage_backend=storage_backend,
        registry_client=registry_client,
    )
    # DatasetMetadata has no field for output columns, so log them instead.
    _logger.info(f"Tabular inference output columns: {output_column_metadata}")

    _logger.info("Tabular inference completed on all datasets.")

    write_kwargs = write_config_to_kwargs(config.write_config)

    results = {}

    for name, ray_result in ray_results.items():
        if ray_result:
            res = DatasetVariable.create(ray_result)
            results[name] = res
            res.save_ray_dataset(**write_kwargs)
            try:
                stats = ray_result.stats()
                if stats:
                    _logger.info(
                        f"[PROFILER] Tabular inference Ray Data operator stats for "
                        f"'{name}':\n{stats}"
                    )
            except Exception as e:
                _logger.debug(f"Could not get Ray Data stats for '{name}': {e}")
        else:
            results[name] = DatasetVariable.create(ray_result)

    _logger.info(f"Tabular inference results: {results}")

    return results
