from typing import Any, Optional

from michelangelo.canvas.lib.shared.json_data.json_data import JSONData

from .job_specs import JobSpecs


class TaskConfig(JSONData):
    """
    Task config class.

    Each task function is required to have a task config, and the task config will be the first argument
    in the task function named as 'config' with type annotation.

    The workflow framework will derive the task config type from task function and parse the task config from
    the pipeline_conf.yaml.

    Example task function signature:
        @canvas_task(config=Ray(...)))
        def example_task_func(config: ExampleTaskConfig, args...):

    Attributes:
        task_function (str): Fully qualified name of the task function.
        config (Any): Task level config. The type is derived from the type annotation in the task function.
    """

    task_function: str
    config: Any
    job_specs: Optional[JobSpecs]


class WorkflowConfig(JSONData):
    """
    Workflow config class.

    This class defines the workflow config schema. It is used to load the configuration specified in
    pipeline_conf.yaml.

    Canvas supports two types of workflow functions: one with workflow level config and one without
    workflow level config. The workflow function signatures are as follows:

    1. Workflow function with workflow level config:
        @workflow
        def example_workflow_func(config: ExampleConfig, task_configs: dict[str, TaskConfig]):

    2. Workflow function without workflow level config:
        @workflow
        def example_workflow_func2(task_configs: dict[str, TaskConfig]):

    Attributes:
        workflow_function (str): Fully qualified name of the workflow function.
        workflow_config (Any): Optional workflow level config. The type is derived from the type annotation
                               in the workflow function. Defaults to None if not provided.
        task_configs (dict[str, TaskConfig]): Task level configs. Each task config is identified by the
                                              task function name.
    """

    workflow_function: str
    workflow_config: Any
    task_configs: dict[str, TaskConfig]
