"""Model package schema reader."""

from __future__ import annotations

import os
from typing import TYPE_CHECKING

import yaml

from michelangelo.lib.model_manager._private.schema.common.serde import dict_to_schema
from michelangelo.lib.model_manager._private.serde.model.deployable_model_schema import (  # noqa: E501
    load_deployable_model_schema,
)

if TYPE_CHECKING:
    from michelangelo.lib.model_manager.schema import ModelSchema

_SCHEMA_PATH = os.path.join("metadata", "schema.yaml")
_CONFIG_FILE = "config.pbtxt"


def load_model_schema(model_path: str) -> ModelSchema | None:
    """Load the model schema stored in a model package.

    ``metadata/schema.yaml`` is read when the package has one: every raw
    package does, and so does a custom deployable package. Otherwise a
    deployable package's schema is read from its ``config.pbtxt``.

    Args:
        model_path: The raw or deployable model package path.

    Returns:
        The package's :class:`ModelSchema`, or ``None`` if the package has
        neither file.

    Raises:
        ValueError: If ``metadata/schema.yaml`` exists but is not a YAML
            mapping.
    """
    schema_path = os.path.join(model_path, _SCHEMA_PATH)
    if not os.path.exists(schema_path):
        if os.path.exists(os.path.join(model_path, _CONFIG_FILE)):
            return load_deployable_model_schema(model_path)
        return None

    with open(schema_path) as f:
        content = yaml.safe_load(f)

    if not isinstance(content, dict):
        raise ValueError(f"{_SCHEMA_PATH} in the model package is not a mapping.")

    for key in ("input_schema", "feature_store_features_schema", "output_schema"):
        content.setdefault(key, [])
    return dict_to_schema(content)
