"""Dynamic task functionality for Uniflow.

This module provides support for dynamic task generation using foreach/expand patterns
and conditional logic. It extends the core decorator system to support YAML-based
workflow definitions with dynamic task creation.

Key features:
- DynamicTaskFunction: Creates multiple task instances from a single definition
- ExpandTask: Handles foreach/expand patterns over lists
- ConditionalTask: Implements if-else conditional logic
- CollectorTask: Aggregates results from multiple dynamic tasks
- YAML workflow parsing and execution

Example:
    Dynamic task expanding over a list::

        @expand_task(over=["item1", "item2", "item3"])
        @task(config=RayTask(head_cpu=2))
        def process_item(item: str) -> dict:
            return {"processed": item}

    Conditional task execution::

        @conditional_task(
            condition=lambda data: data["quality"] > 0.8,
            on_true="train_model",
            on_false="clean_data"
        )
        @task(config=RayTask())
        def quality_check(data: dict) -> dict:
            return {"quality": calculate_quality(data)}
"""

import ast
import inspect
import logging
import threading
from abc import ABC, abstractmethod
from functools import update_wrapper, wraps
from typing import (
    Any, Callable, Dict, Generic, List, Optional, TypeVar, Union
)

from michelangelo.uniflow.core.decorator import TaskFunction, task_context
from michelangelo.uniflow.core.io_registry import IORegistry, default_io
from michelangelo.uniflow.core.ref import Ref, ref, unref
from michelangelo.uniflow.core.task_config import TaskConfig
from michelangelo.uniflow.core.image_spec import ImageSpec

if True:  # sys.version_info < (3, 10):
    from typing_extensions import ParamSpec
else:
    from typing import ParamSpec

P = ParamSpec("P")
R = TypeVar("R")

log = logging.getLogger(__name__)


class DynamicExecutionContext:
    """Context manager for dynamic task execution.

    Tracks the execution state for dynamic workflows including:
    - Current dynamic task instances being executed
    - Results from completed dynamic tasks
    - Conditional branch states
    - Task dependencies and execution order
    """

    def __init__(self):
        self.dynamic_tasks = {}  # task_name -> list of task instances
        self.task_results = {}  # task_name -> list of results
        self.conditions = {}  # condition_name -> boolean result
        self.execution_order = []  # track execution sequence

    def register_dynamic_task(self, task_name: str, instances: List[TaskFunction]):
        """Register dynamic task instances."""
        self.dynamic_tasks[task_name] = instances
        self.task_results[task_name] = []

    def add_result(self, task_name: str, result: Any):
        """Add result from a dynamic task instance."""
        if task_name not in self.task_results:
            self.task_results[task_name] = []
        self.task_results[task_name].append(result)

    def get_results(self, task_name: str) -> List[Any]:
        """Get all results from a dynamic task."""
        return self.task_results.get(task_name, [])

    def set_condition(self, condition_name: str, result: bool):
        """Set the result of a conditional check."""
        self.conditions[condition_name] = result

    def get_condition(self, condition_name: str) -> Optional[bool]:
        """Get the result of a conditional check."""
        return self.conditions.get(condition_name)


# Global dynamic execution context
dynamic_context = threading.local()
dynamic_context.execution = None


class DynamicTaskFunction(Generic[P, R]):
    """Base class for dynamic task functions.

    Extends TaskFunction to support dynamic task generation patterns
    including foreach/expand and conditional execution.
    """

    def __init__(
        self,
        base_task: TaskFunction[P, R],
        dynamic_type: str,
        **kwargs
    ):
        """Initialize dynamic task function.

        Args:
            base_task: The underlying TaskFunction to wrap
            dynamic_type: Type of dynamic behavior ("expand", "conditional", "collect")
            **kwargs: Additional configuration for the specific dynamic type
        """
        self.base_task = base_task
        self.dynamic_type = dynamic_type
        self.config = kwargs
        update_wrapper(self, base_task)

    @abstractmethod
    def execute(self, *args: P.args, **kwargs: P.kwargs) -> R:
        """Execute the dynamic task logic.

        Subclasses must implement this method to define their specific
        dynamic behavior (expand, conditional, etc.).
        """
        pass

    def __call__(self, *args: P.args, **kwargs: P.kwargs) -> R:
        """Execute the dynamic task with lifecycle management."""
        if dynamic_context.execution is None:
            dynamic_context.execution = DynamicExecutionContext()

        return self.execute(*args, **kwargs)


