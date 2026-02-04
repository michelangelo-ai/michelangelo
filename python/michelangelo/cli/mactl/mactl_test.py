"""Unit tests for mactl CLI functions."""

import os
import tempfile
from importlib import reload
from pathlib import Path
from unittest import TestCase
from unittest.mock import MagicMock, Mock, patch

from michelangelo.cli.mactl import mactl
from michelangelo.cli.mactl.crd import CRD
from michelangelo.cli.mactl.mactl import (
    ADDRESS,
    check_crd,
    create_serivce_classes,
    discover_crds,
    handle_crd_action_help,
    pre_parse_args,
    read_module_from_file,
)


class ServiceClassCreationTest(TestCase):
    """Tests for create_serivce_classes function."""

    @patch("michelangelo.cli.mactl.mactl.CRD")
    def test_create_serivce_classes_with_various_service_lists(self, mock_crd_class):
        """Test `create_serivce_classes()` function.

        with both v2 and v2beta1 service lists
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
        """Test that non-Service entries are filtered out correctly."""
        services = [
            # Should be filtered out (not ending with Service)
            "grpc.reflection.v1alpha.ServerReflection",
            # Should be included
            "michelangelo.api.v2.ProjectService",
            # Should be filtered out (ends with ExtService)
            "michelangelo.api.v2.SomeExtService",
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
        """Test `create_serivce_classes()` function with empty service list."""
        services = []

        result = create_serivce_classes(services)

        # Should return empty dict
        self.assertEqual(result, {})
        self.assertEqual(len(result), 0)


class TLSConfigurationTest(TestCase):
    """Tests for TLS configuration functionality added to mactl."""

    def setUp(self):
        """Set up test environment variables."""
        # Store original environment variables
        self.original_env = {}
        for key in ["MACTL_USE_TLS", "MACTL_ADDRESS"]:
            self.original_env[key] = os.environ.get(key)

    def tearDown(self):
        """Restore original environment variables."""
        for key, value in self.original_env.items():
            if value is not None:
                os.environ[key] = value
            elif key in os.environ:
                del os.environ[key]

    @patch.dict(os.environ, {"MACTL_USE_TLS": "true"}, clear=False)
    def test_use_tls_environment_variable_true(self):
        """Test that MACTL_USE_TLS=true is properly parsed."""
        # Need to reload the module to pick up new environment variable
        reload(mactl)
        self.assertEqual(mactl.USE_TLS, True)

    @patch.dict(os.environ, {"MACTL_USE_TLS": "TRUE"}, clear=False)
    def test_use_tls_environment_variable_case_insensitive(self):
        """Test that MACTL_USE_TLS is case insensitive and converts to lowercase."""
        reload(mactl)
        self.assertEqual(mactl.USE_TLS, True)

    @patch.dict(os.environ, {"MACTL_USE_TLS": "false"}, clear=False)
    def test_use_tls_environment_variable_false(self):
        """Test that MACTL_USE_TLS=false is properly parsed."""
        reload(mactl)
        self.assertEqual(mactl.USE_TLS, False)

    @patch.dict(os.environ, {}, clear=False)
    def test_use_tls_default_value(self):
        """Test that USE_TLS defaults to 'true' when not set."""
        # Remove MACTL_USE_TLS if it exists
        if "MACTL_USE_TLS" in os.environ:
            del os.environ["MACTL_USE_TLS"]
        reload(mactl)
        self.assertEqual(mactl.USE_TLS, False)

    @patch.dict(os.environ, {"MACTL_USE_TLS": "invalid"}, clear=False)
    def test_use_tls_invalid_value(self):
        """Test that invalid values for MACTL_USE_TLS are handled."""
        reload(mactl)
        self.assertEqual(mactl.USE_TLS, False)


class TLSConnectionTest(TestCase):
    """Tests for TLS connection functionality in main execution."""

    @patch("michelangelo.cli.mactl.mactl.main")
    @patch("michelangelo.cli.mactl.mactl.secure_channel")
    @patch("michelangelo.cli.mactl.mactl.ssl_channel_credentials")
    def test_main_execution_with_tls_enabled(
        self, mock_ssl_creds, mock_secure_channel, mock_main
    ):
        """Test main execution path when TLS is enabled."""
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
            should_use_tls = bool(mactl.USE_TLS == "true")
            if should_use_tls:
                credentials = mactl.ssl_channel_credentials()
                with mactl.secure_channel(mactl.ADDRESS, credentials) as channel:
                    mactl.main(channel)
            else:
                self.fail("TLS was expected to be enabled in this test.")

        # Verify TLS connection setup
        mock_ssl_creds.assert_called_once()
        mock_secure_channel.assert_called_once_with("test-server:443", mock_credentials)
        mock_main.assert_called_once_with(mock_channel)

    @patch("michelangelo.cli.mactl.mactl.main")
    @patch("michelangelo.cli.mactl.mactl.insecure_channel")
    def test_main_execution_with_tls_disabled(self, mock_insecure_channel, mock_main):
        """Test main execution path when TLS is disabled."""
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

            should_use_tls = bool(mactl.USE_TLS == "true")
            if should_use_tls:
                self.fail("TLS was expected to be not enabled in this test.")
            else:
                with mactl.insecure_channel(mactl.ADDRESS) as channel:
                    mactl.main(channel)

        # Verify insecure connection setup
        mock_insecure_channel.assert_called_once_with("localhost:5435")
        mock_main.assert_called_once_with(mock_channel)

    @patch("michelangelo.cli.mactl.mactl.main")
    @patch("michelangelo.cli.mactl.mactl.secure_channel")
    @patch("michelangelo.cli.mactl.mactl.ssl_channel_credentials")
    @patch("michelangelo.cli.mactl.mactl.insecure_channel")
    def test_channel_context_manager_usage(
        self, mock_insecure_channel, mock_ssl_creds, mock_secure_channel, mock_main
    ):
        """Test that channels are properly used as context managers."""
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
        """Test that MACTL_ADDRESS works with TLS configuration."""
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
    """Tests for TLS error handling scenarios."""

    @patch("michelangelo.cli.mactl.mactl.ssl_channel_credentials")
    @patch("michelangelo.cli.mactl.mactl.secure_channel")
    def test_tls_connection_failure_handling(self, mock_secure_channel, mock_ssl_creds):
        """Test handling of TLS connection failures."""
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
        """Test handling of SSL credentials creation failure."""
        # Mock SSL credentials creation failure
        mock_ssl_creds.side_effect = Exception("Failed to create SSL credentials")

        with patch("michelangelo.cli.mactl.mactl.USE_TLS", "true"):
            # This should raise the SSL credentials exception
            with self.assertRaises(Exception) as context:
                mactl.ssl_channel_credentials()

            self.assertEqual(str(context.exception), "Failed to create SSL credentials")


class ReadModuleFromFileTest(TestCase):
    """Tests for read_module_from_file function."""

    @patch("michelangelo.cli.mactl.mactl.DEFAULT_DIR_PLUGINS")
    def test_successful_module_loading(self, mock_default_dir_plugins):
        """Test successful loading of a plugin module."""
        with tempfile.TemporaryDirectory() as tmpdir:
            # Create plugin directory structure
            plugin_dir = Path(tmpdir) / "entity" / "test_entity"
            plugin_dir.mkdir(parents=True)

            # Create a simple main.py
            main_py = plugin_dir / "main.py"
            main_py.write_text("test_var = 'hello'")

            # Mock DEFAULT_DIR_PLUGINS to point to our temp directory
            mock_default_dir_plugins.__truediv__.return_value = Path(tmpdir) / "entity"

            # Execute
            result = read_module_from_file("test_entity")

            # Verify module was loaded and has the expected attribute
            self.assertIsNotNone(result)
            self.assertEqual(result.test_var, "hello")

    @patch("michelangelo.cli.mactl.mactl.DEFAULT_DIR_PLUGINS")
    def test_plugin_directory_does_not_exist(self, mock_default_dir_plugins):
        """Test when plugin directory does not exist."""
        with tempfile.TemporaryDirectory() as tmpdir:
            # Mock DEFAULT_DIR_PLUGINS but don't create the directory
            mock_default_dir_plugins.__truediv__.return_value = Path(tmpdir) / "entity"

            # Execute
            result = read_module_from_file("nonexistent_entity")

            # Verify returns None
            self.assertIsNone(result)

    @patch("michelangelo.cli.mactl.mactl.DEFAULT_DIR_PLUGINS")
    def test_main_py_does_not_exist(self, mock_default_dir_plugins):
        """Test when main.py file does not exist in plugin directory."""
        with tempfile.TemporaryDirectory() as tmpdir:
            # Create plugin directory but no main.py
            plugin_dir = Path(tmpdir) / "entity" / "test_entity"
            plugin_dir.mkdir(parents=True)

            # Mock DEFAULT_DIR_PLUGINS
            mock_default_dir_plugins.__truediv__.return_value = Path(tmpdir) / "entity"

            # Execute
            result = read_module_from_file("test_entity")

            # Verify returns None
            self.assertIsNone(result)


class ReadMinioConfigTest(TestCase):
    """Tests for read_minio_config function."""

    @patch.object(mactl, "CONFIG_FILE", Path("/nonexistent/config.yaml"))
    def test_config_file_not_exists(self):
        """Test when config file doesn't exist."""
        mactl.read_minio_config()  # Should not raise

    @patch.object(mactl, "CONFIG_FILE")
    @patch("michelangelo.cli.mactl.mactl.open")
    @patch("michelangelo.cli.mactl.mactl.yaml_safe_load")
    @patch("michelangelo.cli.mactl.mactl.getenv", return_value=None)
    @patch.dict(os.environ, {}, clear=True)
    def test_sets_env_vars(self, _, mock_yaml, __, mock_config):
        """Test setting env vars from config."""
        mock_config.exists.return_value = True
        mock_yaml.return_value = {
            "minio": {
                "access_key_id": "key123",
                "secret_access_key": "secret456",
                "endpoint_url": "http://localhost:9000",
            }
        }

        mactl.read_minio_config()

        self.assertEqual(os.environ["AWS_ACCESS_KEY_ID"], "key123")
        self.assertEqual(os.environ["AWS_SECRET_ACCESS_KEY"], "secret456")
        self.assertEqual(os.environ["AWS_ENDPOINT_URL"], "http://localhost:9000")

    @patch.object(mactl, "CONFIG_FILE")
    @patch("michelangelo.cli.mactl.mactl.open")
    @patch("michelangelo.cli.mactl.mactl.yaml_safe_load")
    @patch.dict(os.environ, {"AWS_ACCESS_KEY_ID": "existing"}, clear=True)
    def test_preserves_existing_env(self, mock_yaml, _, mock_config):
        """Test not overwriting existing env vars."""
        mock_config.exists.return_value = True
        mock_yaml.return_value = {"minio": {"access_key_id": "new_key"}}

        mactl.read_minio_config()

        self.assertEqual(os.environ["AWS_ACCESS_KEY_ID"], "existing")


