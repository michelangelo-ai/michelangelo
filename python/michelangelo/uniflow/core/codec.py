"""JSON serialization codec system for Python objects.

This module provides a flexible codec system for encoding and decoding Python objects
to/from JSON. It includes built-in codecs for common Python types including dataclasses,
Pydantic models, enums, type objects, and bytes.

The codec system enables seamless serialization of complex Python objects for storage,
transmission, or inter-process communication in Uniflow workflows.

Example:
    Basic usage with the default codec registry::

        import json
        from dataclasses import dataclass
        from michelangelo.uniflow.core.codec import encoder, decoder

        @dataclass
        class Person:
            name: str
            age: int

        person = Person("Alice", 30)
        json_str = json.dumps(person, cls=type(encoder))
        restored = json.loads(json_str, cls=type(decoder))
"""

import base64
import importlib
import inspect
import logging
from abc import ABC, abstractmethod
from enum import Enum
from json import JSONDecoder, JSONEncoder
from typing import Any, Optional

import pydantic

from michelangelo.uniflow.core.utils import (
    dataclass_dict,
    dot_path,
    import_attribute,
    is_dataclass_instance,
    pydantic_dict,
)

_ATTR_CODEC = "__codec__"

log = logging.getLogger(__name__)


class Codec(ABC):
    """Abstract base class for JSON codecs.

    A codec defines how to encode Python objects to JSON-serializable dictionaries
    and decode them back. Each codec handles a specific type or category of objects.
    """

    @abstractmethod
    def id(self) -> str:
        """Get the unique identifier for this codec.

        Returns:
            A unique string identifier used to tag encoded objects.
        """
        raise NotImplementedError

    @abstractmethod
    def can_encode(self, value: Any) -> bool:
        """Check if this codec can encode the given value.

        Args:
            value: The Python object to check.

        Returns:
            True if this codec can encode the value, False otherwise.
        """
        raise NotImplementedError

    @abstractmethod
    def encode(self, value: Any) -> dict:
        """Encode a Python object to a JSON-serializable dictionary.

        Args:
            value: The Python object to encode. Must satisfy can_encode().

        Returns:
            A dictionary representation of the value.
        """
        raise NotImplementedError

    @abstractmethod
    def decode(self, dct: dict) -> Any:
        """Decode a dictionary back to a Python object.

        Args:
            dct: Dictionary representation created by encode().

        Returns:
            The reconstructed Python object.
        """
        raise NotImplementedError


class DataclassCodec(Codec):
    """Codec for Python dataclass instances.

    Encodes dataclasses by converting them to dictionaries and storing the class
    path for reconstruction. The class must be importable via its module path.

    Example:
        >>> from dataclasses import dataclass
        >>> @dataclass
        ... class Point:
        ...     x: int
        ...     y: int
        >>> codec = DataclassCodec()
        >>> encoded = codec.encode(Point(1, 2))
        >>> decoded = codec.decode(encoded)
    """

    _ATTR_CLASS: str = "__class__"

    def id(self) -> str:
        """Get the codec identifier.

        Returns:
            The string "dataclass".
        """
        return "dataclass"

    def can_encode(self, value: Any) -> bool:
        """Check if value is a dataclass instance.

        Args:
            value: Object to check.

        Returns:
            True if value is a dataclass instance.
        """
        return is_dataclass_instance(value)

    def encode(self, value: Any) -> dict:
        """Encode a dataclass to a dictionary.

        Args:
            value: Dataclass instance to encode.

        Returns:
            Dictionary with dataclass fields and __class__ path.
        """
        assert self.can_encode(value)

        res = dataclass_dict(value)
        assert self._ATTR_CLASS not in res

        res[self._ATTR_CLASS] = dot_path(type(value))
        return res

    def decode(self, dct: dict) -> Any:
        """Decode a dictionary to a dataclass instance.

        Args:
            dct: Dictionary created by encode().

        Returns:
            Reconstructed dataclass instance.
        """
        mod, cls = dct[self._ATTR_CLASS].rsplit(".", 1)
        mod = importlib.import_module(mod)
        cls = getattr(mod, cls)
        del dct[self._ATTR_CLASS]
        return cls(**dct)


