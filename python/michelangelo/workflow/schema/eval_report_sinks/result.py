"""EvalReportSinkResult — the return contract for all EvalReportSink.write() calls."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any

__all__ = ["EvalReportSinkResult"]


@dataclass(frozen=True)
class EvalReportSinkResult:
    """The outcome of a single EvalReportSink.write() call.

    All sinks return this so the plugin can aggregate results regardless of
    which backends are active.

    Attributes:
        name: The ``metadata.name`` of the report after the write, as assigned
            or confirmed by the sink (e.g. echoed back from the API server).
        namespace: The ``metadata.namespace`` after the write. Empty string
            when the sink does not manage namespaces (e.g. local file).
        output_path: Absolute filesystem path to the written file. Empty
            string for sinks that do not write local files (e.g. API sinks).
        extra: Sink-specific metadata (e.g. gRPC resource version, HTTP
            location header). Passed through to the plugin result dict.

    Example:
        >>> r = EvalReportSinkResult(name="q1-eval", namespace="ml-prod")
        >>> r.output_path
        ''
    """

    name: str
    namespace: str
    output_path: str = ""
    extra: dict[str, Any] = field(default_factory=dict)
