"""
MaCTL - Michelangelo Command Line Tool
A command line interface to interact with the Michelangelo API server via gRPC.
"""

from argparse import ArgumentParser
from collections import defaultdict
from collections.abc import MutableMapping
from copy import deepcopy
from dataclasses import dataclass
from functools import partial
from importlib.util import spec_from_file_location, module_from_spec
from inspect import Signature, Parameter
from logging import basicConfig, getLogger, WARNING
from os import getenv, environ
from pathlib import Path
from types import MethodType
from typing import Any, Callable, Optional, Union
import logging
import re
import sys

from google.protobuf import message_factory
from google.protobuf.descriptor_pb2 import (
    FileDescriptorProto,
    MethodDescriptorProto,
)
from google.protobuf.descriptor_pool import DescriptorPool
from google.protobuf.json_format import MessageToDict, ParseDict
from google.protobuf.message import Message
from grpc import (
    Channel,
    RpcError,
    StatusCode,
    insecure_channel,
    secure_channel,
    ssl_channel_credentials,
)
from grpc_reflection.v1alpha import reflection_pb2, reflection_pb2_grpc
from yaml import YAMLError, safe_load as yaml_safe_load
import configparser


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
# Set to "true" to force TLS, "false" to force insecure, or leave unset for auto-detection
USE_TLS: bool = getenv("MACTL_USE_TLS", _rc_config.get("use_tls", "false")).lower() in (
    "true",
    "1",
    "yes",
    "y",
)
if "metadata" in _rc_config:
    METADATA = list(_rc_config["metadata"].items())

METADATA_STUB = METADATA + [("ttl", "600")]

basicConfig(
    level=getattr(logging, getenv("LOG_LEVEL", "WARNING").upper(), WARNING),
    format="%(asctime)s | %(levelname)-8s | %(name)-40s | %(message)s",
)
_LOG = getLogger(__name__)

PWD = Path(__file__).parent.resolve()

_LOG.info(f"Config: ADDRESS={ADDRESS}, USE_TLS={USE_TLS}, METADATA={METADATA}")
DEFAULT_DIR_PLUGINS = PWD / "plugins"
CONFIG_FILE = PWD / "config.yaml"


def camel_to_snake(name: str) -> str:
    res = re.sub("(.)([A-Z][a-z]+)", r"\1_\2", name)
    return re.sub("([a-z0-9])([A-Z])", r"\1_\2", res).lower()


def snake_to_camel(name: str) -> str:
    """
    snake_case → UpperCamelCase(PascalCase)
    ex) "my_function_name" → "MyFunctionName"
    """
    return "".join(word.capitalize() for word in name.split("_"))


def kebab_to_snake(name: str) -> str:
    """
    Converts kebab-case to snake_case (e.g., 'dev-run' -> 'dev_run').
    """
    return name.replace("-", "_")


def bind_signature(signature):
    def decorator(func):
        def wrapper(*args, **kwargs):
            _LOG.debug("Binding signature for function %r", func)
            bound_args = signature.bind(*args, **kwargs)
            bound_args.apply_defaults()
            return func(bound_args)

        return wrapper

    return decorator


def deep_update(d: MutableMapping, u: MutableMapping):
    """
    Update dict-like object in deep way.

    ```py
    d1 = {'a': {'a1': 1, 'a2': 2}}
    d2 = {'a': {'a1': 7, 'a3': 9}}

    deep_update(d1, d2)
    print(d1)
    # {'a': {'a1': 7, 'a2': 2, 'a3': 9}}
    ```
    """
    for k, v in u.items():
        if isinstance(v, MutableMapping) and isinstance(d.get(k), MutableMapping):
            deep_update(d[k], v)
        else:
            d[k] = v
    return d


def get_single_arg(arguments: dict, key: str) -> str:
    """
    Get a single argument from the arguments dictionary.

    Args:
        arguments: The arguments dictionary.
        key: The key of the argument to get.

    Returns:
        The value of the single argument.

    Raises:
        ValueError: If the argument is not a string or a list with one element.
        KeyError: If the argument is missing.
    """
    if key not in arguments:
        raise KeyError(f'argument "{key}" is required')
    value = arguments[key]
    if isinstance(value, str):
        return value
    elif isinstance(value, list):
        if len(value) == 1:
            return value[0]
        else:
            raise ValueError(f'exactly one "{key}" argument is required')
    else:
        raise ValueError(
            f'Argument "{key}" must be a string or a list with one element'
        )


