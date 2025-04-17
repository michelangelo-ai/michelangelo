import importlib
import inspect
import logging
from abc import ABC, abstractmethod
from json import JSONEncoder, JSONDecoder
from typing import Any, Optional
from enum import Enum
import pydantic
import base64

from michelangelo.uniflow.core.utils import (
    dot_path,
    dataclass_dict,
    import_attribute,
    is_dataclass_instance,
    pydantic_dict,
)

_ATTR_CODEC = "__codec__"

log = logging.getLogger(__name__)


class Codec(ABC):
    @abstractmethod
    def id(self) -> str:
        raise NotImplementedError

    @abstractmethod
    def can_encode(self, value: Any) -> bool:
        raise NotImplementedError

    @abstractmethod
    def encode(self, value: Any) -> dict:
        raise NotImplementedError

    @abstractmethod
    def decode(self, dct: dict) -> Any:
        raise NotImplementedError


class DataclassCodec(Codec):
    _ATTR_CLASS: str = "__class__"

    def id(self) -> str:
        return "dataclass"

    def can_encode(self, value: Any) -> bool:
        return is_dataclass_instance(value)

    def encode(self, value: Any) -> dict:
        assert self.can_encode(value)

        res = dataclass_dict(value)
        assert self._ATTR_CLASS not in res

        res[self._ATTR_CLASS] = dot_path(type(value))
        return res

    def decode(self, dct: dict) -> Any:
        mod, cls = dct[self._ATTR_CLASS].rsplit(".", 1)
        mod = importlib.import_module(mod)
        cls = getattr(mod, cls)
        del dct[self._ATTR_CLASS]
        return cls(**dct)


# TODO: andrii: Refactor the codec system to enable codec plugins
# TODO: andrii: Move Pydantic codec outside of the core
class PydanticCodec(Codec):
    _ATTR_CLASS: str = "__class__"

    def id(self) -> str:
        return "pydantic"

    def can_encode(self, value: pydantic.BaseModel) -> bool:
        return isinstance(value, pydantic.BaseModel)

    def encode(self, value: pydantic.BaseModel) -> dict:
        assert self.can_encode(value)

        res = pydantic_dict(value)
        assert self._ATTR_CLASS not in res

        res[self._ATTR_CLASS] = dot_path(type(value))
        return res

    def decode(self, dct: dict) -> Any:
        assert self._ATTR_CLASS in dct
        mod, cls = dct.pop(self._ATTR_CLASS).rsplit(".", 1)
        mod = importlib.import_module(mod)
        cls = getattr(mod, cls)
        return cls(**dct)


class EnumCodec(Codec):
    _ATTR_CLASS: str = "__class__"

    def id(self) -> str:
        return "enum"

    def can_encode(self, value: Any) -> bool:
        return isinstance(value, Enum)

    def encode(self, value: Any) -> dict:
        assert self.can_encode(value)
        return {
            "name": value.name,
            "value": value.value,
            self._ATTR_CLASS: dot_path(type(value)),
        }

    def decode(self, dct: dict) -> Any:
        mod, attr = dct[self._ATTR_CLASS].rsplit(".", 1)
        mod = importlib.import_module(mod)
        attr_v = getattr(mod, attr)
        return attr_v(dct["value"])


class TypeCodec(Codec):
    """
    JSON codec for Python types, such as classes and functions. JSON representation of the type is as follows:

        {
            "path": "path.to.type",
            "__codec__": "type"
        }
    """

    _ATTR_PATH: str = "path"

    def id(self) -> str:
        return "type"

    def can_encode(self, value: Any) -> bool:
        return inspect.isclass(value) or inspect.isfunction(value)

    def encode(self, value: Any) -> dict:
        assert self.can_encode(value)
        return {
            self._ATTR_PATH: dot_path(value),
        }

    def decode(self, dct: dict) -> Any:
        return import_attribute(dct[self._ATTR_PATH])


class BytesCodec(Codec):
    def id(self) -> str:
        return "bytes"

    def can_encode(self, value: Any) -> bool:
        return isinstance(value, bytes)

    def encode(self, value: Any) -> dict:
        assert self.can_encode(value)
        return {
            "value": base64.b64encode(value).decode("ascii"),
        }

    def decode(self, dct: dict) -> Any:
        return base64.b64decode(dct["value"])


class CodecRegistry:
    def __init__(self, codecs: list[Codec]):
        self._registry: dict[str, Codec] = {}
        for c in codecs:
            self.register(c)

    def register(self, codec: Codec):
        codec_id = codec.id()
        assert codec_id
        assert codec_id not in self._registry, (
            f"codec registry conflict: id={codec_id}, codec1={codec}, codec2={self._registry[codec_id]}"
        )
        log.info("register codec: %r", codec)
        self._registry[codec_id] = codec

    def find_for_value(self, value: Any) -> Optional[Codec]:
        found = None
        for c in self._registry.values():
            if c.can_encode(value):
                assert not found, f"codec conflict: {found}, {c}, value: {value}"
                found = c

        return found

    def find_by_id(self, codec_id: str) -> Optional[Codec]:
        return self._registry.get(codec_id)

    def ensure_by_id(self, codec_id) -> Codec:
        codec = self.find_by_id(codec_id)
        assert codec, f"codec not found: id={codec_id}"
        return codec


class Encoder(JSONEncoder):
    def __init__(self, codec_registry: CodecRegistry):
        super().__init__(separators=(",", ":"))
        self.codec_registry = codec_registry

    def default(self, value):
        codec = self.codec_registry.find_for_value(value)
        if not codec:
            return JSONEncoder.default(self, value)

        log.debug("encode: codec=%s, value=%s", codec.id(), value)
        encoded = codec.encode(value)
        assert _ATTR_CODEC not in encoded, (
            f"invalid codec: {_ATTR_CODEC} is reserved property, {codec.id()}, {codec}"
        )
        encoded[_ATTR_CODEC] = codec.id()
        return encoded


class Decoder(JSONDecoder):
    def __init__(self, codec_registry: CodecRegistry):
        super().__init__(object_hook=self.object_hook)
        self.codec_registry = codec_registry

    def object_hook(self, dct: dict):  # pylint: disable=method-hidden
        if _ATTR_CODEC not in dct:
            return dct

        codec_id = dct.pop(_ATTR_CODEC)
        codec = self.codec_registry.ensure_by_id(codec_id)
        return codec.decode(dct)


codec_registry = CodecRegistry(
    [
        DataclassCodec(),
        PydanticCodec(),
        TypeCodec(),
        EnumCodec(),
        BytesCodec(),
    ]
)

encoder, decoder = Encoder(codec_registry), Decoder(codec_registry)
