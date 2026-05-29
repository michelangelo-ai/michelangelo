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

    def test_returns_empty_when_all_candidates_have_no_row_groups(self):
        """Returns [] when all non-zero-byte files have empty row groups."""
        from michelangelo.uniflow.plugins.ray.io import RayDatasetIO

        fs = self._make_fs({"/data/empty.parquet": {"size": 100}})
        _rg_patch = "michelangelo.uniflow.plugins.ray.io._has_row_groups"
        with (
            patch("fsspec.core.url_to_fs", return_value=(fs, "/data")),
            patch(_rg_patch, return_value=False),
        ):
            result = RayDatasetIO.filter_empty_data("/data")
        self.assertEqual(result, [])


class TestHasRowGroups(TestCase):
    """Tests for _has_row_groups()."""

    def test_returns_true_when_row_groups_present(self):
        """Returns True when num_row_groups > 0."""
        from michelangelo.uniflow.plugins.ray.io import _has_row_groups

        mock_meta = MagicMock()
        mock_meta.num_row_groups = 2
        with patch("pyarrow.parquet.read_metadata", return_value=mock_meta):
            self.assertTrue(_has_row_groups("/f.parquet", MagicMock()))

    def test_returns_false_when_zero_row_groups(self):
        """Returns False when num_row_groups == 0."""
        from michelangelo.uniflow.plugins.ray.io import _has_row_groups

        mock_meta = MagicMock()
        mock_meta.num_row_groups = 0
        with patch("pyarrow.parquet.read_metadata", return_value=mock_meta):
            self.assertFalse(_has_row_groups("/f.parquet", MagicMock()))

    def test_returns_false_on_non_oserror(self):
        """Returns False (logs warning) for unexpected exceptions."""
        from michelangelo.uniflow.plugins.ray.io import _has_row_groups

        with patch("pyarrow.parquet.read_metadata", side_effect=ValueError("corrupt")):
            self.assertFalse(_has_row_groups("/f.parquet", MagicMock()))

    def test_reraises_oserror(self):
        """Re-raises OSError instead of swallowing it."""
        from michelangelo.uniflow.plugins.ray.io import _has_row_groups

        with (
            patch("pyarrow.parquet.read_metadata", side_effect=OSError("not found")),
            self.assertRaises(OSError),
        ):
            _has_row_groups("/f.parquet", MagicMock())


