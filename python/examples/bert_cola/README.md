# BERT CoLA Fine-tuning Demo

Fine-tuning BERT for linguistic acceptability classification using the Corpus of Linguistic Acceptability (CoLA) task from the GLUE benchmark. Demonstrates sequence classification with distributed training on Ray.

## Features

- **Pre-trained Model**: BERT base model fine-tuning
- **GLUE Benchmark**: CoLA task for grammatical acceptability
- **Distributed Training**: Ray-based execution
- **HuggingFace Integration**: Uses transformers and datasets libraries
- **Evaluation Metrics**: Matthews correlation coefficient and accuracy

## How to Run

```bash
cd michelangelo-ai/michelangelo/python
poetry install -E example   # installs torch, transformers, datasets, ray
source .venv/bin/activate
poetry run python examples/bert_cola/bert_cola.py
```

> The `example` extra is required. A plain `poetry install` omits it, and the module then fails to import with `No module named 'datasets'`.

## Expected Output

```
Loading dataset from GLUE/CoLA...
Training BERT model...
Epoch 1/3: loss=0.512, accuracy=0.85
Epoch 2/3: loss=0.312, accuracy=0.89
Epoch 3/3: loss=0.201, accuracy=0.92
train_result: TrainOutput(...)
assembled model: AssembledModel(raw_model=..., deployable_model=...)
push results: [PusherResult(name='model', plugin='model_plugin', success=True, ...)]
ok.
```

After training, `assembler` packages the fine-tuned checkpoint into a
deployable Triton package + a raw package (via the custom, Python-backend
assembler path -- BERT's multi-input `forward()` isn't TorchScript-friendly),
and `push_step` registers it in a model registry (`InMemoryRegistryClient` by
default; set `REGISTRY_ENDPOINT` for a real registry). Set `AWS_ENDPOINT_URL`
to push to MinIO/S3-compatible storage instead of a local temp directory.

## Deploying to Triton

`inferenceserver/` holds this example's own Triton serving image and deploy
manifests -- each project owns its own serving image, built from whatever
deps its models actually need, rather than relying on a single shared
default. `inferenceserver/Dockerfile` adds torch/transformers (needed by the
custom python-backend package `assembler.py` produces) on top of the stock
`nvcr.io/nvidia/tritonserver` image; `.github/workflows/build-example-inferenceserver-images.yaml`
discovers and builds every example's `inferenceserver/Dockerfile` (not just
this one) and pushes it whenever that Dockerfile changes.

To deploy: apply `inferenceserver/inferenceserver.yaml`, then
`inferenceserver/deployment.yaml` with `desiredRevision.name` set to the
model name printed by a real `push_step` run.
