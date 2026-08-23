"""Incremental-training support for the native transform task.

Handles loading a base run's fitted transform artifacts and selectively
merging them with a new config spec for a refit.

``_private/`` convention: this file lives in ``_private/`` — do not import
directly from this path. Import from
``michelangelo.workflow.tasks.tabular_native_transform`` instead.
"""

from __future__ import annotations

import logging
import tempfile
from pathlib import Path
from typing import TYPE_CHECKING

import yaml

from michelangelo.lib.native_transform.torch.transform_layer_spec import (
    TorchTransformLayerSpec,
    TransformerMode,
)
from michelangelo.lib.native_transform.torch.transform_spec import TransformSpec
from michelangelo.uniflow.plugins.ray.native_transform import get_numerical_stats_names
from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.schema.tabular_native_transform import (
    IncrementalTrainingConfig,
    TrainingType,
)

if TYPE_CHECKING:
    from michelangelo.lib.artifact_manager.storage_backend import StorageBackend

_logger = logging.getLogger(__name__)

__all__ = [
    "is_baseline",
    "is_incremental",
    "load_incremental_artifacts",
    "merge_specs_for_selective_refit",
]

_TRANSFORM_SPEC_FILE = "transform_spec.yaml"
_FEATURE_STATS_FILE = "transform_feature_stats.yaml"

_PLACEHOLDER_TO_RESOLVED = {
    cls.__name__: cls._resolved_layer_type
    for cls in TorchTransformLayerSpec.__subclasses__()
    if cls._resolved_layer_type is not None
}


def is_incremental(config: IncrementalTrainingConfig | None) -> bool:
    """Check whether *config* specifies ``TrainingType.INCREMENTAL`` mode.

    Args:
        config: The incremental training configuration, or ``None``.

    Returns:
        ``True`` if incremental (refit-from-baseline) mode is active.
    """
    return config is not None and config.training_type == TrainingType.INCREMENTAL


def is_baseline(config: IncrementalTrainingConfig | None) -> bool:
    """Check whether *config* specifies ``TrainingType.BASE`` mode.

    Args:
        config: The incremental training configuration, or ``None``.

    Returns:
        ``True`` if this run is a base run for a future incremental run.
    """
    return config is not None and config.training_type == TrainingType.BASE


def load_incremental_artifacts(
    config: IncrementalTrainingConfig,
    storage_backend: StorageBackend,
) -> tuple[TransformSpec, dict]:
    """Download the base run's artifacts and load its fitted transform state.

    Steps:

    1. Download ``config.baseline_model_uri`` via ``storage_backend`` to a
       temporary directory.
    2. Read ``transform_spec.yaml`` and ``transform_feature_stats.yaml`` from
       that directory.
    3. Reconstruct a ``TransformSpec`` from the loaded spec dict.
    4. When ``config.enforce_full_reuse`` is set, validate every layer uses
       ``TransformerMode.REUSE`` (or the default ``INVALID``, which behaves
       as ``REUSE``).

    Args:
        config: The incremental training configuration. Must specify
            ``training_type == TrainingType.INCREMENTAL`` and a non-``None``
            ``baseline_model_uri``.
        storage_backend: Backend used to download the base run's artifacts.

    Returns:
        A ``(transform_spec, feature_stats)`` tuple, where ``feature_stats``
        is ``{}`` when the base run recorded none.

    Raises:
        ConfigurationError: If ``config.baseline_model_uri`` is unset, the
            downloaded artifacts have no ``transform_spec.yaml``, or
            ``enforce_full_reuse`` is set and a layer's mode is not
            ``REUSE``/``INVALID``.
    """
    if not config.baseline_model_uri:
        raise ConfigurationError(
            "INCREMENTAL mode requires incremental_training.baseline_model_uri "
            "to be set to the base run's raw model artifact URI."
        )

    _logger.info(
        "INCREMENTAL mode: downloading base run artifacts from %s",
        config.baseline_model_uri,
    )
    with tempfile.TemporaryDirectory() as tmp_dir:
        storage_backend.download(config.baseline_model_uri, tmp_dir)
        local_dir = Path(tmp_dir)

        transform_spec_path = local_dir / _TRANSFORM_SPEC_FILE
        if not transform_spec_path.exists():
            raise ConfigurationError(
                f"Base run artifacts at {config.baseline_model_uri!r} do not contain "
                f"{_TRANSFORM_SPEC_FILE}. Was it produced by a TrainingType.BASE run?"
            )
        with open(transform_spec_path) as f:
            base_spec_dict = yaml.safe_load(f)

        feature_stats_path = local_dir / _FEATURE_STATS_FILE
        if feature_stats_path.exists():
            with open(feature_stats_path) as f:
                feature_stats = yaml.safe_load(f)
        else:
            feature_stats = {}

    _logger.info(
        "Loaded base transform spec (%d layers) and %d feature stats from base run "
        "artifacts",
        len(base_spec_dict.get("transform_specs", [])),
        len(feature_stats),
    )

    transform_spec = TransformSpec(raw_transform_specs={"transform_specs": []})
    transform_spec.load_from_dict(base_spec_dict)

    if config.enforce_full_reuse:
        for layer_spec in transform_spec.transform_specs.values():
            if layer_spec.mode not in (TransformerMode.INVALID, TransformerMode.REUSE):
                raise ConfigurationError(
                    f"enforce_full_reuse=True but layer {layer_spec.name!r} has "
                    f"mode={layer_spec.mode.value}. All layers must use REUSE (or "
                    "INVALID, which defaults to REUSE) when enforce_full_reuse is "
                    "enabled."
                )

    return transform_spec, feature_stats


