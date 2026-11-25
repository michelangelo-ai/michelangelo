"""Declarative pipeline packing and explain-plan rendering for mactl.

A declarative pipeline YAML sets ``spec.manifest.type`` to
``PIPELINE_MANIFEST_TYPE_DECLARATIVE`` and writes the ``DeclarativeWorkflow``
fields under ``spec.manifest.content``. mactl normalizes that content and packs
it as ``Any<DeclarativeWorkflow>``. The apiserver validates the graph and writes
the interpreter pin. The YAML can set only ``interpreterPin.version``.
"""

import math
import re
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
_MANIFEST_TYPE_PREFIX = "PIPELINE_MANIFEST_TYPE_"
# Full and short names of every manifest type except the zero value.
_MANIFEST_TYPE_NAMES = {
    name: full_name
    for full_name in pipeline_pb2.PipelineManifest.Type.keys()[1:]
    for name in (full_name, full_name.removeprefix(_MANIFEST_TYPE_PREFIX))
}

_LINE_BREAKS = re.compile(r"\s*[\r\n\t]+\s*")
_CONTROL_CHARS = re.compile(r"[\x00-\x1f\x7f-\x9f]")
_TYPE_URL = f"type.googleapis.com/{DeclarativeWorkflow.DESCRIPTOR.full_name}"
_MAX_SAFE_INTEGER = 2**53 - 1
# YAML aliases let a small file describe a large tree, so the copy walk stops
# at these limits. By estimate, the depth cap is near the server proto
# recursion limit.
_MAX_CONTENT_NODES = 100_000
_MAX_CONTENT_DEPTH = 64
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
    """Return whether the pipeline YAML declares a declarative manifest.

    The type can be a full or a short enum name, in any case. Content with a
    missing or unknown type is an error, because the UNIFLOW path drops it.
    """
    manifest = (yaml_dict.get("spec") or {}).get("manifest") or {}
    manifest_type = manifest.get("type")
    full_name = _MANIFEST_TYPE_NAMES.get(
        manifest_type.upper() if isinstance(manifest_type, str) else None
    )
    if full_name is None and "content" in manifest:
        found = (
            "is missing"
            if manifest_type is None
            else f"{manifest_type!r} is not a manifest type"
        )
        raise DeclarativeManifestError(
            f"spec.manifest.type {found}; set it to DECLARATIVE to apply"
            " spec.manifest.content"
        )
    return full_name == DECLARATIVE_MANIFEST_TYPE


def pack_declarative_manifest(manifest: dict) -> dict:
    """Return the declarative manifest fields of the request.

    The result holds the type, ``content`` as a JSON-form
    ``Any<DeclarativeWorkflow>``, and the pin version when the YAML sets one.
    """
    for field in ("uniflowTar", "uniflow_tar"):
        if field in manifest:
            raise DeclarativeManifestError(
                f"spec.manifest.{field} is server-owned for declarative pipelines;"
                " remove it, because the apiserver sets it from the interpreter pin"
            )
    workflow = normalize_workflow(manifest.get("content"))
    type_url = workflow.pop("@type", _TYPE_URL)
    if type_url != _TYPE_URL:
        raise DeclarativeManifestError(
            f"spec.manifest.content.@type must be {_TYPE_URL}, got {type_url}"
        )
    fields = {
        "type": DECLARATIVE_MANIFEST_TYPE,
        "content": {"@type": _TYPE_URL, **workflow},
    }
    version = _pin_version(manifest)
    if version is not None:
        fields["interpreterPin"] = {"version": version}
    return fields


def _pin_version(manifest: dict) -> Optional[str]:
    """Return interpreterPin.version, the only pin field that a client sets."""
    if {"interpreterPin", "interpreter_pin"} <= set(manifest):
        raise DeclarativeManifestError(
            "spec.manifest: set interpreterPin or interpreter_pin, not both"
        )
    field = "interpreter_pin" if "interpreter_pin" in manifest else "interpreterPin"
    if field not in manifest:
        return None
    pin = manifest[field]
    if not isinstance(pin, dict):
        raise DeclarativeManifestError(
            f"spec.manifest.{field} must be a mapping with one key, version;"
            f" for example {field}: {{version: <release>}}"
        )
    for key in pin:
        if key != "version":
            raise DeclarativeManifestError(
                f"spec.manifest.{field}.{key} is server-owned; to change the pin,"
                f" set only {field}.version"
            )
    version = pin.get("version")
    if not isinstance(version, str) or not version:
        raise DeclarativeManifestError(
            f"spec.manifest.{field}.version must be a release version string,"
            f" got {version!r}"
        )
    return version