class ExpandTaskFunction(DynamicTaskFunction[P, R]):
    """Dynamic task that expands over a list of inputs (foreach pattern).

    Creates multiple instances of the wrapped task, one for each item in the
    expand list. Can expand over static lists or outputs from previous tasks.
    """

    def __init__(
        self,
        base_task: TaskFunction[P, R],
        over: Union[List[Any], str],
        expand_arg: str = "item",
        max_parallel: Optional[int] = None,
    ):
        """Initialize expand task.

        Args:
            base_task: The task to expand
            over: List to expand over, or reference to previous task output ("+task_name")
            expand_arg: Name of the argument to pass each expanded item to
            max_parallel: Maximum number of parallel executions (None for unlimited)
        """
        super().__init__(base_task, "expand", over=over, expand_arg=expand_arg, max_parallel=max_parallel)
        self.over = over
        self.expand_arg = expand_arg
        self.max_parallel = max_parallel

    def execute(self, *args: P.args, **kwargs: P.kwargs) -> List[R]:
        """Execute the task for each item in the expand list.

        Returns:
            List of results, one for each expanded execution
        """
        # Resolve the list to expand over
        expand_list = self._resolve_expand_list(*args, **kwargs)

        # Create task instances
        task_instances = []
        results = []

        for i, item in enumerate(expand_list):
            # Create a copy of kwargs with the expanded item
            instance_kwargs = kwargs.copy()
            instance_kwargs[self.expand_arg] = item

            # Create task instance with unique alias
            instance_alias = f"{self.base_task._alias or self.base_task._fn.__name__}_{i}"
            task_instance = self.base_task.with_overrides(alias=instance_alias)
            task_instances.append(task_instance)

            # Execute the task instance
            log.info(f"Executing expand task instance {instance_alias} with {self.expand_arg}={item}")
            result = task_instance(*args, **instance_kwargs)
            results.append(result)

        # Register in dynamic context
        task_name = self.base_task._alias or self.base_task._fn.__name__
        dynamic_context.execution.register_dynamic_task(task_name, task_instances)
        for result in results:
            dynamic_context.execution.add_result(task_name, result)

        return results

    def _resolve_expand_list(self, *args, **kwargs) -> List[Any]:
        """Resolve the list to expand over.

        Supports both static lists and references to previous task outputs.
        """
        if isinstance(self.over, list):
            # Static list
            return self.over
        elif isinstance(self.over, str) and self.over.startswith("+"):
            # Reference to previous task output
            task_name = self.over[1:]  # Remove the "+" prefix
            if dynamic_context.execution and task_name in dynamic_context.execution.task_results:
                # Get results from previous task
                results = dynamic_context.execution.get_results(task_name)
                if len(results) == 1:
                    # Single result - assume it's a list to expand over
                    result = unref(results[0], self.base_task._io)
                    if isinstance(result, list):
                        return result
                    else:
                        raise ValueError(f"Task {task_name} output is not a list: {type(result)}")
                else:
                    # Multiple results - expand over the results themselves
                    return [unref(result, self.base_task._io) for result in results]
            else:
                # Check if the reference is available from YAML workflow context
                # This happens when the dynamic task is called from YAML parser
                if hasattr(self, '_yaml_context_results'):
                    yaml_results = getattr(self, '_yaml_context_results')
                    if task_name in yaml_results:
                        result = yaml_results[task_name]
                        if isinstance(result, list):
                            return result
                        else:
                            raise ValueError(f"Task {task_name} output is not a list: {type(result)}")
                raise ValueError(f"Cannot resolve expand reference: {self.over}")
        else:
            raise ValueError(f"Invalid expand specification: {self.over}")


