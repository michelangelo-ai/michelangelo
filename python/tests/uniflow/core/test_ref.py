import io
import os
from dataclasses import dataclass
from unittest import TestCase

import fsspec

from michelangelo.uniflow.core.io_registry import default_io
from michelangelo.uniflow.core.ref import Ref, ref, unref


@dataclass
class File:
    id: str
    data: io.BytesIO
    metadata: dict


class Test(TestCase):
    def test_ref_unref(self):
        os.environ["UF_STORAGE_URL"] = "memory://~/storage"

        files = [
            File(
                id="1.txt",
                data=io.BytesIO(b"foo-bar-baz"),
                metadata={
                    "size": 100,
                },
            ),
            File(
                id="2.txt",
                data=io.BytesIO(b"quick-brown-fox"),
                metadata={
                    "size": 101,
                    "tags": ["test", "ref"],
                },
            ),
        ]

        files_ref = ref(files, io=default_io)
        self.assertIsInstance(files_ref, list)
        self.assertEqual(2, len(files_ref))

        f1, f2 = files_ref

        self.assertIsInstance(f1, File)
        self.assertEqual(f1.id, "1.txt")
        self.assertEqual(
            f1.metadata,
            {
                "size": 100,
            },
        )
        self.assertIsInstance(f1.data, Ref)
        with fsspec.open(f1.data.url, mode="rb") as f:
            self.assertEqual(b"foo-bar-baz", f.read())

        self.assertIsInstance(f2, File)
        self.assertEqual(f2.id, "2.txt")
        self.assertEqual(
            f2.metadata,
            {
                "size": 101,
                "tags": ["test", "ref"],
            },
        )
        self.assertIsInstance(f2.data, Ref)
        with fsspec.open(f2.data.url, mode="rb") as f:
            self.assertEqual(b"quick-brown-fox", f.read())

        files_unref = unref(files_ref, io=default_io)
        self.assertIsInstance(files_unref, list)
        self.assertEqual(2, len(files_unref))

        f1, f2 = files_unref

        self.assertIsInstance(f1, File)
        self.assertEqual(f1.id, "1.txt")
        self.assertIsInstance(f1.data, io.BytesIO)
        self.assertEqual(f1.data.getvalue(), b"foo-bar-baz")
        self.assertEqual(
            f1.metadata,
            {
                "size": 100,
            },
        )

        self.assertIsInstance(f2, File)
        self.assertEqual(f2.id, "2.txt")
        self.assertIsInstance(f2.data, io.BytesIO)
        self.assertEqual(f2.data.getvalue(), b"quick-brown-fox")
        self.assertEqual(
            f2.metadata,
            {
                "size": 101,
                "tags": ["test", "ref"],
            },
        )

    def test_ref_none_handling(self):
        """Test that ref() properly handles None values without errors."""
        # Test that ref() returns None when passed None
        result = ref(None, io=default_io)
        self.assertIsNone(result)

    def test_unref_none_handling(self):
        """Test that unref() properly handles None values without errors."""
        # Test that unref() returns None when passed None
        result = unref(None, io=default_io)
        self.assertIsNone(result)

    def test_ref_unref_with_none_values_in_containers(self):
        """Test ref/unref with None values inside containers."""
        os.environ["UF_STORAGE_URL"] = "memory://~/storage"
        # Test list with None values
        data_with_nones = [None, "test", None, {"key": "value"}, None]
        # ref should preserve None values
        ref_result = ref(data_with_nones, io=default_io)
        self.assertIsInstance(ref_result, list)
        self.assertEqual(len(ref_result), 5)
        self.assertIsNone(ref_result[0])
        self.assertEqual(ref_result[1], "test")
        self.assertIsNone(ref_result[2])
        self.assertEqual(ref_result[3], {"key": "value"})
        self.assertIsNone(ref_result[4])
        # unref should preserve None values
        unref_result = unref(ref_result, io=default_io)
        self.assertIsInstance(unref_result, list)
        self.assertEqual(len(unref_result), 5)
        self.assertIsNone(unref_result[0])
        self.assertEqual(unref_result[1], "test")
        self.assertIsNone(unref_result[2])
        self.assertEqual(unref_result[3], {"key": "value"})
        self.assertIsNone(unref_result[4])
        # Test dict with None values
        dict_with_nones = {"a": None, "b": "test", "c": None}
        ref_dict_result = ref(dict_with_nones, io=default_io)
        self.assertIsInstance(ref_dict_result, dict)
        self.assertIsNone(ref_dict_result["a"])
        self.assertEqual(ref_dict_result["b"], "test")
        self.assertIsNone(ref_dict_result["c"])
        unref_dict_result = unref(ref_dict_result, io=default_io)
        self.assertIsInstance(unref_dict_result, dict)
        self.assertIsNone(unref_dict_result["a"])
        self.assertEqual(unref_dict_result["b"], "test")
        self.assertIsNone(unref_dict_result["c"])