@dataclass
class CrdMethodInfo:
    """
    Method information to run CRD member method with grpc reflection
    """

    channel: Channel
    crd_full_name: str
    method_name: str
    input_class: type[Message]
    output_class: type[Message]


def crd_method_call_kwargs(crd_method_info, **kwargs) -> Message:
    """
    Run CRD.method with grpc reflection with custom kwargs
    (for input_class)

    Please make sure crd method input_class can be constructed
    with given kwargs.
    """
    _LOG.debug("Prepare CRD method call (%r) with kwargs: %r", crd_method_call, kwargs)
    # TODO (Hwamin): Add validation for kwargs keys/values
    request_input = crd_method_info.input_class(**kwargs)
    return crd_method_call(crd_method_info, request_input)


def crd_method_call(crd_method_info, request_input: Message) -> Message:
    """
    Call member method call of a CRD with grpc reflection
    """
    _LOG.debug("CRD method info: %r", crd_method_info)
    _LOG.debug("Request input (%r): %r", type(request_input), request_input)

    method_fullname = f"/{crd_method_info.crd_full_name}/{crd_method_info.method_name}"
    _LOG.info("Method fullname for gRPC call: %s", method_fullname)
    stub_method = crd_method_info.channel.unary_unary(
        method_fullname,
        request_serializer=crd_method_info.input_class.SerializeToString,
        response_deserializer=crd_method_info.output_class.FromString,
    )
    response = stub_method(
        request_input,
        metadata=METADATA_STUB,
        timeout=30,
    )
    _LOG.info("Stub method completed (%r): %r", type(response), response)
    return response


def get_func_impl(crd_method_info: CrdMethodInfo, bound_args: Signature) -> Message:
    """
    Default common CRD member method implementation for GET method.
    """
    _LOG.info("Bound arguments: %r", bound_args.arguments)

    if "name" not in bound_args.arguments or not bound_args.arguments["name"]:
        _LOG.debug("No name argument passed. List CRD in the namespace.")
        _self: CRD = bound_args.arguments["self"]
        _self.generate_list(crd_method_info.channel)
        return _self.list(namespace=get_single_arg(bound_args.arguments, "namespace"))

    return crd_method_call_kwargs(
        crd_method_info,
        **{
            "namespace": get_single_arg(bound_args.arguments, "namespace"),
            "name": get_single_arg(bound_args.arguments, "name"),
        },
    )


def list_func_impl(crd_method_info: CrdMethodInfo, bound_args: Signature) -> Message:
    """
    Default common CRD member method implementation for LIST method.
    """
    _LOG.info("Bound arguments: %r", bound_args.arguments)

    return crd_method_call_kwargs(
        crd_method_info,
        **{"namespace": get_single_arg(bound_args.arguments, "namespace")},
    )


def delete_func_impl(crd_method_info: CrdMethodInfo, bound_args: Signature) -> Message:
    """
    Default common CRD member method implementation for DELETE method.
    """
    _LOG.info("Bound arguments: %r", bound_args.arguments)

    return crd_method_call_kwargs(
        crd_method_info,
        **{
            "namespace": get_single_arg(bound_args.arguments, "namespace"),
            "name": get_single_arg(bound_args.arguments, "name"),
        },
    )


