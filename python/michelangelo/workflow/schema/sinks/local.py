"""Config dataclass for LocalFileSink."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from michelangelo.workflow.schema.pusher import DatasetFormat


@dataclass
class LocalFileSinkConfig:
    """Typed configuration for ``LocalFileSink``.

    Attributes:
        destination_path: Directory where the output file is written.
            Created automatically if absent.
        format: Output format — CSV, Parquet, or JSON Lines.
            Defaults to ``DatasetFormat.PARQUET``.
        partition_by: Column names for directory partitioning.
            Reserved for provider subclasses; unused by the built-in writer.

    Example:
        >>> from michelangelo.workflow.schema.pusher import DatasetFormat
        >>> cfg = LocalFileSinkConfig("/tmp/out", format=DatasetFormat.CSV)
        >>> cfg.destination_path
        '/tmp/out'
    """

    destination_path: str
    format: DatasetFormat = None  # type: ignore[assignment]
    partition_by: list[str] = field(default_factory=list)

    def __post_init__(self) -> None:
        """Apply default format when not provided."""
        if self.format is None:
            from michelangelo.workflow.schema.pusher import DatasetFormat

            self.format = DatasetFormat.PARQUET
