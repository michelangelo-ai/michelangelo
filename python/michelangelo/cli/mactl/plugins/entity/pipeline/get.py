"""Pipeline get filters + display columns (Owner, Type).

Adds ``--owner`` and ``--type`` to ``pipeline get`` (server-side filter via
``ListOptionsExt.Operation.Criterion``) and two extra table columns (``OWNER``,
``TYPE``) rendered per row. Uses the framework hooks on ``CRD``
(``additional_get_args``, ``filter_field_map``, ``additional_columns``).

The PipelineType enum module is selected via the ``pipeline_type_pb2_module``
config key (defaults to ``michelangelo.gen.api.v2.pipeline_pb2``). Distributions
that extend the enum (e.g. downstream forks with extra values) can point this at
their own generated module without patching this file.
"""

import importlib
from inspect import Parameter
from logging import getLogger

from google.protobuf.message import Message

from michelangelo.cli.mactl.config import load_config
from michelangelo.cli.mactl.crd import CRD

_LOG = getLogger(__name__)

_PIPELINE_TYPE_PREFIX = "PIPELINE_TYPE_"
_DEFAULT_PB2_MODULE = "michelangelo.gen.api.v2.pipeline_pb2"


def _load_pipeline_pb2():
    """Import the configured ``pipeline_pb2`` module lazily.

    Lazy so importing this plugin module doesn't force resolution of the
    generated proto — callers only trigger it when the get command actually
    needs enum names or values.
    """
    module_path = load_config().get("pipeline_type_pb2_module", _DEFAULT_PB2_MODULE)
    return importlib.import_module(module_path)


def _pipeline_type_names() -> frozenset[str]:
    """Enum member names from the configured pipeline_pb2 module."""
    pb2 = _load_pipeline_pb2()
    return frozenset(v.name for v in pb2.PipelineType.DESCRIPTOR.values)


def _normalize_pipeline_type(value: str) -> str:
    """Return the full ``PIPELINE_TYPE_*`` name for ``value``.

    Accepts the full name or the short suffix (case-insensitive). Raises
    ``ValueError`` when the value is not a declared enum member.
    """
    candidate = value.strip().upper()
    if not candidate.startswith(_PIPELINE_TYPE_PREFIX):
        candidate = _PIPELINE_TYPE_PREFIX + candidate
    names = _pipeline_type_names()
    if candidate not in names:
        valid = sorted(
            n[len(_PIPELINE_TYPE_PREFIX) :]
            for n in names
            if n != "PIPELINE_TYPE_INVALID"
        )
        raise ValueError(
            f"invalid --type {value!r}; expected one of: {', '.join(valid)}"
        )
    return candidate


def _render_owner(item: Message) -> str:
    """Column value for OWNER — empty string when spec.owner is unset."""
    if item.spec.HasField("owner"):
        return item.spec.owner.name or ""
    return ""


def _render_type(item: Message) -> str:
    """Column value for TYPE — short name (``PIPELINE_TYPE_`` prefix stripped).

    Falls back to the numeric enum value when the configured pipeline_pb2 does
    not know this value (e.g. server returns a newer enum than the client's
    proto knows).
    """
    pb2 = _load_pipeline_pb2()
    try:
        name = pb2.PipelineType.Name(item.spec.type)
    except ValueError:
        return str(item.spec.type)
    if name.startswith(_PIPELINE_TYPE_PREFIX):
        return name[len(_PIPELINE_TYPE_PREFIX) :]
    return name


def add_get_filters(crd: CRD) -> None:
    """Register --owner / --type filters and OWNER / TYPE columns on ``crd``."""
    crd.additional_get_args.extend(
        [
            {
                "func_signature": Parameter(
                    "owner", Parameter.POSITIONAL_OR_KEYWORD, default=""
                ),
                "args": ["--owner"],
                "kwargs": {
                    "dest": "owner",
                    "type": str,
                    "default": "",
                    "required": False,
                    "help": "list the pipelines owned by the specified user",
                },
            },
            {
                "func_signature": Parameter(
                    "type", Parameter.POSITIONAL_OR_KEYWORD, default=""
                ),
                "args": ["--type"],
                "kwargs": {
                    "dest": "type",
                    "type": _normalize_pipeline_type,
                    "default": "",
                    "required": False,
                    "help": (
                        "list the pipelines of the specified type "
                        "(e.g. TRAIN, EVAL, UNIFLOW)"
                    ),
                },
            },
        ]
    )
    crd.filter_field_map.update(
        {
            "owner": "pipeline.spec.owner.name",
            "type": "pipeline.spec.type",
        }
    )
    crd.additional_columns.extend(
        [
            {"column_name": "OWNER", "retrieve_func": _render_owner},
            {"column_name": "TYPE", "retrieve_func": _render_type},
        ]
    )
    _LOG.debug("Pipeline get filters + columns registered on crd=%r", crd)
