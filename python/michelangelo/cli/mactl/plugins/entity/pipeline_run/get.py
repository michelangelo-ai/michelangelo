"""Pipeline_run get filters + display columns (Revision, User, Environment, State).

Adds ``--actor`` (EQUAL) and ``--revision`` (LIKE) filters and 4 columns.
Two config keys let downstream override defaults:
``pipeline_run_state_pb2_module`` (pb2 for the STATE enum) and
``pipeline_run_environment_label`` (label key for the ENVIRONMENT column).
"""

import importlib
from inspect import Parameter
from logging import getLogger

from google.protobuf.message import Message

from michelangelo.cli.mactl.config import load_config
from michelangelo.cli.mactl.crd import CRD

_LOG = getLogger(__name__)

_STATE_PREFIX = "PIPELINE_RUN_STATE_"
_DEFAULT_STATE_PB2_MODULE = "michelangelo.gen.api.v2.pipeline_run_pb2"
_DEFAULT_ENV_LABEL = "pipelinerun.michelangelo/environment"

_CRITERION_OPERATOR_EQUAL = 1
_CRITERION_OPERATOR_LIKE = 9


def _load_state_pb2():
    """Import the configured ``pipeline_run_pb2`` module lazily.

    Lazy so importing this plugin doesn't force resolution of the generated
    proto — callers only trigger it when the get command actually renders
    the STATE column.
    """
    module_path = load_config().get(
        "pipeline_run_state_pb2_module", _DEFAULT_STATE_PB2_MODULE
    )
    return importlib.import_module(module_path)


def _env_label() -> str:
    """Label key used for the ENVIRONMENT column."""
    return load_config().get("pipeline_run_environment_label", _DEFAULT_ENV_LABEL)


def _render_revision(item: Message) -> str:
    """Prefer ``spec.revision.name``, fall back to ``spec.draft.name``."""
    if item.spec.HasField("revision") and item.spec.revision.name:
        return item.spec.revision.name
    if item.spec.HasField("draft") and item.spec.draft.name:
        return item.spec.draft.name
    return ""


def _render_user(item: Message) -> str:
    """Actor name — empty string when unset."""
    if item.spec.HasField("actor"):
        return item.spec.actor.name or ""
    return ""


def _render_env(item: Message) -> str:
    """Environment label value — empty string when unset."""
    return item.metadata.labels.get(_env_label(), "")


def _render_state(item: Message) -> str:
    """State short name (``PIPELINE_RUN_STATE_`` prefix stripped).

    Falls back to numeric enum value when the configured pipeline_run_pb2
    doesn't know this value (e.g. server returns a newer enum than the
    client's proto knows).
    """
    pb2 = _load_state_pb2()
    try:
        name = pb2.PipelineRunState.Name(item.status.state)
    except ValueError:
        return str(item.status.state)
    if name.startswith(_STATE_PREFIX):
        return name[len(_STATE_PREFIX) :]
    return name


def add_get_filters(crd: CRD) -> None:
    """Register --actor / --revision filters and 4 columns on ``crd``."""
    crd.additional_get_args.extend(
        [
            {
                "func_signature": Parameter(
                    "actor", Parameter.POSITIONAL_OR_KEYWORD, default=""
                ),
                "args": ["--actor"],
                "kwargs": {
                    "dest": "actor",
                    "type": str,
                    "default": "",
                    "required": False,
                    "help": ("list the pipeline runs launched by the specified user"),
                },
            },
            {
                "func_signature": Parameter(
                    "revision", Parameter.POSITIONAL_OR_KEYWORD, default=""
                ),
                "args": ["--revision"],
                "kwargs": {
                    "dest": "revision",
                    "type": str,
                    "default": "",
                    "required": False,
                    "help": (
                        "list the pipeline runs whose revision name matches"
                        " the given pattern (server-side LIKE)"
                    ),
                },
            },
        ]
    )
    crd.filter_field_map.update(
        {
            "actor": "pipeline_run.spec.actor.name",
            "revision": {
                "field": "pipeline_run.spec.revision.name",
                "operator": _CRITERION_OPERATOR_LIKE,
            },
        }
    )
    crd.additional_columns.extend(
        [
            {"column_name": "REVISION", "retrieve_func": _render_revision},
            {"column_name": "USER", "retrieve_func": _render_user},
            {"column_name": "ENVIRONMENT", "retrieve_func": _render_env},
            {"column_name": "STATE", "retrieve_func": _render_state},
        ]
    )
    _LOG.debug("Pipeline_run get filters + columns registered on crd=%r", crd)
