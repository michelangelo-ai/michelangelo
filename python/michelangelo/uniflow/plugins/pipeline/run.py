"""Python implementation of run_pipeline plugin for local execution.

This module provides the same functionality as the Go/Starlark pipeline plugin,
but can be used directly in Python workflows without requiring Cadence/Starlark.
"""

import logging
import random
import string
import time
from datetime import datetime
from typing import Any, Dict, List, Optional

import grpc
from google.protobuf import struct_pb2
from google.protobuf.struct_pb2 import Struct, Value

from michelangelo.api.v2 import APIClient
from michelangelo.gen.api.options_pb2 import ResourceIdentifier
from michelangelo.gen.api.v2.pipeline_run_pb2 import (
    PipelineRun,
    PipelineRunSpec,
    PipelineRunState,
)
from michelangelo.gen.api.v2.user_pb2 import UserInfo
from michelangelo.gen.k8s.io.apimachinery.pkg.apis.meta.v1.generated_pb2 import (
    CreateOptions,
    GetOptions,
    ObjectMeta,
)
from michelangelo.uniflow.core import star_plugin

log = logging.getLogger(__name__)

# Default timeout: 10 years (matching CadenceLongTimeout)
_DEFAULT_TIMEOUT_SECONDS = 10 * 365 * 24 * 60 * 60
_DEFAULT_POLL_SECONDS = 10


def _generate_pipeline_run_name() -> str:
    """Generate a unique pipeline run name with timestamp and random suffix.

    Returns:
        A unique pipeline run name in format: run-YYYYMMDD-HHMMSS-<random>
    """
    now = datetime.now()
    timestamp = now.strftime("%Y%m%d-%H%M%S")

    # Generate 4 random bytes as hex string (8 characters)
    random_suffix = "".join(random.choices(string.hexdigits.lower(), k=8))

    return f"run-{timestamp}-{random_suffix}"


def _python_to_value(obj: Any) -> Value:
    """Convert a Python object to a protobuf Value.

    Args:
        obj: Python object to convert (str, int, float, bool, dict, list, etc.).

    Returns:
        A protobuf Value representation of the object.
    """
    val = Value()
    if isinstance(obj, str):
        val.string_value = obj
    elif isinstance(obj, (int, float)):
        val.number_value = float(obj)
    elif isinstance(obj, bool):
        val.bool_value = obj
    elif isinstance(obj, dict):
        struct_val = Struct()
        struct_val.update(obj)
        val.struct_value.CopyFrom(struct_val)
    elif isinstance(obj, list):
        list_vals = [_python_to_value(item) for item in obj]
        val.list_value.CopyFrom(struct_pb2.ListValue(values=list_vals))
    else:
        # For other types, convert to string
        val.string_value = str(obj)
    return val


def _build_input_struct(
    environ: Optional[Dict[str, str]] = None,
    args: Optional[List[Any]] = None,
    kwargs: Optional[Dict[str, Any]] = None,
    input_data: Optional[Dict[str, Any]] = None,
) -> Optional[Struct]:
    """Build a protobuf Struct for pipeline input based on provided parameters.

    This matches the structure expected by the Go implementation:
    - For Uniflow pipelines: builds struct with environ, args, kwargs
    - For non-Uniflow pipelines: uses input_data directly

    Args:
        environ: Optional dictionary of environment variables.
        args: Optional list of pipeline-specific arguments.
        kwargs: Optional dictionary of pipeline-specific keyword configurations.
        input_data: Optional input parameters for non-Uniflow pipelines.

    Returns:
        A protobuf Struct representing the input, or None if no input provided.

    Raises:
        ValueError: If input_data is provided together with environ/args/kwargs.
    """
    # Check mutual exclusivity
    has_input_data = input_data is not None
    has_uniflow_params = environ is not None or args is not None or kwargs is not None

    if has_input_data and has_uniflow_params:
        raise ValueError(
            "input_data cannot be used together with environ, args, or kwargs"
        )

    if has_input_data:
        # For non-Uniflow pipelines: use input_data directly
        # Struct.update() handles dict conversion automatically
        pb_struct = Struct()
        pb_struct.update(input_data)
        return pb_struct

    if has_uniflow_params:
        # For Uniflow pipelines: build struct with environ, args, kwargs
        pb_struct = Struct()

        # Process environ (map[string]string)
        if environ is not None:
            env_struct = Struct()
            for k, v in environ.items():
                env_struct[k] = Value(string_value=str(v))
            pb_struct["environ"] = Value(struct_value=env_struct)

        # Process args (list of Struct)
        if args is not None:
            arg_list = []
            for arg in args:
                if isinstance(arg, dict):
                    arg_struct = Struct()
                    arg_struct.update(arg)
                else:
                    # Wrap primitive values in a struct with "value" field
                    arg_struct = Struct()
                    arg_struct["value"] = _python_to_value(arg)
                arg_list.append(Value(struct_value=arg_struct))
            pb_struct["args"] = Value(list_value=struct_pb2.ListValue(values=arg_list))

        # Process kwargs (dict -> sorted list of [key, value] pairs)
        if kwargs is not None:
            # Sort keys for deterministic behavior (matching Go implementation)
            sorted_keys = sorted(kwargs.keys())
            kwarg_list = []
            for key in sorted_keys:
                val = kwargs[key]
                val_proto = _python_to_value(val)
                kwarg_list.append(
                    Value(
                        list_value=struct_pb2.ListValue(
                            values=[
                                Value(string_value=key),
                                val_proto,
                            ]
                        )
                    )
                )
            pb_struct["kwargs"] = Value(list_value=struct_pb2.ListValue(values=kwarg_list))

        return pb_struct if pb_struct else None

    return None


