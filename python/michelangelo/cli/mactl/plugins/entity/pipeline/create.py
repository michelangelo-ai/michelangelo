import json
import tempfile
from copy import deepcopy
from logging import getLogger
from os import getenv
from pathlib import Path
from uuid import uuid4

from git import Repo
from google.protobuf.any_pb2 import Any
from google.protobuf.message import Message
from google.protobuf.struct_pb2 import Struct

from michelangelo.cli.mactl.utils import (
    read_subprocess_outputs,
    run_subprocess_registration,
)
from michelangelo.gen.api.typed_struct_pb2 import TypedStruct

_LOG = getLogger(__name__)

# TODO: Add end-to-end tests for get_pipeline_config_and_tar() with real config files and subprocess execution

# Constants for registration output files
_UNIFLOW_TAR_PATH_FILENAME = "uniflow_tar_path.txt"
_UNIFLOW_INPUT_FILENAME = "uniflow_input.txt"


def get_pipeline_config_and_tar(
    repo_root: Path,
    config_file_relative_path: str,
    bazel_target: str,
    project: str,
    pipeline: str,
    yaml_dict: dict = None,
) -> tuple[Struct, str, str]:
    """Run pipeline registration via subprocess to get uniflow artifacts.

    Executes registration in the user's Python environment to obtain:
    1) uniflow tarball path from "uniflow_tar_path.txt"
    2) uniflow workflow input from "uniflow_input.txt" converted to Struct

    Uses subprocess isolation to maintain clean separation between
    MaCTL's environment and the user's pipeline dependencies.

    Args:
        repo_root: Root directory of the git repository
        config_file_relative_path: Relative path to config file from repo root
        bazel_target: Bazel target (unused)
        project: Project name
        pipeline: Pipeline name

    Returns:
        tuple: (workflow_inputs as Struct, uniflow_tar_path as string, workflow_function_name as string)

    Raises:
        FileNotFoundError: If config file doesn't exist
        RuntimeError: If subprocess registration fails
    """
    config_file_path = repo_root / config_file_relative_path

    # Validate config file exists
    if not config_file_path.exists():
        raise FileNotFoundError(f"Config file {config_file_path} does not exist")

    # Create temporary directory for registration outputs
    with tempfile.TemporaryDirectory(prefix="mactl_") as tmp_dir:
        tmp_path = Path(tmp_dir)

        _LOG.info(
            "Running subprocess registration for project=%s, pipeline=%s",
            project,
            pipeline,
        )

        try:
            # Execute registration in subprocess (user's Python environment)
            # The subprocess will read the config file and discover the workflow function
            result = run_subprocess_registration(
                project=project,
                pipeline=pipeline,
                config_file_path=str(config_file_path),
                output_dir=str(tmp_path),
                storage_url=None,  # Use default S3 path
                output_filename=None,  # Use default filename
                environ=None,
                args=None,
                kwargs=None,
            )

            # Check subprocess result
            if result.returncode != 0:
                _LOG.error(
                    "Subprocess registration failed with exit code %d",
                    result.returncode,
                )
                _LOG.error("Subprocess stderr: %s", result.stderr)
                raise RuntimeError(f"Registration subprocess failed: {result.stderr}")

            _LOG.info("Subprocess registration completed successfully")

        except Exception as e:
            _LOG.error("Failed to execute registration subprocess: %s", e)
            raise RuntimeError(f"Error running pipeline registration: {e}")

        # Read subprocess outputs using status files
        success, message, remote_path = read_subprocess_outputs(str(tmp_path))

        if not success:
            raise RuntimeError(f"Registration failed: {message}")

        _LOG.info("Registration successful: %s", message)

        # Read uniflow tar path
        tar_path_file = tmp_path / _UNIFLOW_TAR_PATH_FILENAME

        try:
            uniflow_tar_path = tar_path_file.read_text().strip()
            _LOG.info("Read uniflow tar path: %s", uniflow_tar_path)
        except FileNotFoundError:
            # Use remote path from status file if direct file read fails
            if remote_path:
                uniflow_tar_path = remote_path
                _LOG.info("Using tar path from status file: %s", uniflow_tar_path)
            else:
                raise RuntimeError(
                    f"Could not read uniflow tar path from {tar_path_file}"
                )

        # Read uniflow workflow input
        input_file_path = tmp_path / _UNIFLOW_INPUT_FILENAME
        try:
            content = input_file_path.read_text()
            input_data = json.loads(content)
        except FileNotFoundError:
            raise RuntimeError(f"Could not read uniflow input from {input_file_path}")
        except json.JSONDecodeError as e:
            raise RuntimeError(f"Error parsing uniflow input JSON: {e}")

        # Convert to protobuf Struct
        workflow_inputs = Struct()
        workflow_inputs.update(input_data)

        # Read workflow function name
        function_name_file = tmp_path / "workflow_function_name.txt"
        try:
            workflow_function_name = function_name_file.read_text().strip()
            _LOG.info("Read workflow function name: %s", workflow_function_name)
        except FileNotFoundError:
            workflow_function_name = ""
            _LOG.warning("Could not read workflow function name file")

        return workflow_inputs, uniflow_tar_path, workflow_function_name


def convert_crd_metadata_pipeline_create(
    yaml_dict: dict, crd_class: type[Message], yaml_path: Path
) -> dict:
    """Convert CRD metadata for pipeline create crd.
    Integrates pipeline registration to get uniflow artifacts.
    """
    _LOG.info("Convert CRD metadata for class %r", crd_class)
    if not isinstance(yaml_dict, dict):
        _LOG.error("Expected a dictionary, got: %r", type(yaml_dict))
        raise ValueError("Expected a dictionary for CRD metadata")

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

    res = {}

    res["metadata"] = {
        "annotations": yaml_dict["metadata"].get("annotations", {}),
        "labels": yaml_dict["metadata"].get("labels", {}),
        "generateName": "",
        "generation": "0",
        "name": pipeline,
        "namespace": project,
        "resourceVersion": "0",
        "uid": str(uuid4()),
    }

    return populate_pipeline_spec_with_workflow_inputs(
        res,
        yaml_dict,
        workflow_inputs,
        repo,
        yaml_path,
        repo_root,
        config_file_relative_path,
        uniflow_tar_path,
        workflow_function_name,
    )


