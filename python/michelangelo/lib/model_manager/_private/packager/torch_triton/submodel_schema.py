"""Auto-derive a ModelSchema for each submodel of a PyTorch model.

A submodel is every computational ``nn.Module`` in the module tree. Schemas
are captured from a single forward pass via PyTorch forward hooks and keyed by
dotted submodel path.

Names: dict keys > NamedTuple fields > forward() param names > AST return
names > output_N. Shapes use the per-sample convention (batch dim stripped,
scalar -> [1]).
"""

from __future__ import annotations

import ast
import inspect
import logging
import os
import textwrap
from typing import Any, Callable

import torch
import yaml

from michelangelo.lib.model_manager._private.schema.common.serde import schema_to_dict
from michelangelo.lib.model_manager._private.utils.torch_utils.model import (
    torch_dtype_to_data_type,
)
from michelangelo.lib.model_manager.schema import (
    DataType,
    ModelSchema,
    ModelSchemaItem,
)

_logger = logging.getLogger(__name__)

# Containers and training-only modules carry no meaningful standalone I/O
# schema.
_SKIP_TYPES = (
    torch.nn.ModuleList,
    torch.nn.ModuleDict,
    torch.nn.Sequential,
    torch.nn.ParameterList,
    torch.nn.ParameterDict,
    torch.nn.modules.loss._Loss,
)


def _strip_batch(shape: list[int]) -> list[int]:
    """Remove the batch dimension and return per-sample shape, defaulting to [1] for scalars."""  # noqa: E501
    per_sample = shape[1:]
    return per_sample if per_sample else [1]


def _schema_item(fact: dict) -> ModelSchemaItem:
    """Build a ModelSchemaItem from a tensor fact dict with dtype/shape/name keys."""
    dtype_str = fact.get("dtype", "")
    try:
        dtype = getattr(torch, dtype_str.replace("torch.", "")) if dtype_str else None
        data_type = (
            torch_dtype_to_data_type(dtype) if dtype is not None else DataType.FLOAT
        )
    except (ValueError, AttributeError):
        data_type = DataType.FLOAT
    return ModelSchemaItem(
        name=fact.get("name", "unknown"),
        data_type=data_type,
        shape=fact.get("shape", []),
    )


def get_forward_param_names(module: torch.nn.Module) -> list[str]:
    """Return the named, non-variadic parameters of forward after self.

    The class's unbound forward is inspected so that self is present in the
    signature, keeping inspection consistent across Python versions.

    Args:
        module: The model whose forward signature is inspected.

    Returns:
        Ordered parameter names excluding self and *args/**kwargs.
    """
    try:
        forward_fn = inspect.unwrap(type(module).forward)
        sig = inspect.signature(forward_fn)
    except (ValueError, TypeError):
        return []
    return [
        name
        for name, param in sig.parameters.items()
        if name != "self"
        and param.kind
        not in (inspect.Parameter.VAR_POSITIONAL, inspect.Parameter.VAR_KEYWORD)
    ]


def _return_names(module: torch.nn.Module) -> list[str | None] | None:
    """Extract variable names from the first return statement of forward() via AST."""
    try:
        tree = ast.parse(textwrap.dedent(inspect.getsource(module.forward)))
    except (OSError, TypeError, SyntaxError, IndentationError):
        return None
    returns = [n for n in ast.walk(tree) if isinstance(n, ast.Return)]
    if not returns or returns[0].value is None:
        return None
    val = returns[0].value
    if isinstance(val, ast.Name):
        return [val.id]
    if isinstance(val, ast.Tuple):
        return [el.id if isinstance(el, ast.Name) else None for el in val.elts]
    return None


def _tensor_facts(obj: Any, param_name: str | None = None) -> list[dict]:
    """Recursively extract {name?, shape, dtype} records from tensor objects."""
    if isinstance(obj, torch.Tensor):
        fact: dict = {"shape": _strip_batch(list(obj.shape)), "dtype": str(obj.dtype)}
        return [{"name": param_name, **fact} if param_name is not None else fact]
    if isinstance(obj, dict) or (
        hasattr(obj, "_asdict") and callable(getattr(obj, "_asdict", None))
    ):
        items = obj.items() if isinstance(obj, dict) else obj._asdict().items()
        return [{"name": k, **f} for k, v in items for f in _tensor_facts(v)]
    if isinstance(obj, (list, tuple)):
        return [f for v in obj for f in _tensor_facts(v)]
    return []


