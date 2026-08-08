"""Imperative helpers that compose transform layers into a tensor map.

An alternate, non-DAG usage path distinct from the main
:class:`~michelangelo.lib.native_transform.torch.transform_spec.TransformSpec` /
:class:`~michelangelo.lib.native_transform.torch.base_transform_module.TorchTransformModule`
machinery: each ``generate_*_transformation`` helper reads a dict of per-feature
specs, applies a transform to entries already present in ``tensor_map``, and
writes the result back into ``tensor_map`` under a derived name.
"""

from __future__ import annotations

import logging

import torch

from michelangelo.lib.native_transform.torch.base_layers import IDHashTokenizer
from michelangelo.lib.native_transform.torch.constants import DEFAULT_TIME_DURATION_UNIT
from michelangelo.lib.native_transform.torch.duration import TimeDuration
from michelangelo.lib.native_transform.torch.scale import ClipAndScale
from michelangelo.lib.native_transform.torch.utils import resolve_torch_dtype

__all__ = [
    "generate_cast_transformation",
    "generate_concatenation_transformation",
    "generate_duration_transformation",
    "generate_idhash_tokenization_transformation",
    "generate_numerical_scaled_transformation",
    "update_output_tensor_map",
]

_logger = logging.getLogger(__name__)


def generate_numerical_scaled_transformation(
    specs: dict, tensor_map: dict[str, torch.Tensor]
) -> None:
    """Clip and scale features already present in ``tensor_map``.

    Args:
        specs: Mapping from feature name to a spec dict with keys
            ``min_value``, ``max_value``, and optionally ``scale_factor``
            (default ``1``) and ``output_type`` (default ``None``). E.g.
            ``{"eta_min_seq": {"min_value": 5, "max_value": 120,
            "scale_factor": 1 / 5, "output_type": "int32"}}``.
        tensor_map: Mapping from feature/output name to tensor, updated in
            place. Features not already present in ``tensor_map`` are
            skipped.

    Returns:
        None. Updates ``tensor_map`` in place with scaled tensors named
        ``scaled_<feature>``.
    """
    for feature, spec in specs.items():
        if feature in tensor_map:
            output_name = f"scaled_{feature}"
            layer = ClipAndScale(
                min_value=spec["min_value"],
                max_value=spec["max_value"],
                scale_factor=spec.get("scale_factor", 1),
                output_type=spec.get("output_type", None),
                input_cols=[feature],
                output_cols=[output_name],
            )
            inputs = {feature: tensor_map[feature]}
            outputs = layer(inputs)
            update_output_tensor_map(outputs[output_name], output_name, tensor_map)


def generate_concatenation_transformation(
    specs: dict, tensor_map: dict[str, torch.Tensor]
) -> None:
    """Concatenate features already present in ``tensor_map``.

    Args:
        specs: Mapping from feature name to a spec dict with keys
            ``input_cols`` (list of column names to concatenate) and
            optionally ``axis`` (default ``1``). E.g.
            ``{"tokenized_derived_carousel_tag": {"input_cols":
            ["tokenized_derived_carousel_tag_0",
            "tokenized_derived_carousel_tag_1"], "axis": 1}}``.
        tensor_map: Mapping from feature/output name to tensor, updated in
            place.

    Returns:
        None. Updates ``tensor_map`` with concatenated tensors named
        ``concatenated_<feature>``.
    """
    for feature, spec in specs.items():
        input_tensors = [tensor_map[col] for col in spec["input_cols"]]
        output_tensor = torch.cat(input_tensors, dim=spec.get("axis", 1))
        output_name = f"concatenated_{feature}"
        update_output_tensor_map(output_tensor, output_name, tensor_map)


def generate_cast_transformation(
    specs: dict, tensor_map: dict[str, torch.Tensor]
) -> None:
    """Cast features already present in ``tensor_map`` to a target dtype.

    Args:
        specs: Mapping from feature name to a spec dict with key ``dtype``
            (default ``torch.float32``), resolved via
            :func:`~michelangelo.lib.native_transform.torch.utils.resolve_torch_dtype`.
            E.g. ``{"did_order": {"dtype": torch.float32}}``.
        tensor_map: Mapping from feature/output name to tensor, updated in
            place. Features not already present in ``tensor_map`` are
            skipped.

    Returns:
        None. Updates ``tensor_map`` with cast tensors named
        ``casted_<feature>``.
    """
    for feature, spec in specs.items():
        if feature in tensor_map:
            input_tensor = tensor_map[feature]
            target_dtype = resolve_torch_dtype(spec.get("dtype", torch.float32))
            output_tensor = input_tensor.to(dtype=target_dtype)
            output_name = f"casted_{feature}"
            update_output_tensor_map(output_tensor, output_name, tensor_map)


