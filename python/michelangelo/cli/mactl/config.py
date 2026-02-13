"""Configuration management for mactl."""

import configparser
from copy import deepcopy
from logging import getLogger
from os import environ, getenv
from pathlib import Path

_LOG = getLogger(__name__)

DEFAULT_CONFIG = {
    "address": "127.0.0.1:14566",
    "use_tls": False,
    "metadata": {
        "rpc-caller": "grpcurl",
        "rpc-service": "ma-apiserver",
        "rpc-encoding": "proto",
    },
    "minio": {
        "access_key_id": "minioadmin",
        "secret_access_key": "minioadmin",
        "endpoint_url": "http://localhost:9091",
    },
    "plugins": [],
}


def _load_rc_config() -> dict:
    """Load configuration from ~/.mactlrc file."""
    config = configparser.ConfigParser()
    rc_file = Path.home() / ".mactlrc"

    if not rc_file.exists():
        return {}

    try:
        config.read(rc_file)
    except Exception:
        return {}

    rc_config = {}

    if "mactl" in config:
        if "address" in config["mactl"]:
            rc_config["address"] = config["mactl"]["address"]
        if "use_tls" in config["mactl"]:
            rc_config["use_tls"] = config["mactl"]["use_tls"]

    if "metadata" in config:
        rc_config["metadata"] = dict(config["metadata"])

    return rc_config


def _deep_merge(base: dict, override: dict) -> dict:
    """Merge override dict into base dict (supports 2 levels)."""
    result = deepcopy(base)
    for key, value in override.items():
        if isinstance(value, dict) and key in result and isinstance(result[key], dict):
            result[key] = {**result[key], **value}
        else:
            result[key] = value
    return result


def _apply_env_overrides(config: dict) -> dict:
    """Apply environment variable overrides to config."""
    result = deepcopy(config)

    # Override settings from MACTL_* env vars
    if getenv("MACTL_ADDRESS"):
        result["address"] = getenv("MACTL_ADDRESS")

    if getenv("MACTL_USE_TLS"):
        use_tls_str = getenv("MACTL_USE_TLS")
        result["use_tls"] = use_tls_str.lower() in ("true", "1", "yes", "y")

    # Override minio settings from AWS_* env vars
    if getenv("AWS_ACCESS_KEY_ID"):
        result["minio"]["access_key_id"] = getenv("AWS_ACCESS_KEY_ID")

    if getenv("AWS_SECRET_ACCESS_KEY"):
        result["minio"]["secret_access_key"] = getenv("AWS_SECRET_ACCESS_KEY")

    if getenv("AWS_ENDPOINT_URL"):
        result["minio"]["endpoint_url"] = getenv("AWS_ENDPOINT_URL")

    return result


def load_config() -> dict:
    """Load complete configuration as dictionary with layered merging.

    Priority (highest to lowest):
    1. Environment variables (MACTL_ADDRESS, MACTL_USE_TLS)
    2. RC file (~/.mactlrc)
    3. Default configuration

    Returns:
        dict: Complete configuration dictionary
    """
    # Start with defaults
    config = deepcopy(DEFAULT_CONFIG)

    # Layer 1: RC file
    rc_config = _load_rc_config()
    if rc_config:
        config = _deep_merge(config, rc_config)

    # Layer 2: Environment variables
    config = _apply_env_overrides(config)

    return config


def setup_minio_env() -> None:
    """Setup Minio environment variables from config.

    Sets AWS_* environment variables for boto3/AWS SDK libraries to use.
    Config priority is already applied in load_config():
    env vars > RC file > defaults.
    """
    config = load_config()
    minio_config = config.get("minio", {})

    # Set AWS env vars from config (for boto3/AWS libraries)
    # Note: If these were already set, they're already in the config dict
    environ["AWS_ACCESS_KEY_ID"] = minio_config.get("access_key_id", "")
    environ["AWS_SECRET_ACCESS_KEY"] = minio_config.get("secret_access_key", "")
    environ["AWS_ENDPOINT_URL"] = minio_config.get("endpoint_url", "")
