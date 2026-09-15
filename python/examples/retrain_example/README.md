# Uniflow retrain example

`retrain-example` composes three OSS Uniflow plugins:

1. `pipeline.run_pipeline` runs the existing `bert-cola-test` training example.
2. `model.get_models_by_pipeline_run` finds the Model registered by that child
   run. OSS Deployments reference the exact `Model.metadata.name`; unlike the
   internal API, they do not append the model revision ID.
3. `deployment.create_or_update_deployment` clones `deployment-example` on the
   first run (or updates the existing target), then
   `deployment.wait_for_deployment` waits for the rollout.

The project, both pipelines, and inference resources must share a namespace in
the current OSS controllers. The checked-in `project.yaml` routes workflows to
the sandbox's default worker queue, and `training_pipeline.yaml` registers the
existing BERT/CoLA tasks through a single-node workflow alongside the inference
demo. The head-only task configuration avoids duplicating the large examples
image across both nodes of the local k3d sandbox.

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
