"""Unit tests for sandbox module."""

import argparse
import base64
import json
import os
import subprocess
from unittest import TestCase
from unittest.mock import Mock, patch

import yaml

from michelangelo.cli.sandbox import sandbox


class CreateFunctionTest(TestCase):
    """Tests for _create function logic."""

    @patch("michelangelo.cli.sandbox.sandbox._import_log_persistence_images")
    @patch("michelangelo.cli.sandbox.sandbox._kube_wait")
    @patch("michelangelo.cli.sandbox.sandbox._create_cadence_domain")
    @patch("michelangelo.cli.sandbox.sandbox._test_compute_chart")
    @patch("michelangelo.cli.sandbox.sandbox._install_compute_chart")
    @patch("michelangelo.cli.sandbox.sandbox.subprocess.check_output")
    @patch("michelangelo.cli.sandbox.sandbox._assert_command")
    @patch("michelangelo.cli.sandbox.sandbox._kube_create")
    @patch("michelangelo.cli.sandbox.sandbox._exec")
    @patch("michelangelo.cli.sandbox.sandbox.tempfile.NamedTemporaryFile")
    @patch("michelangelo.cli.sandbox.sandbox._create_compute_cluster_secrets")
    @patch("michelangelo.cli.sandbox.sandbox._create_compute_cluster_crd")
    @patch("michelangelo.cli.sandbox.sandbox._create_compute_cluster")
    def test_create_with_dedicated_compute_cluster(
        self,
        mock_create_compute_cluster,
        mock_create_crd,
        mock_create_secrets,
        mock_tempfile,
        mock_exec,
        mock_kube_create,
        mock_assert_command,
        mock_check_output,
        mock_install_chart,
        mock_test_chart,
        mock_create_cadence_domain,
        mock_kube_wait,
        mock_import_images,
    ):
        """Test dedicated cluster functions called with compute cluster name."""
        # Setup namespace with create_compute_cluster=True
        ns = argparse.Namespace(
            workflow="cadence",
            object_store="minio",
            exclude=[],
            include_experimental=[],
            create_compute_cluster=True,
            compute_cluster_name="test-compute-cluster",
        )

        # Mock dependencies
        mock_check_output.return_value = (
            b"kuberay\thttps://ray-project.github.io/kuberay-helm\n"
        )
        mock_registry_file = Mock()
        mock_registry_file.name = "/tmp/test-registry.json"
        mock_registry_file.__enter__ = Mock(return_value=mock_registry_file)
        mock_registry_file.__exit__ = Mock(return_value=False)
        mock_tempfile.return_value = mock_registry_file

        sandbox._create(ns)

        # Verify dedicated compute cluster functions were called with the
        # compute cluster name
        mock_create_compute_cluster.assert_called_once_with("test-compute-cluster", [])
        mock_create_crd.assert_called_once_with("test-compute-cluster")
        mock_create_secrets.assert_called_once_with("test-compute-cluster")

        # The control plane gets the compute chart in this mode too
        mock_install_chart.assert_called_once_with(
            "k3d-michelangelo-sandbox", "control-plane", []
        )
        mock_test_chart.assert_called_once_with("k3d-michelangelo-sandbox", [])

        # Ray log collectors on the compute cluster write to the MinIO NodePort
        control_plane_install = next(
            c[0]
            for c in mock_exec.call_args_list
            if c[0][:4] == ("helm", "upgrade", "--install", "michelangelo")
        )
        self.assertIn(
            "controllermgr.jobs.k8sengine.mapper.logPersistence.storageEndpoint="
            "k3d-michelangelo-sandbox-agent-0:30007",
            control_plane_install,
        )

    @patch("michelangelo.cli.sandbox.sandbox._import_log_persistence_images")
    @patch("michelangelo.cli.sandbox.sandbox._kube_wait")
    @patch("michelangelo.cli.sandbox.sandbox._create_cadence_domain")
    @patch("michelangelo.cli.sandbox.sandbox._test_compute_chart")
    @patch("michelangelo.cli.sandbox.sandbox._install_compute_chart")
    @patch("michelangelo.cli.sandbox.sandbox.subprocess.check_output")
    @patch("michelangelo.cli.sandbox.sandbox._assert_command")
    @patch("michelangelo.cli.sandbox.sandbox._kube_create")
    @patch("michelangelo.cli.sandbox.sandbox._exec")
    @patch("michelangelo.cli.sandbox.sandbox.tempfile.NamedTemporaryFile")
    @patch("michelangelo.cli.sandbox.sandbox._create_compute_cluster_secrets")
    @patch("michelangelo.cli.sandbox.sandbox._create_compute_cluster_crd")
    @patch("michelangelo.cli.sandbox.sandbox._create_compute_cluster")
    def test_create_without_dedicated_compute_cluster(
        self,
        mock_create_compute_cluster,
        mock_create_crd,
        mock_create_secrets,
        mock_tempfile,
        mock_exec,
        mock_kube_create,
        mock_assert_command,
        mock_check_output,
        mock_install_chart,
        mock_test_chart,
        mock_create_cadence_domain,
        mock_kube_wait,
        mock_import_images,
    ):
        """Test control plane cluster functions called with sandbox cluster name."""
        # Setup namespace with create_compute_cluster=False
        ns = argparse.Namespace(
            workflow="cadence",
            object_store="minio",
            exclude=[],
            include_experimental=[],
            create_compute_cluster=False,
            compute_cluster_name="test-compute-cluster",
        )

        # Mock dependencies
        mock_check_output.return_value = (
            b"kuberay\thttps://ray-project.github.io/kuberay-helm\n"
        )
        mock_registry_file = Mock()
        mock_registry_file.name = "/tmp/test-registry.json"
        mock_registry_file.__enter__ = Mock(return_value=mock_registry_file)
        mock_registry_file.__exit__ = Mock(return_value=False)
        mock_tempfile.return_value = mock_registry_file
        calls = Mock()
        calls.attach_mock(mock_install_chart, "install_chart")
        calls.attach_mock(mock_exec, "exec")
        calls.attach_mock(mock_test_chart, "test_chart")

        sandbox._create(ns)

        # Verify dedicated compute cluster was NOT created
        mock_create_compute_cluster.assert_not_called()

        # Verify control plane cluster CRD/secrets were created with
        # sandbox cluster name
        mock_create_crd.assert_called_once_with("michelangelo-sandbox")
        mock_create_secrets.assert_called_once_with("michelangelo-sandbox")

        # The chart replaces the operator installs and the hand-built
        # michelangelo-config ConfigMap, and the control plane's MinIO
        # endpoint keeps its default
        mock_install_chart.assert_called_once_with(
            "k3d-michelangelo-sandbox", "control-plane", []
        )
        exec_args = [str(a) for c in mock_exec.call_args_list for a in c[0]]
        self.assertFalse(
            [a for a in exec_args if "michelangelo-config" in a or "rbac-ray" in a]
        )
        self.assertFalse([a for a in exec_args if "storageEndpoint" in a])

        # Chart first (the controllers need its CRDs), then the control plane,
        # then the chart tests, which reach the apiserver
        names = [c[0] for c in calls.mock_calls]
        control_plane_install = next(
            i
            for i, c in enumerate(calls.mock_calls)
            if c[0] == "exec"
            and c[1][:4] == ("helm", "upgrade", "--install", "michelangelo")
        )
        self.assertLess(names.index("install_chart"), control_plane_install)
        self.assertLess(control_plane_install, names.index("test_chart"))

    @patch("michelangelo.cli.sandbox.sandbox._import_log_persistence_images")
    @patch("michelangelo.cli.sandbox.sandbox._kube_wait")
    @patch("michelangelo.cli.sandbox.sandbox._create_cadence_domain")
    @patch("michelangelo.cli.sandbox.sandbox._test_compute_chart")
    @patch("michelangelo.cli.sandbox.sandbox._install_compute_chart")
    @patch("michelangelo.cli.sandbox.sandbox.subprocess.check_output")
    @patch("michelangelo.cli.sandbox.sandbox._assert_command")
    @patch("michelangelo.cli.sandbox.sandbox._kube_create")
    @patch("michelangelo.cli.sandbox.sandbox._exec")
    @patch("michelangelo.cli.sandbox.sandbox.tempfile.NamedTemporaryFile")
    @patch("michelangelo.cli.sandbox.sandbox._create_compute_cluster_secrets")
    @patch("michelangelo.cli.sandbox.sandbox._create_compute_cluster_crd")
    @patch("michelangelo.cli.sandbox.sandbox._create_compute_cluster")
    def test_create_without_ray_imports_history_server_images(
        self,
        mock_create_compute_cluster,
        mock_create_crd,
        mock_create_secrets,
        mock_tempfile,
        mock_exec,
        mock_kube_create,
        mock_assert_command,
        mock_check_output,
        mock_install_chart,
        mock_test_chart,
        mock_create_cadence_domain,
        mock_kube_wait,
        mock_import_images,
    ):
        """With --exclude ray, the History Server images are still imported."""
        ns = argparse.Namespace(
            workflow="cadence",
            object_store="minio",
            exclude=["ray"],
            include_experimental=[],
            create_compute_cluster=False,
            compute_cluster_name="test-compute-cluster",
        )

        mock_check_output.return_value = b""
        mock_registry_file = Mock()
        mock_registry_file.name = "/tmp/test-registry.json"
        mock_registry_file.__enter__ = Mock(return_value=mock_registry_file)
        mock_registry_file.__exit__ = Mock(return_value=False)
        mock_tempfile.return_value = mock_registry_file

        sandbox._create(ns)

        mock_install_chart.assert_called_once_with(
            "k3d-michelangelo-sandbox", "control-plane", ["ray"]
        )
        mock_import_images.assert_called_once_with()


