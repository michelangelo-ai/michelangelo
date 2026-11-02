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


class EnsureObjectstoreServiceTest(TestCase):
    """Test cases for the generated `objectstore` Service."""

    def _render(self, object_store):
        """Run _ensure_objectstore_service and return the applied manifests."""
        captured = {}

        def fake_apply(path, context=None):
            captured["docs"] = list(yaml.safe_load_all(Path(path).read_text()))

        with (
            patch.object(sandbox.subprocess, "run") as mock_run,
            patch.object(sandbox, "_kube_apply", side_effect=fake_apply),
        ):
            mock_run.return_value = Mock(stdout="", returncode=0)
            sandbox._ensure_objectstore_service(
                argparse.Namespace(object_store=object_store)
            )
        return {doc["metadata"]["name"]: doc for doc in captured["docs"]}

    def test_seaweedfs_routes_9090_to_filer_ui(self):
        """SeaweedFS: 9090 targets the filer (8888); S3 port 9091 is unchanged."""
        docs = self._render("seaweedfs")
        svc = docs["objectstore"]
        ports = {p["port"]: p for p in svc["spec"]["ports"]}
        self.assertEqual(svc["spec"]["selector"], {"app": "seaweedfs"})
        self.assertEqual(ports[9090]["targetPort"], 8888)
        self.assertEqual(ports[9090]["nodePort"], 30008)
        self.assertNotIn("targetPort", ports[9091])
        self.assertEqual(ports[9091]["nodePort"], 30007)
        # The ClusterIP `minio` alias keeps plain pass-through ports.
        for p in docs["minio"]["spec"]["ports"]:
            self.assertNotIn("targetPort", p)

    def test_minio_keeps_9090_console(self):
        """MinIO: no targetPort override, so 9090 stays the MinIO Console."""
        svc = self._render("minio")["objectstore"]
        self.assertEqual(svc["spec"]["selector"], {"app": "minio"})
        for p in svc["spec"]["ports"]:
            self.assertNotIn("targetPort", p)
        self.assertEqual([p["port"] for p in svc["spec"]["ports"]], [9090, 9091])


def _fake_kubectl(svc_selector="", pods=(), minio_svc_type="", fail=()):
    """Build a subprocess.run fake answering the object-store detection calls.

    Lookups use `--ignore-not-found=true`, so an absent object is a successful
    empty result. Names in `fail` ("objectstore", "minio-svc", or a Pod name)
    make that lookup exit nonzero instead.
    """

    def answer(key, stdout):
        if key in fail:
            return Mock(stdout="", stderr="connection refused", returncode=1)
        return Mock(stdout=stdout, stderr="", returncode=0)

    def fake_run(args, **kwargs):
        if "--ignore-not-found=true" not in args:
            raise AssertionError(f"lookup without --ignore-not-found: {args}")
        if args[:4] == ["kubectl", "get", "svc", "objectstore"]:
            return answer("objectstore", svc_selector)
        if args[:4] == ["kubectl", "get", "svc", "minio"]:
            return answer("minio-svc", minio_svc_type)
        if args[:3] == ["kubectl", "get", "pod"]:
            name = args[3]
            return answer(name, f"pod/{name}" if name in pods else "")
        raise AssertionError(f"unexpected command: {args}")

    return fake_run


class ObjectStoreArgumentsTest(TestCase):
    """`--object-store` has no static default so the backend can be resolved."""

    def _parse(self, *argv):
        parser = argparse.ArgumentParser()
        sandbox.init_arguments(parser)
        return parser.parse_args(list(argv))

    def test_create_default_is_none(self):
        """`create` without the flag leaves the backend unresolved."""
        self.assertIsNone(self._parse("create").object_store)

    def test_sync_default_is_none(self):
        """`sync` without the flag leaves the backend unresolved."""
        self.assertIsNone(self._parse("sync").object_store)

    def test_explicit_values_are_accepted(self):
        """Both backends are still accepted explicitly."""
        for action in ("create", "sync"):
            for backend in ("minio", "seaweedfs"):
                ns = self._parse(action, "--object-store", backend)
                self.assertEqual(ns.object_store, backend)


