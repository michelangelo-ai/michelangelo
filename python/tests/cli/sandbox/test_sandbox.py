"""Unit tests for sandbox module."""

import argparse
import subprocess
from unittest import TestCase
from unittest.mock import Mock, patch

from michelangelo.cli.sandbox import sandbox


class CreateComputeClusterTest(TestCase):
    """Tests for _create_compute_cluster function."""

    @patch("michelangelo.cli.sandbox.sandbox._setup_buckets_in_cluster")
    @patch("michelangelo.cli.sandbox.sandbox._create_aws_credentials_in_cluster")
    @patch("michelangelo.cli.sandbox.sandbox._create_config_in_cluster")
    @patch("michelangelo.cli.sandbox.sandbox._deploy_minio_to_cluster")
    @patch("michelangelo.cli.sandbox.sandbox._exec")
    def test_create_compute_cluster_success(
        self,
        mock_exec,
        mock_deploy_minio,
        mock_create_config,
        mock_create_aws_creds,
        mock_setup_buckets,
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
        mock_deploy_minio.assert_called_once_with(cluster_name)
        mock_create_config.assert_called_once_with(cluster_name)
        mock_create_aws_creds.assert_called_once_with(cluster_name)
        mock_setup_buckets.assert_called_once_with(cluster_name)

    @patch("michelangelo.cli.sandbox.sandbox._setup_buckets_in_cluster")
    @patch("michelangelo.cli.sandbox.sandbox._create_aws_credentials_in_cluster")
    @patch("michelangelo.cli.sandbox.sandbox._create_config_in_cluster")
    @patch("michelangelo.cli.sandbox.sandbox._deploy_minio_to_cluster")
    @patch("michelangelo.cli.sandbox.sandbox._exec")
    def test_create_compute_cluster_with_ray_ports(
        self,
        mock_exec,
        mock_deploy_minio,
        mock_create_config,
        mock_create_aws_creds,
        mock_setup_buckets,
    ):
        """Test that Ray ports are properly configured."""
        cluster_name = "test-cluster"

        sandbox._create_compute_cluster(cluster_name)

        # Get the k3d cluster create call
        k3d_call = [c for c in mock_exec.call_args_list if c[0][0] == "k3d"][0]
        k3d_args = k3d_call[0]

        # Verify Ray ports are included
        port_args = [
            arg for arg in k3d_args if "10001" in str(arg) or "8265" in str(arg)
        ]
        self.assertGreater(len(port_args), 0, "Ray ports should be configured")

    @patch("michelangelo.cli.sandbox.sandbox._setup_buckets_in_cluster")
    @patch("michelangelo.cli.sandbox.sandbox._create_aws_credentials_in_cluster")
    @patch("michelangelo.cli.sandbox.sandbox._create_config_in_cluster")
    @patch("michelangelo.cli.sandbox.sandbox._deploy_minio_to_cluster")
    @patch("michelangelo.cli.sandbox.sandbox._exec")
    def test_create_compute_cluster_with_minio_ports(
        self,
        mock_exec,
        mock_deploy_minio,
        mock_create_config,
        mock_create_aws_creds,
        mock_setup_buckets,
    ):
        """Test that MinIO ports are properly configured."""
        cluster_name = "test-cluster"

        sandbox._create_compute_cluster(cluster_name)

        # Get the k3d cluster create call
        k3d_call = [c for c in mock_exec.call_args_list if c[0][0] == "k3d"][0]
        k3d_args = k3d_call[0]

        # Verify MinIO ports are included
        port_args = [
            arg for arg in k3d_args if "9190" in str(arg) or "9191" in str(arg)
        ]
        self.assertGreater(len(port_args), 0, "MinIO ports should be configured")


class DeployMinioToClusterTest(TestCase):
    """Tests for _deploy_minio_to_cluster function."""

    @patch("michelangelo.cli.sandbox.sandbox._exec")
    def test_deploy_minio_success(self, mock_exec):
        """Test successful MinIO deployment."""
        cluster_name = "test-cluster"

        sandbox._deploy_minio_to_cluster(cluster_name)

        # Verify kubectl apply was called with correct context
        mock_exec.assert_called_once()
        call_args = mock_exec.call_args[0]

        self.assertEqual(call_args[0], "kubectl")
        self.assertIn("--context", call_args)
        self.assertIn(f"k3d-{cluster_name}", call_args)
        self.assertIn("apply", call_args)
        self.assertIn("-f", call_args)


class CreateConfigInClusterTest(TestCase):
    """Tests for _create_config_in_cluster function."""

    @patch("michelangelo.cli.sandbox.sandbox._exec")
    def test_create_config_success(self, mock_exec):
        """Test successful config creation."""
        cluster_name = "test-cluster"

        sandbox._create_config_in_cluster(cluster_name)

        # Verify kubectl apply was called
        mock_exec.assert_called_once()
        call_args = mock_exec.call_args[0]

        self.assertEqual(call_args[0], "kubectl")
        self.assertIn("--context", call_args)
        self.assertIn(f"k3d-{cluster_name}", call_args)
        self.assertIn("apply", call_args)


class CreateAwsCredentialsInClusterTest(TestCase):
    """Tests for _create_aws_credentials_in_cluster function."""

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


class SetupBucketsInClusterTest(TestCase):
    """Tests for _setup_buckets_in_cluster function."""

    @patch("michelangelo.cli.sandbox.sandbox._exec")
    def test_setup_buckets_success(self, mock_exec):
        """Test successful bucket setup."""
        cluster_name = "test-cluster"

        sandbox._setup_buckets_in_cluster(cluster_name)

        # Verify kubectl apply was called
        mock_exec.assert_called_once()
        call_args = mock_exec.call_args[0]

        self.assertEqual(call_args[0], "kubectl")
        self.assertIn("--context", call_args)
        self.assertIn(f"k3d-{cluster_name}", call_args)
        self.assertIn("apply", call_args)


class CreateMaSystemNamespaceTest(TestCase):
    """Tests for _create_ma_system_namespace function."""

    @patch("michelangelo.cli.sandbox.sandbox._exec")
    @patch("michelangelo.cli.sandbox.sandbox.subprocess.check_output")
    def test_namespace_already_exists(self, mock_check_output, mock_exec):
        """Test when namespace already exists."""
        # Simulate namespace exists
        mock_check_output.return_value = b"ma-system"

        sandbox._create_ma_system_namespace()

        # Verify check was called but create was not
        mock_check_output.assert_called_once()
        mock_exec.assert_not_called()

    @patch("michelangelo.cli.sandbox.sandbox._exec")
    @patch("michelangelo.cli.sandbox.sandbox.subprocess.check_output")
    def test_namespace_does_not_exist(self, mock_check_output, mock_exec):
        """Test when namespace doesn't exist."""
        # Simulate namespace doesn't exist
        mock_check_output.side_effect = subprocess.CalledProcessError(1, "kubectl")

        sandbox._create_ma_system_namespace()

        # Verify create was called
        mock_exec.assert_called_once()
        call_args = mock_exec.call_args[0]

        self.assertEqual(call_args[0], "kubectl")
        self.assertIn("create", call_args)
        self.assertIn("namespace", call_args)
        self.assertIn("ma-system", call_args)


class CreateComputeClusterCrdTest(TestCase):
    """Tests for _create_compute_cluster_crd function."""

    @patch("michelangelo.cli.sandbox.sandbox._exec")
    @patch("michelangelo.cli.sandbox.sandbox._create_ma_system_namespace")
    @patch("michelangelo.cli.sandbox.sandbox.subprocess.check_output")
    def test_create_cluster_crd_success(
        self, mock_check_output, mock_create_ns, mock_exec
    ):
        """Test successful CRD creation."""
        cluster_name = "test-cluster"

        # Mock kubeconfig output
        mock_check_output.return_value = b"apiVersion: v1\nclusters:\n- cluster:\n    certificate-authority-data: dGVzdA==\n    server: https://127.0.0.1:12345\n  name: test"

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


class CreateComputeClusterSecretsTest(TestCase):
    """Tests for _create_compute_cluster_secrets function."""

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


class ApplyComputeClusterRbacTest(TestCase):
    """Tests for _apply_compute_cluster_rbac function."""

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


class DeleteTest(TestCase):
    """Tests for _delete function."""

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


class CreateFunctionComputeClusterTest(TestCase):
    """Tests for _create function compute cluster logic."""

    @patch("michelangelo.cli.sandbox.sandbox._kube_wait")
    @patch("michelangelo.cli.sandbox.sandbox._create_cadence_domain")
    @patch("michelangelo.cli.sandbox.sandbox._create_spark_operator")
    @patch("michelangelo.cli.sandbox.sandbox._create_kuberay_operator")
    @patch("michelangelo.cli.sandbox.sandbox.subprocess.check_output")
    @patch("michelangelo.cli.sandbox.sandbox._assert_command")
    @patch("michelangelo.cli.sandbox.sandbox._kube_create")
    @patch("michelangelo.cli.sandbox.sandbox._exec")
    @patch("michelangelo.cli.sandbox.sandbox.os.environ.get")
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
        """Test _create function with dedicated compute cluster."""
        # Setup namespace with create_compute_cluster=True
        ns = argparse.Namespace(
            workflow="cadence",
            exclude=[],
            include_experimental=[],
            create_compute_cluster=True,
            compute_cluster_name="test-compute-cluster",
        )

        # Mock environment variable
        mock_env_get.return_value = "test-token"
        mock_check_output.return_value = b""

        sandbox._create(ns)

        # Verify dedicated compute cluster functions were called
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
        """Test _create function without dedicated compute cluster (uses control plane)."""
        # Setup namespace with create_compute_cluster=False
        ns = argparse.Namespace(
            workflow="cadence",
            exclude=[],
            include_experimental=[],
            create_compute_cluster=False,
            compute_cluster_name="test-compute-cluster",
        )

        # Mock environment variable
        mock_env_get.return_value = "test-token"
        mock_check_output.return_value = b""

        sandbox._create(ns)

        # Verify dedicated compute cluster was NOT created
        mock_create_compute_cluster.assert_not_called()

        # Verify control plane cluster CRD/RBAC/secrets were created with sandbox cluster name
        mock_create_crd.assert_called_once_with("michelangelo-sandbox")
        mock_apply_rbac.assert_called_once_with("michelangelo-sandbox")
        mock_create_secrets.assert_called_once_with("michelangelo-sandbox")
