## Custom Triton packaging example (`CustomTritonPackager`)

This example shows how to package a tiny custom Python model for Triton using:

- `CustomTritonPackager.create_model_package(...)` (**deployable Triton model repo layout**)
- `CustomTritonPackager.create_raw_model_package(...)` (**raw package with sample data + validation**)

The toy model is `ToyAddBiasModel` in `toy_model.py` (implements the `Model` interface).

### Run (from `python/` directory)

Create **both** packages (default; prints the output file tree):

```bash
PYTHONPATH="." poetry run python ./examples/model_manager/custom_triton_packager/package_model.py
```

Keep outputs under a directory you choose:

```bash
PYTHONPATH="." poetry run python ./examples/model_manager/custom_triton_packager/package_model.py --out /tmp/mm-packages
```

Create only one type:

```bash
PYTHONPATH="." poetry run python ./examples/model_manager/custom_triton_packager/package_model.py --mode deployable --out /tmp/mm-packages
PYTHONPATH="." poetry run python ./examples/model_manager/custom_triton_packager/package_model.py --mode raw --out /tmp/mm-packages
```


