"""YAML parser and validator for Uniflow dynamic workflows.

This module provides functionality to parse YAML workflow configurations and
convert them into executable Uniflow workflows using the dynamic task system.

Key features:
- YAML schema validation
- Task reference resolution ("+task_name")
- Dynamic task pattern detection and conversion
- Integration with existing TaskFunction system
- Workflow execution orchestration

Example YAML workflow:
    metadata:
      name: "ml_pipeline"
      version: "1.0"

    defaults:
      storage_url: "s3://bucket/workflows"
      image_spec: "ml-base:v1.0"

    tasks:
      discover_datasets:
        function: "src.data.discover_datasets"
        config:
          type: "RayTask"
          resources:
            cpu: 2
            memory: "4GB"

      preprocess_data:
        function: "src.data.preprocess"
        expand:
          dataset_id: "+discover_datasets"
        config:
          type: "SparkTask"
        dependencies: ["discover_datasets"]
"""

import importlib
import logging
import re
from pathlib import Path
from typing import Any, Dict, List, Optional, Set, Union

import yaml
from pydantic import BaseModel, Field, validator

from michelangelo.uniflow.core.decorator import TaskFunction, task, workflow
from michelangelo.uniflow.core.image_spec import ImageSpec
from michelangelo.uniflow.core.io_registry import default_io
from michelangelo.uniflow.core.task_config import TaskConfig
from michelangelo.uniflow.plugins.ray import RayTask
from michelangelo.uniflow.plugins.spark import SparkTask

log = logging.getLogger(__name__)


# Pydantic models for YAML schema validation

class ResourceConfig(BaseModel):
    """Resource configuration for tasks."""
    cpu: Optional[int] = None
    memory: Optional[str] = None
    gpu: Optional[int] = None
    disk: Optional[str] = None
    executor_instances: Optional[int] = None  # For Spark
    executor_cores: Optional[int] = None     # For Spark


class TaskConfigSpec(BaseModel):
    """Task configuration specification."""
    type: str  # "RayTask", "SparkTask", etc.
    resources: Optional[ResourceConfig] = None

    @validator("type")
    def validate_task_type(cls, v):
        valid_types = ["RayTask", "SparkTask"]
        if v not in valid_types:
            raise ValueError(f"Task type must be one of {valid_types}")
        return v


class ExpandConfig(BaseModel):
    """Configuration for expand/foreach patterns."""
    dataset_id: Optional[str] = None
    item: Optional[str] = None
    items: Optional[Union[List[Any], str]] = None  # Static list or "+task_reference"
    max_parallel: Optional[int] = None

    # Allow dynamic fields
    class Config:
        extra = "allow"


class ConditionConfig(BaseModel):
    """Configuration for conditional logic."""
    field: Optional[str] = None
    operator: Optional[str] = "=="
    value: Optional[Any] = None
    expression: Optional[str] = None  # For complex expressions
    on_true: Optional[str] = None
    on_false: Optional[str] = None

    @validator("operator")
    def validate_operator(cls, v):
        if v:
            valid_ops = [">", "<", ">=", "<=", "==", "!="]
            if v not in valid_ops:
                raise ValueError(f"Operator must be one of {valid_ops}")
        return v


class CollectConfig(BaseModel):
    """Configuration for collecting results from dynamic tasks."""
    from_task: Union[str, List[str]] = Field(..., alias="from")
    strategy: str = "list"
    field: Optional[str] = None

    @validator("strategy")
    def validate_strategy(cls, v):
        valid_strategies = ["list", "sum", "max", "min", "custom"]
        if v not in valid_strategies:
            raise ValueError(f"Strategy must be one of {valid_strategies}")
        return v


class TaskSpec(BaseModel):
    """Specification for a single task."""
    task_id: Optional[str] = None  # DAG Factory style task_id
    function: str
    description: Optional[str] = None
    config: Optional[TaskConfigSpec] = None
    inputs: Optional[Dict[str, Any]] = None
    outputs: Optional[List[Dict[str, str]]] = None
    dependencies: Optional[List[str]] = None

    # Dynamic task configurations
    expand: Optional[ExpandConfig] = None
    condition: Optional[ConditionConfig] = None
    collect: Optional[CollectConfig] = None
    when: Optional[str] = None  # Simple conditional expression

    cache_enabled: bool = False
    cache_version: Optional[str] = None
    retry_attempts: int = 0
    image_spec: Optional[str] = None


