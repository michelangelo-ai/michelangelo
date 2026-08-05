"""Config loader for ``pipeline_conf.yaml``-style YAML pipeline authoring.

This module turns a ``pipeline_conf.yaml`` file into a runnable pipeline. The
YAML shape is::

    workflow_function: my.module.my_workflow
    workflow_config:            # optional; only if the workflow takes one
      some_field: value
    task_configs:
      train:
        task_function: my.module.train   # optional; defaults to the same-named
                                          # global in the workflow's module
        config:
          learning_rate: 0.01
        job_specs:               # optional; see JobSpecs for the full shape
          ray:
            head: {pod: {resource: {cpu: 4, memory: 8G, disk_size: 20G, gpu: 0}}}
            worker: {pod: {resource: {...}}, min_instances: 1, max_instances: 4}

Resolution is annotation-driven, matching the internal CanvasFlex convention:
the workflow function's own signature says whether it takes
``(workflow_config, task_configs)`` or just ``(task_configs)``, and each task
function's ``config`` parameter's type annotation says which
:class:`~michelangelo.canvas.lib.shared.json_data.json_data.JSONData` subclass
to parse its YAML ``config`` block into. No custom YAML tags (``!py_import`` and
friends) or ``{{var.}}``/``{{task.}}``/``{{fn.}}`` templating are supported in
this phase.
"""

import importlib
import inspect
import typing
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Callable, Union

import yaml

from michelangelo.canvas.lib.shared.json_data.json_data import JSONData
from michelangelo.canvas.schema.v2alpha1.config import TaskConfig, WorkflowConfig
from michelangelo.canvas.schema.v2alpha1.job_specs import JobSpecs

_CONFIG_PARAM = "config"


def _nested_json_data_class(annotation: Any) -> Any:
    """Return the JSONData subclass in ``annotation`` (unwrapping Optional), or None."""
    if typing.get_origin(annotation) is typing.Union:
        args = [a for a in typing.get_args(annotation) if a is not type(None)]
        annotation = args[0] if len(args) == 1 else None
    if isinstance(annotation, type) and issubclass(annotation, JSONData):
        return annotation
    return None


def _fill_nullable_fields(cls: Any, raw: Any) -> Any:
    """Recursively default omitted nullable (Optional) JSONData fields to None.

    YAML authors shouldn't have to spell out ``spark: null`` / ``deps: null``
    for every Optional field, but JSONData's class-level type defaults are not
    applied by the pydantic runtime, leaving Optional fields required. This
    normalizes a raw YAML dict so that omitted nullable fields parse as None.
    Required (non-nullable) fields are left alone and still fail validation
    when missing.
    """
    if not (isinstance(cls, type) and issubclass(cls, JSONData)):
        return raw
    if not isinstance(raw, dict):
        return raw

    out = dict(raw)
    for name, field_info in cls.model_fields.items():
        extra = (field_info.json_schema_extra or {}).get("json_data_field", {})
        if name not in out:
            if extra.get("nullable"):
                out[name] = None
            continue
        nested_cls = _nested_json_data_class(field_info.annotation)
        if nested_cls is not None:
            out[name] = _fill_nullable_fields(nested_cls, out[name])
    return out


def parse_json_data(cls: type, raw: dict) -> Any:
    """Parse a raw YAML mapping into ``cls``, tolerating omitted Optional fields."""
    return cls(**_fill_nullable_fields(cls, raw))


def _import_by_path(dotted_path: str) -> Any:
    """Import an object (function, class) from its fully qualified dotted path."""
    module_path, _, attr_name = dotted_path.rpartition(".")
    if not module_path:
        raise ValueError(
            f"'{dotted_path}' is not a fully qualified name (expected 'module.attr')"
        )
    module = importlib.import_module(module_path)
    return getattr(module, attr_name)


def _config_class_for_param(fn: Callable, param_name: str) -> type:
    """Read the type annotation of ``fn``'s ``param_name`` parameter."""
    sig = inspect.signature(inspect.unwrap(fn))
    if param_name not in sig.parameters:
        raise TypeError(f"{fn.__qualname__} has no '{param_name}' parameter")
    annotation = sig.parameters[param_name].annotation
    if annotation is inspect.Parameter.empty:
        raise TypeError(
            f"{fn.__qualname__}'s '{param_name}' parameter has no type annotation "
            "(the annotation is required to know which config class to parse into)"
        )
    return annotation


