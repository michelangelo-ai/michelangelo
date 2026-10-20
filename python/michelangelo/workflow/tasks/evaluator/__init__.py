"""Evaluator workflow task.

Scores one or more datasets with the metrics declared in
:class:`~michelangelo.workflow.schema.evaluator.EvaluatorConfig`, renders an
``EvaluationReport`` per dataset plus a combined one, and gates the run on the
configured sanity checks.

Modules:
    task: :func:`~michelangelo.workflow.tasks.evaluator.task.evaluate`, the
        pipeline entry point.
    preprocessor: :func:`declare_inputs`, for declaring the raw columns a
        pre-evaluation transform reads.
    exceptions: :class:`SanityCheckFailedError`, raised on a threshold breach.
"""

from __future__ import annotations

from michelangelo.workflow.tasks.evaluator.exceptions import SanityCheckFailedError
from michelangelo.workflow.tasks.evaluator.preprocessor import (
    PreprocessorFn,
    declare_inputs,
)
from michelangelo.workflow.tasks.evaluator.task import evaluate

__all__ = [
    "PreprocessorFn",
    "SanityCheckFailedError",
    "declare_inputs",
    "evaluate",
]
