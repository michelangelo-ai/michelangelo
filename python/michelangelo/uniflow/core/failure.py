"""Failure values and backend extension for workflow failure handlers."""

from dataclasses import dataclass
from typing import Callable, TypeVar


@dataclass(frozen=True)
class WorkflowFailure:
    """Local equivalent of the remote worker's failure value.

    Locally, ``reason`` and ``details["exception_type"]`` contain the Python
    exception class. Remote workers use backend error codes for ``reason`` and
    include ``exception_type`` only when the error supplies one.
    """

    message: str
    reason: str
    details: dict[str, str]
    backtrace: str


@dataclass(frozen=True)
class FailureEntrypoint:
    """A backend-generated Starlark entrypoint for a handled workflow."""

    main_file: str
    main_function: str
    source: str


FailureLowering = Callable[[str, str, str, str], FailureEntrypoint]

WorkflowFunction = TypeVar("WorkflowFunction", bound=Callable)


def bind_workflow_failure_backend(
    workflow_fn: WorkflowFunction, lowering: FailureLowering
) -> WorkflowFunction:
    """Attach a remote failure backend to a handled workflow.

    Backend integrations use this hook; workflow authors only supply on_failure.
    The decorated function and its Python signature are preserved.
    """
    if getattr(workflow_fn, "_uf_on_failure", None) is None:
        raise ValueError("failure backend requires @workflow(on_failure=...)")
    if not callable(lowering):
        raise TypeError("failure backend must be callable")

    workflow_fn._uf_failure_lowering = lowering
    return workflow_fn
