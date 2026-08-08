"""Tests for the generic BytesIO serialization helpers."""

import pickle
from io import BytesIO
from unittest import TestCase

from michelangelo.workflow.variables._private.utils.serialization import (
    retrieve_object,
    save_object,
)


class RetrieveObjectTest(TestCase):
    """Tests for ``retrieve_object``."""

    def test_returns_none_when_payload_is_none(self):
        """Returns None when payload is None."""
        self.assertIsNone(retrieve_object(None))

    def test_returns_object_as_is_when_not_bytesio(self):
        """Returns object as-is when not BytesIO."""
        obj = {"key": "value"}
        self.assertIs(retrieve_object(obj), obj)

    def test_decodes_bytesio_to_object(self):
        """Decodes BytesIO to object."""
        buf = BytesIO()
        pickle.dump([1, 2, 3], buf)
        buf.read()  # move position to end
        self.assertEqual(retrieve_object(buf), [1, 2, 3])

    def test_decodes_bytesio_seeks_to_start(self):
        """Decodes BytesIO by seeking to start first."""
        buf = BytesIO()
        pickle.dump({"a": 1}, buf)
        self.assertEqual(retrieve_object(buf), {"a": 1})


class SaveObjectTest(TestCase):
    """Tests for ``save_object``."""

    def test_returns_none_when_value_is_none(self):
        """Returns None when value is None."""
        self.assertIsNone(save_object(None))

    def test_returns_bytesio_as_is_when_already_bytesio(self):
        """Returns BytesIO as-is when already BytesIO."""
        buf = BytesIO(b"existing")
        self.assertIs(save_object(buf), buf)

    def test_encodes_object_to_bytesio(self):
        """Encodes an object to a pickled BytesIO."""
        result = save_object({"a": 1})
        self.assertIsInstance(result, BytesIO)
        result.seek(0)
        self.assertEqual(pickle.load(result), {"a": 1})

    def test_round_trips_through_retrieve_object(self):
        """save_object then retrieve_object returns the original value."""
        value = [1, "two", 3.0]
        self.assertEqual(retrieve_object(save_object(value)), value)
