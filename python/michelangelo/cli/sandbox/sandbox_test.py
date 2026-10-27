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
                return Mock(stdout="pipelines.michelangelo.api\n")
            return Mock(stdout=raw_pipeline_list)

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


class EnsureCadenceDomainTest(TestCase):
    """Test cases for `_ensure_cadence_domain`."""

    def test_registers_domain_for_cadence(self):
        """The cadence engine gets its domain registered."""
        with patch.object(sandbox, "_create_cadence_domain") as mock_create:
            sandbox._ensure_cadence_domain(argparse.Namespace(workflow="cadence"))
        mock_create.assert_called_once()

    def test_skips_registration_for_temporal(self):
        """Temporal has no Cadence domain to register."""
        with patch.object(sandbox, "_create_cadence_domain") as mock_create:
            sandbox._ensure_cadence_domain(argparse.Namespace(workflow="temporal"))
        mock_create.assert_not_called()


class CreateCadenceDomainTest(TestCase):
    """Test cases for `_create_cadence_domain` idempotency."""

    def test_already_registered_domain_is_not_an_error(self):
        """A domain that already exists is skipped without retrying or exiting."""
        with (
            patch.object(sandbox, "_exec"),
            patch.object(sandbox.subprocess, "run") as mock_run,
            patch.object(sandbox.time, "sleep") as mock_sleep,
        ):
            mock_run.return_value = Mock(
                returncode=1, stdout="", stderr="Domain already exists"
            )
            sandbox._create_cadence_domain([])

        mock_run.assert_called_once()
        mock_sleep.assert_not_called()


class SyncCadenceDomainTest(TestCase):
    """Test cases for domain registration in `ma sandbox sync`."""

    def _run_sync(self, helm_status_stdout: str) -> Mock:
        """Run `_sync` against a mocked cluster, returning an ordered call log."""
        calls = Mock()
        run_results = {
            "cluster": Mock(returncode=0, stdout=""),
            "status": Mock(returncode=0, stdout=helm_status_stdout),
        }

        def fake_run(args, **_):
            return run_results[
                "status" if args[:2] == ["helm", "status"] else "cluster"
            ]

        with (
            patch.object(sandbox.subprocess, "run", side_effect=fake_run),
            patch.object(sandbox, "_exec"),
            patch.object(sandbox, "_refresh_mysql_schema"),
            patch.object(sandbox, "_ensure_credentials_secret"),
            patch.object(sandbox, "_helm_ensure_repos"),
            patch.object(sandbox, "_build_helm_set_args", return_value=[]),
            patch.object(sandbox, "_helm_delete_services"),
            patch.object(sandbox, "_helm_adopt_orphaned_resources"),
            patch.object(sandbox, "_ensure_cadence_domain", calls.ensure),
            patch.object(sandbox, "_helm_wait", calls.wait),
        ):
            sandbox._sync(argparse.Namespace(workflow="cadence"))
        return calls

    def test_domain_registered_before_waiting_after_upgrade(self):
        """A healthy release upgrade registers the domain before the wait."""
        calls = self._run_sync('{"info":{"status":"deployed"}}')
        names = [c[0] for c in calls.mock_calls]
        self.assertEqual(names, ["ensure", "wait"])

    def test_domain_registered_before_waiting_after_reinstall(self):
        """A failed release is reinstalled and still gets the domain registered."""
        calls = self._run_sync('{"info":{"status":"failed"}}')
        names = [c[0] for c in calls.mock_calls]
        self.assertEqual(names, ["ensure", "wait"])
