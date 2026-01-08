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

    # Generate get method to reuse it
    crd.generate_get(channel)

    # Extract Update method info
    method_name, input_class, output_class = crd._extract_method_info(
        channel, crd.full_name, "Update"
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

        # Get the current PipelineRun resource using the generated get method
        current_resource = _self.get(_namespace, _name)
        _LOG.info("Retrieved PipelineRun resource for kill: %r", current_resource)

        # Convert to dict and set kill flag
        current_dict = MessageToDict(current_resource, preserving_proto_field_name=True)

        resource_name = _self.name
        if resource_name in current_dict and "spec" in current_dict[resource_name]:
            current_dict[resource_name]["spec"]["kill"] = True
        else:
            _LOG.error("Missing required spec field in the resource structure")
            raise ValueError(f"Cannot set kill flag on {resource_name}")

        # Create update request
        request_input = input_class()
        ParseDict(current_dict, request_input, ignore_unknown_fields=True)

        _LOG.info(
            "KILL Request input (%r) ready: %r",
            type(request_input),
            request_input,
        )

        # Call Update method
        method_fullname = f"/{_self.full_name}/{method_name}"
        _LOG.info("Method fullname for gRPC call: %s", method_fullname)

        stub_method = channel.unary_unary(
            method_fullname,
            request_serializer=input_class.SerializeToString,
            response_deserializer=output_class.FromString,
        )

        response = stub_method(
            request_input,
            metadata=METADATA_STUB,
            timeout=30,
        )

        # Verify the kill flag was set
        response_dict = MessageToDict(response, preserving_proto_field_name=True)
        resource_name = _self.name
        if (
            resource_name in response_dict
            and "spec" in response_dict[resource_name]
            and response_dict[resource_name]["spec"].get("kill") is True
        ):
            _LOG.info("Kill operation successfully set spec.kill=true")
        else:
            _LOG.error("Kill operation failed: spec.kill not set to true in response")
            raise RuntimeError(
                f"Kill operation failed for {resource_name}: spec.kill not properly set"
            )

        _LOG.info("Kill operation completed (%r): %r", type(response), response)
        return response

    kill_func.__signature__ = func_signature
    crd.kill = MethodType(kill_func, crd)