class EnvironmentConfig(BaseModel):
    """Environment configuration."""
    variables: Optional[Dict[str, Any]] = None
    secrets: Optional[List[str]] = None


class DefaultsConfig(BaseModel):
    """Default configuration values."""
    storage_url: Optional[str] = None
    image_spec: Optional[str] = None
    cache_enabled: bool = False
    cache_version: Optional[str] = None
    retry_attempts: int = 0


class MetadataConfig(BaseModel):
    """Workflow metadata."""
    name: str
    description: Optional[str] = None
    version: str = "1.0"
    author: Optional[str] = None


class WorkflowConfig(BaseModel):
    """Complete workflow configuration."""
    metadata: MetadataConfig
    defaults: Optional[DefaultsConfig] = None
    environment: Optional[EnvironmentConfig] = None
    workflow: Optional[Dict[str, Any]] = None
    tasks: Union[Dict[str, TaskSpec], List[TaskSpec]]  # Support both dict and list formats


class YAMLWorkflowParser:
    """Parser for YAML workflow configurations.

    Handles parsing, validation, and conversion of YAML workflow definitions
    into executable Uniflow workflows with dynamic task support.
    """

    def __init__(self):
        self.config: Optional[WorkflowConfig] = None
        self.task_functions: Dict[str, TaskFunction] = {}
        self.task_graph: Dict[str, Set[str]] = {}  # task_name -> dependencies

    def parse_file(self, yaml_path: Union[str, Path]) -> WorkflowConfig:
        """Parse workflow configuration from YAML file.

        Args:
            yaml_path: Path to the YAML configuration file

        Returns:
            Parsed and validated workflow configuration

        Raises:
            FileNotFoundError: If the YAML file doesn't exist
            yaml.YAMLError: If the YAML is malformed
            ValidationError: If the configuration is invalid
        """
        yaml_path = Path(yaml_path)
        if not yaml_path.exists():
            raise FileNotFoundError(f"YAML file not found: {yaml_path}")

        with open(yaml_path, 'r') as f:
            raw_config = yaml.safe_load(f)

        return self.parse_dict(raw_config)

    def parse_dict(self, config_dict: Dict[str, Any]) -> WorkflowConfig:
        """Parse workflow configuration from dictionary.

        Args:
            config_dict: Raw configuration dictionary

        Returns:
            Parsed and validated workflow configuration
        """
        self.config = WorkflowConfig(**config_dict)
        self._normalize_tasks_format()
        self._validate_workflow()
        return self.config

    def _normalize_tasks_format(self):
        """Convert list-style tasks to dict-style for internal processing."""
        if isinstance(self.config.tasks, list):
            # Convert DAG Factory list format to dict format
            tasks_dict = {}
            for task_spec in self.config.tasks:
                task_name = task_spec.task_id or f"task_{len(tasks_dict)}"
                tasks_dict[task_name] = task_spec
            self.config.tasks = tasks_dict

    def _validate_workflow(self):
        """Validate the workflow configuration."""
        if not self.config:
            raise ValueError("No configuration loaded")

        # Build dependency graph
        self._build_dependency_graph()

        # Check for circular dependencies
        self._check_circular_dependencies()

        # Validate task references
        self._validate_task_references()

    def _build_dependency_graph(self):
        """Build the task dependency graph."""
        self.task_graph = {}

        for task_name, task_spec in self.config.tasks.items():
            dependencies = set()

            # Direct dependencies
            if task_spec.dependencies:
                dependencies.update(task_spec.dependencies)

            # Dependencies from expand references
            if task_spec.expand:
                for field_name, field_value in task_spec.expand.dict().items():
                    if isinstance(field_value, str) and field_value.startswith("+"):
                        ref_task = field_value[1:]
                        dependencies.add(ref_task)

            # Dependencies from collect references
            if task_spec.collect:
                if isinstance(task_spec.collect.from_task, str):
                    if task_spec.collect.from_task.startswith("+"):
                        ref_task = task_spec.collect.from_task[1:]
                        dependencies.add(ref_task)
                elif isinstance(task_spec.collect.from_task, list):
                    for ref in task_spec.collect.from_task:
                        if isinstance(ref, str) and ref.startswith("+"):
                            ref_task = ref[1:]
                            dependencies.add(ref_task)

            self.task_graph[task_name] = dependencies

    def _check_circular_dependencies(self):
        """Check for circular dependencies in the task graph."""
        def has_cycle(node: str, visited: Set[str], rec_stack: Set[str]) -> bool:
            visited.add(node)
            rec_stack.add(node)

            for neighbor in self.task_graph.get(node, set()):
                if neighbor not in visited:
                    if has_cycle(neighbor, visited, rec_stack):
                        return True
                elif neighbor in rec_stack:
                    return True

            rec_stack.remove(node)
            return False

        visited = set()
        for task_name in self.config.tasks.keys():
            if task_name not in visited:
                if has_cycle(task_name, visited, set()):
                    raise ValueError(f"Circular dependency detected involving task: {task_name}")

    def _validate_task_references(self):
        """Validate that all task references exist."""
        task_names = set(self.config.tasks.keys())

        for task_name, dependencies in self.task_graph.items():
            for dep in dependencies:
                if dep not in task_names:
                    raise ValueError(f"Task '{task_name}' references unknown task '{dep}'")

    def _get_execution_order(self) -> List[str]:
        """Get the topological order for task execution."""
        # Kahn's algorithm for topological sorting
        in_degree = {task: 0 for task in self.config.tasks.keys()}

        # Calculate in-degrees
        for task, deps in self.task_graph.items():
            for dep in deps:
                in_degree[task] += 1

        # Find tasks with no dependencies
        queue = [task for task, degree in in_degree.items() if degree == 0]
        execution_order = []

        while queue:
            current = queue.pop(0)
            execution_order.append(current)

            # Update in-degrees for dependent tasks
            for task, deps in self.task_graph.items():
                if current in deps:
                    in_degree[task] -= 1
                    if in_degree[task] == 0:
                        queue.append(task)

        if len(execution_order) != len(self.config.tasks):
            remaining = set(self.config.tasks.keys()) - set(execution_order)
            raise ValueError(f"Circular dependency detected. Remaining tasks: {remaining}")

        return execution_order

    def generate_python_code(self, output_path: Optional[Union[str, Path]] = None) -> str:
        """Generate standalone Python code from the workflow configuration.

        Args:
            output_path: Optional output path for the generated file

        Returns:
            The generated Python code as a string
        """
        if not self.config:
            raise ValueError("No configuration loaded")

        # Generate the Python code
        code_lines = []

        # Header and imports
        code_lines.extend([
            "#!/usr/bin/env python3",
            '"""Generated workflow from YAML configuration."""',
            "",
            "# Auto-generated imports",
            "import os",
            "import logging",
            "from typing import Any, Dict",
            "",
            "# Uniflow imports",
            "import michelangelo.uniflow as uniflow",
            "from michelangelo.uniflow.plugins.ray import RayTask",
            "from michelangelo.uniflow.plugins.spark import SparkTask",
            "",
            "# Configure logging",
            "logging.basicConfig(level=logging.INFO)",
            "log = logging.getLogger(__name__)",
            "",
            "# Workflow metadata",
            f'WORKFLOW_NAME = "{self.config.metadata.name}"',
            f'WORKFLOW_VERSION = "{self.config.metadata.version}"',
        ])

        if self.config.metadata.description:
            code_lines.append(f'WORKFLOW_DESCRIPTION = "{self.config.metadata.description}"')

        code_lines.append("")

        # Generate task function imports and decorators
        for task_name, task_spec in self.config.tasks.items():
            code_lines.extend(self._generate_task_function_code(task_name, task_spec))
            code_lines.append("")

        # Generate main workflow function
        code_lines.extend(self._generate_workflow_function_code())

        # Add main execution block
        code_lines.extend([
            "",
            'if __name__ == "__main__":',
            "    # Set up environment for standalone execution",
            '    os.environ["UF_LOCAL_RUN"] = "1"',
            '    if not os.environ.get("UF_STORAGE_URL"):',
            '        os.environ["UF_STORAGE_URL"] = os.path.expanduser("~/uf_storage")',
            "",
            "    # Run the workflow",
            "    print(f'Starting workflow: {WORKFLOW_NAME} v{WORKFLOW_VERSION}')",
            "    result = yaml_workflow()",
            '    print("✅ Workflow completed successfully!")',
            '    print("📋 Results:")',
            "    for task_name, task_result in result.items():",
            '        print(f"  {task_name}: {task_result}")',
        ])

        # Join all lines
        generated_code = "\n".join(code_lines)

        # Write to file if output path specified
        if output_path:
            output_path = Path(output_path)
        else:
            # Default path in current directory
            yaml_name = self.config.metadata.name.replace(" ", "_").lower()
            output_path = Path(f"generated_{yaml_name}.py")

        output_path.write_text(generated_code)

        # Make file executable
        output_path.chmod(0o755)

        return generated_code

    def _generate_task_function_code(self, task_name: str, task_spec: TaskSpec) -> List[str]:
        """Generate Python code for a single task function."""
        lines = []

        # Import statement
        lines.append(f"# Import function for task: {task_name}")
        try:
            module_path, func_name = task_spec.function.rsplit(".", 1)
            lines.append(f"from {module_path} import {func_name}")
        except ValueError:
            lines.append(f"# Note: {task_spec.function} should be importable")
            lines.append(f"import {task_spec.function}")

        # Task configuration
        task_config_code = self._generate_task_config_code(task_spec)

        # Only use basic @uniflow.task decorator (no dynamic decorators)
        decorator_lines = [f"@uniflow.task(config={task_config_code})"]

        # Function definition
        try:
            module_path, func_name = task_spec.function.rsplit(".", 1)
            function_alias = f"{task_name}_task"
            lines.extend(decorator_lines)
            lines.append(f"def {function_alias}(*args, **kwargs):")
            lines.append(f'    """Generated task function for {task_name}."""')
            lines.append(f"    return {func_name}(*args, **kwargs)")
        except ValueError:
            function_alias = task_spec.function
            lines.extend(decorator_lines)
            lines.append(f"# Note: Using {task_spec.function} directly")

        return lines

    def _generate_task_config_code(self, task_spec: TaskSpec) -> str:
        """Generate task configuration code."""
        if not task_spec.config:
            return "RayTask()"

        config_type = task_spec.config.type
        resources = task_spec.config.resources or ResourceConfig()

        if config_type == "RayTask":
            args = []
            if resources.cpu:
                args.append(f"head_cpu={resources.cpu}")
            if resources.memory:
                args.append(f'head_memory="{resources.memory}"')
            if resources.gpu:
                args.append(f"head_gpu={resources.gpu}")
            return f"RayTask({', '.join(args)})"

        elif config_type == "SparkTask":
            args = []
            if resources.cpu:
                args.append(f"driver_cpu={resources.cpu}")
            if resources.memory:
                args.append(f'driver_memory="{resources.memory}"')
            if resources.executor_cores:
                args.append(f"executor_cpu={resources.executor_cores}")
            if resources.executor_instances:
                args.append(f"executor_instances={resources.executor_instances}")
            return f"SparkTask({', '.join(args)})"

        return "RayTask()"  # fallback

    def _generate_workflow_function_code(self) -> List[str]:
        """Generate the main workflow function code."""
        lines = []

        # Get execution order
        execution_order = self._get_execution_order()

        lines.extend([
            "@uniflow.workflow()",
            "def yaml_workflow(**kwargs):",
            f'    """Generated workflow: {self.config.metadata.name}"""',
            "    # Execute tasks in topological order",
            "    results = {}",
        ])

        # Generate task execution code
        for i, task_name in enumerate(execution_order):
            task_spec = self.config.tasks[task_name]

            lines.append(f"    # Task {i+1}: {task_name}")
            if task_spec.description:
                lines.append(f'    # {task_spec.description}')
            lines.append(f'    log.info("Executing task: {task_name}")')

            # Check if this task has expand (foreach) pattern
            if task_spec.expand:
                lines.extend(self._generate_foreach_task_code(task_name, task_spec))
            else:
                # Regular single task execution
                lines.extend(self._generate_single_task_code(task_name, task_spec))

            lines.append("")

        lines.extend([
            "    return results",
        ])

        return lines

    def _generate_single_task_code(self, task_name: str, task_spec: TaskSpec) -> List[str]:
        """Generate code for a regular single task execution."""
        lines = []

        # Generate task inputs
        if task_spec.inputs:
            lines.append("    task_inputs = {")
            for input_name, input_spec in task_spec.inputs.items():
                if isinstance(input_spec, str) and input_spec.startswith("+"):
                    ref_task = input_spec[1:]
                    lines.append(f'        "{input_name}": results["{ref_task}"],')
                else:
                    lines.append(f'        "{input_name}": {repr(input_spec)},')
            lines.append("    }")
        else:
            lines.append("    task_inputs = {}")

        # Call the task function
        try:
            module_path, func_name = task_spec.function.rsplit(".", 1)
            function_alias = f"{task_name}_task"
        except ValueError:
            function_alias = task_spec.function

        lines.extend([
            f'    results["{task_name}"] = {function_alias}(**task_inputs)',
            f'    log.info("Completed task: {task_name}")',
        ])

        return lines

    def _generate_foreach_task_code(self, task_name: str, task_spec: TaskSpec) -> List[str]:
        """Generate code for a foreach/expand task execution."""
        lines = []

        # Get expand configuration
        expand_config = task_spec.expand.dict(exclude_none=True)
        expand_field, expand_value = next(iter(expand_config.items()))

        if expand_field == "max_parallel":
            raise ValueError("max_parallel is not a valid expand field name")

        # Determine the source of iteration data
        if isinstance(expand_value, str) and expand_value.startswith("+"):
            # Reference to previous task result
            ref_task = expand_value[1:]
            iter_source = f'results["{ref_task}"]'
        else:
            # Static list
            iter_source = repr(expand_value)

        # Generate task inputs (excluding the expand field)
        if task_spec.inputs:
            lines.append("    base_task_inputs = {")
            for input_name, input_spec in task_spec.inputs.items():
                if input_name != expand_field:  # Skip the expand field
                    if isinstance(input_spec, str) and input_spec.startswith("+"):
                        ref_task = input_spec[1:]
                        lines.append(f'        "{input_name}": results["{ref_task}"],')
                    else:
                        lines.append(f'        "{input_name}": {repr(input_spec)},')
            lines.append("    }")
        else:
            lines.append("    base_task_inputs = {}")

        # Call the task function
        try:
            module_path, func_name = task_spec.function.rsplit(".", 1)
            function_alias = f"{task_name}_task"
        except ValueError:
            function_alias = task_spec.function

        # Generate foreach loop
        lines.extend([
            f"    # Foreach loop over {expand_field}",
            f'    {task_name}_results = []',
            f"    for {expand_field}_value in {iter_source}:",
            "        # Create task inputs for this iteration",
            "        iteration_inputs = base_task_inputs.copy()",
            f'        iteration_inputs["{expand_field}"] = {expand_field}_value',
            "",
            f"        # Execute task for this iteration",
            f"        iteration_result = {function_alias}(**iteration_inputs)",
            f"        {task_name}_results.append(iteration_result)",
            f'        log.info("Completed {task_name} iteration for {expand_field}=" + str(iteration_inputs["{expand_field}"]))',
            "",
            f'    results["{task_name}"] = {task_name}_results',
            f'    log.info("Completed all iterations for task: {task_name}")',
        ])

        return lines




