"""Failure values and backend extension for workflow failure handlers."""

from dataclasses import dataclass
from typing import Callable


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
