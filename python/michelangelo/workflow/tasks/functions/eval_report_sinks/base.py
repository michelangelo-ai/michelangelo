"""EvalReportSink — abstract base class for evaluation report sinks.

``EvaluationReport`` uses Michelangelo's own proto schema
(``proto/api/v2/evaluation_report.proto``), not OpenMetrics or OTLP. Custom
sinks that target systems expecting flat key-value metrics (MLflow, W&B, Comet)
will need to convert the proto; see :func:`flatten_report_to_metrics` for a
ready-made helper.
"""

from __future__ import annotations

import contextlib
import logging
from abc import ABC, abstractmethod
from typing import TYPE_CHECKING, Any

_logger = logging.getLogger(__name__)

if TYPE_CHECKING:
    from michelangelo.gen.api.v2.evaluation_report_pb2 import EvaluationReport
    from michelangelo.workflow.schema.eval_report_sinks.result import (
        EvalReportSinkResult,
    )

__all__ = ["EvalReportSink", "flatten_report_to_metrics"]


def flatten_report_to_metrics(report: EvaluationReport) -> dict[str, float]:
    """Extract a flat ``{metric_name: value}`` dict from an ``EvaluationReport``.

    Useful when writing a custom sink for systems that expect per-metric
    key-value pairs (MLflow ``log_metrics``, W&B ``wandb.log``, Comet
    ``log_metrics``, etc.) rather than a structured proto document.

    Metric names are derived from each chart's title, falling back to
    ``"metric_<index>"`` when the title is absent.  Only scalar (single
    data-point) series are extracted; multi-point series are skipped.

    Args:
        report: An ``EvaluationReport`` proto, typically as received in
            ``EvalReportSink.write()``.

    Returns:
        A flat ``dict[str, float]`` suitable for passing directly to
        ``mlflow.log_metrics()``, ``wandb.log()``, or equivalent.

    Example::

        class MLflowEvalReportSink(EvalReportSink):
            def write(self, report, extra_fields=None):
                import mlflow
                mlflow.log_metrics(flatten_report_to_metrics(report))
                return EvalReportSinkResult(
                    name=report.metadata.name,
                    namespace=report.metadata.namespace,
                )
    """
    from google.protobuf.json_format import MessageToDict

    doc = MessageToDict(report, preserving_proto_field_name=True)
    metrics: dict[str, float] = {}
    for i, chart in enumerate(doc.get("spec", {}).get("charts", [])):
        key = chart.get("title") or f"metric_{i}"
        series = chart.get("series", [])
        if len(series) != 1:
            # TODO(v2): support multi-series charts (comparison, overlays) — #1258
            _logger.warning(
                "flatten_report_to_metrics: skipping %r — %d series (expected 1); "
                "data will not appear in the returned dict",
                key,
                len(series),
            )
            continue
        data_points = series[0].get("data_points", [])
        if len(data_points) != 1:
            # TODO(v2): multi-point series; expose step for log_metric(step=) — #1258
            _logger.warning(
                "flatten_report_to_metrics: skipping %r — %d data points (expected 1); "
                "data will not appear in the returned dict",
                key,
                len(data_points),
            )
            continue
        v = data_points[0].get("value")
        if v is None:
            continue
        with contextlib.suppress(TypeError, ValueError):
            metrics[key] = float(v)
    return metrics


class EvalReportSink(ABC):
    """Abstract base class for evaluation report sinks.

    Each sink writes (or pushes) an ``EvaluationReport`` to a specific
    destination — a local JSON file, a gRPC server, a cloud object store, etc.
    The plugin iterates over a list of sinks and calls ``write()`` on each in
    order.

    **Dispatch semantics:** ``write()`` is called synchronously, once per sink,
    in the order sinks appear in ``EvalReportPluginConfig.sinks``. If a sink
    raises, the exception propagates immediately (fail-fast) and subsequent
    sinks in the list are not called. Implementations need not be thread-safe.

    Implementations are infrastructure-specific:

    - ``LocalFileEvalReportSink`` — writes JSON to a local directory
      (built-in, zero dependencies beyond the core package).
    - ``APIClientEvalReportSink`` — pushes to ``APIClient.EvaluationReportService``
      via the shared singleton gRPC channel (built-in).
    - Custom sinks (e.g. cloud storage, message queues, MLflow, W&B) live outside
      this package. Use :func:`flatten_report_to_metrics` to convert the proto to
      a flat dict for systems that expect key-value metrics.

    Example implementation (imports and ``self._bucket`` initialisation elided)::

        # MessageToDict is used the same way as in flatten_report_to_metrics()
        from google.protobuf.json_format import MessageToDict

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

        This method is called synchronously by the plugin. Implementations
        need not be async or thread-safe.

        .. note::
            If ``report_name`` is fixed in ``EvalReportPluginConfig``, retrying
            a failed pipeline run will call ``write()`` again with the same
            ``metadata.name``. Sinks that create server-side resources (e.g.
            ``APIClientEvalReportSink``) should be idempotent or handle duplicates
            gracefully.

        Args:
            report: An ``EvaluationReport`` proto with ``metadata.name`` already
                set by the plugin. Sinks may further enrich the proto before
                sending.
            extra_fields: Additional key-value pairs to merge into the output
                document. Sinks that write structured files (e.g.
                ``LocalFileEvalReportSink``) merge these into the JSON.
                Sinks that push to an API (e.g. ``APIClientEvalReportSink``)
                emit a ``UserWarning`` and ignore them. Treat as read-only —
                do not mutate the dict.

        Returns:
            ``EvalReportSinkResult`` with the resolved ``name``,
            ``namespace``, and optionally ``output_path``.

        Raises:
            IOError: If the write or push fails. The plugin does not catch
                this — the exception aborts the sink loop (fail-fast).
        """
