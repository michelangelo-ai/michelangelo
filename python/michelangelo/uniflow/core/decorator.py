"""Task and workflow decorators for Uniflow.

This module provides the core decorators for defining tasks and workflows in Uniflow.
Tasks are units of computation that can be executed locally or distributed, while
workflows orchestrate multiple tasks together.

The @task decorator wraps functions with execution logic including caching, retry
handling, I/O management, and resource configuration. The @workflow decorator marks
functions as workflow entry points.

Example:
    Basic task definition::

        from michelangelo.uniflow.core import task
        from michelangelo.uniflow.plugins.ray import RayTask

        @task(config=RayTask(head_cpu=2, head_memory="4Gi"))
        def process_data(input_file: str) -> dict:
            # Process data and return results
            return {"status": "complete"}

    Workflow with multiple tasks::

        @workflow()
        def my_workflow():
            data = load_data()
            result = process_data(data)
            return result
"""

import ast
import inspect
import json
import logging
import sys
import threading
from functools import update_wrapper, wraps
from typing import Callable, Generic, Optional, TypeVar

import fsspec

from michelangelo.uniflow.core.codec import encoder
from michelangelo.uniflow.core.image_spec import ImageSpec
from michelangelo.uniflow.core.io_registry import IORegistry, default_io
from michelangelo.uniflow.core.ref import Ref, ref, unref
from michelangelo.uniflow.core.task_config import Dependencies, TaskConfig
from michelangelo.uniflow.core.utils import dot_path

if sys.version_info < (3, 10):
    from typing_extensions import ParamSpec
else:
    from typing import ParamSpec

P = ParamSpec("P")
R = TypeVar("R")

log = logging.getLogger(__name__)

task_context = threading.local()
task_context.config = None
task_context.alias = None

DEFAULT_RETRY_ATTEMPTS = 0


