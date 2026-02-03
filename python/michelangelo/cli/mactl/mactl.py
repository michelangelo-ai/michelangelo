"""MaCTL - Michelangelo Command Line Tool.

A command line interface to interact with the Michelangelo API server via gRPC.
"""

import configparser
import logging
import re
import sys
from argparse import ArgumentParser, Namespace
from collections import defaultdict
from importlib.util import module_from_spec, spec_from_file_location
from logging import WARNING, basicConfig, getLogger
from os import environ, getenv
from pathlib import Path
from typing import Union

from grpc import (
    Channel,
    insecure_channel,
    secure_channel,
    ssl_channel_credentials,
)
from yaml import safe_load as yaml_safe_load

from michelangelo.cli.mactl.crd import CRD, yaml_to_dict
from michelangelo.cli.mactl.grpc_tools import list_services


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


### For Uber-internal server ###
# $ cerberus -r michelangelo-apiserver-staging
ADDRESS = "127.0.0.1:5435"
METADATA = [
    ("rpc-caller", "grpcurl"),
    # ("rpc-service", "michelangelo-apiserver"),
    ("rpc-service", "michelangelo-apiserver-staging"),
    ("rpc-encoding", "proto"),
]

### For OSS server
# $ (Run oss server)
ADDRESS = "127.0.0.1:14566"
# TODO: Change metadata as dict
METADATA = [
    ("rpc-caller", "grpcurl"),
    ("rpc-service", "ma-apiserver"),
    ("rpc-encoding", "proto"),
]

_rc_config = _load_rc_config()

# Apply configuration priority: env vars (highest) > RC file > defaults (lowest)
# Allow overriding the API server address via environment variable
# This enables pointing the CLI to a k8s NodePort (e.g., 127.0.0.1:30009)
ADDRESS = getenv("MACTL_ADDRESS", _rc_config.get("address", ADDRESS))
# Allow overriding TLS usage via environment variable
# Set to "true" to force TLS, "false" to force insecure,
# or leave unset for auto-detection
USE_TLS: bool = getenv("MACTL_USE_TLS", _rc_config.get("use_tls", "false")).lower() in (
    "true",
    "1",
    "yes",
    "y",
)
if "metadata" in _rc_config:
    METADATA = list(_rc_config["metadata"].items())

METADATA_STUB = [*METADATA, ("ttl", "600")]

basicConfig(
    level=getattr(logging, getenv("LOG_LEVEL", "WARNING").upper(), WARNING),
    format="%(asctime)s | %(levelname)-8s | %(name)-40s | %(message)s",
)
_LOG = getLogger(__name__)

PWD = Path(__file__).parent.resolve()
DEFAULT_DIR_PLUGINS = PWD / "plugins"
CONFIG_FILE = PWD / "config.yaml"

_LOG.info(f"Config: ADDRESS={ADDRESS}, USE_TLS={USE_TLS}, METADATA={METADATA}")


def camel_to_snake(name: str) -> str:
    """Converts CamelCase to snake_case (e.g., 'DevRun' -> 'dev_run')."""
    res = re.sub("(.)([A-Z][a-z]+)", r"\1_\2", name)
    return re.sub("([a-z0-9])([A-Z])", r"\1_\2", res).lower()


def kebab_to_snake(name: str) -> str:
    """Converts kebab-case to snake_case (e.g., 'dev-run' -> 'dev_run')."""
    return name.replace("-", "_")


def read_module_from_file(crd_name: str) -> Union[object, None]:
    """Read and load a plugin module from a given file path."""
    _LOG.info("Check Plugin directory: %r", DEFAULT_DIR_PLUGINS)
    plugin_dir = DEFAULT_DIR_PLUGINS / "entity" / crd_name
    if not plugin_dir.exists():
        _LOG.info("Plugin directory does not exist: %r", plugin_dir)
        return

    plugin_main = plugin_dir / "main.py"
    if not plugin_main.exists():
        _LOG.info("Plugin main file does not exist: %r", plugin_main)
        return

    # Add plugin_dir to sys.path to allow package-style imports
    plugin_parent_path = plugin_dir.parents[1].resolve()
    _LOG.debug("Plugin parent path: %r", plugin_parent_path)
    _LOG.debug("Current system path: %r", sys.path)
    if str(plugin_parent_path) not in sys.path:
        sys.path.insert(0, str(plugin_parent_path))

    spec = spec_from_file_location(
        f"plugin_{crd_name}_main", str(plugin_main.resolve())
    )
    if spec is None:
        _LOG.error("Failed to load plugin spec for %r", plugin_main)
        return
    plugin_module = module_from_spec(spec)
    if plugin_module is None:
        _LOG.error("Failed to create plugin module for %r", spec)
        return
    spec.loader.exec_module(plugin_module)  # type: ignore[attr-defined]
    _LOG.info("Loaded plugin module: %r", plugin_module)
    return plugin_module