def create_pipeline_run(
    namespace: str,
    pipeline_name: str,
    pipeline_revision: Optional[str] = None,
    environ: Optional[Dict[str, str]] = None,
    args: Optional[List[Any]] = None,
    kwargs: Optional[Dict[str, Any]] = None,
    input_data: Optional[Dict[str, Any]] = None,
    actor: Optional[str] = None,
) -> PipelineRun:
    """
    Creates a pipeline run with the specified parameters.
    Intended for internal use only.

    Args:
        namespace: str: Namespace where the pipeline will run.
        pipeline_name: str: Name of the pipeline to execute.
        pipeline_revision (Optional): str: Git sha of the specific revision of the pipeline to run.
            If not provided, the latest revision will be used. Only the first 12 characters are used.
        environ (Optional): dict: Dictionary containing environment variables (default: None).
        args (Optional): list: List containing arguments (default: None).
        kwargs (Optional): dict: Dictionary containing keyword arguments (default: None).
        input_data (Optional): dict: Direct input dictionary for orchestration pipelines (default: None).
            Cannot be used together with environ, args, or kwargs.
        actor (Optional): str: Name of the actor creating the pipeline run (default: None).

    Returns:
        PipelineRun: Pipeline run object, which is intended to be used with the poll_pipeline_run method.

    Raises:
        ValueError: If input_data is used together with environ/args/kwargs.
    """
    # Input validation: ensure we do not have both input_data and (environ/args/kwargs)
    if input_data is not None and (environ is not None or args is not None or kwargs is not None):
        raise ValueError(
            "cannot use 'input_data' together with 'environ', 'args', or 'kwargs'; "
            "'input_data' is for orchestration pipelines, while 'environ'/'args'/'kwargs' are for Uniflow pipelines"
        )

    # Generate pipeline run name
    name = _generate_pipeline_run_name()

    # Format revision if provided
    revision = None
    if pipeline_revision:
        sha_prefix = pipeline_revision[:12] if len(pipeline_revision) > 12 else pipeline_revision
        formatted = f"pipeline-{pipeline_name}-{sha_prefix}"
        revision = ResourceIdentifier(namespace=namespace, name=formatted)

    # Build input Struct based on provided parameters
    input_struct = _build_input_struct(
        environ=environ, args=args, kwargs=kwargs, input_data=input_data
    )

    # Build PipelineRun object
    pipeline_run = PipelineRun()
    pipeline_run.metadata.CopyFrom(ObjectMeta(name=name, namespace=namespace))
    pipeline_run.spec.CopyFrom(
        PipelineRunSpec(
            pipeline=ResourceIdentifier(namespace=namespace, name=pipeline_name)
        )
    )

    # Set revision if provided
    if revision:
        pipeline_run.spec.revision.CopyFrom(revision)

    # Set actor if provided
    if actor:
        pipeline_run.spec.actor.CopyFrom(UserInfo(name=actor))

    # Set input Struct if provided
    if input_struct:
        pipeline_run.spec.input.CopyFrom(input_struct)

    # Create pipeline run via API
    log.info(f"Creating pipeline run {name} for pipeline {pipeline_name} in namespace {namespace}")
    created_run = APIClient.PipelineRunService.create_pipeline_run(
        pipeline_run=pipeline_run, create_options=CreateOptions()
    )

    return created_run


