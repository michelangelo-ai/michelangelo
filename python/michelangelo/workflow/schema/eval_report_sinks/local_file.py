"""LocalFileEvalReportSinkConfig — config for the local filesystem EvalReport sink."""

from __future__ import annotations

from dataclasses import dataclass

__all__ = ["LocalFileEvalReportSinkConfig"]


@dataclass
class LocalFileEvalReportSinkConfig:
    """Configuration for ``LocalFileEvalReportSink``.

    Attributes:
        output_dir: Directory where the JSON file is written. A fresh
            ``tempfile.mkdtemp(prefix="michelangelo_reports_")`` directory
            is created automatically when ``None``.

    Example:
        >>> cfg = LocalFileEvalReportSinkConfig(output_dir="/tmp/reports")
        >>> cfg.output_dir
        '/tmp/reports'
    """

    output_dir: str | None = None
