from copy import deepcopy
from importlib.util import spec_from_file_location, module_from_spec
from inspect import Signature, Parameter
from logging import basicConfig, getLogger, WARNING
from os import getenv, environ
from pathlib import Path
from types import MethodType
from typing import Any, Callable, Union
from uuid import uuid4
import logging
import re
import sys

from google.protobuf import message_factory
from google.protobuf.descriptor_pb2 import (
    FileDescriptorProto,
    MethodDescriptorProto,
)
from google.protobuf.descriptor_pool import DescriptorPool
from google.protobuf.json_format import MessageToDict, MessageToJson, ParseDict
from google.protobuf.message import Message
from grpc import Channel, insecure_channel
from grpc_reflection.v1alpha import reflection_pb2, reflection_pb2_grpc
from yaml import YAMLError, safe_load as yaml_safe_load


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

# Allow overriding the API server address via environment variable
# This enables pointing the CLI to a k8s NodePort (e.g., 127.0.0.1:30009)
ADDRESS = getenv("MACTL_ADDRESS", ADDRESS)


METADATA_STUB = METADATA + [("ttl", "600")]

basicConfig(
    level=getattr(logging, getenv("LOG_LEVEL", "WARNING").upper(), WARNING),
    format="%(asctime)s | %(levelname)-8s | %(name)-40s | %(message)s",
)
_LOG = getLogger(__name__)

PWD = Path(__file__).parent.resolve()
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


def bind_signature(signature):
    def decorator(func):
        def wrapper(*args, **kwargs):
            _LOG.debug("Binding signature for function %r", func.__name__)
            bound_args = signature.bind(*args, **kwargs)
            bound_args.apply_defaults()
            return func(bound_args)

        return wrapper

    return decorator


