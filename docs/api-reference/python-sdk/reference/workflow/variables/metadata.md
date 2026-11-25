---
sidebar_label: metadata
title: workflow.variables.metadata
---

Typed metadata for model artifacts in Michelangelo workflow tasks.

#### TRAINING\_FRAMEWORK\_CUSTOM

Training framework identifier for user-defined ``CustomModel`` subclasses.

#### TRAINING\_FRAMEWORK\_PYTORCH

Training framework identifier for plain ``torch.nn.Module`` models.

#### TRAINING\_FRAMEWORK\_LIGHTNING

Training framework identifier for ``pytorch_lightning.LightningModule`` models.

## ModelMetadata Objects

```python
@dataclass
class ModelMetadata()
```

Typed metadata carried by a model artifact through workflow tasks.

Captures framework, assembly state, and optional binary payloads so
downstream tasks (pusher, validator, serving) can make decisions without
opening the artifact itself.

Subclass to add provider-specific fields and extend ``to_registry_dict()``
to include them:

```python
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
```

**Attributes**:

- `training_framework` - Name of the training framework (e.g. ``"pytorch"``,
  ``"xgboost"``, ``"huggingface"``). ``None`` when not recorded.
- `model_class` - Fully-qualified import path of the model class
  (e.g. ``"mypackage.models.Classifier"``). Used to re-instantiate
  the model for validation or fine-tuning. ``None`` when not recorded.
- `assembled` - ``True`` when the feature-transform and model-inference
  stages have been fused into a single artifact. The pusher uses this
  to decide whether a separate transform upload is needed.
- `deployable` - ``True`` when the model has been packaged into a
  serving-ready format (e.g. Triton config + weights). The pusher
  sets ``deployable_artifact_uri`` only when this is ``True``.
- `is_incremental_training` - ``True`` when this model was produced by an
  incremental training run (BASELINE or continuation of an existing
  incremental chain). Used by downstream tasks to propagate chain
  metadata.
- `baseline_model_identifier` - Opaque string tag identifying the original
  baseline model at the root of an incremental training chain.
  ``None`` for non-incremental models, and for the first run of a new
  incremental chain (the BASELINE run itself). Set on continuation
  runs to the identifier of the original baseline.
- `transform_spec` - Opaque feature-transform specification propagated
  from a native-transform model through assembly, used to
  reconstruct the transform at serve time. ``None`` when the model
  has no associated transform. See ``_transform_spec``.
- `feature_stats` - Opaque feature statistics propagated from a
  native-transform model through assembly (e.g. normalization
  parameters). ``None`` when not recorded. See ``_feature_stats``.
- `_schema` - Serialised (pickled) input/output schema. This, not a live
  ``schema`` field, is what actually crosses a workflow task
- `boundary` - a live ``ModelSchema`` object passed by value through
  the workflow orchestrator can be large enough to exceed its
  argument-size limits when inlined. Use the ``schema`` property to
  read it back as a live object. Not included in ``repr``.
- `_sample_data` - Serialised sample inference payload used for
  smoke-testing the deployed model, for the same reason ``_schema``
  is serialised rather than carried live. Use the ``sample_data``
  property to read it back. Not included in ``repr``.
- `_transform_spec` - Serialised ``transform_spec``, for the same
  live-object-crossing-a-task-boundary reason as ``_schema``. Set
  via the ``transform_spec`` property setter, which pickles the
  value for you. Not included in ``repr``.
- `_feature_stats` - Serialised ``feature_stats``, for the same reason.
  Set via the ``feature_stats`` property setter. Not included in
  ``repr``.
- `_hyperparameters` - Serialised (pickled) training hyperparameters. This,
  not a live ``hyperparameters`` field, is what crosses a workflow
  task boundary, for the same workflow-orchestrator argument-size reason
  as ``_schema``. Use the ``hyperparameters`` property to read or
  write it as a live dict. Not included in ``repr``.