class ComputeClusterSetupTest(TestCase):
    """Tests for compute cluster setup functions."""

    @patch("michelangelo.cli.sandbox.sandbox._create_aws_credentials_in_cluster")
    @patch("michelangelo.cli.sandbox.sandbox._create_config_in_compute_cluster")
    @patch("michelangelo.cli.sandbox.sandbox._install_compute_chart")
    @patch("michelangelo.cli.sandbox.sandbox._exec")
    def test_create_compute_cluster_success(
        self,
        mock_exec,
        mock_install_chart,
        mock_create_config,
        mock_create_aws_creds,
    ):
        """Test successful creation of compute cluster."""
        cluster_name = "test-compute-cluster"

        sandbox._create_compute_cluster(cluster_name, ["spark"])

        # Verify k3d cluster creation was called
        k3d_calls = [c for c in mock_exec.call_args_list if c[0][0] == "k3d"]
        self.assertEqual(len(k3d_calls), 1)

        # Verify cluster creation arguments
        k3d_call_args = k3d_calls[0][0]
        self.assertIn("cluster", k3d_call_args)
        self.assertIn("create", k3d_call_args)
        self.assertIn(cluster_name, k3d_call_args)

        # The chart replaces the KubeRay install, the michelangelo-config
        # ConfigMap, the aws-credentials Secret and rbac-ray.yaml
        mock_install_chart.assert_called_once_with(
            f"k3d-{cluster_name}", "compute-cluster", ["spark"]
        )
        self.assertEqual(len(mock_exec.call_args_list), 1)
        mock_create_config.assert_not_called()
        mock_create_aws_creds.assert_not_called()

    @patch("michelangelo.cli.sandbox.sandbox._install_compute_chart")
    @patch("michelangelo.cli.sandbox.sandbox._exec")
    def test_create_compute_cluster_defaults_to_no_exclusions(
        self, mock_exec, mock_install_chart
    ):
        """Without exclusions the compute cluster gets both operators."""
        sandbox._create_compute_cluster("test-compute-cluster")

        mock_install_chart.assert_called_once_with(
            "k3d-test-compute-cluster", "compute-cluster", []
        )

    @patch("michelangelo.cli.sandbox.sandbox._exec")
    def test_create_config_success(self, mock_exec):
        """Test successful config creation in compute cluster."""
        cluster_name = "test-cluster"

        sandbox._create_config_in_compute_cluster(cluster_name)

        # Verify kubectl apply was called
        mock_exec.assert_called_once()
        call_args = mock_exec.call_args[0]

        self.assertEqual(call_args[0], "kubectl")
        self.assertIn("--context", call_args)
        self.assertIn(f"k3d-{cluster_name}", call_args)
        self.assertIn("apply", call_args)

    @patch("michelangelo.cli.sandbox.sandbox._exec")
    def test_create_aws_credentials_success(self, mock_exec):
        """Test successful AWS credentials creation."""
        cluster_name = "test-cluster"

        sandbox._create_aws_credentials_in_cluster(cluster_name)

        # Verify kubectl apply was called
        mock_exec.assert_called_once()
        call_args = mock_exec.call_args[0]

        self.assertEqual(call_args[0], "kubectl")
        self.assertIn("--context", call_args)
        self.assertIn(f"k3d-{cluster_name}", call_args)
        self.assertIn("apply", call_args)

    @patch("michelangelo.cli.sandbox.sandbox._exec")
    @patch("michelangelo.cli.sandbox.sandbox.subprocess.check_output")
    def test_namespace_already_exists(self, mock_check_output, mock_exec):
        """Test when namespace already exists."""
        # Simulate namespace exists
        mock_check_output.return_value = b"ma-system"

        sandbox._ensure_namespace_exists("ma-system")

        # Verify check was called but create was not
        mock_check_output.assert_called_once()
        mock_exec.assert_not_called()

    @patch("michelangelo.cli.sandbox.sandbox._exec")
    @patch("michelangelo.cli.sandbox.sandbox.subprocess.check_output")
    def test_namespace_does_not_exist(self, mock_check_output, mock_exec):
        """Test when namespace doesn't exist."""
        # Simulate namespace doesn't exist
        mock_check_output.side_effect = subprocess.CalledProcessError(1, "kubectl")

        sandbox._ensure_namespace_exists("ma-system")

        # Verify create was called
        mock_exec.assert_called_once()
        call_args = mock_exec.call_args[0]

        self.assertEqual(call_args[0], "kubectl")
        self.assertIn("create", call_args)
        self.assertIn("namespace", call_args)
        self.assertIn("ma-system", call_args)

    @patch("michelangelo.cli.sandbox.sandbox._exec")
    @patch("michelangelo.cli.sandbox.sandbox._ensure_namespace_exists")
    @patch("michelangelo.cli.sandbox.sandbox.subprocess.check_output")
    def test_create_cluster_crd_success(
        self, mock_check_output, mock_create_ns, mock_exec
    ):
        """Test successful CRD creation."""
        cluster_name = "test-cluster"

        # Mock kubeconfig output
        mock_check_output.return_value = (
            b"apiVersion: v1\nclusters:\n- cluster:\n    "
            b"certificate-authority-data: dGVzdA==\n    "
            b"server: https://127.0.0.1:12345\n  name: test"
        )

        sandbox._create_compute_cluster_crd(cluster_name)

        # Verify namespace creation was called
        mock_create_ns.assert_called_once()

        # Verify kubeconfig was retrieved
        mock_check_output.assert_called_once()
        call_args = mock_check_output.call_args[0][0]
        self.assertIn("k3d", call_args)
        self.assertIn("kubeconfig", call_args)
        self.assertIn(cluster_name, call_args)

        # Verify kubectl apply was called via _exec
        mock_exec.assert_called_once()
        exec_call_args = mock_exec.call_args[0]
        self.assertEqual(exec_call_args[0], "kubectl")
        self.assertIn("apply", exec_call_args)

    @patch("michelangelo.cli.sandbox.sandbox._exec")
    @patch("michelangelo.cli.sandbox.sandbox.subprocess.check_output")
    def test_create_secrets_success(self, mock_check_output, mock_exec):
        """Test successful secrets creation."""
        cluster_name = "test-cluster"

        # Mock kubeconfig output with proper structure
        kubeconfig_yaml = """apiVersion: v1
kind: Config
clusters:
- cluster:
    certificate-authority-data: dGVzdENBZGF0YQ==
    server: https://127.0.0.1:12345
  name: test-cluster
users:
- name: test-user
  user:
    client-certificate-data: dGVzdENlcnREYXRh
    client-key-data: dGVzdEtleURhdGE=
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
"""
        mock_check_output.return_value = kubeconfig_yaml.encode()
        applied = []

        def fake_exec(*args, **kwargs):
            if "ray-manager-token" in args:
                return base64.b64encode(b"test-token-value").decode()
            with open(args[args.index("-f") + 1]) as f:
                applied.append((args[:3], yaml.safe_load(f)))
            return None

        mock_exec.side_effect = fake_exec

        sandbox._create_compute_cluster_secrets(cluster_name)

        # Only the kubeconfig comes from check_output; the token comes from the
        # chart's ray-manager-token Secret on the compute cluster
        mock_check_output.assert_called_once_with(
            ["k3d", "kubeconfig", "get", cluster_name]
        )
        self.assertIn(
            ("kubectl", "--context", "k3d-test-cluster"),
            [c[0][:3] for c in mock_exec.call_args_list],
        )

        # Both Secrets are applied to the control plane
        self.assertEqual(
            applied,
            [
                (
                    ("kubectl", "--context", "k3d-michelangelo-sandbox"),
                    {
                        "apiVersion": "v1",
                        "kind": "Secret",
                        "metadata": {
                            "name": "cluster-test-cluster-ca-data",
                            "namespace": "default",
                        },
                        "stringData": {"cadata": "testCAdata"},
                    },
                ),
                (
                    ("kubectl", "--context", "k3d-michelangelo-sandbox"),
                    {
                        "apiVersion": "v1",
                        "kind": "Secret",
                        "metadata": {
                            "name": "cluster-test-cluster-client-token",
                            "namespace": "default",
                        },
                        "stringData": {"token": "test-token-value"},
                    },
                ),
            ],
        )

    @patch("michelangelo.cli.sandbox.sandbox._exec")
    @patch("michelangelo.cli.sandbox.sandbox.subprocess.check_output")
    def test_delete_with_existing_compute_cluster(self, mock_check_output, mock_exec):
        """Test deletion when compute cluster exists."""
        ns = Mock()
        ns.compute_cluster_name = "test-compute"

        # Simulate cluster exists
        mock_check_output.return_value = b"test-compute"

        sandbox._delete(ns)

        # Each demo inference cluster is probed before the compute cluster, so the
        # compute-cluster probe is the last one.
        inference_clusters = sandbox._inference_compute_cluster_names
        self.assertEqual(mock_check_output.call_count, len(inference_clusters) + 1)
        call_args = mock_check_output.call_args[0][0]
        self.assertIn("k3d", call_args)
        self.assertIn("cluster", call_args)
        self.assertIn("get", call_args)
        self.assertIn("test-compute", call_args)

        # Every probe succeeds here, so the inference clusters, the compute cluster,
        # and the sandbox cluster are all deleted.
        delete_calls = [c for c in mock_exec.call_args_list if "delete" in c[0]]
        self.assertEqual(len(delete_calls), len(inference_clusters) + 2)
        deleted = {c[0][-1] for c in delete_calls}
        for inf_cluster in inference_clusters:
            self.assertIn(inf_cluster, deleted)

    @patch("michelangelo.cli.sandbox.sandbox._exec")
    @patch("michelangelo.cli.sandbox.sandbox.subprocess.check_output")
    def test_delete_with_nonexistent_compute_cluster(
        self, mock_check_output, mock_exec
    ):
        """Test deletion when compute cluster doesn't exist."""
        ns = Mock()
        ns.compute_cluster_name = "test-compute"

        # Simulate cluster doesn't exist
        mock_check_output.side_effect = subprocess.CalledProcessError(1, "k3d")

        sandbox._delete(ns)

        # Each demo inference cluster is probed before the compute cluster.
        inference_clusters = sandbox._inference_compute_cluster_names
        self.assertEqual(mock_check_output.call_count, len(inference_clusters) + 1)

        # Every probe fails, so neither the inference clusters nor the compute cluster
        # are deleted; only the main sandbox cluster is.
        delete_calls = [c for c in mock_exec.call_args_list if "delete" in c[0]]
        self.assertEqual(len(delete_calls), 1)

        # Verify it was the main sandbox cluster
        main_delete_call = delete_calls[0][0]
        self.assertIn("michelangelo-sandbox", main_delete_call)

    @patch("michelangelo.cli.sandbox.sandbox._exec")
    @patch("michelangelo.cli.sandbox.sandbox.subprocess.check_output")
    def test_delete_without_compute_cluster_name(self, mock_check_output, mock_exec):
        """Test deletion when no compute cluster name is specified."""
        ns = Mock()
        ns.compute_cluster_name = None

        # Simulate default cluster doesn't exist
        mock_check_output.side_effect = subprocess.CalledProcessError(1, "k3d")

        sandbox._delete(ns)

        # Verify check was called with default name
        call_args = mock_check_output.call_args[0][0]
        self.assertIn("michelangelo-compute-0", call_args)

    @patch("michelangelo.cli.sandbox.sandbox._exec")
    @patch("michelangelo.cli.sandbox.sandbox.subprocess.check_output")
    @patch("builtins.print")
    def test_delete_prints_skip_message(self, mock_print, mock_check_output, mock_exec):
        """Test that skip message is printed when cluster doesn't exist."""
        ns = Mock()
        ns.compute_cluster_name = "test-compute"

        # Simulate cluster doesn't exist
        mock_check_output.side_effect = subprocess.CalledProcessError(1, "k3d")

        sandbox._delete(ns)

        # Verify skip message was printed
        print_calls = [str(c) for c in mock_print.call_args_list]
        skip_message_found = any(
            "not found" in str(c) and "skipping deletion" in str(c) for c in print_calls
        )
        self.assertTrue(skip_message_found, "Skip message should be printed")


# helm show crds output: every subchart's CRDs, each file after a "---" line.
_CRDS_OUTPUT = """---
# Source: kuberay-operator/crds/ray.io_rayclusters.yaml
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: rayclusters.ray.io
spec:
  group: ray.io
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: sparkapplications.sparkoperator.k8s.io
spec:
  group: sparkoperator.k8s.io
"""

