"""Unit tests for config module."""

from unittest import TestCase
from unittest.mock import MagicMock, patch

from michelangelo.cli.mactl.config import (
    DEFAULT_CONFIG,
    _apply_env_overrides,
    _deep_merge,
    _load_rc_config,
    load_config,
    setup_minio_env,
)


class LoadRcConfigTest(TestCase):
    """Test cases for _load_rc_config function."""

    @patch("michelangelo.cli.mactl.config.Path")
    def test_load_rc_config_file_not_exists(self, mock_path):
        """Test _load_rc_config returns empty dict when file doesn't exist."""
        mock_path.home.return_value.__truediv__.return_value.exists.return_value = False
        result = _load_rc_config()
        self.assertEqual(result, {})

    @patch("michelangelo.cli.mactl.config.Path")
    @patch("michelangelo.cli.mactl.config.configparser.ConfigParser")
    def test_load_rc_config_with_mactl_section(self, mock_config_parser, mock_path):
        """Test _load_rc_config loads mactl section correctly."""
        mock_path.home.return_value.__truediv__.return_value.exists.return_value = True

        mock_config = MagicMock()
        mock_config_parser.return_value = mock_config
        mock_config.__contains__ = lambda self, key: key in ["mactl"]
        mock_config.__getitem__ = lambda self, key: {
            "address": "127.0.0.1:8080",
            "use_tls": "true",
        }

        result = _load_rc_config()

        self.assertEqual(result["address"], "127.0.0.1:8080")
        self.assertEqual(result["use_tls"], "true")

    @patch("michelangelo.cli.mactl.config.Path")
    @patch("michelangelo.cli.mactl.config.configparser.ConfigParser")
    def test_load_rc_config_with_metadata_section(self, mock_config_parser, mock_path):
        """Test _load_rc_config loads metadata section correctly."""
        mock_path.home.return_value.__truediv__.return_value.exists.return_value = True

        mock_config = MagicMock()
        mock_config_parser.return_value = mock_config
        mock_config.__contains__ = lambda self, key: key in ["metadata"]
        mock_config.__getitem__ = lambda self, key: {
            "rpc-caller": "test",
            "rpc-service": "test-service",
        }

        result = _load_rc_config()

        self.assertIn("metadata", result)
        self.assertEqual(result["metadata"]["rpc-caller"], "test")

    @patch("michelangelo.cli.mactl.config.Path")
    @patch("michelangelo.cli.mactl.config.configparser.ConfigParser")
    def test_load_rc_config_read_exception(self, mock_config_parser, mock_path):
        """Test _load_rc_config returns empty dict on exception."""
        mock_path.home.return_value.__truediv__.return_value.exists.return_value = True

        mock_config = MagicMock()
        mock_config_parser.return_value = mock_config
        mock_config.read.side_effect = Exception("Test exception")

        result = _load_rc_config()

        self.assertEqual(result, {})


class DeepMergeTest(TestCase):
    """Test cases for _deep_merge function."""

    def test_deep_merge_simple(self):
        """Test deep merge with simple dicts."""
        base = {"a": 1, "b": 2}
        override = {"b": 3, "c": 4}
        result = _deep_merge(base, override)
        self.assertEqual(result, {"a": 1, "b": 3, "c": 4})

    def test_deep_merge_nested(self):
        """Test deep merge with nested dicts."""
        base = {"address": "old", "use_tls": False, "minio": {}}
        override = {"address": "new"}
        result = _deep_merge(base, override)
        self.assertEqual(result["address"], "new")
        self.assertEqual(result["use_tls"], False)

    def test_deep_merge_does_not_modify_original(self):
        """Test deep merge doesn't modify original dicts."""
        base = {"a": 1}
        override = {"b": 2}
        result = _deep_merge(base, override)
        self.assertEqual(base, {"a": 1})
        self.assertEqual(override, {"b": 2})
        self.assertEqual(result, {"a": 1, "b": 2})


