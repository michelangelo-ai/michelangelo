"""SinkResult: structured return type for DataSink.write()."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any


@dataclass(frozen=True)
class SinkResult:
    """Structured result returned by ``DataSink.write()``.

    Attributes:
        uri: Canonical location of the written data. An absolute path for
            local sinks; a URI (``s3://...``, ``hive://...``) for remote sinks.
        num_records: Number of rows written.
        extra: Optional sink-specific metadata (partition paths, byte count,
            table name, etc.). Included verbatim in the plugin's result dict.

    Example:
        >>> result = SinkResult(uri="/tmp/data/data.parquet", num_records=3)
        >>> result.num_records
        3
    """

    uri: str
    num_records: int
    extra: dict[str, Any] = field(default_factory=dict)
