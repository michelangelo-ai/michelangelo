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

        # Generate standalone Python from YAML
        python_code = uniflow.generate_python_from_yaml("workflow.yml", "generated_workflow.py")
        # Then run: poetry run python generated_workflow.py
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


# YAML workflow functionality
from michelangelo.uniflow.core.yaml_parser import (
    validate_yaml_workflow,
    generate_python_from_yaml,
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


    # YAML workflows
    "validate_yaml_workflow",
    "generate_python_from_yaml",
    "YAMLWorkflowParser",
]


__version__ = "1.0.0"