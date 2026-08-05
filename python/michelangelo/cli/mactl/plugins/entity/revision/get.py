"""Revision get filters + display columns (Type, User, Base Resource).

Three type-scoped filters (``--pipeline`` / ``--model`` / ``--deployment``,
mutually exclusive, LIKE match on base_resource.name) and one independent
``--owner`` filter (EQUAL). Uses the framework's callable-form
``filter_field_map`` value to express the mutex + one-flag-two-criteria
composite.
"""

from argparse import ArgumentTypeError
from inspect import Parameter
from logging import getLogger

from google.protobuf.message import Message

from michelangelo.cli.mactl.crd import CRD, Criterion

_LOG = getLogger(__name__)

_TYPE_FLAGS = ("pipeline", "model", "deployment")
_CRITERION_OPERATOR_EQUAL = 1
_CRITERION_OPERATOR_LIKE = 9


def _build_type_criteria(bound_args_arguments: dict) -> list[Criterion]:
    """Emit base_type + base_resource_name criteria pair for the type flags.

    Enforces mutual exclusion — >1 type flag set raises ``ArgumentTypeError``.
    Empty pattern degrades to LIKE ``%`` (match every name of that type).
    Returns ``[]`` when no type flag is set.
    """
    given = [t for t in _TYPE_FLAGS if bound_args_arguments.get(t) is not None]
    if len(given) > 1:
        raise ArgumentTypeError(
            f"conflict options <{', '.join(given)}> are set at the same time"
        )
    if not given:
        return []
    type_kind = given[0]
    pattern = bound_args_arguments.get(type_kind) or "%"
    return [
        {
            "field": "revision.spec.base_type.kind",
            "operator": _CRITERION_OPERATOR_EQUAL,
            "value": type_kind,
        },
        {
            "field": "revision.spec.base_resource.name",
            "operator": _CRITERION_OPERATOR_LIKE,
            "value": pattern,
        },
    ]


def _render_field(message_field: str, sub_field: str):
    """Build a column renderer that reads ``spec.<message_field>.<sub_field>``.

    Returns "" when the parent message is unset or the leaf value is falsy.
    """

    def _render(item: Message) -> str:
        if item.spec.HasField(message_field):
            return getattr(getattr(item.spec, message_field), sub_field) or ""
        return ""

    return _render


def _get_flag_arg(name: str, help_text: str, default=None) -> dict:
    """Build an additional_get_args entry for a `revision get` flag.

    Type flags use ``default=None`` so ``_build_type_criteria`` can
    distinguish "flag omitted" from "flag given with empty value".
    ``--owner`` uses ``default=""`` so the framework's string-form
    ``filter_field_map`` dispatch skips it when unset.
    """
    return {
        "func_signature": Parameter(
            name, Parameter.POSITIONAL_OR_KEYWORD, default=default
        ),
        "args": [f"--{name}"],
        "kwargs": {
            "dest": name,
            "type": str,
            "default": default,
            "required": False,
            "help": help_text,
        },
    }


def add_get_filters(crd: CRD) -> None:
    """Register 4 filter args and 3 columns on the revision CRD."""
    crd.additional_get_args.extend(
        [
            _get_flag_arg(
                "pipeline",
                "list revisions whose base_type is pipeline "
                "(optional pattern matches base_resource.name)",
            ),
            _get_flag_arg(
                "model",
                "list revisions whose base_type is model "
                "(optional pattern matches base_resource.name)",
            ),
            _get_flag_arg(
                "deployment",
                "list revisions whose base_type is deployment "
                "(optional pattern matches base_resource.name)",
            ),
            _get_flag_arg(
                "owner",
                "list revisions owned by the specified user",
                default="",
            ),
        ]
    )
    crd.filter_field_map.update(
        {
            # Callable — handles mutex + one-flag-two-criteria composite for
            # the type-scoped flags. Synthetic key (not an arg dest).
            "_revision_type_group": _build_type_criteria,
            # Independent owner filter, plain EQUAL.
            "owner": "revision.spec.owner.name",
        }
    )
    crd.additional_columns.extend(
        [
            {
                "column_name": "TYPE",
                "retrieve_func": _render_field("base_type", "kind"),
            },
            {"column_name": "USER", "retrieve_func": _render_field("owner", "name")},
            {
                "column_name": "BASE_RESOURCE",
                "retrieve_func": _render_field("base_resource", "name"),
            },
        ]
    )
    _LOG.debug("Revision get filters + columns registered on crd=%r", crd)