class ApplyEnvOverridesTest(TestCase):
    """Test cases for _apply_env_overrides function."""

    @patch("michelangelo.cli.mactl.config.getenv")
    def test_apply_env_overrides_mactl_address(self, mock_getenv):
        """Test env override for MACTL_ADDRESS."""

        def getenv_side_effect(key, default=None):
            if key == "MACTL_ADDRESS":
                return "env-address:9999"
            return None

        mock_getenv.side_effect = getenv_side_effect

        config = {"address": "default-address", "use_tls": False}
        result = _apply_env_overrides(config)

        self.assertEqual(result["address"], "env-address:9999")

    @patch("michelangelo.cli.mactl.config.getenv")
    def test_apply_env_overrides_mactl_use_tls(self, mock_getenv):
        """Test env override for MACTL_USE_TLS."""

        def getenv_side_effect(key, default=None):
            if key == "MACTL_USE_TLS":
                return "true"
            return None

        mock_getenv.side_effect = getenv_side_effect

        config = {"address": "default", "use_tls": False}
        result = _apply_env_overrides(config)

        self.assertTrue(result["use_tls"])

    @patch("michelangelo.cli.mactl.config.getenv")
    def test_apply_env_overrides_aws_credentials(self, mock_getenv):
        """Test env override for AWS_* variables."""

        def getenv_side_effect(key, default=None):
            env_map = {
                "AWS_ACCESS_KEY_ID": "env-key",
                "AWS_SECRET_ACCESS_KEY": "env-secret",
                "AWS_ENDPOINT_URL": "http://env-endpoint",
            }
            return env_map.get(key)

        mock_getenv.side_effect = getenv_side_effect

        config = {
            "address": "default",
            "use_tls": False,
            "minio": {
                "access_key_id": "default-key",
                "secret_access_key": "default-secret",
                "endpoint_url": "http://default",
            },
        }
        result = _apply_env_overrides(config)

        self.assertEqual(result["minio"]["access_key_id"], "env-key")
        self.assertEqual(result["minio"]["secret_access_key"], "env-secret")
        self.assertEqual(result["minio"]["endpoint_url"], "http://env-endpoint")

    @patch("michelangelo.cli.mactl.config.getenv")
    def test_apply_env_overrides_no_env_vars(self, mock_getenv):
        """Test no changes when no env vars set."""
        mock_getenv.return_value = None

        config = {
            "address": "default",
            "use_tls": False,
            "minio": {"access_key_id": "default"},
        }
        result = _apply_env_overrides(config)

        self.assertEqual(result, config)


class LoadConfigTest(TestCase):
    """Test cases for load_config function."""

    @patch("michelangelo.cli.mactl.config._apply_env_overrides")
    @patch("michelangelo.cli.mactl.config._load_rc_config")
    def test_load_config_default_only(self, mock_load_rc, mock_apply_env):
        """Test load_config with defaults only."""
        mock_load_rc.return_value = {}
        mock_apply_env.side_effect = lambda x: x

        result = load_config()

        self.assertEqual(result["address"], "127.0.0.1:14566")
        self.assertFalse(result["use_tls"])

    @patch("michelangelo.cli.mactl.config._apply_env_overrides")
    @patch("michelangelo.cli.mactl.config._load_rc_config")
    def test_load_config_with_rc(self, mock_load_rc, mock_apply_env):
        """Test load_config merges RC config."""
        mock_load_rc.return_value = {"address": "rc-address:8888"}
        mock_apply_env.side_effect = lambda x: x

        result = load_config()

        self.assertEqual(result["address"], "rc-address:8888")
        self.assertFalse(result["use_tls"])

    @patch("michelangelo.cli.mactl.config._apply_env_overrides")
    @patch("michelangelo.cli.mactl.config._load_rc_config")
    def test_load_config_with_env(self, mock_load_rc, mock_apply_env):
        """Test load_config applies env overrides."""
        mock_load_rc.return_value = {}

        def apply_env_side_effect(config):
            config["address"] = "env-address:9999"
            return config

        mock_apply_env.side_effect = apply_env_side_effect

        result = load_config()

        self.assertEqual(result["address"], "env-address:9999")

    @patch("michelangelo.cli.mactl.config._apply_env_overrides")
    @patch("michelangelo.cli.mactl.config._load_rc_config")
    def test_load_config_priority(self, mock_load_rc, mock_apply_env):
        """Test load_config respects priority: env > rc > default."""
        # RC config overrides default
        mock_load_rc.return_value = {"address": "rc-address"}

        # Env overrides RC
        def apply_env_side_effect(config):
            config["address"] = "env-address"
            return config

        mock_apply_env.side_effect = apply_env_side_effect

        result = load_config()

        self.assertEqual(result["address"], "env-address")