class TaskFunction(Generic[P, R]):
    """Executable task wrapper for decorated functions.

    TaskFunction wraps a callable with Uniflow execution logic including caching,
    retry handling, argument serialization/deserialization, and lifecycle hooks.
    It manages task context, resource configuration, and result persistence.

    This class is typically created by the @task decorator and should not be
    instantiated directly.

    Attributes:
        fn: The wrapped Python function.
        config: Task configuration specifying execution environment.
        alias: Optional alternative name for the task.
        io: I/O registry for serialization operations.
        cache_enabled: Whether to enable result caching.
        cache_version: Optional version identifier for cached results.
        retry_attempts: Number of times to retry failed executions.
        image_spec: Optional container image specification.

    Example:
        >>> @task(config=RayTask(head_cpu=1))
        ... def my_task(x: int) -> int:
        ...     return x * 2
        >>> result = my_task(5)  # Executes wrapped function
    """

    def __init__(
        self,
        *,
        fn: Callable[P, R],
        config: TaskConfig,
        alias: Optional[str],
        io: IORegistry,
        cache_enabled: bool = False,
        cache_version: Optional[str] = None,
        retry_attempts: int = DEFAULT_RETRY_ATTEMPTS,
        image_spec: Optional[ImageSpec] = None,
    ):
        """Initialize a TaskFunction.

        Args:
            fn: The function to wrap.
            config: Task configuration defining execution environment.
            alias: Optional alternative task name.
            io: I/O registry for serialization.
            cache_enabled: Enable result caching. Defaults to False.
            cache_version: Optional cache version identifier.
            retry_attempts: Number of retry attempts. Defaults to 0.
            image_spec: Optional container image specification.
        """
        self._fn = fn
        self._config = config
        self._alias = alias
        self._io = io
        self._cache_enabled = cache_enabled
        self._cache_version = cache_version
        self._retry_attempts = retry_attempts
        self._image_spec = image_spec

    @property
    def image_spec(self) -> Optional[ImageSpec]:
        """Get the container image specification for this task.

        Returns:
            The ImageSpec if specified, None otherwise.
        """
        return self._image_spec

    @property
    def fn(self) -> Callable[P, R]:
        """Get the wrapped function.

        Returns:
            The original decorated function.
        """
        return self._fn

    def __call__(self, *args: P.args, **kwargs: P.kwargs) -> R:
        """Execute the task with the given arguments.

        Manages the complete task execution lifecycle:
        1. Checks for nested task calls (executes directly)
        2. Logs task invocation and arguments
        3. Sets up task context
        4. Runs pre-execution hooks
        5. Deserializes input arguments
        6. Executes the wrapped function
        7. Serializes and persists results
        8. Runs post-execution hooks
        9. Cleans up task context

        Args:
            *args: Positional arguments to pass to the wrapped function.
            **kwargs: Keyword arguments to pass to the wrapped function.
                Special keyword _uf_result_url can be used to specify
                where to write the result.

        Returns:
            The result of executing the wrapped function, wrapped in a Ref.

        Raises:
            Any exception raised by the wrapped function or lifecycle hooks.
        """
        fn_path = dot_path(self._fn)
        if task_context.config is not None:
            # Execute nested task call as a regular function without processing.
            log.info("run nested task: %s", fn_path)
            return self._fn(*args, **kwargs)

        log.info("run task: %s", _in_the_box(fn_path))

        sig = inspect.signature(self._fn)
        log.info("signature: %s", sig)

        result_url = kwargs.pop("_uf_result_url", None)
        log.info("result_url: %s", result_url)

        for i, v in enumerate(args):
            log.info("arg[%d]: %r", i, v)

        for k, v in kwargs.items():
            log.info("kwarg[%s]: %r", k, v)

        task_context.config = self._config
        task_context.alias = self._alias

        def reset_task_context():
            """Reset task context to None."""
            task_context.config = None
            task_context.alias = None

        try:
            self._config.pre_run()
        except Exception:
            reset_task_context()
            raise

        try:
            args, kwargs = _unref_args(sig, args, kwargs, self._io)  # type: ignore[return-value]
            res = self._fn(*args, **kwargs)
            res = ref(res, self._io)
            if result_url:
                assert isinstance(result_url, str)
                write_task_result(result_url, res)

        finally:
            try:
                self._config.post_run()
            finally:
                reset_task_context()

        log.info("result: %r", res)
        return res  # type: ignore[return-value]

    def with_overrides(
        self,
        *,
        alias: Optional[str] = None,
        config: Optional[TaskConfig] = None,
        retry_attempts: Optional[int] = None,
    ) -> "TaskFunction[P, R]":
        """Create a new TaskFunction with overridden configuration.

        This method allows creating a variant of the task with different configuration
        while sharing the same function, I/O registry, and cache settings. Useful for
        running the same task with different resource allocations.

        Args:
            alias: Optional alternative task name. If not provided, uses original alias.
            config: Optional task configuration. If provided, this configuration will be
                merged with the original configuration. For example, if the original
                config specifies head_cpu=4 and head_memory="16Gi", and the new
                config specifies head_cpu=8, the result will have head_cpu=8 and
                head_memory="16Gi".
            retry_attempts: Optional retry count. If not provided, uses original value.

        Returns:
            A new TaskFunction instance with the specified overrides.

        Example:
            >>> @task(config=RayTask(head_cpu=2))
            ... def my_task(x):
            ...     return x * 2
            >>> high_cpu_task = my_task.with_overrides(
            ...     alias="my_task_8cpu",
            ...     config=RayTask(head_cpu=8)
            ... )
        """
        return TaskFunction(
            fn=self._fn,
            config=config or self._config,
            alias=alias or self._alias,
            io=self._io,
            cache_enabled=self._cache_enabled,
            cache_version=self._cache_version,
            retry_attempts=retry_attempts or self._retry_attempts,
        )

    def _transpile(self, dependencies: Dependencies) -> ast.AST:
        """Transpile task to Starlark AST for workflow compilation.

        Constructs an AST expression representing the task in Starlark format.
        The expression calls a task factory function with configuration parameters.

        The resulting expression follows the format::

            __task_factory__(alias="task_1", cpu=4, gpu=1, ...)

        Args:
            dependencies: Collection to register external dependencies.

        Returns:
            An AST Call node representing the task in Starlark.

        Example:
            For a task with RayTask config, generates::

                __ray_task__(
                    "my.module.task_function",
                    alias="my_task",
                    head_cpu=4,
                    head_memory="8Gi"
                )
        """
        # Register the task's Starlark Binding (Task Factory Function) in the
        # Dependencies collection.
        binding = self._config.get_binding()
        dependencies.add_star_attribute(
            binding.export,
            binding.star_file,
            binding.function,
        )

        # Construct AST keywords that represent configuration properties for
        # the Task Factory Function
        keywords = []
        if self._alias:
            k = ast.keyword("alias", ast.Constant(self._alias))
            keywords.append(k)

        keywords += self._config.to_keywords()

        keywords.append(ast.keyword("cache_enabled", ast.Constant(self._cache_enabled)))
        keywords.append(ast.keyword("cache_version", ast.Constant(self._cache_version)))
        keywords.append(
            ast.keyword("retry_attempts", ast.Constant(self._retry_attempts))
        )

        if self._image_spec:
            if self._image_spec.container_image:
                keywords.append(
                    ast.keyword(
                        "container_image",
                        ast.Constant(self._image_spec.container_image),
                    )
                )
            if self._image_spec.recipe:
                keywords.append(
                    ast.keyword("recipe", ast.Constant(self._image_spec.recipe))
                )

        # Construct and return AST Call node that calls the Task Factory
        # Function with the keywords.
        origin_fn = inspect.unwrap(self._fn)
        return ast.Call(
            func=ast.Name(id=binding.export, ctx=ast.Load()),
            args=[ast.Constant(dot_path(origin_fn))],
            keywords=keywords,
        )


