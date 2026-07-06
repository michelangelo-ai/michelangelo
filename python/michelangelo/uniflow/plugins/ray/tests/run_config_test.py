"""Tests for create_run_config()."""

from __future__ import annotations

from unittest import TestCase
from unittest.mock import MagicMock, patch

from michelangelo.uniflow.plugins.ray.run_config import create_run_config

_MODULE = "michelangelo.uniflow.plugins.ray.run_config"


class TestCreateRunConfig(TestCase):
    """Tests for create_run_config()."""

    def test_uses_tempdir_when_storage_url_unset(self):
        """No UF_STORAGE_URL -> falls back to a local temp directory."""
        with patch.dict("os.environ", {}, clear=True):
            run_config = create_run_config()
        self.assertIsNone(run_config.storage_filesystem)
        self.assertTrue(run_config.storage_path)

    def test_resolves_storage_from_uf_storage_url(self):
        """UF_STORAGE_URL set -> storage_path/storage_filesystem from _fs_path()."""
        mock_fs = MagicMock()
        with (
            patch.dict("os.environ", {"UF_STORAGE_URL": "s3://bucket/prefix"}),
            patch(f"{_MODULE}._fs_path", return_value=(mock_fs, "s3://bucket/prefix")),
        ):
            run_config = create_run_config()
        self.assertIs(run_config.storage_filesystem, mock_fs)
        self.assertEqual(run_config.storage_path, "bucket/prefix")

    def test_strips_scheme_only_when_filesystem_present(self):
        """A local path from _fs_path() (no filesystem) is passed through as-is."""
        with (
            patch.dict("os.environ", {"UF_STORAGE_URL": "/tmp/uf_storage"}),
            patch(f"{_MODULE}._fs_path", return_value=(None, "/tmp/uf_storage")),
        ):
            run_config = create_run_config()
        self.assertIsNone(run_config.storage_filesystem)
        self.assertEqual(run_config.storage_path, "/tmp/uf_storage")

    def test_explicit_storage_path_overrides_default(self):
        """An explicit storage_path kwarg is not overwritten."""
        with patch.dict("os.environ", {"UF_STORAGE_URL": "s3://bucket/prefix"}):
            run_config = create_run_config(storage_path="/explicit/path")
        self.assertEqual(run_config.storage_path, "/explicit/path")

    def test_explicit_storage_filesystem_overrides_default(self):
        """An explicit storage_filesystem kwarg is not overwritten."""
        mock_fs = MagicMock()
        with patch.dict("os.environ", {}, clear=True):
            run_config = create_run_config(storage_filesystem=mock_fs)
        self.assertIs(run_config.storage_filesystem, mock_fs)

    def test_forwards_other_kwargs(self):
        """Non-storage RunConfig kwargs (e.g. checkpoint_config) pass through."""
        import ray.train

        checkpoint_config = ray.train.CheckpointConfig(num_to_keep=2)
        with patch.dict("os.environ", {}, clear=True):
            run_config = create_run_config(checkpoint_config=checkpoint_config)
        self.assertIs(run_config.checkpoint_config, checkpoint_config)