class CRD:
    """
    Representation of each CRD with its service methods
    """

    def __init__(self, name: str, full_name: str):
        self.name = name
        self.full_name = full_name
        self.func_crd_metadata_converter = convert_crd_metadata

    def __repr__(self):
        return f"CRD(name={self.name}, full_name={self.full_name})"

    def _extract_method_info(
        self, channel: Channel, full_name: str, function_name: str
    ):
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
        _LOG.debug("Retrieved method input class: %r", input_class)
        _LOG.debug("Retrieved method output class: %r", output_class)

        return method_name, input_class, output_class

    def generate_delete(self, channel: Channel):
        """
        Generate delete function of this class.
        """
        _LOG.info("Generate DELETE method for %r / %r", self.name, self.full_name)
        method_name, input_class, output_class = self._extract_method_info(
            channel, self.full_name, "Delete"
        )
        delete_func_signature = Signature(
            [Parameter("self", Parameter.POSITIONAL_OR_KEYWORD)]
            + [
                Parameter(name, Parameter.POSITIONAL_OR_KEYWORD)
                for name in ["namespace", "name"]
            ]
        )

        @bind_signature(delete_func_signature)
        def delete_func(bound_args: Signature) -> Message:
            _LOG.info("Start delete_func for %r", self.full_name)
            _LOG.info("Bound arguments: %r", bound_args.arguments)
            _self: CRD = bound_args.arguments["self"]

            request_input = input_class(
                namespace=bound_args.arguments["namespace"],
                name=bound_args.arguments["name"],
            )
            _LOG.info(
                "DELETE Request input (%r) ready: %r",
                type(request_input),
                request_input,
            )

            method_fullname = f"/{_self.full_name}/{method_name}"
            _LOG.info("Method fullname for gRPC call: %s", method_fullname)
            stub_method = channel.unary_unary(
                method_fullname,
                request_serializer=input_class.SerializeToString,
                response_deserializer=output_class.FromString,
            )
            response = stub_method(
                request_input,
                metadata=METADATA_STUB,
                timeout=30,
            )
            _LOG.info("Stub method completed (%r): %r", type(response), response)
            return response

        delete_func.__signature__ = delete_func_signature  # type: ignore[attr-defined]
        self.delete = MethodType(delete_func, self)

    def generate_get(self, channel: Channel):
        """
        Generate get function of this class.
        """
        _LOG.info("Generate GET method for %r / %r", self.name, self.full_name)
        method_name, input_class, output_class = self._extract_method_info(
            channel, self.full_name, "Get"
        )
        get_func_signature = Signature(
            [Parameter("self", Parameter.POSITIONAL_OR_KEYWORD)]
            + [
                Parameter(name, Parameter.POSITIONAL_OR_KEYWORD)
                for name in ["namespace", "name"]
            ]
        )

        @bind_signature(get_func_signature)
        def get_func(bound_args: Signature) -> Message:
            _LOG.info("Start get_func for %r", self.full_name)
            _LOG.info("Bound arguments: %r", bound_args.arguments)
            _self: CRD = bound_args.arguments["self"]

            request_input = input_class(
                namespace=bound_args.arguments["namespace"],
                name=bound_args.arguments["name"],
            )
            _LOG.info(
                "GET Request input (%r) ready: %r",
                type(request_input),
                request_input,
            )

            method_fullname = f"/{_self.full_name}/{method_name}"
            _LOG.info("Method fullname for gRPC call: %s", method_fullname)
            stub_method = channel.unary_unary(
                method_fullname,
                request_serializer=input_class.SerializeToString,
                response_deserializer=output_class.FromString,
            )
            response = stub_method(
                request_input,
                metadata=METADATA_STUB,
                timeout=30,
            )
            _LOG.info("Stub method completed (%r): %r", type(response), response)
            return response

        get_func.__signature__ = get_func_signature  # type: ignore[attr-defined]
        self.get = MethodType(get_func, self)

    def generate_apply(self, channel: Channel):
        """
        Generate apply function of this class.
        """
        self.generate_get(channel)

        _LOG.info("Generate APPLY method for %r / %r", self.name, self.full_name)
        method_name, input_class, output_class = self._extract_method_info(
            channel, self.full_name, "Update"
        )
        apply_func_signature = Signature(
            [Parameter("self", Parameter.POSITIONAL_OR_KEYWORD)]
            + [Parameter(name, Parameter.POSITIONAL_OR_KEYWORD) for name in ["file"]]
        )

        @bind_signature(apply_func_signature)
        def apply_func(bound_args: Signature) -> Message:
            _LOG.info("Start apply_func for %r", self.full_name)
            _LOG.info("Bound arguments: %r", bound_args.arguments)
            _self: CRD = bound_args.arguments["self"]

            _namespace, _name = get_crd_namespace_and_name_from_yaml(
                bound_args.arguments["file"]
            )
            message_instance = _self.get(_namespace, _name)
            _LOG.info("Retrieved message instance: %r", message_instance)
            request_input = self.read_yaml_and_update_crd_request(
                input_class, bound_args.arguments["file"], message_instance
            )

            method_fullname = f"/{_self.full_name}/{method_name}"
            _LOG.info("Method fullname for gRPC call: %s", method_fullname)
            stub_method = channel.unary_unary(
                method_fullname,
                request_serializer=input_class.SerializeToString,
                response_deserializer=output_class.FromString,
            )
            response = stub_method(
                request_input,
                metadata=METADATA_STUB,
                timeout=30,
            )
            _LOG.info("Stub method completed (%r): %r", type(response), response)
            return response

        apply_func.__signature__ = apply_func_signature  # type: ignore[attr-defined]
        self.apply = MethodType(apply_func, self)

    def generate_create(self, channel: Channel):
        """
        Generate create function of this class.
        """
        _LOG.info("Generate CREATE method for %r / %r", self.name, self.full_name)
        method_name, input_class, output_class = self._extract_method_info(
            channel, self.full_name, "Create"
        )
        create_func_signature = Signature(
            [Parameter("self", Parameter.POSITIONAL_OR_KEYWORD)]
            + [Parameter(name, Parameter.POSITIONAL_OR_KEYWORD) for name in ["file"]]
        )

        @bind_signature(create_func_signature)
        def create_func(bound_args: Signature) -> Message:
            _LOG.info("Start create_func for %r", self.full_name)
            _LOG.info("Bound arguments: %r", bound_args.arguments)
            _self: CRD = bound_args.arguments["self"]

            request_input = read_yaml_to_crd_request(
                input_class,
                _self.name,
                bound_args.arguments["file"],
                _self.func_crd_metadata_converter,
            )

            method_fullname = f"/{_self.full_name}/{method_name}"
            _LOG.info("Method fullname for gRPC call: %s", method_fullname)
            stub_method = channel.unary_unary(
                method_fullname,
                request_serializer=input_class.SerializeToString,
                response_deserializer=output_class.FromString,
            )
            response = stub_method(
                request_input,
                metadata=METADATA_STUB,
                timeout=30,
            )
            _LOG.info("Stub method completed (%r): %r", type(response), response)
            return response

        create_func.__signature__ = create_func_signature  # type: ignore[attr-defined]
        self.create = MethodType(create_func, self)

    def generate_run(self, channel: Channel):
        """
        Generate run function for pipeline runs.
        """
        _LOG.info("Generate RUN method for %r / %r", self.name, self.full_name)

        # Only allow specific resource types for run command
        supported_run_resources = ["pipeline"]
        if self.name not in supported_run_resources:
            _LOG.error("'run %s' is not supported. Supported resources for run: %s", 
                      self.name, ", ".join(supported_run_resources))
            raise ValueError(f"'run {self.name}' is not supported. Supported resources for run: {', '.join(supported_run_resources)}")

        if self.name == "pipeline":
            pipeline_run_service = "michelangelo.api.v2.PipelineRunService"
            methods, method_pool = get_methods_from_service(channel, pipeline_run_service)
            method_name = "CreatePipelineRun"

        _LOG.info("Run input/output of method %r", method_name)
        try:
            method_create = methods[method_name]
        except KeyError:
            _LOG.warning(
                "Method %r not found in service %r",
                method_name,
                self.full_name,
            )
            _LOG.info("Method details: %r", methods)
            return

        _LOG.info("Run method input type: %r", method_create.input_type)
        _LOG.info("Run method output type: %r", method_create.output_type)
        input_class = get_message_class_by_name(method_pool, method_create.input_type[1:])
        output_class = get_message_class_by_name(
            method_pool, method_create.output_type[1:]
        )
        _LOG.info("Retrieved method input class: %r", input_class)
        _LOG.info("Retrieved method output class: %r", output_class)

        run_func_signature = Signature(
            [Parameter("self", Parameter.POSITIONAL_OR_KEYWORD)]
            + [Parameter(name, Parameter.POSITIONAL_OR_KEYWORD) for name in ["namespace", "name"]]
        )

        @bind_signature(run_func_signature)
        def run_func(bound_args: Signature) -> Message:
            _LOG.info("Start run_func for %r", self.full_name)
            _LOG.info("Bound arguments: %r", bound_args.arguments)
            _self: CRD = bound_args.arguments["self"]

            run_kwargs = {
                "namespace": bound_args.arguments["namespace"],
                "name": bound_args.arguments["name"]
            }
            
            pipeline_run_dict = _self.func_crd_metadata_converter(
                run_kwargs, input_class, None
            )
            
            request_input = input_class()
            from google.protobuf.json_format import ParseDict
            ParseDict(pipeline_run_dict, request_input)
            if _self.name == "pipeline":
                service_name = "michelangelo.api.v2.PipelineRunService"
            else:
                service_name = _self.full_name
            method_fullname = f"/{service_name}/{method_name}"
            _LOG.info("Method fullname for gRPC call: %s", method_fullname)
            stub_method = channel.unary_unary(
                method_fullname,
                request_serializer=input_class.SerializeToString,
                response_deserializer=output_class.FromString,
            )
            response = stub_method(
                request_input,
                metadata=METADATA_STUB,
                timeout=30,
            )
            _LOG.info("Stub method completed (%r): %r", type(response), response)
            return response

        run_func.__signature__ = run_func_signature  # type: ignore[attr-defined]
        self.run = MethodType(run_func, self)

    def generate_list(self, channel: Channel):
        """
        Generate list function of this class.
        """
        _LOG.info("Generate LIST method for %r / %r", self.name, self.full_name)

        methods, method_pool = get_methods_from_service(channel, self.full_name)
        method_name = "List" + snake_to_camel(self.name)

        _LOG.info("List intput/output of method %r", method_name)
        try:
            method_list = methods[method_name]
        except KeyError:
            _LOG.warning(
                "Method %r not found in service %r",
                method_name,
                self.full_name,
            )
            _LOG.info("Method details: %r", methods)
            return

        _LOG.info("Create method input type: %r", method_list.input_type)
        _LOG.info("Create method output type: %r", method_list.output_type)
        input_class = get_message_class_by_name(method_pool, method_list.input_type[1:])
        output_class = get_message_class_by_name(
            method_pool, method_list.output_type[1:]
        )
        _LOG.info("Retrieved method input class: %r", input_class)
        _LOG.info("Retrieved method output class: %r", output_class)

        list_func_signature = Signature(
            [Parameter("self", Parameter.POSITIONAL_OR_KEYWORD)]
            + [
                Parameter(name, Parameter.POSITIONAL_OR_KEYWORD)
                for name in ["namespace"]
            ]
        )

        # TODO: Need to handle `StatusCode.RESOURCE_EXHAUSTED` issue (max size)
        def list_func(*args, **kwargs):
            _LOG.info("Start list_func for %r", self.full_name)
            bound_args = list_func_signature.bind(*args, **kwargs)
            bound_args.apply_defaults()
            _LOG.info("Bound arguments: %r", bound_args.arguments)

            request_input = input_class(
                namespace=bound_args.arguments["namespace"],
            )
            _LOG.info(
                "LIST Request input (%r) ready: %r",
                type(request_input),
                request_input,
            )

            method_fullname = f"/{self.full_name}/{method_name}"
            _LOG.info("Method fullname for gRPC call: %s", method_fullname)
            stub_method = channel.unary_unary(
                method_fullname,
                request_serializer=input_class.SerializeToString,
                response_deserializer=output_class.FromString,
            )
            response = stub_method(
                request_input,
                metadata=METADATA_STUB,
                timeout=30,
            )
            _LOG.info("Stub method completed (%r): %r", type(response), response)
            return response

        list_func.__signature__ = list_func_signature  # type: ignore[attr-defined]
        self.list = MethodType(list_func, self)

    def read_yaml_and_update_crd_request(
        self, input_class: type[Message], yaml_path_string: str, original_crd: Message
    ) -> Message:
        """ """

        original_crd_dict = MessageToDict(
            original_crd, preserving_proto_field_name=True
        )
        _LOG.debug("Original CRD dict: %r", original_crd_dict)

        yaml_dict = yaml_to_dict(yaml_path_string)
        _LOG.debug("Finished to read YAML file: %r", yaml_dict)

        original_crd_dict[self.name]["spec"].update(yaml_dict["spec"])
        original_crd_dict[self.name]["metadata"]["generation"] = str(
            int(original_crd_dict[self.name]["metadata"]["generation"]) + 1
        )

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


