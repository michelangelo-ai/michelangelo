"""APISink — pushes an EvaluationReport to a gRPC EvaluationReportService."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING, Any

from michelangelo.workflow.schema.eval_report_sinks.result import EvalReportSinkResult
from michelangelo.workflow.tasks.functions.eval_report_sinks.base import EvalReportSink

if TYPE_CHECKING:
    from michelangelo.gen.api.v2.evaluation_report_pb2 import EvaluationReport
    from michelangelo.workflow.schema.eval_report_sinks.api import APISinkConfig

_logger = logging.getLogger(__name__)

__all__ = ["APISink"]


class APISink(EvalReportSink):
    """EvalReportSink that creates the report via a gRPC EvaluationReportService.

    Works with any server that implements the ``EvaluationReportService``
    interface defined in ``proto/api/v2/evaluation_report_svc.proto`` — a
    local sandbox, a community deployment, or a provider API server.

    Uses an **insecure channel** by default for local sandbox convenience.
    Set ``config.insecure=False`` and point ``config.endpoint`` at a TLS
    endpoint for production use.

    Requires ``grpcio``::

        pip install grpcio

    Args:
        config: ``APISinkConfig`` with the server endpoint and connection
            options.

    Raises:
        ImportError: If ``grpcio`` is not installed.

    Example (local sandbox)::

        from michelangelo.workflow.schema.eval_report_sinks.api import APISinkConfig
        from michelangelo.workflow.tasks.functions.eval_report_sinks import APISink

        sink = APISink(APISinkConfig(endpoint="localhost:50051"))

    Example (remote TLS server)::

        sink = APISink(
            APISinkConfig(
                endpoint="api.michelangelo.io:443",
                namespace="ml-prod",
                insecure=False,
            )
        )
    """

    def __init__(self, config: APISinkConfig) -> None:
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
                "APISink requires the 'grpcio' package. "
                "Install it with: pip install grpcio"
            ) from exc

        channel = (
            grpc.insecure_channel(config.endpoint)
            if config.insecure
            else grpc.secure_channel(
                config.endpoint, grpc.ssl_channel_credentials()
            )
        )
        self._stub = EvaluationReportServiceStub(channel)
        self._config = config
        _logger.info(
            "APISink ready (endpoint=%s, insecure=%s).",
            config.endpoint,
            config.insecure,
        )

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
        except Exception as exc:
            raise OSError(
                f"APISink: gRPC CreateEvaluationReport failed "
                f"(endpoint={self._config.endpoint!r})."
            ) from exc

        created = resp.evaluation_report
        _logger.info(
            "APISink: created report '%s' in namespace '%s'.",
            created.metadata.name,
            created.metadata.namespace,
        )
        return EvalReportSinkResult(
            name=created.metadata.name,
            namespace=created.metadata.namespace,
        )
