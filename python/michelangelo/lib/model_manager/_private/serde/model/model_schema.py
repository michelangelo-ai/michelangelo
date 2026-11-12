"""Read model schemas from raw and deployable model packages."""

from __future__ import annotations

import os
import re

import yaml

from michelangelo.lib.model_manager._private.schema.common.serde import dict_to_schema
from michelangelo.lib.model_manager._private.schema.triton.data_type import (
    DATA_TYPE_MAPPING,
)
from michelangelo.lib.model_manager.schema import (
    DataType,
    ModelSchema,
    ModelSchemaItem,
)

# Triton data type name (``TYPE_FP32``) back to the schema data type.
_TRITON_TYPE_TO_DATA_TYPE = {f"TYPE_{v}": k for k, v in DATA_TYPE_MAPPING.items()}

_ITEM_PATTERN = re.compile(
    r'\{[^}]*?name\s*:\s*"([^"]+)"[^}]*?data_type\s*:\s*(\w+)'
    r"[^}]*?dims\s*:\s*\[\s*([^\]]*?)\s*\]",
    re.DOTALL,
)


def load_raw_model_schema(model_path: str, submodule: str = "") -> ModelSchema:
    """Load the model schema from a raw model package.

    Args:
        model_path: The raw model package path.
        submodule: Dotted submodel path, such as ``"transform_module.layers.0"``.
            When given, the submodel's schema is read from
            ``metadata/submodel_schemas.yaml``. When empty, the top-level schema
            is read from ``metadata/schema.yaml``.

    Returns:
        The schema of the model or the submodel.

    Raises:
        FileNotFoundError: If the schema file does not exist.
        KeyError: If ``submodule`` is not in ``submodel_schemas.yaml``.
    """
    if submodule:
        schemas_yaml = os.path.join(model_path, "metadata", "submodel_schemas.yaml")
        with open(schemas_yaml) as f:
            data = yaml.safe_load(f) or {}
        if submodule not in data:
            raise KeyError(f"Submodel '{submodule}' not found in {schemas_yaml}")
        return dict_to_schema(data[submodule])

    with open(os.path.join(model_path, "metadata", "schema.yaml")) as f:
        return dict_to_schema(yaml.safe_load(f))


def load_deployable_model_schema(model_path: str) -> ModelSchema:
    """Load the input and output schema from a deployable package's ``config.pbtxt``.

    The packagers write every input, including feature-store features, to the
    ``input`` section, so they all land in ``input_schema``.

    Args:
        model_path: The deployable model package path.

    Returns:
        The schema. An item needs a name, a data type and dims to be read; a
        data type with no schema equivalent is read as ``FLOAT``.

    Raises:
        FileNotFoundError: If the package has no ``config.pbtxt``.
    """
    config_path = os.path.join(model_path, "config.pbtxt")
    if not os.path.exists(config_path):
        raise FileNotFoundError(f"config.pbtxt not found in {model_path}")

    with open(config_path) as f:
        content = f.read()
    return ModelSchema(
        input_schema=_parse_io_section("input", content),
        output_schema=_parse_io_section("output", content),
    )


def _parse_io_section(section_name: str, content: str) -> list[ModelSchemaItem]:
    """Parse the ``input`` or ``output`` section of a ``config.pbtxt``."""
    match = re.search(
        rf"{section_name}\s*:\s*\[((?:[^\[\]]|\[[^\]]*\])*)\]", content, re.DOTALL
    )
    if not match:
        return []

    items = []
    for name, data_type, dims in _ITEM_PATTERN.findall(match.group(1)):
        items.append(
            ModelSchemaItem(
                name=name,
                data_type=_TRITON_TYPE_TO_DATA_TYPE.get(data_type, DataType.FLOAT),
                shape=[int(d) for d in dims.split(",") if d.strip()],
            )
        )
    return items
