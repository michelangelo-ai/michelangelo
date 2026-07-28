"""Pipeline get filters + display columns (Owner, Type).

Adds ``--owner`` and ``--type`` to ``pipeline get`` (server-side filter via
``ListOptionsExt.Operation.Criterion``) and two extra table columns (``OWNER``,
``TYPE``) rendered per row. Uses the framework hooks on ``CRD``
(``additional_get_args``, ``filter_field_map``, ``additional_columns``).
"""

from inspect import Parameter
from logging import getLogger

from google.protobuf.message import Message

from michelangelo.cli.mactl.crd import CRD
from michelangelo.gen.api.v2 import pipeline_pb2

_LOG = getLogger(__name__)

_PIPELINE_TYPE_PREFIX = "PIPELINE_TYPE_"
_PIPELINE_TYPE_NAMES = frozenset(
    v.name for v in pipeline_pb2.PipelineType.DESCRIPTOR.values
)


def _normalize_pipeline_type(value: str) -> str:
    """Return the full ``PIPELINE_TYPE_*`` name for ``value``.

    Accepts the full name or the short suffix (case-insensitive). Raises
    ``ValueError`` when the value is not a declared enum member.
    """
    candidate = value.strip().upper()
    if not candidate.startswith(_PIPELINE_TYPE_PREFIX):
        candidate = _PIPELINE_TYPE_PREFIX + candidate
    if candidate not in _PIPELINE_TYPE_NAMES:
        valid = sorted(
            n[len(_PIPELINE_TYPE_PREFIX) :]
            for n in _PIPELINE_TYPE_NAMES
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
    """Column value for TYPE — short name (``PIPELINE_TYPE_`` prefix stripped)."""
    name = pipeline_pb2.PipelineType.Name(item.spec.type)
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