def validate_yaml_workflow(yaml_path: Union[str, Path]) -> bool:
    """Validate a YAML workflow configuration without executing it.

    Args:
        yaml_path: Path to the YAML configuration file

    Returns:
        True if valid, raises exception if invalid
    """
    parser = YAMLWorkflowParser()
    parser.parse_file(yaml_path)
    return True


def generate_python_from_yaml(yaml_path: Union[str, Path], output_path: Optional[Union[str, Path]] = None) -> str:
    """Generate standalone Python code from a YAML workflow.

    Args:
        yaml_path: Path to the YAML configuration file
        output_path: Optional output path for generated Python file

    Returns:
        The generated Python code as a string
    """
    parser = YAMLWorkflowParser()
    parser.parse_file(yaml_path)
    return parser.generate_python_code(output_path)


if __name__ == "__main__":
    import argparse
    import sys

    parser_cli = argparse.ArgumentParser(
        description="Generate standalone Python code from YAML workflows"
    )
    parser_cli.add_argument(
        "yaml_file",
        help="Path to the YAML workflow file"
    )
    parser_cli.add_argument(
        "-o", "--output",
        help="Output path for generated Python file (default: generated_{name}.py)"
    )
    parser_cli.add_argument(
        "--validate-only",
        action="store_true",
        help="Only validate the YAML file without generating code"
    )

    args = parser_cli.parse_args()

    try:
        if args.validate_only:
            validate_yaml_workflow(args.yaml_file)
            print(f"✅ YAML workflow '{args.yaml_file}' is valid!")
        else:
            generated_code = generate_python_from_yaml(args.yaml_file, args.output)
            print(f"✅ Generated Python code from '{args.yaml_file}'")
            if args.output:
                print(f"📁 Output written to: {args.output}")
            else:
                # Default output path
                yaml_name = Path(args.yaml_file).stem
                output_path = f"generated_{yaml_name}.py"
                print(f"📁 Output written to: {output_path}")

    except Exception as e:
        print(f"❌ Error: {e}")
        sys.exit(1)