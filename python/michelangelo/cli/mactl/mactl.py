"""MaCTL - Michelangelo Command Line Tool.

A command line interface to interact with the Michelangelo API server via gRPC.
"""

import importlib
import logging
import re
import sys
from argparse import ArgumentParser, Namespace
from collections import defaultdict
from importlib.util import module_from_spec, spec_from_file_location
from logging import WARNING, basicConfig, getLogger
from os import getenv
from pathlib import Path
from typing import Union

from grpc import (
    Channel,
    insecure_channel,
    secure_channel,
    ssl_channel_credentials,
)

from michelangelo.cli.mactl.config import load_config, setup_minio_env
from michelangelo.cli.mactl.crd import CRD, yaml_to_dict
from michelangelo.cli.mactl.grpc_tools import list_services

# Load configuration
# Priority: env vars (highest) > RC file > defaults (lowest)
_CONFIG = load_config()

ADDRESS = _CONFIG["address"]
USE_TLS: bool = _CONFIG["use_tls"]
METADATA = list(_CONFIG["metadata"].items())

METADATA_STUB = [*METADATA, ("ttl", "600")]

basicConfig(
    level=getattr(logging, getenv("LOG_LEVEL", "WARNING").upper(), WARNING),
    format="%(asctime)s | %(levelname)-8s | %(name)-40s | %(message)s",
)
_LOG = getLogger(__name__)

PWD = Path(__file__).parent.resolve()
DEFAULT_DIR_PLUGINS = PWD / "plugins"

_LOG.info(f"Config: ADDRESS={ADDRESS}, USE_TLS={USE_TLS}, METADATA={METADATA}")


def camel_to_snake(name: str) -> str:
    """Converts CamelCase to snake_case (e.g., 'DevRun' -> 'dev_run')."""
    res = re.sub("(.)([A-Z][a-z]+)", r"\1_\2", name)
    return re.sub("([a-z0-9])([A-Z])", r"\1_\2", res).lower()


def kebab_to_snake(name: str) -> str:
    """Converts kebab-case to snake_case (e.g., 'dev-run' -> 'dev_run')."""
    return name.replace("-", "_")


def read_plugin_modules(crd_name: str, plugin_dirs: list[str]) -> list[object]:
    """Read and load plugin modules from given directories.

    Args:
        crd_name (str): The name of the CRD.
        plugin_dirs (list[str]): List of directories to search for plugins.
            The later directories have higher priority.
    """
    _LOG.info("Read plugin modules from directories: %r", plugin_dirs)
    plugin_modules = []
    for i, plugin_dir in enumerate(plugin_dirs):
        plugin_module = read_module_from_file(crd_name, Path(plugin_dir), i)
        if plugin_module is not None:
            plugin_modules.append(plugin_module)
    _LOG.info("Total %d plugin modules are loaded.", len(plugin_modules))
    return plugin_modules


def read_module_from_file(
    crd_name: str, plugin_dir: Path, num: int = 0
) -> Union[object, None]:
    """Read and load a plugin module from a given file path."""
    _LOG.info("Check Plugin directory: %r", plugin_dir)
    if not plugin_dir.exists() or not plugin_dir.is_dir():
        _LOG.warning(
            "Plugin base directory does not exist (or not a directory): %r", plugin_dir
        )
        return

    plugin_dir = plugin_dir / "entity" / crd_name
    if not plugin_dir.exists() or not plugin_dir.is_dir():
        _LOG.info(
            "Plugin directory does not exist (or not a directory): %r", plugin_dir
        )
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
        f"plugin_{crd_name}_main_{num}", str(plugin_main.resolve())
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
    plugin_modules = read_plugin_modules(
        crd.name, [str(DEFAULT_DIR_PLUGINS), *_CONFIG["plugin"]["dirs"]]
    )

    for i, plugin in enumerate(plugin_modules):
        _LOG.info("Applying plugin module #%d: %r", i, plugin)
        if hasattr(plugin, "apply_plugins"):
            plugin.apply_plugins(crd, channel)
        else:
            _LOG.debug("`apply_plugins` function not found in plugin module %r", plugin)
    _LOG.info("Apply plugin done for %r entity", crd.name)
    return