class SetupMinioEnvTest(TestCase):
    """Test cases for setup_minio_env function."""

    @patch("michelangelo.cli.mactl.config.environ", {})
    @patch("michelangelo.cli.mactl.config.load_config")
    def test_setup_minio_env_sets_vars_from_config(self, mock_load_config):
        """Test setup_minio_env sets AWS env vars from config."""
        mock_load_config.return_value = DEFAULT_CONFIG

        with patch("michelangelo.cli.mactl.config.environ", {}) as mock_environ:
            setup_minio_env()

            self.assertEqual(
                mock_environ["AWS_ACCESS_KEY_ID"],
                DEFAULT_CONFIG["minio"]["access_key_id"],
            )
            self.assertEqual(
                mock_environ["AWS_SECRET_ACCESS_KEY"],
                DEFAULT_CONFIG["minio"]["secret_access_key"],
            )
            self.assertEqual(
                mock_environ["AWS_ENDPOINT_URL"],
                DEFAULT_CONFIG["minio"]["endpoint_url"],
            )

    @patch("michelangelo.cli.mactl.config.environ", {})
    @patch("michelangelo.cli.mactl.config.load_config")
    def test_setup_minio_env_uses_config_with_env_overrides(self, mock_load_config):
        """Test setup_minio_env uses config that already has env overrides."""
        # Simulate config that already has AWS_* env vars applied
        config_with_overrides = {
            "address": DEFAULT_CONFIG["address"],
            "use_tls": DEFAULT_CONFIG["use_tls"],
            "metadata": DEFAULT_CONFIG["metadata"],
            "minio": {
                "access_key_id": "env-override-key",
                "secret_access_key": "env-override-secret",
                "endpoint_url": "http://env-override",
            },
        }
        mock_load_config.return_value = config_with_overrides

        with patch("michelangelo.cli.mactl.config.environ", {}) as mock_environ:
            setup_minio_env()

            # Should set env vars from config (which already has env overrides)
            self.assertEqual(mock_environ["AWS_ACCESS_KEY_ID"], "env-override-key")
            self.assertEqual(
                mock_environ["AWS_SECRET_ACCESS_KEY"], "env-override-secret"
            )
            self.assertEqual(mock_environ["AWS_ENDPOINT_URL"], "http://env-override")


class DefaultConstantsTest(TestCase):
    """Test cases for default constants."""

    def test_default_config_structure(self):
        """Test DEFAULT_CONFIG has correct structure."""
        self.assertIn("address", DEFAULT_CONFIG)
        self.assertIn("use_tls", DEFAULT_CONFIG)
        self.assertIn("metadata", DEFAULT_CONFIG)
        self.assertIn("minio", DEFAULT_CONFIG)

    def test_default_config_values(self):
        """Test DEFAULT_CONFIG default values."""
        self.assertEqual(DEFAULT_CONFIG["address"], "127.0.0.1:14566")
        self.assertFalse(DEFAULT_CONFIG["use_tls"])
        self.assertIn("metadata", DEFAULT_CONFIG)
        self.assertEqual(DEFAULT_CONFIG["metadata"]["rpc-caller"], "grpcurl")

    def test_default_minio_config(self):
        """Test DEFAULT_CONFIG minio section."""
        minio = DEFAULT_CONFIG["minio"]
        self.assertEqual(minio["access_key_id"], "minioadmin")
        self.assertEqual(minio["secret_access_key"], "minioadmin")
        self.assertEqual(minio["endpoint_url"], "http://localhost:9091")