- `hyperparameters` - Live training hyperparameters as a Python dict,
  lazily unpickled from ``_hyperparameters``. Used by
  ``ModelVariable.load_lightning_model()`` to re-instantiate the
  model class via ``model_class(**hyperparameters)``. Has a
- `setter` - assigning a dict pickles it into ``_hyperparameters``
  for you.
- `_e2e_schema` - Serialised end-to-end schema produced by fusing a
  ``FeaturePackageArtifact``'s schema into this model's own
  ``schema`` during assembly, for the same
  live-object-crossing-a-task-boundary reason as ``_schema``. Use
  the ``e2e_schema`` property to read it back. Not included in
  ``repr``.
- `_e2e_sample_data` - Serialised end-to-end sample data produced the
  same way as ``_e2e_schema``. Use the ``e2e_sample_data``
  property to read it back. Not included in ``repr``.
  

**Warnings**:

  The ``_schema``, ``_sample_data``, ``_transform_spec``,
  ``_feature_stats``, ``_hyperparameters``, ``_e2e_schema``, and
  ``_e2e_sample_data`` fields are unpickled on read (``schema``,
  ``sample_data``, ``transform_spec``, ``feature_stats``,
  ``hyperparameters``, ``e2e_schema``, ``e2e_sample_data``
  properties). Unpickling
  executes arbitrary code embedded in the payload, so only construct
  ``ModelMetadata`` from a trusted source (e.g. your own workflow task
  output), never from unvalidated external input.
  

**Example**:

```python
>>> meta = ModelMetadata(training_framework="xgboost", deployable=True)
>>> meta.training_framework
'xgboost'
>>> meta.deployable
True
```

#### schema

```python
@property
def schema() -> ModelSchema | None
```

Typed input/output schema, lazily unpickled from ``_schema``.

Set via ``_schema`` (e.g. ``_schema=io.BytesIO(pickle.dumps(schema))``)
rather than as a constructor keyword — this stays a read-only view so
the only thing carried across a workflow task boundary is the
serialised payload, never a live object passed by value.

**Returns**:

  The unpickled ``ModelSchema``, or ``None`` if ``_schema`` is unset.

#### sample\_data

```python
@property
def sample_data() -> list[dict[str, Any]] | None
```

Sample inference inputs, lazily unpickled from ``_sample_data``.

Set via ``_sample_data`` for the same reason ``schema`` is backed by
``_schema`` — see that property's docstring.

**Returns**:

  The unpickled sample data, or ``None`` if ``_sample_data`` is unset.

#### transform\_spec

```python
@property
def transform_spec() -> dict[str, Any] | None
```

Feature-transform spec, lazily unpickled from ``_transform_spec``.

Unlike ``schema``/``sample_data``, this has a setter: callers may
assign a live dict directly (``meta.transform_spec = {...}``) and it
is pickled into ``_transform_spec`` for you, since the value only
needs to cross a task boundary after assembly, not at construction.

**Returns**:

  The unpickled spec, or ``None`` if ``_transform_spec`` is unset.

#### feature\_stats

```python
@property
def feature_stats() -> dict[str, Any] | None
```

Feature statistics, lazily unpickled from ``_feature_stats``.

Has a setter for the same reason ``transform_spec``'s does — see
that property's docstring.

**Returns**:

  The unpickled stats, or ``None`` if ``_feature_stats`` is unset.

#### hyperparameters

```python
@property
def hyperparameters() -> dict[str, Any] | None
```

Training hyperparameters, lazily unpickled from ``_hyperparameters``.

Has a setter for the same reason ``transform_spec``'s does — see
that property's docstring.

**Returns**:

  The unpickled hyperparameters, or ``None`` if
  ``_hyperparameters`` is unset.

#### e2e\_schema

```python
@property
def e2e_schema() -> ModelSchema | None
```

End-to-end schema, lazily unpickled from ``_e2e_schema``.

Set when a feature package is fused into this model during assembly
(see ``fuse_e2e_schema``); read-only for the same reason ``schema``
is, via the same ``_schema``-backing pattern.

**Returns**:

  The unpickled ``ModelSchema``, or ``None`` if ``_e2e_schema`` is unset.

