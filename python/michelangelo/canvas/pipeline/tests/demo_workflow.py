"""Fixture pipeline used by config_loader/run tests."""

from michelangelo.canvas.lib.shared.json_data.json_data import JSONData
from michelangelo.canvas.pipeline.task import pipeline_task
from michelangelo.uniflow.core.decorator import workflow
from tests.uniflow.core.test_task_config import TaskA


class TrainConfig(JSONData):
    """Config for the ``train`` task."""

    learning_rate: float


class EvalConfig(JSONData):
    """Config for the ``evaluate`` task."""

    threshold: float


class DemoWorkflowConfig(JSONData):
    """Workflow-level config for ``demo_workflow``."""

    dataset: str


@pipeline_task(config=TaskA())
def train(config: TrainConfig):
    """Fixture training task."""
    return {"trained_lr": config.learning_rate}


@pipeline_task(config=TaskA())
def evaluate(config: EvalConfig):
    """Fixture evaluation task."""
    return {"passed": config.threshold > 0}


@workflow()
def demo_workflow(config: DemoWorkflowConfig, task_configs):
    """Fixture workflow with a workflow-level config."""
    train_result = train(config=task_configs["train"])
    eval_result = evaluate(config=task_configs["evaluate"])
    return {"dataset": config.dataset, "train": train_result, "eval": eval_result}


@workflow()
def demo_workflow_no_config(task_configs):
    """Fixture workflow with no workflow-level config."""
    return train(config=task_configs["train"])
