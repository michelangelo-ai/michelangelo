"""TriggerRun `create` function plugin module."""

import uuid
from argparse import ArgumentParser
from inspect import Parameter, Signature
from logging import getLogger
from types import MethodType

from google.protobuf.json_format import MessageToDict, ParseDict
from grpc import Channel, RpcError, StatusCode

from michelangelo.cli.mactl.crd import (
    CRD,
    METADATA_STUB,
    bind_signature,
    get_single_arg,
    inject_func_signature,
)
from michelangelo.cli.mactl.utils import get_user_name

_LOG = getLogger(__name__)


def add_function_signature(crd: CRD) -> None:
    """Add function signature for trigger_run create command."""
    inject_func_signature(
        crd,
        "create",
        {
            "help": "Create a TriggerRun from a pipeline's trigger configuration.",
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
                        "help": "Namespace of the pipeline and trigger run",
                    },
                },
                {
                    "func_signature": Parameter(
                        "pipeline",
                        Parameter.POSITIONAL_OR_KEYWORD,
                    ),
                    "args": ["-p", "--pipeline"],
                    "kwargs": {
                        "type": str,
                        "required": True,
                        "help": "Name of the pipeline to create a trigger run for",
                    },
                },
                {
                    "func_signature": Parameter(
                        "trigger_name",
                        Parameter.POSITIONAL_OR_KEYWORD,
                    ),
                    "args": ["-t", "--trigger-name"],
                    "kwargs": {
                        "type": str,
                        "required": True,
                        "help": (
                            "Name of the trigger from the pipeline's "
                            "registered triggerMap"
                        ),
                    },
                },
            ],
        },
    )


def generate_create(crd: CRD, channel: Channel, parser: ArgumentParser):
    """Generate create function for trigger_run.

    Creates a TriggerRun CR by fetching the pipeline and extracting
    the trigger configuration from its triggerMap.
    """
    _LOG.info("Generating create function for TriggerRun")

    method_name, input_class, output_class = crd._extract_method_info(
        channel, crd.full_name, "Create"
    )
    crd.configure_parser("create", parser)
    func_signature = crd._read_signatures("create")

    @bind_signature(func_signature)
    def create_func(bound_args: Signature):
        """Implementation of trigger_run create command."""
        _LOG.info("Bound arguments: %r", bound_args.arguments)
        _self: CRD = bound_args.arguments["self"]
        namespace = get_single_arg(bound_args.arguments, "namespace")
        pipeline_name = get_single_arg(bound_args.arguments, "pipeline")
        trigger_name = get_single_arg(bound_args.arguments, "trigger_name")

        _LOG.info(
            f"Creating TriggerRun for pipeline={pipeline_name}, "
            f"trigger={trigger_name}, namespace={namespace}"
        )

        # Get pipeline using gRPC directly
        _LOG.info("Fetching pipeline via gRPC")
        pipeline_method_name, pipeline_input_class, pipeline_output_class = (
            _self._extract_method_info(
                channel, "michelangelo.api.v2.PipelineService", "Get"
            )
        )
        get_pipeline_dict = {
            "name": pipeline_name,
            "namespace": namespace,
        }
        get_pipeline_request = pipeline_input_class()
        ParseDict(get_pipeline_dict, get_pipeline_request)
        pipeline_method_fullname = (
            f"/michelangelo.api.v2.PipelineService/{pipeline_method_name}"
        )
        pipeline_stub_method = channel.unary_unary(
            pipeline_method_fullname,
            request_serializer=pipeline_input_class.SerializeToString,
            response_deserializer=pipeline_output_class.FromString,
        )

        try:
            pipeline_response = pipeline_stub_method(
                get_pipeline_request,
                metadata=METADATA_STUB,
                timeout=30,
            )
            pipeline = pipeline_response.pipeline
            _LOG.info(f"Retrieved pipeline: {pipeline.metadata.name}")
        except RpcError as err:
            _LOG.error(f"gRPC error getting pipeline {pipeline_name}: {err}")
            if err.code() == StatusCode.NOT_FOUND:
                raise ValueError(
                    f"Pipeline '{pipeline_name}' not found in namespace '{namespace}'"
                ) from err
            else:
                raise RuntimeError(
                    f"Failed to get pipeline '{pipeline_name}': {err.details()}"
                ) from err

        if not hasattr(pipeline.spec, "manifest") or not hasattr(
            pipeline.spec.manifest, "trigger_map"
        ):
            raise ValueError(
                f"Pipeline '{pipeline_name}' does not have any triggers configured"
            )
        trigger_map = MessageToDict(
            pipeline.spec.manifest.trigger_map, preserving_proto_field_name=True
        )

        if trigger_name not in trigger_map:
            available_triggers = ", ".join(trigger_map.keys())
            raise ValueError(
                f"Trigger '{trigger_name}' not found in pipeline '{pipeline_name}'. "
                f"Available triggers: {available_triggers}"
            )

        _LOG.info(f"Found trigger configuration for: {trigger_name}")

        # Generate trigger run dict with configurations
        trigger_config = trigger_map[trigger_name]
        random_hex = uuid.uuid4().hex[:8]
        trigger_run_name = f"{trigger_name}_{random_hex}"
        trigger_run_dict = {
            "triggerRun": {
                "metadata": {
                    "name": trigger_run_name,
                    "namespace": namespace,
                },
                "spec": {
                    "pipeline": {"name": pipeline_name, "namespace": namespace},
                    "sourceTriggerName": trigger_name,
                    "actor": {"name": get_user_name()},
                    "trigger": trigger_config,
                },
            }
        }

        request_input = input_class()
        ParseDict(trigger_run_dict, request_input)

        _LOG.info(
            "Create request input (%r) ready: %r",
            type(request_input),
            request_input,
        )

        method_fullname = f"/{crd.full_name}/{method_name}"
        _LOG.info("Method fullname for gRPC call: %s", method_fullname)

        stub_method = channel.unary_unary(
            method_fullname,
            request_serializer=input_class.SerializeToString,
            response_deserializer=output_class.FromString,
        )

        try:
            response = stub_method(
                request_input,
                metadata=METADATA_STUB,
                timeout=30,
            )
        except RpcError as err:
            _LOG.error(f"gRPC error creating TriggerRun: {err}")
            raise RuntimeError(
                f"Failed to create TriggerRun: {err.details()}"
            ) from err

        _LOG.info(
            f"Successfully created TriggerRun: {response.trigger_run.metadata.name}"
        )
        return response

    create_func.__signature__ = func_signature
    crd.create = MethodType(create_func, crd)