def task(
    config: TaskConfig,
    alias: Optional[str] = None,
    io: IORegistry = default_io,
    cache_enabled: bool = False,
    cache_version: Optional[str] = None,
    retry_attempts: int = DEFAULT_RETRY_ATTEMPTS,
    image_spec: Optional[ImageSpec] = None,
):
    """Decorator for defining a Uniflow task.

    Wraps a function to make it executable as a Uniflow task with caching,
    retry handling, and resource configuration. Tasks can be executed locally
    or distributed across Ray/Spark clusters depending on the config.

    Args:
        config: Task configuration defining execution environment (e.g., RayTask,
            SparkTask). Specifies resources like CPU, memory, and GPU allocation.
        alias: Optional alternative task name. If not provided, uses function name.
        io: I/O registry for serialization. Defaults to default_io.
        cache_enabled: Enable result caching. When True, the task checks for cached
            results before execution. If found, returns cached result. If not found,
            executes and caches the result. Defaults to False.
        cache_version: Optional version identifier for cached results. When None,
            version is calculated from the Docker image ID. Use this to maintain
            multiple cache versions for the same task.
        retry_attempts: Number of times to retry failed executions. Defaults to 0
            (no retries).
        image_spec: Optional container image specification. Allows specifying custom
            container images and build targets for the task execution environment.

    Returns:
        A decorator that converts a function into a TaskFunction.

    Example:
        Basic task with caching::

            @task(config=RayTask(head_cpu=2), cache_enabled=True)
            def process_data(input_path: str) -> dict:
                # Process data
                return {"status": "complete"}

        Task with custom image::

            @task(
                config=RayTask(head_cpu=4, head_memory="8Gi"),
                image_spec=ImageSpec(
                    container_image="my-image:latest",
                    recipe="bazel://path/to:target"
                )
            )
            def train_model(data: pd.DataFrame) -> Model:
                # Train model
                return trained_model

        Task with alias and retry::

            @task(
                config=SparkTask(driver_cpu=2, executor_cpu=4),
                alias="preprocess_v2",
                retry_attempts=3
            )
            def preprocess(df: DataFrame) -> DataFrame:
                # Preprocess DataFrame
                return processed_df
    """

    def decorator(fn: Callable[P, R]) -> TaskFunction[P, R]:
        """Wrap function as a TaskFunction.

        Args:
            fn: Function to wrap.

        Returns:
            TaskFunction wrapper around the function.
        """
        task_fn = TaskFunction[P, R](
            fn=fn,
            config=config,
            alias=alias,
            io=io,
            cache_enabled=cache_enabled,
            cache_version=cache_version,
            retry_attempts=retry_attempts,
            image_spec=image_spec,
        )
        update_wrapper(task_fn, fn)

        origin_fn = inspect.unwrap(fn)
        origin_fn._uf_task = task_fn
        return task_fn

    return decorator


