## Simple torch model packaging example

This folder contains a tiny torch-backed model (`TorchLinearModel`) and shows how to
package it with `michelangelo.lib.model_manager.packager.custom_triton.CustomTritonPackager`.

The model still uses **numpy ndarrays** for inputs/outputs (as required by the Model
interface + packager validation), but converts to/from torch internally.

### Run (from `python/` directory)

```bash
PYTHONPATH="." poetry run python ./examples/model_manager/simple_custom_torch/package_model.py --out /tmp/mm-simple-torch
```


