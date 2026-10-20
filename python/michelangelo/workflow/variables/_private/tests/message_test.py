"""Tests for MessageVariable, copied from the internal SDK."""

# ruff: noqa: I001
from michelangelo.workflow.variables._private.message import MessageVariable
from michelangelo.workflow.variables._private.tests.base import BaseVariableTestCase
from michelangelo.workflow.variables.metadata import SourceMessageType

from michelangelo.lib.shared.json_data import JSONData

from google.protobuf.struct_pb2 import Value


class JSONDataValue(JSONData):
    """A JSONData model for tests."""

    string_value: str


class MessageVariableTest(BaseVariableTestCase):
    """Tests for MessageVariable."""

    def test_proto(self):
        """A proto value saves and loads through ProtoIO."""
        value = Value()
        value.string_value = "test"

        variable = MessageVariable.create(value)
        self.assertEqual(
            variable.metadata.class_name, "google.protobuf.struct_pb2.Value"
        )
        self.check_variable(variable, load_func="load_proto")

    def test_json_data(self):
        """A JSONData value saves and loads as JSON."""
        value = JSONDataValue(string_value="test")

        variable = MessageVariable.create(value)
        self.assertEqual(
            variable.metadata.class_name,
            f"{JSONDataValue.__module__}.{JSONDataValue.__name__}",
        )
        self.check_variable(variable, load_func="load_json_data", use_io=False)

    def test_invalid(self):
        """Unsupported values and an INVALID type raise TypeError."""
        value = "invalid"

        with self.assertRaises(TypeError):
            MessageVariable.create(value)

        value = Value()
        variable = MessageVariable.create(value)
        variable.metadata.type = SourceMessageType.INVALID
        with self.assertRaises(TypeError):
            variable.save()

        with self.assertRaises(TypeError):
            variable._load()
