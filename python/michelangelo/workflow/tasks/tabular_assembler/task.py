"""Tabular assembler task — dispatches to the framework-specific assembler."""

from __future__ import annotations

from typing import TYPE_CHECKING

from michelangelo.workflow.tasks.tabular_assembler._private.model_class.resolve import (
    resolve_training_framework,
)
from michelangelo.workflow.tasks.tabular_assembler.custom.assembler import (
    custom_assembler,
)
from michelangelo.workflow.tasks.tabular_assembler.torch.assembler import (
    torch_assembler,
)
from michelangelo.workflow.variables.metadata import (
    TRAINING_FRAMEWORK_CUSTOM,
    TRAINING_FRAMEWORK_LIGHTNING,
    TRAINING_FRAMEWORK_PYTORCH,
)
from michelangelo.workflow.variables.types import (
    AssembledModel,
    FeaturePackageArtifact,
    ModelArtifact,
)

if TYPE_CHECKING:
    from michelangelo.lib.artifact_manager.storage_backend import StorageBackend
    from michelangelo.workflow.schema.assembler import TabularAssemblerConfig

__all__ = ["tabular_assembler"]


def tabular_assembler(
    config: TabularAssemblerConfig,
    raw_model: ModelArtifact,
    native_transform_model: ModelArtifact | None = None,
    feature_package: FeaturePackageArtifact | None = None,
    *,
    storage_backend: StorageBackend,
) -> AssembledModel:
    """Assemble a trained tabular model into deployable and raw packages.

    Dispatches to the custom (Python-backend) or PyTorch/Lightning assembler
    based on ``raw_model.metadata.training_framework``, falling back to
    resolving the framework from ``config.model_class`` when the metadata
    field is unset. A custom ``Model`` subclass referenced by
    ``config.model_class`` always routes to the custom path, even when the
    recorded training framework is ``lightning`` — this lets a config
    explicitly force custom packaging of a model whose training framework was
    recorded generically.

    Args:
        config: The assembler configuration.
        raw_model: The trained model to package.
        native_transform_model: Optional native-transform model preceding
            ``raw_model``. Passed through to the custom or PyTorch/Lightning
            assembler; ignored when the framework can't be resolved at all.
        feature_package: Optional feature package preceding ``raw_model``.
            Passed through identically to the custom or PyTorch/Lightning
            assembler; ignored when the framework can't be resolved at all.
        storage_backend: Backend used to download source artifacts and upload
            produced packages. Required, keyword-only — this task boundary is
            an explicit injection point, not a place to silently default to
            throwaway local storage.

    Returns:
        An ``AssembledModel`` with the deployable and raw packaged artifacts.
        When no training framework is recorded, none can be resolved from
        ``config.model_class``, or the recorded framework is unrecognized,
        both ``raw_model`` and ``deployable_model`` are empty placeholder
        artifacts (``path=""``) rather than ``None``, matching this task's
        "always return a pair" contract.
    """
    if (
        raw_model.metadata.training_framework == TRAINING_FRAMEWORK_CUSTOM
        or resolve_training_framework(config.model_class) == TRAINING_FRAMEWORK_CUSTOM
    ):
        return custom_assembler(
            config,
            raw_model,
            native_transform_model,
            feature_package,
            storage_backend=storage_backend,
        )
    if raw_model.metadata.training_framework == TRAINING_FRAMEWORK_PYTORCH:
        return torch_assembler(
            config,
            raw_model,
            feature_package=feature_package,
            storage_backend=storage_backend,
        )
    if raw_model.metadata.training_framework == TRAINING_FRAMEWORK_LIGHTNING:
        return torch_assembler(
            config,
            raw_model,
            native_transform_model,
            feature_package,
            storage_backend=storage_backend,
        )

    return AssembledModel(
        raw_model=ModelArtifact(path=""),
        deployable_model=ModelArtifact(path=""),
    )
