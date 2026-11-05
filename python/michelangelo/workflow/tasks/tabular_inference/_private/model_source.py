"""Resolve where each model comes from."""

from __future__ import annotations

import os
from typing import TYPE_CHECKING, Any

from michelangelo.lib.inference import ModelSource
from michelangelo.workflow.variables import ModelVariable
from michelangelo.workflow.variables.types import AssembledModel

if TYPE_CHECKING:
    from michelangelo.lib.artifact_manager.storage_backend import StorageBackend
    from michelangelo.lib.model_manager.registry.client import ModelRegistryClient
    from michelangelo.workflow.schema.inference import OfflineInferenceConfig


def resolve_model_source(
    config: OfflineInferenceConfig,
    model: Any,
    index: int,
    *,
    storage_backend: StorageBackend | None,
    registry_client: ModelRegistryClient | None,
) -> ModelSource:
    """Find the model that ``config`` should score with.

    A registry reference in ``config.model`` wins. Otherwise the model passed to
    the task is used. A packaged model is always scored from its deployable
    package, the artifact serving runs, so batch and online predictions agree. A
    ``ModelVariable`` has no package and is scored in-process.

    Args:
        config: The per-model configuration.
        model: The model passed to the task, if any.
        index: Position of ``config`` in ``local_models``, for error messages.
        storage_backend: Downloads packages that are not local directories.
        registry_client: Looks up ``config.model``.

    Returns:
        Where to load the model from.

    Raises:
        ValueError: If no model source is available, a needed backend or client
            was not passed, or the model has no deployable package.
        TypeError: If ``model`` is not a supported type.
    """
    where = f"local_models[{index}]"
    if config.model is not None:
        return _from_registry(
            config,
            where,
            storage_backend=storage_backend,
            registry_client=registry_client,
        )
    if isinstance(model, AssembledModel):
        return _from_assembled(model, where, storage_backend)
    if isinstance(model, ModelVariable):
        return ModelSource(model=model.value, schema=model.metadata.schema)
    if model is None:
        raise ValueError(
            f"No model for {where}: set {where}.model to a registry reference, or "
            "pass the model argument (a ModelVariable or AssembledModel)."
        )
    raise TypeError(
        f"model must be a ModelVariable or AssembledModel, got {type(model).__name__}."
    )


def _from_registry(
    config: OfflineInferenceConfig,
    where: str,
    *,
    storage_backend: StorageBackend | None,
    registry_client: ModelRegistryClient | None,
) -> ModelSource:
    """Use a registered model's deployable package."""
    ref = config.model
    if registry_client is None:
        raise ValueError(
            f"{where}.model names registry model {ref.name!r}, so pass "
            "registry_client to look it up."
        )
    if storage_backend is None:
        raise ValueError(
            f"{where}.model names registry model {ref.name!r}, so pass "
            "storage_backend to download it."
        )
    version = None if ref.revision_id is None else str(ref.revision_id)
    registered = registry_client.get_model(ref.name, version)
    if not registered.deployable_artifact_uri:
        raise ValueError(
            f"Registry model {registered.name!r} version {registered.version!r} "
            "has no deployable_artifact_uri. tabular_inference scores the "
            "deployable package; register the model with one."
        )
    # Each worker downloads the package itself, so it exists on whichever node
    # runs the worker.
    return ModelSource(
        uri=registered.deployable_artifact_uri,
        storage_backend=storage_backend,
        name=registered.name,
        version=registered.version,
    )


def _from_assembled(
    model: AssembledModel, where: str, storage_backend: StorageBackend | None
) -> ModelSource:
    """Use an assembled model's deployable package.

    The assembler records the package as a storage URI. A path that is already
    a local directory is used as it is; anything else is downloaded by each
    worker.
    """
    deployable = model.deployable_model
    if deployable is None or not deployable.path:
        raise ValueError(
            f"The model for {where} has no deployable package. tabular_inference "
            "scores the deployable package; pass the output of an assembler."
        )

    schema = deployable.metadata.schema if deployable.metadata else None
    if os.path.isdir(deployable.path):
        return ModelSource(model_path=deployable.path, schema=schema)
    if storage_backend is None:
        raise ValueError(
            f"The deployable package of the model for {where} is at "
            f"{deployable.path!r}, which is not a local directory, so pass "
            "storage_backend to download it."
        )
    return ModelSource(
        uri=deployable.path, storage_backend=storage_backend, schema=schema
    )