#### e2e\_sample\_data

```python
@property
def e2e_sample_data() -> list[dict[str, Any]] | None
```

End-to-end sample data, lazily unpickled from ``_e2e_sample_data``.

Set when a feature package is fused into this model during assembly
(see ``build_e2e_sample_data``); read-only, mirroring ``sample_data``.

**Returns**:

  The unpickled sample data, or ``None`` if ``_e2e_sample_data`` is unset.

#### to\_registry\_dict

```python
def to_registry_dict() -> dict[str, str]
```

Return a flat string dict of public fields suitable for registry tags.

Omits ``None``-valued optional fields and serialises ``bool`` fields as
``"true"`` / ``"false"`` (lowercase) for consistent cross-registry
storage. Binary payload fields (``_schema``, ``_sample_data``,
``_transform_spec``, ``_feature_stats``, ``_hyperparameters``) are excluded.

Subclasses should override this method to include their own fields:

```python
@dataclass
class MyModelMetadata(ModelMetadata):
    training_job_id: str | None = None

    def to_registry_dict(self) -> dict[str, str]:
        result = super().to_registry_dict()
        if self.training_job_id is not None:
            result["training_job_id"] = self.training_job_id
        return result
```

**Returns**:

  A ``dict[str, str]`` ready for ``ModelRegistryClient.register_model(
  metadata=...)``.

## DatasetMetadata Objects

```python
@dataclass
class DatasetMetadata()
```

Typed metadata carried by a ``DatasetVariable`` through workflow tasks.

Currently scoped to the one field ``tabular_native_transform`` needs to
record; extend with additional fields as more tasks need to annotate
dataset variables (mirroring how ``ModelMetadata`` grew field-by-field
per task).

**Attributes**:

- `derived_features` - Names of output columns present in the dataset
  that were not present in its original input (i.e. produced by a
  transform), in sorted order. Empty when no transform was
  applied, or when every output column overlaps an input column
  name.
  

**Example**:

```python
>>> meta = DatasetMetadata(derived_features=["scaled_amount"])
>>> meta.derived_features
['scaled_amount']
```

## FeaturePackageMetadata Objects

```python
@dataclass
class FeaturePackageMetadata()
```

Typed metadata carried by a feature-package artifact.

A feature package describes the schema and sample data produced by a
feature-computation stage (e.g. a feature store lookup or a batch feature
pipeline) that precedes a model. Assemblers fuse this into the model's
own schema/sample data to produce the end-to-end serving contract; see
``fuse_e2e_schema`` and ``build_e2e_sample_data``.

**Attributes**:

- `_schema` - Serialised (pickled) ``FeatureSchema``, for the same
  live-object-crossing-a-task-boundary reason as
  ``ModelMetadata._schema``. Use the ``schema`` property to read it
  back. Not included in ``repr``.
- `_sample_data` - Serialised sample feature payload, for the same reason
  ``_schema`` is serialised. Use the ``sample_data`` property to
  read it back. Not included in ``repr``.
  

**Warnings**:

  The ``_schema`` and ``_sample_data`` fields are unpickled on read.
  Unpickling executes arbitrary code embedded in the payload, so only
  construct ``FeaturePackageMetadata`` from a trusted source, never
  from unvalidated external input.

#### schema

```python
@property
def schema() -> FeatureSchema
```

Typed feature schema, lazily unpickled from ``_schema``.

**Returns**:

  The unpickled ``FeatureSchema``, or an empty ``FeatureSchema()``
  if ``_schema`` is unset.

#### sample\_data

```python
@property
def sample_data() -> list[dict[str, Any]] | None
```

Sample feature inputs, lazily unpickled from ``_sample_data``.

**Returns**:

  The unpickled sample data, or ``None`` if ``_sample_data`` is unset.

## SourceMessageType Objects

```python
class SourceMessageType(Enum)
```

The type of value a ``MessageVariable`` holds.

## MessageMetadata Objects

```python
@dataclass
class MessageMetadata()
```

Metadata of a ``MessageVariable``.

