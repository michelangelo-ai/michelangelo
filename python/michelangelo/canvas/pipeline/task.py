"""Unified task decorator for YAML-driven pipeline authoring.

This module provides ``pipeline_task``, the single decorator used to author tasks
that are configured via a ``pipeline_conf.yaml``-style file (see
:mod:`michelangelo.canvas.pipeline.config_loader`) rather than being wired up in
plain Python. It wraps :func:`michelangelo.uniflow.core.task` with one piece of
required behavior plus optional lifecycle hooks:

Required behavior: when a workflow invokes a task via its ``task_configs`` mapping
(see :mod:`michelangelo.canvas.pipeline.config_loader`), the ``config`` argument it
receives is a :class:`michelangelo.canvas.schema.v2alpha1.config.TaskConfig`
envelope (task_function/config/job_specs), not the task's own business config. The
decorated function is written against the inner business config (matching its
``config`` parameter's type annotation), so ``pipeline_task`` unwraps the envelope
to ``.config`` before calling the wrapped function.

Optional behavior: ``pre_hook``/``post_hook``/``on_error`` let a caller plug in
task lifecycle behavior (auth, logging, custom error handling) without requiring a
second, Canvas-specific decorator layered on top of the generic Uniflow one.

Example:
    Defining a task authored via YAML::

        from michelangelo.canvas.pipeline.task import pipeline_task
        from michelangelo.uniflow.plugins.ray import RayTask

        @pipeline_task(config=RayTask(head_cpu=2))
        def train(config: TrainConfig) -> Model:
            ...
"""

import logging
from functools import wraps
from typing import Callable, Optional

from michelangelo.uniflow.core.decorator import task as uniflow_task
from michelangelo.uniflow.core.image_spec import ImageSpec
from michelangelo.uniflow.core.io_registry import IORegistry, default_io
from michelangelo.uniflow.core.task_config import TaskConfig

_CONFIG_ARG = "config"

log = logging.getLogger(__name__)


def _with_config_unwrap(
    fn: Callable,
    pre_hook: Optional[Callable[[], None]],
    post_hook: Optional[Callable],
    on_error: Optional[Callable[[Exception], None]],
) -> Callable:
    """Wrap ``fn`` to unwrap its envelope ``config`` arg and run lifecycle hooks.

    ``fn`` is always invoked with a
    :class:`michelangelo.canvas.schema.v2alpha1.config.TaskConfig` envelope as its
    first positional or ``config`` keyword argument (that's how
    :meth:`~michelangelo.canvas.pipeline.config_loader.PipelineConfig.resolved_workflow_function`
    calls task functions), so the unwrap here is unconditional rather than
    type-checked.
    """

    @wraps(fn)
    def wrapper(*args, **kwargs):
        if _CONFIG_ARG in kwargs:
            kwargs[_CONFIG_ARG] = kwargs[_CONFIG_ARG].config
        else:
            args = list(args)
            args[0] = args[0].config
            args = tuple(args)

        if pre_hook:
            pre_hook()
        try:
            result = fn(*args, **kwargs)
        except Exception as e:
            if on_error:
                on_error(e)
            else:
                log.error("Error in pipeline task %s: %s", fn.__name__, e)
            raise
        if post_hook:
            post_hook(result)
        return result

    return wrapper


def pipeline_task(
    config: TaskConfig,
    alias: Optional[str] = None,
    io: IORegistry = default_io,
    cache_enabled: bool = False,
    cache_version: Optional[str] = None,
    retry_attempts: int = 0,
    image_spec: Optional[ImageSpec] = None,
    pre_hook: Optional[Callable[[], None]] = None,
    post_hook: Optional[Callable] = None,
    on_error: Optional[Callable[[Exception], None]] = None,
):
    """Decorator for a task authored via ``pipeline_conf.yaml``.

    Args:
        config: Execution/scheduling configuration (e.g. a ``RayTask`` or
            ``SparkTask``), forwarded as-is to
            :func:`michelangelo.uniflow.core.task`.
        alias: Optional alternative task name.
        io: I/O registry for serialization. Defaults to ``default_io``.
        cache_enabled: Enable result caching.
        cache_version: Optional cache version identifier.
        retry_attempts: Number of retry attempts on failure.
        image_spec: Optional container image specification.
        pre_hook: Optional callable invoked before the task function runs.
        post_hook: Optional callable invoked with the task's result after it
            completes successfully.
        on_error: Optional callable invoked with the raised exception if the task
            function fails. The exception is re-raised regardless.

    Returns:
        A decorator that produces a Uniflow task from the wrapped function.
    """

    def decorator(fn: Callable) -> Callable:
        wrapped_fn = _with_config_unwrap(fn, pre_hook, post_hook, on_error)
        return uniflow_task(
            config,
            alias=alias,
            io=io,
            cache_enabled=cache_enabled,
            cache_version=cache_version,
            retry_attempts=retry_attempts,
            image_spec=image_spec,
        )(wrapped_fn)

    return decorator
