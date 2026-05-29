"""EvalReportPusherPlugin — enriches an EvaluationReport and dispatches to sinks."""

from __future__ import annotations

import logging
import uuid
from typing import TYPE_CHECKING, Any

from michelangelo.gen.api.v2.evaluation_report_pb2 import EvaluationReport
from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.tasks.pusher.plugins.base import PusherPluginBase

if TYPE_CHECKING:
    from michelangelo.workflow.schema.pusher import EvalReportPluginConfig

_logger = logging.getLogger(__name__)

__all__ = ["EvalReportPusherPlugin"]


class EvalReportPusherPlugin(PusherPluginBase):
    """Plugin that enriches an EvaluationReport and dispatches it to sinks.

    Resolves ``metadata.name`` (config override → proto field → auto-generated
    UUID), sets it on the proto, then calls each configured sink's ``write()``
    method. The return dict exposes ``name``, ``namespace``, ``output_path``,
    and ``sinks`` for multi-sink result inspection.

    **Sinks** control where the report goes:

    - ``LocalFileEvalReportSink`` (default) — writes JSON to a temp dir.
    - ``GRPCEvalReportSink`` — pushes to any gRPC ``EvaluationReportService``
      endpoint.
    - Custom sinks: subclass ``EvalReportSink`` and pass an instance in
      ``EvalReportPluginConfig.sinks``.

    To send to both a local file and a gRPC endpoint::

        from michelangelo.workflow.schema.eval_report_sinks.api import (
            GRPCEvalReportSinkConfig,
        )
        from michelangelo.workflow.tasks.functions.eval_report_sinks import (
            GRPCEvalReportSink, LocalFileEvalReportSink,
        )
        cfg = EvalReportPluginConfig(
            sinks=[
                LocalFileEvalReportSink(),
                GRPCEvalReportSink(
                    GRPCEvalReportSinkConfig(endpoint="localhost:50051")
                ),
            ],
            report_name="q1-eval",
        )

    To integrate with MLflow after the local file is written::

        result = plugin.execute()
        import mlflow
        mlflow.log_artifact(result["output_path"], artifact_path="eval_reports")

    .. note::
        The ``output_path`` and ``namespace`` at the top level of the result
        dict come from the **first** sink only. In a multi-sink setup, inspect
        ``result["sinks"]`` for per-sink details.

    Args:
        config: ``EvalReportPluginConfig`` controlling sinks, name, and
            extra_fields.
        artifact: An ``EvaluationReport`` protobuf message.
        storage_backend: Unused by this built-in implementation.
        registry_client: Unused by this built-in implementation.

    Raises:
        ConfigurationError: If ``artifact`` is ``None`` or not an
            ``EvaluationReport`` instance.

    Example::

        from michelangelo.gen.api.v2.evaluation_report_pb2 import (
            EvaluationReport,
            EvaluationReportSpec,
        )
        from michelangelo.workflow.schema.pusher import EvalReportPluginConfig
        from michelangelo.workflow.tasks.pusher.plugins.eval_report_plugin import (
            EvalReportPusherPlugin,
        )

        spec = EvaluationReportSpec(title="Q1 Evaluation")
        report = EvaluationReport(spec=spec)

        plugin = EvalReportPusherPlugin(
            config=EvalReportPluginConfig(report_name="q1-eval"),
            artifact=report,
        )
        result = plugin.execute()
        # result["name"]        → "q1-eval"
        # result["namespace"]   → ""  (set by GRPCEvalReportSink / custom sinks)
        # result["output_path"] → "/tmp/michelangelo_reports_.../q1-eval.json"
    """

    def __init__(
        self,
        config: EvalReportPluginConfig,
        artifact: EvaluationReport | None = None,
        storage_backend: Any = None,
        registry_client: Any = None,
    ) -> None:
        """Validate that artifact is a non-None EvaluationReport.

        Args:
            config: Plugin configuration.
            artifact: An ``EvaluationReport`` protobuf message.
            storage_backend: Unused.
            registry_client: Unused.

        Raises:
            ConfigurationError: If ``artifact`` is ``None`` or not an
                ``EvaluationReport`` instance.
        """
        super().__init__(config, artifact, storage_backend, registry_client)
        if artifact is None:
            raise ConfigurationError(
                "EvalReportPusherPlugin requires an EvaluationReport artifact. "
                "Build one with EvaluationReport(spec=EvaluationReportSpec(...)) "
                "and pass it via artifact=."
            )
        if not isinstance(artifact, EvaluationReport):
            raise ConfigurationError(
                f"artifact must be an EvaluationReport; "
                f"got {type(artifact).__name__}. "
                "Use EvaluationReport(spec=EvaluationReportSpec(...)) to build one."
            )

    def execute(self) -> dict[str, Any]:
        """Enrich the EvaluationReport and dispatch to all configured sinks.

        Resolves ``metadata.name`` (config override → proto field →
        auto-generated), sets it on the proto, then calls each sink's
        ``write()`` method with the enriched proto and ``extra_fields``.

        Returns:
            A dict with:

            - ``"name"``: resolved ``metadata.name`` of the report.
            - ``"namespace"``: ``metadata.namespace`` from the first sink
              result (or the proto's own namespace when no sinks are active).
            - ``"output_path"``: ``output_path`` from the first sink result
              (empty string when the first sink does not write local files).
            - ``"sinks"``: list of per-sink result dicts, each with
              ``name``, ``namespace``, ``output_path``, and ``extra``.

            .. note::
                Sinks are called in order (fail-fast). If sink *N* raises,
                sinks *N+1…M* are not called and partial results from sinks
                *0…N-1* are not returned. Inspect ``result["sinks"]`` for
                per-sink details; ``output_path`` and ``namespace`` reflect
                the **first** sink only.

        Raises:
            IOError: If any sink raises during ``write()``. The exception
                propagates immediately; remaining sinks are not called.
        """
        # Resolve name: config override → proto.metadata.name → auto-generate.
        # Set it on the proto so every sink receives the enriched proto.
        name = (
            self._config.report_name
            or self._artifact.metadata.name
            or f"eval-report-{uuid.uuid4().hex[:12]}"
        )
        self._artifact.metadata.name = name

        sinks = self._config.sinks
        if sinks is None:
            from michelangelo.workflow.tasks.functions.eval_report_sinks import (
                LocalFileEvalReportSink,
            )

            sinks = [LocalFileEvalReportSink()]

        sink_results = []
        extra = dict(self._config.extra_fields or {})
        for sink in sinks:
            result = sink.write(self._artifact, extra)
            sink_results.append(
                {
                    "name": result.name,
                    "namespace": result.namespace,
                    "output_path": result.output_path,
                    **result.extra,
                }
            )

        first = sink_results[0] if sink_results else {}
        _logger.info(
            "EvalReportPusherPlugin: dispatched report '%s' to %d sink(s).",
            name,
            len(sink_results),
        )
        return {
            "name": name,
            "namespace": first.get("namespace", self._artifact.metadata.namespace),
            "output_path": first.get("output_path", ""),
            "sinks": sink_results,
        }