@dataclass
class PipelineConfig:
    """A parsed, runnable ``pipeline_conf.yaml``.

    Attributes:
        workflow_config: The typed ``WorkflowConfig`` (workflow_function fqn,
            optional workflow-level config, and per-task ``TaskConfig``s).
        workflow_function: The imported workflow function.
        task_functions: Task name -> resolved (imported) task function, used to
            bind task names in the workflow module's globals.
    """

    workflow_config: WorkflowConfig
    workflow_function: Callable
    task_functions: dict[str, Callable]

    def resolved_workflow_function(self) -> Callable:
        """Return the workflow function with tasks bound to their resolved functions.

        A workflow body calls tasks by their bare name (e.g.
        ``train(config=task_configs["train"])``), so this binds each task name in
        the workflow function's module globals to the function resolved from
        ``pipeline_conf.yaml`` (either an explicit ``task_function`` override or
        the workflow module's own same-named global) before the workflow runs.
        """
        underlying = inspect.unwrap(self.workflow_function)
        underlying.__globals__.update(self.task_functions)
        return self.workflow_function

    def call_workflow(self) -> Any:
        """Invoke the resolved workflow function with the right argument shape."""
        fn = self.resolved_workflow_function()
        sig = inspect.signature(inspect.unwrap(fn))
        if len(sig.parameters) >= 2:
            return fn(
                self.workflow_config.workflow_config, self.workflow_config.task_configs
            )
        return fn(self.workflow_config.task_configs)


def load_pipeline_config(path: Union[str, Path]) -> PipelineConfig:
    """Parse a ``pipeline_conf.yaml`` file into a runnable :class:`PipelineConfig`.

    Args:
        path: Path to the ``pipeline_conf.yaml`` file.

    Returns:
        A :class:`PipelineConfig` ready to run via ``call_workflow()``.

    Raises:
        ValueError: If ``workflow_function`` is missing, or a task has neither an
            explicit ``task_function`` nor a matching global in the workflow's
            module.
        TypeError: If the workflow or a task function is missing a required
            type-annotated parameter.
    """
    with open(path) as f:
        raw = yaml.safe_load(f) or {}

    workflow_function_path = raw.get("workflow_function")
    if not workflow_function_path:
        raise ValueError(
            "pipeline_conf.yaml is missing required key 'workflow_function'"
        )

    workflow_function = _import_by_path(workflow_function_path)
    workflow_module = inspect.getmodule(inspect.unwrap(workflow_function))

    sig = inspect.signature(inspect.unwrap(workflow_function))
    params = list(sig.parameters.values())
    if not params:
        raise TypeError(
            f"workflow function '{workflow_function_path}' must accept at least a "
            "'task_configs' parameter"
        )

    workflow_config_obj = None
    if len(params) >= 2:
        workflow_config_class = params[0].annotation
        if workflow_config_class is inspect.Parameter.empty:
            raise TypeError(
                f"workflow function '{workflow_function_path}'s workflow-level "
                "config parameter has no type annotation"
            )
        raw_workflow_config = raw.get("workflow_config") or {}
        if isinstance(workflow_config_class, type) and issubclass(
            workflow_config_class, JSONData
        ):
            workflow_config_obj = parse_json_data(
                workflow_config_class, raw_workflow_config
            )
        else:
            workflow_config_obj = workflow_config_class(**raw_workflow_config)

    task_configs: dict[str, TaskConfig] = {}
    task_functions: dict[str, Callable] = {}
    for task_name, task_raw in (raw.get("task_configs") or {}).items():
        task_raw = task_raw or {}
        task_function_path = task_raw.get("task_function")
        if task_function_path:
            task_function = _import_by_path(task_function_path)
        else:
            task_function = getattr(workflow_module, task_name, None)
            if task_function is None:
                raise ValueError(
                    f"task '{task_name}' has no 'task_function' and no matching "
                    f"global named '{task_name}' in module '{workflow_module.__name__}'"
                )

        config_class = _config_class_for_param(task_function, _CONFIG_PARAM)
        raw_config = task_raw.get("config") or {}
        if isinstance(config_class, type) and issubclass(config_class, JSONData):
            config_obj = parse_json_data(config_class, raw_config)
        else:
            config_obj = config_class(**raw_config)

        job_specs_obj = None
        if task_raw.get("job_specs") is not None:
            job_specs_obj = parse_json_data(JobSpecs, task_raw["job_specs"])

        task_configs[task_name] = TaskConfig(
            task_function=task_function_path or "",
            config=config_obj,
            job_specs=job_specs_obj,
        )
        task_functions[task_name] = task_function

    workflow_config = WorkflowConfig(
        workflow_function=workflow_function_path,
        workflow_config=workflow_config_obj,
        task_configs=task_configs,
    )

    return PipelineConfig(
        workflow_config=workflow_config,
        workflow_function=workflow_function,
        task_functions=task_functions,
    )
