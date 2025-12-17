## Simple custom model packaging example

This folder contains a tiny dummy model (`DummyEchoModel`) and shows how to package it
with `michelangelo.lib.model_manager.packager.custom_triton.CustomTritonPackager`.

### Run (from `python/` directory)

Create **both** packages:

```bash
PYTHONPATH="." poetry run python ./examples/model_manager/simple_custom/package_model.py --out /tmp/mm-simple-custom
```

Only one type:

```bash
PYTHONPATH="." poetry run python ./examples/model_manager/simple_custom/package_model.py --mode deployable --out /tmp/mm-simple-custom
PYTHONPATH="." poetry run python ./examples/model_manager/simple_custom/package_model.py --mode raw --out /tmp/mm-simple-custom
```