_HELM_REPOS = (
    b"kuberay\thttps://ray-project.github.io/kuberay-helm\n"
    b"spark-operator\thttps://kubeflow.github.io/spark-operator\n"
)


def _b64(value: str) -> str:
    return base64.b64encode(value.encode()).decode()


def _kube_object(owner="", data=None):
    """A resource as kubectl get -o json prints it; `owner` is its Helm release."""
    obj = {"metadata": {"name": "test"}}
    if owner:
        obj["metadata"]["annotations"] = {"meta.helm.sh/release-name": owner}
    if data is not None:
        obj["data"] = data
    return obj


# michelangelo-config as earlier sandboxes applied it on the control plane
# (resources/michelangelo-config.yaml), plus a key added by hand.
_HAND_BUILT_CONFIG = {
    "MA_FILE_SYSTEM": "s3://default",
    "MA_FILE_SYSTEM_S3_SCHEME": "http",
    "AWS_ACCESS_KEY_ID": "michelangeloadmin",
    "AWS_SECRET_ACCESS_KEY": "michelangeloadmin",
    "AWS_ENDPOINT_URL": "http://minio:9091",
    "REGISTRY_ENDPOINT": "michelangelo-apiserver:15566",
    "MA_EXTRA_SETTING": "kept",
}


class ComputeChartTest(TestCase):
    """Tests for installing the michelangelo-compute chart on a cluster."""

    def setUp(self):
        """Start with no recorded commands and a new, supported cluster."""
        self.calls = []
        self.reads = []
        self.files = {}
        self.server_version = {
            "major": "1",
            "minor": "31",
            "gitVersion": "v1.31.5+k3s1",
        }
        self.helm_repos = _HELM_REPOS
        # Installed Helm releases as (release, namespace), and resources by
        # "kind/name". The fakes below read and change them.
        self.releases = set()
        self.objects = {}

    def _given_earlier_sandbox(self):
        """Leave what an earlier sandbox's create installed by hand."""
        self.releases = {
            ("kuberay-operator", "ray-system"),
            ("spark-operator", "spark-operator"),
        }
        self.objects = {
            "serviceaccount/ray-manager": _kube_object(),
            "configmap/michelangelo-config": _kube_object(data=_HAND_BUILT_CONFIG),
        }

    def _fake_exec(self, *args, **kwargs):
        """Record each command, keep the files it reads, answer the reads."""
        args = list(args)
        if "-f" in args:
            i = args.index("-f") + 1
            path = args[i]
            with open(path) as f:
                self.files[args[0]] = (path, os.stat(path).st_mode & 0o777, f.read())
            args[i] = "<file>"
        self.calls.append(tuple(args))
        self._apply_to_cluster(args)
        if "version" in args:
            return json.dumps({"serverVersion": self.server_version})
        if args[:3] == ["helm", "show", "crds"]:
            return _CRDS_OUTPUT
        if "minio-credentials" in args:
            return f"{_b64('test-access-key')} {_b64('test-secret-key')}"
        return None

    def _apply_to_cluster(self, args):
        """Change the cluster state the way the recorded command would."""
        if args[:2] == ["helm", "uninstall"]:
            self.releases.remove((args[2], args[args.index("-n") + 1]))
        for verb, field in [("label", "labels"), ("annotate", "annotations")]:
            if verb in args:
                rest = args[args.index(verb) + 1 :]
                pairs = dict(a.split("=", 1) for a in rest if "=" in a)
                for resource in rest:
                    if "=" not in resource and not resource.startswith("-"):
                        metadata = self.objects[resource]["metadata"]
                        metadata.setdefault(field, {}).update(pairs)
        if "delete" in args and "configmap" in args:
            del self.objects[f"configmap/{args[-1]}"]
        if args[:3] == ["helm", "upgrade", "--install"]:
            # The release renders michelangelo-config from runtime.config.
            self.objects["configmap/michelangelo-config"] = _kube_object(
                owner="michelangelo-compute", data=self._values()["runtime"]["config"]
            )

    def _fake_run(self, args, **kwargs):
        """Answer the reads that tolerate a missing release or resource."""
        self.reads.append(tuple(args))
        if args[:2] == ["helm", "status"]:
            found = (args[2], args[args.index("-n") + 1]) in self.releases
            return subprocess.CompletedProcess(args, 0 if found else 1, "", "")
        obj = self.objects.get(args[args.index("get") + 1])
        if obj is None:
            return subprocess.CompletedProcess(args, 1, "", "NotFound")
        return subprocess.CompletedProcess(args, 0, json.dumps(obj), "")

    def _install(self, target="compute-cluster", exclude=()):
        with (
            patch.object(sandbox, "_exec", side_effect=self._fake_exec),
            patch.object(sandbox.subprocess, "run", side_effect=self._fake_run),
            patch.object(
                sandbox.subprocess, "check_output", return_value=self.helm_repos
            ),
        ):
            sandbox._install_compute_chart("k3d-test-cluster", target, list(exclude))

    def _values(self) -> dict:
        """Return the values file the last install passed to helm."""
        return yaml.safe_load(self.files["helm"][2])

    def test_compute_cluster_install_order(self):
        """Preflight, CRDs, install, legacy RBAC delete, then the chart tests."""
        self._install()

        chart = str(sandbox._compute_chart_dir)
        self.assertEqual(
            self.calls,
            [
                ("kubectl", "--context", "k3d-test-cluster", "version", "-o", "json"),
                ("helm", "dependency", "build", chart),
                ("helm", "show", "crds", chart),
                (
                    "kubectl",
                    "--context",
                    "k3d-test-cluster",
                    "apply",
                    "--server-side",
                    "--force-conflicts",
                    "-f",
                    "<file>",
                ),
                (
                    "kubectl",
                    "--context",
                    "k3d-michelangelo-sandbox",
                    "-n",
                    "default",
                    "get",
                    "secret",
                    "minio-credentials",
                    "-o",
                    "jsonpath={.data.AWS_ACCESS_KEY_ID} {.data.AWS_SECRET_ACCESS_KEY}",
                ),
                (
                    "helm",
                    "upgrade",
                    "--install",
                    "michelangelo-compute",
                    chart,
                    "--kube-context",
                    "k3d-test-cluster",
                    "-n",
                    "michelangelo-compute",
                    "--create-namespace",
                    "-f",
                    "<file>",
                    "--wait",
                    "--timeout",
                    "20m",
                ),
                (
                    "kubectl",
                    "--context",
                    "k3d-test-cluster",
                    "delete",
                    "clusterrolebinding/ray-manager-binding",
                    "clusterrole/ray-manager",
                    "--ignore-not-found",
                ),
                (
                    "helm",
                    "test",
                    "michelangelo-compute",
                    "--kube-context",
                    "k3d-test-cluster",
                    "-n",
                    "michelangelo-compute",
                ),
            ],
        )

    def test_control_plane_install_leaves_chart_tests_to_caller(self):
        """The control plane is tested once its apiserver is up."""
        self._install(target="control-plane")

        self.assertEqual(
            [c[:2] for c in self.calls],
            [
                ("kubectl", "--context"),
                ("helm", "dependency"),
                ("helm", "show"),
                ("kubectl", "--context"),
                ("helm", "upgrade"),
                ("kubectl", "--context"),
            ],
        )
        self.assertEqual(self.calls[-1][3], "delete")

    def test_missing_operator_repos_are_added(self):
        """The operator repos are added before the dependency build."""
        self.helm_repos = b"temporal\thttps://go.temporal.io/helm-charts\n"

        self._install()

        self.assertEqual(
            self.calls[1:4],
            [
                (
                    "helm",
                    "repo",
                    "add",
                    "kuberay",
                    "https://ray-project.github.io/kuberay-helm",
                ),
                (
                    "helm",
                    "repo",
                    "add",
                    "spark-operator",
                    "https://kubeflow.github.io/spark-operator",
                ),
                ("helm", "dependency", "build", str(sandbox._compute_chart_dir)),
            ],
        )

    def test_values_file_is_private_and_removed(self):
        """The values file holds the keys: owner-only, deleted after install."""
        self._install()

        path, mode, content = self.files["helm"]
        self.assertEqual(mode, 0o600)
        self.assertFalse(os.path.exists(path))
        credentials = yaml.safe_load(content)["runtime"]["objectStorageCredentials"]
        self.assertEqual(credentials["accessKeyId"], "test-access-key")

    def test_crds_of_enabled_operators_are_applied(self):
        """CRDs are filtered by the API group of each enabled operator."""
        cases = {
            (): ["rayclusters.ray.io", "sparkapplications.sparkoperator.k8s.io"],
            ("spark",): ["rayclusters.ray.io"],
            ("ray",): ["sparkapplications.sparkoperator.k8s.io"],
        }
        for exclude, names in cases.items():
            with self.subTest(exclude=exclude):
                self.files = {}

                self._install(exclude=exclude)

                _, _, content = self.files["kubectl"]
                self.assertEqual(
                    [d["metadata"]["name"] for d in yaml.safe_load_all(content) if d],
                    names,
                )

    def test_no_crds_applied_without_operators(self):
        """With both operators excluded no CRDs are read or applied."""
        self._install(exclude=("ray", "spark"))

        self.assertNotIn(("helm", "show", "crds"), [c[:3] for c in self.calls])
        self.assertNotIn("kubectl", self.files)

    def test_preflight_rejects_old_kubernetes(self):
        """An old cluster fails with an actionable error before any install."""
        self.server_version = {
            "major": "1",
            "minor": "28",
            "gitVersion": "v1.28.9+k3s1",
        }

        with (
            patch.object(sandbox, "_err_exit", side_effect=SystemExit(1)) as mock_err,
            self.assertRaises(SystemExit),
        ):
            self._install()

        message = mock_err.call_args[0][0]
        for part in [
            "k3d-test-cluster",
            "v1.28.9+k3s1",
            "1.29 or newer",
            "Upgrade k3d",
            "ma sandbox delete && ma sandbox create",
        ]:
            self.assertIn(part, message)
        self.assertEqual(len(self.calls), 1)

    def test_preflight_accepts_supported_versions(self):
        """Kubernetes 1.29 and newer pass, including provider '+' suffixes."""
        for minor in ["29", "29+", "33"]:
            self.server_version = {"major": "1", "minor": minor}
            with (
                self.subTest(minor=minor),
                patch.object(sandbox, "_exec", side_effect=self._fake_exec),
                patch.object(sandbox, "_err_exit") as mock_err,
            ):
                sandbox._check_kube_version("k3d-test-cluster")

                mock_err.assert_not_called()

    def test_control_plane_values(self):
        """The control plane uses in-cluster endpoints and its own Secret."""
        with patch.object(sandbox, "_exec") as mock_exec:
            values = sandbox._compute_values("control-plane", [])

        mock_exec.assert_not_called()
        self.assertEqual(
            values,
            {
                "runtime": {
                    "config": {
                        "MA_FILE_SYSTEM": "s3://default",
                        "MA_FILE_SYSTEM_S3_SCHEME": "http",
                        "AWS_ENDPOINT_URL": "http://objectstore:9091",
                        "REGISTRY_ENDPOINT": "michelangelo-apiserver:15566",
                    },
                    "objectStorageCredentials": {
                        "name": "minio-credentials",
                        "create": False,
                    },
                },
            },
        )

    def test_compute_cluster_values(self):
        """A compute cluster uses NodePorts and a copy of the control plane keys."""
        with patch.object(sandbox, "_exec", side_effect=self._fake_exec):
            values = sandbox._compute_values("compute-cluster", [])

        self.assertEqual(
            self.calls[0][:3], ("kubectl", "--context", "k3d-michelangelo-sandbox")
        )
        self.assertEqual(
            values,
            {
                "runtime": {
                    "config": {
                        "MA_FILE_SYSTEM": "s3://default",
                        "MA_FILE_SYSTEM_S3_SCHEME": "http",
                        "AWS_ENDPOINT_URL": (
                            "http://k3d-michelangelo-sandbox-agent-0:30007"
                        ),
                        "REGISTRY_ENDPOINT": "k3d-michelangelo-sandbox-agent-0:30009",
                    },
                    "objectStorageCredentials": {
                        "name": "minio-credentials",
                        "create": True,
                        "accessKeyId": "test-access-key",
                        "secretAccessKey": "test-secret-key",
                    },
                },
            },
        )

    def test_exclude_disables_subcharts_on_every_cluster(self):
        """--exclude ray|spark turns the subchart off; operators default on."""
        cases = {
            (): {},
            ("ray",): {"kuberay-operator": {"enabled": False}},
            ("spark",): {"spark-operator": {"enabled": False}},
            ("ray", "spark", "grafana"): {
                "kuberay-operator": {"enabled": False},
                "spark-operator": {"enabled": False},
            },
        }
        for target in ["control-plane", "compute-cluster"]:
            for exclude, toggles in cases.items():
                with (
                    self.subTest(target=target, exclude=exclude),
                    patch.object(sandbox, "_exec", side_effect=self._fake_exec),
                ):
                    values = sandbox._compute_values(target, list(exclude))

                    values.pop("runtime")
                    self.assertEqual(values, toggles)

    @patch("michelangelo.cli.sandbox.sandbox.time.sleep")
    def test_token_is_polled_until_set(self, mock_sleep):
        """The token is read once Kubernetes fills in ray-manager-token."""
        with patch.object(
            sandbox, "_exec", side_effect=["", "", _b64("test-token")]
        ) as mock_exec:
            token = sandbox._read_ray_manager_token("test-cluster")

        self.assertEqual(token, "test-token")
        self.assertEqual(mock_exec.call_count, 3)
        self.assertEqual(mock_sleep.call_count, 2)
        mock_exec.assert_called_with(
            "kubectl",
            "--context",
            "k3d-test-cluster",
            "-n",
            "default",
            "get",
            "secret",
            "ray-manager-token",
            "-o",
            "jsonpath={.data.token}",
            capture_output=True,
        )

    @patch("michelangelo.cli.sandbox.sandbox.time.sleep")
    def test_token_polling_times_out(self, mock_sleep):
        """An empty token after 60 seconds is an error naming the Secret."""
        with (
            patch.object(sandbox, "_exec", return_value="") as mock_exec,
            patch.object(sandbox, "_err_exit", side_effect=SystemExit(1)) as mock_err,
            self.assertRaises(SystemExit),
        ):
            sandbox._read_ray_manager_token("test-cluster")

        self.assertEqual(mock_exec.call_count, 30)
        self.assertEqual(sum(c[0][0] for c in mock_sleep.call_args_list), 60)
        self.assertIn("ray-manager-token", mock_err.call_args[0][0])
        self.assertIn("k3d-test-cluster", mock_err.call_args[0][0])

    def test_chart_tests_run_on_the_given_cluster(self):
        """The chart tests run against the release on the given context."""
        with patch.object(sandbox, "_exec") as mock_exec:
            sandbox._test_compute_chart("k3d-test-cluster", [])

        mock_exec.assert_called_once_with(
            "helm",
            "test",
            "michelangelo-compute",
            "--kube-context",
            "k3d-test-cluster",
            "-n",
            "michelangelo-compute",
            raise_error=True,
        )

    def test_failed_chart_tests_print_the_test_pod_log(self):
        """A failed test prints its pod's log from default, which --logs cannot read."""
        failure = subprocess.CalledProcessError(1, "helm")
        with (
            patch.object(sandbox, "_exec", side_effect=failure),
            patch.object(sandbox.subprocess, "run") as mock_run,
            patch.object(sandbox, "_err_exit", side_effect=SystemExit(1)) as mock_err,
            self.assertRaises(SystemExit),
        ):
            sandbox._test_compute_chart("k3d-test-cluster", [])

        mock_run.assert_called_once_with(
            [
                "kubectl",
                "--context",
                "k3d-test-cluster",
                "-n",
                "default",
                "logs",
                "pod/michelangelo-compute-test-runtime-contract",
            ],
            check=False,
        )
        message = mock_err.call_args[0][0]
        self.assertIn("k3d-test-cluster", message)
        self.assertIn("michelangelo-compute-test-runtime-contract", message)

    def test_chart_tests_skipped_without_apiserver(self):
        """The chart tests reach the apiserver, so --exclude apiserver skips them."""
        with patch.object(sandbox, "_exec") as mock_exec:
            sandbox._test_compute_chart("k3d-test-cluster", ["apiserver"])

        mock_exec.assert_not_called()

    def test_earlier_sandbox_migration_order(self):
        """Replace and adopt before the CRDs; delete the ConfigMap after them."""
        self._given_earlier_sandbox()

        self._install()

        chart = str(sandbox._compute_chart_dir)
        self.assertEqual(
            self.calls,
            [
                ("kubectl", "--context", "k3d-test-cluster", "version", "-o", "json"),
                ("helm", "dependency", "build", chart),
                (
                    "helm",
                    "uninstall",
                    "kuberay-operator",
                    "--kube-context",
                    "k3d-test-cluster",
                    "-n",
                    "ray-system",
                    "--wait",
                ),
                (
                    "helm",
                    "uninstall",
                    "spark-operator",
                    "--kube-context",
                    "k3d-test-cluster",
                    "-n",
                    "spark-operator",
                    "--wait",
                ),
                (
                    "kubectl",
                    "--context",
                    "k3d-test-cluster",
                    "-n",
                    "default",
                    "label",
                    "--overwrite",
                    "serviceaccount/ray-manager",
                    "app.kubernetes.io/managed-by=Helm",
                ),
                (
                    "kubectl",
                    "--context",
                    "k3d-test-cluster",
                    "-n",
                    "default",
                    "annotate",
                    "--overwrite",
                    "serviceaccount/ray-manager",
                    "meta.helm.sh/release-name=michelangelo-compute",
                    "meta.helm.sh/release-namespace=michelangelo-compute",
                ),
                ("helm", "show", "crds", chart),
                (
                    "kubectl",
                    "--context",
                    "k3d-test-cluster",
                    "apply",
                    "--server-side",
                    "--force-conflicts",
                    "-f",
                    "<file>",
                ),
                (
                    "kubectl",
                    "--context",
                    "k3d-michelangelo-sandbox",
                    "-n",
                    "default",
                    "get",
                    "secret",
                    "minio-credentials",
                    "-o",
                    "jsonpath={.data.AWS_ACCESS_KEY_ID} {.data.AWS_SECRET_ACCESS_KEY}",
                ),
                (
                    "kubectl",
                    "--context",
                    "k3d-test-cluster",
                    "-n",
                    "default",
                    "delete",
                    "configmap",
                    "michelangelo-config",
                ),
                (
                    "helm",
                    "upgrade",
                    "--install",
                    "michelangelo-compute",
                    chart,
                    "--kube-context",
                    "k3d-test-cluster",
                    "-n",
                    "michelangelo-compute",
                    "--create-namespace",
                    "-f",
                    "<file>",
                    "--wait",
                    "--timeout",
                    "20m",
                ),
                (
                    "kubectl",
                    "--context",
                    "k3d-test-cluster",
                    "delete",
                    "clusterrolebinding/ray-manager-binding",
                    "clusterrole/ray-manager",
                    "--ignore-not-found",
                ),
                (
                    "helm",
                    "test",
                    "michelangelo-compute",
                    "--kube-context",
                    "k3d-test-cluster",
                    "-n",
                    "michelangelo-compute",
                ),
            ],
        )

    def test_migration_reads_name_the_cluster(self):
        """Every read of a release or resource passes the cluster's context."""
        self._given_earlier_sandbox()

        self._install()

        self.assertTrue(self.reads)
        for read in self.reads:
            with self.subTest(read=read):
                flag = "--kube-context" if read[0] == "helm" else "--context"
                self.assertEqual(read[read.index(flag) + 1], "k3d-test-cluster")

    def test_second_run_only_installs_and_deletes_legacy_rbac(self):
        """A migrated cluster gets no uninstall, adoption or ConfigMap delete."""
        self._given_earlier_sandbox()
        self._install()
        self.calls = []

        self._install()

        changes = [
            c
            for c in self.calls
            if {"uninstall", "label", "annotate", "delete"} & set(c)
        ]
        self.assertEqual(
            changes,
            [
                (
                    "kubectl",
                    "--context",
                    "k3d-test-cluster",
                    "delete",
                    "clusterrolebinding/ray-manager-binding",
                    "clusterrole/ray-manager",
                    "--ignore-not-found",
                ),
            ],
        )
        self.assertIn(
            ("helm", "upgrade", "--install", "michelangelo-compute"),
            [c[:4] for c in self.calls],
        )

    def test_chart_owned_config_map_is_kept(self):
        """The release's own ConfigMap is neither deleted nor carried over."""
        for target in ["control-plane", "compute-cluster"]:
            with self.subTest(target=target):
                self.calls = []
                self.objects = {
                    "configmap/michelangelo-config": _kube_object(
                        owner="michelangelo-compute",
                        data={"MA_EXTRA_SETTING": "dropped"},
                    ),
                }

                self._install(target=target)

                self.assertNotIn("configmap", [a for c in self.calls for a in c])
                self.assertNotIn(
                    "MA_EXTRA_SETTING", self._values()["runtime"]["config"]
                )

    def test_standalone_operators_replaced_unless_excluded(self):
        """An enabled operator's release is uninstalled; an excluded one's kept."""
        cases = {
            (): set(),
            ("ray",): {("kuberay-operator", "ray-system")},
            ("spark",): {("spark-operator", "spark-operator")},
            ("ray", "spark"): {
                ("kuberay-operator", "ray-system"),
                ("spark-operator", "spark-operator"),
            },
        }
        for exclude, kept in cases.items():
            with self.subTest(exclude=exclude):
                self._given_earlier_sandbox()
                self.calls = []

                self._install(exclude=exclude)

                self.assertEqual(self.releases, kept)
                uninstalled = {
                    (c[2], c[c.index("-n") + 1]) for c in self.calls if "uninstall" in c
                }
                self.assertEqual(
                    uninstalled,
                    {
                        ("kuberay-operator", "ray-system"),
                        ("spark-operator", "spark-operator"),
                    }
                    - kept,
                )

    def test_hand_built_identity_and_kueue_grants_are_adopted(self):
        """The ServiceAccount, token and the four Kueue grants get Helm metadata."""
        adopted = [
            "serviceaccount/ray-manager",
            "secret/ray-manager-token",
            "role/ray-manager",
            "rolebinding/ray-manager",
            "clusterrole/ray-manager-clusterqueues",
            "clusterrolebinding/ray-manager-clusterqueues",
        ]
        self.objects = {resource: _kube_object() for resource in adopted}

        self._install()

        label, annotate = [c for c in self.calls if {"label", "annotate"} & set(c)]
        self.assertEqual(label[7:-1], tuple(adopted))
        self.assertEqual(annotate[7:-2], tuple(adopted))
        for resource in adopted:
            with self.subTest(resource=resource):
                metadata = self.objects[resource]["metadata"]
                self.assertEqual(
                    metadata["annotations"],
                    {
                        "meta.helm.sh/release-name": "michelangelo-compute",
                        "meta.helm.sh/release-namespace": "michelangelo-compute",
                    },
                )
                self.assertEqual(
                    metadata["labels"], {"app.kubernetes.io/managed-by": "Helm"}
                )
        # Metadata only: nothing else touches them.
        for resource in adopted:
            self.assertEqual(
                [c for c in self.calls if resource in c], [label, annotate]
            )

    def test_only_unowned_resources_are_adopted(self):
        """Owned and absent resources are left out of the adoption."""
        self.objects = {
            "serviceaccount/ray-manager": _kube_object(owner="michelangelo-compute"),
            "secret/ray-manager-token": _kube_object(),
            "rolebinding/ray-manager": _kube_object(),
        }

        self._install()

        label = next(c for c in self.calls if "label" in c)
        self.assertEqual(
            label[7:-1], ("secret/ray-manager-token", "rolebinding/ray-manager")
        )

    def test_hand_built_config_map_keys_carry_over(self):
        """Its keys reach runtime.config without credentials; the sandbox's win."""
        self._given_earlier_sandbox()

        self._install()

        agent = "k3d-michelangelo-sandbox-agent-0"
        self.assertEqual(
            self._values()["runtime"]["config"],
            {
                "MA_FILE_SYSTEM": "s3://default",
                "MA_FILE_SYSTEM_S3_SCHEME": "http",
                "AWS_ENDPOINT_URL": f"http://{agent}:30007",
                "REGISTRY_ENDPOINT": f"{agent}:30009",
                "MA_EXTRA_SETTING": "kept",
            },
        )