class ResolveObjectStoreTest(TestCase):
    """Test cases for `_resolve_object_store`."""

    def _resolve(self, flag, cluster_exists, fake=None):
        ns = argparse.Namespace(object_store=flag)
        with patch.object(sandbox.subprocess, "run", side_effect=fake) as mock_run:
            result = sandbox._resolve_object_store(ns, cluster_exists)
        return result, mock_run

    def test_flag_on_new_sandbox_skips_probing(self):
        """`create` returns an explicit flag as is and reads nothing."""
        for flag in ("minio", "seaweedfs"):
            result, mock_run = self._resolve(flag, False)
            self.assertEqual(result, flag)
            mock_run.assert_not_called()

    def test_flag_matching_selector_is_accepted(self):
        """An explicit flag that matches the Service selector passes."""
        fake = _fake_kubectl(svc_selector="minio", pods=("minio",))
        self.assertEqual(self._resolve("minio", True, fake)[0], "minio")

    def test_flag_without_selector_is_accepted(self):
        """No `objectstore` Service yet: the explicit flag wins."""
        self.assertEqual(self._resolve("minio", True, _fake_kubectl())[0], "minio")

    def test_flag_conflicting_with_selector_is_refused(self):
        """Selector minio with its Pod gone still refuses --object-store seaweedfs."""
        fake = _fake_kubectl(svc_selector="minio", pods=())
        with self.assertRaises(SystemExit) as ctx:
            self._resolve("seaweedfs", True, fake)
        message = str(ctx.exception)
        self.assertIn("runs the minio object store", message)
        self.assertIn("--object-store minio", message)
        self.assertIn("ma sandbox delete", message)

    def test_flag_with_failed_selector_lookup_exits(self):
        """An explicit flag does not skip the fail-closed selector lookup."""
        fake = _fake_kubectl(fail=("objectstore",))
        with self.assertRaises(SystemExit) as ctx:
            self._resolve("seaweedfs", True, fake)
        self.assertIn("Nothing was changed", str(ctx.exception))

    def test_failed_selector_lookup_exits_without_fallback(self):
        """A kubectl failure on the selector is not read as "no Service"."""
        fake = _fake_kubectl(pods=("minio",), fail=("objectstore",))
        with self.assertRaises(SystemExit) as ctx:
            self._resolve(None, True, fake)
        message = str(ctx.exception)
        self.assertIn("Could not read the object-store state", message)
        self.assertIn("connection refused", message)

    def test_failed_pod_lookup_exits(self):
        """A kubectl failure on a Pod lookup is not read as "Pod absent"."""
        fake = _fake_kubectl(fail=("minio",))
        with self.assertRaises(SystemExit) as ctx:
            self._resolve(None, True, fake)
        self.assertIn("Could not read the object-store state", str(ctx.exception))

    def test_failed_legacy_service_lookup_exits(self):
        """A kubectl failure on the legacy `minio` Service lookup exits too."""
        fake = _fake_kubectl(fail=("minio-svc",))
        with self.assertRaises(SystemExit):
            self._resolve(None, True, fake)

    def test_no_cluster_defaults_to_seaweedfs(self):
        """A new sandbox gets SeaweedFS and nothing is detected."""
        result, mock_run = self._resolve(None, False)
        self.assertEqual(result, "seaweedfs")
        mock_run.assert_not_called()

    def test_service_selector_is_used_first(self):
        """The Service selector wins even when the Pod is gone."""
        fake = _fake_kubectl(svc_selector="minio", pods=())
        self.assertEqual(self._resolve(None, True, fake)[0], "minio")
        fake = _fake_kubectl(svc_selector="seaweedfs", pods=("minio",))
        self.assertEqual(self._resolve(None, True, fake)[0], "seaweedfs")

    def test_pod_fallback(self):
        """Without a Service selector the existing Pod decides."""
        fake = _fake_kubectl(pods=("minio",))
        self.assertEqual(self._resolve(None, True, fake)[0], "minio")
        fake = _fake_kubectl(pods=("seaweedfs",))
        self.assertEqual(self._resolve(None, True, fake)[0], "seaweedfs")

    def test_unknown_selector_value_falls_through_to_pods(self):
        """A selector that names neither backend is ignored."""
        fake = _fake_kubectl(svc_selector="other", pods=("minio",))
        self.assertEqual(self._resolve(None, True, fake)[0], "minio")

    def test_legacy_nodeport_service_means_minio(self):
        """A pre-rename NodePort `minio` Service identifies a MinIO cluster."""
        fake = _fake_kubectl(minio_svc_type="NodePort")
        self.assertEqual(self._resolve(None, True, fake)[0], "minio")

    def test_clusterip_minio_service_is_not_legacy(self):
        """The ClusterIP alias is not evidence of a MinIO backend."""
        fake = _fake_kubectl(minio_svc_type="ClusterIP")
        self.assertEqual(self._resolve(None, True, fake)[0], "seaweedfs")

    def test_both_pods_present_exits(self):
        """Two backend Pods and no hint is refused with recreate guidance."""
        fake = _fake_kubectl(pods=("minio", "seaweedfs"))
        with self.assertRaises(SystemExit) as ctx:
            self._resolve(None, True, fake)
        self.assertIn("ma sandbox delete", str(ctx.exception))

    def test_nothing_found_defaults_to_seaweedfs(self):
        """Empty successful lookups mean absent and walk the full fallback order."""
        result, mock_run = self._resolve(None, True, _fake_kubectl())
        self.assertEqual(result, "seaweedfs")
        looked_up = [c.args[0][2:4] for c in mock_run.call_args_list]
        self.assertEqual(
            looked_up,
            [
                ["svc", "objectstore"],
                ["pod", "minio"],
                ["pod", "seaweedfs"],
                ["svc", "minio"],
            ],
        )


