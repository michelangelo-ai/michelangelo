---
sidebar_label: types
title: michelangelo.workflow.variables.types
---

Workflow variable types for artifact storage and push results.

## ModelArtifact Objects

```python
@dataclass
class ModelArtifact()
```

A packaged model artifact ready for upload.

Both the raw model package and the serving-ready deployable artifact are
represented as ``ModelArtifact`` instances. Packaging must be complete
before passing to the pusher — packaging is an assembler-time concern
(e.g. a Ray worker with GPU access).

**Attributes**:

- `path` - Absolute local filesystem path to the packaged artifact file or
  directory.
- `metadata` - Typed metadata forwarded to the model registry at
  registration time. Subclass ``ModelMetadata`` to add
  provider-specific fields.
  

**Example**:

  &gt;&gt;&gt; from michelangelo.workflow.variables.metadata import ModelMetadata
  &gt;&gt;&gt; meta = ModelMetadata(training_framework=&quot;xgboost&quot;, deployable=True)
  &gt;&gt;&gt; artifact = ModelArtifact(path=&quot;/tmp/model&quot;, metadata=meta)
  &gt;&gt;&gt; artifact.metadata.training_framework
  &#x27;xgboost&#x27;

## AssembledModel Objects

```python
@dataclass
class AssembledModel()
```

A trained model transmitted between workflow tasks.

``raw_model`` is required. ``deployable_model`` is optional — omit it for
models that are not packaged for serving (e.g. research checkpoints or
models where ``ModelMetadata.deployable`` is ``False``). When absent,
the pusher skips the deployable upload and sets
``ModelPushResult.deployable_artifact_uri`` to ``None``.

Packaging is the assembler&#x27;s responsibility. The pusher only uploads and
registers pre-packaged artifacts.

**Attributes**:

- ``2 - Raw model package (weights + sample data) intended for
  offline validation and reproducibility.
- ``3 - Optional serving-ready bundle (e.g. Triton config +
  weights) intended for deployment to a model server. ``None`` when
  the model has not been packaged for serving.
  
  Example (with deployable):
  &gt;&gt;&gt; artifact = ModelArtifact(path=&quot;/tmp/model.ubj&quot;)
  &gt;&gt;&gt; assembled = AssembledModel(
  ...     raw_model=artifact,
  ...     deployable_model=artifact,
  ... )
  &gt;&gt;&gt; assembled.raw_model.path
  &#x27;/tmp/model.ubj&#x27;
  
  Example (raw only):
  &gt;&gt;&gt; assembled = AssembledModel(raw_model=ModelArtifact(path=&quot;/tmp/model.ubj&quot;))
  &gt;&gt;&gt; assembled.deployable_model is None
  True

## PusherResult Objects

```python
@dataclass
class PusherResult()
```

The outcome of a single plugin execution.

**Attributes**:

- `name` - Artifact name from ``PusherPluginConfig.name``.
- `plugin` - Plugin name that was invoked (e.g. ``&quot;model_plugin&quot;``).
- `success` - ``True`` if the plugin completed without error.
- `value` - Plugin-specific return data. Empty dict when ``success`` is
  ``False``.
- ``4 - Human-readable error description when ``success`` is ``False``.
  ``None`` when ``success`` is ``True``.
  

**Example**:

  &gt;&gt;&gt; result = PusherResult(
  ...     name=&quot;model&quot;,
  ...     plugin=&quot;model_plugin&quot;,
  ...     success=True,
  ...     value={&quot;model_name&quot;: &quot;clf-v1&quot;, &quot;version&quot;: &quot;1&quot;},
  ... )
  &gt;&gt;&gt; result.success
  True
  &gt;&gt;&gt; result.error is None
  True

