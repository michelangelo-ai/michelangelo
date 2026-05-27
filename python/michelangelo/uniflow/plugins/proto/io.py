"""ProtoIO — read/write protobuf Message objects via fsspec.

Serialises a ``google.protobuf.message.Message`` to JSON text on write and
deserialises it back on read. The type is preserved through the metadata dict
returned by ``write()`` and consumed by ``read()``.

The JSON representation (via ``google.protobuf.json_format``) is chosen over
binary wire format for human-readability and forward-compatibility, matching
the pattern used by TFX and MLflow for proto-based metadata.

Example:

.. code-block:: python

    from google.protobuf import struct_pb2

    msg = struct_pb2.Value(string_value="hello")
    io = ProtoIO()
    metadata = io.write("/tmp/msg.json", msg)
    # metadata == {"value_type": "google.protobuf.struct_pb2:Value"}
    restored = io.read("/tmp/msg.json", metadata)
    assert restored == msg
"""

from __future__ import annotations

import importlib
from typing import Any

import fsspec.core
from google.protobuf import json_format
from google.protobuf.message import Message

from michelangelo.uniflow.core.io_registry import IO

_META_VALUE_TYPE = "value_type"

__all__ = ["ProtoIO"]


class ProtoIO(IO[Message]):
    """Read/write protobuf ``Message`` objects as JSON text via fsspec.

    The message type is encoded as a fully-qualified class name string
    (``"module:ClassName"``) in the metadata dict so the value is
    JSON-serializable and survives refactors and cross-process round-trips.

    Raises:
        ValueError: On ``read()`` when ``metadata`` is ``None``, missing
            ``"value_type"``, or contains an unresolvable class name.
            Always pass the dict returned by ``write()``.

    Example:

    .. code-block:: python

        from google.protobuf import struct_pb2

        io = ProtoIO()
        metadata = io.write("/tmp/v.json", struct_pb2.Value(number_value=1.0))
        # metadata == {"value_type": "google.protobuf.struct_pb2:Value"}
        msg = io.read("/tmp/v.json", metadata)
    """

    def write(self, url: str, value: Message) -> dict[str, Any]:
        """Serialise *value* to JSON text at *url*.

        Args:
            url: Destination path or fsspec URL (local, ``s3://``, etc.).
            value: A ``google.protobuf.message.Message`` instance.

        Returns:
            Metadata dict ``{"value_type": "module:ClassName"}`` required by
            :meth:`read` to reconstruct the message. The value is a
            JSON-serializable string, not a class object.
        """
        fs, path = fsspec.core.url_to_fs(url)
        with fs.open(path, "w") as f:
            f.write(json_format.MessageToJson(value))
        cls = type(value)
        return {_META_VALUE_TYPE: f"{cls.__module__}:{cls.__qualname__}"}

    def read(self, url: str, metadata: dict[str, Any] | None) -> Message:
        """Deserialise a protobuf message from JSON text at *url*.

        Args:
            url: Source path or fsspec URL.
            metadata: Dict returned by :meth:`write` containing
                ``"value_type"`` — a ``"module:ClassName"`` string identifying
                the concrete message class. Must not be ``None``; pass the dict
                returned by ``write()``.

        Returns:
            A populated ``google.protobuf.message.Message`` instance.

        Raises:
            ValueError: If *metadata* is ``None``, missing ``"value_type"``,
                or the encoded class cannot be imported.
        """
        if not metadata or _META_VALUE_TYPE not in metadata:
            raise ValueError(
                "ProtoIO.read() requires the metadata dict returned by write(). "
                f"Expected a dict with key '{_META_VALUE_TYPE}', got: {metadata!r}."
            )
        value_type = metadata[_META_VALUE_TYPE]
        try:
            module_name, class_name = value_type.rsplit(":", 1)
            msg_class = getattr(importlib.import_module(module_name), class_name)
        except (ValueError, ImportError, AttributeError) as exc:
            raise ValueError(
                f"ProtoIO.read() could not resolve message class from "
                f"'{value_type}'. Ensure the module is importable in the reading "
                f"process (it may need to be on the Python path)."
            ) from exc
        fs, path = fsspec.core.url_to_fs(url)
        instance = msg_class()
        with fs.open(path, "r") as f:
            json_format.Parse(f.read(), instance)
        return instance