def handle_workflow_inputs_retrieval(
    repo_root: Path, config_file_relative_path: str, project: str, pipeline: str
) -> tuple[dict, str, str]:
    """Handle workflow inputs retrieval from subprocess registration.
    """
    workflow_inputs = None
    uniflow_tar_path = ""
    workflow_function_name = ""

    # Run pipeline registration to get uniflow artifacts
    try:
        workflow_inputs, uniflow_tar_path, workflow_function_name = (
            get_pipeline_config_and_tar(
                repo_root=repo_root,
                config_file_relative_path=config_file_relative_path,
                bazel_target="",  # Not used
                project=project,
                pipeline=pipeline,
            )
        )
        _LOG.info("Successfully obtained pipeline config and tar")
    except FileNotFoundError as e:
        _LOG.error("Config file not found: %s", e)
        raise ValueError(f"Pipeline configuration file is missing: {e}")
    except RuntimeError as e:
        _LOG.error("Registration subprocess failed: %s", e)
        # Check if this is a critical failure or can be handled gracefully
        if "Python interpreter" in str(e):
            raise ValueError(
                f"Could not detect suitable Python environment for registration: {e}. "
                "Please ensure you're in a valid Python project environment."
            )
        elif "workflow function" in str(e).lower():
            raise ValueError(
                f"Workflow function not found: {e}. "
                f"Please ensure {project}.{pipeline}_workflow exists and is importable."
            )
        else:
            # For other registration failures, continue with graceful degradation
            _LOG.warning(
                "Registration failed, continuing without uniflow artifacts: %s", e
            )
    except Exception as e:
        _LOG.error("Unexpected error during registration: %s", e)
        # For unexpected errors, also use graceful degradation
        _LOG.warning(
            "Unexpected registration failure, continuing without uniflow artifacts"
        )
    return workflow_inputs, uniflow_tar_path, workflow_function_name


def populate_pipeline_spec_with_workflow_inputs(
    res: dict,
    yaml_dict: dict,
    workflow_inputs: dict,
    repo: Repo,
    yaml_path: Path,
    repo_root: Path,
    config_file_relative_path: str,
    uniflow_tar_path: str,
    workflow_function_name: str,
) -> dict:
    """Populate pipeline spec with workflow inputs.
    """
    res["spec"] = deepcopy(yaml_dict["spec"])
    res["spec"]["commit"] = {
        "branch": repo.active_branch.name,
        "git_ref": repo.head.commit.hexsha,
    }
    assert yaml_path.resolve().is_relative_to(repo_root), (
        f"Expected {yaml_path.resolve()} to be relative to {repo_root}"
    )

    res["spec"]["manifest"] = {
        "filePath": config_file_relative_path,
        "type": "PIPELINE_MANIFEST_TYPE_UNIFLOW",
    }
    res["spec"]["owner"] = {"name": getenv("UBER_LDAP_UID")}

    # Add uniflow artifacts if registration succeeded
    if workflow_inputs is not None:
        # Convert protobuf Struct back to dict
        from google.protobuf.json_format import MessageToDict

        input_dict = MessageToDict(workflow_inputs)

        # Create manifest content in the format expected by internal code
        # This matches the structure: value.fields.kwargs.list_value.values...

        # Build kwargs structure
        kwargs_values = []
        for key, value in input_dict.get("kwargs", []):
            kwargs_values.append(
                {
                    "list_value": {
                        "values": [
                            {"string_value": str(key)},
                            {"string_value": str(value)},
                        ]
                    }
                }
            )

        # Build environ structure
        environ_fields = {}
        for key, value in input_dict.get("environ", {}).items():
            environ_fields[key] = {"string_value": str(value)}

        # Create TypedStruct as an Any message with proper @type field
        from google.protobuf.json_format import MessageToDict

        # Create the inner struct for workflow inputs
        inner_struct = Struct()
        inner_struct.update(
            {
                "args": [],
                "environ": input_dict.get("environ", {}),
                "kwargs": input_dict.get("kwargs", []),
            }
        )

        # Create TypedStruct
        typed_struct = TypedStruct()
        typed_struct.type_url = "type.googleapis.com/michelangelo.UniFlowConf"
        typed_struct.value.CopyFrom(inner_struct)

        # Pack into Any message for proper @type handling
        any_message = Any()
        any_message.Pack(typed_struct)

        # Convert to dict for JSON serialization - this will include @type
        content_dict = MessageToDict(any_message)

        res["spec"]["manifest"]["content"] = content_dict
        _LOG.debug("Added content to spec manifest")

    if uniflow_tar_path:
        res["spec"]["manifest"]["uniflowTar"] = uniflow_tar_path

        # Add workflow function name if available
        if workflow_function_name:
            res["spec"]["manifest"]["uniflowFunction"] = workflow_function_name
        else:
            # Fallback to module path if function name not available
            manifest_data = yaml_dict.get("spec", {}).get("manifest", {})
            uniflow_function = manifest_data.get(
                "uniflowFunction"
            ) or manifest_data.get("filePath")
            if uniflow_function:
                res["spec"]["manifest"]["uniflowFunction"] = uniflow_function

    _LOG.debug("Converted CRD metadata: %r", res)
    return res
