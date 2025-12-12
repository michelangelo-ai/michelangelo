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
    @patch("michelangelo.cli.sandbox.sandbox.os.environ.get")
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
        mock_env_get,
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
            workflow="cadence",
            exclude=[],
            include_experimental=[],
            create_compute_cluster=True,
            compute_cluster_name="test-compute-cluster",
        )

        # Mock environment variable and dependencies
        mock_env_get.return_value = "test-token"
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
        mock_create_compute_cluster.assert_called_once_with("test-compute-cluster")
        mock_create_crd.assert_called_once_with("test-compute-cluster")
        mock_apply_rbac.assert_called_once_with("test-compute-cluster")
        mock_create_secrets.assert_called_once_with("test-compute-cluster")

    @patch("michelangelo.cli.sandbox.sandbox._kube_wait")
    @patch("michelangelo.cli.sandbox.sandbox._create_cadence_domain")
    @patch("michelangelo.cli.sandbox.sandbox._create_spark_operator")
    @patch("michelangelo.cli.sandbox.sandbox._create_kuberay_operator")
    @patch("michelangelo.cli.sandbox.sandbox.subprocess.check_output")
    @patch("michelangelo.cli.sandbox.sandbox._assert_command")
    @patch("michelangelo.cli.sandbox.sandbox._kube_create")
    @patch("michelangelo.cli.sandbox.sandbox._exec")
    @patch("michelangelo.cli.sandbox.sandbox.os.environ.get")
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
        mock_env_get,
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
            workflow="cadence",
            exclude=[],
            include_experimental=[],
            create_compute_cluster=False,
            compute_cluster_name="test-compute-cluster",
        )

        # Mock environment variable and dependencies
        mock_env_get.return_value = "test-token"
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
        mock_create_crd.assert_called_once_with("michelangelo-sandbox")
        mock_apply_rbac.assert_called_once_with("michelangelo-sandbox")
        mock_create_secrets.assert_called_once_with("michelangelo-sandbox")


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
        mock_create_config.assert_called_once_with(cluster_name)
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
        ns.compute_cluster_name = "test-compute"

        # Simulate cluster exists
        mock_check_output.return_value = b"test-compute"

        sandbox._delete(ns)

        # Verify check was called
        mock_check_output.assert_called_once()
        call_args = mock_check_output.call_args[0][0]
        self.assertIn("k3d", call_args)
        self.assertIn("cluster", call_args)
        self.assertIn("get", call_args)
        self.assertIn("test-compute", call_args)

        # Verify both clusters were deleted
        delete_calls = [c for c in mock_exec.call_args_list if "delete" in c[0]]
        self.assertEqual(len(delete_calls), 2)

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

        # Verify check was called
        mock_check_output.assert_called_once()

        # Verify only main cluster was deleted (not the compute cluster)
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
