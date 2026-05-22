"""ProtoIO — read/write protobuf Message objects via fsspec.

Serialises a ``google.protobuf.message.Message`` to JSON text on write and
deserialises it back on read. The type is preserved through the metadata dict
returned by ``write()`` and consumed by ``read()``.

Example:
    >>> from google.protobuf import struct_pb2
    >>> msg = struct_pb2.Value(string_value="hello")
    >>> io = ProtoIO()
    >>> io.write("/tmp/msg.json", msg)
    {'value_type': <class '...Value'>}
    >>> io.read("/tmp/msg.json", {'value_type': struct_pb2.Value})
    string_value: "hello"
"""

from __future__ import annotations

from typing import Any

from michelangelo.uniflow.core.io_registry import IO

_META_VALUE_TYPE = "value_type"

__all__ = ["ProtoIO"]


class ProtoIO(IO[Any]):
    """Read/write protobuf ``Message`` objects as JSON text via fsspec.

    Attributes:
        _META_VALUE_TYPE: Key used in the metadata dict to carry the message
            class needed for deserialisation.

    Raises:
        ImportError: If ``google-protobuf`` is not installed.
        KeyError: On ``read()`` when ``metadata`` does not contain
            ``"value_type"``.

    Example:
        >>> from google.protobuf import struct_pb2
        >>> ProtoIO().write("/tmp/v.json", struct_pb2.Value(number_value=1.0))
        {'value_type': <class 'google.protobuf.struct_pb2.Value'>}
    """

    def write(self, url: str, value: Any) -> dict[str, Any]:
        """Serialise *value* to JSON text at *url*.

        Args:
            url: Destination path or fsspec URL (local, ``s3://``, etc.).
            value: A ``google.protobuf.message.Message`` instance.

        Returns:
            Metadata dict ``{"value_type": type(value)}`` required by
            :meth:`read` to reconstruct the message.
        """
        import fsspec.core
        from google.protobuf import json_format

        fs, path = fsspec.core.url_to_fs(url)
        with fs.open(path, "w") as f:
            f.write(json_format.MessageToJson(value))
        return {_META_VALUE_TYPE: type(value)}

    def read(self, url: str, metadata: dict[str, Any]) -> Any:
        """Deserialise a protobuf message from JSON text at *url*.

        Args:
            url: Source path or fsspec URL.
            metadata: Dict returned by :meth:`write` containing
                ``"value_type"`` — the concrete message class.

        Returns:
            A populated ``google.protobuf.message.Message`` instance.
        """
        import fsspec.core
        from google.protobuf import json_format

        fs, path = fsspec.core.url_to_fs(url)
        msg_class = metadata[_META_VALUE_TYPE]
        instance = msg_class()
        with fs.open(path, "r") as f:
            json_format.Parse(f.read(), instance)
        return instance
