"""EvalReportSink ABC, built-in implementations, and utilities.

Import from this package::

    from michelangelo.workflow.tasks.functions.eval_report_sinks import (
        EvalReportSink,
        LocalFileEvalReportSink,
        APIClientEvalReportSink,
        flatten_report_to_metrics,
    )
"""

from __future__ import annotations

from michelangelo.workflow.tasks.functions.eval_report_sinks.api import (
    APIClientEvalReportSink,
)
from michelangelo.workflow.tasks.functions.eval_report_sinks.base import (
    EvalReportSink,
    flatten_report_to_metrics,
)
from michelangelo.workflow.tasks.functions.eval_report_sinks.local_file import (
    LocalFileEvalReportSink,
)

__all__ = [
    "APIClientEvalReportSink",
    "EvalReportSink",
    "LocalFileEvalReportSink",
    "flatten_report_to_metrics",
]