class PodExistsTest(TestCase):
    """Test cases for `_pod_exists` and `_check_object_store_backend`."""

    def test_present_and_absent(self):
        """Output means present; an empty successful result means absent."""
        fake = _fake_kubectl(pods=("minio",))
        with patch.object(sandbox.subprocess, "run", side_effect=fake):
            self.assertTrue(sandbox._pod_exists("minio"))
            self.assertFalse(sandbox._pod_exists("seaweedfs"))

    def test_lookup_failure_exits(self):
        """A nonzero kubectl exit is unknown, not absent."""
        fake = _fake_kubectl(fail=("minio",))
        with (
            patch.object(sandbox.subprocess, "run", side_effect=fake),
            self.assertRaises(SystemExit),
        ):
            sandbox._pod_exists("minio")

    def test_check_backend_exits_when_pod_lookup_fails(self):
        """The mismatch check does not pass when it cannot see the Pods."""
        fake = _fake_kubectl(fail=("minio",))
        with (
            patch.object(sandbox.subprocess, "run", side_effect=fake),
            self.assertRaises(SystemExit) as ctx,
        ):
            sandbox._check_object_store_backend(
                argparse.Namespace(object_store="seaweedfs")
            )
        self.assertIn("Could not read the object-store state", str(ctx.exception))


class ObjectStoreWarningTest(TestCase):
    """Test cases for the MinIO deprecation notice."""

    def test_minio_prints_notice(self):
        """The notice is printed once for minio and names the migration steps."""
        with patch("builtins.print") as mock_print:
            sandbox._warn_if_minio("minio")
        mock_print.assert_called_once()
        message = mock_print.call_args.args[0]
        self.assertIn("deprecated", message)
        self.assertIn("ma sandbox delete", message)
        self.assertIn("NOT carried over", message)
        self.assertNotIn("localhost:9090", message)

    def test_seaweedfs_prints_nothing(self):
        """SeaweedFS gets no notice."""
        with patch("builtins.print") as mock_print:
            sandbox._warn_if_minio("seaweedfs")
        mock_print.assert_not_called()