class BuildHelmSetArgsTest(TestCase):
    """Tests for the control plane's helm --set arguments."""

    _storage_endpoint_flag = (
        "controllermgr.jobs.k8sengine.mapper.logPersistence.storageEndpoint="
        "k3d-michelangelo-sandbox-agent-0:30007"
    )

    def test_compute_cluster_points_log_storage_at_minio_nodeport(self):
        """With a compute cluster, the collectors use the MinIO NodePort."""
        ns = argparse.Namespace(
            workflow="cadence",
            exclude=[],
            helm_set=["controllermgr.foo=bar"],
            create_compute_cluster=True,
        )

        args = sandbox._build_helm_set_args(ns)

        i = args.index(self._storage_endpoint_flag)
        self.assertEqual(args[i - 1], "--set")
        # Caller --set flags come later, so they still override it
        self.assertLess(i, args.index("controllermgr.foo=bar"))

    def test_control_plane_names_the_credentials_secret(self):
        """Both create and sync name minio-credentials, ahead of caller --set flags."""
        flag = (
            "controllermgr.jobs.k8sengine.mapper.logPersistence.credentialsSecret="
            "minio-credentials"
        )
        for ns in [
            argparse.Namespace(
                workflow="cadence",
                exclude=[],
                helm_set=["controllermgr.foo=bar"],
                create_compute_cluster=False,
            ),
            argparse.Namespace(
                workflow="cadence", exclude=[], helm_set=["controllermgr.foo=bar"]
            ),
        ]:
            with self.subTest(ns=ns):
                args = sandbox._build_helm_set_args(ns)

                i = args.index(flag)
                self.assertEqual(args[i - 1], "--set")
                self.assertLess(i, args.index("controllermgr.foo=bar"))

    def test_control_plane_keeps_default_log_storage(self):
        """Without a compute cluster, in create or sync, the default endpoint stays."""
        for ns in [
            argparse.Namespace(
                workflow="cadence",
                exclude=[],
                helm_set=[],
                create_compute_cluster=False,
            ),
            argparse.Namespace(workflow="cadence", exclude=[], helm_set=[]),
        ]:
            with self.subTest(ns=ns):
                args = sandbox._build_helm_set_args(ns)

                self.assertFalse([a for a in args if "storageEndpoint" in a])


