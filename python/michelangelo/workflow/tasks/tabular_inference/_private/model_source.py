"""Resolve where each model comes from."""

from __future__ import annotations

import os
from typing import TYPE_CHECKING

from michelangelo.lib.inference import ModelSource
from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.variables import ModelVariable
from michelangelo.workflow.variables.metadata import (
    TRAINING_FRAMEWORK_CUSTOM,
    TRAINING_FRAMEWORK_LIGHTNING,
    TRAINING_FRAMEWORK_PYTORCH,
)
from michelangelo.workflow.variables.types import AssembledModel

if TYPE_CHECKING:
    from michelangelo.lib.artifact_manager.storage_backend import StorageBackend
    from michelangelo.lib.model_manager.registry.client import ModelRegistryClient
    from michelangelo.workflow.schema.inference import OfflineInferenceConfig

#: Frameworks whose models are scored from the deployable package, the artifact
#: serving runs. Every other framework is scored from the raw package.
_DEPLOYABLE_FRAMEWORKS = (TRAINING_FRAMEWORK_PYTORCH, TRAINING_FRAMEWORK_LIGHTNING)


def resolve_model_source(
    config: OfflineInferenceConfig,
    assembled_model: ModelVariable | AssembledModel | None,
    index: int,
    *,
    storage_backend: StorageBackend | None,
    registry_client: ModelRegistryClient | None,
) -> ModelSource:
    """Find the model that ``config`` should score with.

    A registry reference in ``config.model`` wins. Otherwise ``assembled_model``
    is used. For a packaged model, the training framework picks the package:
    PyTorch and Lightning models use the deployable package. Any other
    framework, or none recorded, uses the raw package, and the package's own
    metadata decides how it loads. A registered model is the reverse: only
    ``custom`` uses the raw artifact. A ``ModelVariable`` has no package and is
    scored in-process.

    Args:
        config: The per-model configuration.
        assembled_model: The model passed to the task, if any.
        index: Position of ``config`` in ``local_models``, for error messages.
        storage_backend: Downloads packages that are not local directories.
        registry_client: Looks up ``config.model``.

    Returns:
        Where to load the model from.

    Raises:
        ConfigurationError: If no model source is available, a needed backend or
            client was not passed, or the package the framework needs has no
            path.
        TypeError: If ``assembled_model`` is not a supported type.
    """
    where = f"local_models[{index}]"
    if config.model is not None:
        return _from_registry(
            config,
            where,
            storage_backend=storage_backend,
            registry_client=registry_client,
        )
    if isinstance(assembled_model, AssembledModel):
        return _from_assembled(assembled_model, where, storage_backend)
    if isinstance(assembled_model, ModelVariable):
        return ModelSource(
            model=assembled_model.value, schema=assembled_model.metadata.schema
        )
    if assembled_model is None:
        raise ConfigurationError(
            f"No model for {where}: set {where}.model to a registry reference, or "
            "pass assembled_model (a ModelVariable or AssembledModel)."
        )
    raise TypeError(
        "assembled_model must be a ModelVariable or AssembledModel, got "
        f"{type(assembled_model).__name__}."
    )


def _from_registry(
    config: OfflineInferenceConfig,
    where: str,
    *,
    storage_backend: StorageBackend | None,
    registry_client: ModelRegistryClient | None,
) -> ModelSource:
    """Use a registered model's deployable artifact, or its raw one if custom."""
    ref = config.model
    if registry_client is None:
        raise ConfigurationError(
            f"{where}.model names registry model {ref.name!r}, so pass "
            "registry_client to look it up."
        )
    if storage_backend is None:
        raise ConfigurationError(
            f"{where}.model names registry model {ref.name!r}, so pass "
            "storage_backend to download it."
        )
    version = None if ref.revision_id is None else str(ref.revision_id)
    registered = registry_client.get_model(ref.name, version)
    if registered.labels.get("training_framework") == TRAINING_FRAMEWORK_CUSTOM:
        uri, kind = registered.artifact_uri, "artifact_uri"
    else:
        uri, kind = registered.deployable_artifact_uri, "deployable_artifact_uri"
    if not uri:
        raise ConfigurationError(
            f"Registry model {registered.name!r} version {registered.version!r} "
            f"has no {kind}."
        )
    # Each worker downloads the package itself, so it exists on whichever node
    # runs the worker.
    return ModelSource(
        uri=uri,
        storage_backend=storage_backend,
        name=registered.name,
        version=registered.version,
    )


def _from_assembled(
    model: AssembledModel, where: str, storage_backend: StorageBackend | None
) -> ModelSource:
    """Use an assembled model's deployable or raw package.

    The assembler records the training framework on the raw model, and records
    each package as a storage URI. A path that is already a local directory is
    used as it is; anything else is downloaded by each worker.
    """
    framework = model.raw_model.metadata.training_framework
    if framework in _DEPLOYABLE_FRAMEWORKS:
        artifact, kind = model.deployable_model, "deployable"
    else:
        artifact, kind = model.raw_model, "raw"
    if artifact is None or not artifact.path:
        raise ConfigurationError(f"The model for {where} has no {kind} package path.")
    schema = artifact.metadata.schema
    if os.path.isdir(artifact.path):
        return ModelSource(model_path=artifact.path, schema=schema)
    if storage_backend is None:
        raise ConfigurationError(
            f"The {kind} package of the model for {where} is at {artifact.path!r}, "
            "which is not a local directory, so pass storage_backend to download it."
        )
    return ModelSource(
        uri=artifact.path, storage_backend=storage_backend, schema=schema
    )