def apply_func_impl(crd_method_info: CrdMethodInfo, bound_args: Signature) -> Message:
    """
    Default common CRD member method implementation for APPLY method.
    """
    _LOG.info("Bound arguments: %r", bound_args.arguments)
    _self: CRD = bound_args.arguments["self"]
    _LOG.info("Start apply_func for %r", _self.full_name)

    _file = get_single_arg(bound_args.arguments, "file")

    _namespace, _name = get_crd_namespace_and_name_from_yaml(_file)

    message_instance = None
    try:
        message_instance = _self.get(_namespace, _name)
    except RpcError as err:
        _LOG.debug("CRD %r / %r does not exist: %r", _namespace, _name, err)
        if err.code() != StatusCode.NOT_FOUND:
            raise

    if message_instance is None:
        # Create new CRD
        _LOG.info("Create a new CRD")
        _self.generate_create(crd_method_info.channel)
        return _self.create(_file)

    # Update existing CRD
    _LOG.info("Retrieved message instance: %r", message_instance)
    request_input = _self.read_yaml_and_update_crd_request(
        crd_method_info.input_class, _file, message_instance
    )
    return crd_method_call(crd_method_info, request_input)


def create_func_impl(crd_method_info: CrdMethodInfo, bound_args: Signature) -> Message:
    """
    Default common CRD member method implementation for CREATE method.
    """
    _LOG.info("Bound arguments: %r", bound_args.arguments)
    _self: CRD = bound_args.arguments["self"]
    _LOG.info("Start create_func for %r", _self.full_name)

    _file = get_single_arg(bound_args.arguments, "file")

    request_input = read_yaml_to_crd_request(
        crd_method_info.input_class,
        _self.name,
        _file,
        _self.func_crd_metadata_converter,
    )
    return crd_method_call(crd_method_info, request_input)


