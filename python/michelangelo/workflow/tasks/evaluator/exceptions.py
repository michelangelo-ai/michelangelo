"""Exceptions raised by the evaluator workflow task."""

from __future__ import annotations

__all__ = ["SanityCheckFailedError"]


class SanityCheckFailedError(Exception):
    """Raised when one or more configured sanity checks breach their threshold.

    This is a verdict on the *model*, not on the configuration: the evaluation
    ran to completion and the resulting metrics failed a gate the pipeline
    author declared. It is deliberately distinct from ``ConfigurationError``
    (a malformed check, caught before any data is read) so a workflow engine
    can treat it as terminal rather than retryable -- a threshold breach is
    deterministic, and a retry would only burn cluster time.

    Every report is written before this is raised, so an operator debugging a
    breach still has the report that explains it.

    Args:
        message: Human-readable description naming each breached check, its
            actual value, and its threshold.

    Example:
        >>> err = SanityCheckFailedError("test/GLOBAL: auc 0.41 < 0.70")
        >>> str(err)
        'test/GLOBAL: auc 0.41 < 0.70'
    """

    def __init__(self, message: str) -> None:
        """Initialize with a human-readable sanity-check failure message."""
        super().__init__(message)
