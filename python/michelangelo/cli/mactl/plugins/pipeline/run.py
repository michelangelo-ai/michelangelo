import time
import uuid
from logging import getLogger

from grpc import Channel

from mactl import CRD

from inspect import Signature, Parameter
from types import MethodType
from google.protobuf.message import Message
from google.protobuf.json_format import ParseDict
from mactl import (
    get_methods_from_service,
    get_message_class_by_name,
    bind_signature,
    METADATA_STUB,
    get_single_arg,
)


_LOG = getLogger(__name__)


def generate_run(crd: CRD, channel: Channel):
    """
    Generate run function for pipeline CRD.
    """
    _LOG.info("Generating `pipeline run` crd for: %s", crd)

    pipeline_run_service = "michelangelo.api.v2.PipelineRunService"
    methods, method_pool = get_methods_from_service(channel, pipeline_run_service)
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

    run_func_signature = Signature(
        [Parameter("self", Parameter.POSITIONAL_OR_KEYWORD)]
        + [
            Parameter(name, Parameter.POSITIONAL_OR_KEYWORD)
            for name in ["namespace", "name"]
        ]
    )

    @bind_signature(run_func_signature)
    def run_func(bound_args: Signature) -> Message:
        _LOG.info("Start run_func for pipeline")
        _LOG.info("Bound arguments: %r", bound_args.arguments)
        _self: CRD = bound_args.arguments["self"]

        _namespace = get_single_arg(bound_args.arguments, "namespace")
        _name = get_single_arg(bound_args.arguments, "name")

        run_kwargs = {
            "namespace": _namespace,
            "name": _name,
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

    run_func.__signature__ = run_func_signature  # type: ignore[attr-defined]
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
    run_name = generate_pipeline_run_name()

    _LOG.info(
        "Generating pipeline run: %s for pipeline: %s in namespace: %s",
        run_name,
        pipeline_name,
        namespace,
    )

    pipeline_run = generate_pipeline_run_object(
        run_name=run_name, pipeline_name=pipeline_name, namespace=namespace
    )

    return {"pipeline_run": pipeline_run}


def generate_pipeline_run_object(
    run_name: str, pipeline_name: str, namespace: str
) -> dict:
    """
    Generate PipelineRun object as dictionary.

    Args:
        run_name: Generated unique name for the pipeline run
        pipeline_name: Name of the target pipeline to run
        namespace: Kubernetes namespace

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
            "resume": {
                "forceResume": False,
                "pipelineRun": {
                    "name": "run-1758668723-006f3623",
                    "namespace": "ma-dev-test",
                },
                "resumeFrom": [],
                "resumeUpTo": [],
            },
        },
    }

    _LOG.info("Generated pipeline run object: %s", run_name)
    return pipeline_run_dict


def generate_pipeline_run_name() -> str:
    """
    Generates a pipeline-run name.
    """
    timestamp = int(time.time())
    uuid8 = str(uuid.uuid4())[:8]
    return f"run-{timestamp}-{uuid8}"
