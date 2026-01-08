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
from michelangelo.uniflow.core.dynamic import (
    DynamicExecutionContext, expand_task, conditional_task, collect_task,
    dynamic_context
)
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
    function: str
    description: Optional[str] = None
    config: Optional[TaskConfigSpec] = None
    inputs: Optional[Dict[str, str]] = None
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
    tasks: Dict[str, TaskSpec]


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
        self._validate_workflow()
        return self.config

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

    def create_workflow_function(self) -> callable:
        """Create an executable workflow function from the configuration.

        Returns:
            A decorated workflow function that can be executed
        """
        if not self.config:
            raise ValueError("No configuration loaded")

        # Create all task functions
        self._create_task_functions()

        # Create the workflow function with environment setup
        def yaml_workflow(**kwargs):
            """Generated workflow function from YAML configuration."""
            import os

            # Set up environment for local execution
            os.environ["UF_LOCAL_RUN"] = "1"
            if not os.environ.get("UF_STORAGE_URL"):
                os.environ["UF_STORAGE_URL"] = os.path.expanduser("~/uf_storage")

            # Set up dynamic execution context
            dynamic_context.execution = DynamicExecutionContext()

            try:
                # Execute tasks in topological order
                execution_order = self._get_execution_order()
                results = {}

                for task_name in execution_order:
                    log.info(f"Executing task: {task_name}")
                    task_func = self.task_functions[task_name]

                    # Get task inputs (resolve references)
                    task_inputs = self._resolve_task_inputs(task_name, results)

                    # Pass workflow results context to dynamic tasks
                    if hasattr(task_func, 'dynamic_type'):
                        task_func._yaml_context_results = results

                    # Execute the task
                    result = task_func(**task_inputs)
                    results[task_name] = result

                    log.info(f"Completed task: {task_name}")

                return results

            finally:
                # Clean up dynamic context
                dynamic_context.execution = None

        # Add workflow decorator and mark as YAML-generated to bypass build validation
        workflow_func = workflow()(yaml_workflow)
        workflow_func._uf_yaml_config = self.config
        workflow_func._uf_yaml_generated = True  # Mark to skip build validation

        return workflow_func

    def _create_task_functions(self):
        """Create TaskFunction instances for all tasks in the configuration."""
        for task_name, task_spec in self.config.tasks.items():
            # Import the function
            func = self._import_function(task_spec.function)

            # Create task configuration
            task_config = self._create_task_config(task_spec)

            # Create image spec if specified
            image_spec = None
            if task_spec.image_spec:
                image_spec = ImageSpec(container_image=task_spec.image_spec)
            elif self.config.defaults and self.config.defaults.image_spec:
                image_spec = ImageSpec(container_image=self.config.defaults.image_spec)

            # Create base task
            base_task = TaskFunction(
                fn=func,
                config=task_config,
                alias=task_name,
                io=default_io,  # Use default IO registry
                cache_enabled=task_spec.cache_enabled or (
                    self.config.defaults.cache_enabled if self.config.defaults else False
                ),
                cache_version=task_spec.cache_version or (
                    self.config.defaults.cache_version if self.config.defaults else None
                ),
                retry_attempts=task_spec.retry_attempts or (
                    self.config.defaults.retry_attempts if self.config.defaults else 0
                ),
                image_spec=image_spec
            )

            # Apply dynamic decorators if specified
            final_task = self._apply_dynamic_decorators(base_task, task_spec)

            self.task_functions[task_name] = final_task

    def _import_function(self, function_path: str) -> callable:
        """Import a function from a module path.

        Args:
            function_path: Dotted path to the function (e.g., "src.data.load_data")

        Returns:
            The imported function
        """
        try:
            module_path, func_name = function_path.rsplit(".", 1)
            module = importlib.import_module(module_path)
            return getattr(module, func_name)
        except (ImportError, AttributeError) as e:
            raise ImportError(f"Cannot import function {function_path}: {e}")

    def _create_task_config(self, task_spec: TaskSpec) -> TaskConfig:
        """Create a TaskConfig instance from task specification."""
        if not task_spec.config:
            # Use a default RayTask configuration
            return RayTask()

        config_type = task_spec.config.type
        resources = task_spec.config.resources or ResourceConfig()

        if config_type == "RayTask":
            return RayTask(
                head_cpu=resources.cpu or 1,
                head_memory=resources.memory or "1Gi",
                head_gpu=resources.gpu or 0
            )
        elif config_type == "SparkTask":
            return SparkTask(
                driver_cpu=resources.cpu or 1,
                driver_memory=resources.memory or "1Gi",
                executor_cpu=resources.executor_cores or 1,
                executor_memory=resources.memory or "1Gi",
                executor_instances=resources.executor_instances or 1
            )
        else:
            raise ValueError(f"Unsupported task config type: {config_type}")

    def _apply_dynamic_decorators(self, base_task: TaskFunction, task_spec: TaskSpec) -> TaskFunction:
        """Apply dynamic decorators to a base task based on specification."""
        task_func = base_task

        # Apply expand decorator if specified
        if task_spec.expand:
            # Find the expand field and value
            expand_config = task_spec.expand.dict(exclude_none=True)
            if len(expand_config) != 1:
                raise ValueError(f"Expand config must have exactly one field, got: {expand_config}")

            expand_field, expand_value = next(iter(expand_config.items()))
            if expand_field == "max_parallel":
                raise ValueError("max_parallel is not a valid expand field name")

            task_func = expand_task(
                over=expand_value,
                expand_arg=expand_field,
                max_parallel=task_spec.expand.max_parallel
            )(task_func)

        # Apply conditional decorator if specified
        if task_spec.condition:
            condition_dict = task_spec.condition.dict(exclude_none=True)
            task_func = conditional_task(
                condition=condition_dict,
                on_true=task_spec.condition.on_true,
                on_false=task_spec.condition.on_false
            )(task_func)

        # Apply collect decorator if specified
        if task_spec.collect:
            task_func = collect_task(
                collect_from=task_spec.collect.from_task,
                aggregation_strategy=task_spec.collect.strategy,
                aggregation_field=task_spec.collect.field
            )(task_func)

        return task_func

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

    def _resolve_task_inputs(self, task_name: str, previous_results: Dict[str, Any]) -> Dict[str, Any]:
        """Resolve task inputs, handling references to previous task outputs."""
        task_spec = self.config.tasks[task_name]
        inputs = {}

        # Add inputs from task specification
        if task_spec.inputs:
            for input_name, input_spec in task_spec.inputs.items():
                if isinstance(input_spec, str) and input_spec.startswith("+"):
                    # Reference to previous task output
                    ref_task = input_spec[1:]
                    if ref_task in previous_results:
                        inputs[input_name] = previous_results[ref_task]
                    else:
                        raise ValueError(f"Referenced task '{ref_task}' not found in previous results")
                else:
                    # Static value
                    inputs[input_name] = input_spec

        return inputs


def load_yaml_workflow(yaml_path: Union[str, Path]) -> callable:
    """Load a workflow from a YAML configuration file.

    Args:
        yaml_path: Path to the YAML configuration file

    Returns:
        An executable workflow function

    Example:
        >>> workflow_func = load_yaml_workflow("pipeline.yml")
        >>> result = workflow_func()
    """
    parser = YAMLWorkflowParser()
    parser.parse_file(yaml_path)
    return parser.create_workflow_function()


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