class SyncTest(TestCase):
    """Tests for sync on a reused sandbox: the chart on every cluster."""

    _credentials_secret_flag = (
        "controllermgr.jobs.k8sengine.mapper.logPersistence.credentialsSecret="
        "minio-credentials"
    )
    _storage_endpoint_flag = (
        "controllermgr.jobs.k8sengine.mapper.logPersistence.storageEndpoint="
        "k3d-michelangelo-sandbox-agent-0:30007"
    )
    # sync upgrades a healthy release and reinstalls a broken one.
    _helm_installs = (("helm", "upgrade"), ("helm", "install"))

    def setUp(self):
        """A healthy single-cluster sandbox: the control plane registers itself."""
        self.log = []
        self.release_deployed = True
        self.cluster_crs_returncode = 0
        self.registered = ["michelangelo-sandbox"]
        self.contexts = ["k3d-michelangelo-sandbox"]
        self.k3d_clusters = {"michelangelo-sandbox"}

    def _given_compute_cluster(self):
        """Add compute-0, as create --create-compute-cluster leaves it."""
        self.registered = ["michelangelo-compute-0", "michelangelo-sandbox"]
        self.contexts = ["k3d-michelangelo-compute-0", "k3d-michelangelo-sandbox"]
        self.k3d_clusters = {"michelangelo-compute-0", "michelangelo-sandbox"}

    def _fake_run(self, args, **kwargs):
        """Log each command and answer it from the sandbox state."""
        self.log.append(("run", tuple(args)))
        if args[:3] == ["k3d", "cluster", "get"]:
            found = args[3] in self.k3d_clusters
            return subprocess.CompletedProcess(args, 0 if found else 1, b"", b"")
        if "clusters.michelangelo.api" in args:
            return subprocess.CompletedProcess(
                args, self.cluster_crs_returncode, " ".join(self.registered), ""
            )
        if args[:3] == ["kubectl", "config", "get-contexts"]:
            return subprocess.CompletedProcess(args, 0, "\n".join(self.contexts), "")
        if args[:3] == ["helm", "status", "michelangelo"]:
            if self.release_deployed:
                return subprocess.CompletedProcess(
                    args, 0, '{"info":{"status":"deployed"}}', ""
                )
            return subprocess.CompletedProcess(args, 1, "", "release: not found")
        return subprocess.CompletedProcess(args, 0, "", "")

    def _recorder(self, name):
        """Return a fake that logs its calls under `name`."""

        def record(*args, **kwargs):
            self.log.append((name, args))

        return record

    def _sync(self, *flags):
        p = argparse.ArgumentParser()
        sandbox.init_arguments(p)
        ns = p.parse_args(["sync", *flags])
        with (
            patch.object(sandbox.subprocess, "run", side_effect=self._fake_run),
            patch.object(sandbox, "_exec", side_effect=self._recorder("exec")),
            patch.object(
                sandbox,
                "_install_compute_chart",
                side_effect=self._recorder("install"),
            ),
            patch.object(
                sandbox, "_test_compute_chart", side_effect=self._recorder("test")
            ),
            patch.object(sandbox, "_helm_wait", side_effect=self._recorder("wait")),
            patch.object(sandbox, "_ensure_mysql_running"),
            patch.object(sandbox, "_ensure_object_store_running"),
            patch.object(sandbox, "_refresh_mysql_schema"),
            patch.object(sandbox, "_ensure_credentials_secret"),
            patch.object(sandbox, "_helm_ensure_repos"),
            patch.object(sandbox, "_helm_delete_services"),
            patch.object(sandbox, "_helm_adopt_orphaned_resources"),
        ):
            sandbox._sync(ns)

    def _calls(self, name):
        return [args for kind, args in self.log if kind == name]

    def _michelangelo_install(self):
        """Return the helm upgrade (or reinstall) of the michelangelo release."""
        return next(
            args
            for args in self._calls("exec")
            if args[:2] in self._helm_installs and args[2] == "michelangelo"
        )

    def _steps(self):
        """Name the logged calls whose order matters."""
        steps = []
        for kind, args in self.log:
            if kind in ("install", "test"):
                steps.append(f"{kind} {args[0]}")
            elif kind == "wait":
                steps.append("wait")
            elif kind == "exec" and args[:2] in self._helm_installs:
                steps.append(" ".join(args[:3]))
            elif kind == "run" and args[:3] == ("kubectl", "rollout", "status"):
                steps.append(f"rollout status {args[3]}")
        return steps

    def test_single_cluster_sync_installs_the_chart_once(self):
        """The control plane's own Cluster CR does not make it a compute cluster."""
        self._sync()

        self.assertEqual(
            self._calls("install"), [("k3d-michelangelo-sandbox", "control-plane", [])]
        )
        install = self._michelangelo_install()
        self.assertIn(self._credentials_secret_flag, install)
        self.assertNotIn(self._storage_endpoint_flag, install)

    def test_compute_mode_sync_installs_on_every_cluster(self):
        """compute-0 gets compute values; collectors write to the MinIO NodePort."""
        self._given_compute_cluster()

        self._sync("--exclude", "spark")

        self.assertEqual(
            self._calls("install"),
            [
                ("k3d-michelangelo-sandbox", "control-plane", ["spark"]),
                ("k3d-michelangelo-compute-0", "compute-cluster", ["spark"]),
            ],
        )
        install = self._michelangelo_install()
        for flag in [self._credentials_secret_flag, self._storage_endpoint_flag]:
            with self.subTest(flag=flag):
                self.assertEqual(install[install.index(flag) - 1], "--set")

    def test_chart_tests_follow_the_apiserver_rollout(self):
        """The control plane is tested once the apiserver is back, then compute-0."""
        self._given_compute_cluster()

        self._sync()

        self.assertEqual(
            self._steps(),
            [
                "install k3d-michelangelo-sandbox",
                "helm upgrade michelangelo",
                "rollout status deployment/michelangelo-apiserver",
                "rollout status deployment/michelangelo-controllermgr",
                "rollout status deployment/michelangelo-worker",
                "wait",
                "test k3d-michelangelo-sandbox",
                "install k3d-michelangelo-compute-0",
            ],
        )

    def test_reinstalled_release_is_tested_after_the_wait(self):
        """Without a healthy release the chart tests follow the reinstall's wait."""
        self._given_compute_cluster()
        self.release_deployed = False

        self._sync()

        self.assertEqual(
            self._steps(),
            [
                "install k3d-michelangelo-sandbox",
                "helm install michelangelo",
                "wait",
                "test k3d-michelangelo-sandbox",
                "install k3d-michelangelo-compute-0",
            ],
        )
        install = self._michelangelo_install()
        self.assertIn(self._credentials_secret_flag, install)
        self.assertIn(self._storage_endpoint_flag, install)

    def test_discovery_needs_a_context_and_a_k3d_cluster(self):
        """Without a k3d-<name> context or a k3d cluster, a registration is skipped."""
        self.registered = ["michelangelo-sandbox", "a", "b", "c"]
        self.contexts = ["k3d-michelangelo-sandbox", "k3d-a", "k3d-b"]
        self.k3d_clusters = {"michelangelo-sandbox", "a", "c"}

        with patch.object(sandbox.subprocess, "run", side_effect=self._fake_run):
            clusters = sandbox._registered_compute_clusters()

        self.assertEqual(clusters, ["a"])
        self.assertEqual(
            self.log[0][1][:3], ("kubectl", "--context", "k3d-michelangelo-sandbox")
        )

    def test_discovery_without_cluster_crs(self):
        """If the Cluster CRs cannot be read, no compute cluster is found."""
        self._given_compute_cluster()
        self.cluster_crs_returncode = 1

        with patch.object(sandbox.subprocess, "run", side_effect=self._fake_run):
            clusters = sandbox._registered_compute_clusters()

        self.assertEqual(clusters, [])
        self.assertEqual(len(self.log), 1)


