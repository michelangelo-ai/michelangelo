"""Tests for improved RayDatasetIO: filter_empty_data, Polars fallback, logging."""

from __future__ import annotations

import sys
from unittest import TestCase
from unittest.mock import MagicMock, patch


class TestRayDatasetIOFilterEmptyData(TestCase):
    """Tests for RayDatasetIO.filter_empty_data()."""

    def _make_fs(self, files: dict):
        fs = MagicMock()
        fs.find = MagicMock(return_value=files)
        return fs

    def test_returns_empty_list_when_no_parquet_files(self):
        """Returns [] when no .parquet files exist under url."""
        from michelangelo.uniflow.plugins.ray.io import RayDatasetIO

        fs = self._make_fs({"/data/readme.txt": {"size": 100}})
        with patch("fsspec.core.url_to_fs", return_value=(fs, "/data")):
            self.assertEqual(RayDatasetIO.filter_empty_data("/data"), [])

    def test_skips_zero_byte_files(self):
        """Discards zero-byte files before metadata check."""
        from michelangelo.uniflow.plugins.ray.io import RayDatasetIO

        fs = self._make_fs(
            {
                "/data/empty.parquet": {"size": 0},
                "/data/data.parquet": {"size": 1024},
            }
        )
        _rg_patch = "michelangelo.uniflow.plugins.ray.io._has_row_groups"
        with (
            patch("fsspec.core.url_to_fs", return_value=(fs, "/data")),
            patch(_rg_patch, return_value=True),
        ):
            result = RayDatasetIO.filter_empty_data("/data")
        self.assertEqual(result, ["/data/data.parquet"])

    def test_skips_files_with_no_row_groups(self):
        """Removes files whose parquet metadata reports 0 row groups."""
        from michelangelo.uniflow.plugins.ray.io import RayDatasetIO

        fs = self._make_fs(
            {
                "/data/a.parquet": {"size": 512},
                "/data/b.parquet": {"size": 512},
            }
        )
        _rg_patch = "michelangelo.uniflow.plugins.ray.io._has_row_groups"
        with (
            patch("fsspec.core.url_to_fs", return_value=(fs, "/data")),
            patch(_rg_patch, side_effect=[True, False]),
        ):
            result = RayDatasetIO.filter_empty_data("/data")
        self.assertEqual(result, ["/data/a.parquet"])

    def test_returns_all_paths_when_all_have_data(self):
        """Returns all paths when every file has row groups."""
        from michelangelo.uniflow.plugins.ray.io import RayDatasetIO

        fs = self._make_fs(
            {
                "/data/part-0.parquet": {"size": 100},
                "/data/part-1.parquet": {"size": 200},
            }
        )
        _rg_patch = "michelangelo.uniflow.plugins.ray.io._has_row_groups"
        with (
            patch("fsspec.core.url_to_fs", return_value=(fs, "/data")),
            patch(_rg_patch, return_value=True),
        ):
            result = RayDatasetIO.filter_empty_data("/data")
        self.assertEqual(
            sorted(result), ["/data/part-0.parquet", "/data/part-1.parquet"]
        )


class TestHasRowGroups(TestCase):
    """Tests for _has_row_groups()."""

    def test_returns_true_when_row_groups_present(self):
        """Returns True when num_row_groups > 0."""
        from michelangelo.uniflow.plugins.ray.io import _has_row_groups

        mock_pq = MagicMock()
        mock_pq.read_metadata.return_value.num_row_groups = 2
        with patch.dict(sys.modules, {"pyarrow.parquet": mock_pq}):
            self.assertTrue(_has_row_groups("/f.parquet", MagicMock()))

    def test_returns_false_when_zero_row_groups(self):
        """Returns False when num_row_groups == 0."""
        from michelangelo.uniflow.plugins.ray.io import _has_row_groups

        mock_pq = MagicMock()
        mock_pq.read_metadata.return_value.num_row_groups = 0
        with patch.dict(sys.modules, {"pyarrow.parquet": mock_pq}):
            self.assertFalse(_has_row_groups("/f.parquet", MagicMock()))

    def test_returns_false_on_non_oserror(self):
        """Returns False (logs warning) for unexpected exceptions."""
        from michelangelo.uniflow.plugins.ray.io import _has_row_groups

        mock_pq = MagicMock()
        mock_pq.read_metadata.side_effect = ValueError("corrupt")
        with patch.dict(sys.modules, {"pyarrow.parquet": mock_pq}):
            self.assertFalse(_has_row_groups("/f.parquet", MagicMock()))

    def test_reraises_oserror(self):
        """Re-raises OSError instead of swallowing it."""
        from michelangelo.uniflow.plugins.ray.io import _has_row_groups

        mock_pq = MagicMock()
        mock_pq.read_metadata.side_effect = OSError("not found")
        with (
            patch.dict(sys.modules, {"pyarrow.parquet": mock_pq}),
            self.assertRaises(OSError),
        ):
            _has_row_groups("/f.parquet", MagicMock())


