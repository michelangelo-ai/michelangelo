from michelangelo.uniflow.core import workflow
from michelangelo.sdk.workflow.tasks.tabular_feature_prep import task_function as tabular_feature_prep
from michelangelo.sdk.workflow.tasks.tabular_transform import task_function as tabular_transform
from michelangelo.sdk.workflow.tasks.tabular_trainer import task_function as tabular_trainer
from michelangelo.sdk.workflow.tasks.tabular_assembler import task_function as tabular_assembler
from michelangelo.sdk.workflow.tasks.tabular_inference import task_function as tabular_inference
from michelangelo.sdk.workflow.tasks.evaluator import task_function as evaluator
from michelangelo.sdk.workflow.tasks.pusher import task_function as pusher
from michelangelo.sdk.workflow.schema.v2alpha1.config import TaskConfig, WorkflowConfig


@workflow()
def tabular_train(
    workflow_config: WorkflowConfig,  # noqa: ARG001, adding workflow config to the function signature to make it future proof
    task_configs: dict[str, TaskConfig],
):
    (
        train_dataset,
        validation_dataset,
        test_dataset,
        calibration_dataset,
    ) = tabular_feature_prep(config=task_configs["tabular_feature_prep"])

    if "tabular_transform" in task_configs:
        transform_result = tabular_transform(
            config=task_configs["tabular_transform"],
            datasets={
                "train": train_dataset,
                "validation": validation_dataset,
            },
        )

    raw_model = tabular_trainer(
        config=task_configs["tabular_trainer"],
        train_dataset=transform_result.transformed_datasets["train"] if "tabular_transform" in task_configs else train_dataset,
        validation_dataset=transform_result.transformed_datasets["validation"] if "tabular_transform" in task_configs else validation_dataset,
    )

    assembled_model = tabular_assembler(
        config=task_configs["tabular_assembler"],
        raw_model=raw_model,
        feature_package=transform_result.feature_package if "tabular_transform" in task_configs else None,
    )

    inference_results = tabular_inference(
        config=task_configs["tabular_inference"],
        datasets={
            "train": train_dataset,
            "validation": validation_dataset,
            "test": test_dataset,
        },
        assembled_model=assembled_model,
    )

    evaluation_result = evaluator(
        config=task_configs["evaluator"],
        datasets={
            "train": inference_results.get("train"),
            "validation": inference_results.get("validation"),
            "test": inference_results.get("test"),
        },
    )

    assembled_model.performance_evaluation_report = evaluation_result.reports.get("performance_evaluation_report")

    artifacts = {"model": assembled_model}

    for dataset_name in ["train", "validation", "test"]:
        metrics = evaluation_result.metrics.get(dataset_name)
        if metrics:
            artifacts[dataset_name + "_summary_metrics"] = metrics.summary_metrics
            artifacts[dataset_name + "_instance_metrics"] = metrics.instance_metrics
        artifacts[dataset_name + "_inference_result"] = inference_results.get(dataset_name)

    pusher_results = pusher(config=task_configs["pusher"], items=artifacts)

    model_name = None
    for result in pusher_results:
        if result.name == "model" and result.plugin == "model_plugin":
            model_name = result.value
            break

    return {"model_name": model_name}
