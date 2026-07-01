"""Typed metadata for model artifacts in Michelangelo workflow tasks."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from io import BytesIO


TRAINING_FRAMEWORK_CUSTOM = "custom"
"""Training framework identifier for user-defined ``CustomModel`` subclasses."""

TRAINING_FRAMEWORK_PYTORCH = "pytorch"
"""Training framework identifier for plain ``torch.nn.Module`` models."""

TRAINING_FRAMEWORK_LIGHTNING = "lightning"
"""Training framework identifier for ``pytorch_lightning.LightningModule`` models."""


@dataclass
class ModelMetadata:
    """Typed metadata carried by a model artifact through workflow tasks.

    Captures framework, assembly state, and optional binary payloads so
    downstream tasks (pusher, validator, serving) can make decisions without
    opening the artifact itself.

    Subclass to add provider-specific fields and extend ``to_registry_dict()``
    to include them::

        @dataclass
        class MyModelMetadata(ModelMetadata):
            training_job_id: str | None = None
            experiment_id: str | None = None

            def to_registry_dict(self) -> dict[str, str]:
                result = super().to_registry_dict()
                if self.training_job_id is not None:
                    result["training_job_id"] = self.training_job_id
                if self.experiment_id is not None:
                    result["experiment_id"] = self.experiment_id
                return result

    Attributes:
        training_framework: Name of the training framework (e.g. ``"pytorch"``,
            ``"xgboost"``, ``"huggingface"``). ``None`` when not recorded.
        model_class: Fully-qualified import path of the model class
            (e.g. ``"mypackage.models.Classifier"``). Used to re-instantiate
            the model for validation or fine-tuning. ``None`` when not recorded.
        assembled: ``True`` when the feature-transform and model-inference
            stages have been fused into a single artifact. The pusher uses this
            to decide whether a separate transform upload is needed.
        deployable: ``True`` when the model has been packaged into a
            serving-ready format (e.g. Triton config + weights). The pusher
            sets ``deployable_artifact_uri`` only when this is ``True``.
        is_incremental_training: ``True`` when this model was produced by an
            incremental training run (BASELINE or continuation of an existing
            incremental chain). Used by downstream tasks to propagate chain
            metadata.
        baseline_model_identifier: Opaque string tag identifying the original
            baseline model at the root of an incremental training chain.
            ``None`` for non-incremental models, and for the first run of a new
            incremental chain (the BASELINE run itself). Set on continuation
            runs to the identifier of the original baseline.
        _schema: Serialised input/output schema (e.g. protobuf or JSON bytes).
            Not included in ``repr`` to avoid flooding logs.
        _sample_data: Serialised sample inference payload used for smoke-testing
            the deployed model. Not included in ``repr``.
        _hyperparameters: Serialised training hyperparameters for
            reproducibility. Not included in ``repr``.
        hyperparameters: Live training hyperparameters as a Python dict.
            Used by ``ModelVariable.load_lightning_model()`` to re-instantiate
            the model class via ``model_class(**hyperparameters)``. Distinct
            from ``_hyperparameters``, which is the registry-bound serialised
            form.

    Example:
        >>> meta = ModelMetadata(training_framework="xgboost", deployable=True)
        >>> meta.training_framework
        'xgboost'
        >>> meta.deployable
        True
    """

    training_framework: str | None = None
    model_class: str | None = None
    assembled: bool = False
    deployable: bool = False
    is_incremental_training: bool = False
    baseline_model_identifier: str | None = None
    _schema: BytesIO | None = field(default=None, repr=False)
    _sample_data: BytesIO | None = field(default=None, repr=False)
    _hyperparameters: BytesIO | None = field(default=None, repr=False)
    hyperparameters: dict[str, Any] | None = None

    def to_registry_dict(self) -> dict[str, str]:
        """Return a flat string dict of public fields suitable for registry tags.

        Omits ``None``-valued optional fields and serialises ``bool`` fields as
        ``"true"`` / ``"false"`` (lowercase) for consistent cross-registry
        storage. Binary payload fields (``_schema``, ``_sample_data``,
        ``_hyperparameters``) are excluded.

        Subclasses should override this method to include their own fields::

            @dataclass
            class MyModelMetadata(ModelMetadata):
                training_job_id: str | None = None

                def to_registry_dict(self) -> dict[str, str]:
                    result = super().to_registry_dict()
                    if self.training_job_id is not None:
                        result["training_job_id"] = self.training_job_id
                    return result

        Returns:
            A ``dict[str, str]`` ready for ``ModelRegistryClient.register_model(
            metadata=...)``.
        """
        result: dict[str, str] = {}
        if self.training_framework is not None:
            result["training_framework"] = self.training_framework
        if self.model_class is not None:
            result["model_class"] = self.model_class
        result["assembled"] = str(self.assembled).lower()
        result["deployable"] = str(self.deployable).lower()
        result["is_incremental_training"] = str(self.is_incremental_training).lower()
        if self.baseline_model_identifier is not None:
            result["baseline_model_identifier"] = self.baseline_model_identifier
        return result
