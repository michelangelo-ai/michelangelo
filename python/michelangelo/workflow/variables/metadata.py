"""Typed metadata for model artifacts in Michelangelo workflow tasks."""

from __future__ import annotations

import pickle
from dataclasses import dataclass, field
from io import BytesIO
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from michelangelo.lib.model_manager.schema import ModelSchema


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
        transform_spec: Opaque feature-transform specification propagated
            from a native-transform model through assembly, used to
            reconstruct the transform at serve time. ``None`` when the model
            has no associated transform. See ``_transform_spec``.
        feature_stats: Opaque feature statistics propagated from a
            native-transform model through assembly (e.g. normalization
            parameters). ``None`` when not recorded. See ``_feature_stats``.
        _schema: Serialised (pickled) input/output schema. This, not a live
            ``schema`` field, is what actually crosses a workflow task
            boundary: a live ``ModelSchema`` object passed by value through
            the workflow orchestrator can be large enough to exceed its
            argument-size limits when inlined. Use the ``schema`` property to
            read it back as a live object. Not included in ``repr``.
        _sample_data: Serialised sample inference payload used for
            smoke-testing the deployed model, for the same reason ``_schema``
            is serialised rather than carried live. Use the ``sample_data``
            property to read it back. Not included in ``repr``.
        _transform_spec: Serialised ``transform_spec``, for the same
            live-object-crossing-a-task-boundary reason as ``_schema``. Set
            via the ``transform_spec`` property setter, which pickles the
            value for you. Not included in ``repr``.
        _feature_stats: Serialised ``feature_stats``, for the same reason.
            Set via the ``feature_stats`` property setter. Not included in
            ``repr``.
        _hyperparameters: Serialised (pickled) training hyperparameters. This,
            not a live ``hyperparameters`` field, is what crosses a workflow
            task boundary, for the same workflow-orchestrator argument-size reason
            as ``_schema``. Use the ``hyperparameters`` property to read or
            write it as a live dict. Not included in ``repr``.
        hyperparameters: Live training hyperparameters as a Python dict,
            lazily unpickled from ``_hyperparameters``. Used by
            ``ModelVariable.load_lightning_model()`` to re-instantiate the
            model class via ``model_class(**hyperparameters)``. Has a
            setter: assigning a dict pickles it into ``_hyperparameters``
            for you.

    Warning:
        The ``_schema``, ``_sample_data``, ``_transform_spec``,
        ``_feature_stats``, and ``_hyperparameters`` fields are unpickled on
        read (``schema``, ``sample_data``, ``transform_spec``,
        ``feature_stats``, ``hyperparameters`` properties). Unpickling
        executes arbitrary code embedded in the payload, so only construct
        ``ModelMetadata`` from a trusted source (e.g. your own workflow task
        output), never from unvalidated external input.

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
    _transform_spec: BytesIO | None = field(default=None, repr=False)
    _feature_stats: BytesIO | None = field(default=None, repr=False)
    _hyperparameters: BytesIO | None = field(default=None, repr=False)

    @property
    def schema(self) -> ModelSchema | None:
        """Typed input/output schema, lazily unpickled from ``_schema``.

        Set via ``_schema`` (e.g. ``_schema=io.BytesIO(pickle.dumps(schema))``)
        rather than as a constructor keyword — this stays a read-only view so
        the only thing carried across a workflow task boundary is the
        serialised payload, never a live object passed by value.

        Returns:
            The unpickled ``ModelSchema``, or ``None`` if ``_schema`` is unset.
        """
        if self._schema is None:
            return None
        self._schema.seek(0)
        return pickle.loads(self._schema.read())

    @property
    def sample_data(self) -> list[dict[str, Any]] | None:
        """Sample inference inputs, lazily unpickled from ``_sample_data``.

        Set via ``_sample_data`` for the same reason ``schema`` is backed by
        ``_schema`` — see that property's docstring.

        Returns:
            The unpickled sample data, or ``None`` if ``_sample_data`` is unset.
        """
        if self._sample_data is None:
            return None
        self._sample_data.seek(0)
        return pickle.loads(self._sample_data.read())

    @property
    def transform_spec(self) -> dict[str, Any] | None:
        """Feature-transform spec, lazily unpickled from ``_transform_spec``.

        Unlike ``schema``/``sample_data``, this has a setter: callers may
        assign a live dict directly (``meta.transform_spec = {...}``) and it
        is pickled into ``_transform_spec`` for you, since the value only
        needs to cross a task boundary after assembly, not at construction.

        Returns:
            The unpickled spec, or ``None`` if ``_transform_spec`` is unset.
        """
        if self._transform_spec is None:
            return None
        self._transform_spec.seek(0)
        return pickle.loads(self._transform_spec.read())

    @transform_spec.setter
    def transform_spec(self, value: dict[str, Any] | None) -> None:
        self._transform_spec = None if value is None else BytesIO(pickle.dumps(value))

    @property
    def feature_stats(self) -> dict[str, Any] | None:
        """Feature statistics, lazily unpickled from ``_feature_stats``.

        Has a setter for the same reason ``transform_spec``'s does — see
        that property's docstring.

        Returns:
            The unpickled stats, or ``None`` if ``_feature_stats`` is unset.
        """
        if self._feature_stats is None:
            return None
        self._feature_stats.seek(0)
        return pickle.loads(self._feature_stats.read())

    @feature_stats.setter
    def feature_stats(self, value: dict[str, Any] | None) -> None:
        self._feature_stats = None if value is None else BytesIO(pickle.dumps(value))

    @property
    def hyperparameters(self) -> dict[str, Any] | None:
        """Training hyperparameters, lazily unpickled from ``_hyperparameters``.

        Has a setter for the same reason ``transform_spec``'s does — see
        that property's docstring.

        Returns:
            The unpickled hyperparameters, or ``None`` if
            ``_hyperparameters`` is unset.
        """
        if self._hyperparameters is None:
            return None
        self._hyperparameters.seek(0)
        return pickle.loads(self._hyperparameters.read())

    @hyperparameters.setter
    def hyperparameters(self, value: dict[str, Any] | None) -> None:
        self._hyperparameters = None if value is None else BytesIO(pickle.dumps(value))

    def to_registry_dict(self) -> dict[str, str]:
        """Return a flat string dict of public fields suitable for registry tags.

        Omits ``None``-valued optional fields and serialises ``bool`` fields as
        ``"true"`` / ``"false"`` (lowercase) for consistent cross-registry
        storage. Binary payload fields (``_schema``, ``_sample_data``,
        ``_transform_spec``, ``_feature_stats``, ``_hyperparameters``) are excluded.

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
