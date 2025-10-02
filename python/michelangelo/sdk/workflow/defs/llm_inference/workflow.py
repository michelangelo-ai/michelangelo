from uber.ai.uniflow.core import workflow
from uber.ai.michelangelo.sdk.workflow.schema.v2alpha1.config import TaskConfig
from uber.ai.michelangelo.sdk.workflow.tasks.llm_inference import task_function as llm_inference_task
from uber.ai.michelangelo.sdk.workflow.tasks.llm_feature_prep import task_function as llm_feature_prep
from uber.ai.michelangelo.sdk.workflow.tasks.pusher import task_function as pusher


@workflow()
def llm_inference(task_configs: dict[str, TaskConfig]):
    (
        dataset,
        _,
        _,
    ) = llm_feature_prep(config=task_configs["llm_feature_prep"])

    inference_results = llm_inference_task(config=task_configs["llm_inference"], datasets={"train": dataset})

    dataset_artifact = {"dataset": inference_results}

    pusher(config=task_configs["pusher"], items=dataset_artifact)
