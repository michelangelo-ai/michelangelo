"""Sinks that push an EvaluationReport to a gRPC or MA APIClient endpoint.

Two implementations are provided:

- ``GRPCEvalReportSink`` — self-contained; opens its own gRPC channel to any
  server that implements the ``EvaluationReportService`` proto interface.
- ``APIClientEvalReportSink`` — zero-channel; delegates to
  ``APIClient.EvaluationReportService``, reusing the shared singleton channel.
"""

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

__all__ = ["APIClientEvalReportSink", "GRPCEvalReportSink"]


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
    """EvalReportSink that opens its own gRPC channel to any EvaluationReportService.

    Works with any server that implements the ``EvaluationReportService``
    interface defined in ``proto/api/v2/evaluation_report_svc.proto``.

    Uses the ``APIClient`` ``BaseService`` infrastructure for header injection
    and retry policy (3 attempts, exponential 0.1 s → 10 s backoff on
    INTERNAL / UNAVAILABLE / UNKNOWN). Requires ``grpcio``::

        pip install grpcio

    Supports the context-manager protocol for explicit channel cleanup::

        with GRPCEvalReportSink(cfg) as sink:
            sink.write(report)

    To reuse the channel already managed by ``APIClient`` instead of opening
    a new one, use ``APIClientEvalReportSink``.

    Args:
        config: ``GRPCEvalReportSinkConfig`` with the server endpoint and
            connection options.

    Raises:
        ImportError: If ``grpcio`` is not installed.

    Example (local insecure server)::

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
        """Connect to the gRPC EvaluationReportService.

        Args:
            config: Connection configuration with endpoint and TLS options.

        Raises:
            ImportError: If ``grpcio`` is not installed.
        """
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
        ``extra_fields`` are ignored — they are not part of the proto schema.

        Args:
            report: An ``EvaluationReport`` proto with ``metadata.name`` set.

                .. warning::
                    This method mutates ``report.metadata.namespace`` in place
                    when ``config.namespace`` is set. In a multi-sink workflow
                    where the same proto is passed to multiple sinks, this
                    mutation is visible to all subsequent sinks. Clone the
                    report before passing if you need to preserve the original
                    namespace.

            extra_fields: Ignored by this sink.

        Returns:
            ``EvalReportSinkResult`` with name and namespace as confirmed by
            the server response.

        Raises:
            IOError: If the gRPC call fails.
        """
        if self._config.namespace:
            report.metadata.namespace = self._config.namespace

        try:
            created = self._svc.create(report, timeout=self._config.timeout_seconds)
        except Exception as exc:
            try:
                import grpc as _grpc
            except ImportError:
                raise exc from None
            if not isinstance(exc, _grpc.RpcError):
                raise
            raise OSError(
                f"GRPCEvalReportSink: gRPC CreateEvaluationReport failed "
                f"(endpoint={self._config.endpoint!r}, "
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


class APIClientEvalReportSink(EvalReportSink):
    """EvalReportSink that delegates to ``APIClient.EvaluationReportService``.

    Reuses the shared gRPC channel already managed by ``APIClient`` — no
    additional channel is opened or closed. Use this when the calling process
    already initialises ``APIClient`` via the ``MA_API_SERVER`` environment
    variable and you want eval-report writes to share that connection.

    Requires ``MA_API_SERVER`` to be set in the environment before the first
    ``write()`` call (the channel is opened lazily on the first RPC).

    Does **not** inject a namespace — the caller is responsible for setting
    ``report.metadata.namespace`` before calling ``write()``.

    Example::

        import os
        os.environ["MA_API_SERVER"] = "localhost:50051"
        from michelangelo.api.v2 import APIClient
        APIClient.set_caller("my-trainer")  # optional, sets rpc-caller header

        from michelangelo.workflow.tasks.functions.eval_report_sinks import (
            APIClientEvalReportSink,
        )

        sink = APIClientEvalReportSink()
        report.metadata.namespace = "my-project"
        sink.write(report)
    """

    def __init__(self) -> None:
        """Bind to ``APIClient.EvaluationReportService``.

        Raises:
            ValueError: On the first ``write()`` call if ``MA_API_SERVER`` is
                not set in the environment (raised by the lazy channel init).
        """
        from michelangelo.api.v2 import APIClient

        self._svc: _EvaluationReportServiceType = APIClient.EvaluationReportService
        _logger.info("APIClientEvalReportSink ready (APIClient channel).")

    def write(
        self,
        report: EvaluationReport,
        extra_fields: dict[str, Any] | None = None,
    ) -> EvalReportSinkResult:
        """Create the evaluation report via ``APIClient.EvaluationReportService``.

        ``extra_fields`` are ignored — they are not part of the proto schema
        and cannot be forwarded to the API server.

        Args:
            report: An ``EvaluationReport`` proto with ``metadata.name`` and
                ``metadata.namespace`` already set by the caller.
            extra_fields: Ignored by this sink.

        Returns:
            ``EvalReportSinkResult`` with name and namespace as confirmed by
            the server response.

        Raises:
            IOError: If the gRPC call fails.
            ValueError: If ``MA_API_SERVER`` is not set (raised on first call).
        """
        try:
            created = self._svc.create_evaluation_report(report)
        except Exception as exc:
            try:
                import grpc as _grpc
            except ImportError:
                raise exc from None
            if not isinstance(exc, _grpc.RpcError):
                raise
            raise OSError(
                f"APIClientEvalReportSink: gRPC CreateEvaluationReport failed "
                f"(code={exc.code()}, details={exc.details()!r})."  # type: ignore[attr-defined]
            ) from exc

        _logger.info(
            "APIClientEvalReportSink: created report '%s' in namespace '%s'.",
            created.metadata.name,
            created.metadata.namespace,
        )
        return EvalReportSinkResult(
            name=created.metadata.name,
            namespace=created.metadata.namespace,
        )
