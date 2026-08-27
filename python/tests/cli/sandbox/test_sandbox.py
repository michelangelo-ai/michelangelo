"""Unit tests for sandbox module."""

import argparse
import subprocess
from unittest import TestCase
from unittest.mock import Mock, patch

from michelangelo.cli.sandbox import sandbox


class CreateFunctionTest(TestCase):
    """Tests for _create function logic."""

    @patch("michelangelo.cli.sandbox.sandbox._kube_wait")
    @patch("michelangelo.cli.sandbox.sandbox._create_cadence_domain")
    @patch("michelangelo.cli.sandbox.sandbox._create_spark_operator")
    @patch("michelangelo.cli.sandbox.sandbox._create_kuberay_operator")
    @patch("michelangelo.cli.sandbox.sandbox.subprocess.check_output")
    @patch("michelangelo.cli.sandbox.sandbox._assert_command")
    @patch("michelangelo.cli.sandbox.sandbox._kube_create")
    @patch("michelangelo.cli.sandbox.sandbox._exec")
    @patch("michelangelo.cli.sandbox.sandbox.tempfile.NamedTemporaryFile")
    @patch("michelangelo.cli.sandbox.sandbox._create_compute_cluster_secrets")
    @patch("michelangelo.cli.sandbox.sandbox._apply_compute_cluster_rbac")
    @patch("michelangelo.cli.sandbox.sandbox._create_compute_cluster_crd")
    @patch("michelangelo.cli.sandbox.sandbox._create_compute_cluster")
    def test_create_with_dedicated_compute_cluster(
        self,
        mock_create_compute_cluster,
        mock_create_crd,
        mock_apply_rbac,
        mock_create_secrets,
        mock_tempfile,
        mock_exec,
        mock_kube_create,
        mock_assert_command,
        mock_check_output,
        mock_create_kuberay,
        mock_create_spark,
        mock_create_cadence_domain,
        mock_kube_wait,
    ):
        """Test dedicated cluster functions called with compute cluster name."""
        # Setup namespace with create_compute_cluster=True
        ns = argparse.Namespace(
            name="sandbox",
            port_offset=0,
            workflow="cadence",
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
        mock_create_compute_cluster.assert_called_once_with(
            "test-compute-cluster", "sandbox", port_offset=0
        )
        mock_create_crd.assert_called_once_with("test-compute-cluster", "sandbox")
        mock_apply_rbac.assert_called_once_with("test-compute-cluster")
        mock_create_secrets.assert_called_once_with("test-compute-cluster", "sandbox")

    @patch("michelangelo.cli.sandbox.sandbox._kube_wait")
    @patch("michelangelo.cli.sandbox.sandbox._create_cadence_domain")
    @patch("michelangelo.cli.sandbox.sandbox._create_spark_operator")
    @patch("michelangelo.cli.sandbox.sandbox._create_kuberay_operator")
    @patch("michelangelo.cli.sandbox.sandbox.subprocess.check_output")
    @patch("michelangelo.cli.sandbox.sandbox._assert_command")
    @patch("michelangelo.cli.sandbox.sandbox._kube_create")
    @patch("michelangelo.cli.sandbox.sandbox._exec")
    @patch("michelangelo.cli.sandbox.sandbox.tempfile.NamedTemporaryFile")
    @patch("michelangelo.cli.sandbox.sandbox._create_compute_cluster_secrets")
    @patch("michelangelo.cli.sandbox.sandbox._apply_compute_cluster_rbac")
    @patch("michelangelo.cli.sandbox.sandbox._create_compute_cluster_crd")
    @patch("michelangelo.cli.sandbox.sandbox._create_compute_cluster")
    def test_create_without_dedicated_compute_cluster(
        self,
        mock_create_compute_cluster,
        mock_create_crd,
        mock_apply_rbac,
        mock_create_secrets,
        mock_tempfile,
        mock_exec,
        mock_kube_create,
        mock_assert_command,
        mock_check_output,
        mock_create_kuberay,
        mock_create_spark,
        mock_create_cadence_domain,
        mock_kube_wait,
    ):
        """Test control plane cluster functions called with sandbox cluster name."""
        # Setup namespace with create_compute_cluster=False
        ns = argparse.Namespace(
            name="sandbox",
            port_offset=0,
            workflow="cadence",
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

        sandbox._create(ns)

        # Verify dedicated compute cluster was NOT created
        mock_create_compute_cluster.assert_not_called()

        # Verify control plane cluster CRD/RBAC/secrets were created with
        # sandbox cluster name
        mock_create_crd.assert_called_once_with("michelangelo-sandbox", "sandbox")
        mock_apply_rbac.assert_called_once_with("michelangelo-sandbox")
        mock_create_secrets.assert_called_once_with("michelangelo-sandbox", "sandbox")


class ComputeClusterSetupTest(TestCase):
    """Tests for compute cluster setup functions."""

    @patch("michelangelo.cli.sandbox.sandbox._create_aws_credentials_in_cluster")
    @patch("michelangelo.cli.sandbox.sandbox._create_config_in_compute_cluster")
    @patch("michelangelo.cli.sandbox.sandbox._exec")
    def test_create_compute_cluster_success(
        self,
        mock_exec,
        mock_create_config,
        mock_create_aws_creds,
    ):
        """Test successful creation of compute cluster."""
        cluster_name = "test-compute-cluster"

        sandbox._create_compute_cluster(cluster_name)

        # Verify k3d cluster creation was called
        k3d_calls = [c for c in mock_exec.call_args_list if c[0][0] == "k3d"]
        self.assertEqual(len(k3d_calls), 1)

        # Verify cluster creation arguments
        k3d_call_args = k3d_calls[0][0]
        self.assertIn("cluster", k3d_call_args)
        self.assertIn("create", k3d_call_args)
        self.assertIn(cluster_name, k3d_call_args)

        # Verify helm install for kuberay was called
        helm_calls = [c for c in mock_exec.call_args_list if c[0][0] == "helm"]
        self.assertEqual(len(helm_calls), 1)

        # Verify all setup functions were called
        mock_create_config.assert_called_once_with(cluster_name, "sandbox")
        mock_create_aws_creds.assert_called_once_with(cluster_name)

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
        # Mock both check_output calls (kubeconfig and kubectl create token)
        mock_check_output.side_effect = [
            kubeconfig_yaml.encode(),
            b"test-token-value",
        ]

        sandbox._create_compute_cluster_secrets(cluster_name)

        # Verify check_output was called twice (kubeconfig and token)
        self.assertEqual(mock_check_output.call_count, 2)

        # Verify kubectl apply was called multiple times (CA secret and token secret)
        self.assertGreaterEqual(mock_exec.call_count, 2)

    @patch("michelangelo.cli.sandbox.sandbox._exec")
    def test_apply_rbac_success(self, mock_exec):
        """Test successful RBAC application."""
        cluster_name = "test-cluster"

        sandbox._apply_compute_cluster_rbac(cluster_name)

        # Verify kubectl apply was called
        mock_exec.assert_called_once()
        call_args = mock_exec.call_args[0]

        self.assertEqual(call_args[0], "kubectl")
        self.assertIn("--context", call_args)
        self.assertIn(f"k3d-{cluster_name}", call_args)
        self.assertIn("apply", call_args)
        self.assertIn("-f", call_args)

    @patch("michelangelo.cli.sandbox.sandbox._exec")
    @patch("michelangelo.cli.sandbox.sandbox.subprocess.check_output")
    def test_delete_with_existing_compute_cluster(self, mock_check_output, mock_exec):
        """Test deletion when compute cluster exists."""
        ns = Mock()
        ns.name = "sandbox"
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
        ns.name = "sandbox"
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
        ns.name = "sandbox"
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
        ns.name = "sandbox"
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


class NamespaceIsolationTest(TestCase):
    """Tests for namespace-based isolation of named sandboxes."""

    def test_cluster_name_and_context_always_resolve_to_shared_cluster(self):
        """Named sandboxes share the default sandbox's k3d cluster/context."""
        self.assertEqual(sandbox._cluster_name("dev2"), "michelangelo-sandbox")
        self.assertEqual(
            sandbox._cluster_name("dev2"), sandbox._cluster_name("sandbox")
        )
        self.assertEqual(sandbox._kube_context("dev2"), "k3d-michelangelo-sandbox")

    def test_namespace_default_vs_named(self):
        """Default sandbox uses 'default'; named sandboxes get their own."""
        self.assertEqual(sandbox._namespace("sandbox"), "default")
        self.assertEqual(sandbox._namespace("dev2"), "dev2")

    def test_build_helm_set_args_shares_infra_for_named_sandbox(self):
        """Named sandboxes disable bundled infra and point at the shared default."""
        ns = argparse.Namespace(
            name="dev2",
            port_offset=100,
            workflow="cadence",
            exclude=[],
            helm_set=[],
        )
        args = sandbox._build_helm_set_args(ns)
        self.assertIn("cadence.enabled=false", args)
        self.assertIn("temporal.enabled=false", args)
        self.assertIn("metadataStorage.host=mysql.default.svc.cluster.local", args)
        self.assertIn(
            "objectStorage.endpoint=minio.default.svc.cluster.local:9091", args
        )
        self.assertIn(
            "workflow.endpoint=michelangelo-cadence-frontend"
            ".default.svc.cluster.local:7833",
            args,
        )
        # NodePorts must be offset so they don't collide with the default
        # sandbox's own NodePorts on the shared cluster.
        self.assertIn("apiserver.service.nodePort=30109", args)
        self.assertIn("envoy.service.nodePort=30110", args)
        self.assertIn("ui.service.nodePort=30111", args)

    def test_build_helm_set_args_default_sandbox_unaffected(self):
        """Default sandbox keeps its own in-namespace endpoints, no overrides."""
        ns = argparse.Namespace(
            name="sandbox",
            port_offset=0,
            workflow="cadence",
            exclude=[],
            helm_set=[],
        )
        args = sandbox._build_helm_set_args(ns)
        self.assertNotIn("metadataStorage.host=mysql.default.svc.cluster.local", args)
        self.assertIn("workflow.endpoint=michelangelo-cadence-frontend:7833", args)

    @patch("michelangelo.cli.sandbox.sandbox._deploy_services")
    @patch("michelangelo.cli.sandbox.sandbox._exec")
    @patch("michelangelo.cli.sandbox.sandbox.subprocess.run")
    def test_create_named_sandbox_reuses_shared_cluster(
        self, mock_run, mock_exec, mock_deploy
    ):
        """`create --name dev2` must not create a new k3d cluster."""
        mock_run.return_value = Mock(returncode=0)
        ns = argparse.Namespace(name="dev2", port_offset=100, exclude=[])

        sandbox._create(ns)

        create_calls = [
            c
            for c in mock_exec.call_args_list
            if "create" in c[0] and "cluster" in c[0]
        ]
        self.assertEqual(create_calls, [])
        mock_deploy.assert_called_once_with(ns)

    @patch("michelangelo.cli.sandbox.sandbox._deploy_services")
    @patch("michelangelo.cli.sandbox.sandbox._exec")
    @patch("michelangelo.cli.sandbox.sandbox.subprocess.run")
    def test_create_named_sandbox_errors_when_shared_cluster_missing(
        self, mock_run, mock_exec, mock_deploy
    ):
        """`create --name dev2` should fail fast if there's no shared cluster."""
        mock_run.return_value = Mock(returncode=1)
        ns = argparse.Namespace(name="dev2", port_offset=100, exclude=[])

        with self.assertRaises(SystemExit):
            sandbox._create(ns)

        mock_deploy.assert_not_called()

    @patch("michelangelo.cli.sandbox.sandbox.subprocess.run")
    def test_delete_named_sandbox_only_removes_namespace(self, mock_run):
        """`delete --name dev2` must not touch the shared k3d cluster."""
        ns = Mock()
        ns.name = "dev2"

        sandbox._delete(ns)

        commands = [c[0][0] for c in mock_run.call_args_list]
        self.assertTrue(any("uninstall" in cmd for cmd in commands))
        self.assertTrue(any("delete" in cmd and "namespace" in cmd for cmd in commands))
        self.assertFalse(
            any("k3d" in cmd for cmd in commands),
            "named-sandbox delete must never touch k3d",
        )

    @patch("michelangelo.cli.sandbox.sandbox._exec")
    def test_start_named_sandbox_is_a_noop(self, mock_exec):
        """`start --name dev2` is a no-op — namespaces can't be started."""
        ns = Mock()
        ns.name = "dev2"
        sandbox._start(ns)
        mock_exec.assert_not_called()

    @patch("michelangelo.cli.sandbox.sandbox._exec")
    def test_stop_named_sandbox_is_a_noop(self, mock_exec):
        """`stop --name dev2` is a no-op — namespaces can't be stopped."""
        ns = Mock()
        ns.name = "dev2"
        sandbox._stop(ns)
        mock_exec.assert_not_called()
