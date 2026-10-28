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


class DemoActionRegistryTest(TestCase):
    """Parser registration and dispatch share a single `_demo_actions` table."""

    def test_parser_accepts_every_registered_demo_action(self):
        """Every key in `_demo_actions` is a valid `ma sandbox demo` subcommand."""
        parser = argparse.ArgumentParser()
        sandbox.init_arguments(parser)
        for action in sandbox._demo_actions():
            ns = parser.parse_args(["demo", action])
            self.assertEqual(ns.demo_action, action)

    def test_parser_rejects_unknown_demo_action(self):
        """Argparse rejects a demo action that is not in the registry."""
        parser = argparse.ArgumentParser()
        sandbox.init_arguments(parser)
        with self.assertRaises(SystemExit):
            parser.parse_args(["demo", "not-a-real-demo"])

    def test_create_demo_crs_dispatches_registered_actions(self):
        """Each registered action reaches its handler after the shared project CR."""
        called = []
        handlers = {
            action: (lambda a=action: called.append(a))
            for action in sandbox._demo_actions()
        }
        project = {
            "apiVersion": "michelangelo.api/v2",
            "kind": "Project",
            "metadata": {"name": "demo", "namespace": "ma-dev-test"},
        }
        with tempfile.TemporaryDirectory() as tmp_dir:
            demo_dir = Path(tmp_dir) / "demo"
            demo_dir.mkdir()
            with open(demo_dir / "project.yaml", "w") as f:
                yaml.safe_dump(project, f)

            with (
                patch.object(sandbox, "_dir", Path(tmp_dir)),
                patch.object(sandbox, "_assert_sandbox_cluster_running"),
                patch.object(sandbox, "_ensure_namespace_exists"),
                patch.object(sandbox, "_kube_apply"),
                patch.object(
                    sandbox,
                    "_demo_actions",
                    return_value={
                        action: sandbox._DemoAction(help=action, handler=handler)
                        for action, handler in handlers.items()
                    },
                ),
            ):
                for action in handlers:
                    sandbox._create_demo_crs(argparse.Namespace(demo_action=action))

        self.assertEqual(called, list(handlers))

    def test_unknown_action_rejected_before_cluster_work(self):
        """An unknown action fails before any cluster or apply work."""
        with (
            patch.object(sandbox, "_assert_sandbox_cluster_running") as mock_running,
            patch.object(sandbox, "_kube_apply") as mock_apply,
            self.assertRaises(ValueError) as ctx,
        ):
            sandbox._create_demo_crs(argparse.Namespace(demo_action="bogus"))
        self.assertIn("bogus", str(ctx.exception))
        mock_running.assert_not_called()
        mock_apply.assert_not_called()
