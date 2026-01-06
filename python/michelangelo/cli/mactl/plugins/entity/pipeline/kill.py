"""Pipeline kill command implementation.

This module provides functionality to kill running pipeline runs by setting
the kill flag on a PipelineRun resource.
"""

from argparse import ArgumentParser
from inspect import Parameter, Signature
from logging import getLogger
from types import MethodType
from typing import Optional

from google.protobuf.json_format import MessageToDict, ParseDict
from google.protobuf.message import Message
from grpc import Channel

from michelangelo.cli.mactl.crd import (
    CRD,
    METADATA_STUB,
    bind_signature,
    get_single_arg,
    inject_func_signature,
)
from michelangelo.cli.mactl.grpc_tools import (
    get_message_class_by_name,
    get_methods_from_service,
)

# Import TypedStruct to register it in the descriptor pool
from michelangelo.gen.api import typed_struct_pb2  # noqa: F401

_LOG = getLogger(__name__)


def add_function_signature(crd: CRD) -> None:
    """Add function signature for pipeline kill command."""
    inject_func_signature(
        crd,
        "kill",
        {
            "help": "Kill a running pipeline.",
            "args": [
                {
                    "func_signature": Parameter(
                        "namespace",
                        Parameter.POSITIONAL_OR_KEYWORD,
                    ),
                    "args": ["-n", "--namespace"],
                    "kwargs": {
                        "type": str,
                        "required": True,
                        "help": "Namespace of the resource",
                    },
                },
                {
                    "func_signature": Parameter(
                        "name",
                        Parameter.POSITIONAL_OR_KEYWORD,
                    ),
                    "args": ["--name"],
                    "kwargs": {
                        "type": str,
                        "required": True,
                        "help": "Name of the pipeline run resource",
                    },
                },
                {
                    "func_signature": Parameter(
                        "yes",
                        Parameter.POSITIONAL_OR_KEYWORD,
                        default=False,
                    ),
                    "args": ["--yes"],
                    "kwargs": {
                        "action": "store_true",
                        "help": (
                            "Automatic yes to prompts; assume 'yes' as answer to "
                            "all prompts and run non-interactively."
                        ),
                    },
                },
            ],
        },
    )


def generate_kill(crd: CRD, channel: Channel, parser: Optional[ArgumentParser] = None):
    """Generate kill function for pipeline CRD.

    This function creates a kill command that sets the kill flag on a PipelineRun
    resource by calling the UpdatePipelineRun API.
    """
    _LOG.info("Generating `pipeline kill` for: %s", crd)

    # Use PipelineRunService directly since this operates on PipelineRun resources
    # (similar to how pipeline run command works)
    pipeline_run_service = "michelangelo.api.v2.PipelineRunService"
    methods, method_pool = get_methods_from_service(
        channel, pipeline_run_service, crd.metadata
    )

    # Get the GetPipelineRun and UpdatePipelineRun methods
    get_method_name = "GetPipelineRun"
    update_method_name = "UpdatePipelineRun"

    try:
        get_method = methods[get_method_name]
        update_method = methods[update_method_name]
    except KeyError as e:
        _LOG.error("Method not found in service %r: %s", pipeline_run_service, e)
        raise

    _LOG.info("Get method: %r", get_method)
    _LOG.info("Update method: %r", update_method)

    # Get input and output classes
    get_input_class = get_message_class_by_name(method_pool, get_method.input_type[1:])
    get_output_class = get_message_class_by_name(
        method_pool, get_method.output_type[1:]
    )
    update_input_class = get_message_class_by_name(
        method_pool, update_method.input_type[1:]
    )
    update_output_class = get_message_class_by_name(
        method_pool, update_method.output_type[1:]
    )

    crd.configure_parser("kill", parser)
    func_signature = crd._read_signatures("kill")

    @bind_signature(func_signature)
    def kill_func(bound_args: Signature) -> Message:
        _LOG.info("Start kill_func for pipeline")
        _LOG.info("Bound arguments: %r", bound_args.arguments)
        _self: CRD = bound_args.arguments["self"]
        _name = get_single_arg(bound_args.arguments, "name")
        _namespace = get_single_arg(bound_args.arguments, "namespace")
        _yes = bound_args.arguments.get("yes", False)

        if not _yes:
            confirmation = input(f" > kill pipeline run '{_name}'? [y/N] ")
            if confirmation.lower() not in ["y", "yes"]:
                print("Kill operation cancelled.")
                return None

        # Get the current PipelineRun resource
        _LOG.info("Retrieving PipelineRun: namespace=%s, name=%s", _namespace, _name)
        get_request = get_input_class(
            namespace=_namespace,
            name=_name,
        )

        get_method_fullname = f"/{pipeline_run_service}/{get_method_name}"
        get_stub = channel.unary_unary(
            get_method_fullname,
            request_serializer=get_input_class.SerializeToString,
            response_deserializer=get_output_class.FromString,
        )

        current_resource = get_stub(
            get_request,
            metadata=METADATA_STUB,
            timeout=30,
        )
        _LOG.info("Retrieved PipelineRun resource for kill: %r", current_resource)

        # Convert to dict and set kill flag
        current_dict = MessageToDict(current_resource, preserving_proto_field_name=True)

        if "pipeline_run" in current_dict and "spec" in current_dict["pipeline_run"]:
            current_dict["pipeline_run"]["spec"]["kill"] = True
        else:
            _LOG.error("Missing required spec field in the PipelineRun structure")
            raise ValueError("Cannot set kill flag on pipeline_run")

        # Create update request
        update_request = update_input_class()
        ParseDict(current_dict, update_request, ignore_unknown_fields=True)

        _LOG.info(
            "KILL Request input (%r) ready: %r",
            type(update_request),
            update_request,
        )

        # Call UpdatePipelineRun
        update_method_fullname = f"/{pipeline_run_service}/{update_method_name}"
        _LOG.info("Method fullname for gRPC call: %s", update_method_fullname)

        update_stub = channel.unary_unary(
            update_method_fullname,
            request_serializer=update_input_class.SerializeToString,
            response_deserializer=update_output_class.FromString,
        )

        response = update_stub(
            update_request,
            metadata=METADATA_STUB,
            timeout=30,
        )

        # Verify the kill flag was set
        response_dict = MessageToDict(response, preserving_proto_field_name=True)
        if (
            "pipeline_run" in response_dict
            and "spec" in response_dict["pipeline_run"]
            and response_dict["pipeline_run"]["spec"].get("kill") is True
        ):
            _LOG.info("Kill operation successfully set spec.kill=true")
        else:
            _LOG.error("Kill operation failed: spec.kill not set to true in response")
            raise RuntimeError(
                "Kill operation failed for pipeline_run: spec.kill not properly set"
            )

        _LOG.info("Kill operation completed (%r): %r", type(response), response)
        return response

    kill_func.__signature__ = func_signature
    crd.kill = MethodType(kill_func, crd)
