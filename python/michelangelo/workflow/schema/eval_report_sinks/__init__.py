"""Typed configs and result contracts for EvalReportSink implementations."""

from __future__ import annotations

from michelangelo.workflow.schema.eval_report_sinks.local_file import (
    LocalFileEvalReportSinkConfig,
)
from michelangelo.workflow.schema.eval_report_sinks.result import EvalReportSinkResult

__all__ = [
    "EvalReportSinkResult",
    "LocalFileEvalReportSinkConfig",
]
