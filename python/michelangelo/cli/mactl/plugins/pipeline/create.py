import json
import tempfile
from copy import deepcopy
from logging import getLogger
from os import getenv
from pathlib import Path
from uuid import uuid4

from git import Repo
from google.protobuf.message import Message
from google.protobuf.struct_pb2 import Struct
from grpc import Channel

from mactl import CRD, PWD
from michelangelo.cli.mactl.utils import (
    run_subprocess_registration,
    read_subprocess_outputs
)


_LOG = getLogger(__name__)

# Constants for registration output files
_UNIFLOW_TAR_TB_PATH_FILENAME = "uniflow_tar_tb_path.txt"
_UNIFLOW_INPUT_FILENAME = "uniflow_input.txt"


def get_pipeline_config_and_tar(repo_root: Path, config_file_relative_path: str, 
                               bazel_target: str, project: str, pipeline: str, 
                               yaml_dict: dict = None) -> tuple[Struct, str]:
    """
    Run pipeline registration via subprocess to get uniflow artifacts.
    
    Executes registration in the user's Python environment to obtain:
    1) uniflow tarball path in TerraBlob from "uniflow_tar_tb_path.txt" 
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
        tuple: (workflow_inputs as Struct, uniflow_tar_tb_path as string)
        
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
        
        _LOG.info("Running subprocess registration for project=%s, pipeline=%s", project, pipeline)
        
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
                _LOG.error("Subprocess registration failed with exit code %d", result.returncode)
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
        
        # Read uniflow tar TerraBlob path
        tb_path_file = tmp_path / _UNIFLOW_TAR_TB_PATH_FILENAME
        if not tb_path_file.exists():
            # Fall back to default uniflow tar path file if TB path doesn't exist
            tb_path_file = tmp_path / "uniflow_tar_path.txt"
            
        try:
            uniflow_tar_tb_path = tb_path_file.read_text().strip()
            _LOG.info("Read uniflow tar path: %s", uniflow_tar_tb_path)
        except FileNotFoundError:
            # Use remote path from status file if direct file read fails
            if remote_path:
                uniflow_tar_tb_path = remote_path
                _LOG.info("Using tar path from status file: %s", uniflow_tar_tb_path)
            else:
                raise RuntimeError(f"Could not read uniflow tar path from {tb_path_file}")
        
        # Read uniflow workflow input  
        input_file_path = tmp_path / _UNIFLOW_INPUT_FILENAME
        try:
            content = input_file_path.read_text()
            input_data = json.loads(content)
            _LOG.info("Read uniflow input data")
        except FileNotFoundError:
            raise RuntimeError(f"Could not read uniflow input from {input_file_path}")
        except json.JSONDecodeError as e:
            raise RuntimeError(f"Error parsing uniflow input JSON: {e}")
        
        # Convert to protobuf Struct
        workflow_inputs = Struct()
        workflow_inputs.update(input_data)
        
        return workflow_inputs, uniflow_tar_tb_path


def generate_create(crd: CRD, channel: Channel):
    _LOG.info("Generating `pipeline create` crd for: %s", crd)

    crd.func_crd_metadata_converter = convert_crd_metadata_pipeline_create
    crd.generate_create(channel)


def convert_crd_metadata_pipeline_create(
    yaml_dict: dict, crd_class: type[Message], yaml_path: Path
) -> dict:
    """
    Convert CRD metadata for pipeline create crd.
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
    
    # Run pipeline registration to get uniflow artifacts
    try:
        workflow_inputs, uniflow_tar_tb_path = get_pipeline_config_and_tar(
            repo_root=repo_root,
            config_file_relative_path=config_file_relative_path,
            bazel_target="",  # Not used
            project=project,
            pipeline=pipeline
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
            _LOG.warning("Registration failed, continuing without uniflow artifacts: %s", e)
            workflow_inputs = None
            uniflow_tar_tb_path = ""
    except Exception as e:
        _LOG.error("Unexpected error during registration: %s", e)
        # For unexpected errors, also use graceful degradation
        _LOG.warning("Unexpected registration failure, continuing without uniflow artifacts")
        workflow_inputs = None
        uniflow_tar_tb_path = ""

    res = {"spec": deepcopy(yaml_dict["spec"])}
    res["metadata"] = {
        "clusterName": "",
        "generateName": "",
        "generation": "0",
        "name": pipeline,
        "namespace": project,
        "resourceVersion": "0",
        "uid": str(uuid4()),
    }
    res["spec"]["commit"] = {
        "branch": repo.active_branch.name,
        "git_ref": repo.head.commit.hexsha,
    }
    assert yaml_path.resolve().is_relative_to(repo_root), (
        f"Expected {yaml_path.resolve()} to be relative to {repo_root}"
    )
    
    res["spec"]["manifest"] = {
        "filePath": config_file_relative_path,
        "type": "PIPELINE_MANIFEST_TYPE_YAML",
    }
    res["spec"]["owner"] = {"name": getenv("UBER_LDAP_UID")}
    
    # Add uniflow artifacts if registration succeeded
    # Note: The current protobuf schema doesn't support workflow_inputs or uniflow_tar_path
    # These would need to be added to the manifest or stored differently
    if workflow_inputs is not None:
        _LOG.info("Workflow inputs discovered but not added to spec (schema limitation)")
        
    if uniflow_tar_tb_path:
        _LOG.info("Uniflow tar path: %s (not added to spec due to schema limitation)", uniflow_tar_tb_path)
    
    _LOG.debug("Converted CRD metadata: %r", res)
    return res
