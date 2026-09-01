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

`inferenceserver/` holds this example's deploy manifests. Rather than a
Dockerfile, `inferenceserver/requirements.txt` lists the packages the
custom python-backend `assembler.py` produces needs -- CI
(`.github/workflows/build-example-triton-images.yaml`) builds a per-project
serving image from the shared `docker/triton-serving.Dockerfile` using that
list, and `inferenceserver.yaml`'s `servingSpec.containerBuildTemplate`
points at the result. No project ever writes or maintains a Dockerfile;
only a plain dependency list.

To deploy: apply `inferenceserver/inferenceserver.yaml`, then
`inferenceserver/deployment.yaml` with `desiredRevision.name` set to the
model name printed by a real `bert_cola.py` run's `push_step`.
