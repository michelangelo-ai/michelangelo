import logging
import os
import uuid
from dataclasses import dataclass
from typing import Optional

import pydantic

from michelangelo.uniflow.core.io_registry import IORegistry
from michelangelo.uniflow.core.utils import (
    dataclass_dict,
    is_dataclass_instance,
    pydantic_dict,
)

log = logging.getLogger(__name__)


@dataclass
class Ref:
    url: str
    type: type
    metadata: Optional[dict] = None


def ref(value, io: IORegistry):
    # Handle None values from tasks without explicit return statements
    if value is None:
        return None

    # If container type - recurse
    if isinstance(value, list):
        return [ref(v, io) for v in value]
    if isinstance(value, tuple):
        res = [ref(v, io) for v in value]
        return tuple(res)
    if isinstance(value, dict):
        return {k: ref(v, io) for k, v in value.items()}
    if is_dataclass_instance(value):
        res = {k: ref(v, io) for k, v in dataclass_dict(value).items()}
        return type(value)(**res)
    if isinstance(value, pydantic.BaseModel):
        res = {k: ref(v, io) for k, v in pydantic_dict(value).items()}
        return type(value)(**res)

    t = type(value)
    if t not in io:
        return value  # t is not a supported container type and is not a custom type, return as is

    # Custom type - write checkpoint: run IO.write and replace the value with Ref dataclass
    ref_url = "/".join([os.environ["UF_STORAGE_URL"], uuid.uuid4().hex])
    metadata = io[t].write(ref_url, value)
    return Ref(
        url=ref_url,
        type=t,
        metadata=metadata,
    )


def unref(value, io: IORegistry):
    # Handle None values explicitly
    if value is None:
        return None
    # If Ref - read checkpoint: run IO.read and replace Ref with the actual value
    if isinstance(value, Ref):
        value_type = value.type
        # when type json is missing "__class__" key, it's not get unmarsheled to the correct type, so we need to add
        # logic to get the path from the metadata and import the module and class
        if isinstance(value_type, dict) and "path" in value_type:
            import importlib

            path = value_type["path"]
            log.debug(f"Resolving type for path: {path}")
            module_name, class_name = path.rsplit(".", 1)  # Split into module and class
            module = importlib.import_module(
                module_name
            )  # Dynamically import the module
            value_type = getattr(module, class_name)
        return io[value_type].read(value.url, value.metadata)

    # If container type - recurse
    if isinstance(value, list):
        return [unref(v, io) for v in value]
    if isinstance(value, tuple):
        res = [unref(v, io) for v in value]
        return tuple(res)
    if isinstance(value, dict):
        return {k: unref(v, io) for k, v in value.items()}
    if is_dataclass_instance(value):
        res = {k: unref(v, io) for k, v in dataclass_dict(value).items()}
        return type(value)(**res)
    if isinstance(value, pydantic.BaseModel):
        res = {k: unref(v, io) for k, v in pydantic_dict(value).items()}
        return type(value)(**res)

    # if value is not Ref and is not any supported container type, return as is
    return value