class CRD:
    """
    Representation of each CRD with its service methods
    """

    def __init__(self, name: str, full_name: str):
        self.name = name
        self.full_name = full_name
        self.func_crd_metadata_converter = convert_crd_metadata
        self.method_info: dict = {}
        self.func_signature: dict = {
            "apply": [
                {
                    "func_signature": Parameter(
                        "file",
                        Parameter.POSITIONAL_OR_KEYWORD,
                    ),
                    "args": ["-f", "--file"],
                    "kwargs": {
                        "dest": "file",
                        "type": str,
                        "required": True,
                        "help": "Custom Resource YAML file (can be configured with --file)",
                    },
                },
            ],
            "delete": [
                {
                    "func_signature": Parameter(
                        "namespace", Parameter.POSITIONAL_OR_KEYWORD
                    ),
                    "args": ["-n", "--namespace"],
                    "kwargs": {
                        "type": str,
                        "required": True,
                        "help": "Namespace of the resource",
                    },
                },
                {
                    "func_signature": Parameter(
                        "name",
                        Parameter.POSITIONAL_OR_KEYWORD,
                        default="",
                    ),
                    "args": ["--name"],
                    "kwargs": {
                        "dest": "name",
                        "type": str,
                        "required": True,
                        "help": "Name of the resource",
                    },
                },
            ],
            "get": [
                {
                    "args": ["name"],
                    "kwargs": {
                        "nargs": "?",
                        "type": str,
                        "help": "Name of the resource (can be configured with --name)",
                    },
                },
                {
                    "func_signature": Parameter(
                        "namespace", Parameter.POSITIONAL_OR_KEYWORD
                    ),
                    "args": ["-n", "--namespace"],
                    "kwargs": {
                        "type": str,
                        "required": True,
                        "help": "Namespace of the resource",
                    },
                },
                {
                    "func_signature": Parameter(
                        "name",
                        Parameter.POSITIONAL_OR_KEYWORD,
                        default="",
                    ),
                    "args": ["--name"],
                    "kwargs": {
                        "dest": "name",
                        "type": str,
                        "required": False,
                        "help": "Name of the resource",
                    },
                },
            ],
        }

    def __repr__(self):
        return f"CRD(name={self.name}, full_name={self.full_name})"

    def configure_parser(self, action: str, parser: Optional[ArgumentParser]) -> None:
        """
        Configure argparse parser for action, if parse is set.
        Detailed arguments would be defined by `arguments`.

        Args:
            parser: ArgumentParser to configure
            arguments: list of args and kwargs to add to the parser
        """
        _LOG.info("Configuring argparse (%r) for CRD `%r` action", parser, action)
        if parser is None:
            return
        _LOG.debug(
            "Start to configure parser with args: %r", self.func_signature[action]
        )
        for arg_def in self.func_signature[action]:
            args = arg_def.get("args", [])
            kwargs = arg_def.get("kwargs", {})
            parser.add_argument(*args, **kwargs)

    def _read_signatures(self, method_name: str) -> Signature:
        """
        Read function signatures for method name.
        """
        _LOG.debug("Prepare func signature for `%r` function", method_name)
        res = Signature(
            [Parameter("self", Parameter.POSITIONAL_OR_KEYWORD)]
            + [
                arg["func_signature"]
                for arg in self.func_signature[method_name]
                if "func_signature" in arg
            ]
        )
        _LOG.debug("Read func signature: %r", res)
        return res

    def _extract_method_info(
        self, channel: Channel, full_name: str, function_name: str
    ) -> tuple[str, type[Message], type[Message]]:
        """
        Extract method information and their input/output types
        """
        assert isinstance(function_name, str), function_name
        assert function_name in ["Get", "Update", "Create", "List", "Delete"]

        methods, method_pool = get_methods_from_service(channel, full_name)
        method_name = function_name + snake_to_camel(self.name)

        _LOG.info("Get intput/output of method %r", method_name)
        try:
            method = methods[method_name]
        except KeyError:
            _LOG.warning(
                "Method %r not found in service %r",
                method_name,
                full_name,
            )
            _LOG.info("Method details: %r", methods)
            raise ValueError(f"Method {method_name} not found in service {full_name}")

        _LOG.debug("%r method input type: %r", function_name, method.input_type)
        _LOG.debug("%r method output type: %r", function_name, method.output_type)
        input_class = get_message_class_by_name(method_pool, method.input_type[1:])
        output_class = get_message_class_by_name(method_pool, method.output_type[1:])
        _LOG.debug(
            "Retrieved method input class: (%r) %r", type(input_class), input_class
        )
        _LOG.debug(
            "Retrieved method output class: (%r) %r", type(input_class), output_class
        )
        return method_name, input_class, output_class

    def generate_delete(self, channel: Channel, parser: Optional[ArgumentParser] = None):
        """
        Generate delete function of this class.
        """
        _LOG.info("Generate DELETE method for %r / %r", self.name, self.full_name)
        method_info = CrdMethodInfo(
            channel,
            self.full_name,
            *self._extract_method_info(channel, self.full_name, "Delete"),
        )

        self.configure_parser("delete", parser)
        func_signature = self._read_signatures("delete")

        bound_func = partial(delete_func_impl, method_info)
        bound_func = bind_signature(func_signature)(bound_func)
        self.delete = MethodType(bound_func, self)
        _LOG.debug("Generated DELETE injected well: %r", self.delete)

    def generate_get(self, channel: Channel, parser: Optional[ArgumentParser] = None):
        """
        Generate get function of this class.
        Optionally configure argparse parser with arguments.

        Args:
            channel: gRPC channel
            parser: Optional ArgumentParser to configure with --namespace and --name
        """
        _LOG.info("Generate GET method for %r / %r", self.name, self.full_name)
        method_info = CrdMethodInfo(
            channel,
            self.full_name,
            *self._extract_method_info(channel, self.full_name, "Get"),
        )

        self.configure_parser("get", parser)
        func_signature = self._read_signatures("get")

        bound_func = partial(get_func_impl, method_info)
        bound_func = bind_signature(func_signature)(bound_func)
        self.get = MethodType(bound_func, self)
        _LOG.debug("Generated GET injected well: %r", self.get)

    def generate_apply(self, channel: Channel, parser: Optional[ArgumentParser] = None):
        """
        Generate apply function of this class.
        """
        self.generate_get(channel)

        _LOG.info("Generate APPLY method for %r / %r", self.name, self.full_name)
        method_info = CrdMethodInfo(
            channel,
            self.full_name,
            *self._extract_method_info(channel, self.full_name, "Update"),
        )

        self.configure_parser("apply", parser)
        func_signature = self._read_signatures("apply")

        bound_func = partial(apply_func_impl, method_info)
        bound_func = bind_signature(func_signature)(bound_func)
        self.apply = MethodType(bound_func, self)
        _LOG.debug("Generated APPLY injected well: %r", self.apply)

    def generate_create(self, channel: Channel):
        """
        Generate create function of this class.
        """
        _LOG.info("Generate CREATE method for %r / %r", self.name, self.full_name)

        method_info = CrdMethodInfo(
            channel,
            self.full_name,
            *self._extract_method_info(channel, self.full_name, "Create"),
        )
        create_func_signature = Signature(
            [Parameter("self", Parameter.POSITIONAL_OR_KEYWORD)]
            + [Parameter(name, Parameter.POSITIONAL_OR_KEYWORD) for name in ["file"]]
        )

        bound_func = partial(create_func_impl, method_info)
        bound_func = bind_signature(create_func_signature)(bound_func)
        self.create = MethodType(bound_func, self)
        _LOG.debug("Generated CREATE injected well: %r", self.create)

    def generate_list(self, channel: Channel):
        """
        Generate list function of this class.
        """
        _LOG.info("Generate LIST method for %r / %r", self.name, self.full_name)

        method_info = CrdMethodInfo(
            channel,
            self.full_name,
            *self._extract_method_info(channel, self.full_name, "List"),
        )
        list_func_signature = Signature(
            [Parameter("self", Parameter.POSITIONAL_OR_KEYWORD)]
            + [
                Parameter(name, Parameter.POSITIONAL_OR_KEYWORD)
                for name in ["namespace"]
            ]
        )

        bound_func = partial(list_func_impl, method_info)
        bound_func = bind_signature(list_func_signature)(bound_func)
        self.list = MethodType(bound_func, self)
        _LOG.debug("Generated LIST injected well: %r", self.list)

    def read_yaml_and_update_crd_request(
        self, input_class: type[Message], yaml_path_string: str, original_crd: Message
    ) -> Message:
        """
        Read a YAML file and update the original CRD request instance.
        """
        original_crd_dict: dict = MessageToDict(
            original_crd, preserving_proto_field_name=True
        )
        _LOG.debug("Original CRD dict: %r", original_crd_dict)

        yaml_dict = yaml_to_dict(yaml_path_string)
        _LOG.debug(
            "Remove top-level apiVersion/kind from YAML dict,"
            " since we don't allow to change typemeta"
        )
        yaml_dict.pop("apiVersion", None)
        yaml_dict.pop("kind", None)
        _LOG.debug("Finished to read YAML file: %r", yaml_dict)

        deep_update(original_crd_dict[self.name], yaml_dict)
        _LOG.debug("Updated CRD config dict: %r", original_crd_dict)

        res = input_class()
        ParseDict(original_crd_dict, res)
        _LOG.info("Updated CRD instance to send API (%r): %r", type(res), res)
        return res