def _output_facts(output: Any, return_names: list[str | None] | None) -> list[dict]:
    """Build output fact records, enriched with AST-recovered names."""
    if isinstance(output, dict) or (
        hasattr(output, "_asdict") and callable(getattr(output, "_asdict", None))
    ):
        return _tensor_facts(output)
    if isinstance(output, torch.Tensor):
        name = (
            return_names[0]
            if return_names and len(return_names) == 1 and return_names[0]
            else "output_0"
        )
        return [
            {
                "name": name,
                "shape": _strip_batch(list(output.shape)),
                "dtype": str(output.dtype),
            }
        ]
    if isinstance(output, (list, tuple)):
        facts = []
        for i, v in enumerate(output):
            inner = _tensor_facts(v)
            if len(inner) == 1 and "name" not in inner[0]:
                # Single unnamed tensor -- assign from return_names or indexed
                # fallback. Only applies when there is exactly one fact:
                # multiple facts come from a nested dict/NamedTuple and already
                # carry their own names.
                ast_name = (
                    return_names[i] if return_names and i < len(return_names) else None
                )
                inner[0]["name"] = ast_name or f"output_{i}"
            facts.extend(inner)
        return facts
    return _tensor_facts(output)


def capture_submodel_schemas(
    model: torch.nn.Module,
    run_forward: Callable[[], Any],
) -> tuple[Any, dict[str, ModelSchema]]:
    """Capture a ModelSchema for every computational submodel.

    Attaches forward hooks to each computational submodel, runs the forward
    pass, and returns the captured schemas.

    Args:
        model: The model to introspect (must already be in eval mode with
            no_grad active).
        run_forward: A callable that invokes the model and returns its output.
            The caller owns the invocation so the existing model-invocation
            logic is preserved exactly.

    Returns:
        A tuple ``(model_output, submodel_schemas)`` where ``model_output`` is
        the raw forward output and ``submodel_schemas`` is a dict of
        ModelSchema objects keyed by dotted submodel path. Hook failures are
        logged and silently skipped; they never propagate to the caller.
    """
    captured: dict = {}
    registered = [
        (n, c) for n, c in model.named_modules() if n and not isinstance(c, _SKIP_TYPES)
    ]
    for name, _ in registered:
        captured[name] = {"inputs": None, "outputs": None}

    def _make_hook(hname: str, hchild: torch.nn.Module):
        param_names = get_forward_param_names(hchild)
        return_names = _return_names(hchild)

        def hook(_module, inputs, output):
            if captured[hname]["inputs"] is not None:
                return
            try:
                captured[hname]["inputs"] = [
                    f
                    for i, inp in enumerate(inputs)
                    for f in _tensor_facts(
                        inp,
                        param_name=param_names[i]
                        if i < len(param_names)
                        else f"arg_{i}",
                    )
                ]
                captured[hname]["outputs"] = _output_facts(output, return_names)
            except Exception:
                # A hook failure must never break packaging -- skip this
                # submodel.
                _logger.debug(
                    "submodel_schema: skipping %s (hook error)",
                    hname,
                    exc_info=True,
                )
                captured[hname] = {"inputs": None, "outputs": None}

        return hook

    handles = [
        child.register_forward_hook(_make_hook(name, child))
        for name, child in registered
    ]
    try:
        model_output = run_forward()
    finally:
        for handle in handles:
            handle.remove()

    return model_output, {
        name: ModelSchema(
            input_schema=[_schema_item(f) for f in data["inputs"] or []],
            output_schema=[_schema_item(f) for f in data["outputs"] or []],
        )
        for name, data in captured.items()
        if data["inputs"] is not None
    }


def write_submodel_schemas(
    package_path: str,
    submodel_schemas: dict[str, ModelSchema],
    schemas_file_name: str,
) -> None:
    """Write all submodel schemas to ``metadata/<schemas_file_name>``.

    The file is a single YAML keyed by dotted submodel path, each value a
    ModelSchema dict in the same format as ``schema.yaml``. Loading a specific
    submodel's schema is a dict lookup on the key.

    Args:
        package_path: The root path of the model package.
        submodel_schemas: A dict of ModelSchema objects keyed by dotted
            submodel path.
        schemas_file_name: The name of the file to write under ``metadata/``.
    """
    if not submodel_schemas:
        return
    schemas_path = os.path.join(package_path, "metadata", schemas_file_name)
    data = {path: schema_to_dict(schema) for path, schema in submodel_schemas.items()}
    with open(schemas_path, "w") as f:
        yaml.dump(data, f, sort_keys=False, default_flow_style=False)
