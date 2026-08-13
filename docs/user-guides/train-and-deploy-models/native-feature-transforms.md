---
sidebar_position: 4
---

# Native Feature Transforms

This guide covers `michelangelo.lib.native_transform` — a library of PyTorch
feature-transform layers that run identically at training time and at
serving time, so the exact same code that scaled, bucketized, or tokenized a
column during training also runs inside the served model.

## What it does

A native transform is described declaratively as a **`TransformSpec`**: a DAG
of layer specs (`Concatenate`, `Cast`, `Bucketization`, `StandardScaler`, and
others) parsed from a plain dict or YAML file. Some specs are **fitted
placeholders** — for example `StandardScaler` needs a column's mean and
standard deviation before it can run — and are resolved into a concrete,
executable spec once those statistics are computed from the training data.

The typical lifecycle is:

1. **Fit** — ask the spec what statistics its placeholders need
   (`get_numerical_statistics_computation_specs`), compute them over the
   training dataset, and hydrate the placeholders (`update_*` methods).
2. **Materialize** — turn the fitted spec into a single executable
   `torch.nn.Module` (`get_transform_module`).
3. **Export** — the module is `torch.jit.script`-compatible, so it can be
   embedded directly in a served model artifact.

## How to use it

### Define a spec

```python
from michelangelo.lib.native_transform.torch import TransformSpec

raw_spec = {
    "transform_specs": [
        {
            "transform_name": "StandardScaler",
            "input_cols": ["age"],
            "output_cols": ["age_scaled"],
        },
        {
            "transform_name": "MinMaxScaler",
            "input_cols": ["price"],
            "output_cols": ["price_scaled"],
        },
    ]
}
spec = TransformSpec(raw_transform_specs=raw_spec)
```

`raw_transform_specs` can also be loaded from a YAML file via
`TransformSpec(transform_spec_yaml_path="spec.yaml")`.

### Fit statistics and hydrate placeholders

```python
# Ask the spec what statistics its placeholders need.
stats_needed = spec.get_numerical_statistics_computation_specs()
# {"age": {..., "mean": True, ...}, "price": {..., "min": True, "max": True, ...}}

# Compute these over your training dataset (Ray, Spark, pandas — whatever
# fits your pipeline), then hydrate the placeholders in place.
fitted_stats = {
    "age_mean": 35.2,
    "age_std": 12.1,
    "price_min": 0.0,
    "price_max": 500.0,
}
spec.update_standard_scaler_specs(fitted_stats)
spec.update_min_max_scaler_specs(fitted_stats)
```

### Persist the fitted spec as a workflow value

Once fitted, a `TransformSpec` is a plain value you'll want to pass between
pipeline stages (e.g. from a "fit transforms" task to a "train" task) or
persist alongside a model artifact. `TransformSpecIO` adapts `TransformSpec`
to Uniflow's [`default_io`](../reference/type-system.md) registry, so it
round-trips through `write`/`read` the same way a `DataFrame` or `Dataset`
does:

```python
from michelangelo.lib.native_transform.torch.io import TransformSpecIO

io = TransformSpecIO()
io.write("s3://bucket/run-id/transform_spec.json", spec)

# ... later, in a different task or process:
restored_spec = io.read("s3://bucket/run-id/transform_spec.json", None)
```

To have any Uniflow task that declares a `TransformSpec`-typed input or
output get this serialization for free, without calling `TransformSpecIO`
directly, import the plugin package to register it in `default_io`:

```python
import michelangelo.uniflow.plugins.native_transform  # noqa: F401  (registers TransformSpec with default_io)
```

See [Type System](../reference/type-system.md) for how task I/O typing
works.

### Materialize and run

```python
import torch
from michelangelo.lib.native_transform.torch import get_transform_module

module = get_transform_module(restored_spec, start_level=0)

outputs = module({
    "age": torch.tensor([20.0, 35.0, 50.0]),
    "price": torch.tensor([100.0, 250.0, 500.0]),
})
# {"age_scaled": tensor([...]), "price_scaled": tensor([...])}

# TorchScript-export for serving.
scripted_module = torch.jit.script(module)
```

`start_level`/`end_level` let you materialize a subset of the DAG's
dependency levels — useful when some transforms run upstream (e.g. in a Ray
preprocessing task) and others run inside the served model.

## How to extend it

Adding a new transform layer requires three pieces, all in
`michelangelo.lib.native_transform.torch`:

1. A pydantic **layer spec** in `transform_layer_spec.py`, subclassing
   `TorchTransformLayerSpec`. This is the declarative, serializable
   description of the layer's configuration.
2. An executable **layer** in `base_layers.py` (or a new module), subclassing
   `TorchTransformBaseLayer` — a `torch.nn.Module` whose constructor accepts
   the same fields as the spec (`to_transform_layers` calls
   `LayerClass(**layer_spec.model_dump())`).
3. Registration of both in `transform_spec.py`'s `TORCH_TRANSFORM_LAYERS_DICT`
   (spec class name → layer class) and `TORCH_TRANSFORM_LAYERS_SPECS_DICT`
   (the raw spec dict's `transform_name` → spec class).

If the new layer needs fitted statistics (like `StandardScaler` or
`MinMaxScaler`), add a placeholder spec plus an `update_*` hydration method on
`TransformSpec` that resolves it into a concrete spec, following the pattern
of `update_standard_scaler_specs`/`update_min_max_scaler_specs`.

To support persisting a *new* transform-related type as a workflow value the
way `TransformSpecIO` does for `TransformSpec`, implement the
`IO[T]` protocol (`write`/`read`) and register it with
`default_io[YourType] = YourTypeIO` in a `michelangelo.uniflow.plugins.*`
package, matching the pattern in
`michelangelo/uniflow/plugins/native_transform/io.py`.
