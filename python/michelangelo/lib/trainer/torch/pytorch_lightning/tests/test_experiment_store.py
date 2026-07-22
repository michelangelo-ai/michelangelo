"""Tests for the default ``FsspecExperimentStore``.

Covers ``michelangelo.lib.trainer.torch.pytorch_lightning.experiment_store``:
the fsspec marker-file round-trip on a local ``tmp_path`` filesystem, the
"nothing to resume" / corrupt-marker paths (both returning ``None`` without
raising), the best-effort swallow on a ``track`` write failure, and structural
conformance to the :class:`ExperimentStore` protocol.
"""

from __future__ import annotations

import json
from unittest.mock import patch

import pytest

# Importing the store pulls in the package ``__init__`` which eagerly imports the
# Ray/Lightning-backed trainer. Skip cleanly if those heavy deps are missing.
pytest.importorskip("ray")
pytest.importorskip("torch")
pytest.importorskip("pytorch_lightning")

from michelangelo.lib.trainer.torch.pytorch_lightning.experiment_store import (
    FsspecExperimentStore,
)
from michelangelo.lib.trainer.torch.pytorch_lightning.schema import (
    ExperimentStore,
)

_STORE_MODULE = "michelangelo.lib.trainer.torch.pytorch_lightning.experiment_store"


class TestProtocolConformance:
    """``FsspecExperimentStore`` structurally satisfies the protocol."""

    def test_is_experiment_store(self):
        """The default impl passes the ``@runtime_checkable`` protocol check."""
        assert isinstance(FsspecExperimentStore(), ExperimentStore)


class TestMarkerPath:
    """Deterministic marker-path derivation from the stable identity."""

    def test_marker_path_scheme(self):
        """Path is ``{storage_path}/.michelangelo_resume/{run_name}.json``."""
        store = FsspecExperimentStore()
        assert (
            store._marker_path("/root/runs", "my_run")
            == "/root/runs/.michelangelo_resume/my_run.json"
        )

    def test_marker_path_strips_trailing_slash(self):
        """A trailing slash on ``storage_path`` does not double up."""
        store = FsspecExperimentStore()
        assert (
            store._marker_path("/root/runs/", "my_run")
            == "/root/runs/.michelangelo_resume/my_run.json"
        )


class TestRoundTrip:
    """Track-then-locate round-trip against a real local filesystem."""

    def test_track_then_locate_returns_experiment_path(self, tmp_path):
        """``track`` writes a marker that ``locate_resumable`` reads back."""
        store = FsspecExperimentStore()
        storage_path = str(tmp_path)
        store.track(
            storage_path=storage_path,
            run_name="run1",
            experiment_path="/exp/run1_dir",
        )

        # Marker file exists at the deterministic path with the expected payload.
        marker = tmp_path / ".michelangelo_resume" / "run1.json"
        assert marker.exists()
        payload = json.loads(marker.read_text())
        assert payload["experiment_path"] == "/exp/run1_dir"
        assert payload["run_name"] == "run1"
        assert payload["schema_version"] == FsspecExperimentStore._SCHEMA_VERSION
        assert "written_at" in payload

        located = store.locate_resumable(storage_path=storage_path, run_name="run1")
        assert located == "/exp/run1_dir"

    def test_distinct_run_names_are_isolated(self, tmp_path):
        """Markers are keyed by run name; one run does not shadow another."""
        store = FsspecExperimentStore()
        storage_path = str(tmp_path)
        store.track(storage_path=storage_path, run_name="a", experiment_path="/exp/a")
        store.track(storage_path=storage_path, run_name="b", experiment_path="/exp/b")
        assert (
            store.locate_resumable(storage_path=storage_path, run_name="a") == "/exp/a"
        )
        assert (
            store.locate_resumable(storage_path=storage_path, run_name="b") == "/exp/b"
        )

    def test_track_overwrites_previous_marker(self, tmp_path):
        """A second ``track`` for the same identity replaces the marker."""
        store = FsspecExperimentStore()
        storage_path = str(tmp_path)
        store.track(
            storage_path=storage_path, run_name="run1", experiment_path="/exp/old"
        )
        store.track(
            storage_path=storage_path, run_name="run1", experiment_path="/exp/new"
        )
        assert (
            store.locate_resumable(storage_path=storage_path, run_name="run1")
            == "/exp/new"
        )


class TestNothingToResume:
    """``locate_resumable`` returns ``None`` without raising on the sad paths."""

    def test_missing_marker_returns_none(self, tmp_path):
        """No marker file → nothing to resume."""
        store = FsspecExperimentStore()
        assert (
            store.locate_resumable(storage_path=str(tmp_path), run_name="absent")
            is None
        )

    def test_corrupt_marker_returns_none(self, tmp_path):
        """A marker with invalid JSON → ``None`` (swallowed), not a crash."""
        store = FsspecExperimentStore()
        marker_dir = tmp_path / ".michelangelo_resume"
        marker_dir.mkdir()
        (marker_dir / "run1.json").write_text("{ not valid json")
        assert (
            store.locate_resumable(storage_path=str(tmp_path), run_name="run1") is None
        )

    def test_marker_without_experiment_path_returns_none(self, tmp_path):
        """A well-formed marker missing ``experiment_path`` → ``None``."""
        store = FsspecExperimentStore()
        marker_dir = tmp_path / ".michelangelo_resume"
        marker_dir.mkdir()
        (marker_dir / "run1.json").write_text(json.dumps({"run_name": "run1"}))
        assert (
            store.locate_resumable(storage_path=str(tmp_path), run_name="run1") is None
        )

    def test_locate_swallows_filesystem_errors(self):
        """A raising filesystem layer is swallowed; ``locate`` returns ``None``."""
        store = FsspecExperimentStore()
        with patch(f"{_STORE_MODULE}.url_to_fs", side_effect=OSError("boom")):
            assert store.locate_resumable(storage_path="/root", run_name="run1") is None


class TestTrackNeverRaises:
    """``track`` is best-effort: a write failure must never propagate."""

    def test_track_swallows_filesystem_errors(self):
        """A raising filesystem layer during ``track`` does not propagate."""
        store = FsspecExperimentStore()
        with patch(f"{_STORE_MODULE}.url_to_fs", side_effect=OSError("boom")):
            # Must not raise.
            store.track(
                storage_path="/root", run_name="run1", experiment_path="/exp/dir"
            )


class TestStorageOptions:
    """``storage_options`` are forwarded to fsspec and the store stays picklable."""

    def test_storage_options_forwarded(self, tmp_path):
        """Constructor ``storage_options`` are passed through to ``url_to_fs``."""
        store = FsspecExperimentStore(storage_options={"anon": True})
        with patch(f"{_STORE_MODULE}.url_to_fs", side_effect=OSError("stop")) as u2fs:
            store.locate_resumable(storage_path=str(tmp_path), run_name="run1")
        _, kwargs = u2fs.call_args
        assert kwargs == {"anon": True}

    def test_store_is_picklable(self):
        """The store holds only a plain dict, so it survives pickling to workers."""
        import pickle

        restored = pickle.loads(pickle.dumps(FsspecExperimentStore({"anon": True})))
        assert isinstance(restored, FsspecExperimentStore)
        assert restored._storage_options == {"anon": True}
