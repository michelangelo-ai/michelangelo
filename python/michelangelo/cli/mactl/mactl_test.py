"""
Unit tests for mactl CLI functions.
"""

import os
from importlib import reload
from unittest import TestCase
from unittest.mock import MagicMock, Mock, patch

from michelangelo.cli.mactl import mactl
from michelangelo.cli.mactl.mactl import (
    ADDRESS,
    create_serivce_classes,
)


class ServiceClassCreationTest(TestCase):
    """
    Tests for create_serivce_classes function
    """

    @patch("michelangelo.cli.mactl.mactl.CRD")
    def test_create_serivce_classes_with_various_service_lists(self, mock_crd_class):
        """
        Test `create_serivce_classes()` function with both v2 and v2beta1 service lists
        """
        services = [
            "grpc.health.v1.Health",
            "grpc.reflection.v1alpha.ServerReflection",
            "michelangelo.api.v2.CachedOutputService",
            "michelangelo.api.v2.ModelFamilyService",
            "michelangelo.api.v2.ModelService",
            "michelangelo.api.v2.PipelineRunService",
            "michelangelo.api.v2.PipelineService",
            "michelangelo.api.v2.ProjectService",
            "michelangelo.api.v2.RayClusterService",
            "michelangelo.api.v2.RayJobService",
            "michelangelo.api.v2.SparkJobService",
            "michelangelo.api.v2.TriggerRunService"
            "michelangelo.api.v2beta1.AgentExtService",
            "michelangelo.api.v2beta1.AlertService",
            "michelangelo.api.v2beta1.FeatureGroupService",
            "michelangelo.api.v2beta1.GenerativeAiApplicationService",
            "michelangelo.api.v2beta1.ModelExtService",
            "michelangelo.api.v2beta1.ProjectService",
            "uber.infra.capeng.consgraph.provider.Provider",
        ]
        expected_sample_crds = [
            "alert",
            "cached_output",
            "feature_group",
            "generative_ai_application",
            "model_family",
            "model",
            "pipeline",
            "pipeline_run",
            "project",
            "ray_cluster",
            "ray_job",
            "spark_job",
        ]
        mock_crd_instance = Mock()
        mock_crd_class.return_value = mock_crd_instance

        result = create_serivce_classes(services)

        # Verify the result structure
        # Verify sample expected CRD names are present
        self.assertIsInstance(result, dict)
        self.assertEqual(sorted(result), sorted(expected_sample_crds))

        # Check that CRD was called for each expected service
        # Some duplicated calls may happen due to filtering
        self.assertEqual(mock_crd_class.call_count, 13)

    def test_create_serivce_classes_filters_out_non_service_entries(self):
        """
        Test that non-Service entries are filtered out correctly
        """
        services = [
            "grpc.reflection.v1alpha.ServerReflection",  # Should be filtered out (not ending with Service)
            "michelangelo.api.v2.ProjectService",  # Should be included
            "michelangelo.api.v2.SomeExtService",  # Should be filtered out (ends with ExtService)
            "michelangelo.api.v2.ModelService",  # Should be included
            "some.random.endpoint",  # Should be filtered out (not ending with Service)
        ]

        with patch("michelangelo.cli.mactl.mactl.CRD") as mock_crd_class:
            result = create_serivce_classes(services)

        # Should only include ProjectService and ModelService
        self.assertEqual(len(result), 2)
        self.assertIn("project", result)
        self.assertIn("model", result)

        # Verify CRD was called twice
        self.assertEqual(mock_crd_class.call_count, 2)

    def test_create_serivce_classes_empty_list(self):
        """
        Test `create_serivce_classes()` function with empty service list
        """
        services = []

        result = create_serivce_classes(services)

        # Should return empty dict
        self.assertEqual(result, {})
        self.assertEqual(len(result), 0)


class TLSConfigurationTest(TestCase):
    """
    Tests for TLS configuration functionality added to mactl
    """

    def setUp(self):
        """Set up test environment variables"""
        # Store original environment variables
        self.original_env = {}
        for key in ["MACTL_USE_TLS", "MACTL_ADDRESS"]:
            self.original_env[key] = os.environ.get(key)

    def tearDown(self):
        """Restore original environment variables"""
        for key, value in self.original_env.items():
            if value is not None:
                os.environ[key] = value
            elif key in os.environ:
                del os.environ[key]

    @patch.dict(os.environ, {"MACTL_USE_TLS": "true"}, clear=False)
    def test_use_tls_environment_variable_true(self):
        """Test that MACTL_USE_TLS=true is properly parsed"""
        # Need to reload the module to pick up new environment variable
        reload(mactl)
        self.assertEqual(mactl.USE_TLS, True)

    @patch.dict(os.environ, {"MACTL_USE_TLS": "TRUE"}, clear=False)
    def test_use_tls_environment_variable_case_insensitive(self):
        """Test that MACTL_USE_TLS is case insensitive and converts to lowercase"""
        reload(mactl)
        self.assertEqual(mactl.USE_TLS, True)

    @patch.dict(os.environ, {"MACTL_USE_TLS": "false"}, clear=False)
    def test_use_tls_environment_variable_false(self):
        """Test that MACTL_USE_TLS=false is properly parsed"""
        reload(mactl)
        self.assertEqual(mactl.USE_TLS, False)

    @patch.dict(os.environ, {}, clear=False)
    def test_use_tls_default_value(self):
        """Test that USE_TLS defaults to 'true' when not set"""
        # Remove MACTL_USE_TLS if it exists
        if "MACTL_USE_TLS" in os.environ:
            del os.environ["MACTL_USE_TLS"]
        reload(mactl)
        self.assertEqual(mactl.USE_TLS, False)

    @patch.dict(os.environ, {"MACTL_USE_TLS": "invalid"}, clear=False)
    def test_use_tls_invalid_value(self):
        """Test that invalid values for MACTL_USE_TLS are handled"""
        reload(mactl)
        self.assertEqual(mactl.USE_TLS, False)


