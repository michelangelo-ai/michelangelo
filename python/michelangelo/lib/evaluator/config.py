"""Load an :class:`EvaluatorConfig` from a plain dict or a YAML file.

Pipelines carry evaluator configuration as a proto ``Struct``, which arrives
here as ordinary Python dicts. These loaders turn that into the typed schema
objects the rest of the evaluator works with.
"""

from __future__ import annotations

from typing import Any

from michelangelo.workflow.schema.evaluator import (
    ColumnMapping,
    EvaluatorConfig,
    MetricConfig,
)

__all__ = ["load_config_from_dict", "load_config_from_yaml"]


def load_config_from_dict(config_dict: dict[str, Any]) -> EvaluatorConfig:
    """Build an :class:`EvaluatorConfig` from a nested dict.

    Args:
        config_dict: Mapping with a ``metrics`` list. Each entry needs ``name``
            and ``metric``; ``params``, ``columns``, and ``filter_expr`` are
            optional.

    Returns:
        The parsed configuration.
    """
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

    return EvaluatorConfig(metrics=metrics)


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
