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
        """No Michelangelo CRDs in the cluster -> snapshot dir has no CRD yaml files."""
        with tempfile.TemporaryDirectory() as tmp_dir:
            with (
                patch.object(sandbox, "_dir", Path(tmp_dir)),
                patch.object(sandbox, "_assert_sandbox_cluster_running"),
                patch.object(sandbox.subprocess, "run") as mock_run,
                patch.object(
                    sandbox, "_snapshot_capture_helm_values", return_value=False
                ),
            ):
                mock_run.return_value = Mock(stdout="", returncode=0)
                sandbox._snapshot_create(argparse.Namespace())

            snapshots_dir = Path(tmp_dir) / "snapshots"
            written = list(snapshots_dir.glob("*/*.yaml"))
            self.assertEqual(written, [])

    def test_create_writes_stripped_resource_and_keeps_status(self):
        """Volatile fields are stripped from a captured resource, status kept."""
        raw_pipeline_list = yaml.safe_dump(
            {
                "apiVersion": "v1",
                "kind": "List",
                "items": [
                    {
                        "apiVersion": "michelangelo.api/v2",
                        "kind": "Pipeline",
                        "metadata": {
                            "name": "eval-pipeline",
                            "namespace": "ma-dev-test",
                            "uid": "c3eb9f45-0748-41f0-9415-f46b8b01ac5c",
                            "resourceVersion": "1775",
                            "creationTimestamp": "2026-07-30T20:45:45Z",
                            "generation": 1,
                            "ownerReferences": [{"name": "some-owner"}],
                            "annotations": {
                                "kubectl.kubernetes.io/last-applied-configuration": (
                                    "{}"
                                ),
                                "michelangelo/MetadataStoragePrimaryKey": (
                                    "c3eb9f45-0748-41f0-9415-f46b8b01ac5c"
                                ),
                                "michelangelo/worker_queue": "default",
                            },
                        },
                        "spec": {"type": "PIPELINE_TYPE_EVAL"},
                        "status": {"state": "PIPELINE_STATE_READY"},
                    }
                ],
            }
        )

        def fake_run(args, **kwargs):
            if "api-resources" in args:
                return Mock(stdout="pipelines.michelangelo.api\n", returncode=0)
            if args[:3] == ["helm", "get", "values"]:
                return Mock(stdout="{}\n", returncode=0)
            return Mock(stdout=raw_pipeline_list, returncode=0)

        with tempfile.TemporaryDirectory() as tmp_dir:
            with (
                patch.object(sandbox, "_dir", Path(tmp_dir)),
                patch.object(sandbox, "_assert_sandbox_cluster_running"),
                patch.object(sandbox.subprocess, "run", side_effect=fake_run),
            ):
                sandbox._snapshot_create(argparse.Namespace())

            written_files = list(Path(tmp_dir).glob("snapshots/*/pipelines.yaml"))
            self.assertEqual(len(written_files), 1)

            with open(written_files[0]) as f:
                written = yaml.safe_load(f)

            item = written["items"][0]
            self.assertNotIn("uid", item["metadata"])
            self.assertNotIn("resourceVersion", item["metadata"])
            self.assertNotIn("creationTimestamp", item["metadata"])
            self.assertNotIn("generation", item["metadata"])
            self.assertNotIn("ownerReferences", item["metadata"])
            annotations = item["metadata"]["annotations"]
            self.assertNotIn(
                "kubectl.kubernetes.io/last-applied-configuration", annotations
            )
            self.assertNotIn("michelangelo/MetadataStoragePrimaryKey", annotations)
            self.assertEqual(annotations, {"michelangelo/worker_queue": "default"})
            self.assertEqual(item["spec"], {"type": "PIPELINE_TYPE_EVAL"})
            self.assertEqual(item["status"], {"state": "PIPELINE_STATE_READY"})

    def test_create_captures_and_redacts_helm_values(self):
        """helm-values.yaml is written with rootPassword redacted."""
        helm_values = yaml.safe_dump(
            {
                "workflow": {"engine": "cadence"},
                "metadataStorage": {
                    "host": "mysql",
                    "rootPassword": "super-secret",
                },
            }
        )

        def fake_run(args, **kwargs):
            if "api-resources" in args:
                return Mock(stdout="", returncode=0)
            if args[:3] == ["helm", "get", "values"]:
                return Mock(stdout=helm_values, returncode=0)
            return Mock(stdout="", returncode=0)

        with tempfile.TemporaryDirectory() as tmp_dir:
            with (
                patch.object(sandbox, "_dir", Path(tmp_dir)),
                patch.object(sandbox, "_assert_sandbox_cluster_running"),
                patch.object(sandbox.subprocess, "run", side_effect=fake_run),
            ):
                sandbox._snapshot_create(argparse.Namespace())

            written_files = list(Path(tmp_dir).glob("snapshots/*/helm-values.yaml"))
            self.assertEqual(len(written_files), 1)

            with open(written_files[0]) as f:
                written = yaml.safe_load(f)

            self.assertEqual(written["metadataStorage"]["rootPassword"], "<redacted>")
            self.assertEqual(written["metadataStorage"]["host"], "mysql")
            self.assertEqual(written["workflow"], {"engine": "cadence"})

    def test_create_continues_when_helm_get_values_fails(self):
        """A broken/missing Helm release warns and skips settings, doesn't abort."""

        def fake_run(args, **kwargs):
            if "api-resources" in args:
                return Mock(stdout="", returncode=0)
            if args[:3] == ["helm", "get", "values"]:
                return Mock(stdout="", stderr="release: not found", returncode=1)
            return Mock(stdout="", returncode=0)

        with tempfile.TemporaryDirectory() as tmp_dir:
            with (
                patch.object(sandbox, "_dir", Path(tmp_dir)),
                patch.object(sandbox, "_assert_sandbox_cluster_running"),
                patch.object(sandbox.subprocess, "run", side_effect=fake_run),
            ):
                # Should not raise even though the helm release is missing.
                sandbox._snapshot_create(argparse.Namespace())

            written_files = list(Path(tmp_dir).glob("snapshots/*/helm-values.yaml"))
            self.assertEqual(written_files, [])


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

    def test_restore_never_applies_helm_values_file(self):
        """helm-values.yaml is not a manifest and must never be kubectl-applied."""
        with tempfile.TemporaryDirectory() as tmp_dir:
            snapshot_dir = Path(tmp_dir) / "snapshots" / "20260101-000000"
            snapshot_dir.mkdir(parents=True)
            self._write_kind_file(snapshot_dir, "pipelines.yaml", [])
            with open(snapshot_dir / "helm-values.yaml", "w") as f:
                yaml.safe_dump({"workflow": {"engine": "cadence"}}, f)

            with (
                patch.object(sandbox, "_dir", Path(tmp_dir)),
                patch.object(sandbox, "_assert_sandbox_cluster_running"),
                patch.object(sandbox, "_kube_apply") as mock_apply,
                patch.object(sandbox, "_helm_get_values", return_value=None),
            ):
                sandbox._snapshot_restore(
                    argparse.Namespace(timestamp="20260101-000000")
                )

            applied_paths = {call.args[0].name for call in mock_apply.call_args_list}
            self.assertEqual(applied_paths, {"pipelines.yaml"})

    def test_restore_prints_diff_between_captured_and_live_helm_values(self):
        """A settings difference between snapshot and live sandbox is surfaced."""
        with tempfile.TemporaryDirectory() as tmp_dir:
            snapshot_dir = Path(tmp_dir) / "snapshots" / "20260101-000000"
            snapshot_dir.mkdir(parents=True)
            with open(snapshot_dir / "helm-values.yaml", "w") as f:
                yaml.safe_dump(
                    {"metadataStorage": {"enable": False}}, f, sort_keys=False
                )

            with (
                patch.object(sandbox, "_dir", Path(tmp_dir)),
                patch.object(sandbox, "_assert_sandbox_cluster_running"),
                patch.object(sandbox, "_kube_apply"),
                patch.object(
                    sandbox,
                    "_helm_get_values",
                    return_value={"metadataStorage": {"enable": True}},
                ),
                patch("builtins.print") as mock_print,
            ):
                sandbox._snapshot_restore(
                    argparse.Namespace(timestamp="20260101-000000")
                )

            printed = "\n".join(str(call.args[0]) for call in mock_print.call_args_list)
            self.assertIn("differ", printed)
            self.assertIn("enable: false", printed)
            self.assertIn("enable: true", printed)

    def test_restore_diff_ignores_redacted_password_noise(self):
        """A redacted field never shows up as a spurious diff on its own.

        The captured file always has rootPassword redacted, but the live
        release never does — without redacting the live side too, every
        restore would show a rootPassword diff line even with no real
        settings drift.
        """
        with tempfile.TemporaryDirectory() as tmp_dir:
            snapshot_dir = Path(tmp_dir) / "snapshots" / "20260101-000000"
            snapshot_dir.mkdir(parents=True)
            with open(snapshot_dir / "helm-values.yaml", "w") as f:
                yaml.safe_dump(
                    {"metadataStorage": {"rootPassword": "<redacted>"}},
                    f,
                    sort_keys=False,
                )

            with (
                patch.object(sandbox, "_dir", Path(tmp_dir)),
                patch.object(sandbox, "_assert_sandbox_cluster_running"),
                patch.object(sandbox, "_kube_apply"),
                patch.object(
                    sandbox,
                    "_helm_get_values",
                    return_value={"metadataStorage": {"rootPassword": "root"}},
                ),
                patch("builtins.print") as mock_print,
            ):
                sandbox._snapshot_restore(
                    argparse.Namespace(timestamp="20260101-000000")
                )

            printed = "\n".join(str(call.args[0]) for call in mock_print.call_args_list)
            self.assertIn("match the live sandbox", printed)
            self.assertNotIn("differ from", printed)

    def test_restore_reports_no_helm_values_file_silently(self):
        """An old snapshot lacking helm-values.yaml restores with no settings diff."""
        with tempfile.TemporaryDirectory() as tmp_dir:
            snapshot_dir = Path(tmp_dir) / "snapshots" / "20260101-000000"
            snapshot_dir.mkdir(parents=True)
            self._write_kind_file(snapshot_dir, "pipelines.yaml", [])

            with (
                patch.object(sandbox, "_dir", Path(tmp_dir)),
                patch.object(sandbox, "_assert_sandbox_cluster_running"),
                patch.object(sandbox, "_kube_apply"),
                patch.object(sandbox, "_helm_get_values") as mock_helm_values,
            ):
                sandbox._snapshot_restore(
                    argparse.Namespace(timestamp="20260101-000000")
                )

            mock_helm_values.assert_not_called()