def poll_pipeline_run(
    namespace: str,
    name: str,
    timeout_seconds: int = _DEFAULT_TIMEOUT_SECONDS,
    poll_seconds: int = _DEFAULT_POLL_SECONDS,
) -> Dict[str, Any]:
    """
    Polls a created pipeline run until it reaches a terminal state or times out.
    Intended for internal use only.

    Args:
        namespace: str: Namespace where the pipeline will run.
        name: str: Name of the pipeline run to poll.
        timeout_seconds: int: Maximum time to wait for pipeline completion in seconds
            (default: _DEFAULT_TIMEOUT_SECONDS = 10 years).
        poll_seconds: int: Interval between status checks in seconds (default: 10).

    Returns:
        dict: Pipeline run result containing status and metadata in the following format:
        {
            "metadata": {
                "name": str,
                "namespace": str,
            },
            "status": {
                "state": str,  # e.g., "PIPELINE_RUN_STATE_SUCCEEDED"
            }
        }

    Raises:
        RuntimeError: If pipeline run fails or is killed, or if pipeline run not found.
        TimeoutError: If pipeline run exceeds the specified timeout duration.
    """
    successful_terminal_states = [
        PipelineRunState.PIPELINE_RUN_STATE_SUCCEEDED,
        PipelineRunState.PIPELINE_RUN_STATE_SKIPPED,
    ]

    failed_terminal_states = [
        PipelineRunState.PIPELINE_RUN_STATE_FAILED,
        PipelineRunState.PIPELINE_RUN_STATE_KILLED,
    ]

    log.info(
        f"Monitoring pipeline run {name} in namespace {namespace} "
        f"(timeout={timeout_seconds}s, poll_interval={poll_seconds}s)"
    )

    start_time = time.time()
    while time.time() - start_time < timeout_seconds:
        try:
            current_run = APIClient.PipelineRunService.get_pipeline_run(
                namespace=namespace, name=name, get_options=GetOptions()
            )

            state = current_run.status.state

            # Get state value and name (handle both int and enum)
            if isinstance(state, int):
                state_value = state
                state_name = PipelineRunState.Name(state)
            else:
                state_value = state.value if hasattr(state, 'value') else int(state)
                state_name = state.name if hasattr(state, 'name') else str(state)

            # Check for successful terminal states
            if state_value in successful_terminal_states:
                log.info(f"Pipeline run {name} completed with state: {state_name}")
                # Convert final state to string name
                final_state = current_run.status.state
                if isinstance(final_state, int):
                    final_state_name = PipelineRunState.Name(final_state)
                else:
                    final_state_name = (
                        final_state.name if hasattr(final_state, 'name') else str(final_state)
                    )
                return {
                    "metadata": {
                        "name": current_run.metadata.name,
                        "namespace": current_run.metadata.namespace,
                    },
                    "status": {
                        "state": final_state_name,
                    },
                }

            # Check for failed terminal states
            elif state_value in failed_terminal_states:
                state_name = PipelineRunState.Name(state_value) if isinstance(state, int) else state_name
                error_msg = f"Pipeline run {name} failed with status {state_name}"
                if current_run.status.error_message:
                    error_msg += f": {current_run.status.error_message}"
                raise RuntimeError(error_msg)

        except grpc.RpcError as e:
            # Handle gRPC-specific errors
            if e.code() == grpc.StatusCode.NOT_FOUND:
                raise RuntimeError(
                    f"Pipeline run {name} not found in namespace {namespace}"
                ) from e
            elif e.code() in (
                grpc.StatusCode.UNAVAILABLE,
                grpc.StatusCode.DEADLINE_EXCEEDED,
                grpc.StatusCode.RESOURCE_EXHAUSTED,
            ):
                # Transient errors - continue polling
                log.debug(f"Transient error polling pipeline run {name}: {e.details()}")
            elif e.code() in (
                grpc.StatusCode.PERMISSION_DENIED,
                grpc.StatusCode.UNAUTHENTICATED,
                grpc.StatusCode.INVALID_ARGUMENT,
            ):
                raise RuntimeError(
                    f"Failed to get pipeline run {name}: {e.details()}"
                ) from e
            else:
                # Other gRPC errors - continue polling
                log.debug(f"Error polling pipeline run {name}: {e.details()}")
        except RuntimeError:
            # Re-raise RuntimeError (from failed state check)
            raise
        except Exception as e:
            # For other exceptions, log and continue polling (network issues, etc.)
            log.debug(f"Error polling pipeline run {name}: {e}")

        time.sleep(poll_seconds)

    raise TimeoutError(f"Pipeline run {name} timed out after {timeout_seconds} seconds")


