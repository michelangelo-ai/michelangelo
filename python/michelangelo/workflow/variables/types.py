"""Workflow variable types for artifact storage and push results."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any

from michelangelo.workflow.variables.metadata import ModelMetadata


@dataclass
class ModelArtifact:
    """A packaged model artifact ready for upload.

    Both the raw model package and the serving-ready deployable artifact are
    represented as ``ModelArtifact`` instances. Packaging must be complete
    before passing to the pusher — packaging is an assembler-time concern
    (e.g. a Ray worker with GPU access).

    Attributes:
        path: Absolute local filesystem path to the packaged artifact file or
            directory.
        metadata: Typed metadata forwarded to the model registry at
            registration time. Subclass ``ModelMetadata`` to add
            provider-specific fields.

    Example:
        >>> from michelangelo.workflow.variables.metadata import ModelMetadata
        >>> meta = ModelMetadata(training_framework="xgboost", deployable=True)
        >>> artifact = ModelArtifact(path="/tmp/model", metadata=meta)
        >>> artifact.metadata.training_framework
        'xgboost'
    """

    path: str
    metadata: ModelMetadata = field(default_factory=ModelMetadata)


@dataclass
class AssembledModel:
    """A trained model transmitted between workflow tasks.

    Both artifacts must be fully packaged before passing to the pusher.
    Packaging is the assembler's responsibility. The pusher only uploads and
    registers pre-packaged artifacts.

    Attributes:
        raw_model: Raw model package (weights + sample data) intended for
            offline validation and reproducibility.
        deployable_model: Serving-ready bundle (e.g. Triton config + weights)
            intended for deployment to a model server.

    Example:
        >>> artifact = ModelArtifact(path="/tmp/model.ubj")
        >>> assembled = AssembledModel(
        ...     raw_model=artifact,
        ...     deployable_model=artifact,
        ... )
        >>> assembled.raw_model.path
        '/tmp/model.ubj'
    """

    raw_model: ModelArtifact
    deployable_model: ModelArtifact


@dataclass
class PusherResult:
    """The outcome of a single plugin execution.

    Attributes:
        name: Artifact name from ``PusherPluginConfig.name``.
        plugin: Plugin name that was invoked (e.g. ``"model_plugin"``).
        success: ``True`` if the plugin completed without error.
        value: Plugin-specific return data. Empty dict when ``success`` is
            ``False``.
        error: Human-readable error description when ``success`` is ``False``.
            ``None`` when ``success`` is ``True``.

    Example:
        >>> result = PusherResult(
        ...     name="model",
        ...     plugin="model_plugin",
        ...     success=True,
        ...     value={"model_name": "clf-v1", "version": "1"},
        ... )
        >>> result.success
        True
        >>> result.error is None
        True
    """

    name: str
    plugin: str
    success: bool
    value: dict[str, Any] = field(default_factory=dict)
    error: str | None = None
