"""Pipeline `dev_run` function plugin module."""

from argparse import ArgumentParser
from inspect import Parameter, Signature
from logging import getLogger
from pathlib import Path
from types import MethodType
from typing import Optional

from git import Repo
from google.protobuf.json_format import ParseDict
from google.protobuf.message import Message
from grpc import Channel

from michelangelo.cli.mactl.crd import (
    CRD,
    METADATA_STUB,
    bind_signature,
    get_single_arg,
    inject_func_signature,
    yaml_to_dict,
)
from michelangelo.cli.mactl.grpc_tools import (
    get_message_class_by_name,
    get_methods_from_service,
)
from michelangelo.cli.mactl.plugins.entity.pipeline.create import (
    handle_workflow_inputs_retrieval,
    populate_pipeline_spec_with_workflow_inputs,
)
from michelangelo.cli.mactl.plugins.entity.pipeline.run import (
    generate_pipeline_run_name,
    generate_pipeline_run_object,
)
from michelangelo.uniflow.core.file_sync import DefaultFileSync

_ENV_VARIABLE_KEY = "env"
_UNIFLOW_IMAGE_ANNOTATION_KEY = "michelangelo/uniflow-image"

_LOG = getLogger(__name__)


def add_function_signature(crd: CRD) -> None:
    """Add function signature for pipeline dev_run command."""
    inject_func_signature(
        crd,
        "dev_run",
        {
            "help": "Run a pipeline locally for development and testing purposes",
            "args": [
                {
                    "func_signature": Parameter(
                        "file",
                        Parameter.POSITIONAL_OR_KEYWORD,
                    ),
                    "args": ["-f", "--file"],
                    "kwargs": {
                        "type": str,
                        "required": True,
                        "help": "Path to the pipeline YAML configuration file",
                    },
                },
                {
                    "func_signature": Parameter(
                        "env",
                        Parameter.POSITIONAL_OR_KEYWORD,
                        default=[],
                    ),
                    "args": ["--env"],
                    "kwargs": {
                        "type": str,
                        "required": False,
                        "action": "append",
                        "default": [],
                        "help": (
                            "Environment variable in format KEY=VALUE"
                            " (can be used multiple times)"
                        ),
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
                        "help": (
                            "Resume from a previous pipeline run. Format:"
                            " 'pipeline_run_name[:step_name]'"
                        ),
                    },
                },
                {
                    "func_signature": Parameter(
                        "file_sync",
                        Parameter.POSITIONAL_OR_KEYWORD,
                        default=False,
                    ),
                    "args": ["--file-sync"],
                    "kwargs": {
                        "action": "store_true",
                        "required": False,
                        "default": False,
                        "help": "Enable file synchronization for local code changes",
                    },
                },
            ],
        },
    )


def generate_dev_run(
    crd: CRD, channel: Channel, parser: Optional[ArgumentParser] = None
):
    """Generate dev run function for pipeline CRD."""
    _LOG.info("Generating `pipeline run` cr for dev-run: %s", crd)

    pipeline_run_service = "michelangelo.api.v2.PipelineRunService"
    methods, method_pool = get_methods_from_service(
        channel, pipeline_run_service, crd.metadata
    )
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

    crd.configure_parser("dev_run", parser)
    func_signature = crd._read_signatures("dev_run")

    @bind_signature(func_signature)
    def dev_run_func(bound_args: Signature) -> Message:
        _LOG.info("Start dev_run_func for pipeline")
        _LOG.info("Bound arguments: %r", bound_args.arguments)
        _self: CRD = bound_args.arguments["self"]

        _file = get_single_arg(bound_args.arguments, "file")

        # Handle optional resume_from parameter
        _resume_from_raw = bound_args.arguments.get("resume_from")
        if _resume_from_raw:
            _resume_from = get_single_arg(bound_args.arguments, "resume_from")
        else:
            _resume_from = None

        # Handle optional file_sync parameter
        _file_sync = bound_args.arguments.get("file_sync", False)  # pragma: no cover

        environment_variables = _process_env_variables(
            bound_args.arguments.get("env", [])
        )

        # parse pipeline yaml file
        yaml_path_string = _file
        yaml_path = Path(yaml_path_string).resolve()
        yaml_dict = _add_optional_params_to_yaml_dict(
            yaml_to_dict(yaml_path_string),
            environment_variables,
            _resume_from,
            _file_sync,
        )

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

    dev_run_func.__signature__ = func_signature  # type: ignore[attr-defined]
    crd.dev_run = MethodType(dev_run_func, crd)


