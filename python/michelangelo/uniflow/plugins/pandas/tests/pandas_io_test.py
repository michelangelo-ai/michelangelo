"""Tests for michelangelo.uniflow.plugins.pandas.io — PandasIO read/write."""

from __future__ import annotations

import os
import tempfile
from unittest import TestCase

import pandas as pd

from michelangelo.uniflow.plugins.pandas.io import PandasIO

_DF = pd.DataFrame(
    [
        {"name": "alice", "score": 0.92},
        {"name": "bob", "score": 0.88},
        {"name": "carol", "score": 0.95},
    ]
)


class TestPandasIORoundtrip(TestCase):
    """Write → read roundtrip tests for PandasIO."""

    def _dest(self) -> str:
        return tempfile.mkdtemp()

    def test_roundtrip_preserves_rows(self):
        """Written DataFrame is read back with the same number of rows."""
        io = PandasIO()
        dest = self._dest()
        io.write(dest, _DF.copy())
        result = io.read(dest, None)
        self.assertEqual(len(result), len(_DF))

    def test_roundtrip_preserves_columns(self):
        """Written DataFrame is read back with the same column names."""
        io = PandasIO()
        dest = self._dest()
        io.write(dest, _DF.copy())
        result = io.read(dest, None)
        self.assertListEqual(
            sorted(result.columns.tolist()), sorted(_DF.columns.tolist())
        )

    def test_roundtrip_preserves_values(self):
        """Written DataFrame is read back with the same values."""
        io = PandasIO()
        dest = self._dest()
        io.write(dest, _DF.copy())
        result = io.read(dest, None)
        self.assertEqual(result["name"].tolist(), _DF["name"].tolist())

    def test_writes_parquet_part_files(self):
        """It writes at least one part-*.parquet file in the destination."""
        io = PandasIO()
        dest = self._dest()
        io.write(dest, _DF.copy())
        files = [f for f in os.listdir(dest) if f.endswith(".parquet")]
        self.assertGreater(len(files), 0)
        self.assertTrue(all(f.startswith("part-") for f in files))

    def test_empty_dataframe_roundtrip(self):
        """It writes and reads back a zero-row DataFrame correctly."""
        io = PandasIO()
        dest = self._dest()
        io.write(dest, pd.DataFrame(columns=["name", "score"]))
        result = io.read(dest, None)
        self.assertEqual(len(result), 0)

    def test_creates_destination_directory(self):
        """It creates a nested destination directory that does not yet exist."""
        io = PandasIO()
        base = tempfile.mkdtemp()
        dest = os.path.join(base, "new", "nested")
        io.write(dest, _DF.copy())
        self.assertTrue(os.path.isdir(dest))

    def test_ensure_dir_fallback_when_exists_not_supported(self):
        """_ensure_dir() falls back to ls() when fs.exists() is absent."""
        from unittest.mock import MagicMock, patch

        dest = tempfile.mkdtemp()
        mock_fs = MagicMock(spec=[])  # empty spec — all attrs raise AttributeError
        mock_fs.ls = MagicMock(side_effect=FileNotFoundError("not found"))
        mock_fs.mkdir = MagicMock()
        with open(os.devnull, "wb") as devnull:
            mock_fs.open = MagicMock(return_value=devnull)
            with patch("fsspec.core.url_to_fs", return_value=(mock_fs, dest)):
                io = PandasIO()
                import contextlib

                with contextlib.suppress(Exception):
                    io.write(dest, _DF.copy())
            mock_fs.mkdir.assert_called()

    def test_write_returns_none(self):
        """write() returns None (no metadata needed for the read path)."""
        io = PandasIO()
        result = io.write(self._dest(), _DF.copy())
        self.assertIsNone(result)

    def test_large_dataframe_produces_multiple_part_files(self):
        """A DataFrame exceeding max_rows_per_file is split into multiple parts."""
        from michelangelo.uniflow.plugins.pandas.io import _MAX_ROWS_PER_FILE

        rows = _MAX_ROWS_PER_FILE + 1
        large_df = pd.DataFrame({"x": range(rows)})
        io = PandasIO()
        dest = self._dest()
        io.write(dest, large_df)
        files = [f for f in os.listdir(dest) if f.endswith(".parquet")]
        self.assertGreater(len(files), 1)
        result = io.read(dest, None)
        self.assertEqual(len(result), rows)


class TestPandasIORegistration(TestCase):
    """Tests for PandasIO registration with IORegistry."""

    def test_can_register_in_default_io(self):
        """PandasIO can be registered in default_io and retrieved by type."""
        from michelangelo.uniflow.core.io_registry import IORegistry

        registry = IORegistry({})
        registry.set(pd.DataFrame, PandasIO())
        handler = registry[pd.DataFrame]
        self.assertIsInstance(handler, PandasIO)

    def test_registered_handler_roundtrip(self):
        """A handler retrieved from the registry performs a correct roundtrip."""
        from michelangelo.uniflow.core.io_registry import IORegistry

        registry = IORegistry({pd.DataFrame: PandasIO()})
        dest = tempfile.mkdtemp()
        registry[pd.DataFrame].write(dest, _DF.copy())
        result = registry[pd.DataFrame].read(dest, None)
        self.assertEqual(len(result), len(_DF))