class ConditionalTaskFunction(DynamicTaskFunction[P, R]):
    """Dynamic task that executes conditionally based on runtime evaluation.

    Evaluates a condition and executes different tasks based on the result.
    Supports both simple field comparisons and complex boolean expressions.
    """

    def __init__(
        self,
        base_task: TaskFunction[P, R],
        condition: Union[Callable, str, Dict],
        on_true: Optional[str] = None,
        on_false: Optional[str] = None,
    ):
        """Initialize conditional task.

        Args:
            base_task: The task that performs the condition check
            condition: Condition to evaluate (function, expression, or config dict)
            on_true: Task to execute if condition is true
            on_false: Task to execute if condition is false
        """
        super().__init__(
            base_task, "conditional",
            condition=condition, on_true=on_true, on_false=on_false
        )
        self.condition = condition
        self.on_true = on_true
        self.on_false = on_false

    def execute(self, *args: P.args, **kwargs: P.kwargs) -> R:
        """Execute the base task and evaluate condition.

        Returns:
            The result of the base task execution along with condition state
        """
        # Execute the base task to get data for condition evaluation
        base_result = self.base_task(*args, **kwargs)

        # Evaluate the condition
        condition_result = self._evaluate_condition(base_result, *args, **kwargs)

        # Store condition result in context
        condition_name = f"{self.base_task._alias or self.base_task._fn.__name__}_condition"
        dynamic_context.execution.set_condition(condition_name, condition_result)

        log.info(f"Condition {condition_name} evaluated to {condition_result}")

        # Add condition result to the base result
        if isinstance(base_result, dict):
            base_result = base_result.copy()
            base_result["condition_result"] = condition_result
        else:
            # Wrap non-dict results
            base_result = {
                "original_result": base_result,
                "condition_result": condition_result
            }

        return base_result

    def _evaluate_condition(self, base_result: Any, *args, **kwargs) -> bool:
        """Evaluate the condition against the base result."""
        if callable(self.condition):
            # Function-based condition
            return bool(self.condition(base_result))
        elif isinstance(self.condition, str):
            # Expression-based condition
            return self._evaluate_expression(self.condition, base_result, *args, **kwargs)
        elif isinstance(self.condition, dict):
            # Dictionary-based condition (field, operator, value)
            return self._evaluate_dict_condition(self.condition, base_result)
        else:
            raise ValueError(f"Unsupported condition type: {type(self.condition)}")

    def _evaluate_expression(self, expression: str, result: Any, *args, **kwargs) -> bool:
        """Evaluate a string expression as a boolean."""
        # Simple implementation - in production would use a proper expression parser
        # For now, support basic comparisons
        if " > " in expression:
            field, value = expression.split(" > ", 1)
            field = field.strip()
            value = float(value.strip())
            return self._get_field_value(field, result) > value
        elif " < " in expression:
            field, value = expression.split(" < ", 1)
            field = field.strip()
            value = float(value.strip())
            return self._get_field_value(field, result) < value
        elif " == " in expression:
            field, value = expression.split(" == ", 1)
            field = field.strip()
            value = value.strip().strip('"\'')
            return self._get_field_value(field, result) == value
        else:
            raise ValueError(f"Unsupported expression: {expression}")

    def _evaluate_dict_condition(self, condition_dict: Dict, result: Any) -> bool:
        """Evaluate a dictionary-based condition."""
        field = condition_dict.get("field")
        operator = condition_dict.get("operator", "==")
        value = condition_dict.get("value")

        field_value = self._get_field_value(field, result)

        if operator == ">":
            return field_value > value
        elif operator == "<":
            return field_value < value
        elif operator == ">=":
            return field_value >= value
        elif operator == "<=":
            return field_value <= value
        elif operator == "==":
            return field_value == value
        elif operator == "!=":
            return field_value != value
        else:
            raise ValueError(f"Unsupported operator: {operator}")

    def _get_field_value(self, field: str, result: Any) -> Any:
        """Get a field value from the result."""
        if field is None:
            return result

        # Dereference if it's a Ref
        if isinstance(result, Ref):
            result = unref(result, self.base_task._io)

        if isinstance(result, dict):
            return result.get(field)
        elif hasattr(result, field):
            return getattr(result, field)
        else:
            raise ValueError(f"Field {field} not found in result {type(result)}")


