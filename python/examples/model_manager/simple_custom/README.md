## Simple custom model packaging example

This folder contains a tiny dummy model (`DummyEchoModel`) and shows how to package it
with `michelangelo.lib.model_manager.packager.custom_triton.CustomTritonPackager`.

### Prerequisites

`poetry install --extras "model-manager"` (numpy, torch, onnx and related packages). No sandbox or GPU is needed; the example runs entirely locally.

### Run (from `python/` directory)

`--out` is required. `--mode` is one of `deployable`, `raw` or `both` (default `both`).

Create **both** packages:

```bash
PYTHONPATH="." poetry run python ./examples/model_manager/simple_custom/package_model.py --out /tmp/mm-simple-custom
```

Only one type:

```bash
PYTHONPATH="." poetry run python ./examples/model_manager/simple_custom/package_model.py --mode deployable --out /tmp/mm-simple-custom
PYTHONPATH="." poetry run python ./examples/model_manager/simple_custom/package_model.py --mode raw --out /tmp/mm-simple-custom
```

### Expected output

The script writes `artifacts/`, plus `deployable/` and/or `raw/` under the `--out` directory, and prints each package's file tree. Look for:

```
Deployable Triton package created at: /tmp/mm-simple-custom/deployable
...
Raw model package created at: /tmp/mm-simple-custom/raw
...
Loaded raw model predicts: {...}
```

The last line only appears when the raw package is built (`--mode raw` or `both`).
