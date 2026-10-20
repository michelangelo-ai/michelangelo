"""MessageVariable, copied from the internal SDK."""

# ruff: noqa: I001
from dataclasses import dataclass

from michelangelo.workflow.variables.metadata import MessageMetadata, SourceMessageType
from michelangelo.lib.shared.json_data import JSONData
from michelangelo.lib.shared.utils.class_utils import get_full_class_name

from michelangelo.uniflow.core.utils import import_attribute
from michelangelo.uniflow.plugins.proto.io import ProtoIO

from google.protobuf.message import Message

from .base import Variable

import fsspec


@dataclass
class MessageVariable(Variable):
    """Represents a piece of message."""

    @classmethod
    def create(cls, value) -> "MessageVariable":
        """A factory method to create a message variable with the given value."""
        res = super().create(value)
        res.metadata = MessageMetadata()
        res.metadata.class_name = get_full_class_name(value)

        if isinstance(value, Message):
            res.metadata.type = SourceMessageType.PROTO
        elif isinstance(value, JSONData):
            res.metadata.type = SourceMessageType.JSON_DATA
        else:
            raise TypeError(f"Unsupported message type: {type(value)}")

        return res

    def _load(self):
        if self.metadata.type == SourceMessageType.PROTO:
            self.load_proto()
        elif self.metadata.type == SourceMessageType.JSON_DATA:
            self.load_json_data()
        else:
            raise TypeError(f"Unsupported message type: {self.metadata.type}")

    def load_proto(self):
        """Load a protobuf value."""
        self._load_value_using_io(ProtoIO)

    def load_json_data(self):
        """Load a JSONData value."""
        fs, path = fsspec.core.url_to_fs(self.path)
        with fs.open(path, "r") as f:
            json = f.read()
        clazz = import_attribute(self.metadata.class_name)
        self._value = clazz.parse_raw(json)
        self._saved = True

    def save(self):
        """Save the value."""
        if self.metadata.type == SourceMessageType.PROTO:
            self.save_proto()
        elif self.metadata.type == SourceMessageType.JSON_DATA:
            self.save_json_data()
        else:
            raise TypeError(f"Unsupported message type: {self.metadata.type}")

    def save_proto(self):
        """Save a protobuf value."""
        self._save_value_using_io(ProtoIO)

    def save_json_data(self):
        """Save a JSONData value."""
        fs, path = fsspec.core.url_to_fs(self.path)
        with fs.open(path, "w") as f:
            f.write(self.value.model_dump_json())
        self._saved = True
