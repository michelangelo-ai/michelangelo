"""Run offline model inference on Ray datasets for the tabular inference task."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

from michelangelo.lib.inference.ray.inference import (
    BatchOptions,
    InferenceConfig,
    ModelIdentifier,
    OutputColumnMetadata,
    PaddingOptions,
    batch_model_inference,
)
from michelangelo.lib.model_manager.constants import RawModelType
from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.variables.metadata import (
    TRAINING_FRAMEWORK_CUSTOM,
    TRAINING_FRAMEWORK_LIGHTNING,
    TRAINING_FRAMEWORK_PYTORCH,
)

if TYPE_CHECKING:
    import ray

    from michelangelo.lib.artifact_manager.storage_backend import StorageBackend
    from michelangelo.lib.model_manager.registry.client import ModelRegistryClient
    from michelangelo.workflow.schema.inference import OfflineInferenceConfig
    from michelangelo.workflow.variables.types import AssembledModel

_logger = logging.getLogger(__name__)

__all__ = ["offline_inference"]

# Training frameworks recorded on a model, mapped to the model types the
# inference library loads by. A framework that is not listed is passed through
# unchanged.
_FRAMEWORK_TO_MODEL_TYPE = {
    TRAINING_FRAMEWORK_CUSTOM: RawModelType.CUSTOM_PYTHON,
    TRAINING_FRAMEWORK_PYTORCH: RawModelType.TORCH,
    TRAINING_FRAMEWORK_LIGHTNING: RawModelType.LIGHTNING,
}


def offline_inference(
    config: list[OfflineInferenceConfig],
    datasets: dict[str, ray.data.Dataset],
    assembled_model: AssembledModel | None = None,
    *,
    storage_backend: StorageBackend | None = None,
    registry_client: ModelRegistryClient | None = None,
) -> tuple[dict[str, ray.data.Dataset], list[OutputColumnMetadata]]:
    """Run each configured model over the datasets.

    The model package comes from ``assembled_model`` when it is given, for every
    config entry. The training framework recorded on the raw model picks the
    package: PyTorch and Lightning models use the deployable package and any
    other framework uses the raw package. Otherwise a config entry's ``model``
    is looked up in the registry: ``custom`` models use the raw artifact and
    every other model uses the deployable artifact.

    Args:
        config: One entry per model to run. An empty list returns ``datasets``
            unchanged.
        datasets: Dataset name to Ray dataset.
        assembled_model: The model to use for every entry, if any.
        storage_backend: Downloads model packages that are not local
            directories. Required to resolve a registry model.
        registry_client: Looks up registry models. Required when an entry has a
            ``model`` and no ``assembled_model`` is given.

    Returns:
        The datasets with prediction columns added, and the metadata of the
        output columns the models produced.

    Raises:
        ConfigurationError: If a registry model is needed and
            ``registry_client`` or ``storage_backend`` is missing, or the
            registry model has no artifact URI.
    """
    if not config:
        _logger.warning(
            "No model inference config provided. "
            "Skipping model inference and returning the original datasets."
        )
        return datasets, []

    model_path, model_type = None, None
    if assembled_model:
        model_type = _FRAMEWORK_TO_MODEL_TYPE.get(
            assembled_model.raw_model.metadata.training_framework,
            assembled_model.raw_model.metadata.training_framework,
        )

        if model_type in (RawModelType.TORCH, RawModelType.LIGHTNING):
            model_path = assembled_model.deployable_model.path
        else:
            model_path = assembled_model.raw_model.path

    inference_configs = []
    for cfg in config:
        cfg_model_path, cfg_model_type = model_path, model_type
        if cfg.model and not assembled_model:
            cfg_model_path, cfg_model_type = _resolve_registry_model(
                cfg, storage_backend, registry_client
            )

        inference_configs.append(
            InferenceConfig(
                model=ModelIdentifier(
                    name=cfg.model.name if cfg.model else None,
                    revision_id=cfg.model.revision_id if cfg.model else None,
                    model_path=cfg_model_path,
                    model_type=cfg_model_type,
                ),
                input_columns=cfg.input_columns,
                batch_options=BatchOptions(
                    batch_size=cfg.batch_options.batch_size,
                    num_gpus=cfg.batch_options.num_gpus,
                    concurrency=cfg.batch_options.concurrency,
                    num_cpus=cfg.batch_options.num_cpus,
                    max_concurrency=cfg.batch_options.max_concurrency,
                    max_tasks_in_flight_per_actor=(
                        cfg.batch_options.max_tasks_in_flight_per_actor
                    ),
                    max_restarts=cfg.batch_options.max_restarts,
                    max_task_retries=cfg.batch_options.max_task_retries,
                )
                if cfg.batch_options
                else None,
                padding_options=PaddingOptions(
                    column_padding_values=cfg.padding_options.column_padding_values,
                )
                if cfg.padding_options
                else None,
                columns_to_keep=cfg.columns_to_keep,
            )
        )

    results, metadata = batch_model_inference(
        inference_configs, datasets, storage_backend
    )

    output_column_metadata = [
        OutputColumnMetadata(
            column_name=col.column_name,
            model_name=col.model_name,
        )
        for col in metadata
    ]

    return results, output_column_metadata


def _resolve_registry_model(
    cfg: OfflineInferenceConfig,
    storage_backend: StorageBackend | None,
    registry_client: ModelRegistryClient | None,
) -> tuple[str, str | None]:
    """Look up ``cfg.model`` in the registry and pick its package.

    Args:
        cfg: The config entry whose ``model`` names a registry model.
        storage_backend: Downloads the package, so it must be given.
        registry_client: Looks up the model, so it must be given.

    Returns:
        The package URI and its model type. A ``custom`` model uses the raw
        artifact and every other model uses the deployable artifact.

    Raises:
        ConfigurationError: If ``registry_client`` or ``storage_backend`` is
            missing, or the registry model has no artifact URI.
    """
    name = cfg.model.name
    if registry_client is None:
        raise ConfigurationError(
            f"model.name {name!r} needs a registry model, so pass registry_client "
            "to look it up."
        )
    if storage_backend is None:
        raise ConfigurationError(
            f"model.name {name!r} needs a registry model, so pass storage_backend "
            "to download it."
        )
    revision_id = cfg.model.revision_id
    registered = registry_client.get_model(
        name, str(revision_id) if revision_id is not None else None
    )
    framework = registered.labels.get("training_framework")
    if framework == TRAINING_FRAMEWORK_CUSTOM:
        uri, kind = registered.artifact_uri, "artifact_uri"
        model_type = RawModelType.CUSTOM_PYTHON
    else:
        uri, kind = registered.deployable_artifact_uri, "deployable_artifact_uri"
        model_type = _FRAMEWORK_TO_MODEL_TYPE.get(framework)
    if not uri:
        raise ConfigurationError(
            f"Registry model {registered.name!r} version {registered.version!r} "
            f"has no {kind}."
        )
    return uri, model_type