def workflow():
    """Decorator for defining a Uniflow workflow.

    Marks a function as a workflow entry point. Workflows orchestrate multiple
    tasks together and define the overall execution flow. Unlike tasks, workflows
    are always executed locally and serve as the coordination layer.

    Returns:
        A decorator that marks a function as a workflow.

    Example:
        Simple workflow::

            @workflow()
            def my_workflow(input_file: str):
                # Load data
                data = load_task(input_file)

                # Process data
                result = process_task(data)

                # Save results
                save_task(result)

                return result

        Workflow with multiple stages::

            @workflow()
            def training_pipeline(dataset_path: str, model_type: str):
                # Data preparation stage
                raw_data = load_data(dataset_path)
                clean_data = clean_data_task(raw_data)

                # Training stage
                model = train_model_task(clean_data, model_type)

                # Evaluation stage
                metrics = evaluate_model_task(model, clean_data)

                return {"model": model, "metrics": metrics}
    """

    def decorator(fn: Callable[P, R]) -> Callable[P, R]:
        """Mark function as a workflow.

        Args:
            fn: Function to mark as workflow.

        Returns:
            The original function with workflow marker.
        """
        log.debug("init-decorator: %s", dot_path(fn))

        @wraps(fn)
        def wrapper(*args, **kwargs) -> R:
            """Execute the workflow function.

            Args:
                *args: Positional arguments for the workflow.
                **kwargs: Keyword arguments for the workflow.

            Returns:
                The result of executing the workflow.
            """
            return fn(*args, **kwargs)

        fn._uf_workflow = True
        return wrapper

    return decorator


def star_plugin(binding: str):
    """Decorator for Starlark plugin functions.

    Marks a Python function as a Starlark plugin, making it available for
    invocation from Starlark workflow definitions. Plugins bridge Python
    functionality into the Starlark execution environment.

    Args:
        binding: The binding name to use in Starlark.

    Returns:
        A decorator that marks a function as a Starlark plugin.

    Example:
        >>> @star_plugin(binding="custom_transform")
        ... def transform_data(data: dict) -> dict:
        ...     # Transform logic
        ...     return transformed
    """

    def decorator(fn: Callable[P, R]) -> Callable[P, R]:
        """Mark function as a Starlark plugin.

        Args:
            fn: Function to mark as plugin.

        Returns:
            The original function with plugin markers.
        """

        @wraps(fn)
        def wrapper(*args, **kwargs) -> R:
            """Execute the plugin function.

            Args:
                *args: Positional arguments.
                **kwargs: Keyword arguments.

            Returns:
                The result of executing the plugin.
            """
            return fn(*args, **kwargs)

        fn._uf_star_plugin = True
        fn._uf_star_plugin_binding = binding
        return wrapper

    return decorator


def is_star_plugin(fn) -> bool:
    """Check if a function is a Starlark plugin.

    Args:
        fn: Function to check.

    Returns:
        True if the function is marked as a Starlark plugin, False otherwise.
    """
    return getattr(fn, "_uf_star_plugin", False)


