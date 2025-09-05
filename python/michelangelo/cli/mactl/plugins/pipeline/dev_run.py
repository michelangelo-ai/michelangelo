from types import MethodType
from logging import getLogger
from inspect import Signature, Parameter
from pathlib import Path


from git import Repo
from google.protobuf.json_format import ParseDict
from google.protobuf.message import Message
from grpc import Channel


from mactl import (
    CRD,
    METADATA_STUB,
    bind_signature,
    get_message_class_by_name,
    get_methods_from_service,
    yaml_to_dict,
)

from plugins.pipeline.create import (
    handle_workflow_inputs_retrieval,
    populate_pipeline_spec_with_workflow_inputs,
)

from plugins.pipeline.run import (
    generate_pipeline_run_object,
    generate_pipeline_run_name,
)

_ENV_VARIABLE_KEY = "env"

_LOG = getLogger(__name__)


def generate_dev_run(crd: CRD, channel: Channel):
    """
    Generate dev run function for pipeline CRD.
    """
    _LOG.info("Generating `pipeline run` cr for dev-run: %s", crd)

    pipeline_run_service = "michelangelo.api.v2.PipelineRunService"
    methods, method_pool = get_methods_from_service(channel, pipeline_run_service)
    method_name = "CreatePipelineRun"

    _LOG.info("Run input/output of method %r", method_name)
    try:
        method_run = methods[method_name]
    except KeyError:
        _LOG.warning(
            "Method %r not found in service %r", method_name, pipeline_run_service
        )
        return

    _LOG.info("Run method input type: %r", method_run.input_type)
    _LOG.info("Run method output type: %r", method_run.output_type)
    input_class = get_message_class_by_name(method_pool, method_run.input_type[1:])
    output_class = get_message_class_by_name(method_pool, method_run.output_type[1:])

    dev_run_func_signature = Signature(
        [Parameter("self", Parameter.POSITIONAL_OR_KEYWORD)]
        + [Parameter(name, Parameter.POSITIONAL_OR_KEYWORD) for name in ["file", "env"]]
    )

    @bind_signature(dev_run_func_signature)
    def dev_run_func(bound_args: Signature) -> Message:
        _LOG.info("Start dev_run_func for pipeline")
        _LOG.info("Bound arguments: %r", bound_args.arguments)
        _self: CRD = bound_args.arguments["self"]

        if len(bound_args.arguments["file"]) != 1:
            raise ValueError('exactly one "file" argument is required')

        environment_variables = _process_env_variables(bound_args.arguments["env"])

        # parse pipeline yaml file
        yaml_path_string = bound_args.arguments["file"][0]
        yaml_path = Path(yaml_path_string).resolve()
        yaml_dict = yaml_to_dict(yaml_path_string)
        yaml_dict[_ENV_VARIABLE_KEY] = environment_variables

        pipeline_dev_run_dict = _self.func_crd_metadata_converter(
            yaml_dict, input_class, yaml_path
        )

        _LOG.debug("CR content: %r", pipeline_dev_run_dict)

        request_input = input_class()
        ParseDict(pipeline_dev_run_dict, request_input)

        method_fullname = f"/{pipeline_run_service}/{method_name}"
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

    dev_run_func.__signature__ = dev_run_func_signature
    crd.dev_run = MethodType(dev_run_func, crd)


def convert_crd_metadata_pipeline_dev_run(
    yaml_dict: dict, crd_class: type[Message], yaml_path: Path
) -> dict:
    """
    Convert CRD metadata for pipeline dev-run command.
    This function generates a PipelineRunRequest object from command line arguments.
    """
    _LOG.info("Converting metadata for pipeline run command")

    if not isinstance(yaml_dict, dict):
        _LOG.error("Expected a dictionary, got: %r", type(yaml_dict))
        raise ValueError("Expected a dictionary for pipeline run metadata")

    repo = Repo(".", search_parent_directories=True)
    repo_root = Path(repo.git.rev_parse("--show-toplevel")).resolve()
    _LOG.info("Current git repository info: %r", repo)

    # Extract project and pipeline names from metadata
    project = yaml_dict["metadata"]["namespace"]  # Assuming namespace maps to project
    pipeline = yaml_dict["metadata"]["name"]

    # Get relative path of config file from repo root
    config_file_relative_path = str(yaml_path.relative_to(repo_root))

    workflow_inputs, uniflow_tar_path, workflow_function_name = (
        handle_workflow_inputs_retrieval(
            repo_root, config_file_relative_path, project, pipeline
        )
    )

    pipeline_spec = populate_pipeline_spec_with_workflow_inputs(
        {},
        yaml_dict,
        workflow_inputs,
        repo,
        yaml_path,
        repo_root,
        config_file_relative_path,
        uniflow_tar_path,
        workflow_function_name,
    )

    pipeline_dev_run_cr = generate_pipeline_dev_run_object(yaml_dict, pipeline_spec)
    return {"pipeline_run": pipeline_dev_run_cr}


def generate_pipeline_dev_run_object(yaml_dict: dict, pipeline_spec: dict) -> dict:
    """
    Generate Pipeline Dev Run CR as dictionary.
    """

    namespace = yaml_dict.get("metadata", {}).get("namespace", "")
    pipeline_name = yaml_dict.get("metadata", {}).get("name", "")
    pipeline_run_name = generate_pipeline_run_name()

    pipeline_run_obj = generate_pipeline_run_object(
        run_name=pipeline_run_name, pipeline_name=pipeline_name, namespace=namespace
    )

    pipeline_run_spec = pipeline_run_obj.setdefault("spec", {})
    # embed environment variables into pipeline_run.spec.inputs
    pipeline_run_spec["input"] = yaml_dict.get(_ENV_VARIABLE_KEY, {})
    # embed pipeline_spec into pipeline_run.pipeline_run_spec
    pipeline_run_spec["pipeline_spec"] = pipeline_spec.get("spec", {})

    return pipeline_run_obj


def _process_env_variables(env_variables: list[str]) -> dict:
    """
    Process environment variables which are passed as a list of strings.
    Format of the environment variables is "<ENV_VAR>=<VALUE>".
    """
    env_dict = {}
    for env_variable in env_variables:
        key_value_pair = env_variable.split("=", 1)
        if len(key_value_pair) != 2:
            raise TypeError(
                f"Invalid environment variable format: {env_variable}, expected format is <ENV_VAR>=<VALUE>"
            )
        env_dict[key_value_pair[0]] = key_value_pair[1]
    return env_dict