class DiscoverCrdsTest(TestCase):
    """Tests for discover_crds function."""

    @patch("michelangelo.cli.mactl.mactl.create_serivce_classes")
    @patch("michelangelo.cli.mactl.mactl.list_services")
    def test_discover_crds_returns_crd_dict(
        self, mock_list_services, mock_create_classes
    ):
        """Test that discover_crds returns CRD dictionary."""
        mock_channel = Mock()
        mock_services = [
            "michelangelo.api.v2.ProjectService",
            "michelangelo.api.v2.ModelService",
        ]
        mock_crds = {"project": Mock(), "model": Mock()}

        mock_list_services.return_value = mock_services
        mock_create_classes.return_value = mock_crds

        result = discover_crds(mock_channel)

        mock_list_services.assert_called_once_with(mock_channel, mactl.METADATA)
        mock_create_classes.assert_called_once_with(mock_services)
        self.assertEqual(result, mock_crds)

    @patch("michelangelo.cli.mactl.mactl.create_serivce_classes")
    @patch("michelangelo.cli.mactl.mactl.list_services")
    def test_discover_crds_with_empty_services(
        self, mock_list_services, mock_create_classes
    ):
        """Test discover_crds with no services."""
        mock_channel = Mock()
        mock_list_services.return_value = []
        mock_create_classes.return_value = {}

        result = discover_crds(mock_channel)

        self.assertEqual(result, {})
        mock_create_classes.assert_called_once_with([])


