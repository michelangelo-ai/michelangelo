r"""Workflow execution context for local and remote runs.

This module provides the Context class and create_context() function for managing
workflow execution environments. It handles both local execution (for development
and testing) and remote execution (for production deployments on Cadence/Temporal).

The context system provides:

- Unified interface for local and remote workflow execution
- Environment variable management
- Command-line argument parsing
- Workflow validation and packaging
- Integration with Cadence and Temporal workflow engines

Example:
    Local workflow execution::

        from michelangelo.uniflow.core.context import create_context
        from michelangelo.uniflow.core.decorator import workflow

        @workflow()
        def my_workflow():
            return "Hello, World!"

        if __name__ == "__main__":
            ctx = create_context()
            ctx.run(my_workflow)

    Remote workflow execution::

        # Command line:
        # python my_workflow.py remote-run \\
        #     --storage-url s3://bucket/storage \\
        #     --image my-image:latest

        ctx = create_context()  # Automatically detects remote-run mode
        ctx.run(my_workflow)
"""

import argparse
import logging
import os
import sys
from dataclasses import dataclass, field
from typing import Callable, Optional

from michelangelo.uniflow.core.build import build
from michelangelo.uniflow.core.remote_run import (
    DEFAULT_EXECUTION_TIMEOUT_SECONDS,
    RemoteRun,
    RemoteRunTemporal,
)
from michelangelo.uniflow.core.utils import LOGGING_FORMAT, ArgparseEnvironAction

log = logging.getLogger(__name__)
cadence = "cadence"
temporal = "temporal"


@dataclass(frozen=True)
class Context:
    """Represents the context for running a workflow, either locally or in-cluster.

    Attributes:
        _args: Command-line arguments for the run.
        _target: The mode of the workflow execution. It can be "local-run" or
            "remote-run".
        environ: Environment variables to set during execution.
    """

    _args: list
    _target: str
    environ: dict = field(default_factory=dict)

    def is_local_run(self):
        """Check if the context is configured for local execution.

        Returns:
            True if running in local mode, False for remote execution.
        """
        return self._target == "local-run"

    def run(self, fn, *args, **kwargs):
        """Executes the workflow function in the specified context.

        Args:
            fn: The workflow function to execute.
            *args: Positional arguments to pass to the function.
            **kwargs: Keyword arguments to pass to the function.
        """
        os.environ.update(self.environ)

        if self._target == "local-run":
            p = argparse.ArgumentParser()
            p.add_argument(
                "--environ",
                "--env",
                action=ArgparseEnvironAction,
                nargs="*",
                default={},
            )
            ns = p.parse_args(self._args)

            os.environ.update(ns.environ)
            _local_run(fn, *args, **kwargs)
            return

        if self._target in ("remote-run", "cluster-run"):
            p = _remote_run_argument_parser(environ=True)
            ns = p.parse_args(self._args).__dict__
            _remote_run(
                environ={**self.environ, **ns.pop("environ")},
                fn=fn,
                args=args,
                kwargs=kwargs,
                **ns,
            )
            return
        raise ValueError(f"Unsupported target: {self._target}")


def create_context() -> Context:
    """Create and configure the execution context based on command-line arguments.

    Parses sys.argv to determine execution mode (local-run or remote-run) and
    constructs an appropriate Context instance. If no mode is specified, defaults
    to local-run.

    Returns:
        A Context instance configured for the requested execution mode.

    Raises:
        AssertionError: If an unsupported execution target is specified.

    Example:
        Creating context for local execution::

            # python my_workflow.py
            # or: python my_workflow.py local-run
            ctx = create_context()
            assert ctx.is_local_run()

        Creating context for remote execution::

            # python my_workflow.py remote-run --storage-url s3://... --image ...
            ctx = create_context()
            assert not ctx.is_local_run()
    """
    logging.basicConfig(level=logging.INFO, format=LOGGING_FORMAT, force=True)

    args = sys.argv[1:]
    if not args or args[0].startswith("-"):
        args = ["local-run", *args]

    target, args = args[0], args[1:]

    assert target in ["local-run", "remote-run"], (
        f"Unsupported target: {target}; args: {sys.argv[1:]}"
    )

    ctx = Context(_args=args, _target=target)
    log.info("ctx: %r", ctx)
    return ctx


