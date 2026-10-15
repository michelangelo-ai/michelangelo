"""Raw model package schema reader."""

from __future__ import annotations

import os
from typing import TYPE_CHECKING

import yaml

from michelangelo.lib.model_manager._private.schema.common.serde import dict_to_schema

if TYPE_CHECKING:
    from michelangelo.lib.model_manager.schema import ModelSchema

_SCHEMA_PATH = os.path.join("metadata", "schema.yaml")


def load_model_schema(model_path: str) -> ModelSchema | None:
    """Load the model schema stored in a raw model package.

    Args:
        model_path: The raw model package path.

    Returns:
        The package's :class:`ModelSchema`, or ``None`` if the package does not
        carry a schema file.

    Raises:
        ValueError: If the schema file exists but is not a YAML mapping.
    """
    schema_path = os.path.join(model_path, _SCHEMA_PATH)
    if not os.path.exists(schema_path):
        return None

    with open(schema_path) as f:
        content = yaml.safe_load(f)

    if not isinstance(content, dict):
        raise ValueError(f"{_SCHEMA_PATH} in the model package is not a mapping.")

    for key in ("input_schema", "feature_store_features_schema", "output_schema"):
        content.setdefault(key, [])
    return dict_to_schema(content)
