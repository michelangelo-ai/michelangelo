"""Pipeline delete command implementation.

This module provides a pipeline-specific ``delete`` command that overrides the
generic CRD delete. It always requests Kubernetes foreground propagation, so
deleting a Pipeline always terminates and removes its child runs (PipelineRuns,
TriggerRuns): k8s garbage collection drains the children via their finalizers
and deletes them before the Pipeline itself.

A warn/confirm prompt guards the delete because this cascade is destructive and
irreversible.
"""

from argparse import ArgumentParser
from inspect import Parameter, Signature
from logging import getLogger
from types import MethodType
from typing import Optional

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

# k8s DeletePropagation policy that triggers foreground cascade deletion.
# This matches metav1.DeletePropagationForeground on the server side: the
# apiserver records it as the michelangelo/DeletePropagation annotation and the
# ingester honours it as client.PropagationPolicy(Foreground), deleting the
# Pipeline's children before the Pipeline itself.
_PROPAGATION_FOREGROUND = "Foreground"


def add_function_signature(crd: CRD) -> None:
    """Add function signature for pipeline delete command."""
    inject_func_signature(
        crd,
        "delete",
        {
            "help": "Delete a pipeline.",
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
                        "help": "Name of the pipeline resource",
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


def generate_delete(
    crd: CRD, channel: Channel, parser: Optional[ArgumentParser] = None
):
    """Generate pipeline-specific delete function for the pipeline CRD.

    This overrides the generic delete (``CRD.generate_delete``) for the pipeline
    entity only. It always sets foreground propagation so the server cascades the
    delete to all child runs (PipelineRuns, TriggerRuns).
    """
    _LOG.info("Generating `pipeline delete` for: %s", crd)

    # Extract Delete method info
    method_name, input_class, output_class = crd._extract_method_info(
        channel, crd.full_name, "Delete"
    )

    crd.configure_parser("delete", parser)
    func_signature = crd._read_signatures("delete")

    @bind_signature(func_signature)
    def delete_func(bound_args: Signature) -> Optional[Message]:
        _LOG.info("Start delete_func for pipeline")
        _LOG.info("Bound arguments: %r", bound_args.arguments)
        _self: CRD = bound_args.arguments["self"]
        _name = get_single_arg(bound_args.arguments, "name")
        _namespace = get_single_arg(bound_args.arguments, "namespace")
        _yes = bound_args.arguments.get("yes", False)

        if not _yes:
            print(
                f" ! WARNING: deleting pipeline '{_name}' will terminate and "
                f"delete all of its child runs (PipelineRuns, TriggerRuns)."
            )
            confirmation = input(
                f" > delete pipeline '{_name}'? This cannot be undone. [y/N] "
            )
            if confirmation.lower() not in ["y", "yes"]:
                print("Delete operation cancelled.")
                return None

        # Build the delete request. Always set foreground propagation so that
        # k8s GC cascades the delete to owned children (PipelineRuns,
        # TriggerRuns) via their drain finalizers before removing the Pipeline.
        request_input = input_class()
        request_input.name = _name
        request_input.namespace = _namespace
        request_input.delete_options.propagationPolicy = _PROPAGATION_FOREGROUND

        _LOG.info(
            "DELETE Request input (%r) ready: %r",
            type(request_input),
            request_input,
        )

        # Call Delete method
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

        print(response)
        _LOG.info("Delete operation completed (%r): %r", type(response), response)
        return response

    delete_func.__signature__ = func_signature
    crd.delete = MethodType(delete_func, crd)
