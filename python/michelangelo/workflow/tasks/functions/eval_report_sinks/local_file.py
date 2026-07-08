"""LocalFileEvalReportSink — writes an EvaluationReport to a local JSON file."""

from __future__ import annotations

import json
import logging
import os
import tempfile
from typing import TYPE_CHECKING, Any

from google.protobuf.json_format import MessageToDict

from michelangelo.workflow.schema.eval_report_sinks.result import EvalReportSinkResult
from michelangelo.workflow.tasks.functions.eval_report_sinks.base import EvalReportSink

if TYPE_CHECKING:
    from michelangelo.gen.api.v2.evaluation_report_pb2 import EvaluationReport
    from michelangelo.workflow.schema.eval_report_sinks.local_file import (
        LocalFileEvalReportSinkConfig,
    )

_logger = logging.getLogger(__name__)

__all__ = ["LocalFileEvalReportSink"]


class LocalFileEvalReportSink(EvalReportSink):
    """EvalReportSink that serializes an EvaluationReport to a local JSON file.

    Intended for development, testing, and single-machine workflows.
    Suitable as the default sink when no external service is configured.

    Args:
        config: ``LocalFileEvalReportSinkConfig`` controlling the output directory.
            A fresh ``tempfile.mkdtemp(prefix="michelangelo_reports_")``
            directory is created automatically when ``config`` is ``None`` or
            ``config.output_dir`` is ``None``.

    Example:
        >>> import tempfile
        >>> from michelangelo.workflow.schema.eval_report_sinks.local_file import (
        ...     LocalFileEvalReportSinkConfig,
        ... )
        >>> sink = LocalFileEvalReportSink(
        ...     LocalFileEvalReportSinkConfig(output_dir=tempfile.mkdtemp())
        ... )
    """

    def __init__(
        self,
        config: LocalFileEvalReportSinkConfig | None = None,
    ) -> None:
        """Initialise with an optional output directory config."""
        self._config = config

    def write(
        self,
        report: EvaluationReport,
        extra_fields: dict[str, Any] | None = None,
    ) -> EvalReportSinkResult:
        """Serialize the report to a JSON file in the configured directory.

        Args:
            report: An ``EvaluationReport`` proto with ``metadata.name`` set.
            extra_fields: Optional key-value pairs merged into the JSON
                document. ``extra_fields`` take precedence over proto fields
                on key collision.

        Returns:
            ``EvalReportSinkResult`` with the file path in ``output_path``.

        Raises:
            IOError: If the output directory cannot be created or the file
                cannot be written.
            ValueError: If ``report.metadata.name`` is empty (the plugin
                must set it before calling the sink).
        """
        name = report.metadata.name
        if not name:
            raise ValueError(
                "EvaluationReport.metadata.name must be set before writing. "
                "The plugin sets this automatically."
            )

        output_dir = (
            self._config.output_dir
            if self._config and self._config.output_dir
            else tempfile.mkdtemp(prefix="michelangelo_reports_")
        )
        os.makedirs(output_dir, exist_ok=True)

        document = {
            **MessageToDict(report, preserving_proto_field_name=True),
            **(extra_fields or {}),
        }

        output_path = os.path.join(output_dir, f"{name}.json")
        with open(output_path, "w", encoding="utf-8") as f:
            json.dump(document, f, indent=2)

        _logger.info(
            "LocalFileEvalReportSink: wrote report '%s' to '%s'.",
            name,
            output_path,
        )
        return EvalReportSinkResult(
            name=name,
            namespace=report.metadata.namespace,
            output_path=output_path,
        )