def convert_crd_metadata_pipeline_dev_run(
    yaml_dict: dict, crd_class: type[Message], yaml_path: Path
) -> dict:
    """Convert CRD metadata for pipeline dev-run command.

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

    # Extract resume_from parameter if present
    resume_from = yaml_dict.get("resume_from")

    # Extract file_sync parameter and create tarball if enabled
    file_sync = yaml_dict.get("file_sync", False)
    file_sync_tarball_url = ""
    if file_sync:
        docker_image = (
            yaml_dict.get("metadata", {})
            .get("annotations", {})
            .get(_UNIFLOW_IMAGE_ANNOTATION_KEY, "")
        )
        _LOG.info("Creating file-sync tarball with docker_image: %s", docker_image)
        file_sync_tarball_url = DefaultFileSync(
            docker_image=docker_image,
        ).create_and_upload_tarball()
        _LOG.info("File-sync tarball uploaded to: %s", file_sync_tarball_url)

    pipeline_dev_run_cr = generate_pipeline_dev_run_object(
        yaml_dict, pipeline_spec, resume_from, file_sync_tarball_url
    )
    return {"pipeline_run": pipeline_dev_run_cr}


def generate_pipeline_dev_run_object(
    yaml_dict: dict,
    pipeline_spec: dict,
    resume_from: Optional[str] = None,
    file_sync_tarball_url: str = "",
) -> dict:
    """Generate Pipeline Dev Run CR as dictionary.

    Args:
        yaml_dict: YAML configuration dictionary
        pipeline_spec: Pipeline specification dictionary
        resume_from: Optional resume specification in format
            "pipeline_run_name:step_name"
        file_sync_tarball_url: Optional remote storage URL for file-sync tarball
    """
    namespace = yaml_dict.get("metadata", {}).get("namespace", "")
    pipeline_name = yaml_dict.get("metadata", {}).get("name", "")
    pipeline_run_name = generate_pipeline_run_name()

    pipeline_run_obj = generate_pipeline_run_object(
        run_name=pipeline_run_name,
        pipeline_name=pipeline_name,
        namespace=namespace,
        resume_from=resume_from,
    )

    pipeline_run_spec = pipeline_run_obj.setdefault("spec", {})
    # embed environment variables into pipeline_run.spec.inputs
    environment_variables = yaml_dict.get(_ENV_VARIABLE_KEY, {})

    # Add file-sync tarball URL to environment if present
    if file_sync_tarball_url:
        environment_variables["UF_FILE_SYNC_TARBALL_URL"] = file_sync_tarball_url

    if environment_variables:
        pipeline_run_spec["input"] = {
            "environ": environment_variables,
        }

    # embed uniflow image annotations into pipeline_run.metadata.annotations
    uniflow_image_annotation_value = (
        yaml_dict.get("metadata", {})
        .get("annotations", {})
        .get(_UNIFLOW_IMAGE_ANNOTATION_KEY, "")
    )
    if uniflow_image_annotation_value:
        pipeline_run_metadata = pipeline_run_obj.setdefault("metadata", {})
        pipeline_run_metadata["annotations"] = {
            _UNIFLOW_IMAGE_ANNOTATION_KEY: uniflow_image_annotation_value,
        }

    # embed pipeline_spec into pipeline_run.pipeline_run_spec
    pipeline_run_spec["pipeline_spec"] = pipeline_spec.get("spec", {})

    return pipeline_run_obj


def _process_env_variables(env_variables: list[str]) -> dict:
    """Process environment variables which are passed as a list of strings.

    Format of the environment variables is "<ENV_VAR>=<VALUE>".
    """
    env_dict = {}
    for env_variable in env_variables:
        key_value_pair = env_variable.split("=", 1)
        if len(key_value_pair) != 2:
            raise TypeError(
                f"Invalid environment variable format: {env_variable}, "
                f"expected format is <ENV_VAR>=<VALUE>"
            )
        env_dict[key_value_pair[0]] = key_value_pair[1]
    return env_dict


def _add_optional_params_to_yaml_dict(
    yaml_dict: dict,
    environment_variables: dict,
    resume_from: Optional[str],
    file_sync: bool,
) -> dict:
    """Add optional parameters to yaml_dict for dev_run command.

    Args:
        yaml_dict: Base YAML configuration dictionary
        environment_variables: Environment variables to add
        resume_from: Optional resume specification
        file_sync: Whether file sync is enabled

    Returns:
        Updated yaml_dict with optional parameters
    """
    yaml_dict[_ENV_VARIABLE_KEY] = environment_variables

    if resume_from:
        yaml_dict["resume_from"] = resume_from

    if file_sync:
        yaml_dict["file_sync"] = file_sync

    return yaml_dict
