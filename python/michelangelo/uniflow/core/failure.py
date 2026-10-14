"""Failure values and backend extension for workflow failure handlers."""

from dataclasses import dataclass
from typing import Callable


@dataclass(frozen=True)
class WorkflowFailure:
    """Information passed to a workflow's local failure handler."""

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
