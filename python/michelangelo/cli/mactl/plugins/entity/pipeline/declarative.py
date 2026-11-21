"""Declarative pipeline packing and explain-plan rendering for mactl.

A declarative pipeline YAML sets ``spec.manifest.type`` to
``PIPELINE_MANIFEST_TYPE_DECLARATIVE`` and writes the ``DeclarativeWorkflow``
fields under ``spec.manifest.content``. mactl normalizes that content and packs
it as ``Any<DeclarativeWorkflow>``. The apiserver validates the graph and owns
the interpreter pin.
"""

from copy import deepcopy
from os.path import commonprefix
from typing import Any, Optional

from google.protobuf.descriptor import Descriptor, EnumDescriptor, FieldDescriptor
from google.protobuf.json_format import MessageToJson, Parse

from michelangelo.gen.api.v2 import pipeline_pb2
from michelangelo.gen.api.v2.declarative_pb2 import (
    DeclarativeWorkflow,
    ExplainPlan,
    ParameterType,
)

DECLARATIVE_MANIFEST_TYPE = pipeline_pb2.PipelineManifest.Type.Name(
    pipeline_pb2.PipelineManifest.PIPELINE_MANIFEST_TYPE_DECLARATIVE
)
EXPLAIN_PLAN_ANNOTATION = "michelangelo/declarative-explain-plan"

_TYPE_URL = f"type.googleapis.com/{DeclarativeWorkflow.DESCRIPTOR.full_name}"
_MAX_SAFE_INTEGER = 2**53 - 1
_SERVER_OWNED_MANIFEST_FIELDS = (
    "interpreterPin",
    "interpreter_pin",
    "uniflowTar",
    "uniflow_tar",
)
# bool precedes int because bool is an int subclass.
_INFERRED_PARAMETER_TYPES = (
    (bool, ParameterType.PARAMETER_TYPE_BOOLEAN),
    (int, ParameterType.PARAMETER_TYPE_INTEGER),
    (float, ParameterType.PARAMETER_TYPE_NUMBER),
    (str, ParameterType.PARAMETER_TYPE_STRING),
    (list, ParameterType.PARAMETER_TYPE_LIST),
    (dict, ParameterType.PARAMETER_TYPE_DICT),
)


class DeclarativeManifestError(ValueError):
    """The declarative manifest in the YAML cannot be packed."""


def is_declarative(yaml_dict: dict) -> bool:
    """Return whether the pipeline YAML declares a declarative manifest."""
    manifest = (yaml_dict.get("spec") or {}).get("manifest") or {}
    return manifest.get("type") == DECLARATIVE_MANIFEST_TYPE


def pack_declarative_content(manifest: dict) -> dict:
    """Return ``manifest.content`` as a JSON-form ``Any<DeclarativeWorkflow>``."""
    for field in _SERVER_OWNED_MANIFEST_FIELDS:
        if field in manifest:
            raise DeclarativeManifestError(
                f"spec.manifest.{field} is server-owned for declarative pipelines;"
                " remove it"
            )
    workflow = normalize_workflow(manifest.get("content"))
    type_url = workflow.pop("@type", _TYPE_URL)
    if type_url != _TYPE_URL:
        raise DeclarativeManifestError(
            f"spec.manifest.content.@type must be {_TYPE_URL}, got {type_url}"
        )
    return {"@type": _TYPE_URL, **workflow}


def normalize_workflow(content: dict) -> dict:
    """Return a copy of the author's workflow dict in the wire form.

    Rejects integers that an IEEE double cannot hold exactly, infers missing
    parameter types from their defaults, and maps short enum names to full
    names.
    """
    if not isinstance(content, dict):
        raise DeclarativeManifestError(
            "spec.manifest.content must be a mapping of DeclarativeWorkflow fields"
        )
    workflow = deepcopy(content)
    _check_safe_integers(workflow, "content")
    parameters = workflow.get("parameters")
    if isinstance(parameters, dict):
        _infer_parameter_types(parameters)
    _normalize_enums(workflow, DeclarativeWorkflow.DESCRIPTOR, "content")
    return workflow


