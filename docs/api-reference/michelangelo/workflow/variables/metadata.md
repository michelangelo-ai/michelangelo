---
sidebar_label: metadata
title: michelangelo.workflow.variables.metadata
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
to include them::

@dataclass
class MyModelMetadata(ModelMetadata):
training_job_id: str | None = None
experiment_id: str | None = None

def to_registry_dict(self) -&gt; dict[str, str]:
result = super().to_registry_dict()
if self.training_job_id is not None:
result[&quot;training_job_id&quot;] = self.training_job_id
if self.experiment_id is not None:
result[&quot;experiment_id&quot;] = self.experiment_id
return result

**Attributes**:

- `training_framework` - Name of the training framework (e.g. ``&quot;pytorch&quot;``,
  ``&quot;xgboost&quot;``, ``&quot;huggingface&quot;``). ``None`` when not recorded.
- ``1 - Fully-qualified import path of the model class
  (e.g. ``&quot;mypackage.models.Classifier&quot;``). Used to re-instantiate
  the model for validation or fine-tuning. ``None`` when not recorded.
- ``6 - ``True`` when the feature-transform and model-inference
  stages have been fused into a single artifact. The pusher uses this
  to decide whether a separate transform upload is needed.
- ``9 - ``True`` when the model has been packaged into a
  serving-ready format (e.g. Triton config + weights). The pusher
  sets ``deployable_artifact_uri`` only when this is ``True``.
- `training_framework`6 - ``True`` when this model was produced by an
  incremental training run (BASELINE or continuation of an existing
  incremental chain). Used by downstream tasks to propagate chain
  metadata.
- `training_framework`9 - Opaque string tag identifying the original
  baseline model at the root of an incremental training chain.
  ``None`` for non-incremental models, and for the first run of a new
  incremental chain (the BASELINE run itself). Set on continuation
  runs to the identifier of the original baseline.
- ``2 - Serialised input/output schema (e.g. protobuf or JSON bytes).
  Not included in ``repr`` to avoid flooding logs.
- ``5 - Serialised sample inference payload used for smoke-testing
  the deployed model. Not included in ``repr``.
- ``8 - Serialised training hyperparameters for
  reproducibility. Not included in ``repr``.
- ``1 - Live training hyperparameters as a Python dict.
  Used by ``ModelVariable.load_lightning_model()`` to re-instantiate
  the model class via ``model_class(**hyperparameters)``. Distinct
  from ``_hyperparameters``, which is the registry-bound serialised
  form.
  

**Example**:

  &gt;&gt;&gt; meta = ModelMetadata(training_framework=&quot;xgboost&quot;, deployable=True)
  &gt;&gt;&gt; meta.training_framework
  &#x27;xgboost&#x27;
  &gt;&gt;&gt; meta.deployable
  True

#### to\_registry\_dict

```python
def to_registry_dict() -> dict[str, str]
```

Return a flat string dict of public fields suitable for registry tags.

Omits ``None``-valued optional fields and serialises ``bool`` fields as
``&quot;true&quot;`` / ``&quot;false&quot;`` (lowercase) for consistent cross-registry
storage. Binary payload fields (``_schema``, ``_sample_data``,
``_hyperparameters``) are excluded.

Subclasses should override this method to include their own fields::

@dataclass
class MyModelMetadata(ModelMetadata):
training_job_id: str | None = None

def to_registry_dict(self) -&gt; dict[str, str]:
result = super().to_registry_dict()
if self.training_job_id is not None:
result[&quot;training_job_id&quot;] = self.training_job_id
return result

**Returns**:

  A ``dict[str, str]`` ready for ``ModelRegistryClient.register_model(
  metadata=...)``.

