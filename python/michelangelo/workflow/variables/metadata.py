"""Typed metadata for model artifacts in CanvasFlex workflow tasks."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from io import BytesIO


@dataclass
class ModelMetadata:
    """Typed metadata carried by a model artifact through workflow tasks.

    Captures framework, assembly state, and optional binary payloads so
    downstream tasks (pusher, validator, serving) can make decisions without
    opening the artifact itself.

    Subclass to add provider-specific fields::

        @dataclass
        class UberModelMetadata(ModelMetadata):
            training_job_id: str | None = None
            experiment_id: str | None = None

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
        _schema: Serialised input/output schema (e.g. protobuf or JSON bytes).
            Not included in ``repr`` to avoid flooding logs.
        _sample_data: Serialised sample inference payload used for smoke-testing
            the deployed model. Not included in ``repr``.
        _hyperparameters: Serialised training hyperparameters for
            reproducibility. Not included in ``repr``.

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
    _schema: BytesIO | None = field(default=None, repr=False)
    _sample_data: BytesIO | None = field(default=None, repr=False)
    _hyperparameters: BytesIO | None = field(default=None, repr=False)