def read_plugins(crd: CRD, channel: Channel) -> None:
    """Read and apply plugins for a given crd."""
    _LOG.info("Read plugins for crd: %r", crd)
    plugin_module = read_module_from_file(crd.name)
    if plugin_module is None:
        return

    plugin_module.apply_plugins(crd, channel)
    _LOG.info("Apply plugin done for %r entity", crd.name)
    return


def read_plugin_command(
    crd: CRD, user_command_action: str, crds: dict[str, CRD], channel: Channel
) -> None:
    """Read and apply plugins for a given crd."""
    _LOG.info("Read plugins for crd: %r", crd)
    plugin_module = read_module_from_file(crd.name)
    if plugin_module is None:
        return

    if hasattr(plugin_module, "apply_plugin_command"):
        plugin_module.apply_plugin_command(crd, user_command_action, crds, channel)
        _LOG.info("Apply plugin done for %r entity", crd.name)
        return

    _LOG.info(
        "Plugin module %r does not have `apply_plugin_command` function",
        plugin_module,
    )


def get_crd_name_from_yaml(yaml_path_string: str) -> str:
    """Reads a YAML file and returns its content as a dictionary."""
    _LOG.info("Start to Read YAML file: %r", yaml_path_string)
    yaml_dict = yaml_to_dict(yaml_path_string)

    assert "apiVersion" in yaml_dict, "YAML must contain 'apiVersion' key"
    assert "kind" in yaml_dict, "YAML must contain 'kind' key"

    api_version = yaml_dict["apiVersion"]
    kind = yaml_dict["kind"]

    _LOG.info("API Version: %s, Kind: %s", api_version, kind)
    assert isinstance(kind, str), "kind must be a string"
    return kind


def create_serivce_classes(services: list[str]) -> dict[str, CRD]:
    """Create service classes from a list of service names."""
    res = {}
    # TODO: we don't have to create all CRD instances for all services
    for service in services:
        if service.endswith("Service") and not service.endswith("ExtService"):
            name = camel_to_snake(re.sub(r"Service$", "", service.split(".")[-1]))
            res[name] = CRD(name=name, full_name=service, metadata=METADATA)
    _LOG.info("Created %d CRD instances: %r", len(res), res)
    return res


def parse_args() -> tuple[list[str], dict[str, list[str]]]:
    """Parse command line arguments.

    Returns a tuple of (args, kwargs).
    """
    args = []
    kwargs = defaultdict(list)
    for arg in sys.argv[1:]:
        if "=" in arg:
            key, value = arg.split("=", 1)
            key = key.lstrip("-")
            kwargs[key].append(value)
        elif arg.startswith("--"):
            # Handle boolean flags like --yes
            key = arg.lstrip("-")
            kwargs[key].append(True)
        else:
            args.append(arg)
    _LOG.info("Parsed arguments: %r  /  %r", args, kwargs)
    return args, kwargs


def handle_args() -> tuple[str, str, dict[str, list[str]]]:
    """(Legacy to be deprecated) Handle command line arguments."""
    args, kwargs = parse_args()

    # New syntax: mactl <resource> <action> [options]
    user_command_crd = args[0]
    user_command_action = args[1]

    # For file-based actions, validate file parameter exists
    # (preserving original validation)
    if user_command_action in ["apply", "create", "dev-run"]:
        assert len(kwargs["file"]) == 1, f"exactly one yaml file is required! {kwargs}"

    user_command_action = kebab_to_snake(user_command_action)

    _LOG.info(
        "User command CRD: %r / User command action: %r",
        user_command_crd,
        user_command_action,
    )
    return user_command_crd, user_command_action, kwargs


def print_help_available_actions(actions: list[tuple[str, str]]) -> None:
    """Print help message of available action command."""
    if not actions:
        print("\nNo available actions.")
        return

    action_names = [action[0] for action in actions]
    max_action_length = max(len(action) for action in action_names)
    help_position = min(max_action_length + 2, 24)
    action_width = help_position - 2  # subtract indent

    print("\nAvailable actions:")
    for action, help_text in actions:
        if len(action) <= action_width:
            # Short action: same line with padding
            print(f"  {action:{action_width}}  {help_text}")
        else:
            # Long action: next line for help
            print(f"  {action}")
            print(f"  {'':{help_position}}{help_text}")


def read_minio_config():
    """Read configuration for Minio environment variables."""
    if not CONFIG_FILE.exists():
        return

    with open(CONFIG_FILE) as f:
        config = yaml_safe_load(f) or {}
    minio_config = config.get("minio", {})
    if not getenv("AWS_ACCESS_KEY_ID"):
        environ["AWS_ACCESS_KEY_ID"] = minio_config.get("access_key_id", "")
    if not getenv("AWS_SECRET_ACCESS_KEY"):
        environ["AWS_SECRET_ACCESS_KEY"] = minio_config.get("secret_access_key", "")
    if not getenv("AWS_ENDPOINT_URL"):
        environ["AWS_ENDPOINT_URL"] = minio_config.get("endpoint_url", "")


