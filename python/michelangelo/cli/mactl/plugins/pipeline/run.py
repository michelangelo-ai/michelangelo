from argparse import ArgumentParser
from inspect import Signature, Parameter
from logging import getLogger
from types import MethodType
from typing import Optional
import time
import uuid

from google.protobuf.json_format import ParseDict
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


_LOG = getLogger(__name__)


def add_function_signature(crd: CRD) -> None:
    """
    Add function signature for pipeline run command.
    """
    inject_func_signature(
        crd,
        "run",
        {
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
                        "help": "Name of the resource",
                    },
                },
                {
                    "func_signature": Parameter(
                        "resume_from",
                        Parameter.POSITIONAL_OR_KEYWORD,
                        default=None,
                    ),
                    "args": ["--resume_from"],
                    "kwargs": {
                        "type": str,
                        "required": False,
                        "default": None,
                        "help": "Resume from a previous pipeline run. Format: 'pipeline_run_name[:step_name]'",
                    },
                },
            ],
        },
    )


def generate_run(crd: CRD, channel: Channel, parser: Optional[ArgumentParser] = None):
    """
    Generate run function for pipeline CRD.
    """
    _LOG.info("Generating `pipeline run` crd for: %s", crd)

    pipeline_run_service = "michelangelo.api.v2.PipelineRunService"
    methods, method_pool = get_methods_from_service(
        channel, pipeline_run_service, crd.metadata
    )
    method_name = "CreatePipelineRun"

    _LOG.info("Run input/output of method %r", method_name)
    try:
        method_create = methods[method_name]
    except KeyError:
        _LOG.warning(
            "Method %r not found in service %r", method_name, pipeline_run_service
        )
        return

    _LOG.info("Run method input type: %r", method_create.input_type)
    _LOG.info("Run method output type: %r", method_create.output_type)
    input_class = get_message_class_by_name(method_pool, method_create.input_type[1:])
    output_class = get_message_class_by_name(method_pool, method_create.output_type[1:])

    crd.configure_parser("run", parser)
    func_signature = crd._read_signatures("run")

    @bind_signature(func_signature)
    def run_func(bound_args: Signature) -> Message:
        _LOG.info("Start run_func for pipeline")
        _LOG.info("Bound arguments: %r", bound_args.arguments)
        _self: CRD = bound_args.arguments["self"]

        _namespace = get_single_arg(bound_args.arguments, "namespace")
        _name = get_single_arg(bound_args.arguments, "name")

        # Handle optional resume_from parameter
        _resume_from = bound_args.arguments.get("resume_from")

        run_kwargs = {
            "namespace": _namespace,
            "name": _name,
            "resume_from": _resume_from,
        }

        pipeline_run_dict = _self.func_crd_metadata_converter(
            run_kwargs, input_class, None
        )

        request_input = input_class()
        ParseDict(pipeline_run_dict, request_input)

        service_name = "michelangelo.api.v2.PipelineRunService"
        method_fullname = f"/{service_name}/{method_name}"
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
        _LOG.info("Stub method completed (%r): %r", type(response), response)
        return response

    run_func.__signature__ = func_signature  # type: ignore[attr-defined]
    crd.run = MethodType(run_func, crd)


def convert_crd_metadata_pipeline_run(
    yaml_dict: dict, crd_class: type, yaml_path
) -> dict:
    """
    Convert CRD metadata for pipeline run command.
    This function generates a CreatePipelineRunRequest object from command line arguments.
    """
    _LOG.info("Converting metadata for pipeline run command")

    if not isinstance(yaml_dict, dict):
        _LOG.error("Expected a dictionary, got: %r", type(yaml_dict))
        raise ValueError("Expected a dictionary for pipeline run metadata")

    # Validate required arguments
    if "namespace" not in yaml_dict:
        raise ValueError("--namespace is required for pipeline run")
    if "name" not in yaml_dict:
        raise ValueError("--name is required for pipeline run")

    namespace = yaml_dict["namespace"]
    pipeline_name = yaml_dict["name"]
    resume_from = yaml_dict.get("resume_from")
    run_name = generate_pipeline_run_name()

    _LOG.info(
        "Generating pipeline run: %s for pipeline: %s in namespace: %s",
        run_name,
        pipeline_name,
        namespace,
    )

    pipeline_run = generate_pipeline_run_object(
        run_name=run_name,
        pipeline_name=pipeline_name,
        namespace=namespace,
        resume_from=resume_from,
    )

    return {"pipeline_run": pipeline_run}


def generate_pipeline_run_object(
    run_name: str, pipeline_name: str, namespace: str, resume_from: str = None
) -> dict:
    """
    Generate PipelineRun object as dictionary.

    Args:
        run_name: Generated unique name for the pipeline run
        pipeline_name: Name of the target pipeline to run
        namespace: Kubernetes namespace
        resume_from: Optional resume specification in format "pipeline_run_name:step_name"

    Returns:
        dict: Configured pipeline run object as dictionary
    """

    pipeline_run_dict = {
        "typeMeta": {
            "kind": "PipelineRun",
            "apiVersion": "michelangelo.api/v2",
        },
        "metadata": {
            "name": run_name,
            "namespace": namespace,
        },
        "spec": {
            "pipeline": {
                "name": pipeline_name,
                "namespace": namespace,
            },
            "actor": {
                "name": "mactl-user",
            },
        },
    }

    # Add resume spec if resume_from is provided
    if resume_from:
        resume_spec = parse_resume_from(resume_from, namespace)
        if resume_spec:
            pipeline_run_dict["spec"]["resume"] = resume_spec
            _LOG.info("Added resume spec to pipeline run: %r", resume_spec)
        else:
            _LOG.warning("Failed to parse resume_from: %r", resume_from)

    _LOG.info("Generated pipeline run object: %s", run_name)
    return pipeline_run_dict


def parse_resume_from(resume_from: str, namespace: str) -> dict:
    """
    Parse the resume_from parameter and return a resume spec.

    Args:
        resume_from: Resume specification in format "pipeline_run_name" or "pipeline_run_name:step_name"
        namespace: Kubernetes namespace for the pipeline run reference

    Returns:
        dict: Resume spec dictionary matching the Resume proto message
    """
    if not resume_from:
        _LOG.error(
            "Invalid resume_from format. Expected 'pipeline_run_name' or 'pipeline_run_name:step_name', got: %r",
            resume_from,
        )
        return None

    # Check if step name is provided
    if ":" in resume_from:
        pipeline_run_name, step_name = resume_from.split(":", 1)
        resume_from_list = [step_name]
    else:
        pipeline_run_name = resume_from
        resume_from_list = []

    resume_spec = {
        "pipelineRun": {
            "name": pipeline_run_name,
            "namespace": namespace,
        },
        "resumeFrom": resume_from_list,
    }

    _LOG.info("Parsed resume_from '%s' to resume spec: %r", resume_from, resume_spec)
    return resume_spec


def generate_pipeline_run_name() -> str:
    """
    Generates a pipeline-run name.
    """
    timestamp = int(time.time())
    uuid8 = str(uuid.uuid4())[:8]
    return f"run-{timestamp}-{uuid8}"