def list_services(channel) -> list[str]:
    stub = reflection_pb2_grpc.ServerReflectionStub(channel)

    request = reflection_pb2.ServerReflectionRequest(list_services="")
    responses = stub.ServerReflectionInfo(
        iter([request]),
        metadata=METADATA,
    )
    _LOG.info("Got response from ServerReflection: %r", responses)

    for response in responses:
        services = response.list_services_response.service
        return [s.name for s in services]
    raise ValueError("No services found")


def get_message_class_by_name(pool: DescriptorPool, message_name: str) -> type[Message]:
    """
    message_name example: "michelangelo.api.v2beta1.Pipeline"
    """
    descriptor = pool.FindMessageTypeByName(message_name)
    # factory = message_factory.MessageFactory(pool)
    # MessageClass = factory.GetPrototype(descriptor)
    MessageClass = message_factory.GetMessageClass(descriptor)
    return MessageClass


def read_plugins(
    crd: CRD, user_command_action: str, crds: dict[str, CRD], channel: Channel
) -> None:
    """
    Read and apply plugins for a given crd.
    """
    _LOG.info("Read plugins for crd: %r", crd)
    _LOG.info("Check Plugin directory: %r", DEFAULT_DIR_PLUGINS)
    plugin_dir = DEFAULT_DIR_PLUGINS / crd.name
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
        f"plugin_{crd.name}_main", str(plugin_main.resolve())
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

    plugin_module.apply_plugins(crd, user_command_action, crds, channel)
    return


