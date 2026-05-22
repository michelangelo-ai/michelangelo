"""Config dataclass for HiveSink."""

from __future__ import annotations

from dataclasses import dataclass, field


@dataclass
class HiveSinkConfig:
    """Typed configuration for ``HiveSink``.

    Mirrors the internal ``HiveSink(JSONData)`` schema — all sink parameters
    are declared here as typed fields so they can be validated at pipeline
    definition time, serialised, and inspected by the workflow engine.

    Attributes:
        database: Hive database name.
        table: Hive table name.
        mode: Write mode passed to ``spark.write.mode()``.
            One of ``"overwrite"``, ``"append"``, ``"ignore"``, ``"error"``.
        partition_by: Column names used for Hive partition keys.

    Example:
        >>> cfg = HiveSinkConfig(database="ml", table="predictions")
        >>> cfg.mode
        'overwrite'
    """

    database: str
    table: str
    mode: str = "overwrite"
    partition_by: list[str] = field(default_factory=list)

    def __post_init__(self) -> None:
        """Validate mode on construction."""
        _valid_modes = {"overwrite", "append", "ignore", "error"}
        if self.mode not in _valid_modes:
            raise ValueError(
                f"Invalid mode {self.mode!r}. Must be one of {sorted(_valid_modes)}."
            )
