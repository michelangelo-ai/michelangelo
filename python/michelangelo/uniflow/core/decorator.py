import ast
import inspect
import logging
import os
import sys
import threading
import json
import fsspec
from functools import wraps, update_wrapper
from typing import Callable, Optional, TypeVar, Generic

from michelangelo.uniflow.core.io_registry import IORegistry, default_io
from michelangelo.uniflow.core.codec import encoder
from michelangelo.uniflow.core.ref import ref, unref, Ref
from michelangelo.uniflow.core.task_config import TaskConfig, Dependencies
from michelangelo.uniflow.core.utils import dot_path
from michelangelo.uniflow.core.image_spec import ImageSpec

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
        return self._image_spec

    @property
    def fn(self) -> Callable[P, R]:
        return self._fn

    def __call__(self, *args: P.args, **kwargs: P.kwargs) -> R:
        fn_path = dot_path(self._fn)
        if task_context.config is not None:
            # Execute nested task call as a regular function without additional processing.
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
        image_spec: Optional[ImageSpec] = None,
    ) -> "TaskFunction[P, R]":
        """
        Creates a new TaskFunction instance with overridden alias and/or config.

        This method allows you to create a new TaskFunction instance that shares the same
        function, IO registry, and cache settings as the original, but with a different
        alias and/or configuration.

        Parameters:
            alias (Optional[str]): An optional alias for the task. If not provided, the original alias is used.
            config (Optional[TaskConfig]): An optional TaskConfig object.
                                           This object will be merged the original configuration in the decorator.
                                           For example, if the original configuration specifies a head_cpu = 4, head_memory = 16GB,
                                           and the new configuration specifies a CPU count of 8,
                                           the resulting configuration will have a head_cpu = 8 and a head_memory = 16GB.
            retry_attempts (Optional[int]): An optional retry attempts for the task. Default is 1 (no retries).
            image_spec (Optional[ImageSpec]): An optional ImageSpec object. This object will be merged the original image spec in the decorator.
                                             For example, if the original image spec specifies a container image = "docker.io/library/examples:latest",
                                             and the new image spec specifies a container image = "docker.io/library/examples:latest",
                                             the resulting image spec will have a container image = "docker.io/library/examples:latest".
        Returns:
            TaskFunction[P, R]: A new TaskFunction instance with the specified overrides.
        """
        return TaskFunction(
            fn=self._fn,
            config=config or self._config,
            alias=alias or self._alias,
            io=self._io,
            cache_enabled=self._cache_enabled,
            cache_version=self._cache_version,
            retry_attempts=retry_attempts or self._retry_attempts,
            image_spec=image_spec or self._image_spec,
        )

    def _transpile(self, dependencies: Dependencies) -> ast.AST:
        """
        Constructs and returns an AST expression that corresponds to a transpiled Starlark code representation of the task function.

        The resulting expression follows the format: `<task-factory>([k=v, ...])`, which evaluates to a Starlark function encapsulating the orchestration
        logic for the specified configuration (task handler function).

        This expression may reference external attributes, which will be included in the Dependencies collection.

        For example, the generated expression may appear as follows:

            __python_task__(alias="task_1", cpu=4, gpu=1)

        In this instance, `__python_task__` is an external dependency function serving as the factory function that returns the task handler function.

        Parameters:
            dependencies: A collection of external dependencies used during the transpilation process.

        Returns:
            ast.AST: An AST node representing a call to the task's bound Starlark factory function.
        """

        # Register the task's Starlark Binding (Task Factory Function) in the Dependencies collection.
        binding = self._config.get_binding()
        dependencies.add_star_attribute(
            binding.export,
            binding.star_file,
            binding.function,
        )

        # Construct AST keywords that represent configuration properties for the Task Factory Function
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
                keywords.append(ast.keyword("container_image", ast.Constant(self._image_spec.container_image)))
            if self._image_spec.receipt:
                keywords.append(ast.keyword("receipt", ast.Constant(self._image_spec.receipt)))

        # Construct and return AST Call node that calls the Task Factory Function with the keywords.
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
    """
    Decorator for defining a task function. Usage example:

        @task(config=Python(cpu=1), cache_enabled=True, cache_version="v0")
        def task_1(a, b):
            return a + b

        @task(
            config=RayTask(cpu=1),
            image_spec=ImageSpec(
                container_image="my-image:latest",
                receipt="bazel://path/to:target"
            )
        )
        def task_with_custom_image():
            pass

    Parameters:
        config: Task configuration object. It defines task type, such as Python, Spark, etc, as well as type-specific parameters.
        alias: Optional[str]: Alternative task name. If not provided, the task name is inferred from the function name.
        io: IORegistry: IO implementation to use for checkpointing.
        cache_enabled: bool: Enable caching for the task. Default is False.
            If enabled, before executing the task, we will try to query the task result from the cache.
                If the result is found, the task will be skipped and the cached result will be returned.
                If the result is not found, the task will be executed and the result will be stored in the cache.
            If disabled, the task will always be executed. Note that the task result will be still be cached.
        cache_version: Optional[str]: Cache version for the task. Default is None.
            We can use this to save multiple versions of caches for the same task and specify the cache version used to
            skip the task. If it is None, the default cached version will be calculated by the docker image id of the task.
        retry_attempts: int: Number of retry attempts for the task. Default is 1 (no retries).
        image_spec: Optional[ImageSpec]: Container image specification for the task. Allows specifying custom container images and build targets.
    """

    def decorator(fn: Callable[P, R]) -> TaskFunction[P, R]:
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
    def decorator(fn: Callable[P, R]) -> Callable[P, R]:
        log.debug("init-decorator: %s", dot_path(fn))

        @wraps(fn)
        def wrapper(*args, **kwargs) -> R:
            return fn(*args, **kwargs)

        fn._uf_workflow = True
        return wrapper

    return decorator


def star_plugin(binding: str):
    def decorator(fn: Callable[P, R]) -> Callable[P, R]:
        @wraps(fn)
        def wrapper(*args, **kwargs) -> R:
            return fn(*args, **kwargs)

        fn._uf_star_plugin = True
        fn._uf_star_plugin_binding = binding
        return wrapper

    return decorator


def is_star_plugin(fn) -> bool:
    return getattr(fn, "_uf_star_plugin", False)


def is_workflow(fn) -> bool:
    return getattr(fn, "_uf_workflow", False)


def get_star_plugin_binding(fn) -> str:
    return fn._uf_star_plugin_binding


def _in_the_box(txt):
    txt = f"│ {txt:^75} │"
    n = len(txt)
    border = "─" * (n - 2)
    t_border = "╭" + border + "╮"
    b_border = "╰" + border + "╯"
    return f"\n\n{t_border}\n{txt}\n{b_border}\n"


def write_task_result(url: str, value):
    with fsspec.open(url, mode="wt") as f:
        json.dump(value, f, default=encoder.default)


def _unref_args(
    sig: inspect.Signature, args: tuple, kwargs: dict, io: IORegistry
) -> tuple[tuple, dict]:
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
