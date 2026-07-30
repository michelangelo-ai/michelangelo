"""Workflow + tasks authored for YAML config (see pipeline_conf.yaml in this dir).

Real tasks would use a distributed TaskConfig (RayTask, SparkTask); this example
uses a minimal local-execution TaskConfig so the example runs without a Ray/Spark
cluster.
"""

from dataclasses import dataclass
from pathlib import Path

from michelangelo.canvas.lib.shared.json_data.json_data import JSONData
from michelangelo.canvas.pipeline.task import pipeline_task
from michelangelo.uniflow.core.decorator import workflow
from michelangelo.uniflow.core.task_config import TaskBinding, TaskConfig


@dataclass
class LocalTask(TaskConfig):
    """No-op TaskConfig for running a task in-process, with no scheduling."""

    def get_binding(self) -> TaskBinding:
        """Return a placeholder binding; this config never gets transpiled."""
        return TaskBinding(
            star_file=Path(__file__), function="local_task", export="__local_task"
        )

    @classmethod
    def get_config_binding(cls) -> TaskBinding:
        """Return a placeholder binding; this config never gets transpiled."""
        return TaskBinding(
            star_file=Path(__file__), function="local_config", export="__local_config"
        )

    def pre_run(self):
        """No setup needed for in-process execution."""

    def post_run(self):
        """No cleanup needed for in-process execution."""


class PrepareDataConfig(JSONData):
    """Config for the ``prepare_data`` task."""

    dataset: str


class TrainConfig(JSONData):
    """Config for the ``train`` task."""

    learning_rate: float
    epochs: int


class PipelineWorkflowConfig(JSONData):
    """Workflow-level config for ``canvasflex_pipeline_example``."""

    experiment_name: str


@pipeline_task(config=LocalTask())
def prepare_data(config: PrepareDataConfig) -> dict:
    """Load (a stand-in for) the configured dataset."""
    return {"dataset": config.dataset, "rows": 1000}


@pipeline_task(config=LocalTask())
def train(config: TrainConfig, data: dict) -> dict:
    """Train (a stand-in for) a model on the prepared data."""
    return {
        "model": f"model-trained-on-{data['dataset']}",
        "final_loss": config.learning_rate * 10 / config.epochs,
    }


@workflow()
def canvasflex_pipeline_example(config: PipelineWorkflowConfig, task_configs: dict):
    """Prepare data, then train a model on it."""
    data = prepare_data(config=task_configs["prepare_data"])
    model = train(config=task_configs["train"], data=data)
    return {"experiment_name": config.experiment_name, "model": model}