def _completed(args, returncode=0, stdout=""):
    return subprocess.CompletedProcess(args, returncode, stdout, "")


class KueueDemoTest(TestCase):
    """Tests for demo kueue, and for sync on a cluster that runs Kueue."""

    _control_plane = "k3d-michelangelo-sandbox"
    _compute = "k3d-michelangelo-compute-0"

    def setUp(self):
        """A compute-mode sandbox whose compute-0 runs an earlier demo's Kueue."""
        self.log = []
        self.slept = 0
        # Installed Helm releases, (context, release, namespace) → their values,
        # and the values of every helm upgrade --install, in order.
        self.releases = {
            (context, "michelangelo-compute", "michelangelo-compute"): {}
            for context in (self._control_plane, self._compute)
        }
        self.releases[(self._compute, "kueue", "kueue-system")] = {}
        self.installs = []
        # What successive reads of compute-0 list; the last answer repeats.
        self.workload_reads = [[]]
        self.crd_reads = [[]]
        # Exit code of every read of compute-0's workloads.
        self.workloads_returncode = 0
        # The command argument that makes its command fail.
        self.failing = None

    @staticmethod
    def _flag(args, name):
        return args[args.index(name) + 1]

    def _release(self, args, release_index):
        """Return the (context, release, namespace) a helm command names."""
        return (
            self._flag(args, "--kube-context"),
            args[release_index],
            self._flag(args, "-n"),
        )

    def _fake_exec(self, *args, **kwargs):
        """Record a command, apply it to the sandbox and answer its reads."""
        args = list(args)
        install = args[:3] == ["helm", "upgrade", "--install"]
        if install:
            # The values file is removed once the command returns.
            i = args.index("-f") + 1
            with open(args[i]) as f:
                values = yaml.safe_load(f)
            args[i] = "<file>"
        self.log.append(("exec", tuple(args)))
        if self.failing in args:
            raise subprocess.CalledProcessError(1, args)
        if install:
            release = self._release(args, 3)
            self.releases[release] = values
            self.installs.append((*release[:2], values))
        if args[:2] == ["helm", "uninstall"]:
            del self.releases[self._release(args, 2)]
        if "version" in args:
            return json.dumps({"serverVersion": {"major": "1", "minor": "31"}})
        if args[:3] == ["helm", "show", "crds"]:
            return ""
        if "minio-credentials" in args:
            return f"{_b64('test-access-key')} {_b64('test-secret-key')}"
        return None

    @staticmethod
    def _next(reads):
        return reads.pop(0) if len(reads) > 1 else reads[0]

    def _fake_run(self, args, **kwargs):
        """Answer the reads that may fail from the sandbox state."""
        self.log.append(("run", tuple(args)))
        if args[:2] == ["helm", "status"] and "--kube-context" in args:
            return _completed(args, 0 if self._release(args, 2) in self.releases else 1)
        if args[:2] == ["helm", "status"]:  # sync's michelangelo release
            return _completed(args, 0, '{"info":{"status":"deployed"}}')
        if args[:3] == ["helm", "get", "values"]:
            values = self.releases.get(self._release(args, 3))
            if values is None:
                return _completed(args, 1)
            # helm prints null for a release installed without values.
            return _completed(args, 0, json.dumps(values or None))
        if "workloads.kueue.x-k8s.io" in args:
            workloads = "\n".join(self._next(self.workload_reads))
            return _completed(args, self.workloads_returncode, workloads)
        if sandbox._kueue_crds_jsonpath in args:
            return _completed(args, 0, " ".join(self._next(self.crd_reads)))
        if "clusters.michelangelo.api" in args:
            return _completed(args, 0, "michelangelo-compute-0 michelangelo-sandbox")
        if args[:3] == ["kubectl", "config", "get-contexts"]:
            return _completed(args, 0, f"{self._compute}\n{self._control_plane}")
        if args[:3] == ["k3d", "cluster", "get"]:
            return _completed(args)
        return _completed(args, 1)  # e.g. no hand-built object to adopt

    def _record_cluster_cr(self, cluster_name, scheduler_type=""):
        self.log.append(("cluster-cr", (cluster_name, scheduler_type)))

    def _record_sleep(self, seconds):
        self.slept += seconds

    def _demo(self, cluster_name="michelangelo-compute-0"):
        p = argparse.ArgumentParser()
        sandbox.init_arguments(p)
        ns = p.parse_args(["demo", "kueue", "--compute-cluster-name", cluster_name])
        with (
            patch.object(sandbox, "_exec", side_effect=self._fake_exec),
            patch.object(sandbox.subprocess, "run", side_effect=self._fake_run),
            patch.object(sandbox, "_helm_ensure_repos"),
            patch.object(
                sandbox,
                "_create_compute_cluster_crd",
                side_effect=self._record_cluster_cr,
            ),
            patch.object(sandbox.time, "sleep", side_effect=self._record_sleep),
        ):
            sandbox._create_kueue_demo_crs(ns)

    def _demo_fails(self) -> str:
        """Run the demo, expecting it to exit; return its error message."""
        with (
            patch.object(sandbox, "_err_exit", side_effect=SystemExit(1)) as mock_err,
            self.assertRaises(SystemExit),
        ):
            self._demo()
        return mock_err.call_args[0][0]

    def _sync(self):
        p = argparse.ArgumentParser()
        sandbox.init_arguments(p)
        ns = p.parse_args(["sync"])
        with (
            patch.object(sandbox, "_exec", side_effect=self._fake_exec),
            patch.object(sandbox.subprocess, "run", side_effect=self._fake_run),
            patch.object(sandbox, "_helm_ensure_repos"),
            patch.object(sandbox, "_helm_wait"),
            patch.object(sandbox, "_ensure_mysql_running"),
            patch.object(sandbox, "_ensure_object_store_running"),
            patch.object(sandbox, "_refresh_mysql_schema"),
            patch.object(sandbox, "_ensure_credentials_secret"),
        ):
            sandbox._sync(ns)

    def _installed(self, context, release):
        """Return the values of the last install of the release on the cluster."""
        return next(
            values
            for c, r, values in reversed(self.installs)
            if (c, r) == (context, release)
        )

    def _steps(self):
        """Name the logged calls whose order matters."""
        steps = []
        for kind, args in self.log:
            if kind == "cluster-cr":
                steps.append(f"mark {args[1]}" if args[1] else "unmark")
            elif kind == "run":
                if "workloads.kueue.x-k8s.io" in args:
                    steps.append("list workloads")
                elif sandbox._kueue_crds_jsonpath in args:
                    steps.append("list kueue crds")
            elif "delete" in args:
                steps.append(f"delete {self._flag(args, 'delete')}")
            elif "wait" in args:
                steps.append(f"wait {self._flag(args, 'wait')}")
            elif args[:2] in (("helm", "uninstall"), ("helm", "test")):
                steps.append(" ".join(args[1:3]))
            elif args[:3] == ("helm", "upgrade", "--install"):
                steps.append(f"install {args[3]}")
        return steps

    def test_demo_replaces_a_standalone_kueue_in_order(self):
        """Drain, delete the queues, uninstall, then the charts and the mark."""
        self.workload_reads = [["workload.kueue.x-k8s.io/raycluster-demo-1a2b3"], []]
        self.crd_reads = [["workloads.kueue.x-k8s.io"], []]

        self._demo()

        self.assertEqual(
            self._steps(),
            [
                "unmark",
                "list workloads",
                "list workloads",
                "delete localqueue/ma-ma-dev-test",
                "delete clusterqueue/cluster-queue",
                "delete resourceflavor/default-flavor",
                "uninstall kueue",
                "list kueue crds",
                "list kueue crds",
                "install michelangelo-compute",
                "delete clusterrolebinding/ray-manager-binding",
                "test michelangelo-compute",
                "wait --for=condition=Established",
                "wait --for=condition=Available",
                "install michelangelo-compute-queues",
                "mark SCHEDULER_TYPE_KUEUE",
            ],
        )
        context = self._compute
        commands = [args for kind, args in self.log if kind in ("exec", "run")]
        for command in [
            (
                "helm",
                "status",
                "kueue",
                "--kube-context",
                context,
                "-n",
                "kueue-system",
            ),
            (
                "kubectl",
                "--context",
                context,
                "get",
                "workloads.kueue.x-k8s.io",
                "--all-namespaces",
                "-o",
                "name",
            ),
            (
                "kubectl",
                "--context",
                context,
                "-n",
                "default",
                "delete",
                "localqueue/ma-ma-dev-test",
                "--wait",
                "--ignore-not-found",
                "--timeout=2m",
            ),
            (
                "kubectl",
                "--context",
                context,
                "delete",
                "clusterqueue/cluster-queue",
                "--wait",
                "--ignore-not-found",
                "--timeout=2m",
            ),
            (
                "kubectl",
                "--context",
                context,
                "delete",
                "resourceflavor/default-flavor",
                "--wait",
                "--ignore-not-found",
                "--timeout=2m",
            ),
            (
                "helm",
                "uninstall",
                "kueue",
                "--kube-context",
                context,
                "-n",
                "kueue-system",
                "--wait",
            ),
            (
                "kubectl",
                "--context",
                context,
                "get",
                "crd",
                "-o",
                'jsonpath={.items[?(@.spec.group=="kueue.x-k8s.io")].metadata.name}',
            ),
            (
                "kubectl",
                "--context",
                context,
                "wait",
                "--for=condition=Established",
                "crd",
                "-l",
                "app.kubernetes.io/name=kueue,"
                "app.kubernetes.io/instance=michelangelo-compute",
                "--timeout=5m",
            ),
            (
                "kubectl",
                "--context",
                context,
                "-n",
                "michelangelo-compute",
                "wait",
                "--for=condition=Available",
                "deployment/michelangelo-compute-kueue-controller-manager",
                "--timeout=5m",
            ),
            (
                "helm",
                "upgrade",
                "--install",
                "michelangelo-compute-queues",
                str(sandbox._compute_queues_chart_dir),
                "--kube-context",
                context,
                "-n",
                "michelangelo-compute",
                "-f",
                "<file>",
                "--wait",
            ),
        ]:
            with self.subTest(command=command[:4]):
                self.assertIn(command, commands)
        self.assertNotIn((context, "kueue", "kueue-system"), self.releases)
        self.assertEqual(
            self._installed(context, "michelangelo-compute")["kueue"],
            {"enabled": True},
        )
        self.assertEqual(self.slept, 10)

    def test_demo_without_a_standalone_kueue_skips_the_replacement(self):
        """No standalone release: no unmark, drain, delete or uninstall."""
        del self.releases[(self._compute, "kueue", "kueue-system")]

        self._demo()

        self.assertEqual(
            self._steps(),
            [
                "install michelangelo-compute",
                "delete clusterrolebinding/ray-manager-binding",
                "test michelangelo-compute",
                "wait --for=condition=Established",
                "wait --for=condition=Available",
                "install michelangelo-compute-queues",
                "mark SCHEDULER_TYPE_KUEUE",
            ],
        )

    def test_demo_queues_get_the_demo_quota_and_local_queue(self):
        """The queues chart gets 8 CPU / 16Gi and the demo project's LocalQueue."""
        self._demo()

        self.assertEqual(
            self._installed(self._compute, "michelangelo-compute-queues"),
            {"quota": {"cpu": 8, "memory": "16Gi"}, "localQueues": ["ma-ma-dev-test"]},
        )

    def test_demo_keeps_the_operator_toggles_of_the_release(self):
        """An operator the sandbox was created without stays disabled."""
        cases = {
            (): {},
            ("spark",): {"spark-operator": {"enabled": False}},
            ("ray", "spark"): {
                "kuberay-operator": {"enabled": False},
                "spark-operator": {"enabled": False},
            },
        }
        for exclude, toggles in cases.items():
            with self.subTest(exclude=exclude):
                self.setUp()
                release = (
                    self._compute,
                    "michelangelo-compute",
                    "michelangelo-compute",
                )
                self.releases[release] = dict(toggles)

                self._demo()

                values = self._installed(self._compute, "michelangelo-compute")
                values.pop("runtime")
                self.assertEqual(values, {**toggles, "kueue": {"enabled": True}})

    def test_demo_enables_kueue_that_the_release_turned_off(self):
        """The release's kueue.enabled: false becomes true; its other keys stay."""
        release = (self._compute, "michelangelo-compute", "michelangelo-compute")
        self.releases[release] = {
            "kueue": {"enabled": False, "controllerManager": {"replicas": 3}}
        }

        self._demo()

        self.assertEqual(
            self._installed(self._compute, "michelangelo-compute")["kueue"],
            {"enabled": True, "controllerManager": {"replicas": 3}},
        )

    def test_demo_on_the_sandbox_cluster_uses_control_plane_values(self):
        """Single-cluster mode: the control plane's release gets Kueue."""
        self.releases[(self._control_plane, "kueue", "kueue-system")] = {}

        self._demo(cluster_name="michelangelo-sandbox")

        values = self._installed(self._control_plane, "michelangelo-compute")
        self.assertEqual(
            values["runtime"]["objectStorageCredentials"],
            {"name": "minio-credentials", "create": False},
        )
        self.assertEqual(values["kueue"], {"enabled": True})
        self.assertNotIn((self._control_plane, "kueue", "kueue-system"), self.releases)
        self.assertIn((self._compute, "kueue", "kueue-system"), self.releases)
        # Its chart tests run in create and sync, once the apiserver is up.
        self.assertNotIn("test michelangelo-compute", self._steps())
        self.assertEqual(
            [args for kind, args in self.log if kind == "cluster-cr"],
            [
                ("michelangelo-sandbox", ""),
                ("michelangelo-sandbox", "SCHEDULER_TYPE_KUEUE"),
            ],
        )

    def test_demo_calls_name_their_cluster(self):
        """Every helm and kubectl call that reaches a cluster names it."""
        self.workload_reads = [["workload.kueue.x-k8s.io/raycluster-demo-1a2b3"], []]
        self.crd_reads = [["workloads.kueue.x-k8s.io"], []]

        self._demo()

        client_only = [("helm", "dependency"), ("helm", "show")]
        for kind, args in self.log:
            if kind == "cluster-cr" or args[:2] in client_only:
                continue
            with self.subTest(args=args):
                flag = "--kube-context" if args[0] == "helm" else "--context"
                # Compute clusters copy the control plane's object-storage keys.
                if "minio-credentials" in args:
                    self.assertEqual(self._flag(args, flag), self._control_plane)
                else:
                    self.assertEqual(self._flag(args, flag), self._compute)

    def test_workloads_that_remain_stop_the_demo(self):
        """Workloads left after 5 minutes stop the demo before any delete."""
        self.workload_reads = [["workload.kueue.x-k8s.io/raycluster-demo-1a2b3"]]

        message = self._demo_fails()

        self.assertEqual(self.slept, 300)
        self.assertEqual(self._steps()[-1], "list workloads")
        for part in [
            self._compute,
            "5 minutes",
            "get workloads --all-namespaces",
            "ma sandbox demo kueue again",
        ]:
            self.assertIn(part, message)

    def test_failed_workload_reads_stop_the_demo(self):
        """A read that fails is not an empty list: the release stays installed."""
        self.workloads_returncode = 1

        message = self._demo_fails()

        self.assertEqual(self.slept, 300)
        self.assertNotIn("uninstall kueue", self._steps())
        self.assertIn((self._compute, "kueue", "kueue-system"), self.releases)
        self.assertIn("Kueue workloads remain", message)

    def test_queue_object_left_after_two_minutes_stops_the_demo(self):
        """A queue object Kueue does not release stops the demo before uninstall."""
        self.failing = "clusterqueue/cluster-queue"

        message = self._demo_fails()

        self.assertEqual(self._steps()[-1], "delete clusterqueue/cluster-queue")
        self.assertIn((self._compute, "kueue", "kueue-system"), self.releases)
        for part in [
            "clusterqueue/cluster-queue",
            "2 minutes",
            f"kubectl --context {self._compute} -n kueue-system get pods",
            "ma sandbox demo kueue again",
        ]:
            self.assertIn(part, message)

    def test_kueue_crds_left_after_two_minutes_stop_the_demo(self):
        """CRDs still being deleted stop the demo before the chart installs."""
        self.crd_reads = [["clusterqueues.kueue.x-k8s.io"]]

        message = self._demo_fails()

        self.assertEqual(self.slept, 120)
        self.assertEqual(self._steps()[-1], "list kueue crds")
        for part in [
            self._compute,
            "2 minutes",
            "finalizers",
            "ma sandbox demo kueue again",
        ]:
            self.assertIn(part, message)

    def test_kueue_crds_not_established_stop_the_demo(self):
        """CRDs that are not Established stop the demo before the queues chart."""
        self.failing = "--for=condition=Established"

        message = self._demo_fails()

        self.assertEqual(self._steps()[-1], "wait --for=condition=Established")
        for part in [
            "Established",
            "5 minutes",
            "helm status michelangelo-compute --kube-context "
            f"{self._compute} -n michelangelo-compute",
            "ma sandbox demo kueue again",
        ]:
            self.assertIn(part, message)

    def test_kueue_controller_not_available_stops_the_demo(self):
        """An unavailable controller stops the demo before the queues chart."""
        self.failing = "--for=condition=Available"

        message = self._demo_fails()

        self.assertEqual(self._steps()[-1], "wait --for=condition=Available")
        for part in [
            "Available",
            "5 minutes",
            f"kubectl --context {self._compute} -n michelangelo-compute get pods",
            "ma sandbox demo kueue again",
        ]:
            self.assertIn(part, message)

    def test_sync_after_demo_kueue_keeps_kueue_enabled(self):
        """Sync keeps kueue.enabled=true and its kueue block, so the CRDs stay."""
        for cluster, context, other in [
            ("michelangelo-compute-0", self._compute, self._control_plane),
            ("michelangelo-sandbox", self._control_plane, self._compute),
        ]:
            with self.subTest(cluster=cluster):
                self.setUp()
                self._demo(cluster_name=cluster)
                release = (context, "michelangelo-compute", "michelangelo-compute")
                self.releases[release]["kueue"]["controllerManager"] = {"replicas": 3}
                self.installs = []

                self._sync()

                self.assertEqual(
                    [install[:2] for install in self.installs],
                    [
                        (self._control_plane, "michelangelo-compute"),
                        (self._compute, "michelangelo-compute"),
                    ],
                )
                self.assertEqual(
                    self._installed(context, "michelangelo-compute")["kueue"],
                    {"enabled": True, "controllerManager": {"replicas": 3}},
                )
                self.assertNotIn(
                    "kueue", self._installed(other, "michelangelo-compute")
                )
                queues = (
                    context,
                    "michelangelo-compute-queues",
                    "michelangelo-compute",
                )
                self.assertIn(queues, self.releases)

    def test_sync_never_turns_kueue_on(self):
        """A kueue block that is off, or has no enabled key, stays out of sync."""
        for kueue in [{"enabled": False}, {"controllerManager": {"replicas": 3}}]:
            with self.subTest(kueue=kueue):
                self.setUp()
                release = (
                    self._compute,
                    "michelangelo-compute",
                    "michelangelo-compute",
                )
                self.releases[release] = {"kueue": kueue}

                self._sync()

                for context in [self._control_plane, self._compute]:
                    self.assertNotIn(
                        "kueue", self._installed(context, "michelangelo-compute")
                    )

    def test_sync_without_a_compute_release_installs_chart_defaults(self):
        """No release to read: sync installs with chart defaults and goes on."""
        self.releases = {}

        self._sync()

        for context in [self._control_plane, self._compute]:
            with self.subTest(context=context):
                values = self._installed(context, "michelangelo-compute")
                self.assertEqual(list(values), ["runtime"])
        self.assertEqual(self._steps()[-1], "test michelangelo-compute")