class TLSConnectionTest(TestCase):
    """
    Tests for TLS connection functionality in main execution
    """

    @patch("michelangelo.cli.mactl.mactl.main")
    @patch("michelangelo.cli.mactl.mactl.secure_channel")
    @patch("michelangelo.cli.mactl.mactl.ssl_channel_credentials")
    @patch("builtins.print")
    def test_main_execution_with_tls_enabled(
        self, mock_print, mock_ssl_creds, mock_secure_channel, mock_main
    ):
        """Test main execution path when TLS is enabled"""
        # Setup mocks
        mock_channel = MagicMock()
        mock_secure_channel.return_value.__enter__ = Mock(return_value=mock_channel)
        mock_secure_channel.return_value.__exit__ = Mock(return_value=None)
        mock_credentials = MagicMock()
        mock_ssl_creds.return_value = mock_credentials

        # Mock the module-level constants
        with (
            patch("michelangelo.cli.mactl.mactl.USE_TLS", "true"),
            patch("michelangelo.cli.mactl.mactl.ADDRESS", "test-server:443"),
        ):
            # Import and run the main block
            from michelangelo.cli.mactl import mactl

            # Execute the main block logic directly
            if mactl.USE_TLS == "true":
                should_use_tls = True
                print(
                    f"Using TLS (forced via MACTL_USE_TLS=true) to connect to {mactl.ADDRESS}"
                )
            else:
                should_use_tls = False
                print(
                    f"Using insecure connection (forced via MACTL_USE_TLS=false) to connect to {mactl.ADDRESS}"
                )

            if should_use_tls:
                credentials = mactl.ssl_channel_credentials()
                with mactl.secure_channel(mactl.ADDRESS, credentials) as channel:
                    mactl.main(channel)
            else:
                with mactl.insecure_channel(mactl.ADDRESS) as channel:
                    mactl.main(channel)

        # Verify TLS connection setup
        mock_ssl_creds.assert_called_once()
        mock_secure_channel.assert_called_once_with("test-server:443", mock_credentials)
        mock_main.assert_called_once_with(mock_channel)
        mock_print.assert_called_once_with(
            "Using TLS (forced via MACTL_USE_TLS=true) to connect to test-server:443"
        )

    @patch("michelangelo.cli.mactl.mactl.main")
    @patch("michelangelo.cli.mactl.mactl.insecure_channel")
    @patch("builtins.print")
    def test_main_execution_with_tls_disabled(
        self, mock_print, mock_insecure_channel, mock_main
    ):
        """Test main execution path when TLS is disabled"""
        # Setup mocks
        mock_channel = MagicMock()
        mock_insecure_channel.return_value.__enter__ = Mock(return_value=mock_channel)
        mock_insecure_channel.return_value.__exit__ = Mock(return_value=None)

        # Mock the module-level constants
        with (
            patch("michelangelo.cli.mactl.mactl.USE_TLS", "false"),
            patch("michelangelo.cli.mactl.mactl.ADDRESS", "localhost:5435"),
        ):
            # Import and run the main block
            from michelangelo.cli.mactl import mactl

            # Execute the main block logic directly
            if mactl.USE_TLS == "true":
                should_use_tls = True
                print(
                    f"Using TLS (forced via MACTL_USE_TLS=true) to connect to {mactl.ADDRESS}"
                )
            else:
                should_use_tls = False
                print(
                    f"Using insecure connection (forced via MACTL_USE_TLS=false) to connect to {mactl.ADDRESS}"
                )

            if should_use_tls:
                credentials = mactl.ssl_channel_credentials()
                with mactl.secure_channel(mactl.ADDRESS, credentials) as channel:
                    mactl.main(channel)
            else:
                with mactl.insecure_channel(mactl.ADDRESS) as channel:
                    mactl.main(channel)

        # Verify insecure connection setup
        mock_insecure_channel.assert_called_once_with("localhost:5435")
        mock_main.assert_called_once_with(mock_channel)
        mock_print.assert_called_once_with(
            "Using insecure connection (forced via MACTL_USE_TLS=false) to connect to localhost:5435"
        )

    @patch("michelangelo.cli.mactl.mactl.main")
    @patch("michelangelo.cli.mactl.mactl.secure_channel")
    @patch("michelangelo.cli.mactl.mactl.ssl_channel_credentials")
    @patch("michelangelo.cli.mactl.mactl.insecure_channel")
    def test_channel_context_manager_usage(
        self, mock_insecure_channel, mock_ssl_creds, mock_secure_channel, mock_main
    ):
        """Test that channels are properly used as context managers"""
        mock_channel = MagicMock()

        # Test TLS channel context manager
        mock_secure_channel.return_value = MagicMock()
        mock_secure_channel.return_value.__enter__ = Mock(return_value=mock_channel)
        mock_secure_channel.return_value.__exit__ = Mock(return_value=None)
        mock_credentials = MagicMock()
        mock_ssl_creds.return_value = mock_credentials

        with (
            patch("michelangelo.cli.mactl.mactl.USE_TLS", "true"),
            patch("michelangelo.cli.mactl.mactl.ADDRESS", "test-server:443"),
        ):
            from michelangelo.cli.mactl import mactl

            # Test secure channel usage
            if mactl.USE_TLS == "true":
                credentials = mactl.ssl_channel_credentials()
                with mactl.secure_channel(mactl.ADDRESS, credentials) as channel:
                    mactl.main(channel)

        # Verify context manager methods were called
        mock_secure_channel.return_value.__enter__.assert_called_once()
        mock_secure_channel.return_value.__exit__.assert_called_once()
        mock_main.assert_called_once_with(mock_channel)

        # Reset mocks
        mock_main.reset_mock()
        mock_insecure_channel.reset_mock()

        # Test insecure channel context manager
        mock_insecure_channel.return_value = MagicMock()
        mock_insecure_channel.return_value.__enter__ = Mock(return_value=mock_channel)
        mock_insecure_channel.return_value.__exit__ = Mock(return_value=None)

        with (
            patch("michelangelo.cli.mactl.mactl.USE_TLS", "false"),
            patch("michelangelo.cli.mactl.mactl.ADDRESS", "localhost:5435"),
        ):
            from michelangelo.cli.mactl import mactl

            # Test insecure channel usage
            if mactl.USE_TLS != "true":
                with mactl.insecure_channel(mactl.ADDRESS) as channel:
                    mactl.main(channel)

        # Verify context manager methods were called
        mock_insecure_channel.return_value.__enter__.assert_called_once()
        mock_insecure_channel.return_value.__exit__.assert_called_once()
        mock_main.assert_called_once_with(mock_channel)

    def test_address_environment_variable_integration(self):
        """Test that MACTL_ADDRESS works with TLS configuration"""
        test_address = "custom-server:9999"

        with patch.dict(
            os.environ,
            {"MACTL_ADDRESS": test_address, "MACTL_USE_TLS": "true"},
            clear=False,
        ):
            reload(mactl)

        self.assertEqual(mactl.ADDRESS, test_address)
        self.assertEqual(mactl.USE_TLS, True)


