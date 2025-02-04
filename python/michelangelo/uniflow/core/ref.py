import logging
import os
import pydantic
from typing import Optional
import uuid
from dataclasses import dataclass

from michelangelo.uniflow.core.io_registry import IORegistry
from michelangelo.uniflow.core.utils import dataclass_dict, is_dataclass_instance, pydantic_dict

log = logging.getLogger(__name__)


@dataclass
class Ref:
    url: str
    type: type
    metadata: Optional[dict] = None


def ref(value, io: IORegistry):
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
    # If Ref - read checkpoint: run IO.read and replace Ref with the actual value
    if isinstance(value, Ref):
        return io[value.type].read(value.url, value.metadata)

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
