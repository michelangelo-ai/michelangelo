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
    """
    Represents the context for running a workflow, either locally or in-cluster.

    Attributes:
        _args: Command-line arguments for the run.
        _target: The mode of the workflow execution. It can be "local-run" or "remote-run"
        environ: Environment variables to set during execution.
    """

    _args: list
    _target: str
    environ: dict = field(default_factory=dict)

    def is_local_run(self):
        return self._target == "local-run"

    def run(self, fn, *args, **kwargs):
        """
        Executes the workflow function in the specified context.

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
    """
    Creates and configures the execution context based on command-line arguments.
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
    """
    Execute a given workflow function in Local Mode. Sets up the necessary environment for running workflows locally
    ensuring local storage and execution.
    """

    # Validate the function's code.
    try:
        build(fn)
    except Exception as err:
        err_message = "Error in building the @workflow function. Ensure it meets all required workflow code specifications."
        raise RuntimeError(err_message) from err

    os.environ["UF_LOCAL_RUN"] = "1"

    # Set local storage path for execution checkpoints.
    os.environ["UF_STORAGE_URL"] = os.path.expanduser("~/uf_storage")

    # Execute the provided workflow function with the specified arguments and keyword arguments.
    fn(*args, **kw)


def _remote_run_argument_parser(environ=False) -> argparse.ArgumentParser:
    """
    Creates an argument parser for the Remote Run Target.

    Args:
        environ: Whether to include --environ option.

    Returns:
        argparse.ArgumentParser: Configured argument parser.
    """

    p = argparse.ArgumentParser()
    p.add_argument(
        "--workflow",
        default=cadence,
        help="The workflow engine to use for remote execution. Options: cadence, temporal. Default is cadence.",
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
):
    """
    Execute a given workflow function in Remote Mode.

    Args:
        fn: The workflow function to be executed remotely.
        environ: Environment variables to be injected for the workflow remote run.
        args: Arguments for the workflow function.
        kwargs: Keyword arguments for the workflow function.
        execution_timeout_seconds: Execution timeout in seconds.
        cron: Cron expression for scheduling periodic workflow runs.
        storage_url: Persistent storage URL for saving and loading workflow checkpoints.
        image: Container image to use for running workflow tasks.
        yes: Automatically answer yes to confirmation prompts.
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
    rr.run()
