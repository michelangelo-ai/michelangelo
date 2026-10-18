"""Unit tests for ``...tabular_assembler._private.file.wrap``."""

from __future__ import annotations

import os
import shutil
import tempfile
import unittest
from unittest.mock import patch

from michelangelo.lib.model_manager._private.utils.asset_utils.download import (
    download_assets,
)
from michelangelo.lib.model_manager.constants import StorageType
from michelangelo.workflow.tasks.tabular_assembler._private.file.wrap import (
    ensure_model_directory,
)


class EnsureModelDirectoryTest(unittest.TestCase):
    """Tests for ``ensure_model_directory``."""

    def setUp(self) -> None:
        """Create a temp working directory per test."""
        self._tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self._tmp.cleanup)
        self.root = self._tmp.name

    def _write(self, path: str, data: bytes = b"weights") -> str:
        with open(path, "wb") as f:
            f.write(data)
        return path

    def test_file_is_wrapped_into_directory_under_same_path(self) -> None:
        """A file becomes a directory at the same path holding the file."""
        path = self._write(os.path.join(self.root, "raw_model"), b"abc")

        result = ensure_model_directory(path)

        self.assertEqual(result, path)
        self.assertTrue(os.path.isdir(path))
        self.assertEqual(os.listdir(path), ["raw_model"])
        with open(os.path.join(path, "raw_model"), "rb") as f:
            self.assertEqual(f.read(), b"abc")

    def test_file_name_override_is_used(self) -> None:
        """``file_name`` sets the name inside the wrapper directory."""
        path = self._write(os.path.join(self.root, "raw_model"))

        ensure_model_directory(path, file_name="checkpoint.pt")

        self.assertEqual(os.listdir(path), ["checkpoint.pt"])

    def test_file_name_uses_final_component_only(self) -> None:
        """Path separators in ``file_name`` cannot escape the wrapper."""
        path = self._write(os.path.join(self.root, "raw_model"))

        ensure_model_directory(path, file_name="../../evil/ckpt.pt")

        self.assertEqual(os.listdir(path), ["ckpt.pt"])

    def test_degenerate_file_names_fall_back_to_local_basename(self) -> None:
        """Names that are empty, ``.`` or ``..`` use the local basename."""
        for bad_name in ("", ".", "..", "a/..", "a/."):
            with self.subTest(file_name=bad_name):
                path = self._write(os.path.join(self.root, "raw_model"))

                ensure_model_directory(path, file_name=bad_name)

                self.assertEqual(os.listdir(path), ["raw_model"])
                shutil.rmtree(path)

    def test_hidden_file_is_preserved(self) -> None:
        """A dot-prefixed file name survives wrapping."""
        path = self._write(os.path.join(self.root, "raw_model"))

        ensure_model_directory(path, file_name=".hidden_ckpt")

        self.assertEqual(os.listdir(path), [".hidden_ckpt"])

    def test_wrapped_file_may_share_the_directory_name(self) -> None:
        """A file named like the target path does not collide with it."""
        path = self._write(os.path.join(self.root, "model"))

        ensure_model_directory(path, file_name="model")

        self.assertTrue(os.path.isfile(os.path.join(path, "model")))

    def test_no_staging_leftovers_next_to_model(self) -> None:
        """Only the model path remains in the parent directory afterwards."""
        path = self._write(os.path.join(self.root, "raw_model"))

        ensure_model_directory(path)

        self.assertEqual(os.listdir(self.root), ["raw_model"])

    def test_directory_is_returned_unchanged(self) -> None:
        """A directory input is left exactly as it was."""
        path = os.path.join(self.root, "raw_model")
        os.makedirs(os.path.join(path, "sub"))
        self._write(os.path.join(path, "model.bin"))
        self._write(os.path.join(path, ".hidden"))

        result = ensure_model_directory(path, file_name="ignored.pt")

        self.assertEqual(result, path)
        self.assertEqual(sorted(os.listdir(path)), [".hidden", "model.bin", "sub"])

    def test_empty_directory_is_returned_unchanged(self) -> None:
        """An empty directory is still a directory."""
        path = os.path.join(self.root, "raw_model")
        os.makedirs(path)

        self.assertEqual(ensure_model_directory(path), path)
        self.assertEqual(os.listdir(path), [])

    def test_failed_move_cleans_up_staging_directory(self) -> None:
        """If moving the file fails, the error propagates and nothing is left."""
        path = self._write(os.path.join(self.root, "raw_model"))

        with (
            patch(
                "michelangelo.workflow.tasks.tabular_assembler._private.file."
                "wrap.shutil.move",
                side_effect=OSError("disk full"),
            ),
            self.assertRaises(OSError),
        ):
            ensure_model_directory(path)

        self.assertEqual(os.listdir(self.root), ["raw_model"])
        self.assertTrue(os.path.isfile(path))

    def test_missing_path_raises(self) -> None:
        """A path that does not exist raises ``FileNotFoundError``."""
        with self.assertRaises(FileNotFoundError):
            ensure_model_directory(os.path.join(self.root, "absent"))

    def test_packager_download_step_accepts_wrapped_file(self) -> None:
        """The packager's download-then-makedirs sequence works after wrapping.

        Without wrapping, ``download_assets`` leaves a file at the target and
        ``os.makedirs(..., exist_ok=True)`` raises ``FileExistsError``.
        """
        src = self._write(os.path.join(self.root, "raw_model"))
        target = os.path.join(self.root, "0", "model")
        os.makedirs(os.path.dirname(target))

        wrapped = ensure_model_directory(src, file_name="ckpt.pt")
        download_assets(wrapped, target, StorageType.LOCAL)
        os.makedirs(target, exist_ok=True)

        self.assertEqual(os.listdir(target), ["ckpt.pt"])

        unwrapped = self._write(os.path.join(self.root, "other"))
        target2 = os.path.join(self.root, "0", "model2")
        download_assets(unwrapped, target2, StorageType.LOCAL)
        with self.assertRaises(FileExistsError):
            os.makedirs(target2, exist_ok=True)