class TestChunkList(TestCase):
    """Tests for _chunk_list()."""

    def test_zero_chunks_treated_as_one(self):
        """num_chunks <= 0 is treated as 1 — returns single chunk."""
        from michelangelo.uniflow.plugins.ray.io import _chunk_list

        self.assertEqual(_chunk_list(["a", "b"], 0), [["a", "b"]])

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

    def test_raises_file_not_found_when_no_files_found(self):
        """read() raises FileNotFoundError when no parquet files exist at url."""
        from michelangelo.uniflow.plugins.ray.io import RayDatasetIO

        mods, _ = self._mock_ray()
        _fs = "michelangelo.uniflow.plugins.ray.io._fs_path"
        with (
            patch.dict(sys.modules, mods),
            patch(_fs, return_value=(None, "/d")),
            patch.object(RayDatasetIO, "filter_empty_data", return_value=[]),
            self.assertRaises(FileNotFoundError),
        ):
            RayDatasetIO().read("/d", None)

    def test_polars_fallback_triggered_on_nested_array_error(self):
        """read() calls _read_parquet_fallback on the PyArrow nested-data error."""
        import michelangelo.uniflow.plugins.ray.io as io_mod
        from michelangelo.uniflow.plugins.ray.io import (
            _NESTED_CHUNKED_ARRAY_ERROR,
            RayDatasetIO,
        )

        mock_ds = MagicMock()
        mock_ray = MagicMock()
        mock_ray.data.read_parquet.side_effect = Exception(_NESTED_CHUNKED_ARRAY_ERROR)

        _fs = "michelangelo.uniflow.plugins.ray.io._fs_path"
        _fe = ["/d/f.parquet"]
        with (
            patch.object(io_mod, "ray", mock_ray),
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
        import michelangelo.uniflow.plugins.ray.io as io_mod
        from michelangelo.uniflow.plugins.ray.io import RayDatasetIO

        mock_ray = MagicMock()
        mock_ray.data.read_parquet.side_effect = RuntimeError("disk full")

        _fs = "michelangelo.uniflow.plugins.ray.io._fs_path"
        _fe = ["/d/f.parquet"]
        with (
            patch.object(io_mod, "ray", mock_ray),
            patch(_fs, return_value=(None, "/d")),
            patch.object(RayDatasetIO, "filter_empty_data", return_value=_fe),
            self.assertRaises(RuntimeError),
        ):
            RayDatasetIO().read("/d", None)

    def test_read_returns_dataset_on_success(self):
        """read() returns the Ray Dataset when read_parquet succeeds."""
        import michelangelo.uniflow.plugins.ray.io as io_mod
        from michelangelo.uniflow.plugins.ray.io import RayDatasetIO

        mock_ds = MagicMock()
        mock_ray = MagicMock()
        mock_ray.data.read_parquet.return_value = mock_ds

        _fs = "michelangelo.uniflow.plugins.ray.io._fs_path"
        _fe = ["/d/f.parquet"]
        with (
            patch.object(io_mod, "ray", mock_ray),
            patch(_fs, return_value=(None, "/d")),
            patch.object(RayDatasetIO, "filter_empty_data", return_value=_fe),
        ):
            result = RayDatasetIO().read("/d", None)

        self.assertIs(result, mock_ds)

    def test_write_calls_write_parquet(self):
        """write() passes the PyArrow filesystem and path to Dataset.write_parquet."""
        from michelangelo.uniflow.plugins.ray.io import RayDatasetIO

        mock_ds = MagicMock()
        mock_fs = MagicMock()
        with patch(
            "michelangelo.uniflow.plugins.ray.io._fs_path",
            return_value=(mock_fs, "/d"),
        ):
            RayDatasetIO().write("/d", mock_ds)
        mock_ds.write_parquet.assert_called_once_with("/d", filesystem=mock_fs)

    def test_read_parquet_fallback_calls_read_datasource(self):
        """_read_parquet_fallback delegates to ray.data.read_datasource."""
        import michelangelo.uniflow.plugins.ray.io as io_mod
        from michelangelo.uniflow.plugins.ray.io import RayDatasetIO

        mock_ray = MagicMock()
        mock_result = MagicMock()
        mock_ray.data.read_datasource.return_value = mock_result

        with patch.object(io_mod, "ray", mock_ray):
            result = RayDatasetIO._read_parquet_fallback("/d", ["/d/f.parquet"])

        self.assertIs(result, mock_result)
        mock_ray.data.read_datasource.assert_called_once()


class TestParquetPolarsDatasourceNoPolars(TestCase):
    """Tests for _ParquetPolarsDatasource when Polars is not installed."""

    def test_read_fn_raises_import_error_when_polars_missing(self):
        """read_fn raises ImportError when polars is absent at call time."""
        import michelangelo.uniflow.plugins.ray.io as io_mod
        from michelangelo.uniflow.plugins.ray.io import _ParquetPolarsDatasource

        mock_read_task = MagicMock()
        captured_fns = []
        mock_read_task.side_effect = lambda fn, meta: captured_fns.append(fn)
        mock_block_meta = MagicMock()

        src = _ParquetPolarsDatasource(url="/tmp", paths=["/tmp/f.parquet"])
        with (
            patch.object(io_mod, "ReadTask", mock_read_task),
            patch.object(io_mod, "BlockMetadata", return_value=mock_block_meta),
        ):
            src.get_read_tasks(1)

        self.assertEqual(len(captured_fns), 1)
        with (
            patch.dict(sys.modules, {"polars": None}),
            self.assertRaises((ImportError, ModuleNotFoundError)) as ctx,
        ):
            list(captured_fns[0]())
        self.assertIn("ray-polars", str(ctx.exception))


class TestFsPathAndResolveFs(TestCase):
    """Tests for _fs_path() env-var switching and resolve_fs() S3 branch."""

    def test_fs_path_uses_fsspec_when_env_set(self):
        """_fs_path() returns the raw fsspec FS when UF_PLUGIN_RAY_USE_FSSPEC=1."""
        from michelangelo.uniflow.plugins.ray.io import (
            UF_PLUGIN_RAY_USE_FSSPEC,
            _fs_path,
        )

        mock_fs = MagicMock()
        with (
            patch.dict("os.environ", {UF_PLUGIN_RAY_USE_FSSPEC: "1"}),
            patch("fsspec.core.url_to_fs", return_value=(mock_fs, "/d")) as mock_url,
        ):
            fs, path = _fs_path("s3://bucket/d")
        mock_url.assert_called_once_with("s3://bucket/d")
        self.assertIs(fs, mock_fs)
        self.assertEqual(path, "/d")

    def test_fs_path_uses_pyarrow_by_default(self):
        """_fs_path() falls back to resolve_fs when env var is '0' (default)."""
        from michelangelo.uniflow.plugins.ray.io import (
            UF_PLUGIN_RAY_USE_FSSPEC,
            _fs_path,
        )

        with (
            patch.dict("os.environ", {UF_PLUGIN_RAY_USE_FSSPEC: "0"}),
            patch(
                "michelangelo.uniflow.plugins.ray.io.resolve_fs", return_value=None
            ) as mock_rfs,
        ):
            fs, _path = _fs_path("local:///tmp/d")
        mock_rfs.assert_called_once_with("local")
        self.assertIsNone(fs)

    def test_resolve_fs_returns_s3_filesystem(self):
        """resolve_fs('s3') returns a PyArrow S3FileSystem."""
        import types as _t

        from michelangelo.uniflow.plugins.ray.io import resolve_fs

        mock_s3fs = MagicMock()
        mock_pa_fs = _t.SimpleNamespace(S3FileSystem=MagicMock(return_value=mock_s3fs))
        mock_pa = _t.SimpleNamespace(fs=mock_pa_fs)
        with patch.dict(sys.modules, {"pyarrow": mock_pa, "pyarrow.fs": mock_pa_fs}):
            result = resolve_fs("s3")
        self.assertIs(result, mock_s3fs)

    def test_resolve_fs_returns_none_for_local(self):
        """resolve_fs returns None for non-s3 protocols."""
        from michelangelo.uniflow.plugins.ray.io import resolve_fs

        self.assertIsNone(resolve_fs("local"))
        self.assertIsNone(resolve_fs("file"))
        self.assertIsNone(resolve_fs(""))
