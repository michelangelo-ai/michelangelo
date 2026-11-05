"""Read the model schema of a deployable package from its ``config.pbtxt``."""

from __future__ import annotations

import os
import re

from michelangelo.lib.model_manager._private.schema.triton.data_type import (
    DATA_TYPE_MAPPING,
)
from michelangelo.lib.model_manager.schema import (
    DataType,
    ModelSchema,
    ModelSchemaItem,
)

_CONFIG_FILE = "config.pbtxt"
_TRITON_TO_DATA_TYPE = {v: k for k, v in DATA_TYPE_MAPPING.items()}
_SECTION_START = r"^\s*{}\s*:\s*\["
_NAME = re.compile(r'name\s*:\s*"([^"]+)"')
_DATA_TYPE = re.compile(r"data_type\s*:\s*TYPE_(\w+)")
_DIMS = re.compile(r"dims\s*:\s*\[([^\]]*)\]")
_OPTIONAL = re.compile(r"optional\s*:\s*true", re.IGNORECASE)


def load_deployable_model_schema(model_path: str) -> ModelSchema:
    """Load the input and output schema from a deployable package.

    The packagers write every input, including feature-store features, to the
    ``input`` section, so they all land in ``input_schema``.

    Args:
        model_path: The deployable model package path.

    Returns:
        The schema. A dims of ``[-1]`` stays ``[-1]``, which is also how the
        packagers write an item that had no shape.

    Raises:
        FileNotFoundError: If the package has no ``config.pbtxt``.
    """
    config_path = os.path.join(model_path, _CONFIG_FILE)
    if not os.path.exists(config_path):
        raise FileNotFoundError(f"{_CONFIG_FILE} not found in {model_path}.")

    with open(config_path) as f:
        content = f.read()
    return ModelSchema(
        input_schema=_parse_items(_section(content, "input")),
        output_schema=_parse_items(_section(content, "output")),
    )


def _section(content: str, key: str) -> str:
    """Return the text between the brackets of ``key: [ ... ]``."""
    match = re.search(_SECTION_START.format(key), content, re.MULTILINE)
    if not match:
        return ""
    depth, start = 1, match.end()
    for i in range(start, len(content)):
        if content[i] == "[":
            depth += 1
        elif content[i] == "]":
            depth -= 1
            if depth == 0:
                return content[start:i]
    return content[start:]


def _parse_items(section: str) -> list[ModelSchemaItem]:
    """Parse the ``{ name, data_type, dims }`` blocks of one section."""
    items = []
    for block in re.findall(r"\{([^{}]*)\}", section):
        name = _NAME.search(block)
        if not name:
            continue
        data_type = _DATA_TYPE.search(block)
        dims = _DIMS.search(block)
        items.append(
            ModelSchemaItem(
                name=name.group(1),
                data_type=(
                    _TRITON_TO_DATA_TYPE.get(data_type.group(1), DataType.UNKNOWN)
                    if data_type
                    else DataType.UNKNOWN
                ),
                shape=(
                    [int(d) for d in dims.group(1).split(",") if d.strip()]
                    if dims
                    else None
                ),
                optional=True if _OPTIONAL.search(block) else None,
            )
        )
    return items
