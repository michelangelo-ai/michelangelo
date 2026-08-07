"""Fixture pipeline for testing explicit task_function overrides."""

from michelangelo.canvas.lib.shared.json_data.json_data import JSONData
from michelangelo.canvas.pipeline.task import pipeline_task
from michelangelo.uniflow.core.decorator import workflow
from tests.uniflow.core.test_task_config import TaskA


class TrainConfig(JSONData):
    """Config for the ``train``/``custom_train`` tasks."""

    learning_rate: float


@pipeline_task(config=TaskA())
def train(config: TrainConfig):
    """Default training task implementation."""
    return {"trained_lr": config.learning_rate, "variant": "default"}


@pipeline_task(config=TaskA())
def custom_train(config: TrainConfig):
    """Alternate training task implementation, used via an explicit override."""
    return {"trained_lr": config.learning_rate, "variant": "custom"}


@workflow()
def demo_workflow(task_configs):
    """Fixture workflow used to test task_function overrides."""
    return train(config=task_configs["train"])