def read_plugins(crd: CRD, user_command_action: str, crds: dict[str, CRD]) -> None:
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
    """
    _LOG.info("Convert CRD metadata for class %r from %r", crd_class, yaml_path)
    if not isinstance(yaml_dict, dict):
        _LOG.error("Expected a dictionary, got: %r", type(yaml_dict))
        raise ValueError("Expected a dictionary for CRD metadata")

    res = {"spec": deepcopy(yaml_dict["spec"])}
    res["metadata"] = {
        "clusterName": "",
        "generateName": "",
        "generation": "0",
        "name": yaml_dict["metadata"]["name"],
        "namespace": yaml_dict["metadata"]["namespace"],
        "resourceVersion": "0",
        "uid": str(uuid4()),
    }
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

    # 1. Get FileDescriptor containing the service
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


def parse_args() -> tuple[list[str], dict[str, str]]:
    """
    Parse command line arguments.
    Returns a tuple of (args, kwargs).
    """
    args = []
    kwargs = {}
    for arg in sys.argv[1:]:
        if "=" in arg:
            key, value = arg.split("=", 1)
            key = key.lstrip("-")
            kwargs[key] = value
        else:
            args.append(arg)
    _LOG.info("Parsed arguments: %r  /  %r", args, kwargs)
    return args, kwargs


def handle_args() -> tuple[str, str, dict[str, str]]:
    args, kwargs = parse_args()

    user_command_action = args[0]
    assert user_command_action in ["get", "create", "apply", "delete", "list", "run"]

    if user_command_action not in ["apply", "create", "run"]:
        user_command_crd = args[1]
    elif user_command_action == "run":
        # For run command: mactl run pipeline --namespace=<namespace> --name=<name>
        user_command_crd = args[1]  # This should be "pipeline"
    else:
        _LOG.info("Action is 'apply', read user_command_crd from yaml")
        user_command_crd = camel_to_snake(get_crd_name_from_yaml(kwargs["file"]))

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
    # Load config and set environment variables
    if CONFIG_FILE.exists():
        with open(CONFIG_FILE, 'r') as f:
            config = yaml_safe_load(f) or {}
        minio_config = config.get("minio", {})
        if not getenv("AWS_ACCESS_KEY_ID"):
            environ["AWS_ACCESS_KEY_ID"] = minio_config.get("access_key_id", "")
        if not getenv("AWS_SECRET_ACCESS_KEY"):
            environ["AWS_SECRET_ACCESS_KEY"] = minio_config.get("secret_access_key", "")
        if not getenv("AWS_ENDPOINT_URL"):
            environ["AWS_ENDPOINT_URL"] = minio_config.get("endpoint_url", "")

    user_command_crd, user_command_action, kwargs = handle_args()

    print("Starting mactl...")
    services = list_services(channel)
    _LOG.info("Got %d services: %r", len(services), services)

    crds = create_serivce_classes(services)

    assert user_command_crd in crds, (
        f"Command {user_command_crd} not found in services: {list(crds)}"
    )
    assert hasattr(crds[user_command_crd], f"generate_{user_command_action}"), (
        f"Command '{crds[user_command_crd]}' does not have generator"
        f" for '{user_command_action}' action."
    )
    func_action_generator = getattr(
        crds[user_command_crd], f"generate_{user_command_action}"
    )
    func_action_generator(channel)
    read_plugins(crds[user_command_crd], user_command_action, crds)

    assert hasattr(crds[user_command_crd], user_command_action)
    func_action = getattr(crds[user_command_crd], user_command_action)
    result = func_action(**kwargs)

    # Convert to JSON and pretty print
    json_output = MessageToJson(
        result, 
        always_print_fields_with_no_presence=True, 
        preserving_proto_field_name=True
    )
    print(json_output)


if __name__ == "__main__":
    with insecure_channel(ADDRESS) as channel:
        main(channel)