def is_workflow(fn) -> bool:
    """Check if a function is a workflow.

    Args:
        fn: Function to check.

    Returns:
        True if the function is marked as a workflow, False otherwise.
    """
    return getattr(fn, "_uf_workflow", False)


def get_star_plugin_binding(fn) -> str:
    """Get the Starlark binding name for a plugin function.

    Args:
        fn: Function to get binding from.

    Returns:
        The binding name string.

    Raises:
        AttributeError: If the function is not a Starlark plugin.
    """
    return fn._uf_star_plugin_binding


def _in_the_box(txt):
    """Format text in a decorative box for logging.

    Args:
        txt: Text to display in the box.

    Returns:
        String with text centered in a box with rounded corners.

    Example:
        >>> print(_in_the_box("Running Task"))
        ╭───────────────────────────────────╮
        │          Running Task             │
        ╰───────────────────────────────────╯
    """
    txt = f"│ {txt:^75} │"
    n = len(txt)
    border = "─" * (n - 2)
    t_border = "╭" + border + "╮"
    b_border = "╰" + border + "╯"
    return f"\n\n{t_border}\n{txt}\n{b_border}\n"


def write_task_result(url: str, value):
    """Write task result to a file at the specified URL.

    Serializes the value to JSON using the codec system and writes to the
    specified filesystem URL.

    Args:
        url: Filesystem URL where to write the result.
        value: The value to serialize and write.

    Raises:
        IOError: If writing to the URL fails.
    """
    with fsspec.open(url, mode="wt") as f:
        json.dump(value, f, default=encoder.default)


def _unref_args(
    sig: inspect.Signature, args: tuple, kwargs: dict, io: IORegistry
) -> tuple[tuple, dict]:
    """Dereference Ref arguments while preserving explicitly passed Refs.

    Processes task arguments to load data from Ref objects while preserving
    arguments explicitly typed as Ref in the function signature.

    Args:
        sig: Function signature to check for Ref-typed parameters.
        args: Positional arguments tuple.
        kwargs: Keyword arguments dictionary.
        io: I/O registry for loading Ref data.

    Returns:
        Tuple of (dereferenced positional args, dereferenced keyword args).
    """
    # Copy args and kwargs to avoid modifying the original
    pos_args, key_args = list(args), dict(kwargs)

    # Extract explicit refs args, if any
    pos_refs, key_refs = _replace_ref_args(sig, pos_args, key_args)
    # Unref: Load ref values and replace each ref with its actual value
    pos_args, key_args = unref(pos_args, io), unref(key_args, io)
    assert isinstance(pos_args, list)
    assert isinstance(key_args, dict)

    # Put the extracted refs back into the unrefed args
    for i, r in pos_refs.items():
        pos_args[i] = r

    key_args.update(key_refs)

    return tuple(pos_args), key_args


def _replace_ref_args(
    sig: inspect.Signature, pos_args: list, key_args: dict
) -> tuple[dict, dict]:
    """Extract arguments explicitly typed as Ref from argument lists.

    Identifies function parameters with Ref type annotation and extracts their
    values from the argument lists, replacing them with None.

    Args:
        sig: Function signature to check for Ref-typed parameters.
        pos_args: Positional arguments list (modified in-place).
        key_args: Keyword arguments dictionary (modified in-place).

    Returns:
        Tuple of (extracted positional refs, extracted keyword refs) as dictionaries
        mapping positions/names to Ref values.
    """
    replaced_pos_args, replaced_key_args = {}, {}
    for i, p in enumerate(sig.parameters.values()):
        if p.annotation != Ref:
            continue
        if i < len(pos_args):
            replaced_pos_args[i] = pos_args[i]
            pos_args[i] = None
        else:
            k = p.name
            if k in key_args:
                replaced_key_args[k] = key_args[k]
                key_args[k] = None
    return replaced_pos_args, replaced_key_args