def discover_crds(channel: Channel) -> dict[str, CRD]:
    """Discover CRDs from the API server."""
    services = list_services(channel, METADATA)
    _LOG.info("Discovered %d services: %r", len(services), services)
    return create_serivce_classes(services)


def pre_parse_args(crds: dict[str, CRD]) -> tuple[Namespace, list[str]]:
    """Pre-parse to get namespace, entity, and remaining info."""
    base_parser = ArgumentParser(description="MaCTL - Michelangelo CLI", add_help=False)
    base_parser.add_argument(
        "-vv",
        "--verbose",
        action="store_true",
        help="Increase verbosity level",
    )
    entity_subparsers = base_parser.add_subparsers(dest="entity", required=True)

    for crd_name in crds:
        entity_subparsers.add_parser(crd_name, add_help=False)

    namespace, remaining = base_parser.parse_known_args()
    _LOG.debug(
        "Parsed arguments -- namespace: %r / remaining: %r", namespace, remaining
    )
    return namespace, remaining


def handle_crd_action_help(crd: CRD, remaining: list[str]) -> None:
    """Handle CRD-level help command."""
    # TODO: this will be generated by CRD automatically later
    if len(remaining) < 1:
        print(f"Usage: ma {crd.name} <action> [options]")
        print_help_available_actions(
            [
                (k, v.get("help", ""))
                for k, v in crd.func_signature.items()
            ]
        )
        print(f"\nRun 'ma {crd.name} --help' for more information")
        sys.exit(1)

    if len(remaining) >= 1 and remaining[0] in ["--help", "-h"]:
        print(f"Usage: ma {crd.name} <action> [options]")
        print_help_available_actions(
            [
                (k, v.get("help", ""))
                for k, v in crd.func_signature.items()
            ]
        )
        print(f"\nFor action-specific help, use: ma {crd.name} <action> --help")
        sys.exit(0)


def main(channel: Channel):
    """Main function for mactl."""
    _LOG.debug("Starting mactl...")

    # Load config and set environment variables
    read_minio_config()

    # Phase 1: Discover CRDs and create resource subcommands
    crds = discover_crds(channel)

    # Phase 2: Pre-parse to get selected resource
    namespace, remaining = pre_parse_args(crds)
    user_command_crd = str(namespace.entity)

    # Load plugins for target CRD
    read_plugins(
        crds[user_command_crd],
        channel,
    )

    # Handle CRD-level help (e.g., "ma project --help" or "ma project -h")
    handle_crd_action_help(crds[user_command_crd], remaining)

    # Phase 3: Generate method + configure argparse
    user_command_action = kebab_to_snake(remaining[0])
    action_parser = ArgumentParser(
        prog=f"mactl {user_command_crd} {user_command_action}"
    )

    # TODO: this will be handled by CRD automatically later with argparse
    if user_command_action not in crds[user_command_crd].func_signature:
        _LOG.debug(
            "Unknown action `%r`: %r",
            user_command_action,
            crds[user_command_crd].func_signature,
        )
        print(f"Unknown action: `{user_command_action}`")
        print_help_available_actions(
            [
                (k, v.get("help", ""))
                for k, v in crds[user_command_crd].func_signature.items()
            ]
        )
        print(f"\nRun 'ma {user_command_crd} --help' for more information")
        sys.exit(1)

    # Load target function plugin
    read_plugin_command(
        crds[user_command_crd],
        user_command_action,
        crds,
        channel,
    )

    _LOG.debug(
        "Generating action `%r` for CRD `%r`: %r",
        user_command_action,
        crds[user_command_crd],
        dir(crds[user_command_crd]),
    )
    func_generator = getattr(crds[user_command_crd], f"generate_{user_command_action}")
    func_generator(channel, action_parser)

    # Phase 4: Parse remaining arguments
    args = action_parser.parse_args(remaining[1:])

    # Phase 5: Execute
    func_action = getattr(crds[user_command_crd], user_command_action)
    _LOG.debug("target action function is ready: %r", func_action)
    func_action(**vars(args))

    # Convert to JSON and pretty print
    # temporary disable json converting due to issue:
    #   some missing proto message info causing error.
    """
    json_output = MessageToJson(
        result,
        always_print_fields_with_no_presence=True,
        preserving_proto_field_name=True
    )
    print(json_output)
    """


def run():
    """Entry point for mactl."""
    if USE_TLS:
        _LOG.info(
            "Using TLS (forced via MACTL_USE_TLS=%r) to connect to %r",
            USE_TLS,
            ADDRESS,
        )
        # Use secure TLS connection
        with secure_channel(ADDRESS, ssl_channel_credentials()) as channel:
            return main(channel)

    _LOG.info(
        "Using TLS (forced via MACTL_USE_TLS=%r) to connect to %r",
        USE_TLS,
        ADDRESS,
    )
    # Use secure TLS connection
    # Use insecure connection for local development
    with insecure_channel(ADDRESS) as channel:
        return main(channel)


if __name__ == "__main__":
    run()
