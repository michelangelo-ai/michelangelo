"""Uniflow: Unified workflow orchestration for ML pipelines.

Uniflow provides a framework for defining and executing ML workflows that can run
both locally for development and remotely at production scale. It supports both
Python-first decorators and YAML-based workflow definitions.

Key features:
- @task and @workflow decorators for Python-first development
- YAML workflow definitions for configuration-driven development
- Dynamic task generation (foreach/expand patterns)
- Conditional workflow execution (if-else logic)
- Local and distributed execution (Ray, Spark, Cadence/Temporal)
- Automatic caching and retry handling
- Data persistence and serialization

Examples:
    Python-first workflow::

        import michelangelo.uniflow as uniflow
        from michelangelo.uniflow.plugins.ray import RayTask

        @uniflow.task(config=RayTask(head_cpu=2))
        def process_data(input_file: str) -> dict:
            # Process data
            return {"status": "complete"}

        @uniflow.workflow()
        def my_workflow():
            result = process_data("data.csv")
            return result

    YAML-based workflow::

        # workflow.yml
        metadata:
          name: "data_pipeline"
          version: "1.0"

        tasks:
          process_files:
            function: "src.data.process"
            expand:
              filename: ["file1.csv", "file2.csv"]
            config:
              type: "RayTask"
              resources:
                cpu: 2

        # Python code
        workflow_func = uniflow.load_yaml_workflow("workflow.yml")
        result = workflow_func()

    Dynamic workflows with conditions::

        @uniflow.conditional_task(
            condition=lambda result: result["quality"] > 0.8,
            on_true="train_model",
            on_false="clean_data"
        )
        @uniflow.task(config=RayTask())
        def quality_check(data: dict) -> dict:
            return {"quality": calculate_quality(data)}
"""

# Core decorators and context
from michelangelo.uniflow.core import (
    create_context,
    task,
    workflow,
    star_plugin,
    task_context,
    ImageSpec,
    IO,
)

# Dynamic workflow functionality
from michelangelo.uniflow.core.dynamic import (
    expand_task,
    conditional_task,
    collect_task,
    DynamicExecutionContext,
)

# YAML workflow functionality
from michelangelo.uniflow.core.yaml_parser import (
    load_yaml_workflow,
    validate_yaml_workflow,
    YAMLWorkflowParser,
)

# Task configurations from plugins
from michelangelo.uniflow.plugins.ray import RayTask
from michelangelo.uniflow.plugins.spark import SparkTask

__all__ = [
    # Core functionality
    "create_context",
    "task",
    "workflow",
    "star_plugin",
    "task_context",
    "ImageSpec",
    "IO",

    # Task configurations
    "RayTask",
    "SparkTask",

    # Dynamic workflows
    "expand_task",
    "conditional_task",
    "collect_task",
    "DynamicExecutionContext",

    # YAML workflows
    "load_yaml_workflow",
    "validate_yaml_workflow",
    "YAMLWorkflowParser",
]

# Convenience aliases for common patterns
foreach_task = expand_task  # Alternative name for expand_task
if_else_task = conditional_task  # Alternative name for conditional_task

__version__ = "1.0.0"