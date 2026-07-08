"""Pipeline `run` function plugin module."""

import uuid
from argparse import ArgumentParser
from datetime import datetime, timezone
from inspect import Parameter, Signature
from logging import getLogger
from types import MethodType
from typing import Optional

from google.protobuf.json_format import ParseDict
from google.protobuf.message import Message
from grpc import Channel

import michelangelo.cli.mactl.crd as _crd_module
from michelangelo.cli.mactl.crd import (
    CRD,
    bind_signature,
    get_single_arg,
    inject_func_signature,
)
from michelangelo.cli.mactl.grpc_tools import (
    get_message_class_by_name,
    get_methods_from_service,
    get_service_name,
)
from michelangelo.cli.mactl.utils import get_user_name

_LOG = getLogger(__name__)

# TODO(#938): Add E2E tests for pipeline run command with representative scenarios
# (normal run, resume from checkpoint)


def add_function_signature(crd: CRD) -> None:
    """Add function signature for pipeline run command."""
    inject_func_signature(
        crd,
        "run",
        {
            "help": (
                "Run a registered pipeline. (requires pipeline to be created first)"
            ),
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
                        "help": (
                            "Resume from a previous pipeline run. Format: "
                            "'pipeline_run_name[:step_name]'"
                        ),
                    },
                },
                {
                    "func_signature": Parameter(
                        "notify_slack",
                        Parameter.POSITIONAL_OR_KEYWORD,
                        default=None,
                    ),
                    "args": ["--notify-slack"],
                    "kwargs": {
                        "type": str,
                        "action": "append",
                        "default": None,
                        "help": (
                            "Slack destination (channel or @user) for run "
                            "notifications. Repeatable or comma-separated."
                        ),
                    },
                },
                {
                    "func_signature": Parameter(
                        "notify_email",
                        Parameter.POSITIONAL_OR_KEYWORD,
                        default=None,
                    ),
                    "args": ["--notify-email"],
                    "kwargs": {
                        "type": str,
                        "action": "append",
                        "default": None,
                        "help": (
                            "Email address for run notifications. "
                            "Repeatable or comma-separated."
                        ),
                    },
                },
                {
                    "func_signature": Parameter(
                        "notify_on",
                        Parameter.POSITIONAL_OR_KEYWORD,
                        default=None,
                    ),
                    "args": ["--notify-on"],
                    "kwargs": {
                        "type": str,
                        "action": "append",
                        "default": None,
                        "help": (
                            "Event type to notify on: SUCCEEDED, FAILED, "
                            "KILLED, SKIPPED, STARTED. Repeatable or comma-separated. "
                            "Applies to all destinations. Default: SUCCEEDED, FAILED, "
                            "KILLED, SKIPPED (terminal states)"
                        ),
                    },
                },
            ],
        },
    )


def generate_run(crd: CRD, channel: Channel, parser: Optional[ArgumentParser] = None):
    """Generate run function for pipeline CRD."""
    _LOG.info("Generating `pipeline run` crd for: %s", crd)
    pipeline_run_service = get_service_name(
        channel,
        crd.metadata,
        "PipelineRunService",
        fallback="michelangelo.api.v2.PipelineRunService",
    )
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

        # Handle optional parameters
        _resume_from = bound_args.arguments.get("resume_from")
        _notify_slack = bound_args.arguments.get("notify_slack")
        _notify_email = bound_args.arguments.get("notify_email")
        _notify_on = bound_args.arguments.get("notify_on")

        run_kwargs = {
            "namespace": _namespace,
            "name": _name,
            "resume_from": _resume_from,
            "notify_slack": _notify_slack,
            "notify_email": _notify_email,
            "notify_on": _notify_on,
        }

        pipeline_run_dict = _self.func_crd_metadata_converter(
            run_kwargs, input_class, None
        )

        request_input = input_class()
        ParseDict(pipeline_run_dict, request_input)

        # Use auto-detected service name
        method_fullname = f"/{pipeline_run_service}/{method_name}"
        _LOG.info("Method fullname for gRPC call: %s", method_fullname)

        stub_method = channel.unary_unary(
            method_fullname,
            request_serializer=input_class.SerializeToString,
            response_deserializer=output_class.FromString,
        )
        response = stub_method(
            request_input,
            metadata=_crd_module.METADATA_STUB,
            timeout=30,
        )
        _LOG.info("Stub method completed (%r): %r", type(response), response)
        print(response)
        return response

    run_func.__signature__ = func_signature  # type: ignore[attr-defined]
    crd.run = MethodType(run_func, crd)


