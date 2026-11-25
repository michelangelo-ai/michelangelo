# BERT CoLA Fine-tuning Demo

Fine-tuning BERT for linguistic acceptability classification using the Corpus of Linguistic Acceptability (CoLA) task from the GLUE benchmark. Demonstrates sequence classification with distributed training on Ray.

## Features

- **Pre-trained Model**: `bert-base-cased` fine-tuning
- **GLUE Benchmark**: CoLA task for grammatical acceptability (the entrypoint ships with SST-2 selected; see below)
- **Distributed Training**: Ray-based execution
- **HuggingFace Integration**: Uses transformers and datasets libraries
- **Evaluation Metric**: Matthews correlation coefficient (`matthews_correlation`)

## Prerequisites

- **Extras**: `poetry install -E example` (installs torch, transformers, datasets, ray). A plain `poetry install` omits it, and the module then fails to import with `No module named 'datasets'`.
- **Network access**: the run downloads the dataset (`nyu-mll/glue`) and `bert-base-cased` from the HuggingFace Hub.
- **GPU**: not required. Training uses mixed precision automatically if GPUs are available.
- **Sandbox**: not required for a local run. Remote runs need the sandbox and the `example` image; see [`../README.md`](../README.md).

## How to Run

From the `python/` directory. `PYTHONPATH=.` is required because the script imports `examples.bert_cola.*` and only `michelangelo` is installed as a package.

```bash
cd python
poetry install -E example
PYTHONPATH=. poetry run python examples/bert_cola/bert_cola.py
```

The entrypoint's `__main__` block selects which configuration runs. As shipped it runs **SST-2** (`name="sst2"`, `tokenizer_max_length=256`). To run CoLA, comment out the "Example 2" `ctx.run(...)` line and uncomment `ctx.run(train_workflow)` ("Example 1").

The example is a smoke test: `load_data` keeps a 1% random sample of each split and `train` runs one epoch.

For a remote run on the sandbox, see [`../README.md`](../README.md#remote-run).

## Expected Output

The workflow body prints these lines, interleaved with the Ray, HuggingFace and `Trainer` logs from the tasks. Metric values vary between runs.

```
[train_workflow] Starting with config:
  - Dataset: nyu-mll/glue/sst2
  - Tokenizer max length: 256
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