# The optional Secret comes first, so on a key both set the ConfigMap wins.
_CREDENTIALS_ENV_FROM = [
    {"secretRef": {"name": sandbox._compute_credentials_secret, "optional": True}},
    {"configMapRef": {"name": "michelangelo-config"}},
]


class CredentialsSecretEnvFromTest(TestCase):
    """Tests for the sandbox workloads that read the credentials Secret."""

    def test_model_sync_reads_the_secret_before_the_config_map(self):
        """The DaemonSet _apply_model_sync renders lists the Secret first."""
        with (
            patch.object(sandbox.subprocess, "run") as mock_run,
            patch.object(sandbox, "_exec"),
        ):
            mock_run.return_value = Mock(stdout="")
            sandbox._apply_model_sync("test-is", context="k3d-test-cluster")

        applied = [
            doc
            for c in mock_run.call_args_list
            for doc in yaml.safe_load_all(c.kwargs.get("input") or "")
            if doc
        ]
        (daemon_set,) = [doc for doc in applied if doc["kind"] == "DaemonSet"]
        (container,) = daemon_set["spec"]["template"]["spec"]["containers"]
        self.assertEqual(container["envFrom"], _CREDENTIALS_ENV_FROM)

    def test_demo_spark_application_reads_the_secret_before_the_config_map(self):
        """Driver and executor of the demo SparkApplication list the Secret first."""
        with open(sandbox._dir / "resources" / "spark-app.yaml") as f:
            app = yaml.safe_load(f)

        for role in ["driver", "executor"]:
            with self.subTest(role=role):
                self.assertEqual(app["spec"][role]["envFrom"], _CREDENTIALS_ENV_FROM)


