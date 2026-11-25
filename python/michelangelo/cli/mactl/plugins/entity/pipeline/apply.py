"""Pipeline `apply` function plugin module."""

from inspect import Parameter
from logging import getLogger
from typing import Optional

from google.protobuf.json_format import ParseError
from google.protobuf.message import Message
from grpc import RpcError, StatusCode

from michelangelo.cli.mactl.crd import (
    CRD,
    CrdMethodInfo,
    apply_dry_run_to_request,
    crd_method_call,
    get_crd_namespace_and_name_from_yaml,
    read_yaml_to_crd_request,
)
from michelangelo.cli.mactl.plugins.entity.pipeline.declarative import (
    DeclarativeManifestError,
    explain_plan_from,
    printable,
    render_explain_plan,
)

_LOG = getLogger(__name__)

_OUTPUT_ARG = {
    "func_signature": Parameter(
        "output", Parameter.POSITIONAL_OR_KEYWORD, default="table"
    ),
    "args": ["-o", "--output"],
    "kwargs": {
        "dest": "output",
        "type": str,
        "choices": ["table", "json"],
        "default": "table",
        "help": (
            "Format of the declarative dry-run explain plan, one of: table|json"
            " (default: table). Other responses print unchanged."
        ),
    },
}


def add_apply_output_arg(crd: CRD) -> None:
    """Add -o/--output to `pipeline apply` once."""
    args = crd.func_signature["apply"]["args"]
    if all(arg.get("kwargs", {}).get("dest") != "output" for arg in args):
        args.append(_OUTPUT_ARG)


def pipeline_apply_func_impl(
    update_method_info: CrdMethodInfo,
    bound_args,
) -> Message:
    """Pipeline apply implementation.

    A server rejection or a manifest error exits with a one-line message
    instead of a traceback.

    TODO: this plugin monkey-patches ``apply_func_impl``, so framework
    ``-r/--root`` + ``-R/--recursive`` wiring is bypassed for the pipeline CRD.
    Other CRDs are covered. Follow-up PR to plumb ``external_root`` here if needed.
    """
    arguments = bound_args.arguments
    try:
        response = _create_or_update(update_method_info, arguments)
        plan = explain_plan_from(response.pipeline)
    except (RpcError, DeclarativeManifestError, ParseError) as err:
        raise SystemExit(f"Error: {_error_message(err)}") from None
    if plan is None:
        print(response)
    else:
        print(render_explain_plan(plan, arguments.get("output", "table")))
    return response


def _create_or_update(update_method_info: CrdMethodInfo, arguments: dict) -> Message:
    _self: CRD = arguments["self"]
    _file = arguments["file"]
    channel = update_method_info.channel

    _namespace, _name = get_crd_namespace_and_name_from_yaml(_file)
    existing = _get_existing(_self, _namespace, _name)

    if existing is None:
        _LOG.info("Create a new pipeline")
        method_info = CrdMethodInfo(
            channel,
            _self.full_name,
            *_self._extract_method_info(channel, _self.full_name, "Create"),
        )
        options_attr = "create_options"
    else:
        _LOG.info("Updating existing pipeline: %r", existing)
        method_info = update_method_info
        options_attr = "update_options"

    request_input = read_yaml_to_crd_request(
        method_info.input_class,
        _self.name,
        _file,
        _self.func_crd_metadata_converter,
    )
    if existing is not None:
        getattr(request_input, _self.name).metadata.resourceVersion = getattr(
            existing, _self.name
        ).metadata.resourceVersion
    apply_dry_run_to_request(request_input, options_attr, arguments)
    return crd_method_call(method_info, request_input)


def _get_existing(_self: CRD, namespace: str, name: str) -> Optional[Message]:
    # _get, not get: get prints a table row, which would corrupt -o json.
    try:
        return _self._get(namespace=namespace, name=name)
    except RpcError as err:
        _LOG.debug("Pipeline %r / %r does not exist: %r", namespace, name, err)
        if err.code() != StatusCode.NOT_FOUND:
            raise
    return None


def _error_message(err: Exception) -> str:
    if isinstance(err, RpcError) and hasattr(err, "details"):
        return printable(f"{err.code().name}: {err.details()}")
    return printable(str(err))