def _local_run(fn: Callable, *args, **kw):
    """Execute a workflow function in Local Mode.

    Sets up the necessary environment for running workflows locally,
    ensuring local storage and execution. Validates the workflow code
    before execution.

    Args:
        fn: The workflow function to execute.
        *args: Positional arguments to pass to the workflow function.
        **kw: Keyword arguments to pass to the workflow function.

    Raises:
        RuntimeError: If the workflow function fails validation.
    """
    # Validate the function's code.
    try:
        build(fn)
    except Exception as err:
        err_message = (
            "Error in building the @workflow function. "
            "Ensure it meets all required workflow code specifications."
        )
        raise RuntimeError(err_message) from err

    os.environ["UF_LOCAL_RUN"] = "1"

    # Set local storage path for execution checkpoints.
    os.environ["UF_STORAGE_URL"] = os.path.expanduser("~/uf_storage")

    # Execute the provided workflow function with the specified arguments
    # and keyword arguments.
    fn(*args, **kw)


def _remote_run_argument_parser(environ=False) -> argparse.ArgumentParser:
    """Creates an argument parser for the Remote Run Target.

    Args:
        environ: Whether to include --environ option.

    Returns:
        argparse.ArgumentParser: Configured argument parser.
    """
    p = argparse.ArgumentParser()
    p.add_argument(
        "--workflow",
        default=cadence,
        help=(
            "The workflow engine to use for remote execution. "
            "Options: cadence, temporal. Default is cadence."
        ),
    )
    p.add_argument(
        "--storage-url",
        required=True,
        help="Persistent storage URL for saving and loading workflow checkpoints.",
    )
    p.add_argument(
        "--image",
        required=True,
        help="Container image to use for running workflow tasks.",
    )
    p.add_argument(
        "--execution-timeout-seconds",
        default=DEFAULT_EXECUTION_TIMEOUT_SECONDS,
        type=int,
    )
    p.add_argument(
        "--cron", help="Cron expression for scheduling periodic workflow runs."
    )
    p.add_argument(
        "--yes",
        action="store_true",
        help="Automatically answer yes to confirmation prompts.",
    )
    p.add_argument(
        "--file-sync",
        action="store_true",
        help=(
            "Sync local code changes from the current git repository to the remote run."
        ),
    )

    if environ:
        p.add_argument(
            "--environ", "--env", action=ArgparseEnvironAction, nargs="*", default={}
        )

    return p


def _remote_run(
    *,
    fn: Callable,
    environ: Optional[dict] = None,
    args: Optional[tuple] = None,
    kwargs: Optional[dict] = None,
    execution_timeout_seconds: int = DEFAULT_EXECUTION_TIMEOUT_SECONDS,
    cron: Optional[str] = None,
    storage_url: str = "",
    image: str = "",
    yes: bool = False,
    workflow: str = cadence,
    file_sync: bool = False,
):
    """Execute a workflow function in Remote Mode.

    Packages and submits the workflow for remote execution on Cadence or Temporal
    workflow engines.

    Args:
        fn: The workflow function to be executed remotely.
        environ: Environment variables to be injected for the workflow remote run.
        args: Arguments for the workflow function.
        kwargs: Keyword arguments for the workflow function.
        execution_timeout_seconds: Execution timeout in seconds.
        cron: Cron expression for scheduling periodic workflow runs.
        storage_url: Persistent storage URL for saving and loading workflow
            checkpoints.
        image: Container image to use for running workflow tasks.
        yes: Automatically answer yes to confirmation prompts.
        workflow: Workflow engine to use ("cadence" or "temporal"). Defaults to
            "cadence".
        file_sync: Sync local code changes to the remote run.
    """
    assert storage_url
    assert image

    environ = environ or {}
    args = args or ()
    kwargs = kwargs or {}

    assert isinstance(environ, dict)

    if workflow == cadence:
        rr = RemoteRun(
            fn=fn,
            image=image,
            storage_url=storage_url,
        )
    elif workflow == temporal:
        rr = RemoteRunTemporal(
            fn=fn,
            image=image,
            storage_url=storage_url,
        )
    rr.environ = environ
    rr.args = args
    rr.kwargs = kwargs
    rr.execution_timeout_seconds = execution_timeout_seconds
    rr.cron = cron
    rr.yes = yes
    rr.file_sync = file_sync
    rr.run()
