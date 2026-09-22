---
sidebar_label: types
title: workflow.variables.types
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

  >>> from michelangelo.workflow.variables.metadata import ModelMetadata
  >>> meta = ModelMetadata(training_framework="xgboost", deployable=True)
  >>> artifact = ModelArtifact(path="/tmp/model", metadata=meta)
  >>> artifact.metadata.training_framework
  'xgboost'

## FeaturePackageArtifact Objects

```python
@dataclass
class FeaturePackageArtifact()
```

A feature package preceding a model's feature-computation stage.

Assemblers fuse this into the model's own schema/sample data to produce
the end-to-end serving contract (see ``fuse_e2e_schema``,
``build_e2e_sample_data``).

**Attributes**:

- `path` - Absolute local filesystem path to the feature package.
- `metadata` - Typed metadata describing the feature package's schema and
  sample data.
  

**Example**:

  >>> from michelangelo.workflow.variables.metadata import (
  ...     FeaturePackageMetadata,
  ... )
  >>> package = FeaturePackageArtifact(
  ...     path="/tmp/features", metadata=FeaturePackageMetadata()
  ... )
  >>> package.metadata.schema
  FeatureSchema(input_schema=[], feature_store_features_schema=[],
  derived_features_schema=[])

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

Packaging is the assembler's responsibility. The pusher only uploads and
registers pre-packaged artifacts.

**Attributes**:

- `raw_model` - Raw model package (weights + sample data) intended for
  offline validation and reproducibility.
- `deployable_model` - Optional serving-ready bundle (e.g. Triton config +
  weights) intended for deployment to a model server. ``None`` when
  the model has not been packaged for serving.
- `feature_package` - Optional feature package fused into the deployable
  model's end-to-end schema/sample data during assembly. ``None``
  when the model has no upstream feature-computation stage.
  
  Example (with deployable):
  >>> artifact = ModelArtifact(path="/tmp/model.ubj")
  >>> assembled = AssembledModel(
  ...     raw_model=artifact,
  ...     deployable_model=artifact,
  ... )
  >>> assembled.raw_model.path
  '/tmp/model.ubj'
  
  Example (raw only):
  >>> assembled = AssembledModel(raw_model=ModelArtifact(path="/tmp/model.ubj"))
  >>> assembled.deployable_model is None
  True

## NativeTransformResult Objects

```python
@dataclass
class NativeTransformResult()
```

The result of the native transform task (``tabular_native_transform``).

Contains the transformed datasets and the PyTorch transform module
(model). For incremental-training flows, the transform spec and feature
stats are stored on ``model.metadata`` (``transform_spec`` and
``feature_stats``) so downstream tasks (assembler, pusher) can persist
them as a transform checkpoint for a future incremental run.

**Attributes**:

- `transformed_datasets` - Mapping of dataset name (e.g. ``"train"``,
  ``"validation"``, ``"test"``) to its transformed
  ``DatasetVariable``. Datasets that had no transform spec applied
  (empty inputs) are passed through unchanged.
- `model` - The materialized transform module, wrapped as a
  ``ModelVariable``, or ``None`` when the transform spec produced
  no layers (e.g. an empty spec).
  

**Example**:

  >>> result = NativeTransformResult(
  ...     transformed_datasets={"train": DatasetVariable.create(None)},
  ... )
  >>> result.model is None
  True

## PusherResult Objects

```python
@dataclass
class PusherResult()
```

The outcome of a single plugin execution.

**Attributes**:

- `name` - Artifact name from ``PusherPluginConfig.name``.
- `plugin` - Plugin name that was invoked (e.g. ``"model_plugin"``).
- `success` - ``True`` if the plugin completed without error.
- `value` - Plugin-specific return data. Empty dict when ``success`` is
  ``False``.
- `error` - Human-readable error description when ``success`` is ``False``.
  ``None`` when ``success`` is ``True``.
  

**Example**:

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