@star_plugin("pipeline.run_pipeline")
def run_pipeline(
    namespace: str,
    pipeline_name: str,
    pipeline_revision: Optional[str] = None,
    environ: Optional[Dict[str, str]] = None,
    args: Optional[List[Any]] = None,
    kwargs: Optional[Dict[str, Any]] = None,
    timeout_seconds: int = 0,  # 0 means use _DEFAULT_TIMEOUT_SECONDS
    poll_seconds: int = 10,  # Use literal to avoid transpilation issues
    input_data: Optional[Dict[str, Any]] = None,
    actor: Optional[str] = None,
) -> Dict[str, Any]:
    """Create and wait for a child pipeline run to complete synchronously.

    This function matches the Go/Starlark implementation exactly in terms of
    parameters, return types, and behavior. It combines pipeline run creation
    and monitoring into a single synchronous operation.

    Args:
        namespace: Namespace where the pipeline run will be created (required).
        pipeline_name: Name of the pipeline to run (required).
        pipeline_revision: Optional git SHA specifying a particular pipeline version
            for reproducible runs (formatted as pipeline-{name}-{sha[0:12]}).
        environ: Optional dictionary of environment variables (map[string]string),
            typically used for resource configuration (e.g., Spark CPU/memory).
        args: Optional list of pipeline-specific arguments.
        kwargs: Optional dictionary of pipeline-specific keyword configurations
            (most common way to pass input parameters).
        timeout_seconds: Maximum time in seconds to wait for completion
            (default: 0 = uses default timeout of 10 years).
        poll_seconds: Polling interval in seconds (default: 10).
        input_data: Optional input parameters for non-Uniflow pipelines.
            Designed for non-Uniflow pipelines. Users should use either input_data or
            (environ/args/kwargs), but not both at the same time.
        actor: Optional name of the actor creating the pipeline run (default: None).

    Returns:
        Dictionary with pipeline run details:
        {
            "metadata": {
                "name": str,
                "namespace": str
            },
            "status": {
                "state": str  # e.g., "PIPELINE_RUN_STATE_SUCCEEDED"
            }
        }

    Raises:
        ValueError: If input_data is provided together with environ/args/kwargs,
            or if required parameters are missing.
        RuntimeError: If the pipeline run fails or is killed.
        TimeoutError: If the pipeline run exceeds timeout_seconds.
    """
    # Set default timeout if not provided
    if timeout_seconds == 0:
        timeout_seconds = _DEFAULT_TIMEOUT_SECONDS

    try:
        # Create the pipeline run
        pipeline_run = create_pipeline_run(
            namespace=namespace,
            pipeline_name=pipeline_name,
            pipeline_revision=pipeline_revision,
            environ=environ,
            args=args,
            kwargs=kwargs,
            input_data=input_data,
            actor=actor,
        )

        pipeline_run_name = pipeline_run.metadata.name

        # Poll until completion
        completed_run = poll_pipeline_run(
            namespace=namespace,
            name=pipeline_run_name,
            timeout_seconds=timeout_seconds,
            poll_seconds=poll_seconds,
        )

        return completed_run

    except Exception as e:
        # Re-raise known exceptions as-is
        if isinstance(e, (ValueError, RuntimeError, TimeoutError)):
            raise
        # Wrap unexpected exceptions
        raise RuntimeError(f"Pipeline run failed: {e!s}") from e