def _check_safe_integers(node: Any, path: str) -> None:
    if isinstance(node, dict):
        for key, value in node.items():
            _check_safe_integers(value, f"{path}.{key}")
    elif isinstance(node, list):
        for index, value in enumerate(node):
            _check_safe_integers(value, f"{path}[{index}]")
    elif (
        isinstance(node, int)
        and not isinstance(node, bool)
        and abs(node) > _MAX_SAFE_INTEGER
    ):
        raise DeclarativeManifestError(
            f"{path}: integer {node} is outside ±(2^53-1); numbers travel as"
            " IEEE doubles, so declare it as a string"
        )


def _infer_parameter_types(parameters: dict) -> None:
    for name, spec in parameters.items():
        if not isinstance(spec, dict) or "type" in spec:
            continue
        path = f"content.parameters.{name}"
        default = spec.get("default")
        if default is None:
            raise DeclarativeManifestError(
                f"{path}: set type; it cannot be inferred without a non-null default"
            )
        inferred = next(
            (
                t
                for py_type, t in _INFERRED_PARAMETER_TYPES
                if isinstance(default, py_type)
            ),
            None,
        )
        if inferred is None:
            raise DeclarativeManifestError(
                f"{path}: set type; it cannot be inferred from a default of YAML"
                f" type {type(default).__name__}"
            )
        spec["type"] = ParameterType.Name(inferred)


def _normalize_enums(node: dict, descriptor: Descriptor, path: str) -> None:
    fields = {field.name: field for field in descriptor.fields}
    fields.update({field.json_name: field for field in descriptor.fields})
    for key, value in node.items():
        field = fields.get(key)
        if field is None:
            continue
        field_path = f"{path}.{key}"
        if field.message_type is not None and field.message_type.GetOptions().map_entry:
            value_field = field.message_type.fields_by_name["value"]
            if isinstance(value, dict):
                for map_key in value:
                    value[map_key] = _normalize_value(
                        value_field, value[map_key], f"{field_path}.{map_key}"
                    )
        elif isinstance(value, list):
            node[key] = [
                _normalize_value(field, item, f"{field_path}[{index}]")
                for index, item in enumerate(value)
            ]
        else:
            node[key] = _normalize_value(field, value, field_path)


def _normalize_value(field: FieldDescriptor, value: Any, path: str) -> Any:
    if field.enum_type is not None:
        return _full_enum_name(field.enum_type, value, path)
    message = field.message_type
    # google.protobuf types (Struct, Value, Duration) hold user data, not enums.
    if (
        message is not None
        and not message.full_name.startswith("google.protobuf.")
        and isinstance(value, dict)
    ):
        _normalize_enums(value, message, path)
    return value


def _full_enum_name(enum: EnumDescriptor, value: Any, path: str) -> Any:
    if not isinstance(value, str) or value in enum.values_by_name:
        return value
    names = [v.name for v in enum.values]
    prefix = commonprefix(names).rpartition("_")[0] + "_"
    full_name = prefix + value.upper()
    if full_name in enum.values_by_name:
        return full_name
    short_names = ", ".join(name[len(prefix) :] for name in names)
    raise DeclarativeManifestError(
        f"{path}: unknown value {value!r}; use one of {short_names}"
    )


def explain_plan_from(pipeline: pipeline_pb2.Pipeline) -> Optional[ExplainPlan]:
    """Return the dry-run explain plan on the pipeline, or None when absent.

    The annotation value is the protojson encoding of ExplainPlan. Unknown
    fields are ignored so that a newer apiserver does not break dry-run.
    """
    annotations = pipeline.metadata.annotations
    if EXPLAIN_PLAN_ANNOTATION not in annotations:
        return None
    return Parse(
        annotations[EXPLAIN_PLAN_ANNOTATION], ExplainPlan(), ignore_unknown_fields=True
    )


def render_explain_plan(plan: ExplainPlan, output: str) -> str:
    """Render the plan as a table, or as JSON when output is "json"."""
    if output == "json":
        return MessageToJson(
            plan,
            preserving_proto_field_name=True,
            always_print_fields_with_no_presence=True,
        )
    level_width = max(len("LEVEL"), len(str(len(plan.levels) - 1)))
    lines = [
        f"Interpreter: {plan.interpreter_version}",
        f"Tasks: {plan.task_count}",
        "",
        f"{'LEVEL'.ljust(level_width)}  TASKS",
    ]
    lines += [
        f"{str(index).ljust(level_width)}  {', '.join(level.tasks)}"
        for index, level in enumerate(plan.levels)
    ]
    if plan.warnings:
        lines += ["", "WARNINGS", *plan.warnings]
    return "\n".join(lines)
