"""The ``TransformSpec`` DAG engine.

Bridges the two parallel representations of a native transform: a
declarative, serializable ``*LayerSpec`` (see
:mod:`~michelangelo.lib.native_transform.torch.transform_layer_spec`) and an
executable ``nn.Module`` layer (see
:mod:`~michelangelo.lib.native_transform.torch.base_layers` and
:mod:`~michelangelo.lib.native_transform.torch.stats_layers`).

:class:`TransformSpec` parses a raw dict (typically loaded from YAML) of
layer specs, topologically sorts them into dependency "levels" by their
input/output column references, and exposes per-level accessors used to
drive fit-then-materialize training: compute the statistics a level's
placeholder specs need
(:meth:`TransformSpec.get_numerical_statistics_computation_specs`), hydrate
those placeholders into concrete specs once the statistics are available
(the ``update_*`` methods), then materialize the level as executable layers
(:meth:`TransformSpec.to_transform_layers`).

Dispatch for placeholder hydration and this fitted category detection is by
``isinstance()`` against the spec's real pydantic type. This is a deliberate
change from the internal implementation, which dispatched by parsing a
name-string prefix off ``layer_spec.name`` (the lowercased class name, from
:func:`~michelangelo.lib.native_transform.torch.utils.generate_layer_name`).
Since every spec here is a concrete, statically-known pydantic class,
``isinstance()`` is strictly more robust: it cannot silently break if a
layer's name or ``generate_layer_name()``'s output format ever changes,
which the string-prefix approach could.
"""

from __future__ import annotations

import json
from enum import Enum
from typing import Any

import torch
import yaml

from michelangelo.lib.native_transform.torch.base_layers import (
    CaseWhen,
    Cast,
    Ceil,
    Clip,
    Compare,
    Concatenate,
    Constant,
    Divide,
    Floor,
    IdentityTransform,
    IDHashTokenizer,
    LogTransform,
    PadOrCrop1D,
    Scale,
    Stack,
    Subtract,
    TensorColFillNone,
    Tile,
    TorchTransformBaseLayer,
)
from michelangelo.lib.native_transform.torch.constants import (
    DEFAULT_MIN_MAX_SCALER_NAME,
    DEFAULT_STANDARD_SCALER_NAME,
    TORCH_DTYPE_CLASS_NAME_TO_TORCH_TYPE_MAP,
    TORCH_TYPE_TO_TORCH_DTYPE_CLASS_NAME_MAP,
)
from michelangelo.lib.native_transform.torch.stats_layers import (
    Bucketization,
    MinMax,
    Normalization,
)
from michelangelo.lib.native_transform.torch.transform_layer_spec import (
    BucketizationLayerSpec,
    CaseWhenLayerSpec,
    CastLayerSpec,
    CeilLayerSpec,
    ClipLayerSpec,
    CompareLayerSpec,
    ConcatenateLayerSpec,
    ConstantLayerSpec,
    DivideLayerSpec,
    FloorLayerSpec,
    IdentityTransformLayerSpec,
    IDHashTokenizerLayerSpec,
    LogTransformLayerSpec,
    MinMaxLayerSpec,
    MinMaxScalerLayerSpec,
    NormalizationLayerSpec,
    NumericalStandardTransformLayerSpec,
    PadOrCrop1DLayerSpec,
    PercentileBucketizationLayerSpec,
    ScaleLayerSpec,
    StackLayerSpec,
    StandardScalerLayerSpec,
    SubtractLayerSpec,
    TensorColFillNoneLayerSpec,
    TileLayerSpec,
    TorchTransformLayerSpec,
)

__all__ = [
    "TORCH_TRANSFORM_LAYERS_DICT",
    "TORCH_TRANSFORM_LAYERS_SPECS_DICT",
    "TORCH_TRANSFORM_LAYER_CLASS_NAME_TO_SPEC_CLASS_NAME_DICT",
    "TransformSpec",
]


TORCH_TRANSFORM_LAYERS_DICT: dict[str, type[TorchTransformBaseLayer]] = {
    layer_cls.__name__: layer_cls
    for layer_cls in (
        Concatenate,
        Stack,
        MinMax,
        Normalization,
        Bucketization,
        TensorColFillNone,
        Cast,
        CaseWhen,
        Compare,
        Constant,
        Divide,
        PadOrCrop1D,
        Tile,
        LogTransform,
        Subtract,
        Scale,
        Floor,
        Ceil,
        Clip,
        IDHashTokenizer,
        IdentityTransform,
    )
}
"""Maps a transform layer's class name to the executable layer class."""


