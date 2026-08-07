"""Scenario-level tests for `ma sandbox snapshot`."""

import argparse
import tempfile
from pathlib import Path
from unittest import TestCase
from unittest.mock import Mock, patch

import yaml

from michelangelo.cli.sandbox import sandbox


class SnapshotCreateTest(TestCase):
    """Test cases for `ma sandbox snapshot create`."""

    def test_create_on_empty_sandbox_writes_no_files(self):
        """No Michelangelo CRDs in the cluster -> snapshot dir has no yaml files."""
        with tempfile.TemporaryDirectory() as tmp_dir:
            with (
                patch.object(sandbox, "_dir", Path(tmp_dir)),
                patch.object(sandbox, "_assert_sandbox_cluster_running"),
                patch.object(sandbox.subprocess, "run") as mock_run,
            ):
                mock_run.return_value = Mock(stdout="", returncode=0)
                sandbox._snapshot_create(argparse.Namespace())

            snapshots_dir = Path(tmp_dir) / "snapshots"
            written = list(snapshots_dir.glob("*/*.yaml"))
            self.assertEqual(written, [])


class SnapshotRestoreTest(TestCase):
    """Test cases for `ma sandbox snapshot restore`."""

    def _write_kind_file(self, snapshot_dir: Path, filename: str, items: list):
        with open(snapshot_dir / filename, "w") as f:
            yaml.safe_dump({"apiVersion": "v1", "kind": "List", "items": items}, f)

    def test_restore_resource_namespace_without_matching_project(self):
        """A resource whose namespace has no Project in the snapshot still applies."""
        with tempfile.TemporaryDirectory() as tmp_dir:
            snapshot_dir = Path(tmp_dir) / "snapshots" / "20260101-000000"
            snapshot_dir.mkdir(parents=True)
            self._write_kind_file(
                snapshot_dir,
                "projects.yaml",
                [{"metadata": {"name": "known", "namespace": "known-ns"}}],
            )
            self._write_kind_file(
                snapshot_dir,
                "pipelines.yaml",
                [{"metadata": {"name": "orphan-pipeline", "namespace": "orphan-ns"}}],
            )

            with (
                patch.object(sandbox, "_dir", Path(tmp_dir)),
                patch.object(sandbox, "_assert_sandbox_cluster_running"),
                patch.object(sandbox, "_ensure_namespace_exists") as mock_ensure_ns,
                patch.object(sandbox, "_kube_apply") as mock_apply,
            ):
                sandbox._snapshot_restore(
                    argparse.Namespace(timestamp="20260101-000000")
                )

            # Only the namespace backed by a Project is proactively ensured —
            # the orphan namespace is left to already exist or to fail at
            # apply time, not treated as a restore-blocking error.
            mock_ensure_ns.assert_called_once_with("known-ns")

            applied_paths = {call.args[0].name for call in mock_apply.call_args_list}
            self.assertEqual(applied_paths, {"projects.yaml", "pipelines.yaml"})

    def test_restore_with_no_projects_file_skips_namespace_ensure(self):
        """No projects.yaml in snapshot -> everything applies, no namespace setup."""
        with tempfile.TemporaryDirectory() as tmp_dir:
            snapshot_dir = Path(tmp_dir) / "snapshots" / "20260101-000000"
            snapshot_dir.mkdir(parents=True)
            self._write_kind_file(
                snapshot_dir,
                "pipelines.yaml",
                [{"metadata": {"name": "some-pipeline", "namespace": "some-ns"}}],
            )

            with (
                patch.object(sandbox, "_dir", Path(tmp_dir)),
                patch.object(sandbox, "_assert_sandbox_cluster_running"),
                patch.object(sandbox, "_ensure_namespace_exists") as mock_ensure_ns,
                patch.object(sandbox, "_kube_apply") as mock_apply,
            ):
                sandbox._snapshot_restore(
                    argparse.Namespace(timestamp="20260101-000000")
                )

            mock_ensure_ns.assert_not_called()
            applied_paths = {call.args[0].name for call in mock_apply.call_args_list}
            self.assertEqual(applied_paths, {"pipelines.yaml"})

    def test_restore_accepts_full_path_not_just_bare_timestamp(self):
        """A full/relative path (e.g. copy-pasted from `create`'s output) works too."""
        with tempfile.TemporaryDirectory() as tmp_dir:
            snapshot_dir = Path(tmp_dir) / "snapshots" / "20260101-000000"
            snapshot_dir.mkdir(parents=True)
            self._write_kind_file(snapshot_dir, "pipelines.yaml", [])

            full_path = "michelangelo/cli/sandbox/snapshots/20260101-000000"
            with (
                patch.object(sandbox, "_dir", Path(tmp_dir)),
                patch.object(sandbox, "_assert_sandbox_cluster_running"),
                patch.object(sandbox, "_kube_apply") as mock_apply,
            ):
                sandbox._snapshot_restore(argparse.Namespace(timestamp=full_path))

            mock_apply.assert_called_once()

    def test_restore_missing_snapshot_directory_errors(self):
        """Restoring a nonexistent timestamp exits with an error, not a crash."""
        with (
            tempfile.TemporaryDirectory() as tmp_dir,
            patch.object(sandbox, "_dir", Path(tmp_dir)),
            patch.object(sandbox, "_assert_sandbox_cluster_running"),
            patch.object(sandbox, "_err_exit", side_effect=SystemExit(1)) as mock_err,
            self.assertRaises(SystemExit),
        ):
            sandbox._snapshot_restore(argparse.Namespace(timestamp="does-not-exist"))
        mock_err.assert_called_once()
