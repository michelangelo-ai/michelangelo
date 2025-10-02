from uber.ai.uniflow.core import workflow
from uber.ai.michelangelo.sdk.workflow.tasks.document_loader import task_function as document_loader
from uber.ai.michelangelo.sdk.workflow.tasks.llm_inference import task_function as llm_inference
from uber.ai.michelangelo.sdk.workflow.tasks.genai_evaluator import task_function as genai_evaluator
from uber.ai.michelangelo.sdk.workflow.schema.v2alpha1.config import TaskConfig, WorkflowConfig


@workflow()
def agent_eval(workflow_config: WorkflowConfig, task_configs: dict[str, TaskConfig]):  # noqa: ARG001
    dataset, _ = document_loader(config=task_configs["document_loader"])

    inference_results = llm_inference(config=task_configs["llm_inference"], datasets={"llm_inference": dataset})

    genai_evaluator(config=task_configs["genai_evaluator"], dataset=inference_results["llm_inference"])
