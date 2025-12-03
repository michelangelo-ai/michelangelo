"""Task configuration and dependencies for Uniflow workflows.

This module defines the core abstractions for task configuration in Uniflow.
TaskConfig subclasses specify resource requirements, execution environments,
and lifecycle hooks for tasks. The Dependencies class tracks external
references during workflow transpilation.

Key components:
- TaskConfig: Abstract base for all task configurations (RayTask, SparkTask, etc.)
- TaskBinding: Links task configurations to their Starlark orchestration functions
- Dependencies: Tracks Starlark and Python dependencies during transpilation

Example:
    Creating a custom task configuration::

        from pathlib import Path
        from michelangelo.uniflow.core.task_config import TaskConfig, TaskBinding

        @dataclass
        class MyTask(TaskConfig):
            cpu: int = 1
            memory: str = "4Gi"

            def get_binding(self) -> TaskBinding:
                return TaskBinding(
                    star_file=Path(__file__).parent / "my_task.star",
                    function="my_task",
                    export="__my_task"
                )

            def pre_run(self):
                # Setup code
                pass

            def post_run(self):
                # Cleanup code
                pass
"""

import ast
import dataclasses
from abc import ABC, abstractmethod
from pathlib import Path
from typing import Callable


class Dependencies:
    """Collection of dependencies required to execute a transpiled workflow.

    Tracks three types of dependencies encountered during workflow transpilation:

    1. **Starlark Attributes**: Functions and attributes defined in .star files
    2. **Starlark Plugins**: Built-in functions from the @plugin module
    3. **Python Functions**: Workflow and utility functions defined in Python

    These dependencies are used to generate load statements and package all
    necessary code into the workflow tarball.

    Attributes:
        star_attributes: Maps alias to (star_file_path, attribute_name) tuples.
        star_plugins: Maps alias to plugin identifier.
        py_functions: Maps alias to Python callable objects.

    Example:
        >>> deps = Dependencies()
        >>> deps.add_star_attribute("__ray_task", Path("ray.star"), "ray_task")
        >>> deps.add_py_function("helper", my_helper_function)
        >>> len(deps.py_functions)
        1
    """

    def __init__(self):
        """Initialize an empty dependency collection."""
        self.star_attributes: dict[str, tuple[Path, str]] = {}
        self.star_plugins: dict[str, str] = {}
        self.py_functions: dict[str, Callable] = {}

    def add_star_attribute(self, alias: str, star_file: Path, attribute: str):
        """Add a Starlark attribute dependency.

        Registers a function or attribute from a .star file that needs to be
        loaded in the generated Starlark code.

        Args:
            alias: The name to use when loading the attribute.
            star_file: Path to the .star file containing the attribute.
            attribute: Name of the attribute to load from the file.

        Example:
            >>> deps.add_star_attribute("__ray_task", Path("ray.star"), "ray_task")
            >>> # Generates: load("ray.star", __ray_task="ray_task")
        """
        star_file = star_file.resolve()
        self.star_attributes[alias] = (
            star_file,
            attribute,
        )

    def add_star_plugin(self, alias: str, plugin_id: str):
        """Add a Starlark plugin dependency.

        Registers a plugin from the @plugin module that needs to be loaded.

        Args:
            alias: The alias to use for the plugin.
            plugin_id: The plugin identifier (e.g., "chronon").

        Example:
            >>> deps.add_star_plugin("__chronon__", "chronon")
            >>> # Generates: load("@plugin", __chronon__="chronon")
        """
        self.star_plugins[alias] = plugin_id

    def add_py_function(self, alias: str, fn: Callable):
        """Add a Python function dependency.

        Registers a Python function (typically a workflow or helper) that needs
        to be transpiled and included in the package.

        Args:
            alias: The name to use for the function.
            fn: The Python callable to include.

        Example:
            >>> def my_workflow():
            ...     pass
            >>> deps.add_py_function("my_workflow", my_workflow)
        """
        self.py_functions[alias] = fn


@dataclasses.dataclass
class TaskBinding:
    """Binding between a TaskConfig and its Starlark orchestration function.

    Links a Python task configuration class to the Starlark function that
    handles the actual task execution. Used during transpilation to generate
    proper load statements.

    The binding generates a Starlark load statement in the format::

        load(star_file, export="function")

    Attributes:
        star_file: Path to the .star file containing the orchestration function.
        function: Name of the Starlark function in the file.
        export: Name to use when importing the function (typically with __ prefix).

    Example:
        >>> binding = TaskBinding(
        ...     star_file=Path("ray/task.star"),
        ...     function="ray_task",
        ...     export="__ray_task"
        ... )
        >>> # Generates: load("ray/task.star", __ray_task="ray_task")
    """

    star_file: Path
    """Path to the Starlark file defining the orchestration function."""

    function: str
    """Name of the Starlark function in the file."""

    export: str
    """Exported name for the function (used in load statements)."""


