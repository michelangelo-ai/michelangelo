import subprocess
import sys
from logging import getLogger
from pathlib import Path
from typing import Any, Dict

import yaml
from grpc import Channel
from inspect import Signature, Parameter
from types import MethodType

from michelangelo.cli.mactl.mactl import CRD, bind_signature, get_single_arg


_LOG = getLogger(__name__)


def validate_pipeline_exists(pipeline_name: str, namespace: str) -> bool:
    """
    Validate that a pipeline exists in the specified namespace using kubectl.

    Args:
        pipeline_name: Name of the pipeline to check
        namespace: Kubernetes namespace to check in

    Returns:
        True if pipeline exists, False otherwise
    """
    try:
        cmd = ["kubectl", "get", "pipeline", pipeline_name, "-n", namespace]
        result = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            check=True
        )
        _LOG.info("Pipeline %s exists in namespace %s", pipeline_name, namespace)
        return True
    except subprocess.CalledProcessError as e:
        _LOG.error("Pipeline %s not found in namespace %s: %s",
                  pipeline_name, namespace, e.stderr)
        return False
    except Exception as e:
        _LOG.error("Error checking pipeline existence: %s", e)
        return False


def parse_trigger_yaml(file_path: str) -> Dict[str, Any]:
    """
    Parse trigger run YAML file to extract pipeline information.

    Args:
        file_path: Path to the trigger run YAML file

    Returns:
        Dictionary containing parsed YAML data

    Raises:
        FileNotFoundError: If file doesn't exist
        yaml.YAMLError: If YAML parsing fails
    """
    path = Path(file_path)
    if not path.exists():
        raise FileNotFoundError(f"Trigger file not found: {file_path}")

    try:
        with path.open('r') as f:
            data = yaml.safe_load(f)
        return data
    except yaml.YAMLError as e:
        _LOG.error("Failed to parse YAML file %s: %s", file_path, e)
        raise


def apply_trigger_run(file_path: str) -> None:
    """
    Apply trigger run configuration using kubectl.

    Args:
        file_path: Path to the trigger run YAML file

    Raises:
        subprocess.CalledProcessError: If kubectl apply fails
    """
    try:
        cmd = ["kubectl", "apply", "-f", file_path]
        result = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            check=True
        )
        _LOG.info("Successfully applied trigger run: %s", result.stdout.strip())
        print(result.stdout.strip())
    except subprocess.CalledProcessError as e:
        _LOG.error("Failed to apply trigger run: %s", e.stderr)
        print(f"Error applying trigger run: {e.stderr}", file=sys.stderr)
        raise


def generate_run(crd: CRD, channel: Channel):
    """
    Generate run function for trigger CRD.
    """
    _LOG.info("Generating `trigger run` crd for: %s", crd)

    run_func_signature = Signature([
        Parameter("self", Parameter.POSITIONAL_OR_KEYWORD),
        Parameter("file", Parameter.POSITIONAL_OR_KEYWORD)
    ])

    @bind_signature(run_func_signature)
    def run_func(bound_args: Signature) -> str:
        _LOG.info("Start run_func for trigger")
        _LOG.info("Bound arguments: %r", bound_args.arguments)

        _self: CRD = bound_args.arguments["self"]
        _file = get_single_arg(bound_args.arguments, "file")

        _LOG.info("Processing trigger run with file: %s", _file)

        try:
            # Parse the trigger YAML file
            trigger_data = parse_trigger_yaml(_file)

            # Extract pipeline information
            pipeline_spec = trigger_data.get("spec", {}).get("pipeline", {})
            pipeline_name = pipeline_spec.get("name")
            pipeline_namespace = pipeline_spec.get("namespace")

            if not pipeline_name or not pipeline_namespace:
                raise ValueError(
                    "Pipeline name and namespace must be specified in trigger YAML. "
                    f"Found name: {pipeline_name}, namespace: {pipeline_namespace}"
                )

            _LOG.info("Validating pipeline %s in namespace %s",
                     pipeline_name, pipeline_namespace)

            # Validate pipeline exists
            if not validate_pipeline_exists(pipeline_name, pipeline_namespace):
                raise RuntimeError(
                    f"Pipeline '{pipeline_name}' not found in namespace '{pipeline_namespace}'. "
                    "Please run 'mactl pipeline apply' first."
                )

            # Apply the trigger run
            apply_trigger_run(_file)

            return f"Successfully created trigger run for pipeline '{pipeline_name}' in namespace '{pipeline_namespace}'"

        except Exception as e:
            _LOG.error("Failed to process trigger run: %s", e)
            print(f"Error: {e}", file=sys.stderr)
            sys.exit(1)

    crd.run = MethodType(run_func, crd)
    _LOG.info("Trigger run function attached to CRD: %s", crd)