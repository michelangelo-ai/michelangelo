"""Sinks that push an EvaluationReport to a gRPC or MA APIClient endpoint.

Two implementations are provided:

- ``GRPCEvalReportSink`` — self-contained; opens its own gRPC channel to any
  server that implements the ``EvaluationReportService`` proto interface.
- ``APIClientEvalReportSink`` — zero-channel; delegates to
  ``APIClient.EvaluationReportService``, reusing the shared singleton channel.
"""

from __future__ import annotations

import copy
import logging
import warnings
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


def _raise_as_oserror(exc: Exception, context: str) -> None:
    """Re-raise a grpc.RpcError as OSError; pass all other exceptions through."""
    try:
        import grpc as _grpc
    except ImportError:
        raise exc from None
    if not isinstance(exc, _grpc.RpcError):
        raise exc
    raise OSError(
        f"{context}: gRPC CreateEvaluationReport failed "
        f"(code={exc.code()}, details={exc.details()!r})."  # type: ignore[attr-defined]
    ) from exc


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
                The ``config.namespace`` value, if set, is injected into a
                deep copy of each report before the RPC — the caller's proto
                is never mutated. The ``rpc-caller`` header defaults to
                ``"michelangelo-eval-report-sink"``; expose a custom value
                via ``GRPCEvalReportSinkConfig`` in a future release (#1258).

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

        When ``config.namespace`` is set, injects it into a deep copy of the
        report before the RPC. The caller's proto is never mutated.
        ``extra_fields`` are not part of the proto schema and cannot be
        forwarded to the server — a ``UserWarning`` is emitted if provided.

        Args:
            report: An ``EvaluationReport`` proto with ``metadata.name`` set.
            extra_fields: Not supported by this sink. Pass ``None`` or omit.
                A ``UserWarning`` is emitted if a non-empty dict is provided.

        Returns:
            ``EvalReportSinkResult`` with name and namespace as confirmed by
            the server response.

        Raises:
            IOError: If the gRPC call fails.
        """
        if extra_fields:
            warnings.warn(
                f"GRPCEvalReportSink.write() received extra_fields but this sink "
                f"does not support extra fields ({list(extra_fields)!r} ignored). "
                "Use LocalFileEvalReportSink if you need extra fields in the output.",
                UserWarning,
                stacklevel=2,
            )

        if self._config.namespace:
            report = copy.deepcopy(report)
            report.metadata.namespace = self._config.namespace

        try:
            created = self._svc.create(report, timeout=self._config.timeout_seconds)
        except Exception as exc:
            _raise_as_oserror(
                exc, f"GRPCEvalReportSink(endpoint={self._config.endpoint!r})"
            )

        _logger.debug(
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

    **When to use each sink:**

    - Use ``APIClientEvalReportSink`` when your process already calls
      ``APIClient`` services and you want a shared connection.
    - Use ``GRPCEvalReportSink`` when you need an isolated channel, a
      different endpoint, or automatic namespace injection.

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

    def __init__(self, svc: _EvaluationReportServiceType | None = None) -> None:
        """Bind to ``APIClient.EvaluationReportService``.

        Args:
            svc: Optional pre-built ``EvaluationReportService`` instance. When
                ``None`` (default), the service is taken from
                ``APIClient.EvaluationReportService``. Pass an explicit service
                for testing or to target a different instance.

        Raises:
            RuntimeError: If ``APIClient.EvaluationReportService`` is ``None``
                (i.e. ``MA_API_SERVER`` was not set before this import).
        """
        if svc is not None:
            self._svc: _EvaluationReportServiceType = svc
        else:
            from michelangelo.api.v2 import APIClient

            self._svc = APIClient.EvaluationReportService
            if self._svc is None:
                raise RuntimeError(
                    "APIClient.EvaluationReportService is not initialized. "
                    "Set MA_API_SERVER in the environment before constructing "
                    "APIClientEvalReportSink."
                )
        _logger.info("APIClientEvalReportSink ready (APIClient channel).")

    def write(
        self,
        report: EvaluationReport,
        extra_fields: dict[str, Any] | None = None,
    ) -> EvalReportSinkResult:
        """Create the evaluation report via ``APIClient.EvaluationReportService``.

        ``extra_fields`` are not part of the proto schema and cannot be
        forwarded to the server — a ``UserWarning`` is emitted if provided.

        Args:
            report: An ``EvaluationReport`` proto with ``metadata.name`` and
                ``metadata.namespace`` already set by the caller.
            extra_fields: Not supported by this sink. Pass ``None`` or omit.
                A ``UserWarning`` is emitted if a non-empty dict is provided.

        Returns:
            ``EvalReportSinkResult`` with name and namespace as confirmed by
            the server response.

        Raises:
            IOError: If the gRPC call fails.
            ValueError: If ``MA_API_SERVER`` is not set (raised on first call).
        """
        if extra_fields:
            warnings.warn(
                f"APIClientEvalReportSink.write() received extra_fields but this sink "
                f"does not support extra fields ({list(extra_fields)!r} ignored). "
                "Use LocalFileEvalReportSink if you need extra fields in the output.",
                UserWarning,
                stacklevel=2,
            )

        try:
            created = self._svc.create_evaluation_report(report)
        except Exception as exc:
            _raise_as_oserror(exc, "APIClientEvalReportSink")

        _logger.debug(
            "APIClientEvalReportSink: created report '%s' in namespace '%s'.",
            created.metadata.name,
            created.metadata.namespace,
        )
        return EvalReportSinkResult(
            name=created.metadata.name,
            namespace=created.metadata.namespace,
        )
