"""EvalReportSink ABC and built-in implementations.

Import from submodules::

    from michelangelo.workflow.tasks.functions.eval_report_sinks import (
        EvalReportSink,
        LocalFileEvalReportSink,
        APISink,
    )
"""

from __future__ import annotations

from michelangelo.workflow.tasks.functions.eval_report_sinks.api import APISink
from michelangelo.workflow.tasks.functions.eval_report_sinks.base import EvalReportSink
from michelangelo.workflow.tasks.functions.eval_report_sinks.local_file import (
    LocalFileEvalReportSink,
)

__all__ = ["APISink", "EvalReportSink", "LocalFileEvalReportSink"]
