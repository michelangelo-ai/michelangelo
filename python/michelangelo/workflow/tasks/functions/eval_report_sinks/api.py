"""GRPCEvalReportSink — pushes an EvaluationReport to a gRPC EvaluationReportService."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING, Any

from michelangelo.workflow.schema.eval_report_sinks.result import EvalReportSinkResult
from michelangelo.workflow.tasks.functions.eval_report_sinks.base import EvalReportSink

if TYPE_CHECKING:
    from michelangelo.gen.api.v2.evaluation_report_pb2 import EvaluationReport
    from michelangelo.workflow.schema.eval_report_sinks.api import (
        GRPCEvalReportSinkConfig,
    )

_logger = logging.getLogger(__name__)

__all__ = ["GRPCEvalReportSink"]


class GRPCEvalReportSink(EvalReportSink):
    """EvalReportSink that creates the report via a gRPC EvaluationReportService.

    Works with any server that implements the ``EvaluationReportService``
    interface defined in ``proto/api/v2/evaluation_report_svc.proto``.

    Uses a **plaintext channel** by default for local development convenience.
    Set ``config.insecure=False`` and point ``config.endpoint`` at a TLS
    endpoint for any non-local use.

    Requires ``grpcio``::

        pip install grpcio

    This sink implements the context-manager protocol for explicit channel
    cleanup. Use it as a context manager in long-running processes::

        with GRPCEvalReportSink(cfg) as sink:
            sink.write(report)

    Args:
        config: ``GRPCEvalReportSinkConfig`` with the server endpoint and
            connection options.

    Raises:
        ImportError: If ``grpcio`` is not installed.

    Example (local server)::

        from michelangelo.workflow.schema.eval_report_sinks.api import (
            GRPCEvalReportSinkConfig,
        )
        from michelangelo.workflow.tasks.functions.eval_report_sinks import (
            GRPCEvalReportSink,
        )

        sink = GRPCEvalReportSink(
            GRPCEvalReportSinkConfig(endpoint="localhost:50051")
        )

    Example (remote TLS server)::

        sink = GRPCEvalReportSink(
            GRPCEvalReportSinkConfig(
                endpoint="eval-reports.example.com:443",
                namespace="ml-prod",
                insecure=False,
            )
        )
    """

    def __init__(self, config: GRPCEvalReportSinkConfig) -> None:
        """Connect to the gRPC endpoint described by ``config``.

        Args:
            config: Connection configuration.

        Raises:
            ImportError: If ``grpcio`` is not installed.
        """
        try:
            import grpc

            from michelangelo.gen.api.v2.evaluation_report_svc_pb2_grpc import (
                EvaluationReportServiceStub,
            )
        except ImportError as exc:
            raise ImportError(
                "GRPCEvalReportSink requires the 'grpcio' package. "
                "Install it with: pip install grpcio"
            ) from exc

        self._channel = (
            grpc.insecure_channel(config.endpoint)
            if config.insecure
            else grpc.secure_channel(
                config.endpoint, grpc.ssl_channel_credentials()
            )
        )
        self._stub = EvaluationReportServiceStub(self._channel)
        self._config = config
        _logger.info(
            "GRPCEvalReportSink ready (endpoint=%s, insecure=%s).",
            config.endpoint,
            config.insecure,
        )

    def close(self) -> None:
        """Close the underlying gRPC channel and release resources."""
        self._channel.close()

    def __enter__(self) -> GRPCEvalReportSink:
        """Return self to support use as a context manager."""
        return self

    def __exit__(self, *exc: object) -> None:
        """Close the channel on context-manager exit."""
        self.close()

    def write(
        self,
        report: EvaluationReport,
        extra_fields: dict[str, Any] | None = None,
    ) -> EvalReportSinkResult:
        """Create the evaluation report via gRPC.

        Injects ``config.namespace`` into ``report.metadata.namespace`` when
        set, then calls ``EvaluationReportService.CreateEvaluationReport``.
        ``extra_fields`` are ignored — they are not part of the proto schema
        and cannot be forwarded to the API server.

        Args:
            report: An ``EvaluationReport`` proto with ``metadata.name`` set.
            extra_fields: Ignored by this sink.

        Returns:
            ``EvalReportSinkResult`` with name and namespace as confirmed by
            the server response.

        Raises:
            IOError: If the gRPC call fails.
        """
        import grpc

        from michelangelo.gen.api.v2.evaluation_report_svc_pb2 import (
            CreateEvaluationReportRequest,
        )

        if self._config.namespace:
            report.metadata.namespace = self._config.namespace

        try:
            resp = self._stub.CreateEvaluationReport(
                CreateEvaluationReportRequest(evaluation_report=report),
                timeout=self._config.timeout_seconds,
            )
        except grpc.RpcError as exc:
            raise OSError(
                f"GRPCEvalReportSink: gRPC CreateEvaluationReport failed "
                f"(endpoint={self._config.endpoint!r}, "
                f"code={exc.code()}, details={exc.details()!r})."
            ) from exc

        created = resp.evaluation_report
        _logger.info(
            "GRPCEvalReportSink: created report '%s' in namespace '%s'.",
            created.metadata.name,
            created.metadata.namespace,
        )
        return EvalReportSinkResult(
            name=created.metadata.name,
            namespace=created.metadata.namespace,
        )