# TODO: andrii: Refactor the codec system to enable codec plugins
# TODO: andrii: Move Pydantic codec outside of the core
class PydanticCodec(Codec):
    """Codec for Pydantic BaseModel instances.

    Encodes Pydantic models by converting them to dictionaries and storing the
    model class path for reconstruction. The model class must be importable.

    Example:
        >>> from pydantic import BaseModel
        >>> class User(BaseModel):
        ...     username: str
        ...     email: str
        >>> codec = PydanticCodec()
        >>> encoded = codec.encode(User(username="alice", email="alice@example.com"))
        >>> decoded = codec.decode(encoded)
    """

    _ATTR_CLASS: str = "__class__"

    def id(self) -> str:
        """Get the codec identifier.

        Returns:
            The string "pydantic".
        """
        return "pydantic"

    def can_encode(self, value: pydantic.BaseModel) -> bool:
        """Check if value is a Pydantic model instance.

        Args:
            value: Object to check.

        Returns:
            True if value is a Pydantic BaseModel instance.
        """
        return isinstance(value, pydantic.BaseModel)

    def encode(self, value: pydantic.BaseModel) -> dict:
        """Encode a Pydantic model to a dictionary.

        Args:
            value: Pydantic model instance to encode.

        Returns:
            Dictionary with model fields and __class__ path.
        """
        assert self.can_encode(value)

        res = pydantic_dict(value)
        assert self._ATTR_CLASS not in res

        res[self._ATTR_CLASS] = dot_path(type(value))
        return res

    def decode(self, dct: dict) -> Any:
        """Decode a dictionary to a Pydantic model instance.

        Args:
            dct: Dictionary created by encode().

        Returns:
            Reconstructed Pydantic model instance.
        """
        assert self._ATTR_CLASS in dct
        mod, cls = dct.pop(self._ATTR_CLASS).rsplit(".", 1)
        mod = importlib.import_module(mod)
        cls = getattr(mod, cls)
        return cls(**dct)


class EnumCodec(Codec):
    """Codec for Python Enum instances.

    Encodes enums by storing their name, value, and class path. This allows
    reconstruction of the exact enum member.

    Example:
        >>> from enum import Enum
        >>> class Color(Enum):
        ...     RED = 1
        ...     GREEN = 2
        ...     BLUE = 3
        >>> codec = EnumCodec()
        >>> encoded = codec.encode(Color.RED)
        >>> decoded = codec.decode(encoded)
    """

    _ATTR_CLASS: str = "__class__"

    def id(self) -> str:
        """Get the codec identifier.

        Returns:
            The string "enum".
        """
        return "enum"

    def can_encode(self, value: Any) -> bool:
        """Check if value is an Enum instance.

        Args:
            value: Object to check.

        Returns:
            True if value is an Enum instance.
        """
        return isinstance(value, Enum)

    def encode(self, value: Any) -> dict:
        """Encode an Enum to a dictionary.

        Args:
            value: Enum instance to encode.

        Returns:
            Dictionary with enum name, value, and __class__ path.
        """
        assert self.can_encode(value)
        return {
            "name": value.name,
            "value": value.value,
            self._ATTR_CLASS: dot_path(type(value)),
        }

    def decode(self, dct: dict) -> Any:
        """Decode a dictionary to an Enum instance.

        Args:
            dct: Dictionary created by encode().

        Returns:
            Reconstructed Enum member.
        """
        mod, attr = dct[self._ATTR_CLASS].rsplit(".", 1)
        mod = importlib.import_module(mod)
        attr_v = getattr(mod, attr)
        return attr_v(dct["value"])


class TypeCodec(Codec):
    """Codec for Python type objects (classes and functions).

    Encodes type objects by storing their fully-qualified import path.
    This allows serialization of class and function references.

    JSON representation::

        {
            "path": "path.to.type",
            "__codec__": "type"
        }

    Example:
        >>> codec = TypeCodec()
        >>> encoded = codec.encode(str)
        >>> decoded = codec.decode(encoded)
        >>> decoded is str
        True
    """

    _ATTR_PATH: str = "path"

    def id(self) -> str:
        """Get the codec identifier.

        Returns:
            The string "type".
        """
        return "type"

    def can_encode(self, value: Any) -> bool:
        """Check if value is a class or function.

        Args:
            value: Object to check.

        Returns:
            True if value is a class or function object.
        """
        return inspect.isclass(value) or inspect.isfunction(value)

    def encode(self, value: Any) -> dict:
        """Encode a type object to a dictionary.

        Args:
            value: Class or function to encode.

        Returns:
            Dictionary with the type's import path.
        """
        assert self.can_encode(value)
        return {
            self._ATTR_PATH: dot_path(value),
        }

    def decode(self, dct: dict) -> Any:
        """Decode a dictionary to a type object.

        Args:
            dct: Dictionary created by encode().

        Returns:
            The imported type object.
        """
        return import_attribute(dct[self._ATTR_PATH])