class ArgumentParsingTest(TestCase):
    """Tests for CLI argument parsing."""

    def _parse(self, args):
        parser = argparse.ArgumentParser()
        sandbox.init_arguments(parser)
        return parser.parse_args(args)

    def test_create_accepts_set_flag(self):
        """`ma sandbox create --set` should parse into ns.helm_set, same as sync."""
        ns = self._parse(
            [
                "create",
                "--set",
                "images.apiserver.tag=0.5.0-rc.1",
                "--set",
                "images.worker.tag=0.5.0-rc.1",
            ]
        )
        self.assertEqual(
            ns.helm_set,
            ["images.apiserver.tag=0.5.0-rc.1", "images.worker.tag=0.5.0-rc.1"],
        )

    def test_create_set_defaults_to_empty(self):
        """`ma sandbox create` without --set should default helm_set to []."""
        ns = self._parse(["create"])
        self.assertEqual(ns.helm_set, [])

    def test_create_object_store_defaults_to_minio(self):
        """`ma sandbox create` without --object-store should default to minio."""
        ns = self._parse(["create"])
        self.assertEqual(ns.object_store, "minio")

    def test_sync_object_store_defaults_to_minio(self):
        """`ma sandbox sync` without --object-store should default to minio."""
        ns = self._parse(["sync"])
        self.assertEqual(ns.object_store, "minio")

    def test_create_accepts_object_store_seaweedfs(self):
        """`ma sandbox create --object-store seaweedfs` parses through."""
        ns = self._parse(["create", "--object-store", "seaweedfs"])
        self.assertEqual(ns.object_store, "seaweedfs")

    def test_create_rejects_unknown_object_store(self):
        """An unsupported --object-store value is rejected by argparse."""
        with self.assertRaises(SystemExit):
            self._parse(["create", "--object-store", "ceph"])


class DeployServicesObjectStoreTest(TestCase):
    """Tests for `_deploy_services`'s object-store resource selection."""

    def _run_deploy_services(self, object_store):
        ns = argparse.Namespace(
            workflow="cadence",
            object_store=object_store,
            exclude=[],
            include_experimental=[],
            create_compute_cluster=False,
            compute_cluster_name="test-compute-cluster",
        )
        with (
            patch("michelangelo.cli.sandbox.sandbox._kube_apply") as mock_kube_apply,
            patch("michelangelo.cli.sandbox.sandbox._create_bucket_setup"),
            patch("michelangelo.cli.sandbox.sandbox._import_log_persistence_images"),
            patch("michelangelo.cli.sandbox.sandbox._ensure_credentials_secret"),
            patch("michelangelo.cli.sandbox.sandbox._assert_command"),
            patch(
                "michelangelo.cli.sandbox.sandbox.subprocess.check_output",
                return_value=b"",
            ),
            patch("michelangelo.cli.sandbox.sandbox._install_compute_chart"),
            patch("michelangelo.cli.sandbox.sandbox._kube_wait"),
            patch("michelangelo.cli.sandbox.sandbox._deploy_app_services"),
            patch("michelangelo.cli.sandbox.sandbox._test_compute_chart"),
            patch("michelangelo.cli.sandbox.sandbox._create_cadence_domain"),
            patch("michelangelo.cli.sandbox.sandbox._create_compute_cluster_crd"),
            patch("michelangelo.cli.sandbox.sandbox._create_compute_cluster_secrets"),
        ):
            sandbox._deploy_services(ns)
        applied = {call.args[0].name for call in mock_kube_apply.call_args_list}
        return applied

    def test_default_object_store_applies_minio_yaml(self):
        """object_store="minio" (default) applies minio.yaml only."""
        applied = self._run_deploy_services("minio")
        self.assertIn("minio.yaml", applied)
        self.assertNotIn("weed-server.yaml", applied)

    def test_seaweedfs_object_store_applies_weed_server_yaml(self):
        """object_store="seaweedfs" applies weed-server.yaml only."""
        applied = self._run_deploy_services("seaweedfs")
        self.assertIn("weed-server.yaml", applied)
        self.assertNotIn("minio.yaml", applied)


class K3dImageImportTest(TestCase):
    """Tests for _k3d_import_image."""

    @staticmethod
    def _fake_run(save_returncode):
        def run(cmd, **kwargs):
            if cmd[:2] == ["docker", "version"]:
                return Mock(returncode=0, stdout="linux/arm64\n")
            if cmd[:2] == ["docker", "save"]:
                return Mock(returncode=save_returncode)
            return Mock(returncode=0)

        return run

    @patch("michelangelo.cli.sandbox.sandbox.subprocess.run")
    def test_imports_single_platform_archive(self, mock_run):
        """k3d imports a tarball holding only the Docker server's platform."""
        mock_run.side_effect = self._fake_run(save_returncode=0)

        self.assertTrue(sandbox._k3d_import_image("nginx:1.27-alpine"))

        save = mock_run.call_args_list[1][0][0]
        k3d = mock_run.call_args_list[2][0][0]
        self.assertEqual(save[:4], ["docker", "save", "--platform", "linux/arm64"])
        self.assertEqual(save[-1], "nginx:1.27-alpine")
        archive = save[save.index("-o") + 1]
        self.assertEqual(k3d[:4], ["k3d", "image", "import", archive])

    @patch("michelangelo.cli.sandbox.sandbox.subprocess.run")
    def test_falls_back_to_plain_import(self, mock_run):
        """Without `docker save --platform`, k3d imports the image by name."""
        mock_run.side_effect = self._fake_run(save_returncode=1)

        self.assertTrue(sandbox._k3d_import_image("nginx:1.27-alpine"))

        k3d = mock_run.call_args_list[2][0][0]
        self.assertEqual(k3d[:4], ["k3d", "image", "import", "nginx:1.27-alpine"])


class ImportLogPersistenceImagesTest(TestCase):
    """Tests for _import_log_persistence_images."""

    @patch("michelangelo.cli.sandbox.sandbox._LOG_PERSISTENCE_IMAGES", ["img:1"])
    @patch("michelangelo.cli.sandbox.sandbox._k3d_import_image", return_value=True)
    @patch("michelangelo.cli.sandbox.sandbox.subprocess.run")
    def test_failed_pull_imports_local_copy(self, mock_run, mock_import):
        """A failed pull still imports a copy already on the host."""
        mock_run.side_effect = lambda cmd, **kwargs: Mock(
            returncode=1 if cmd[:2] == ["docker", "pull"] else 0
        )
        sandbox._import_log_persistence_images()
        mock_import.assert_called_once_with("img:1")

    @patch("michelangelo.cli.sandbox.sandbox._LOG_PERSISTENCE_IMAGES", ["img:1"])
    @patch("michelangelo.cli.sandbox.sandbox._k3d_import_image")
    @patch("michelangelo.cli.sandbox.sandbox.subprocess.run")
    def test_failed_pull_without_local_copy_skips(self, mock_run, mock_import):
        """A failed pull with no copy on the host skips the image."""
        mock_run.return_value = Mock(returncode=1)
        sandbox._import_log_persistence_images()
        mock_import.assert_not_called()


class TestEnsurePodRunning:
    """_ensure_pod_running recreates a bare infra pod that is dead or stale."""

    WANTED = "mysql:8.0"

    def _run(self, stdout):
        """Run the helper with kubectl reporting `stdout`; return (calls, apply)."""
        from michelangelo.cli.sandbox.sandbox import _ensure_pod_running

        with (
            patch("michelangelo.cli.sandbox.sandbox._kube_apply") as apply,
            patch(
                "michelangelo.cli.sandbox.sandbox._manifest_pod_image",
                return_value=self.WANTED,
            ),
            patch("michelangelo.cli.sandbox.sandbox.subprocess.run") as run,
        ):
            run.return_value = Mock(stdout=stdout)
            _ensure_pod_running("mysql", "mysql.yaml")
        return [call.args[0] for call in run.call_args_list], apply

    def test_running_pod_with_current_image_is_left_alone(self):
        """A Running pod on the manifest's image is not touched."""
        commands, apply = self._run(f"Running|{self.WANTED}\n")

        assert len(commands) == 1
        apply.assert_not_called()

    def test_failed_pod_is_recreated(self):
        """A Failed (evicted) pod is deleted, re-applied and waited on."""
        commands, apply = self._run(f"Failed|{self.WANTED}\n")

        assert any(cmd[:4] == ["kubectl", "delete", "pod", "mysql"] for cmd in commands)
        assert any(
            cmd[:3] == ["kubectl", "wait", "--for=condition=ready"] for cmd in commands
        )
        apply.assert_called_once()

    def test_missing_pod_is_created(self):
        """A missing pod is created from the manifest."""
        _, apply = self._run("")

        apply.assert_called_once()

    def test_running_pod_with_stale_image_is_recreated(self):
        """A crash-looping pod still reports Running; a changed image replaces it."""
        _, apply = self._run("Running|ghcr.io/old/mysql:arm64-only\n")

        apply.assert_called_once()


class TestEnsureObjectStoreRunning:
    """_ensure_object_store_running checks only the selected backend's pod."""

    def _run(self, object_store):
        from michelangelo.cli.sandbox.sandbox import _ensure_object_store_running

        with patch("michelangelo.cli.sandbox.sandbox._ensure_pod_running") as ensure:
            _ensure_object_store_running(argparse.Namespace(object_store=object_store))
        return ensure

    def test_minio_backend_checks_minio_pod(self):
        """The default backend checks the minio pod from minio.yaml."""
        self._run("minio").assert_called_once_with("minio", "minio.yaml")

    def test_seaweedfs_backend_checks_seaweedfs_pod(self):
        """SeaweedFS must not recreate a minio pod, which would steal the Service."""
        self._run("seaweedfs").assert_called_once_with("seaweedfs", "weed-server.yaml")


class TestManifestPodImage:
    """_manifest_pod_image reads the Pod's image out of the resources manifest."""

    def test_reads_the_pod_image(self):
        """The helper finds the minio Pod's image in minio.yaml."""
        from michelangelo.cli.sandbox.sandbox import _manifest_pod_image

        assert _manifest_pod_image("minio.yaml", "minio").endswith(
            "minio:RELEASE.2025-05-24T17-08-30Z"
        )

    def test_unknown_pod_returns_empty(self):
        """A pod name that is not in the manifest yields no image."""
        from michelangelo.cli.sandbox.sandbox import _manifest_pod_image

        assert _manifest_pod_image("mysql.yaml", "nope") == ""