def _stats_keys_for_refit(transform_spec: TransformSpec) -> set[str]:
    """Compute the feature-stats keys that must be removed for REFIT columns."""
    keys: set[str] = set()
    for level in set(transform_spec.transform_levels.values()):
        specs = transform_spec.get_numerical_statistics_computation_specs(level)
        keys.update(get_numerical_stats_names(specs))
    _logger.info(
        "Identified %d feature stats keys to remove for REFIT layers: %r",
        len(keys),
        keys,
    )
    return keys


def _layer_key(
    layer_spec: TorchTransformLayerSpec,
) -> tuple[str, tuple[str, ...], tuple[str, ...]]:
    """Build a matching key from layer type, input columns, and output columns.

    Placeholder types are normalized to their resolved counterparts so that
    a config spec with e.g. ``StandardScalerLayerSpec`` matches a base
    model's fitted ``NormalizationLayerSpec``.
    """
    type_name = _PLACEHOLDER_TO_RESOLVED.get(
        type(layer_spec).__name__, type(layer_spec).__name__
    )
    return (type_name, tuple(layer_spec.input_cols), tuple(layer_spec.output_cols))


def merge_specs_for_selective_refit(
    base_spec: TransformSpec,
    config_spec: TransformSpec,
    base_feature_stats: dict,
) -> tuple[TransformSpec, dict]:
    """Merge a base spec (concrete layers) with a config spec (placeholders) for refit.

    For each layer in the config spec with ``mode=REFIT``, the config's
    placeholder layer replaces the base spec's concrete layer. All other
    layers keep the base spec's fitted values. Stats for REFIT layers' input
    columns are removed from ``feature_stats`` so the pipeline recomputes
    them.

    Args:
        base_spec: The base run's fitted ``TransformSpec``, mutated and
            returned in place.
        config_spec: The current run's config spec, whose ``REFIT``-mode
            layers override the corresponding base layers.
        base_feature_stats: The base run's feature statistics.

    Returns:
        A ``(merged_transform_spec, filtered_feature_stats)`` tuple.

    Raises:
        ConfigurationError: If a non-``REFIT`` layer in ``config_spec`` does
            not match any layer in ``base_spec`` by type, input columns, and
            output columns.
    """
    refit_lookup = {}
    for layer_spec in config_spec.transform_specs.values():
        if layer_spec.mode == TransformerMode.REFIT:
            refit_lookup[_layer_key(layer_spec)] = layer_spec
    _logger.info(
        "%d/%d REFIT layers in config spec: %r",
        len(refit_lookup),
        len(config_spec.transform_specs),
        refit_lookup,
    )

    base_keys = {_layer_key(ls) for ls in base_spec.transform_specs.values()}
    for layer_spec in config_spec.transform_specs.values():
        if (
            layer_spec.mode != TransformerMode.REFIT
            and _layer_key(layer_spec) not in base_keys
        ):
            raise ConfigurationError(
                f"Layer {layer_spec.name!r} (type={type(layer_spec).__name__}, "
                f"input_cols={layer_spec.input_cols}, "
                f"output_cols={layer_spec.output_cols}) has mode=REUSE but does "
                "not exist in the base run. REUSE layers must match the base "
                "run's transform spec by type, input_cols, and output_cols."
            )

    merged_layers: dict[str, TorchTransformLayerSpec] = {}
    for layer_spec in base_spec.transform_specs.values():
        key = _layer_key(layer_spec)
        if key in refit_lookup:
            refit_layer = refit_lookup[key]
            refit_layer.mode = TransformerMode.REFIT
            merged_layers[refit_layer.name] = refit_layer
            _logger.info(
                "REFIT layer: %r (input_cols=%r)",
                refit_layer.name,
                refit_layer.input_cols,
            )
        else:
            layer_spec.mode = TransformerMode.REUSE
            merged_layers[layer_spec.name] = layer_spec
            _logger.info(
                "REUSE layer: %r (input_cols=%r)",
                layer_spec.name,
                layer_spec.input_cols,
            )

    base_spec.transform_specs = merged_layers
    base_spec.transform_levels = base_spec._topological_sort()

    refit_output_cols: set[str] = set()
    for layer_spec in merged_layers.values():
        if layer_spec.mode == TransformerMode.REFIT:
            refit_output_cols.update(layer_spec.output_cols)
    for layer_spec in merged_layers.values():
        if layer_spec.mode == TransformerMode.REUSE:
            overlap = set(layer_spec.input_cols) & refit_output_cols
            if overlap:
                _logger.warning(
                    "REUSE layer %r consumes columns %r produced by a REFIT layer. "
                    "The REUSE layer will use base run values while its input "
                    "columns are refitted; verify this is intentional.",
                    layer_spec.name,
                    overlap,
                )

    refit_stats_keys = _stats_keys_for_refit(base_spec)
    filtered_stats = {
        k: v for k, v in base_feature_stats.items() if k not in refit_stats_keys
    }
    _logger.info(
        "Selective refit: %d REFIT layers, %d REUSE layers, removed %d stats entries",
        len(refit_lookup),
        len(merged_layers) - len(refit_lookup),
        len(base_feature_stats) - len(filtered_stats),
    )

    return base_spec, filtered_stats
