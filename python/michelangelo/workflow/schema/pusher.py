"""Configuration dataclasses for the pusher workflow task.

These classes are the canonical configuration schema for ``push()`` and its
plugins. Workflow definitions import directly from this module; the task
implementation at ``michelangelo.workflow.tasks.pusher`` re-exports them for
backwards compatibility.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum
from typing import TYPE_CHECKING, Any, ClassVar

from michelangelo.workflow.schema.exceptions import ConfigurationError

if TYPE_CHECKING:
    from michelangelo.workflow.schema.data_sink import DataSink

__all__ = [
    "DatasetFormat",
    "DatasetPluginConfig",
    "EvalReportPluginConfig",
    "ModelPluginConfig",
    "PusherConfig",
    "PusherPluginConfig",
]


class DatasetFormat(Enum):
    """Supported output formats for ``DatasetPusherPlugin``.

    Attributes:
        CSV: Comma-separated values text format.
        PARQUET: Columnar binary format (recommended for large datasets).
        JSON: JSON lines format — one record per line.

    Example:
        >>> DatasetFormat.PARQUET.value
        'parquet'
    """

    CSV = "csv"
    PARQUET = "parquet"
    JSON = "json"


@dataclass
class ModelPluginConfig:
    """Configuration for ``ModelPusherPlugin``.

    Attributes:
        model_name: Name to register the model under in the registry. A
            unique name is generated automatically when ``None``.
        description: Optional human-readable description stored in the
            registry alongside the model.
        extra_metadata: Additional string key-value pairs forwarded to
            the registry at registration time as free-form tags (e.g.
            ``{"team": "pricing", "region": "us-east"}``). These are
            push-time registry labels and are separate from
            ``ModelArtifact.metadata``, which carries typed artifact
            properties (framework, deployable flag, etc.) set by the
            assembler.

    Example:
        >>> cfg = ModelPluginConfig(model_name="boston-xgb")
        >>> cfg.model_name
        'boston-xgb'
        >>> cfg.extra_metadata
        {}
    """

    model_name: str | None = None
    description: str | None = None
    extra_metadata: dict[str, str] = field(default_factory=dict)


@dataclass
class DatasetPluginConfig:
    """Configuration for ``DatasetPusherPlugin``.

    Configure via an explicit sink list or the backwards-compatible shorthand:

        # Shorthand (auto-creates a LocalFileSink):
        DatasetPluginConfig(destination_path="/tmp/out", format=DatasetFormat.CSV)

        # Explicit sinks (preferred):
        DatasetPluginConfig(sinks=[LocalFileSink("/tmp/out", DatasetFormat.CSV)])

        # Multi-sink (write to local file and a remote target simultaneously):
        DatasetPluginConfig(sinks=[LocalFileSink("/tmp/out"), HiveSink("db", "table")])

    Attributes:
        sinks: Ordered list of sinks to write to. All sinks receive the same
            ``DatasetVariable``. Each sink accesses ``variable.value`` in its
            native format — ``LocalFileSink`` checks ``isinstance(pd.DataFrame)``
            and raises ``TypeError`` for non-pandas; ``HiveSink`` accesses
            ``variable.value`` as a native Spark DataFrame (no ``toPandas()``
            collection to the driver).
        destination_path: Convenience shorthand. When ``sinks`` is ``None``
            (not provided) and ``destination_path`` is set, a ``LocalFileSink``
            is auto-created by ``__post_init__``. Passing ``sinks=[]`` explicitly
            disables the auto-create even when ``destination_path`` is set.
        format: Used only with the ``destination_path`` shorthand.
        partition_by: Forwarded to the auto-created ``LocalFileSink``. Ignored
            when ``sinks`` is provided explicitly.

    Example:
        >>> cfg = DatasetPluginConfig(destination_path="/tmp/data")
        >>> cfg.format
        <DatasetFormat.PARQUET: 'parquet'>
    """

    sinks: list[DataSink] | None = None
    destination_path: str | None = None
    format: DatasetFormat = DatasetFormat.PARQUET
    partition_by: list[str] = field(default_factory=list)

    def __post_init__(self) -> None:
        """Auto-create a LocalFileSink from destination_path when sinks is unset."""
        if self.sinks is None:
            if self.destination_path is not None:
                from michelangelo.workflow.schema.data_sink import LocalFileSink

                self.sinks = [
                    LocalFileSink(
                        self.destination_path,
                        format=self.format,
                        partition_by=self.partition_by or None,
                    )
                ]
            else:
                self.sinks = []


@dataclass
class EvalReportPluginConfig:
    """Configuration for ``EvalReportPusherPlugin``.

    Attributes:
        report_name: Name assigned to the evaluation report. A unique
            name is generated automatically when ``None``.
        extra_fields: Additional key-value pairs merged into the report
            document before it is written.

    Example:
        >>> cfg = EvalReportPluginConfig(report_name="run-2026")
        >>> cfg.extra_fields
        {}
    """

    report_name: str | None = None
    extra_fields: dict[str, Any] = field(default_factory=dict)


@dataclass
class PusherPluginConfig:
    """Configuration for a single artifact push within a ``PusherConfig``.

    Exactly one of the typed plugin config fields or the extension pair
    (``plugin_name`` + ``plugin_config``) must be set. Setting zero or
    more than one raises ``ConfigurationError``.

    Attributes:
        name: Artifact identifier matching a key in the ``artifacts``
            dict passed to ``push()``.
        model_plugin: Config for ``ModelPusherPlugin``.
        dataset_plugin: Config for ``DatasetPusherPlugin``.
        eval_report_plugin: Config for ``EvalReportPusherPlugin``.
        plugin_name: Name of a provider-registered plugin. Use together
            with ``plugin_config`` when extending beyond the three
            built-in plugins.
        plugin_config: Raw configuration dict forwarded to the custom
            plugin named by ``plugin_name``.

    Example:
        >>> from michelangelo.workflow.schema.pusher import (
        ...     ModelPluginConfig, PusherPluginConfig
        ... )
        >>> cfg = PusherPluginConfig(
        ...     name="clf",
        ...     model_plugin=ModelPluginConfig(model_name="my-clf"),
        ... )
        >>> cfg.resolved_plugin_name()
        'model_plugin'
    """

    name: str
    model_plugin: ModelPluginConfig | None = None
    dataset_plugin: DatasetPluginConfig | None = None
    eval_report_plugin: EvalReportPluginConfig | None = None
    plugin_name: str | None = None
    plugin_config: dict[str, Any] | None = None

    _BUILTIN_FIELDS: ClassVar[tuple[str, ...]] = (
        "model_plugin",
        "dataset_plugin",
        "eval_report_plugin",
    )

    def __post_init__(self) -> None:
        """Validate that plugin_name and plugin_config are set together."""
        has_name = self.plugin_name is not None
        has_cfg = self.plugin_config is not None
        if has_name != has_cfg:
            raise ConfigurationError(
                f"Artifact '{self.name}': plugin_name and plugin_config must "
                "both be set or both be None."
            )

    def resolved_plugin_name(self) -> str:
        """Return the name of the active plugin.

        Returns:
            Plugin name string — one of the built-in field names
            (e.g. ``"model_plugin"``) or the value of ``plugin_name``
            for provider-registered plugins.

        Raises:
            ConfigurationError: If zero plugins are specified, more than
                one built-in field is set, or both a typed field and
                ``plugin_name`` are set simultaneously.

        Example:
            >>> cfg = PusherPluginConfig(
            ...     name="report",
            ...     eval_report_plugin=EvalReportPluginConfig(),
            ... )
            >>> cfg.resolved_plugin_name()
            'eval_report_plugin'
        """
        active = [f for f in self._BUILTIN_FIELDS if getattr(self, f) is not None]
        if len(active) > 1:
            raise ConfigurationError(
                f"Artifact '{self.name}' has multiple plugin configs set: {active}. "
                "Exactly one must be specified."
            )
        if len(active) == 1:
            if self.plugin_name is not None:
                raise ConfigurationError(
                    f"Artifact '{self.name}' sets both '{active[0]}' and "
                    "'plugin_name'. Use exactly one."
                )
            return active[0]
        if self.plugin_name:
            return self.plugin_name
        raise ConfigurationError(
            f"No plugin specified for artifact '{self.name}'. "
            f"Set one of: {list(self._BUILTIN_FIELDS)}, or set plugin_name."
        )

    def resolved_plugin_config(self) -> Any:
        """Return the typed config for built-in plugins or raw dict for custom ones.

        For built-in plugins, returns the typed config dataclass (e.g.
        ``ModelPluginConfig``). For provider-registered plugins where
        ``plugin_name`` is set to a name that is not a dataclass field,
        returns ``plugin_config`` (the raw dict).

        Returns:
            The active plugin config — a typed dataclass or a ``dict``.

        Raises:
            ConfigurationError: Propagated from ``resolved_plugin_name()``
                if the plugin specification is ambiguous or missing.

        Example:
            >>> raw = {"table": "ml_evals"}
            >>> cfg = PusherPluginConfig(
            ...     name="data",
            ...     plugin_name="hive_plugin",
            ...     plugin_config=raw,
            ... )
            >>> cfg.resolved_plugin_config() is raw
            True
        """
        plugin = self.resolved_plugin_name()
        typed = getattr(self, plugin, None)
        if typed is not None:
            return typed
        if self.plugin_config is not None:
            return self.plugin_config
        raise ConfigurationError(
            f"Artifact '{self.name}': plugin_name='{self.plugin_name}' is set "
            "but plugin_config is None. Provide a config dict via plugin_config."
        )


@dataclass
class PusherConfig:
    """Top-level configuration for a ``push()`` call.

    Attributes:
        items: Ordered list of artifact push configurations. Each item
            maps one artifact name to one plugin.

    Example:
        >>> cfg = PusherConfig(items=[
        ...     PusherPluginConfig(
        ...         name="model",
        ...         model_plugin=ModelPluginConfig(),
        ...     )
        ... ])
        >>> len(cfg.items)
        1
    """

    items: list[PusherPluginConfig] = field(default_factory=list)

    def __post_init__(self) -> None:
        """Validate that artifact names within the config are unique."""
        seen: set[str] = set()
        dupes: list[str] = []
        for n in (item.name for item in self.items):
            if n in seen:
                dupes.append(n)
            seen.add(n)
        if dupes:
            raise ConfigurationError(
                f"Duplicate artifact names in PusherConfig: {sorted(set(dupes))}. "
                "Each artifact name must be unique."
            )
