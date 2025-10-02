from uber.ai.uniflow.core import workflow
from uber.ai.michelangelo.sdk.workflow.tasks.tabular_feature_prep import task_function as tabular_feature_prep
from uber.ai.michelangelo.sdk.workflow.tasks.tabular_inference import task_function as inference
from uber.ai.michelangelo.sdk.workflow.tasks.evaluator import task_function as evaluator
from uber.ai.michelangelo.sdk.workflow.tasks.comparator import task_function as comparator
from uber.ai.michelangelo.sdk.workflow.tasks.pusher import task_function as pusher
from uber.ai.michelangelo.sdk.workflow.schema.v2alpha1.config import TaskConfig, WorkflowConfig
from uber.ai.michelangelo.sdk.workflow.utils.plugin_constants import (
    PUSHER_PLUGIN_SUMMARY_METRICS,
    PUSHER_PLUGIN_INSTANCE_METRICS,
    PUSHER_PLUGIN_MODEL_PERFORMANCE_REPORT,
    PUSHER_PLUGIN_PREDICTION_RESULT,
    PUSHER_PLUGIN_COMPARATOR_RESULT,
)


INPUT_DATASET_NAME = "dataset"


@workflow()
def tabular_eval(
    workflow_config: WorkflowConfig,  # noqa: ARG001
    task_configs: dict[str, TaskConfig],
):
    (
        dataset,
        _,
        _,
        _,
    ) = tabular_feature_prep(config=task_configs["tabular_feature_prep"])

    artifacts = {}
    # By default, we pass the dataset down to the next step.
    # If the inference step is configured, we pass the inference results down to the next step.
    passed_down_dataset = dataset
    if "inference" in task_configs:
        inference_results = inference.with_overrides(alias="inference")(
            config=task_configs["inference"],
            datasets={
                INPUT_DATASET_NAME: dataset,
            },
            assembled_model=None,
        )

        artifacts[PUSHER_PLUGIN_PREDICTION_RESULT] = inference_results.get(INPUT_DATASET_NAME)
        passed_down_dataset = inference_results.get(INPUT_DATASET_NAME)

    # Track comparator result separately for return value
    comparator_result = None

    # Optional evaluator step
    if "evaluator" in task_configs:
        evaluation_result = evaluator(
            config=task_configs["evaluator"],
            datasets={
                INPUT_DATASET_NAME: passed_down_dataset,
            },
        )

        # Add evaluation metrics to artifacts
        metrics = evaluation_result.metrics.get(INPUT_DATASET_NAME)
        if metrics:
            artifacts[PUSHER_PLUGIN_SUMMARY_METRICS] = metrics.summary_metrics
            artifacts[PUSHER_PLUGIN_INSTANCE_METRICS] = metrics.instance_metrics
        report = evaluation_result.reports.get(PUSHER_PLUGIN_MODEL_PERFORMANCE_REPORT)
        if report:
            artifacts[PUSHER_PLUGIN_MODEL_PERFORMANCE_REPORT] = report

    # Optional comparator step
    if "comparator" in task_configs:
        comparator_result = comparator(
            config=task_configs["comparator"],
            datasets={
                INPUT_DATASET_NAME: passed_down_dataset,
            },
        )
        artifacts[PUSHER_PLUGIN_COMPARATOR_RESULT] = comparator_result

    pusher(config=task_configs["pusher"], items=artifacts)

    if comparator_result != None:  # noqa: E711
        return {
            "comparator_result": comparator_result,
        }
    else:
        return {}
