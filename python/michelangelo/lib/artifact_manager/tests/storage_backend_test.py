"""Tests for the storage backend module."""

from __future__ import annotations

import os
import tempfile
from unittest import TestCase

from michelangelo.lib.artifact_manager.storage_backend import (
    LocalStorageBackend,
    StorageBackend,
)


class TestStorageBackendABC(TestCase):
    """Tests for the StorageBackend abstract base class."""

    def test_cannot_be_instantiated_directly(self):
        """It raises TypeError when instantiated without abstract methods."""
        with self.assertRaises(TypeError):
            StorageBackend()  # type: ignore[abstract]


class TestLocalStorageBackendFileUpload(TestCase):
    """Tests for LocalStorageBackend file upload and download."""

    def setUp(self) -> None:
        """Set up a temporary store directory and backend instance."""
        self._store_dir = tempfile.mkdtemp()
        self._backend = LocalStorageBackend(base_dir=self._store_dir)

    def test_upload_returns_absolute_uri(self):
        """It returns an absolute path URI after uploading a file."""
        with tempfile.NamedTemporaryFile(delete=False, suffix=".txt") as f:
            f.write(b"hello")
            src = f.name
        uri = self._backend.upload(src, "data/artifact.txt")
        self.assertTrue(os.path.isabs(uri))
        self.assertTrue(uri.startswith(self._store_dir))

    def test_upload_file_roundtrip(self):
        """It downloads the exact bytes that were uploaded."""
        with tempfile.NamedTemporaryFile(delete=False, suffix=".bin") as f:
            f.write(b"canvasflex-pusher")
            src = f.name
        uri = self._backend.upload(src, "files/test.bin")

        with tempfile.NamedTemporaryFile(delete=False, suffix=".bin") as tmp:
            dest = tmp.name
        self._backend.download(uri, dest)

        with open(dest, "rb") as fh:
            self.assertEqual(fh.read(), b"canvasflex-pusher")

    def test_upload_creates_nested_parent_directories(self):
        """It creates nested destination directories automatically."""
        with tempfile.NamedTemporaryFile(delete=False) as f:
            src = f.name
        uri = self._backend.upload(src, "a/b/c/artifact.bin")
        self.assertTrue(os.path.exists(uri))

    def test_upload_overwrites_existing_file(self):
        """It replaces an existing artifact at the same key."""
        with tempfile.NamedTemporaryFile(delete=False) as f:
            f.write(b"v1")
            src1 = f.name
        with tempfile.NamedTemporaryFile(delete=False) as f:
            f.write(b"v2")
            src2 = f.name

        self._backend.upload(src1, "artifact.bin")
        uri = self._backend.upload(src2, "artifact.bin")

        with tempfile.NamedTemporaryFile(delete=False, suffix=".bin") as tmp:
            dest = tmp.name
        self._backend.download(uri, dest)
        with open(dest, "rb") as fh:
            self.assertEqual(fh.read(), b"v2")


class TestLocalStorageBackendDirectoryUpload(TestCase):
    """Tests for LocalStorageBackend directory upload and download."""

    def setUp(self) -> None:
        """Set up a temporary store directory and backend instance."""
        self._store_dir = tempfile.mkdtemp()
        self._backend = LocalStorageBackend(base_dir=self._store_dir)

    def test_upload_directory_roundtrip(self):
        """It uploads and downloads a directory preserving file contents."""
        src_dir = tempfile.mkdtemp()
        with open(os.path.join(src_dir, "weights.txt"), "w") as f:
            f.write("model-weights")
        with open(os.path.join(src_dir, "config.json"), "w") as f:
            f.write("{}")

        uri = self._backend.upload(src_dir, "models/classifier/v1")

        dest_dir = tempfile.mkdtemp()
        self._backend.download(uri, dest_dir)

        with open(os.path.join(dest_dir, "weights.txt")) as fh:
            self.assertEqual(fh.read(), "model-weights")
        with open(os.path.join(dest_dir, "config.json")) as fh:
            self.assertEqual(fh.read(), "{}")

    def test_upload_directory_uri_is_under_base_dir(self):
        """It stores the directory under base_dir using the destination key."""
        src_dir = tempfile.mkdtemp()
        uri = self._backend.upload(src_dir, "models/v1")
        self.assertTrue(uri.startswith(self._store_dir))
        self.assertTrue(os.path.isdir(uri))

    def test_upload_directory_replaces_existing(self):
        """It replaces an existing directory artifact at the same key."""
        src_v1 = tempfile.mkdtemp()
        with open(os.path.join(src_v1, "file.txt"), "w") as f:
            f.write("v1")
        src_v2 = tempfile.mkdtemp()
        with open(os.path.join(src_v2, "file.txt"), "w") as f:
            f.write("v2")

        self._backend.upload(src_v1, "models/cls")
        uri = self._backend.upload(src_v2, "models/cls")

        dest_dir = tempfile.mkdtemp()
        self._backend.download(uri, dest_dir)
        with open(os.path.join(dest_dir, "file.txt")) as fh:
            self.assertEqual(fh.read(), "v2")

    def test_upload_directory_cleans_up_tmp_on_failure(self):
        """It removes the temp artifact and re-raises when copytree fails."""
        from unittest.mock import patch

        src_dir = tempfile.mkdtemp()
        with (
            patch("shutil.copytree", side_effect=OSError("disk full")),
            self.assertRaises(OSError),
        ):
            self._backend.upload(src_dir, "models/v1")

        # No stray .__tmp_ directories should remain under base_dir
        leftovers = [name for name in os.listdir(self._store_dir) if ".__tmp_" in name]
        self.assertEqual(leftovers, [])


class TestLocalStorageBackendDownloadErrors(TestCase):
    """Tests for LocalStorageBackend download error handling."""

    def test_raises_value_error_for_foreign_uri(self):
        """It raises ValueError when the URI belongs to a different backend."""
        backend_a = LocalStorageBackend(base_dir=tempfile.mkdtemp())
        backend_b = LocalStorageBackend(base_dir=tempfile.mkdtemp())

        with tempfile.NamedTemporaryFile(delete=False) as f:
            src = f.name
        uri_from_a = backend_a.upload(src, "artifact.bin")

        with self.assertRaises(ValueError) as ctx:
            backend_b.download(uri_from_a, "/tmp/dest")

        self.assertIn("not managed by this LocalStorageBackend", str(ctx.exception))
        self.assertIn(backend_b._base_dir, str(ctx.exception))

    def test_raises_value_error_for_sibling_directory_uri(self):
        """It rejects a URI whose path shares a prefix but is a sibling directory."""
        import os

        store_dir = tempfile.mkdtemp()
        # Create a sibling dir whose name starts with store_dir
        sibling_dir = store_dir + "_sibling"
        os.makedirs(sibling_dir, exist_ok=True)
        backend = LocalStorageBackend(base_dir=store_dir)

        foreign_uri = os.path.join(sibling_dir, "artifact.bin")
        with open(foreign_uri, "w") as f:
            f.write("foreign")

        with self.assertRaises(ValueError):
            backend.download(foreign_uri, tempfile.mktemp())

    def test_upload_raises_for_empty_destination_key(self):
        """It raises ValueError when destination_key is an empty string."""
        backend = LocalStorageBackend(base_dir=tempfile.mkdtemp())
        with tempfile.NamedTemporaryFile(delete=False) as f:
            src = f.name
        with self.assertRaises(ValueError) as ctx:
            backend.upload(src, "")
        self.assertIn("non-empty", str(ctx.exception))
