# Uniflow retrain example

`retrain-example` composes two OSS Uniflow plugins:

1. `pipeline.run_pipeline` runs the existing `bert-cola-test` training example.
2. `model.deploy_model` finds the immutable Model registered by that child run
   and rolls it out to `inference-server-example`.

The project, both pipelines, and inference resources must share a namespace in
the current OSS controllers. The checked-in `project.yaml` routes workflows to
the sandbox's default worker queue, and `training_pipeline.yaml` registers the
existing `examples.bert_cola.bert_cola` workflow alongside the inference demo.

Set up the sandbox resources and register both pipelines:

```bash
ma sandbox demo inference
ma project apply --file=examples/retrain_example/project.yaml
ma pipeline apply --file=examples/retrain_example/training_pipeline.yaml
ma pipeline apply --file=examples/retrain_example/pipeline.yaml
```

Run the orchestration locally, while its training child runs in the sandbox:

```bash
MA_API_SERVER=localhost:15566 \
  python -m examples.retrain_example.retrain local-run
```

Run the same Python workflow remotely through the Starlark worker:

```bash
MA_API_SERVER=localhost:15566 \
AWS_ACCESS_KEY_ID=minioadmin \
AWS_SECRET_ACCESS_KEY=minioadmin \
AWS_ENDPOINT_URL=http://localhost:9091 \
  python -m examples.retrain_example.retrain remote-run \
    --storage-url s3://default/uniflow/retrain-example \
    --image ghcr.io/michelangelo-ai/examples:main \
    --yes
```
