"""TriggerRun `rerun` function plugin module."""

from argparse import ArgumentParser
from inspect import Parameter, Signature
from logging import getLogger
from types import MethodType
from typing import Optional

from google.protobuf.json_format import ParseDict
from google.protobuf.message import Message
from grpc import Channel, RpcError

import michelangelo.cli.mactl.crd as crd_module
from michelangelo.cli.mactl.utils import get_user_name

_LOG = getLogger(__name__)


def add_function_signature(crd: crd_module.CRD) -> None:
    """Add function signature for trigger_run rerun command."""
    crd_module.inject_func_signature(
        crd,
        "rerun",
        {
            "help": (
                "Create a batch rerun TriggerRun that resumes a set of failed "
                "pipeline runs from a point in the pipeline DAG."
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
                        "help": "Namespace of the pipeline runs and new trigger run",
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
                        "help": "Name for the new batch rerun trigger run",
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
                        "help": "Name of the pipeline the failed runs belong to",
                    },
                },
                {
                    "func_signature": Parameter(
                        "pipeline_run",
                        Parameter.POSITIONAL_OR_KEYWORD,
                        default=None,
                    ),
                    "args": ["--pipeline-run"],
                    "kwargs": {
                        "type": str,
                        "action": "append",
                        "required": True,
                        "help": (
                            "Name of a failed pipeline run to rerun. "
                            "Repeatable, one per pipeline run."
                        ),
                    },
                },
                {
                    "func_signature": Parameter(
                        "resume_from",
                        Parameter.POSITIONAL_OR_KEYWORD,
                        default=None,
                    ),
                    "args": ["--resume-from"],
                    "kwargs": {
                        "type": str,
                        "action": "append",
                        "default": None,
                        "help": (
                            "DAG node to resume every rerun from. Repeatable. "
                            "Omit to resume from the point each run failed."
                        ),
                    },
                },
                {
                    "func_signature": Parameter(
                        "resume_up_to",
                        Parameter.POSITIONAL_OR_KEYWORD,
                        default=None,
                    ),
                    "args": ["--resume-up-to"],
                    "kwargs": {
                        "type": str,
                        "action": "append",
                        "default": None,
                        "help": (
                            "DAG node to stop every rerun at, inclusive. Repeatable. "
                            "Omit to run to completion."
                        ),
                    },
                },
                {
                    "func_signature": Parameter(
                        "revision",
                        Parameter.POSITIONAL_OR_KEYWORD,
                        default=None,
                    ),
                    "args": ["--revision"],
                    "kwargs": {
                        "type": str,
                        "default": None,
                        "help": (
                            "Pipeline revision to rerun on. Defaults to the "
                            "pipeline's current definition."
                        ),
                    },
                },
                {
                    "func_signature": Parameter(
                        "dry_run",
                        Parameter.POSITIONAL_OR_KEYWORD,
                        default=False,
                    ),
                    "args": ["--dry-run"],
                    "kwargs": {
                        "dest": "dry_run",
                        "action": "store_true",
                        "default": False,
                        "help": (
                            "Send the request with server-side dry-run "
                            "(k8s.io CreateOptions.dryRun=['All']); server "
                            "validates the batch rerun config without "
                            "creating a TriggerRun."
                        ),
                    },
                },
            ],
        },
    )


def generate_rerun(
    crd: crd_module.CRD, channel: Channel, parser: Optional[ArgumentParser] = None
):
    """Generate rerun function for trigger_run.

    Creates a batch_rerun TriggerRun: a new PipelineRun, resumed from the given
    DAG nodes, is created for each --pipeline-run.
    """
    _LOG.info("Generating `trigger_run rerun` for: %s", crd)

    method_name, input_class, output_class = crd._extract_method_info(
        channel, crd.full_name, "Create"
    )
    crd.configure_parser("rerun", parser)
    func_signature = crd._read_signatures("rerun")

    @crd_module.bind_signature(func_signature)
    def rerun_func(bound_args: Signature) -> Message:
        _LOG.info("Start rerun_func for trigger_run")
        _LOG.info("Bound arguments: %r", bound_args.arguments)
        namespace = crd_module.get_single_arg(bound_args.arguments, "namespace")
        name = crd_module.get_single_arg(bound_args.arguments, "name")
        pipeline_name = crd_module.get_single_arg(bound_args.arguments, "pipeline")
        pipeline_runs = bound_args.arguments.get("pipeline_run") or []
        resume_from = bound_args.arguments.get("resume_from") or []
        resume_up_to = bound_args.arguments.get("resume_up_to") or []
        revision = bound_args.arguments.get("revision")

        trigger_run_dict = {
            "triggerRun": {
                "metadata": {"name": name, "namespace": namespace},
                "spec": {
                    "pipeline": {"name": pipeline_name, "namespace": namespace},
                    "actor": {"name": get_user_name()},
                    "trigger": {
                        "batchRerun": {
                            "pipelineRuns": [
                                {"name": pr, "namespace": namespace}
                                for pr in pipeline_runs
                            ],
                            "resumeFrom": resume_from,
                            "resumeUpTo": resume_up_to,
                        }
                    },
                },
            }
        }
        if revision:
            trigger_run_dict["triggerRun"]["spec"]["revision"] = {
                "name": revision,
                "namespace": namespace,
            }

        request_input = input_class()
        ParseDict(trigger_run_dict, request_input)
        crd_module.apply_dry_run_to_request(
            request_input, "create_options", bound_args.arguments
        )

        _LOG.info(
            "RERUN Request input (%r) ready: %r",
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
                metadata=crd_module.METADATA_STUB,
                timeout=30,
            )
        except RpcError as err:
            _LOG.error("gRPC error creating batch rerun TriggerRun: %s", err)
            raise RuntimeError(
                f"Failed to create batch rerun TriggerRun: {err.details()}"
            ) from err

        _LOG.info("Rerun operation completed (%r): %r", type(response), response)
        return response

    rerun_func.__signature__ = func_signature
    crd.rerun = MethodType(rerun_func, crd)
