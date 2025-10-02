from uber.ai.uniflow.core import workflow
from uber.ai.michelangelo.sdk.workflow.tasks.tabular_feature_prep import task_function as tabular_feature_prep
from uber.ai.michelangelo.sdk.workflow.tasks.tabular_inference import task_function as inference
from uber.ai.michelangelo.sdk.workflow.tasks.pusher import task_function as pusher
from uber.ai.michelangelo.sdk.workflow.schema.v2alpha1.config import TaskConfig, WorkflowConfig


@workflow()
def tabular_inference(
    workflow_config: WorkflowConfig,  # noqa: ARG001
    task_configs: dict[str, TaskConfig],
):
    (
        dataset,
        _,
        _,
        _,
    ) = tabular_feature_prep(config=task_configs["tabular_feature_prep"])

    inference_results = inference.with_overrides(alias="inference")(
        config=task_configs["inference"],
        datasets={
            "dataset": dataset,
        },
        assembled_model=None,
    )

    artifacts = {
        "inference_result": inference_results["dataset"],
    }

    pusher(config=task_configs["pusher"], items=artifacts)
