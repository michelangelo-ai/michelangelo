"""GRPCEvalReportSink — pushes an EvaluationReport to a gRPC EvaluationReportService."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING, Any

from michelangelo.api.v2.services.base import (
    _CHANNEL_OPTIONS,
    _TIMEOUT_SECONDS,
    BaseService,
    Context,
)
from michelangelo.workflow.schema.eval_report_sinks.result import EvalReportSinkResult
from michelangelo.workflow.tasks.functions.eval_report_sinks.base import EvalReportSink

if TYPE_CHECKING:
    from michelangelo.api.v2.services.gen.evaluation_report import (
        EvaluationReportService as _EvaluationReportServiceType,
    )
    from michelangelo.gen.api.v2.evaluation_report_pb2 import EvaluationReport
    from michelangelo.workflow.schema.eval_report_sinks.api import (
        GRPCEvalReportSinkConfig,
    )

_logger = logging.getLogger(__name__)

__all__ = ["GRPCEvalReportSink"]


def _make_channel(config: GRPCEvalReportSinkConfig):  # type: ignore[type-arg]
    """Create a gRPC channel for the given sink config."""
    import grpc

    return (
        grpc.insecure_channel(config.endpoint, options=_CHANNEL_OPTIONS)
        if config.insecure
        else grpc.secure_channel(
            config.endpoint,
            grpc.ssl_channel_credentials(),
            options=_CHANNEL_OPTIONS,
        )
    )


class _EvalReportGRPCService(BaseService):
    """Private gRPC service for EvaluationReportService.

    Uses the ``APIClient`` ``BaseService`` infrastructure (header injection,
    retry policy via channel options) with a per-instance channel so each
    ``GRPCEvalReportSink`` can target an independent endpoint.
    """

    def __init__(self, context: Context) -> None:
        from michelangelo.gen.api.v2.evaluation_report_svc_pb2_grpc import (
            EvaluationReportServiceStub,
        )

        super().__init__(context, EvaluationReportServiceStub)

    def create(
        self,
        report: EvaluationReport,
        timeout: int = _TIMEOUT_SECONDS,
    ) -> EvaluationReport:
        """Call ``CreateEvaluationReport`` and return the created proto."""
        from michelangelo.gen.api.v2.evaluation_report_svc_pb2 import (
            CreateEvaluationReportRequest,
        )

        resp = self._stub.CreateEvaluationReport(
            CreateEvaluationReportRequest(evaluation_report=report),
            metadata=self._get_metadata({}),
            timeout=timeout,
        )
        return resp.evaluation_report


class GRPCEvalReportSink(EvalReportSink):
    """EvalReportSink that creates the report via a gRPC EvaluationReportService.

    Works with any server that implements the ``EvaluationReportService``
    interface defined in ``proto/api/v2/evaluation_report_svc.proto``.

    Two usage modes:

    **Self-contained** (``config`` provided, recommended for custom endpoints):
    Opens its own gRPC channel using the ``APIClient`` ``BaseService``
    infrastructure for header injection and retry policy (3 attempts,
    exponential 0.1 s → 10 s backoff on INTERNAL / UNAVAILABLE / UNKNOWN).
    Requires ``grpcio``::

        pip install grpcio

    **APIClient convenience** (``config=None``):
    Delegates to ``APIClient.EvaluationReportService``, reusing the channel
    already established by ``APIClient.init()``. Requires ``MA_API_SERVER`` to
    be set in the environment and ``APIClient.init()`` to have been called
    before the sink is created. No extra channel is opened or closed.

    The self-contained path supports the context-manager protocol for explicit
    channel cleanup::

        with GRPCEvalReportSink(cfg) as sink:
            sink.write(report)

    Args:
        config: ``GRPCEvalReportSinkConfig`` with the server endpoint and
            connection options. Pass ``None`` to reuse the ``APIClient``
            channel instead of opening a new one.

    Raises:
        ImportError: If ``grpcio`` is not installed (self-contained path only).

    Example (self-contained, local server)::

        from michelangelo.workflow.schema.eval_report_sinks.api import (
            GRPCEvalReportSinkConfig,
        )
        from michelangelo.workflow.tasks.functions.eval_report_sinks import (
            GRPCEvalReportSink,
        )

        sink = GRPCEvalReportSink(
            GRPCEvalReportSinkConfig(endpoint="localhost:50051")
        )

    Example (self-contained, remote TLS server)::

        sink = GRPCEvalReportSink(
            GRPCEvalReportSinkConfig(
                endpoint="eval-reports.example.com:443",
                namespace="ml-prod",
                insecure=False,
            )
        )

    Example (APIClient convenience path)::

        from michelangelo.api.v2 import APIClient
        APIClient.init()
        sink = GRPCEvalReportSink()  # config=None, reuses APIClient channel
    """

    def __init__(self, config: GRPCEvalReportSinkConfig | None = None) -> None:
        """Connect to the gRPC EvaluationReportService.

        Args:
            config: Connection configuration. When ``None``, reuses the
                ``APIClient`` channel — ``APIClient.init()`` must have been
                called before this constructor.

        Raises:
            ImportError: If ``grpcio`` is not installed (``config`` provided path).
        """
        if config is None:
            from michelangelo.api.v2 import APIClient

            self._svc: _EvalReportGRPCService | _EvaluationReportServiceType = (
                APIClient.EvaluationReportService
            )
            self._channel = None
            self._config = None
            _logger.info("GRPCEvalReportSink ready (APIClient channel).")
        else:
            try:
                import grpc  # noqa: F401
            except ImportError as exc:
                raise ImportError(
                    "GRPCEvalReportSink requires the 'grpcio' package. "
                    "Install it with: pip install grpcio"
                ) from exc

            self._channel = _make_channel(config)
            ctx = Context()
            ctx.channel = self._channel
            # DefaultHeaderProvider requires a caller; set a default so the sink
            # works without APIClient.set_caller() being called globally.
            ctx.header_provider._caller = "michelangelo-eval-report-sink"
            self._svc = _EvalReportGRPCService(ctx)
            self._config = config
            _logger.info(
                "GRPCEvalReportSink ready (endpoint=%s, insecure=%s).",
                config.endpoint,
                config.insecure,
            )

    def close(self) -> None:
        """Close the underlying gRPC channel and release resources.

        No-op when the sink was constructed without a ``config`` (APIClient
        path) — the channel lifecycle is managed by ``APIClient`` in that case.
        """
        if self._channel is not None:
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
        set (self-contained path only), then calls
        ``EvaluationReportService.CreateEvaluationReport``.
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
        if self._config is not None and self._config.namespace:
            report.metadata.namespace = self._config.namespace

        try:
            if self._config is None:
                created = self._svc.create_evaluation_report(report)
            else:
                created = self._svc.create(report, timeout=self._config.timeout_seconds)
        except Exception as exc:
            # grpc.RpcError is only available when grpcio is installed.
            try:
                import grpc as _grpc

                if not isinstance(exc, _grpc.RpcError):
                    raise
            except ImportError:
                raise
            endpoint = self._config.endpoint if self._config else "APIClient"
            raise OSError(
                f"GRPCEvalReportSink: gRPC CreateEvaluationReport failed "
                f"(endpoint={endpoint!r}, "
                f"code={exc.code()}, details={exc.details()!r})."  # type: ignore[attr-defined]
            ) from exc

        _logger.info(
            "GRPCEvalReportSink: created report '%s' in namespace '%s'.",
            created.metadata.name,
            created.metadata.namespace,
        )
        return EvalReportSinkResult(
            name=created.metadata.name,
            namespace=created.metadata.namespace,
        )