def read_yaml_to_crd_request(
    crd_class: type[Message],
    crd_name: str,
    yaml_path_string: str,
    func_crd_metadata_converter: Callable,
) -> Message:
    """
    Reads a YAML file and converts it to a CRD request instance.
    """
    yaml_path = Path(yaml_path_string).resolve()
    yaml_dict = yaml_to_dict(yaml_path_string)
    crd_dict = {
        crd_name: func_crd_metadata_converter(yaml_dict, crd_class, yaml_path),
    }
    _LOG.debug("CRD content: %r", crd_dict)
    crd_instance = crd_class()
    ParseDict(crd_dict, crd_instance)
    _LOG.info("Parsed CRD instance (%r): %r", type(crd_instance), crd_instance)
    return crd_instance


def convert_crd_metadata(
    yaml_dict: dict, crd_class: type[Message], yaml_path: Path
) -> dict:
    """
    Convert CRD metadata for a given class.

    Since Michelangelo yaml format is putting `apiVersion` and `kind`
    at the top level, we need to move them inside of the `typemeta` field.
    """
    _LOG.info("Convert CRD metadata for class %r from %r", crd_class, yaml_path)
    if not isinstance(yaml_dict, dict):
        _LOG.error("Expected a dictionary, got: %r", type(yaml_dict))
        raise ValueError("Expected a dictionary for CRD metadata")
    _LOG.debug("Raw yaml dict: metadata: %r", yaml_dict)

    res = deepcopy(yaml_dict)
    if "apiVersion" in res:
        res.setdefault("typeMeta", {})["apiVersion"] = res.pop("apiVersion")
    if "kind" in res:
        res.setdefault("typeMeta", {})["kind"] = res.pop("kind")
    _LOG.debug("Converted CRD metadata: %r", res)
    return res


def get_crd_namespace_and_name_from_yaml(yaml_path_string: str) -> tuple[str, str]:
    """
    Reads a YAML file and returns its content as a dictionary.
    """
    _LOG.info("Start to Read YAML file: %r", yaml_path_string)
    yaml_dict = yaml_to_dict(yaml_path_string)

    assert "metadata" in yaml_dict, "YAML must contain 'metadata' key"

    metadata = yaml_dict["metadata"]

    assert "namespace" in metadata, "YAML metadata must contain 'namespace' key"
    assert "name" in metadata, "YAML metadata must contain 'name' key"

    namespace = metadata["namespace"]
    name = metadata["name"]

    _LOG.info("Retrieved namespace: %r, name: %r", namespace, name)
    assert isinstance(namespace, str), "kind must be a string"
    assert isinstance(name, str), "kind must be a string"
    return namespace, name


def get_crd_name_from_yaml(yaml_path_string: str) -> str:
    """
    Reads a YAML file and returns its content as a dictionary.
    """
    _LOG.info("Start to Read YAML file: %r", yaml_path_string)
    yaml_dict = yaml_to_dict(yaml_path_string)

    assert "apiVersion" in yaml_dict, "YAML must contain 'apiVersion' key"
    assert "kind" in yaml_dict, "YAML must contain 'kind' key"

    api_version = yaml_dict["apiVersion"]
    kind = yaml_dict["kind"]

    _LOG.info("API Version: %s, Kind: %s", api_version, kind)
    assert isinstance(kind, str), "kind must be a string"
    return kind


def yaml_to_dict(yaml_path_string: str) -> dict[str, Any]:
    """Converts a YAML string to a Python dictionary."""
    _LOG.info(
        "Start to Read YAML file to dict: %r",
        yaml_path_string,
    )
    yaml_path = Path(yaml_path_string).resolve()
    with yaml_path.open("r") as f:
        yaml_content = f.read()
    _LOG.debug("YAML content: %r", yaml_content)

    try:
        res = yaml_safe_load(yaml_content)
    except YAMLError as e:
        _LOG.error("Error loading YAML: %s", e)
        raise ValueError(f"Error loading YAML: {e}") from e

    _LOG.info("YAML content loaded successfully: %r", list(res))
    return res