class CollectorTaskFunction(DynamicTaskFunction[P, R]):
    """Task that collects and aggregates results from multiple dynamic tasks.

    Gathers outputs from expand tasks or other dynamic tasks and provides
    various aggregation strategies (list, sum, max, min, custom function).
    """

    def __init__(
        self,
        base_task: TaskFunction[P, R],
        collect_from: Union[str, List[str]],
        aggregation_strategy: str = "list",
        aggregation_field: Optional[str] = None,
        aggregation_func: Optional[Callable] = None,
    ):
        """Initialize collector task.

        Args:
            base_task: The task that performs the collection/aggregation
            collect_from: Task name(s) to collect results from ("+task_name" format)
            aggregation_strategy: How to aggregate results ("list", "sum", "max", "min", "custom")
            aggregation_field: Field to aggregate on (for dict results)
            aggregation_func: Custom aggregation function
        """
        super().__init__(
            base_task, "collect",
            collect_from=collect_from,
            aggregation_strategy=aggregation_strategy,
            aggregation_field=aggregation_field,
            aggregation_func=aggregation_func
        )
        self.collect_from = collect_from if isinstance(collect_from, list) else [collect_from]
        self.aggregation_strategy = aggregation_strategy
        self.aggregation_field = aggregation_field
        self.aggregation_func = aggregation_func

    def execute(self, *args: P.args, **kwargs: P.kwargs) -> R:
        """Execute collection and aggregation."""
        # Collect results from specified tasks
        collected_results = []

        for task_ref in self.collect_from:
            if task_ref.startswith("+"):
                task_name = task_ref[1:]
                if dynamic_context.execution and task_name in dynamic_context.execution.task_results:
                    task_results = dynamic_context.execution.get_results(task_name)
                    # Dereference the results
                    dereferenced_results = [unref(result, self.base_task._io) for result in task_results]
                    collected_results.extend(dereferenced_results)
                elif hasattr(self, '_yaml_context_results'):
                    # Check YAML workflow context
                    yaml_results = getattr(self, '_yaml_context_results')
                    if task_name in yaml_results:
                        result = yaml_results[task_name]
                        # If it's a list of results (from expand task), extend with all items
                        if isinstance(result, list):
                            collected_results.extend(result)
                        else:
                            # Single result, add as single item
                            collected_results.append(result)
                    else:
                        log.warning(f"No results found for task: {task_name}")
                else:
                    log.warning(f"No results found for task: {task_name}")
            else:
                raise ValueError(f"Invalid collect reference: {task_ref}")

        # Apply aggregation strategy
        aggregated_result = self._aggregate_results(collected_results)

        # Execute the base task with aggregated results
        kwargs["collected_results"] = aggregated_result
        return self.base_task(*args, **kwargs)

    def _aggregate_results(self, results: List[Any]) -> Any:
        """Aggregate the collected results."""
        if not results:
            return []

        if self.aggregation_strategy == "list":
            return results
        elif self.aggregation_strategy == "sum":
            if self.aggregation_field:
                return sum(self._get_field_value(self.aggregation_field, r) for r in results)
            else:
                return sum(results)
        elif self.aggregation_strategy == "max":
            if self.aggregation_field:
                # Return the full object that has the max value for the field
                return max(results, key=lambda r: self._get_field_value(self.aggregation_field, r))
            else:
                return max(results)
        elif self.aggregation_strategy == "min":
            if self.aggregation_field:
                # Return the full object that has the min value for the field
                return min(results, key=lambda r: self._get_field_value(self.aggregation_field, r))
            else:
                return min(results)
        elif self.aggregation_strategy == "custom" and self.aggregation_func:
            return self.aggregation_func(results)
        else:
            raise ValueError(f"Unsupported aggregation strategy: {self.aggregation_strategy}")

    def _get_field_value(self, field: str, result: Any) -> Any:
        """Get a field value from the result (same as ConditionalTaskFunction)."""
        if isinstance(result, dict):
            return result.get(field)
        elif hasattr(result, field):
            return getattr(result, field)
        else:
            raise ValueError(f"Field {field} not found in result {type(result)}")


# Decorator functions for creating dynamic tasks

def expand_task(
    over: Union[List[Any], str],
    expand_arg: str = "item",
    max_parallel: Optional[int] = None,
):
    """Decorator to create an expand/foreach dynamic task.

    Args:
        over: List to expand over, or reference to previous task output ("+task_name")
        expand_arg: Name of the argument to pass each expanded item to
        max_parallel: Maximum number of parallel executions

    Example:
        @expand_task(over=["file1.txt", "file2.txt", "file3.txt"], expand_arg="filename")
        @task(config=RayTask(head_cpu=2))
        def process_file(filename: str) -> dict:
            return {"processed": filename}
    """
    def decorator(task_fn: TaskFunction[P, R]) -> ExpandTaskFunction[P, R]:
        return ExpandTaskFunction(task_fn, over, expand_arg, max_parallel)

    return decorator


def conditional_task(
    condition: Union[Callable, str, Dict],
    on_true: Optional[str] = None,
    on_false: Optional[str] = None,
):
    """Decorator to create a conditional dynamic task.

    Args:
        condition: Condition to evaluate (function, expression, or config dict)
        on_true: Task to execute if condition is true
        on_false: Task to execute if condition is false

    Example:
        @conditional_task(
            condition=lambda result: result["quality"] > 0.8,
            on_true="train_model",
            on_false="clean_data"
        )
        @task(config=RayTask())
        def quality_check(data: dict) -> dict:
            return {"quality": calculate_quality(data)}
    """
    def decorator(task_fn: TaskFunction[P, R]) -> ConditionalTaskFunction[P, R]:
        return ConditionalTaskFunction(task_fn, condition, on_true, on_false)

    return decorator


def collect_task(
    collect_from: Union[str, List[str]],
    aggregation_strategy: str = "list",
    aggregation_field: Optional[str] = None,
    aggregation_func: Optional[Callable] = None,
):
    """Decorator to create a collector dynamic task.

    Args:
        collect_from: Task name(s) to collect results from ("+task_name" format)
        aggregation_strategy: How to aggregate results ("list", "sum", "max", "min", "custom")
        aggregation_field: Field to aggregate on (for dict results)
        aggregation_func: Custom aggregation function

    Example:
        @collect_task(
            collect_from="+preprocess_files",
            aggregation_strategy="list"
        )
        @task(config=RayTask())
        def merge_results(collected_results: List[dict]) -> dict:
            return {"merged": collected_results}
    """
    def decorator(task_fn: TaskFunction[P, R]) -> CollectorTaskFunction[P, R]:
        return CollectorTaskFunction(
            task_fn, collect_from, aggregation_strategy,
            aggregation_field, aggregation_func
        )

    return decorator