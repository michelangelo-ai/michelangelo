import io
import os
from dataclasses import dataclass
from unittest import TestCase

import fsspec

from michelangelo.uniflow.core.io_registry import default_io
from michelangelo.uniflow.core.ref import ref, Ref, unref


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