class TaskConfig(ABC):
    """Abstract base class for task execution configurations.

    TaskConfig subclasses define how tasks are executed by specifying:

    - **Configuration properties**: Resource requirements (CPU, memory, GPU, etc.)
    - **Lifecycle hooks**: Code to run before and after task execution
    - **Starlark binding**: Link to the orchestration function in .star files

    Subclasses should be dataclasses with fields representing configuration
    options. These fields are automatically converted to Starlark keywords.

    Example:
        >>> from dataclasses import dataclass
        >>> @dataclass
        ... class RayTask(TaskConfig):
        ...     head_cpu: int = 1
        ...     head_memory: str = "4Gi"
        ...
        ...     def get_binding(self) -> TaskBinding:
        ...         return TaskBinding(
        ...             star_file=Path(__file__).parent / "task.star",
        ...             function="ray_task",
        ...             export="__ray_task"
        ...         )
        ...
        ...     def pre_run(self):
        ...         # Initialize Ray cluster
        ...         pass
        ...
        ...     def post_run(self):
        ...         # Cleanup Ray resources
        ...         pass
    """

    @abstractmethod
    def get_binding(self) -> TaskBinding:
        """Get the TaskBinding linking this config to its Starlark function.

        Returns the binding that specifies which Starlark file and function
        handle task orchestration for this configuration type.

        Returns:
            TaskBinding specifying the star file, function name, and export name.

        Example:
            >>> def get_binding(self) -> TaskBinding:
            ...     return TaskBinding(
            ...         star_file=Path(__file__).parent / "ray_task.star",
            ...         function="ray_task",
            ...         export="__ray_task"
            ...     )

        Note:
            Use a double underscore prefix for export names (e.g., "__ray_task")
            to avoid naming conflicts with user code.
        """
        raise NotImplementedError

    @classmethod
    @abstractmethod
    def get_config_binding(cls) -> TaskBinding:
        """Get the TaskBinding for the configuration class itself.

        Similar to get_binding() but called on the class rather than instance.
        Used during transpilation when the TaskConfig class is referenced.

        Returns:
            TaskBinding for the configuration's Starlark representation.

        Example:
            >>> @classmethod
            ... def get_config_binding(cls) -> TaskBinding:
            ...     return TaskBinding(
            ...         star_file=Path(__file__).parent / "config.star",
            ...         function="ray_config",
            ...         export="__ray_config"
            ...     )
        """
        raise NotImplementedError

    @abstractmethod
    def pre_run(self):
        """Execute setup code before the task function runs.

        Called by the task execution framework before invoking the user's
        task function. Use this to initialize resources, set up environments,
        or perform validation.

        Example:
            >>> def pre_run(self):
            ...     # Initialize Spark session
            ...     self.spark = SparkSession.builder.getOrCreate()
        """
        raise NotImplementedError

    @abstractmethod
    def post_run(self):
        """Execute cleanup code after the task function completes.

        Called by the task execution framework after the user's task function
        returns, even if an exception occurred. Use this to release resources,
        close connections, or perform cleanup.

        Example:
            >>> def post_run(self):
            ...     # Stop Spark session
            ...     if self.spark:
            ...         self.spark.stop()
        """
        raise NotImplementedError

    def to_keywords(self) -> list[ast.keyword]:
        """Convert configuration fields to AST keyword nodes.

        Generates AST keyword nodes from the dataclass fields, excluding any
        fields with None values. Used during transpilation to construct the
        Starlark function call with configuration parameters.

        Returns:
            List of ast.keyword nodes for non-None configuration fields.

        Example:
            For a RayTask with head_cpu=4 and head_memory="8Gi"::

                keywords = config.to_keywords()
                # Generates AST equivalent to:
                # __ray_task__(head_cpu=4, head_memory="8Gi")

        Note:
            Only includes fields with non-None values. This allows optional
            configuration parameters to be omitted from the generated code.
        """
        assert dataclasses.is_dataclass(self)
        res = []
        for f in dataclasses.fields(self):
            v = getattr(self, f.name)
            if v is not None:
                # Exclude None-valued properties from the keyword list.
                # Perhaps we should revise keyword exclusion logic. Instead of excluding None, we should exclude special "Undefined" values.
                # TODO: andrii: Consider using special "Undefined" marker object for keyword exclusion.
                k = ast.keyword(f.name, ast.Constant(v))
                res.append(k)
        return res