def get_methods_from_service(
    channel: Channel, service: str
) -> tuple[dict[str, MethodDescriptorProto], DescriptorPool]:
    """
    Get methods from a service descriptor.
    """
    methods = list(get_service_descriptors(channel, service))
    _LOG.info("Succeed to retireve %d methods", len(methods))
    _LOG.debug("Detailed Method information: %r", methods)
    if not methods:
        _LOG.error("No methods found for service: %s", service)
        raise ValueError(f"No methods found for service: {service}")

    _LOG.info("Method list for service: %r", [m.name for m in methods])
    # Assume only one method matching
    pool = retrieve_full_descriptor_pool(channel, methods[0].name)
    method_services = [m for m in methods[0].service]
    _LOG.info(
        "Method service list for %d service: %r / %r",
        len(method_services),
        [s.name for s in method_services],
        [m.name for s in method_services for m in s.method],
    )
    if not method_services:
        _LOG.error("No methods found for service: %s", method_services)
        raise ValueError(f"No methods found for service: {method_services}")
    res = {m.name: m for m in method_services[0].method}
    _LOG.info(
        "Result: %d methods: %r",
        len(res),
        {k: type(v) for k, v in res.items()},
    )
    return res, pool


def get_all_file_descriptors_by_filename(
    channel: Channel,
    filename: str,
    deps: int = 0,
    visited: Union[None, set[str]] = None,
):
    """
    Get all file descriptors recursively by filename using ServerReflection.
    """
    _LOG.debug(
        "Start to get all descriptors with deps %2d for %r / %r / %r",
        deps,
        channel,
        filename,
        len(visited) if visited is not None else None,
    )

    if deps > 1000:
        raise RecursionError(
            "Maximum recursion depth exceeded in get_all_file_descriptors_by_filename"
        )

    if visited is None:
        visited = set()
    elif filename in visited:
        return
    visited.add(filename)

    stub = reflection_pb2_grpc.ServerReflectionStub(channel)
    request = reflection_pb2.ServerReflectionRequest(file_by_filename=filename)
    responses = stub.ServerReflectionInfo(iter([request]), metadata=METADATA)
    for response in responses:
        for fd_bytes in response.file_descriptor_response.file_descriptor_proto:
            fd = FileDescriptorProto()
            fd.ParseFromString(fd_bytes)
            _LOG.debug("Deps: %r", fd.dependency)
            for dependency in fd.dependency:
                yield from get_all_file_descriptors_by_filename(
                    channel, dependency, deps + 1, visited
                )
            yield fd


def retrieve_full_descriptor_pool(channel: Channel, filename: str) -> DescriptorPool:
    """
    Retrieve the full descriptor pool for a given filename.
    This is useful for introspection and understanding the service's API.
    """
    _LOG.info("Start retrieve full descriptor pool for %r / %r", channel, filename)
    fds = list(get_all_file_descriptors_by_filename(channel, filename))
    # print([type(f) for f in fds])
    pool = DescriptorPool()
    for fd in fds:
        pool.Add(fd)

    _LOG.info("Retrieved %d file descriptors", len(fds))
    return pool


def create_serivce_classes(services: list[str]) -> dict[str, CRD]:
    """
    Create service classes from a list of service names
    """
    res = {}
    for service in services:
        if service.endswith("Service") and not service.endswith("ExtService"):
            name = camel_to_snake(re.sub(r"Service$", "", service.split(".")[-1]))
            res[name] = CRD(name=name, full_name=service)
    _LOG.info("Created %d CRD instances: %r", len(res), res)
    return res


def get_service_descriptors(channel, service_name) -> list[FileDescriptorProto]:
    _LOG.info(
        "Start to get service descriptors for %r / %r",
        channel,
        service_name,
    )
    stub = reflection_pb2_grpc.ServerReflectionStub(channel)

    # Get FileDescriptor containing the service
    request = reflection_pb2.ServerReflectionRequest(
        file_containing_symbol=service_name, host=""
    )
    responses = stub.ServerReflectionInfo(
        iter([request]),
        metadata=METADATA,
    )

    for response in responses:
        file_desc_proto = response.file_descriptor_response.file_descriptor_proto
        for fd_bytes in file_desc_proto:
            fd = FileDescriptorProto()
            fd.ParseFromString(fd_bytes)
            yield fd