TORCH_TRANSFORM_LAYERS_SPECS_DICT: dict[str, type[TorchTransformLayerSpec]] = {
    spec_cls.__name__.replace("LayerSpec", ""): spec_cls
    for spec_cls in (
        ConcatenateLayerSpec,
        StackLayerSpec,
        MinMaxLayerSpec,
        NormalizationLayerSpec,
        NumericalStandardTransformLayerSpec,
        BucketizationLayerSpec,
        PercentileBucketizationLayerSpec,
        TensorColFillNoneLayerSpec,
        CastLayerSpec,
        CaseWhenLayerSpec,
        CompareLayerSpec,
        ConstantLayerSpec,
        DivideLayerSpec,
        PadOrCrop1DLayerSpec,
        TileLayerSpec,
        LogTransformLayerSpec,
        SubtractLayerSpec,
        ScaleLayerSpec,
        FloorLayerSpec,
        CeilLayerSpec,
        ClipLayerSpec,
        IDHashTokenizerLayerSpec,
        IdentityTransformLayerSpec,
    )
}
# Placeholder specs, keyed by the transform name a raw spec dict names them
# with, that get resolved to a concrete spec after stats/vocab hydration.
TORCH_TRANSFORM_LAYERS_SPECS_DICT[DEFAULT_STANDARD_SCALER_NAME] = (
    StandardScalerLayerSpec
)
TORCH_TRANSFORM_LAYERS_SPECS_DICT[DEFAULT_MIN_MAX_SCALER_NAME] = MinMaxScalerLayerSpec
"""Maps a raw spec dict's ``transform_name`` to the pydantic spec class."""


TORCH_TRANSFORM_LAYER_CLASS_NAME_TO_SPEC_CLASS_NAME_DICT: dict[
    str, type[TorchTransformLayerSpec]
] = {
    spec_cls.__name__: spec_cls
    for spec_cls in TORCH_TRANSFORM_LAYERS_SPECS_DICT.values()
}
"""Maps a spec class's own ``__name__`` to itself, for ``to_dict``/``load_from_dict``
round-trips that persist ``layer_spec_class_name`` rather than the transform name."""


