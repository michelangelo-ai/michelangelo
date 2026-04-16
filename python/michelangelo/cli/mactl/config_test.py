"""Unit tests for config module."""

from pathlib import Path
from unittest import TestCase
from unittest.mock import mock_open, patch

from michelangelo.cli.mactl.config import (
    DEFAULT_CONFIG,
    PACKAGE_CONFIG_FILE,
    USER_CONFIG_FILE,
    _apply_env_overrides,
    _deep_merge,
    _load_toml_file,
    load_config,
    setup_minio_env,
)


class LoadTomlFileTest(TestCase):
    """Test cases for _load_toml_file function."""

    def test_returns_empty_dict_when_file_missing(self):
        """Test _load_toml_file returns empty dict when file doesn't exist."""
        result = _load_toml_file(Path("/nonexistent/path/config.toml"))
        self.assertEqual(result, {})

    @patch("builtins.open", mock_open())
    @patch("michelangelo.cli.mactl.config.tomllib.load")
    def test_loads_toml_when_file_exists(self, mock_toml_load):
        """Test _load_toml_file parses TOML correctly."""
        expected = {"address": "127.0.0.1:8080", "use_tls": True}
        mock_toml_load.return_value = expected

        with patch.object(Path, "exists", return_value=True):
            result = _load_toml_file(Path("/fake/config.toml"))

        self.assertEqual(result, expected)

    def test_returns_empty_dict_on_read_exception(self):
        """Test _load_toml_file returns empty dict on exception."""
        with (
            patch.object(Path, "exists", return_value=True),
            patch("builtins.open", side_effect=OSError("permission denied")),
        ):
            result = _load_toml_file(Path("/fake/config.toml"))

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
    @patch("michelangelo.cli.mactl.config._load_toml_file")
    def test_load_config_default_only(self, mock_load_toml, mock_apply_env):
        """Test load_config with defaults only (both config files missing)."""
        mock_load_toml.return_value = {}
        mock_apply_env.side_effect = lambda x: x

        result = load_config()

        self.assertEqual(result["address"], "127.0.0.1:15566")
        self.assertFalse(result["use_tls"])

    @patch("michelangelo.cli.mactl.config._apply_env_overrides")
    @patch("michelangelo.cli.mactl.config._load_toml_file")
    def test_package_config_overrides_defaults(self, mock_load_toml, mock_apply_env):
        """Test config.toml (package) values override built-in defaults."""
        mock_load_toml.side_effect = [
            {"address": "pkg-address:8888"},  # PACKAGE_CONFIG_FILE
            {},  # USER_CONFIG_FILE
        ]
        mock_apply_env.side_effect = lambda x: x

        result = load_config()

        self.assertEqual(result["address"], "pkg-address:8888")
        self.assertFalse(result["use_tls"])

    @patch("michelangelo.cli.mactl.config._apply_env_overrides")
    @patch("michelangelo.cli.mactl.config._load_toml_file")
    def test_user_config_overrides_package_config(self, mock_load_toml, mock_apply_env):
        """Test user_config.toml values override config.toml values."""
        mock_load_toml.side_effect = [
            {"address": "pkg-address:8888"},  # PACKAGE_CONFIG_FILE
            {"address": "user-address:9999"},  # USER_CONFIG_FILE
        ]
        mock_apply_env.side_effect = lambda x: x

        result = load_config()

        self.assertEqual(result["address"], "user-address:9999")

    @patch("michelangelo.cli.mactl.config._apply_env_overrides")
    @patch("michelangelo.cli.mactl.config._load_toml_file")
    def test_env_overrides_user_config(self, mock_load_toml, mock_apply_env):
        """Test env vars override user_config.toml."""
        mock_load_toml.side_effect = [
            {},
            {"address": "user-address:9999"},
        ]

        def apply_env(config):
            config["address"] = "env-address:1111"
            return config

        mock_apply_env.side_effect = apply_env

        result = load_config()

        self.assertEqual(result["address"], "env-address:1111")

    @patch("michelangelo.cli.mactl.config._apply_env_overrides")
    @patch("michelangelo.cli.mactl.config._load_toml_file")
    def test_load_config_calls_files_in_order(self, mock_load_toml, mock_apply_env):
        """Test load_config loads package config then user config."""
        mock_load_toml.return_value = {}
        mock_apply_env.side_effect = lambda x: x

        load_config()

        calls = mock_load_toml.call_args_list
        self.assertEqual(len(calls), 2)
        self.assertEqual(calls[0][0][0], PACKAGE_CONFIG_FILE)
        self.assertEqual(calls[1][0][0], USER_CONFIG_FILE)


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
        self.assertIn("plugin", DEFAULT_CONFIG)

    def test_default_config_values(self):
        """Test DEFAULT_CONFIG default values."""
        self.assertEqual(DEFAULT_CONFIG["address"], "127.0.0.1:15566")
        self.assertFalse(DEFAULT_CONFIG["use_tls"])
        self.assertEqual(DEFAULT_CONFIG["metadata"]["rpc-caller"], "grpcurl")

    def test_default_minio_config(self):
        """Test DEFAULT_CONFIG minio section."""
        minio = DEFAULT_CONFIG["minio"]
        self.assertEqual(minio["access_key_id"], "ma-minio")
        self.assertEqual(minio["secret_access_key"], "g0Sfr9hRcsfRXuBS")
        self.assertEqual(minio["endpoint_url"], "http://localhost:9091")

    def test_config_file_paths(self):
        """Test config file paths are in ~/.ma/."""
        self.assertEqual(PACKAGE_CONFIG_FILE.name, "config.toml")
        self.assertEqual(USER_CONFIG_FILE.name, "user_config.toml")
        self.assertEqual(PACKAGE_CONFIG_FILE.parent, USER_CONFIG_FILE.parent)
