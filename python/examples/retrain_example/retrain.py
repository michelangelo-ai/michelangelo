"""Run an existing training pipeline and deploy the model it produces."""

import michelangelo.uniflow.core as uniflow
from michelangelo.api.v2 import APIClient
from michelangelo.uniflow.core.lib import (
    create_or_update_deployment,
    wait_for_deployment,
)
from michelangelo.uniflow.plugins.model import get_models_by_pipeline_run
from michelangelo.uniflow.plugins.pipeline import run_pipeline


@uniflow.workflow()
def retrain_workflow(
    namespace="default",
    retrainer_pipeline="bert-cola-test",
    deployment_name="retrain-example",
    deployment_template="deployment-example",
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
    models = get_models_by_pipeline_run(
        namespace=namespace,
        pipeline_run_name=child_run["metadata"]["name"],
    )
    model_name = models[0]["name"]
    deployment = create_or_update_deployment(
        namespace=namespace,
        deployment_name=deployment_name,
        model_revision_name=model_name,
        deployment_template=deployment_template,
    )
    deployment_status = wait_for_deployment(
        namespace=namespace,
        deployment_name=deployment["deployment_name"],
        expected_model_revision_name=model_name,
        timeout=timeout_seconds,
        poll=poll_seconds,
    )
    return {
        "pipeline_run": child_run,
        "model_name": model_name,
        "deployment_name": deployment["deployment_name"],
        "deployment_stage": deployment_status["stage"],
    }


if __name__ == "__main__":
    APIClient.set_caller("retrain-example")
    ctx = uniflow.create_context()
    ctx.environ["MA_NAMESPACE"] = "default"
    ctx.run(retrain_workflow)
