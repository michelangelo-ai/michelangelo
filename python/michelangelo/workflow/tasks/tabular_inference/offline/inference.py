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

    Each config entry is resolved on its own:

    - An entry with a ``model`` is looked up in the registry, even when
      ``assembled_model`` is given. A ``custom`` model uses the raw artifact and
      every other model uses the deployable artifact. Its predictions carry
      ``model_name`` and ``model_version`` columns.
    - An entry without a ``model`` scores ``assembled_model``. The training
      framework recorded on its raw model picks the package: PyTorch and
      Lightning models use the deployable package and any other framework uses
      the raw package. Its predictions carry no model name, because an
      assembled model has none.

    The two rules differ for frameworks other than custom, PyTorch and Lightning
    on purpose: an assembled model always records its framework, while a
    registry model may have no ``training_framework`` label, so the registry
    defaults to the deployable artifact, the one built for serving.

    Every entry is checked before any Ray work starts.

    Args:
        config: One entry per model to run. An empty list returns ``datasets``
            unchanged.
        datasets: Dataset name to Ray dataset.
        assembled_model: The model scored by the entry without a ``model``.
        storage_backend: Downloads model packages that are not local
            directories. Required to resolve a registry model.
        registry_client: Looks up registry models. Required when an entry has a
            ``model``.

    Returns:
        The datasets with prediction columns added, and the metadata of the
        output columns the models produced.

    Raises:
        ConfigurationError: If an entry has no ``model`` and no
            ``assembled_model`` is given, more than one entry would score
            ``assembled_model`` (every row would be scored twice by the same
            model), a PyTorch or Lightning ``assembled_model`` has no
            deployable package, or a registry model is needed and
            ``registry_client`` or ``storage_backend`` is missing, the model
            is not registered, or it has no artifact URI.
    """
    if not config:
        _logger.warning(
            "No model inference config provided. "
            "Skipping model inference and returning the original datasets."
        )
        return datasets, []

    assembled_entries = [i for i, cfg in enumerate(config) if cfg.model is None]
    if assembled_entries and assembled_model is None:
        raise ConfigurationError(
            f"local_models[{assembled_entries[0]}] has no model and no "
            "assembled_model was passed, so there is nothing to score."
        )
    if len(assembled_entries) > 1:
        raise ConfigurationError(
            f"local_models{assembled_entries} have no model, so each would score "
            "the assembled model and duplicate every row. Give at most one entry "
            "no model."
        )

    inference_configs = []
    for cfg in config:
        if cfg.model is None:
            model = _resolve_assembled_model(assembled_model)
        else:
            model = _resolve_registry_model(cfg, storage_backend, registry_client)

        inference_configs.append(
            InferenceConfig(
                model=model,
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


def _resolve_assembled_model(assembled_model: AssembledModel) -> ModelIdentifier:
    """Pick the package of ``assembled_model`` by its training framework.

    Args:
        assembled_model: The assembler's output.

    Returns:
        The model to score, with no name: PyTorch and Lightning models use the
        deployable package, any other framework the raw package.

    Raises:
        ConfigurationError: If a PyTorch or Lightning model has no deployable
            package.
    """
    framework = assembled_model.raw_model.metadata.training_framework
    model_type = _FRAMEWORK_TO_MODEL_TYPE.get(framework, framework)
    if model_type in (RawModelType.TORCH, RawModelType.LIGHTNING):
        if assembled_model.deployable_model is None:
            raise ConfigurationError(
                f"The assembled {framework} model has no deployable package, "
                "which is what PyTorch and Lightning models are scored from. "
                "Assemble it with a deployable package."
            )
        model_path = assembled_model.deployable_model.path
    else:
        model_path = assembled_model.raw_model.path
    return ModelIdentifier(model_path=model_path, model_type=model_type)


def _resolve_registry_model(
    cfg: OfflineInferenceConfig,
    storage_backend: StorageBackend | None,
    registry_client: ModelRegistryClient | None,
) -> ModelIdentifier:
    """Look up ``cfg.model`` in the registry and pick its package.

    Args:
        cfg: The config entry whose ``model`` names a registry model.
        storage_backend: Downloads the package, so it must be given.
        registry_client: Looks up the model, so it must be given.

    Returns:
        The model to score: its name, its revision (the registry's latest when
        ``cfg.model.revision_id`` is not set and the version is numeric), the
        package URI and its model type. A ``custom`` model uses the raw
        artifact and every other model uses the deployable artifact.

    Raises:
        ConfigurationError: If ``registry_client`` or ``storage_backend`` is
            missing, the model or revision is not registered, or the registry
            model has no artifact URI.
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
    try:
        registered = registry_client.get_model(
            name, str(revision_id) if revision_id is not None else None
        )
    except KeyError as e:
        raise ConfigurationError(
            f"Registry model {name!r} revision {revision_id!r} not found."
        ) from e
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
    if revision_id is None and str(registered.version).isdigit():
        revision_id = int(registered.version)
    return ModelIdentifier(
        name=name, revision_id=revision_id, model_path=uri, model_type=model_type
    )
