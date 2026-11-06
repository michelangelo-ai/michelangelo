"""Model schema loader."""

from __future__ import annotations

import os
from typing import TYPE_CHECKING

from michelangelo.lib.model_manager._private.serde.model import (
    load_deployable_model_schema,
    load_raw_model_schema,
)

if TYPE_CHECKING:
    from michelangelo.lib.model_manager.schema import ModelSchema


def load_model_schema(model_path: str, submodule: str = "") -> ModelSchema:
    """Load the model schema from a raw or deployable model package.

    A raw package's schema is read from ``metadata/schema.yaml`` and a
    deployable package's from its ``config.pbtxt``. A package with both is read
    as a raw package.

    Args:
        model_path: The model package path.
        submodule: Dotted submodel path, such as ``"transform_module.layers.0"``.
            When given, returns that submodel's schema from
            ``metadata/submodel_schemas.yaml``. Only raw packages have
            submodel schemas.

    Returns:
        The schema of the model or the submodel.

    Raises:
        FileNotFoundError: If ``model_path`` does not exist, or a schema file
            the package needs is missing.
        ValueError: If the path is neither a raw nor a deployable package, or
            ``submodule`` is given for a deployable package.
        KeyError: If ``submodule`` is not in ``submodel_schemas.yaml``.
    """
    if not os.path.exists(model_path):
        raise FileNotFoundError(f"Model path not found: {model_path}")

    is_raw = os.path.exists(os.path.join(model_path, "metadata", "schema.yaml"))
    is_deployable = os.path.exists(os.path.join(model_path, "config.pbtxt"))

    if not is_raw and not is_deployable:
        raise ValueError(
            f"Unknown model package format at {model_path}. Expected either "
            "'metadata/schema.yaml' (raw) or 'config.pbtxt' (deployable)."
        )

    if submodule:
        if is_raw:
            return load_raw_model_schema(model_path, submodule=submodule)
        raise ValueError(
            "submodule= is not supported for deployable model packages. "
            "Submodel schemas are only available for raw packages."
        )

    if is_raw:
        return load_raw_model_schema(model_path)
    return load_deployable_model_schema(model_path)