class TestChunkList(TestCase):
    """Tests for _chunk_list()."""

    def test_empty_list_returns_empty(self):
        """Returns [] for an empty input."""
        from michelangelo.uniflow.plugins.ray.io import _chunk_list

        self.assertEqual(_chunk_list([], 4), [])

    def test_single_chunk(self):
        """Returns single chunk when num_chunks=1."""
        from michelangelo.uniflow.plugins.ray.io import _chunk_list

        self.assertEqual(_chunk_list(["a", "b", "c"], 1), [["a", "b", "c"]])

    def test_all_items_preserved_across_chunks(self):
        """All items appear exactly once across all chunks."""
        from michelangelo.uniflow.plugins.ray.io import _chunk_list

        result = _chunk_list(["a", "b", "c", "d"], 2)
        flat = [item for chunk in result for item in chunk]
        self.assertEqual(sorted(flat), ["a", "b", "c", "d"])

    def test_more_chunks_than_items(self):
        """Handles num_chunks > len(lst) without duplicating items."""
        from michelangelo.uniflow.plugins.ray.io import _chunk_list

        result = _chunk_list(["a", "b"], 10)
        flat = [item for chunk in result for item in chunk]
        self.assertEqual(sorted(flat), ["a", "b"])


class TestRayDatasetIOReadPaths(TestCase):
    """Tests for RayDatasetIO.read() — empty and fallback code paths."""

    def _mock_ray(self):
        """Return (sys.modules patch dict, mock_data namespace) for ray + ray.data."""
        import types as _t

        mock_data = _t.SimpleNamespace(
            from_items=MagicMock(return_value=MagicMock()),
            read_parquet=MagicMock(return_value=MagicMock()),
            read_datasource=MagicMock(return_value=MagicMock()),
        )
        mock_ray = _t.SimpleNamespace(data=mock_data)
        return {"ray": mock_ray, "ray.data": mock_data}, mock_data

    def test_returns_empty_dataset_when_no_files_found(self):
        """read() returns ray.data.from_items([]) when no data is found."""
        from michelangelo.uniflow.plugins.ray.io import RayDatasetIO

        mods, mock_data = self._mock_ray()
        mock_empty = MagicMock()
        mock_data.from_items = MagicMock(return_value=mock_empty)

        _fs = "michelangelo.uniflow.plugins.ray.io._fs_path"
        with (
            patch.dict(sys.modules, mods),
            patch(_fs, return_value=(None, "/d")),
            patch.object(RayDatasetIO, "filter_empty_data", return_value=[]),
        ):
            result = RayDatasetIO().read("/d", None)

        mock_data.from_items.assert_called_once_with([])
        self.assertIs(result, mock_empty)

    def test_polars_fallback_triggered_on_nested_array_error(self):
        """read() calls _read_parquet_fallback on the PyArrow nested-data error."""
        from michelangelo.uniflow.plugins.ray.io import (
            _NESTED_CHUNKED_ARRAY_ERROR,
            RayDatasetIO,
        )

        mods, mock_data = self._mock_ray()
        mock_ds = MagicMock()
        mock_data.read_parquet = MagicMock(
            side_effect=Exception(_NESTED_CHUNKED_ARRAY_ERROR)
        )

        _fs = "michelangelo.uniflow.plugins.ray.io._fs_path"
        _fe = ["/d/f.parquet"]
        with (
            patch.dict(sys.modules, mods),
            patch(_fs, return_value=(None, "/d")),
            patch.object(RayDatasetIO, "filter_empty_data", return_value=_fe),
            patch.object(
                RayDatasetIO, "_read_parquet_fallback", return_value=mock_ds
            ) as mock_fb,
        ):
            result = RayDatasetIO().read("/d", None)

        mock_fb.assert_called_once_with("/d", ["/d/f.parquet"])
        self.assertIs(result, mock_ds)

    def test_reraises_unrelated_exceptions(self):
        """read() propagates exceptions unrelated to the nested-data bug."""
        from michelangelo.uniflow.plugins.ray.io import RayDatasetIO

        mods, mock_data = self._mock_ray()
        mock_data.read_parquet = MagicMock(side_effect=RuntimeError("disk full"))

        _fs = "michelangelo.uniflow.plugins.ray.io._fs_path"
        _fe = ["/d/f.parquet"]
        with (
            patch.dict(sys.modules, mods),
            patch(_fs, return_value=(None, "/d")),
            patch.object(RayDatasetIO, "filter_empty_data", return_value=_fe),
            self.assertRaises(RuntimeError),
        ):
            RayDatasetIO().read("/d", None)


class TestParquetPolarsDatasourceNoPolars(TestCase):
    """Tests for _ParquetPolarsDatasource when Polars is not installed."""

    def test_read_fn_raises_import_error_when_polars_missing(self):
        """read_fn raises ImportError when polars is absent at call time."""
        import types as _t

        from michelangelo.uniflow.plugins.ray.io import _ParquetPolarsDatasource

        # Mock ray.data.ReadTask and BlockMetadata so get_read_tasks() can run.
        captured_fns = []
        mock_read_task = MagicMock(side_effect=lambda fn, meta: captured_fns.append(fn))
        mock_block_meta = MagicMock()
        mock_ray_data = _t.SimpleNamespace(
            ReadTask=mock_read_task,
            block=_t.SimpleNamespace(BlockMetadata=lambda **kw: mock_block_meta),
        )
        mods = {"ray.data": mock_ray_data, "ray.data.block": mock_ray_data.block}

        src = _ParquetPolarsDatasource(url="/tmp", paths=["/tmp/f.parquet"])
        with patch.dict(sys.modules, mods):
            src.get_read_tasks(1)

        # The read_fn is captured; calling it with polars absent raises ImportError.
        self.assertEqual(len(captured_fns), 1)
        with (
            patch.dict(sys.modules, {"polars": None}),
            self.assertRaises((ImportError, ModuleNotFoundError)),
        ):
            list(captured_fns[0]())