def read_plugin_command(
    crd: CRD, user_command_action: str, crds: dict[str, CRD], channel: Channel
) -> None:
    """Read and apply plugins for a given crd."""
    _LOG.info("Read plugins for crd: %r", crd)
    plugin_modules = read_plugin_modules(
        crd.name, [str(DEFAULT_DIR_PLUGINS), *_CONFIG["plugin"]["dirs"]]
    )

    for i, plugin in enumerate(plugin_modules):
        _LOG.info("Applying plugin module #%d: %r", i, plugin)
        if hasattr(plugin, "apply_plugin_command"):
            plugin.apply_plugin_command(crd, user_command_action, crds, channel)
        else:
            _LOG.debug(
                "`apply_plugin_command` function found in plugin module %r", plugin
            )
    _LOG.info("Apply plugin done for %r entity", crd.name)
    return


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
    # TODO(#935): we don't have to create all CRD instances for all services
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


def check_crd(crd: CRD, user_command_action: str) -> None:
    """Check CRD action validity."""
    # TODO: this will be handled by CRD automatically later with argparse
    if user_command_action not in crd.func_signature:
        _LOG.debug(
            "Unknown action `%r`: %r",
            user_command_action,
            crd.func_signature,
        )
        print(f"Unknown action: `{crd.name}`")
        print_help_available_actions(
            [(k, v.get("help", "")) for k, v in crd.func_signature.items()]
        )
        print(f"\nRun 'ma {crd.name} --help' for more information")
        sys.exit(1)


def discover_all_plugins() -> dict[str, list[object]]:
    """Discover and import all entity plugins early.

    Scans all plugin directories and imports all entity plugin modules,
    but does NOT apply them yet (lazy application).

    Returns:
        dict[str, list[object]]: Plugin registry mapping entity names to modules
        Example: {
            'pipeline': [pipeline_plugin_module_0, pipeline_plugin_module_1],
            'pipeline_run': [pipeline_run_plugin_module_0],
        }
    """
    _LOG.info("Discovering all entity plugins...")

    registry: dict[str, list[object]] = {}
    plugin_dirs = [str(DEFAULT_DIR_PLUGINS), *_CONFIG["plugin"]["dirs"]]

    for plugin_dir_path in plugin_dirs:
        plugin_dir = Path(plugin_dir_path)
        entity_base = plugin_dir / "entity"

        if not entity_base.exists() or not entity_base.is_dir():
            _LOG.debug("Plugin entity directory not found: %r", entity_base)
            continue

        _LOG.debug("Scanning plugin directory: %r", entity_base)

        for entity_dir in entity_base.iterdir():
            if not entity_dir.is_dir():
                continue

            entity_name = entity_dir.name

            if entity_name.startswith("__"):
                continue

            _LOG.debug("Found entity plugin: %s", entity_name)

            plugin_module = read_module_from_file(
                entity_name, plugin_dir, len(registry.get(entity_name, []))
            )

            if plugin_module is not None:
                if entity_name not in registry:
                    registry[entity_name] = []
                registry[entity_name].append(plugin_module)
                _LOG.info("Registered plugin for entity '%s'", entity_name)

    _LOG.info(
        "Plugin discovery complete. Found plugins for %d entities: %s",
        len(registry),
        list(registry.keys()),
    )

    return registry


def apply_entity_plugins(
    crd: CRD, channel: Channel, plugin_registry: dict[str, list[object]]
) -> None:
    """Apply entity-level plugins from pre-loaded registry.

    Args:
        crd: CRD instance for the selected entity
        channel: gRPC channel
        plugin_registry: Pre-loaded plugin modules from discover_all_plugins()
    """
    plugins = plugin_registry.get(crd.name, [])

    if not plugins:
        _LOG.debug("No entity plugins found for '%s'", crd.name)
        return

    _LOG.info("Applying %d entity plugin(s) for '%s'", len(plugins), crd.name)

    for i, plugin in enumerate(plugins):
        _LOG.debug("Applying entity plugin #%d: %r", i, plugin)
        if hasattr(plugin, "apply_plugins"):
            plugin.apply_plugins(crd, channel)
        else:
            _LOG.debug(
                "Plugin module %r has no 'apply_plugins' function (skipped)", plugin
            )

    _LOG.info("Entity plugins applied for '%s'", crd.name)