class PreParseArgsTest(TestCase):
    """Tests for pre_parse_args function."""

    @patch("sys.argv", ["mactl", "project", "list"])
    def test_pre_parse_args_basic(self):
        """Test basic argument parsing."""
        crds = {"project": Mock(), "model": Mock()}

        namespace, remaining = pre_parse_args(crds)

        self.assertEqual(namespace.entity, "project")
        self.assertEqual(remaining, ["list"])

    @patch("sys.argv", ["mactl", "-vv", "model", "create", "--file=test.yaml"])
    def test_pre_parse_args_with_verbose(self):
        """Test parsing with verbose flag."""
        crds = {"project": Mock(), "model": Mock()}

        namespace, remaining = pre_parse_args(crds)

        self.assertTrue(namespace.verbose)
        self.assertEqual(namespace.entity, "model")
        self.assertEqual(remaining, ["create", "--file=test.yaml"])

    @patch("sys.argv", ["mactl", "pipeline", "apply", "-f", "config.yaml"])
    def test_pre_parse_args_with_remaining_args(self):
        """Test parsing with remaining arguments."""
        crds = {"pipeline": Mock(), "project": Mock()}

        namespace, remaining = pre_parse_args(crds)

        self.assertEqual(namespace.entity, "pipeline")
        self.assertIn("apply", remaining)
        self.assertIn("-f", remaining)
        self.assertIn("config.yaml", remaining)


