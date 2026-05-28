"""EvalReportSink ABC and built-in implementations.

Import from this package::

    from michelangelo.workflow.tasks.functions.eval_report_sinks import (
        EvalReportSink,
        LocalFileEvalReportSink,
        GRPCEvalReportSink,
    )

Note:
    ``GRPCEvalReportSink`` can be imported without ``grpcio`` installed.
    The ``ImportError`` is raised at *construction* time, not import time.
"""

from __future__ import annotations

from michelangelo.workflow.tasks.functions.eval_report_sinks.api import (
    GRPCEvalReportSink,
)
from michelangelo.workflow.tasks.functions.eval_report_sinks.base import EvalReportSink
from michelangelo.workflow.tasks.functions.eval_report_sinks.local_file import (
    LocalFileEvalReportSink,
)

__all__ = ["EvalReportSink", "GRPCEvalReportSink", "LocalFileEvalReportSink"]
