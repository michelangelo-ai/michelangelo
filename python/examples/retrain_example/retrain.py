"""Run an existing training pipeline and deploy the model it produces."""

import michelangelo.uniflow.core as uniflow
from michelangelo.api.v2 import APIClient
from michelangelo.uniflow.plugins.model import deploy_model
from michelangelo.uniflow.plugins.pipeline import run_pipeline


@uniflow.workflow()
def retrain(
    namespace="default",
    retrainer_pipeline="bert-cola-test",
    deployment_name="retrain-example",
    inference_server_name="inference-server-example",
    path="nyu-mll/glue",
    name="cola",
    tokenizer_max_length=128,
    timeout_seconds=3600,
    poll_seconds=10,
):
    """Train BERT on CoLA and deploy the resulting immutable model."""
    child_run = run_pipeline(
        namespace=namespace,
        pipeline_name=retrainer_pipeline,
        kwargs={
            "path": path,
            "name": name,
            "tokenizer_max_length": tokenizer_max_length,
        },
        timeout_seconds=timeout_seconds,
        poll_seconds=poll_seconds,
    )
    deployment = deploy_model(
        namespace=namespace,
        deployment_name=deployment_name,
        pipeline_run_name=child_run["metadata"]["name"],
        inference_server_name=inference_server_name,
        timeout_seconds=timeout_seconds,
        poll_seconds=poll_seconds,
    )
    return {
        "pipeline_run": child_run,
        "deployment": deployment,
    }


if __name__ == "__main__":
    APIClient.set_caller("retrain-example")
    ctx = uniflow.create_context()
    ctx.environ["MA_NAMESPACE"] = "default"
    ctx.run(retrain)
