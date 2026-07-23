"""Helpers for wiring k8s-standard mutation options onto CRD requests.

Today only ``--dry-run`` is wired; more options (``--field-manager``,
``--field-validation``) can slot in next to it as needed.
"""

from collections.abc import Mapping
from logging import getLogger
from typing import Any

from google.protobuf.message import Message

_LOG = getLogger(__name__)


def apply_dry_run_to_request(
    request: Message,
    options_attr: str,
    bound_args_arguments: Mapping[str, Any],
) -> None:
    """Set server-side dry-run on ``request.<options_attr>`` when opted in.

    Reads ``bound_args_arguments["dry_run"]`` and, when true, appends the
    ``"All"`` sentinel to the ``dryRun`` list on the nested ``CreateOptions`` /
    ``UpdateOptions`` / ``DeleteOptions`` submessage. Server does full
    validation (schema, RBAC, admission, quota) then rolls back — nothing
    persists.

    Args:
        request: The RPC request proto (e.g. ``UpdatePipelineRequest``).
        options_attr: Field name of the options submessage on ``request``
            (``"create_options"``, ``"update_options"``, ``"delete_options"``).
        bound_args_arguments: ``bound_args.arguments`` from the CRD dispatcher.

    Notes:
        * The k8s.io apimachinery submessages use ``dryRun`` (camelCase) as the
          Python attribute name — writing ``.dry_run`` raises ``AttributeError``.
        * ``dryRun`` is a repeated string; use ``.append("All")`` not assignment.
    """
    if not bound_args_arguments.get("dry_run", False):
        return
    options = getattr(request, options_attr)
    options.dryRun.append("All")
    _LOG.info("Dry-run enabled: %s.dryRun=%s", options_attr, list(options.dryRun))


def should_emit_metrics(
    bound_args_arguments: Mapping[str, Any], environment: str
) -> bool:
    """Return True only when metrics should reach the pipeline.

    Suppresses on dry-run OR non-production environments — matches the
    reference client's gate. No metrics code lives in this repo yet; callers
    that add one should route through here so both conditions stay coupled.
    """
    if bound_args_arguments.get("dry_run", False):
        return False
    return environment == "production"
