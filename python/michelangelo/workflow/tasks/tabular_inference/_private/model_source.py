"""Resolve where each model comes from."""

from __future__ import annotations

import os
from typing import TYPE_CHECKING, Any

from michelangelo.lib.inference import ModelSource
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
    from michelangelo.workflow.variables.types import ModelArtifact

#: Frameworks scored from the deployable package, the artifact serving runs.
_DEPLOYABLE_FRAMEWORKS = (TRAINING_FRAMEWORK_PYTORCH, TRAINING_FRAMEWORK_LIGHTNING)


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
    the task is used. A packaged model's training framework picks the package:
    PyTorch and Lightning models are scored from the deployable package, the
    artifact serving runs, and custom models from the raw package. A
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
            was not passed, the training framework is missing or not supported,
            or the package that framework needs is missing.
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


def _wants_deployable(framework: str | None, model_name: str) -> bool:
    """Whether a model with ``framework`` is scored from its deployable package.

    Raises:
        ValueError: If ``framework`` is missing or not supported.
    """
    if framework in _DEPLOYABLE_FRAMEWORKS:
        return True
    if framework == TRAINING_FRAMEWORK_CUSTOM:
        return False
    supported = ", ".join((*_DEPLOYABLE_FRAMEWORKS, TRAINING_FRAMEWORK_CUSTOM))
    if framework is None:
        raise ValueError(
            f"{model_name} has no training framework recorded, so it is not "
            f"known which package to score. Supported frameworks: {supported}."
        )
    raise ValueError(
        f"{model_name} has training framework {framework!r}, which "
        f"tabular_inference does not support. Supported frameworks: {supported}."
    )


def _from_registry(
    config: OfflineInferenceConfig,
    where: str,
    *,
    storage_backend: StorageBackend | None,
    registry_client: ModelRegistryClient | None,
) -> ModelSource:
    """Use a registered model's deployable or raw package."""
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
    label = f"Registry model {registered.name!r} version {registered.version!r}"
    if _wants_deployable(registered.labels.get("training_framework"), label):
        uri, kind = registered.deployable_artifact_uri, "deployable_artifact_uri"
    else:
        uri, kind = registered.artifact_uri, "artifact_uri"
    if not uri:
        raise ValueError(f"{label} has no {kind}.")
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
    label = f"The model for {where}"
    if _wants_deployable(model.raw_model.metadata.training_framework, label):
        artifact, kind = model.deployable_model, "deployable"
    else:
        artifact, kind = model.raw_model, "raw"
    if artifact is None or not artifact.path:
        raise ValueError(f"{label} has no {kind} package path.")
    return _source(artifact, label, kind, storage_backend)


def _source(
    artifact: ModelArtifact,
    label: str,
    kind: str,
    storage_backend: StorageBackend | None,
) -> ModelSource:
    """Point at a local package directory, or a URI each worker downloads."""
    schema = artifact.metadata.schema if artifact.metadata else None
    if os.path.isdir(artifact.path):
        return ModelSource(model_path=artifact.path, schema=schema)
    if storage_backend is None:
        raise ValueError(
            f"The {kind} package of {label[0].lower()}{label[1:]} is at "
            f"{artifact.path!r}, which is not a local directory, so pass "
            "storage_backend to download it."
        )
    return ModelSource(
        uri=artifact.path, storage_backend=storage_backend, schema=schema
    )
