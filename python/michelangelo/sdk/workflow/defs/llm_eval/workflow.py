from uber.ai.uniflow.core import workflow
from uber.ai.michelangelo.sdk.workflow.tasks.llm_feature_prep import task_function as llm_feature_prep
from uber.ai.michelangelo.sdk.workflow.tasks.llm_inference import task_function as llm_inference
from uber.ai.michelangelo.sdk.workflow.tasks.evaluator import task_function as evaluator
from uber.ai.michelangelo.sdk.workflow.tasks.pusher import task_function as pusher
from uber.ai.michelangelo.sdk.workflow.schema.v2alpha1.config import TaskConfig, WorkflowConfig


@workflow()
def llm_eval(workflow_config: WorkflowConfig, task_configs: dict[str, TaskConfig]):  # noqa: ARG001
    (
        dataset,
        _,
        _,
    ) = llm_feature_prep(config=task_configs["llm_feature_prep"])

    inference_results = llm_inference(config=task_configs["llm_inference"], datasets={"dataset": dataset})

    _ = evaluator(
        config=task_configs["evaluator"],
        datasets=inference_results,
    )

    artifacts = {}

    for inference_dataset_name, inference_result in inference_results.items():
        artifacts[inference_dataset_name + "_inference_result"] = inference_result

    pusher(config=task_configs["pusher"], items=artifacts)