def generate_duration_transformation(
    specs: dict, tensor_map: dict[str, torch.Tensor]
) -> None:
    """Compute a time duration between two features already in ``tensor_map``.

    Args:
        specs: Mapping from feature name to a spec dict with keys ``target``
            and ``source`` (column names in ``tensor_map``), and optionally
            ``target_shape``, ``source_shape``, ``unit`` (default
            :data:`~michelangelo.lib.native_transform.torch.constants.DEFAULT_TIME_DURATION_UNIT`),
            ``min_value``, ``max_value``, and ``log_scale``. E.g.
            ``{"duration_epoch_ms": {"target": "current_epoch_ms",
            "target_shape": (-1, 1), "source": "splitted_derived_epoch_seq",
            "source_shape": (-1, 33), "unit": 24 * 60 * 60 * 1000,
            "min_value": 0, "max_value": 365, "log_scale": True}}``.
        tensor_map: Mapping from feature/output name to tensor, updated in
            place.

    Returns:
        None. Updates ``tensor_map`` with duration tensors named
        ``duration_<feature>``.
    """
    for feature, spec in specs.items():
        target_col = spec["target"]
        source_col = spec["source"]
        output_name = f"duration_{feature}"

        layer = TimeDuration(
            unit=spec.get("unit", DEFAULT_TIME_DURATION_UNIT),
            target_shape=spec.get("target_shape", None),
            source_shape=spec.get("source_shape", None),
            min_value=spec.get("min_value", None),
            max_value=spec.get("max_value", None),
            log_scale=spec.get("log_scale", False),
            input_cols=[target_col, source_col],
            output_cols=[output_name],
        )

        inputs = {
            target_col: tensor_map[target_col],
            source_col: tensor_map[source_col],
        }
        outputs = layer(inputs)
        update_output_tensor_map(outputs[output_name], output_name, tensor_map)


def update_output_tensor_map(
    tensor: torch.Tensor, tensor_name: str, tensor_map: dict[str, torch.Tensor]
) -> None:
    """Write a transformed tensor into ``tensor_map`` under ``tensor_name``.

    Args:
        tensor: The tensor produced by a transform.
        tensor_name: The key to store ``tensor`` under.
        tensor_map: Mapping from feature/output name to tensor, updated in
            place.

    Returns:
        None.
    """
    tensor_map[tensor_name] = tensor
    _logger.info("Transformation for feature %s is finished.", tensor_name)


def generate_idhash_tokenization_transformation(
    specs: dict, tensor_map: dict[str, torch.Tensor]
) -> None:
    """Tokenize ID features already in ``tensor_map`` against a vocabulary.

    Maps arbitrary integer IDs to contiguous indices based on the provided
    vocabulary, via
    :class:`~michelangelo.lib.native_transform.torch.base_layers.IDHashTokenizer`.

    Args:
        specs: Mapping from feature name to a spec dict with key
            ``vocabulary`` (list of integer values) and optionally
            ``output_col`` (defaults to ``"tokenized_<feature>"``). E.g.
            ``{"store_id": {"vocabulary": [1001, 2002, 3003, 4004],
            "output_col": "store_token"}}``. A feature with an empty or
            missing ``vocabulary`` is skipped.
        tensor_map: Mapping from feature/output name to tensor, updated in
            place. Features not already present in ``tensor_map`` are
            skipped.

    Returns:
        None. Updates ``tensor_map`` with tokenized tensors, named
        ``tokenized_<feature>`` unless ``output_col`` is given.

    Example:
        >>> tensor_map = {"store_id": torch.tensor([1001, 9999, 2002])}
        >>> specs = {"store_id": {"vocabulary": [1001, 2002, 3003],
        ...                       "output_col": "store_token"}}
        >>> generate_idhash_tokenization_transformation(specs, tensor_map)
        >>> tensor_map["store_token"]
        tensor([0, 3, 1])
    """
    for feature, spec in specs.items():
        if feature in tensor_map:
            # Allow the caller to specify output_col; otherwise use the
            # default naming convention.
            output_name = spec.get("output_col", f"tokenized_{feature}")
            vocabulary = spec.get("vocabulary")

            if not vocabulary:
                _logger.warning(
                    "Empty vocabulary provided for feature %s, skipping tokenization.",
                    feature,
                )
                continue

            layer = IDHashTokenizer(
                input_cols=[feature],
                output_cols=[output_name],
                vocabulary=vocabulary,
            )

            inputs = {feature: tensor_map[feature]}
            outputs = layer(inputs)
            update_output_tensor_map(outputs[output_name], output_name, tensor_map)