def convert_crd_metadata_pipeline_run(
    yaml_dict: dict, crd_class: type, yaml_path
) -> dict:
    """Convert CRD metadata for pipeline run command.

    This function generates a CreatePipelineRunRequest object from command line
    arguments.
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
    notify_slack = yaml_dict.get("notify_slack")
    notify_email = yaml_dict.get("notify_email")
    notify_on = yaml_dict.get("notify_on")
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
        notify_slack=notify_slack,
        notify_email=notify_email,
        notify_on=notify_on,
    )

    return {"pipeline_run": pipeline_run}


def generate_pipeline_run_object(
    run_name: str,
    pipeline_name: str,
    namespace: str,
    resume_from: Optional[str] = None,
    notify_slack: Optional[list[str]] = None,
    notify_email: Optional[list[str]] = None,
    notify_on: Optional[list[str]] = None,
) -> dict:
    """Generate PipelineRun object as dictionary.

    Args:
        run_name: Generated unique name for the pipeline run
        pipeline_name: Name of the target pipeline to run
        namespace: Kubernetes namespace
        resume_from: Optional resume specification in format
            "pipeline_run_name:step_name"
        notify_slack: Slack destinations for notifications
        notify_email: Email addresses for notifications
        notify_on: Event types to notify on (defaults to all)

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
                "name": get_user_name(),
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

    # Add notifications if --slack or --email provided
    notifications = _build_notifications(
        notify_slack=notify_slack,
        notify_email=notify_email,
        notify_on=notify_on,
    )
    if notifications:
        pipeline_run_dict["spec"]["notifications"] = notifications

    _LOG.info("Generated pipeline run object: %s", run_name)
    return pipeline_run_dict


_NOTIFY_ON_MAP = {
    "SUCCEEDED": "EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED",
    "FAILED": "EVENT_TYPE_PIPELINE_RUN_STATE_FAILED",
    "KILLED": "EVENT_TYPE_PIPELINE_RUN_STATE_KILLED",
    "SKIPPED": "EVENT_TYPE_PIPELINE_RUN_STATE_SKIPPED",
    "STARTED": "EVENT_TYPE_PIPELINE_RUN_STATE_STARTED",
}

_DEFAULT_NOTIFY_ON = [
    "EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED",
    "EVENT_TYPE_PIPELINE_RUN_STATE_FAILED",
    "EVENT_TYPE_PIPELINE_RUN_STATE_KILLED",
    "EVENT_TYPE_PIPELINE_RUN_STATE_SKIPPED",
]


def _split_csv(values: Optional[list[str]]) -> list[str]:
    """Flatten comma-separated values into a single list, stripping whitespace."""
    if not values:
        return []
    return [item.strip() for raw in values for item in raw.split(",") if item.strip()]


def _build_notifications(
    notify_slack: Optional[list[str]] = None,
    notify_email: Optional[list[str]] = None,
    notify_on: Optional[list[str]] = None,
) -> list[dict]:
    """Build notification entries from notification flags."""
    slack_destinations = _split_csv(notify_slack)
    email_addresses = _split_csv(notify_email)
    event_keys = _split_csv(notify_on)

    if not slack_destinations and not email_addresses:
        if event_keys:
            _LOG.warning(
                "--notify-on specified without --notify-slack or --notify-email; "
                "no notifications will be sent"
            )
        return []

    if event_keys:
        invalid = [e for e in event_keys if e not in _NOTIFY_ON_MAP]
        if invalid:
            raise ValueError(
                f"Invalid --notify-on values: {invalid}. "
                f"Valid choices: {list(_NOTIFY_ON_MAP)}"
            )
        event_types = [_NOTIFY_ON_MAP[e] for e in event_keys]
    else:
        event_types = _DEFAULT_NOTIFY_ON

    notifications: list[dict] = []
    if slack_destinations:
        notifications.append(
            {
                "notificationType": "NOTIFICATION_TYPE_SLACK",
                "eventTypes": event_types,
                "resourceType": "RESOURCE_TYPE_PIPELINE_RUN",
                "slackDestinations": slack_destinations,
            }
        )
    if email_addresses:
        notifications.append(
            {
                "notificationType": "NOTIFICATION_TYPE_EMAIL",
                "eventTypes": event_types,
                "resourceType": "RESOURCE_TYPE_PIPELINE_RUN",
                "emails": email_addresses,
            }
        )
    return notifications


def parse_resume_from(resume_from: str, namespace: str) -> dict:
    """Parse the resume_from parameter and return a resume spec.

    Args:
        resume_from: Resume specification in format "pipeline_run_name" or
            "pipeline_run_name:step_name"
        namespace: Kubernetes namespace for the pipeline run reference

    Returns:
        dict: Resume spec dictionary matching the Resume proto message
    """
    if not resume_from:
        _LOG.error(
            "Invalid resume_from format. Expected 'pipeline_run_name' or "
            "'pipeline_run_name:step_name', got: %r",
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
    """Generates a pipeline-run name.

    Format: run-YYYYMMDD-HHMMSS-{uuid8}  (always 28 characters)
    Example: run-20260402-143022-a3f7c2b1
    """
    now = datetime.now(timezone.utc)
    uuid8 = str(uuid.uuid4())[:8]
    return f"run-{now.strftime('%Y%m%d-%H%M%S')}-{uuid8}"