def apply_command_plugins(
    crd: CRD,
    action: str,
    crds: dict[str, CRD],
    channel: Channel,
    plugin_registry: dict[str, list[object]],
) -> None:
    """Apply command-level plugins from pre-loaded registry.

    Args:
        crd: CRD instance for the selected entity
        action: User command action (e.g., "apply", "run")
        crds: All CRD instances
        channel: gRPC channel
        plugin_registry: Pre-loaded plugin modules
    """
    plugins = plugin_registry.get(crd.name, [])

    if not plugins:
        _LOG.debug("No command plugins found for '%s'", crd.name)
        return

    _LOG.info(
        "Applying %d command plugin(s) for '%s.%s'", len(plugins), crd.name, action
    )

    for i, plugin in enumerate(plugins):
        _LOG.debug("Applying command plugin #%d: %r", i, plugin)
        if hasattr(plugin, "apply_plugin_command"):
            plugin.apply_plugin_command(crd, action, crds, channel)
        else:
            _LOG.debug(
                "Plugin module %r has no 'apply_plugin_command' function (skipped)",
                plugin,
            )

    _LOG.info("Command plugins applied for '%s.%s'", crd.name, action)


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
    # TODO(#937): this will be generated by CRD automatically later
    if len(remaining) < 1:
        print(f"Usage: ma {crd.name} <action> [options]")
        print_help_available_actions(
            [(k, v.get("help", "")) for k, v in crd.func_signature.items()]
        )
        print(f"\nRun 'ma {crd.name} --help' for more information")
        sys.exit(1)

    if len(remaining) >= 1 and remaining[0] in ["--help", "-h"]:
        print(f"Usage: ma {crd.name} <action> [options]")
        print_help_available_actions(
            [(k, v.get("help", "")) for k, v in crd.func_signature.items()]
        )
        print(f"\nFor action-specific help, use: ma {crd.name} <action> --help")
        sys.exit(0)


def main(channel: Channel, plugin_registry: dict[str, list[object]]):
    """Main function for mactl.

    Args:
        channel: gRPC channel
        plugin_registry: Pre-loaded plugin modules from discover_all_plugins()
    """
    _LOG.debug("Starting mactl...")

    # Load config and set environment variables
    setup_minio_env()

    # Phase 1: Discover CRDs and create resource subcommands
    crds = discover_crds(channel)

    # Phase 2: Pre-parse to get selected resource
    namespace, remaining = pre_parse_args(crds)
    user_command_crd = str(namespace.entity)

    # Apply entity-level plugins from registry
    apply_entity_plugins(crds[user_command_crd], channel, plugin_registry)

    # Handle CRD-level help (e.g., "ma project --help" or "ma project -h")
    handle_crd_action_help(crds[user_command_crd], remaining)

    # Phase 3: Generate method + configure argparse
    user_command_action = kebab_to_snake(remaining[0])
    check_crd(crds[user_command_crd], user_command_action)

    # Apply command-level plugins from registry
    apply_command_plugins(
        crds[user_command_crd], user_command_action, crds, channel, plugin_registry
    )

    _LOG.debug(
        "Generating action `%r` for CRD `%r`: %r",
        user_command_action,
        crds[user_command_crd],
        dir(crds[user_command_crd]),
    )
    func_generator = getattr(crds[user_command_crd], f"generate_{user_command_action}")
    action_parser = ArgumentParser(
        prog=f"mactl {user_command_crd} {user_command_action}"
    )
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


def _is_service_name(address: str) -> bool:
    """Check if address is a service name (not host:port)."""
    return ":" not in address and "." not in address


def run():
    """Entry point for mactl."""
    # Discover all plugins early (before connection)
    _LOG.info("Discovering all plugins...")
    plugin_registry = discover_all_plugins()
    _LOG.info("Plugin discovery complete")

    proxy_module_path = _CONFIG["plugin"].get("proxy", "")

    if _is_service_name(ADDRESS) and proxy_module_path:
        proxy_mod = importlib.import_module(proxy_module_path)
        cli_proxy_class = proxy_mod.CLIProxy
        with cli_proxy_class() as proxy:
            local_address = proxy.create_tunnel(ADDRESS)
            with insecure_channel(local_address) as channel:
                return main(channel)

    if USE_TLS:
        _LOG.info(
            "Using TLS (forced via MACTL_USE_TLS=%r) to connect to %r",
            USE_TLS,
            ADDRESS,
        )
        # Use secure TLS connection
        with secure_channel(ADDRESS, ssl_channel_credentials()) as channel:
            return main(channel, plugin_registry)

    _LOG.info(
        "Using insecure connection (MACTL_USE_TLS=%r) to connect to %r",
        USE_TLS,
        ADDRESS,
    )
    # Use insecure connection for local development
    with insecure_channel(ADDRESS) as channel:
        return main(channel, plugin_registry)


if __name__ == "__main__":
    run()