class BytesCodec(Codec):
    """Codec for Python bytes objects.

    Encodes bytes as base64-encoded strings for JSON compatibility.

    Example:
        >>> codec = BytesCodec()
        >>> data = b"Hello, World!"
        >>> encoded = codec.encode(data)
        >>> decoded = codec.decode(encoded)
        >>> decoded == data
        True
    """

    def id(self) -> str:
        """Get the codec identifier.

        Returns:
            The string "bytes".
        """
        return "bytes"

    def can_encode(self, value: Any) -> bool:
        """Check if value is a bytes object.

        Args:
            value: Object to check.

        Returns:
            True if value is a bytes object.
        """
        return isinstance(value, bytes)

    def encode(self, value: Any) -> dict:
        """Encode bytes to a base64-encoded dictionary.

        Args:
            value: Bytes object to encode.

        Returns:
            Dictionary with base64-encoded bytes.
        """
        assert self.can_encode(value)
        return {
            "value": base64.b64encode(value).decode("ascii"),
        }

    def decode(self, dct: dict) -> Any:
        """Decode a base64-encoded dictionary to bytes.

        Args:
            dct: Dictionary created by encode().

        Returns:
            Decoded bytes object.
        """
        return base64.b64decode(dct["value"])


class CodecRegistry:
    """Registry for managing and selecting codecs.

    Maintains a collection of codecs and provides lookup by ID or by value type.
    Ensures that each codec ID is unique and that values match exactly one codec.

    Example:
        >>> registry = CodecRegistry([DataclassCodec(), EnumCodec()])
        >>> codec = registry.find_for_value(MyDataclass())
        >>> encoded = codec.encode(MyDataclass())
    """

    def __init__(self, codecs: list[Codec]):
        """Initialize the codec registry.

        Args:
            codecs: List of codec instances to register.
        """
        self._registry: dict[str, Codec] = {}
        for c in codecs:
            self.register(c)

    def register(self, codec: Codec):
        """Register a new codec.

        Args:
            codec: The codec instance to register.

        Raises:
            AssertionError: If a codec with the same ID is already registered.
        """
        codec_id = codec.id()
        assert codec_id
        assert codec_id not in self._registry, (
            f"codec registry conflict: id={codec_id}, codec1={codec}, "
            f"codec2={self._registry[codec_id]}"
        )
        log.info("register codec: %r", codec)
        self._registry[codec_id] = codec

    def find_for_value(self, value: Any) -> Optional[Codec]:
        """Find a codec that can encode the given value.

        Args:
            value: The value to find a codec for.

        Returns:
            The codec that can encode the value, or None if not found.

        Raises:
            AssertionError: If multiple codecs can encode the value.
        """
        found = None
        for c in self._registry.values():
            if c.can_encode(value):
                assert not found, f"codec conflict: {found}, {c}, value: {value}"
                found = c

        return found

    def find_by_id(self, codec_id: str) -> Optional[Codec]:
        """Find a codec by its ID.

        Args:
            codec_id: The codec identifier.

        Returns:
            The codec with the given ID, or None if not found.
        """
        return self._registry.get(codec_id)

    def ensure_by_id(self, codec_id) -> Codec:
        """Get a codec by ID, raising an error if not found.

        Args:
            codec_id: The codec identifier.

        Returns:
            The codec with the given ID.

        Raises:
            AssertionError: If no codec with the given ID exists.
        """
        codec = self.find_by_id(codec_id)
        assert codec, f"codec not found: id={codec_id}"
        return codec


class Encoder(JSONEncoder):
    """JSON encoder that uses the codec system for custom types.

    Extends JSONEncoder to handle Python objects via registered codecs.
    Objects are encoded with a __codec__ marker for reconstruction.

    Example:
        >>> import json
        >>> registry = CodecRegistry([DataclassCodec()])
        >>> encoder = Encoder(registry)
        >>> json.dumps(my_dataclass, cls=type(encoder))
    """

    def __init__(self, codec_registry: CodecRegistry):
        """Initialize the encoder.

        Args:
            codec_registry: The codec registry to use for encoding.
        """
        super().__init__(separators=(",", ":"))
        self.codec_registry = codec_registry

    def default(self, value):
        """Encode custom Python objects using codecs.

        Args:
            value: The object to encode.

        Returns:
            JSON-serializable representation of the value.
        """
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
    """JSON decoder that uses the codec system for custom types.

    Extends JSONDecoder to reconstruct Python objects from dictionaries
    marked with __codec__ identifiers.

    Example:
        >>> import json
        >>> registry = CodecRegistry([DataclassCodec()])
        >>> decoder = Decoder(registry)
        >>> json.loads(json_str, cls=type(decoder))
    """

    def __init__(self, codec_registry: CodecRegistry):
        """Initialize the decoder.

        Args:
            codec_registry: The codec registry to use for decoding.
        """
        super().__init__(object_hook=self.object_hook)
        self.codec_registry = codec_registry

    def object_hook(self, dct: dict):  # pylint: disable=method-hidden
        """Decode dictionaries with codec markers.

        Args:
            dct: Dictionary potentially containing a __codec__ marker.

        Returns:
            Decoded Python object if __codec__ is present, otherwise the dict.
        """
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