class TransformSpec:
    """A declarative DAG of native transform layers.

    Parses a raw dict of layer specs (typically loaded from YAML) into
    :class:`~michelangelo.lib.native_transform.torch.transform_layer_spec.TorchTransformLayerSpec`
    instances, topologically sorts them into dependency "levels" by their
    input/output column references, and exposes accessors used to drive
    fit-then-materialize training and serving.

    Args:
        transform_spec_yaml_path: Path to a YAML file containing the raw
            transform spec dict. Takes precedence over
            ``raw_transform_specs`` when both are provided.
        raw_transform_specs: The raw transform spec dict directly, as an
            alternative to loading it from a YAML file.

    Raises:
        ValueError: If neither ``transform_spec_yaml_path`` nor
            ``raw_transform_specs`` is provided.
    """

    def __init__(
        self,
        transform_spec_yaml_path: str | None = None,
        raw_transform_specs: dict[str, Any] | None = None,
    ) -> None:
        """Initialize the TransformSpec.

        Args:
            transform_spec_yaml_path: Path to a YAML file containing the raw
                transform spec dict. Takes precedence over
                ``raw_transform_specs`` when both are provided.
            raw_transform_specs: The raw transform spec dict directly, as an
                alternative to loading it from a YAML file.

        Raises:
            ValueError: If neither ``transform_spec_yaml_path`` nor
                ``raw_transform_specs`` is provided.
        """
        self.transform_spec_yaml_path = transform_spec_yaml_path
        if transform_spec_yaml_path is None and raw_transform_specs is None:
            raise ValueError(
                "Either transform_spec_yaml_path or raw_transform_specs must be "
                "provided"
            )
        if transform_spec_yaml_path is not None:
            with open(transform_spec_yaml_path) as f:
                raw_transform_specs = yaml.safe_load(f)
        self.transform_specs: dict[str, TorchTransformLayerSpec] = (
            self._generate_transform_specs(raw_transform_specs)
        )
        self.columns_to_keep: list[str] | None = self._generate_columns_to_keep(
            raw_transform_specs
        )
        self.transform_levels: dict[str, int] = self._topological_sort()

    def update_input_dtype(
        self, data_dtype_dict: dict[str, torch.dtype], target_transform_level: int = 0
    ) -> None:
        """Populate ``input_dtype`` on every level-0 spec from a dataset's dtypes.

        Args:
            data_dtype_dict: Mapping from column name to the dtype observed
                in the dataset.
            target_transform_level: Only specs at this level are updated.

        Raises:
            ValueError: If a spec's first input column is not present in
                ``data_dtype_dict``.
        """
        for layer_name, layer_spec in self.transform_specs.items():
            if self.transform_levels[layer_spec.name] != target_transform_level:
                continue
            if layer_spec.input_cols[0] not in data_dtype_dict:
                raise ValueError(
                    f"Input column {layer_spec.input_cols[0]} not found in "
                    "data dtype dict"
                )
            layer_spec.input_dtype = data_dtype_dict.get(
                layer_spec.input_cols[0], torch.float32
            )
            # Regenerate the layer spec so any dtype-dependent validators re-run.
            spec_cls = TORCH_TRANSFORM_LAYERS_SPECS_DICT[
                layer_spec.__class__.__name__.replace("LayerSpec", "")
            ]
            self.transform_specs[layer_name] = spec_cls(**layer_spec.model_dump())

    def update_numerical_standard_transform_parameters(
        self, percentile_dict: dict[str, float], target_transform_level: int = 0
    ) -> None:
        """Resolve percentile-string cap/default values on numerical-transform specs.

        Args:
            percentile_dict: Mapping from ``f"{column}_{percentile}"`` to the
                computed percentile value.
            target_transform_level: Only specs at this level are updated.
        """
        for layer_spec in self.transform_specs.values():
            if self.transform_levels[layer_spec.name] != target_transform_level:
                continue
            if not isinstance(layer_spec, NumericalStandardTransformLayerSpec):
                continue
            input_col = layer_spec.input_cols[0]
            if not isinstance(layer_spec.cap_min, float):
                layer_spec.cap_min = float(
                    percentile_dict.get(
                        f"{input_col}_{int(100 * float(layer_spec.cap_min))!s}", 0.0
                    )
                )
            if not isinstance(layer_spec.cap_max, float):
                layer_spec.cap_max = float(
                    percentile_dict.get(
                        f"{input_col}_{int(100 * float(layer_spec.cap_max))!s}", 1.0
                    )
                )
            if not isinstance(layer_spec.default_value, float):
                layer_spec.default_value = float(
                    percentile_dict.get(
                        f"{input_col}_{int(100 * float(layer_spec.default_value))!s}",
                        -1.0,
                    )
                )

    def update_standard_scaler_specs(
        self, stats_dict: dict[str, float], target_transform_level: int = 0
    ) -> None:
        """Hydrate ``StandardScalerLayerSpec`` into a fitted ``NormalizationLayerSpec``.

        Args:
            stats_dict: Mapping from ``f"{column}_mean"`` / ``f"{column}_std"``
                to the computed statistic.
            target_transform_level: Only specs at this level are hydrated.
        """
        new_layer_specs: dict[str, TorchTransformLayerSpec] = {}
        for layer_spec in self.transform_specs.values():
            if self.transform_levels[
                layer_spec.name
            ] != target_transform_level or not isinstance(
                layer_spec, StandardScalerLayerSpec
            ):
                new_layer_specs[layer_spec.name] = layer_spec
                continue
            input_cols = layer_spec.input_cols
            mean_list = []
            std_list = []
            for input_col in input_cols:
                mean_list.append(stats_dict.get(f"{input_col}_mean", 0.0))
                std_list.append(stats_dict.get(f"{input_col}_std", 1.0))
            new_layer_spec = NormalizationLayerSpec(
                input_dtype=torch.float32,
                input_cols=input_cols,
                output_cols=layer_spec.output_cols,
                mean=mean_list,
                std=std_list,
            )
            new_layer_specs[new_layer_spec.name] = new_layer_spec
        self.transform_specs = new_layer_specs
        self.transform_levels = self._topological_sort()

    def update_min_max_scaler_specs(
        self, stats_dict: dict[str, float], target_transform_level: int = 0
    ) -> None:
        """Hydrate ``MinMaxScalerLayerSpec`` into a fitted ``MinMaxLayerSpec``.

        Args:
            stats_dict: Mapping from ``f"{column}_min"`` / ``f"{column}_max"``
                to the computed statistic.
            target_transform_level: Only specs at this level are hydrated.
        """
        new_layer_specs: dict[str, TorchTransformLayerSpec] = {}
        for layer_spec in self.transform_specs.values():
            if self.transform_levels[
                layer_spec.name
            ] != target_transform_level or not isinstance(
                layer_spec, MinMaxScalerLayerSpec
            ):
                new_layer_specs[layer_spec.name] = layer_spec
                continue
            input_cols = layer_spec.input_cols
            min_list = []
            max_list = []
            for input_col in input_cols:
                min_list.append(stats_dict.get(f"{input_col}_min", 0.0))
                max_list.append(stats_dict.get(f"{input_col}_max", 1.0))
            new_layer_spec = MinMaxLayerSpec(
                input_dtype=torch.float32,
                input_cols=input_cols,
                output_cols=layer_spec.output_cols,
                min=min_list,
                max=max_list,
            )
            new_layer_specs[new_layer_spec.name] = new_layer_spec
        self.transform_specs = new_layer_specs
        self.transform_levels = self._topological_sort()

    def update_bucketization_specs(
        self, stats_dict: dict[str, float], target_transform_level: int = 0
    ) -> None:
        """Hydrate ``PercentileBucketizationLayerSpec`` using computed percentiles.

        Args:
            stats_dict: Mapping from ``f"{column}_{percentile}"`` to the
                computed boundary value.
            target_transform_level: Only specs at this level are hydrated.
        """
        new_layer_specs: dict[str, TorchTransformLayerSpec] = {}
        for layer_spec in self.transform_specs.values():
            if self.transform_levels[
                layer_spec.name
            ] != target_transform_level or not self._is_percentile_bucketization_layer(
                layer_spec
            ):
                new_layer_specs[layer_spec.name] = layer_spec
                continue
            input_col = layer_spec.input_cols[0]
            new_boundaries = []
            for percentile in layer_spec.percentiles:
                percentile_key = f"{input_col}_{int(100 * float(percentile))!s}"
                new_boundaries.append(float(stats_dict.get(percentile_key, 0.0)))
            new_layer_spec = BucketizationLayerSpec(
                input_dtype=layer_spec.input_dtype,
                input_cols=layer_spec.input_cols,
                output_cols=layer_spec.output_cols,
                boundaries=new_boundaries,
                dtype=layer_spec.dtype,
            )
            new_layer_specs[new_layer_spec.name] = new_layer_spec
        self.transform_specs = new_layer_specs
        self.transform_levels = self._topological_sort()

    def to_transform_layers(
        self, target_transform_level: int = 0
    ) -> list[TorchTransformBaseLayer]:
        """Materialize every spec at a level into an executable layer.

        Args:
            target_transform_level: Only specs at this level are materialized.

        Returns:
            The materialized layers, in spec-dict iteration order.

        Raises:
            ValueError: If a spec's layer class is not registered in
                :data:`TORCH_TRANSFORM_LAYERS_DICT` (i.e. it is still an
                unfitted placeholder).
        """
        layers = []
        for layer_spec in self.transform_specs.values():
            if self.transform_levels[layer_spec.name] != target_transform_level:
                continue
            layer_name = layer_spec.__class__.__name__.replace("LayerSpec", "")
            if layer_name not in TORCH_TRANSFORM_LAYERS_DICT:
                raise ValueError(f"Transform layer {layer_name} not found")
            layers.append(
                TORCH_TRANSFORM_LAYERS_DICT[layer_name](**layer_spec.model_dump())
            )
        return layers

    def get_transform_layer_spec(
        self,
        target_transform_level: int | None = None,
        target_transform_type: type[TorchTransformLayerSpec] | None = None,
    ) -> list[TorchTransformLayerSpec]:
        """Look up specs by level and/or spec type.

        Args:
            target_transform_level: Restrict to specs at this level. When
                ``None``, every level is considered.
            target_transform_type: Restrict to specs of this type. When
                ``None``, every type is considered.

        Returns:
            The matching specs.

        Raises:
            ValueError: If no spec matches the given filters.
        """
        valid_layer_specs = []
        for layer_spec in self.transform_specs.values():
            if (
                target_transform_level is None
                or self.transform_levels[layer_spec.name] == target_transform_level
            ) and (
                target_transform_type is None
                or type(layer_spec).__name__ == target_transform_type.__name__
            ):
                valid_layer_specs.append(layer_spec)
        if len(valid_layer_specs) == 0:
            raise ValueError(
                "No transform layer spec found for transform level "
                f"{target_transform_level}"
            )
        return valid_layer_specs

    def get_input_dtype_map(
        self, target_transform_level: int = 0
    ) -> dict[str, torch.dtype]:
        """Get column -> dtype mapping for input columns at the given level.

        Only meaningful after :meth:`update_input_dtype` has been called,
        which populates ``input_dtype`` from the dataset's schema.

        Args:
            target_transform_level: Only specs at this level are considered.

        Returns:
            Mapping from input column name to its resolved dtype.
        """
        dtype_map: dict[str, torch.dtype] = {}
        for layer_spec in self.transform_specs.values():
            if self.transform_levels[layer_spec.name] != target_transform_level:
                continue
            if layer_spec.input_dtype is not None:
                for col in layer_spec.input_cols:
                    if col not in dtype_map:
                        dtype_map[col] = layer_spec.input_dtype
        return dtype_map

    def get_transform_input_cols(self, target_transform_level: int = 0) -> list[str]:
        """Get the sorted set of input columns referenced at a level.

        Args:
            target_transform_level: Only specs at this level are considered.

        Returns:
            The sorted, de-duplicated input column names.
        """
        input_cols = set()
        for layer_spec in self.transform_specs.values():
            if self.transform_levels[layer_spec.name] != target_transform_level:
                continue
            input_cols.update(layer_spec.input_cols)
        return sorted(input_cols)

    def get_transform_output_cols(self, target_transform_level: int = 0) -> list[str]:
        """Get the sorted set of output columns produced at a level.

        Args:
            target_transform_level: Only specs at this level are considered.

        Returns:
            The sorted, de-duplicated output column names.
        """
        output_cols = set()
        for layer_spec in self.transform_specs.values():
            if self.transform_levels[layer_spec.name] != target_transform_level:
                continue
            output_cols.update(layer_spec.output_cols)
        return sorted(output_cols)

    def get_max_transform_level(self) -> int:
        """Get the highest transform level in the DAG.

        Returns:
            The maximum level, or ``0`` if there are no specs.
        """
        return max(self.transform_levels.values()) if self.transform_levels else 0

    def get_numerical_statistics_computation_specs(
        self, target_transform_level: int = 0
    ) -> dict[str, dict[str, Any]]:
        """Get the statistics required to hydrate placeholders at a level.

        Args:
            target_transform_level: Only specs at this level are considered.

        Returns:
            Mapping from input column to a dict with keys ``percentiles``
            (a sorted, de-duplicated list of required percentile values),
            and boolean ``mean``, ``std``, ``min``, ``max`` flags.
        """
        numerical_statistics_computation_specs: dict[str, dict[str, Any]] = {}
        for layer_spec in self.transform_specs.values():
            if self.transform_levels[layer_spec.name] != target_transform_level:
                continue
            self._process_layer_for_statistics(
                layer_spec, numerical_statistics_computation_specs
            )
        self._finalize_numerical_statistics_computation_specs(
            numerical_statistics_computation_specs
        )
        return numerical_statistics_computation_specs

    def _process_layer_for_statistics(
        self,
        layer_spec: TorchTransformLayerSpec,
        statistics_specs: dict[str, dict[str, Any]],
    ) -> None:
        """Dispatch a single spec to its statistics-requirement collector."""
        if isinstance(layer_spec, NumericalStandardTransformLayerSpec):
            self._add_numerical_transform_requirements(layer_spec, statistics_specs)
        elif isinstance(layer_spec, StandardScalerLayerSpec):
            self._add_standard_scaler_requirements(layer_spec, statistics_specs)
        elif isinstance(layer_spec, MinMaxScalerLayerSpec):
            self._add_min_max_scaler_requirements(layer_spec, statistics_specs)
        elif self._is_percentile_bucketization_layer(layer_spec):
            self._add_percentile_bucketization_requirements(
                layer_spec, statistics_specs
            )

    def _is_percentile_bucketization_layer(
        self, layer_spec: TorchTransformLayerSpec
    ) -> bool:
        """Check whether a spec is an unfitted ``PercentileBucketizationLayerSpec``."""
        return isinstance(layer_spec, PercentileBucketizationLayerSpec)

    def _add_numerical_transform_requirements(
        self,
        layer_spec: NumericalStandardTransformLayerSpec,
        statistics_specs: dict[str, dict[str, Any]],
    ) -> None:
        """Add percentile requirements for a numerical-transform spec, if capping."""
        if not layer_spec.is_cap_value:
            return
        input_col = layer_spec.input_cols[0]
        self._ensure_statistics_entry(input_col, statistics_specs)
        for value in (layer_spec.cap_min, layer_spec.cap_max, layer_spec.default_value):
            if isinstance(value, str):
                statistics_specs[input_col]["percentiles"].append(value)

    def _add_standard_scaler_requirements(
        self,
        layer_spec: StandardScalerLayerSpec,
        statistics_specs: dict[str, dict[str, Any]],
    ) -> None:
        """Add mean/std requirements for a standard-scaler placeholder."""
        for input_col in layer_spec.input_cols:
            self._ensure_statistics_entry(input_col, statistics_specs)
            if layer_spec.with_mean:
                statistics_specs[input_col]["mean"] = True
            if layer_spec.with_std:
                statistics_specs[input_col]["std"] = True

    def _add_min_max_scaler_requirements(
        self,
        layer_spec: MinMaxScalerLayerSpec,
        statistics_specs: dict[str, dict[str, Any]],
    ) -> None:
        """Add min/max requirements for a min-max-scaler placeholder."""
        for input_col in layer_spec.input_cols:
            self._ensure_statistics_entry(input_col, statistics_specs)
            statistics_specs[input_col]["min"] = True
            statistics_specs[input_col]["max"] = True

    def _add_percentile_bucketization_requirements(
        self,
        layer_spec: PercentileBucketizationLayerSpec,
        statistics_specs: dict[str, dict[str, Any]],
    ) -> None:
        """Add percentile requirements for a percentile-bucketization placeholder."""
        input_col = layer_spec.input_cols[0]
        self._ensure_statistics_entry(input_col, statistics_specs)
        for percentile in layer_spec.percentiles:
            if percentile <= 0:
                continue
            if percentile < 1:
                statistics_specs[input_col]["percentiles"].append(percentile)
            elif percentile < 100:
                statistics_specs[input_col]["percentiles"].append(percentile / 100)

    def _ensure_statistics_entry(
        self, input_col: str, statistics_specs: dict[str, dict[str, Any]]
    ) -> None:
        """Initialize the statistics-requirement entry for a column, if absent."""
        if input_col not in statistics_specs:
            statistics_specs[input_col] = {
                "percentiles": [],
                "max": False,
                "min": False,
                "std": False,
                "mean": False,
            }

    def _finalize_numerical_statistics_computation_specs(
        self, statistics_specs: dict[str, dict[str, Any]]
    ) -> None:
        """De-duplicate and sort the collected percentile requirements, in place."""
        for stats in statistics_specs.values():
            stats["percentiles"] = sorted(set(stats["percentiles"]))

    def _generate_columns_to_keep(
        self, raw_transform_specs: dict[str, Any]
    ) -> list[str] | None:
        """Validate and return the raw spec dict's ``columns_to_keep``, if any.

        Args:
            raw_transform_specs: The raw transform spec dict.

        Returns:
            The validated ``columns_to_keep`` list, or ``None`` if absent.

        Raises:
            ValueError: If ``columns_to_keep`` is present but is not a
                non-empty list of strings.
        """
        columns_to_keep = raw_transform_specs.get("columns_to_keep")
        if columns_to_keep is not None:
            if not isinstance(columns_to_keep, list):
                raise ValueError("columns_to_keep must be a list")
            if len(columns_to_keep) == 0:
                raise ValueError("columns_to_keep is empty")
            if not all(isinstance(col, str) for col in columns_to_keep):
                raise ValueError("columns_to_keep must be a list of strings")
        return columns_to_keep

    def _generate_transform_specs(
        self, raw_transform_specs: dict[str, Any]
    ) -> dict[str, TorchTransformLayerSpec]:
        """Parse, deduplicate, and validate the raw spec dict's transform specs."""
        transform_specs = self._load_transform_spec(raw_transform_specs)
        filtered_specs = self._validate_transform_specs(transform_specs)
        return {layer_spec.name: layer_spec for layer_spec in filtered_specs}

    def _load_transform_spec(
        self, raw_transform_specs: dict[str, Any]
    ) -> list[TorchTransformLayerSpec]:
        """Parse each raw spec dict entry into its pydantic spec class.

        Raises:
            ValueError: If an entry's ``transform_name`` is not registered in
                :data:`TORCH_TRANSFORM_LAYERS_SPECS_DICT`.
        """
        transform_specs = []
        for transform_layer_spec in raw_transform_specs.get("transform_specs", []):
            layer_spec_name = transform_layer_spec.get("transform_name")
            if layer_spec_name not in TORCH_TRANSFORM_LAYERS_SPECS_DICT:
                raise ValueError(f"Transform layer spec {layer_spec_name} not found")
            transform_specs.append(
                TORCH_TRANSFORM_LAYERS_SPECS_DICT[layer_spec_name](
                    **transform_layer_spec
                )
            )
        return transform_specs

    def _validate_transform_specs(
        self, transform_specs: list[TorchTransformLayerSpec]
    ) -> list[TorchTransformLayerSpec]:
        """De-duplicate specs and validate output-column/name uniqueness.

        Raises:
            ValueError: If an output column is produced by more than one
                layer, or a layer name is duplicated.
        """
        seen_specs = set()
        filtered_specs = []
        for layer_spec in transform_specs:
            spec_key = self._get_transform_spec_key(layer_spec)
            if spec_key in seen_specs:
                continue
            seen_specs.add(spec_key)
            filtered_specs.append(layer_spec)

        output_cols: set[str] = set()
        layer_names: set[str] = set()
        for layer_spec in filtered_specs:
            for output_col in layer_spec.output_cols:
                if output_col in output_cols:
                    raise ValueError(
                        f"Output column {output_col} is produced by multiple layers"
                    )
                output_cols.add(output_col)
            if layer_spec.name in layer_names:
                raise ValueError(f"Layer name {layer_spec.name} is duplicated")
            layer_names.add(layer_spec.name)
        return filtered_specs

    def _topological_sort(self) -> dict[str, int]:
        """Topologically sort specs into dependency levels via column references.

        A spec at index ``i`` depends on any other spec that produces one of
        its input columns; a spec with no such dependency starts at level 0,
        and every other spec's level is one past the maximum level of its
        dependencies.

        Returns:
            Mapping from spec name to its transform level.

        Raises:
            ValueError: If the specs' input/output column references form a
                circular dependency.
        """
        column_to_producer: dict[str, int] = {}
        layer_dependencies: dict[int, set[int]] = {}
        layer_dependents: dict[int, set[int]] = {}

        transform_specs = list(self.transform_specs.values())

        for i, layer_spec in enumerate(transform_specs):
            layer_dependencies[i] = set()
            layer_dependents[i] = set()
            for output_col in layer_spec.output_cols:
                if output_col in column_to_producer:
                    raise ValueError(
                        f"Column '{output_col}' is produced by multiple layers"
                    )
                column_to_producer[output_col] = i

        for i, layer_spec in enumerate(transform_specs):
            for input_col in layer_spec.input_cols:
                # If input_col is not produced by any layer, it is raw data.
                if input_col in column_to_producer:
                    producer_idx = column_to_producer[input_col]
                    layer_dependencies[i].add(producer_idx)
                    layer_dependents[producer_idx].add(i)

        degree_groups: list[list[TorchTransformLayerSpec]] = []
        remaining_layers = set(range(len(transform_specs)))
        processed_layers: set[int] = set()

        while remaining_layers:
            current_group = [
                layer_idx
                for layer_idx in remaining_layers
                if layer_dependencies[layer_idx].issubset(processed_layers)
            ]
            if not current_group:
                remaining_specs = [transform_specs[i] for i in remaining_layers]
                remaining_names = [spec.name for spec in remaining_specs]
                raise ValueError(
                    f"Circular dependency detected among layers: {remaining_names}"
                )

            degree_groups.append([transform_specs[i] for i in current_group])
            for layer_idx in current_group:
                processed_layers.add(layer_idx)
                remaining_layers.remove(layer_idx)

        transform_levels: dict[str, int] = {}
        for i, group in enumerate(degree_groups):
            for layer_spec in group:
                transform_levels[layer_spec.name] = i
        return transform_levels

    def _get_transform_spec_key(self, transform_spec: TorchTransformLayerSpec) -> str:
        """Generate a dedup key for a spec, ignoring its auto-generated name."""
        spec_dict = transform_spec.model_dump()
        spec_dict.pop("name", None)
        sorted_items = sorted(spec_dict.items())
        class_name = transform_spec.__class__.__name__
        return f"{class_name}:{sorted_items!s}"

    def to_dict(self) -> dict[str, Any]:
        """Serialize the DAG to a plain, YAML/JSON-safe dict.

        ``torch.dtype`` fields are serialized via
        :data:`~michelangelo.lib.native_transform.torch.constants.TORCH_TYPE_TO_TORCH_DTYPE_CLASS_NAME_MAP`
        and enum fields via their ``.value``, so the result round-trips
        through both ``json.dumps`` and ``yaml.safe_dump``.

        Returns:
            A dict with ``transform_spec_yaml_path``, ``columns_to_keep``,
            and a ``transform_specs`` list of per-layer dicts (each tagged
            with ``layer_spec_class_name`` for :meth:`load_from_dict`).
        """
        spec_dict: dict[str, Any] = {
            "transform_spec_yaml_path": self.transform_spec_yaml_path,
            "transform_specs": [],
            "columns_to_keep": self.columns_to_keep,
        }
        for layer_name, layer_spec in self.transform_specs.items():
            layer_spec_dict = layer_spec.model_dump()
            if "name" not in layer_spec_dict:
                layer_spec_dict["name"] = layer_name
            layer_spec_dict["layer_spec_class_name"] = layer_spec.__class__.__name__
            for key, value in layer_spec_dict.items():
                if isinstance(value, torch.dtype):
                    layer_spec_dict[key] = TORCH_TYPE_TO_TORCH_DTYPE_CLASS_NAME_MAP[
                        value
                    ]
                elif isinstance(value, Enum):
                    layer_spec_dict[key] = value.value
            spec_dict["transform_specs"].append(layer_spec_dict)
        return spec_dict

    def to_json(self) -> str:
        """Serialize the DAG to a JSON string.

        Returns:
            The JSON serialization of :meth:`to_dict`.
        """
        return json.dumps(self.to_dict())

    def load_from_json(self, json_str: str) -> None:
        """Replace this DAG's state from a JSON string produced by :meth:`to_json`.

        Args:
            json_str: The JSON string to load from.
        """
        self.load_from_dict(json.loads(json_str))

    def load_from_dict(self, spec_dict: dict[str, Any]) -> None:
        """Replace this DAG's state from a dict produced by :meth:`to_dict`.

        Args:
            spec_dict: The dict to load from.

        Raises:
            ValueError: If an entry's ``layer_spec_class_name`` is not
                registered in
                :data:`TORCH_TRANSFORM_LAYER_CLASS_NAME_TO_SPEC_CLASS_NAME_DICT`.
        """
        self.transform_spec_yaml_path = spec_dict["transform_spec_yaml_path"]
        self.columns_to_keep = spec_dict["columns_to_keep"]
        transform_specs = []
        for layer_spec in spec_dict["transform_specs"]:
            for key, value in layer_spec.items():
                if (
                    isinstance(value, str)
                    and value in TORCH_DTYPE_CLASS_NAME_TO_TORCH_TYPE_MAP
                ):
                    layer_spec[key] = TORCH_DTYPE_CLASS_NAME_TO_TORCH_TYPE_MAP[value]
            layer_spec_class_name = layer_spec["layer_spec_class_name"]
            if (
                layer_spec_class_name
                not in TORCH_TRANSFORM_LAYER_CLASS_NAME_TO_SPEC_CLASS_NAME_DICT
            ):
                raise ValueError(
                    f"Layer spec class name {layer_spec_class_name} not found"
                )
            spec_cls = TORCH_TRANSFORM_LAYER_CLASS_NAME_TO_SPEC_CLASS_NAME_DICT[
                layer_spec_class_name
            ]
            transform_specs.append(spec_cls(**layer_spec))
        self.transform_specs = {
            layer_spec.name: layer_spec for layer_spec in transform_specs
        }
        self.transform_levels = self._topological_sort()