def normalize_workflow(content: dict) -> dict:
    """Return a copy of the author's workflow dict in the wire form.

    Rejects values that JSON cannot hold exactly, infers missing parameter
    types from their defaults, and maps short enum names to full names.
    """
    if not isinstance(content, dict):
        raise DeclarativeManifestError(
            "spec.manifest.content must be a mapping of DeclarativeWorkflow fields"
        )
    workflow = _copy_checked(content, "content", [_MAX_CONTENT_NODES], 0)
    parameters = workflow.get("parameters")
    if isinstance(parameters, dict):
        _infer_parameter_types(parameters)
    _normalize_enums(workflow, DeclarativeWorkflow.DESCRIPTOR, "content")
    return workflow


def _copy_checked(node: Any, path: str, budget: list, depth: int) -> Any:
    """Return a copy of node with a new dict or list for each YAML alias.

    The walk counts nodes, so an alias tree fails before it expands in memory.
    It rejects non-string keys, non-finite floats, and unsafe integers.
    """
    budget[0] -= 1
    if budget[0] < 0:
        raise DeclarativeManifestError(
            f"spec.manifest.content has more than {_MAX_CONTENT_NODES} values"
            " after YAML aliases are expanded"
        )
    if depth > _MAX_CONTENT_DEPTH:
        raise DeclarativeManifestError(
            f"{path}: nested deeper than {_MAX_CONTENT_DEPTH} levels"
        )
    if isinstance(node, dict):
        copy = {}
        for key, value in node.items():
            if not isinstance(key, str):
                raise DeclarativeManifestError(
                    f"{path}: key {key!r} is a YAML {type(key).__name__}; quote it"
                )
            copy[key] = _copy_checked(value, f"{path}.{key}", budget, depth + 1)
        return copy
    if isinstance(node, list):
        return [
            _copy_checked(value, f"{path}[{index}]", budget, depth + 1)
            for index, value in enumerate(node)
        ]
    if isinstance(node, float) and not math.isfinite(node):
        raise DeclarativeManifestError(f"{path}: {node} is not a finite number")
    if (
        isinstance(node, int)
        and not isinstance(node, bool)
        and abs(node) > _MAX_SAFE_INTEGER
    ):
        raise DeclarativeManifestError(
            f"{path}: integer {node} is outside ±(2^53-1); numbers travel as"
            " IEEE doubles, so declare it as a string"
        )
    return node


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
    for field in descriptor.fields:
        if field.name != field.json_name and {field.name, field.json_name} <= set(node):
            raise DeclarativeManifestError(
                f"{path}: set {field.json_name} or {field.name}, not both"
            )
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
        value is None
        or message is None
        or message.full_name.startswith("google.protobuf.")
    ):
        return value
    if not isinstance(value, dict):
        raise DeclarativeManifestError(
            f"{path}: expected a mapping of {message.name} fields,"
            f" got {type(value).__name__} {value!r}"
        )
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


def printable(text: str) -> str:
    """Return text on one line, with each control character as a hex escape.

    Server text then cannot move the cursor or change the terminal.
    """
    text = _LINE_BREAKS.sub(" ", text)
    return _CONTROL_CHARS.sub(lambda m: f"\\x{ord(m.group()):02x}", text)


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
    """Render the plan as a table, or as JSON when output is "json".

    The table ends with a line that says that the dry run saved nothing.
    """
    if output == "json":
        return MessageToJson(
            plan,
            preserving_proto_field_name=True,
            always_print_fields_with_no_presence=True,
        )
    level_width = max(len("LEVEL"), len(str(len(plan.levels) - 1)))
    lines = [
        f"Interpreter: {printable(plan.interpreter_version)}",
        f"Tasks: {plan.task_count}",
        "",
        f"{'LEVEL'.ljust(level_width)}  TASKS",
    ]
    lines += [
        f"{str(index).ljust(level_width)}  {printable(', '.join(level.tasks))}"
        for index, level in enumerate(plan.levels)
    ]
    if plan.warnings:
        lines += ["", "WARNINGS", *(printable(w) for w in plan.warnings)]
    # The apiserver sets the plan only on a dry run.
    lines += ["", "Dry run: nothing was saved."]
    return "\n".join(lines)