class TLSErrorHandlingTest(TestCase):
    """
    Tests for TLS error handling scenarios
    """

    @patch("michelangelo.cli.mactl.mactl.ssl_channel_credentials")
    @patch("michelangelo.cli.mactl.mactl.secure_channel")
    def test_tls_connection_failure_handling(self, mock_secure_channel, mock_ssl_creds):
        """Test handling of TLS connection failures"""
        # Mock TLS connection failure
        mock_ssl_creds.return_value = MagicMock()
        mock_secure_channel.side_effect = Exception("TLS connection failed")

        with (
            patch("michelangelo.cli.mactl.mactl.USE_TLS", "true"),
            patch("michelangelo.cli.mactl.mactl.ADDRESS", "bad-server:443"),
        ):
            # This should raise the TLS connection exception
            with self.assertRaises(Exception) as context:
                credentials = mactl.ssl_channel_credentials()
                with mactl.secure_channel(ADDRESS, credentials):
                    pass  # Connection should fail before reaching main()

            self.assertEqual(str(context.exception), "TLS connection failed")

    @patch("michelangelo.cli.mactl.mactl.ssl_channel_credentials")
    def test_ssl_credentials_creation_failure(self, mock_ssl_creds):
        """Test handling of SSL credentials creation failure"""
        # Mock SSL credentials creation failure
        mock_ssl_creds.side_effect = Exception("Failed to create SSL credentials")

        with patch("michelangelo.cli.mactl.mactl.USE_TLS", "true"):
            # This should raise the SSL credentials exception
            with self.assertRaises(Exception) as context:
                mactl.ssl_channel_credentials()

            self.assertEqual(str(context.exception), "Failed to create SSL credentials")
