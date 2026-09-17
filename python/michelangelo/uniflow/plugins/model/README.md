# Model deployment plugin

`deploy_model` deploys the model registered by a completed child pipeline run and
waits synchronously for the requested rollout.

```python
from michelangelo.uniflow.plugins.model import deploy_model

result = deploy_model(
    namespace="default",
    deployment_name="retrain-example",
    pipeline_run_name=child_run["metadata"]["name"],
    inference_server_name="inference-server-example",
    timeout_seconds=1800,
)
```

The local Python implementation and remote Starlark implementation have the same
signature and return shape. `model_name` may be omitted when the pipeline run
registered exactly one model. If it registered multiple models, the plugin reports
the candidates and requires `model_name` so that list ordering can never decide what
is deployed.

The plugin creates a rolling Deployment when one does not exist and updates its
desired model on retrain. It refuses to silently move an existing Deployment to a
different inference server. A completed rollback or cleanup is a failure for the
requested deploy operation, even if the Deployment controller successfully restored
an older model.

OSS Deployments reference the immutable Model resource by its exact
`model.metadata.name`. They do not append `model.spec.revision_id`: the OSS pusher
creates unique physical model names, and the OSS deployment controller resolves
`desired_revision.name` with an exact Model API lookup.
