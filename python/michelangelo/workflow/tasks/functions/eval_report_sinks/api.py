"""Sinks that push an EvaluationReport to the Michelangelo API.

One implementation is provided:

- ``APIClientEvalReportSink`` — delegates to
  ``APIClient.EvaluationReportService``, reusing the shared singleton channel.

To target a different endpoint, pass an explicit service via the ``svc`` param::

    from michelangelo.api.v2 import APIClient
    client = APIClient(endpoint="other-server:50051", caller="my-trainer")
    sink = APIClientEvalReportSink(svc=client.EvaluationReportService)
"""

from __future__ import annotations

import logging
import warnings
from typing import TYPE_CHECKING, Any, NoReturn

from michelangelo.workflow.schema.eval_report_sinks.result import EvalReportSinkResult
from michelangelo.workflow.tasks.functions.eval_report_sinks.base import EvalReportSink

if TYPE_CHECKING:
    from michelangelo.api.v2.services.gen.evaluation_report import (
        EvaluationReportService as _EvaluationReportServiceType,
    )
    from michelangelo.gen.api.v2.evaluation_report_pb2 import EvaluationReport

_logger = logging.getLogger(__name__)

__all__ = ["APIClientEvalReportSink"]


def _raise_as_oserror(exc: Exception, context: str) -> NoReturn:
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

    To target a **different endpoint** than the default ``MA_API_SERVER``, pass
    an explicit service built from a per-instance ``APIClient``::

        from michelangelo.api.v2 import APIClient
        client = APIClient(endpoint="other-server:50051", caller="my-trainer")
        sink = APIClientEvalReportSink(svc=client.EvaluationReportService)

    Example (default — reads ``MA_API_SERVER``)::

        import os
        os.environ["MA_API_SERVER"] = "localhost:50051"
        from michelangelo.api.v2 import APIClient
        APIClient.set_caller("my-trainer")  # optional, sets the rpc-caller header

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
                to target a different endpoint or for testing without patching
                globals::

                    from michelangelo.api.v2 import APIClient
                    client = APIClient(endpoint="other:50051", caller="job")
                    sink = APIClientEvalReportSink(svc=client.EvaluationReportService)

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
        _logger.debug("APIClientEvalReportSink ready (APIClient channel).")

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
            OSError: If the gRPC call fails.
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