def parse_args() -> tuple[list[str], dict[str, list[str]]]:
    """
    Parse command line arguments.
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
    args, kwargs = parse_args()

    # New syntax: mactl <resource> <action> [options]
    user_command_crd = args[0]
    user_command_action = args[1]

    # For file-based actions, validate file parameter exists (preserving original validation)
    if user_command_action in ["apply", "create", "dev-run"]:
        assert len(kwargs["file"]) == 1, f"exactly one yaml file is required! {kwargs}"

    user_command_action = kebab_to_snake(user_command_action)

    _LOG.info(
        "User command CRD: %r / User command action: %r",
        user_command_crd,
        user_command_action,
    )
    return user_command_crd, user_command_action, kwargs


def main(channel: Channel):
    """
    Main function for mactl
    """
    _LOG.debug("Starting mactl...")

    # Load config and set environment variables
    if CONFIG_FILE.exists():
        with open(CONFIG_FILE, "r") as f:
            config = yaml_safe_load(f) or {}
        minio_config = config.get("minio", {})
        if not getenv("AWS_ACCESS_KEY_ID"):
            environ["AWS_ACCESS_KEY_ID"] = minio_config.get("access_key_id", "")
        if not getenv("AWS_SECRET_ACCESS_KEY"):
            environ["AWS_SECRET_ACCESS_KEY"] = minio_config.get("secret_access_key", "")
        if not getenv("AWS_ENDPOINT_URL"):
            environ["AWS_ENDPOINT_URL"] = minio_config.get("endpoint_url", "")

    # Phase 1: Discover CRDs and create resource subcommands
    services = list_services(channel)
    _LOG.info("Got %d services: %r", len(services), services)
    crds = create_serivce_classes(services)

    # Phase 2: Pre-parse to get selected resource
    base_parser = ArgumentParser(description="MaCTL - Michelangelo CLI", add_help=False)
    base_parser.add_argument(
        "-vv",
        "--verbose",
        action="store_true",
        help="Increase verbosity level",
    )
    resource_subparsers = base_parser.add_subparsers(dest="resource", required=True)

    for crd_name in crds.keys():
        resource_subparsers.add_parser(crd_name, add_help=False)

    # Parse only resource name, leave rest for later
    namespace, remaining = base_parser.parse_known_args()
    user_command_crd = namespace.resource

    # Handle CRD-level help (e.g., "ma project --help" or "ma project -h")
    # TODO: this will be generated by CRD automatically later
    if len(remaining) >= 1 and remaining[0] in ["--help", "-h"]:
        print(f"Usage: mactl {user_command_crd} <action> [options]")
        print("\nAvailable actions:")
        print("  get       Get or List specific resource(s)")
        print("  apply     Create or update a resource from YAML file")
        print("  delete    Delete a resource")
        print(
            f"\nFor action-specific help, use: mactl {user_command_crd} <action> --help"
        )
        sys.exit(0)

    if len(remaining) < 1:
        print(f"Usage: mactl {user_command_crd} <action>")
        print("Available actions: get, apply, delete")
        print(f"Run 'mactl {user_command_crd} --help' for more information")
        sys.exit(1)

    user_command_action = kebab_to_snake(remaining[0])

    # Phase 3: Generate method + configure argparse
    action_parser = ArgumentParser(
        prog=f"mactl {user_command_crd} {user_command_action}"
    )

    # Load plugins (may also configure action_parser)
    read_plugins(
        crds[user_command_crd],
        user_command_action,
        crds,
        channel,
    )

    func_generator = getattr(crds[user_command_crd], f"generate_{user_command_action}")
    func_generator(channel, action_parser)

    # Phase 4: Parse remaining arguments
    args = action_parser.parse_args(remaining[1:])

    # Phase 5: Execute
    func_action = getattr(crds[user_command_crd], user_command_action)
    _LOG.debug("target action function is ready: %r", func_action)
    result = func_action(**vars(args))

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
    print(result)


def run():
    """
    Entry point for mactl
    """
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
