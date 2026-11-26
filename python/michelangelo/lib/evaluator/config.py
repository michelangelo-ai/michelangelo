"""Load an :class:`EvaluatorConfig` from a plain dict or a YAML file.

Pipelines carry evaluator configuration as a proto ``Struct``, which arrives
here as ordinary Python dicts. These loaders turn that into the typed schema
objects the rest of the evaluator works with.
"""

from __future__ import annotations

from typing import Any

from michelangelo.workflow.schema.evaluator import (
    ColumnMapping,
    EmbeddingProjection,
    EvaluatorConfig,
    MetricConfig,
    SanityCheck,
)
from michelangelo.workflow.schema.exceptions import ConfigurationError

__all__ = ["load_config_from_dict", "load_config_from_yaml"]

# Top-level keys passed through to EvaluatorConfig as they are.
_SCALAR_FIELDS = (
    "segment_columns",
    "enable_global_segment_evaluation",
    "min_segment_row_count",
    "data_preprocessor",
    "segment_shuffle_partitions",
)
_KNOWN_FIELDS = {
    "metrics",
    "sanity_checks",
    "embedding_projections",
    *_SCALAR_FIELDS,
}


def _build(cls: type, entry: dict[str, Any], key: str, index: int) -> Any:
    """Construct one ``cls`` from a config entry, naming it on a bad key."""
    try:
        return cls(**entry)
    except TypeError as err:
        raise ConfigurationError(f"{key}[{index}]: {err}") from err


def load_config_from_dict(config_dict: dict[str, Any]) -> EvaluatorConfig:
    """Build an :class:`EvaluatorConfig` from a nested dict.

    Args:
        config_dict: Mapping with a ``metrics`` list. Each entry needs ``name``
            and ``metric``; ``params``, ``columns``, and ``filter_expr`` are
            optional. ``sanity_checks`` and ``embedding_projections`` are lists
            of mappings with the fields of :class:`SanityCheck` and
            :class:`EmbeddingProjection`. ``segment_columns``,
            ``enable_global_segment_evaluation``, ``min_segment_row_count``,
            ``data_preprocessor`` and ``segment_shuffle_partitions`` are passed
            through as they are.

    Returns:
        The parsed configuration.

    Raises:
        ConfigurationError: If the mapping has a key this loader does not know
            (including the legacy ``steps``, which must be built as
            :class:`EvaluatorConfig` objects directly), or an entry has an
            unknown or missing field.
    """
    unknown = sorted(set(config_dict) - _KNOWN_FIELDS)
    if unknown:
        raise ConfigurationError(
            f"unknown evaluator config key(s): {', '.join(unknown)}. "
            f"Known keys: {', '.join(sorted(_KNOWN_FIELDS))}."
        )

    metrics = []
    for metric_dict in config_dict.get("metrics", []):
        columns = None
        if "columns" in metric_dict:
            col_dict = metric_dict["columns"]
            columns = ColumnMapping(
                prediction_col=col_dict["prediction_col"],
                target_col=col_dict["target_col"],
                index_col=col_dict.get("index_col"),
                sample_weight_col=col_dict.get("sample_weight_col"),
                extra_cols=col_dict.get("extra_cols"),
            )

        metrics.append(
            MetricConfig(
                name=metric_dict["name"],
                metric=metric_dict["metric"],
                params=metric_dict.get("params", {}),
                columns=columns,
                filter_expr=metric_dict.get("filter_expr"),
            )
        )

    sanity_checks = [
        _build(SanityCheck, entry, "sanity_checks", i)
        for i, entry in enumerate(config_dict.get("sanity_checks", []))
    ]
    embedding_projections = [
        _build(EmbeddingProjection, entry, "embedding_projections", i)
        for i, entry in enumerate(config_dict.get("embedding_projections", []))
    ]
    scalars = {key: config_dict[key] for key in _SCALAR_FIELDS if key in config_dict}

    return EvaluatorConfig(
        metrics=metrics,
        sanity_checks=sanity_checks,
        embedding_projections=embedding_projections,
        **scalars,
    )


def load_config_from_yaml(yaml_path: str) -> EvaluatorConfig:
    """Build an :class:`EvaluatorConfig` from a YAML file.

    Args:
        yaml_path: Path to a YAML document with the same shape
            :func:`load_config_from_dict` expects.

    Returns:
        The parsed configuration.
    """
    import yaml

    with open(yaml_path) as f:
        config_dict = yaml.safe_load(f)

    return load_config_from_dict(config_dict)
