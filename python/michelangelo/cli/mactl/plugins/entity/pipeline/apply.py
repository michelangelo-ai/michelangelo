"""Pipeline `apply` function plugin module."""

import sys
from inspect import Parameter
from logging import getLogger
from typing import Optional

from google.protobuf import descriptor_pool
from google.protobuf.json_format import MessageToJson, ParseError
from google.protobuf.message import Message
from google.protobuf.text_format import MessageToString
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
    EXPLAIN_PLAN_ANNOTATION,
    DeclarativeManifestError,
    explain_plan_from,
    printable,
    render_explain_plan,
)
from michelangelo.gen.api.v2.declarative_pb2 import ExplainPlan
from michelangelo.gen.api.v2.pipeline_pb2 import PipelineManifest

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
            "Output format, one of: table|json (default: table). A dry run of a"
            " declarative pipeline prints the explain plan. Other responses"
            " print the pipeline as text, or as JSON with json."
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
    except (RpcError, DeclarativeManifestError, ParseError) as err:
        raise SystemExit(f"Error: {_error_message(err)}") from None
    output = arguments.get("output", "table")
    plan = _dry_run_plan(response, arguments)
    if plan is not None:
        print(render_explain_plan(plan, output))
    elif output == "json":
        _print_json(response)
    else:
        _print_text(response)
    return response


def _print_json(response: Message) -> None:
    try:
        print(MessageToJson(response, preserving_proto_field_name=True))
    except TypeError as err:
        # MessageToJson raises TypeError for an Any type that is not loaded.
        print(
            f"Warning: cannot print the response as JSON ({printable(str(err))});"
            " printing it as text",
            file=sys.stderr,
        )
        _print_text(response)


def _print_text(response: Message) -> None:
    if _is_declarative_response(response):
        # The default pool expands the Any, so the stored workflow is readable.
        print(MessageToString(response, descriptor_pool=descriptor_pool.Default()))
    else:
        print(response)


def _is_declarative_response(response: Message) -> bool:
    # The CLI builds the response class from server reflection, in a separate
    # descriptor pool. Thus compare the enum value, not the class.
    return (
        response.pipeline.spec.manifest.type
        == PipelineManifest.PIPELINE_MANIFEST_TYPE_DECLARATIVE
    )


def _dry_run_plan(response: Message, arguments: dict) -> Optional[ExplainPlan]:
    # The apiserver sets the plan only on a dry run. A write is already stored,
    # so it must not fail on the plan.
    if not arguments.get("dry_run", False):
        return None
    try:
        return explain_plan_from(response.pipeline)
    except ParseError as err:
        raise SystemExit(
            f"Error: cannot read the {EXPLAIN_PLAN_ANNOTATION} annotation:"
            f" {_error_message(err)}"
        ) from None


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
