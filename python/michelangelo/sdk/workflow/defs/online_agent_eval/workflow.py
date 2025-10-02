from uber.ai.uniflow.core import workflow
from uber.ai.michelangelo.sdk.workflow.tasks.document_loader import task_function as document_loader
from uber.ai.michelangelo.sdk.workflow.tasks.genai_evaluator import task_function as genai_evaluator
from uber.ai.michelangelo.sdk.workflow.tasks.pusher import task_function as pusher_offline
from uber.ai.michelangelo.sdk.workflow.schema.v2alpha1.config import TaskConfig, WorkflowConfig


@workflow()
def online_agent_eval(workflow_config: WorkflowConfig, task_configs: dict[str, TaskConfig]):  # noqa: ARG001
    """
    Online Agent Evaluation Workflow.

    This workflow is similar to agent_eval but skips the llm_inference step since
    the agent responses are already available in the Hive table data.

    Steps:
    1. Load data from Hive table using SparkSQL plugin
    2. Directly evaluate using genai_evaluator
    3. Push evaluation results to Hive using pusher_offline (for online evaluation only)
    """
    dataset, _ = document_loader(config=task_configs["document_loader"])

    # Skip llm_inference and go directly to evaluation since responses are already in the data
    eval_results = genai_evaluator(config=task_configs["genai_evaluator"], dataset=dataset)

    # If evaluation results are returned (online evaluation), push them to Hive
    if eval_results != None and "pusher_offline" in task_configs:  # noqa: E711
        # Create artifacts dict for pusher - following the same pattern as vector_gen workflow
        eval_artifacts = {"evaluation_results": eval_results}
        pusher_offline.with_overrides(alias="pusher_offline")(task_configs["pusher_offline"], eval_artifacts)


# For backward compatibility and standard naming
workflow_function = online_agent_eval