class ObjectStoreSyncCreateFlowTest(TestCase):
    """`create` and `sync` resolve the backend at the right point."""

    def test_create_resolves_seaweedfs_and_warns_for_minio_flag(self):
        """`create` with no flag resolves to seaweedfs without a notice."""
        ns = argparse.Namespace(object_store=None, exclude=[], workflow="cadence")
        with (
            patch.object(sandbox, "_exec"),
            patch.object(sandbox, "_helm_chart_ports", return_value=[]),
            patch.object(sandbox, "_deploy_services") as deploy,
            patch.object(sandbox, "_warn_if_minio") as warn,
        ):
            sandbox._create(ns)
        self.assertEqual(ns.object_store, "seaweedfs")
        warn.assert_called_once_with("seaweedfs")
        deploy.assert_called_once_with(ns)

    def test_sync_detects_backend_before_mysql_step(self):
        """Detection and the mismatch check run before `_ensure_mysql_running`."""
        ns = argparse.Namespace(object_store=None)
        calls = []

        def record(name, result=None):
            return Mock(side_effect=lambda *a, **k: calls.append(name) or result)

        stop = RuntimeError("stop after the MySQL step")
        with (
            patch.object(sandbox.subprocess, "run", return_value=Mock(returncode=0)),
            patch.object(sandbox, "_exec"),
            patch.object(
                sandbox,
                "_resolve_object_store",
                new=record("resolve", "minio"),
            ),
            patch.object(sandbox, "_check_object_store_backend", new=record("check")),
            patch.object(sandbox, "_warn_if_minio", new=record("warn")),
            patch.object(sandbox, "_ensure_mysql_running", side_effect=stop),
            self.assertRaises(RuntimeError),
        ):
            sandbox._sync(ns)
        self.assertEqual(ns.object_store, "minio")
        self.assertEqual(calls, ["resolve", "check", "warn"])

    def test_sync_without_cluster_falls_back_to_create(self):
        """No cluster means a full create, which resolves to seaweedfs."""
        ns = argparse.Namespace(object_store=None)
        with (
            patch.object(sandbox.subprocess, "run", return_value=Mock(returncode=1)),
            patch.object(sandbox, "_create") as create,
        ):
            sandbox._sync(ns)
        create.assert_called_once_with(ns)

    def test_sync_without_cluster_creates_without_kubectl_lookups(self):
        """No cluster: `_create` resolves seaweedfs and no kubectl lookup runs."""
        ns = argparse.Namespace(object_store=None, exclude=[], workflow="cadence")
        with (
            patch.object(sandbox.subprocess, "run", return_value=Mock(returncode=1)),
            patch.object(sandbox, "_exec"),
            patch.object(sandbox, "_helm_chart_ports", return_value=[]),
            patch.object(sandbox, "_deploy_services") as deploy,
            patch.object(sandbox, "_kubectl_output") as lookup,
        ):
            sandbox._sync(ns)
        self.assertEqual(ns.object_store, "seaweedfs")
        deploy.assert_called_once_with(ns)
        lookup.assert_not_called()

    def test_sync_lookup_failure_stops_before_service_change(self):
        """A failed lookup exits before MySQL or the `objectstore` Service."""
        ns = argparse.Namespace(object_store=None)
        fake = _fake_kubectl(fail=("objectstore",))

        def fake_run(args, **kwargs):
            if args[:3] == ["k3d", "cluster", "get"]:
                return Mock(returncode=0)
            return fake(args, **kwargs)

        with (
            patch.object(sandbox.subprocess, "run", side_effect=fake_run),
            patch.object(sandbox, "_exec"),
            patch.object(sandbox, "_ensure_mysql_running") as mysql,
            patch.object(sandbox, "_ensure_objectstore_service") as service,
            patch.object(sandbox, "_ensure_object_store_running") as running,
            self.assertRaises(SystemExit),
        ):
            sandbox._sync(ns)
        mysql.assert_not_called()
        service.assert_not_called()
        running.assert_not_called()
