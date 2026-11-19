## Simple torch model packaging example

This folder contains a tiny torch-backed model (`TorchLinearModel`) and shows how to
package it with `michelangelo.lib.model_manager.packager.custom_triton.CustomTritonPackager`.

The model still uses **numpy ndarrays** for inputs/outputs (as required by the Model
interface + packager validation), but converts to/from torch internally.

### Prerequisites

`poetry install --extras "model-manager"` (includes torch and numpy; the model raises an import error without torch). No sandbox or GPU is needed.

### Run (from `python/` directory)

`--out` is required. `--mode` is one of `deployable`, `raw` or `both` (default `both`).

```bash
PYTHONPATH="." poetry run python ./examples/model_manager/simple_custom_torch/package_model.py --out /tmp/mm-simple-torch
```

### Expected output

The script writes `artifacts/`, `deployable/` and `raw/` under the `--out` directory and prints each package's file tree. Look for:

```
Deployable Triton package created at: /tmp/mm-simple-torch/deployable
...
Raw model package created at: /tmp/mm-simple-torch/raw
...
Loaded raw model predicts: {...}
```