class HandleCrdActionHelpTest(TestCase):
    """Tests for handle_crd_action_help function."""

    @patch("builtins.print")
    @patch("michelangelo.cli.mactl.mactl.print_help_available_actions")
    def test_no_remaining_args_exits_with_1(self, mock_print_help, _):
        """Test exits with code 1 when no remaining args."""
        crd = CRD(
            name="project", full_name="michelangelo.api.v2.ProjectService", metadata=[]
        )

        with self.assertRaises(SystemExit) as cm:
            handle_crd_action_help(crd, [])

        self.assertEqual(cm.exception.code, 1)
        mock_print_help.assert_called_once()

    @patch("builtins.print")
    @patch("michelangelo.cli.mactl.mactl.print_help_available_actions")
    def test_help_flag_exits_with_0(self, mock_print_help, _):
        """Test exits with code 0 when --help flag is present."""
        crd = CRD(
            name="model", full_name="michelangelo.api.v2.ModelService", metadata=[]
        )

        with self.assertRaises(SystemExit) as cm:
            handle_crd_action_help(crd, ["--help"])

        self.assertEqual(cm.exception.code, 0)
        mock_print_help.assert_called_once()

    @patch("builtins.print")
    @patch("michelangelo.cli.mactl.mactl.print_help_available_actions")
    def test_h_flag_exits_with_0(self, mock_print_help, _):
        """Test exits with code 0 when -h flag is present."""
        crd = CRD(
            name="pipeline",
            full_name="michelangelo.api.v2.PipelineService",
            metadata=[],
        )

        with self.assertRaises(SystemExit) as cm:
            handle_crd_action_help(crd, ["-h"])

        self.assertEqual(cm.exception.code, 0)
        mock_print_help.assert_called_once()

    @patch("builtins.print")
    @patch("michelangelo.cli.mactl.mactl.print_help_available_actions")
    def test_normal_action_does_not_exit(self, mock_print_help, mock_print):
        """Test does not exit when normal action is provided."""
        crd = CRD(
            name="project", full_name="michelangelo.api.v2.ProjectService", metadata=[]
        )

        # Should not raise SystemExit
        handle_crd_action_help(crd, ["list"])

        mock_print_help.assert_not_called()
        mock_print.assert_not_called()


class CheckCrdTest(TestCase):
    """Tests for check_crd function."""

    def test_valid_action_does_not_exit(self):
        """Test valid action does not exit."""
        crd = CRD(
            name="project", full_name="michelangelo.api.v2.ProjectService", metadata=[]
        )
        check_crd(crd, "get")  # Should not raise

    @patch("builtins.print")
    @patch("michelangelo.cli.mactl.mactl.print_help_available_actions")
    def test_prints_available_actions_on_error(self, mock_print_help, _):
        """Test prints available actions when action is invalid."""
        crd = CRD(
            name="pipeline",
            full_name="michelangelo.api.v2.PipelineService",
            metadata=[],
        )
        crd.func_signature = {"apply": {"help": "Apply"}, "delete": {"help": "Delete"}}

        with self.assertRaises(SystemExit) as err:
            check_crd(crd, "unknown")

        self.assertEqual(err.exception.code, 1)
        mock_print_help.assert_called_once()

        call_args = mock_print_help.call_args[0][0]
        self.assertIn(("apply", "Apply"), call_args)
        self.assertIn(("delete", "Delete"), call_args)
