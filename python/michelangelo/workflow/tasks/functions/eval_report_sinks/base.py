"""EvalReportSink — abstract base class for evaluation report sinks."""

from __future__ import annotations

from abc import ABC, abstractmethod
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from michelangelo.gen.api.v2.evaluation_report_pb2 import EvaluationReport
    from michelangelo.workflow.schema.eval_report_sinks.result import (
        EvalReportSinkResult,
    )

__all__ = ["EvalReportSink"]


class EvalReportSink(ABC):
    """Abstract base class for evaluation report sinks.

    Each sink writes (or pushes) an ``EvaluationReport`` to a specific
    destination — a local JSON file, a gRPC API server, a cloud object
    store, etc. The plugin iterates over a list of sinks and calls
    ``write()`` on each.

    Implementations are infrastructure-specific:

    - ``LocalFileEvalReportSink`` — writes JSON to a local directory
      (built-in, zero dependencies beyond the core package).
    - ``GRPCEvalReportSink`` — pushes to any ``EvaluationReportService`` gRPC endpoint,
      including a local sandbox server (built-in, requires ``grpcio``).
    - Community / provider sinks live outside this package.

    Example implementation::

        class S3EvalReportSink(EvalReportSink):
            def write(
                self,
                report: EvaluationReport,
                extra_fields: dict[str, Any] | None = None,
            ) -> EvalReportSinkResult:
                doc = MessageToDict(report, preserving_proto_field_name=True)
                doc.update(extra_fields or {})
                key = f"eval-reports/{report.metadata.name}.json"
                s3.put_object(Body=json.dumps(doc), Bucket=self._bucket, Key=key)
                return EvalReportSinkResult(
                    name=report.metadata.name,
                    namespace=report.metadata.namespace,
                    output_path=f"s3://{self._bucket}/{key}",
                )
    """

    @abstractmethod
    def write(
        self,
        report: EvaluationReport,
        extra_fields: dict[str, Any] | None = None,
    ) -> EvalReportSinkResult:
        """Write or push the evaluation report.

        Args:
            report: An ``EvaluationReport`` proto with ``metadata.name`` already
                set by the plugin. Sinks may further enrich the proto (e.g.
                ``GRPCEvalReportSink`` injects ``metadata.namespace``).
            extra_fields: Additional key-value pairs to merge into the output
                document. Sinks that write structured files (e.g.
                ``LocalFileEvalReportSink``) merge these into the JSON.
                Sinks that push to an API (e.g. ``GRPCEvalReportSink``) ignore them.

        Returns:
            ``EvalReportSinkResult`` with the resolved ``name``,
            ``namespace``, and optionally ``output_path``.

        Raises:
            IOError: If the write or push fails.
        """
