"""CRD class and its member method implementations."""

from argparse import ArgumentParser
from collections.abc import MutableMapping, Sequence
from copy import deepcopy
from dataclasses import dataclass
from datetime import datetime
from functools import partial
from inspect import Parameter, Signature
from logging import getLogger
from pathlib import Path
from types import MethodType
from typing import Any, Callable, Optional

from google.protobuf.json_format import MessageToDict, ParseDict
from google.protobuf.message import Message
from grpc import (
    Channel,
    RpcError,
    StatusCode,
)
from yaml import YAMLError
from yaml import safe_load as yaml_safe_load

from michelangelo.cli.mactl.grpc_tools import (
    get_message_class_by_name,
    get_methods_from_service,
)

_LOG = getLogger(__name__)
METADATA_STUB = []


def bind_signature(signature):
    """Decorator to bind function signature to a function."""

    def decorator(func):
        def wrapper(*args, **kwargs):
            _LOG.debug("Binding signature for function %r", func)
            bound_args = signature.bind(*args, **kwargs)
            bound_args.apply_defaults()
            return func(bound_args)

        return wrapper

    return decorator


def convert_crd_metadata(
    yaml_dict: dict, crd_class: type[Message], yaml_path: Path
) -> dict:
    """Convert CRD metadata for a given class.

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


def deep_update(d: MutableMapping, u: MutableMapping):
    """Update dict-like object in deep way.

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


def get_crd_namespace_and_name_from_yaml(yaml_path_string: str) -> tuple[str, str]:
    """Reads a YAML file and returns its content as a dictionary."""
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


def get_single_arg(arguments: dict, key: str) -> str:
    """Get a single argument from the arguments dictionary.

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


def read_yaml_to_crd_request(
    crd_class: type[Message],
    crd_name: str,
    yaml_path_string: str,
    func_crd_metadata_converter: Callable,
) -> Message:
    """Reads a YAML file and converts it to a CRD request instance."""
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


def snake_to_camel(name: str) -> str:
    """snake_case → UpperCamelCase(PascalCase).

    ex) "my_function_name" → "MyFunctionName"
    """
    return "".join(word.capitalize() for word in name.split("_"))


@dataclass
class CrdMethodInfo:
    """Method information to run CRD member method with grpc reflection."""

    channel: Channel
    crd_full_name: str
    method_name: str
    input_class: type[Message]
    output_class: type[Message]


def crd_method_call_kwargs(crd_method_info, **kwargs) -> Message:
    """Run CRD.method with grpc reflection with custom kwargs.

    (for input_class)
    Please make sure crd method input_class can be constructed
    with given kwargs.
    """
    _LOG.debug("Prepare CRD method call (%r) with kwargs: %r", crd_method_call, kwargs)
    # TODO (Hwamin): Add validation for kwargs keys/values
    request_input = crd_method_info.input_class(**kwargs)
    return crd_method_call(crd_method_info, request_input)


def crd_method_call(crd_method_info, request_input: Message) -> Message:
    """Call member method call of a CRD with grpc reflection."""
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
    """Default common CRD member method implementation for GET method."""
    _LOG.info("Bound arguments: %r", bound_args.arguments)

    if "name" not in bound_args.arguments or not bound_args.arguments["name"]:
        _LOG.debug("No name argument passed. List CRD in the namespace.")
        _self: CRD = bound_args.arguments["self"]
        _self.generate_list(crd_method_info.channel)
        return _self.list(
            namespace=get_single_arg(bound_args.arguments, "namespace"),
            limit=bound_args.arguments.get("limit", 100),
        )

    call_res = crd_method_call_kwargs(
        crd_method_info,
        **{
            "namespace": get_single_arg(bound_args.arguments, "namespace"),
            "name": get_single_arg(bound_args.arguments, "name"),
        },
    )
    print(call_res)
    return call_res


def prepare_column_info() -> list[dict]:
    """Prepare column info for formatted printing of CRD items."""
    res = [
        {
            "column_name": "NAMESPACE",
            "retrieve_func": lambda item: item.metadata.namespace,
            "max_length": len("NAMESPACE") + 1,
        },
        {
            "column_name": "NAME",
            "retrieve_func": lambda item: item.metadata.name,
            "max_length": len("NAME") + 1,
        },
        {
            "column_name": "LAST_UPDATED_SPEC",
            "retrieve_func": lambda item: (
                datetime.fromtimestamp(
                    int(item.metadata.labels["michelangelo/UpdateTimestamp"])
                    / 1_000_000
                ).strftime("%Y-%m-%d_%H:%M:%S")
                if item.metadata.labels.get("michelangelo/UpdateTimestamp", "")
                else "N/A"
            ),
            "max_length": len("LAST_UPDATED_SPEC") + 1,
        },
    ]
    _LOG.debug("Prepared column info: %r", res)
    return res


def print_list_formatted(items: Sequence[Message]):
    """Print list of CRD items in formatted way."""
    _LOG.info("Print list of CRD items: %r (length %d)", type(items), len(items))

    ansi_header = "\033[1;37;44m"  # bold + white + blue background
    ansi_reset = "\033[0m"

    column_info = prepare_column_info()
    for item in items:
        for col in column_info:
            col["max_length"] = max(
                col["max_length"], len(col["retrieve_func"](item)) + 1
            )

    print(
        f"{ansi_header} "
        + "".join([col["column_name"].ljust(col["max_length"]) for col in column_info])
        + f"{ansi_reset}"
    )
    for item in items:
        print(
            " "
            + "".join(
                [
                    col["retrieve_func"](item).ljust(col["max_length"])
                    for col in column_info
                ]
            )
        )


def list_func_impl(crd_method_info: CrdMethodInfo, bound_args: Signature) -> Message:
    """Default common CRD member method implementation for LIST method."""
    _LOG.info("Bound arguments: %r", bound_args.arguments)

    limit = bound_args.arguments.get("limit", 100)

    request_dict = {
        "namespace": get_single_arg(bound_args.arguments, "namespace"),
        "list_options": {"limit": limit},
        "list_options_ext": {
            "pagination": {
                "offset": 0,
                "limit": limit,
            }
        },
    }

    request_input = crd_method_info.input_class()
    ParseDict(request_dict, request_input)
    _LOG.info("ListRequest built: %r", request_input)
    call_res = crd_method_call(crd_method_info, request_input)
    _LOG.debug("Succeed to list CRDs: %r", type(call_res))
    results = {k.name: v for k, v in call_res.ListFields() if k.name.endswith("_list")}
    _LOG.debug(
        "Extracted keys (%s): %r / %r",
        len(results),
        list(results),
        [type(v) for v in results.values()],
    )
    # we assume the there is only one list field in the response message
    raw_elems = results[next(iter(results))]

    print_list_formatted(raw_elems.items)

    # Show warning if we got exactly the limit (there might be more)
    if len(raw_elems.items) == limit:
        print(
            f"\n⚠️  The response is limited to {limit} pipelines. "
            f"There may be more than {limit} results. "
            f"Consider a larger limit with --limit argument or using filter "
            f"to narrow down the result. (default: 100)"
        )

    return call_res


def delete_func_impl(crd_method_info: CrdMethodInfo, bound_args: Signature) -> Message:
    """Default common CRD member method implementation for DELETE method."""
    _LOG.info("Bound arguments: %r", bound_args.arguments)

    call_res = crd_method_call_kwargs(
        crd_method_info,
        **{
            "namespace": get_single_arg(bound_args.arguments, "namespace"),
            "name": get_single_arg(bound_args.arguments, "name"),
        },
    )
    print(call_res)
    return call_res


def apply_func_impl(crd_method_info: CrdMethodInfo, bound_args: Signature) -> Message:
    """Default common CRD member method implementation for APPLY method."""
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
    call_res = crd_method_call(crd_method_info, request_input)
    print(call_res)
    return call_res


def create_func_impl(crd_method_info: CrdMethodInfo, bound_args: Signature) -> Message:
    """Default common CRD member method implementation for CREATE method."""
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
    call_res = crd_method_call(crd_method_info, request_input)
    print(call_res)
    return call_res


class CRD:
    """Representation of each CRD with its service methods."""

    def __init__(self, name: str, full_name: str, metadata: list):
        """Initialize CRD."""
        self.name = name
        self.full_name = full_name
        self.func_crd_metadata_converter = convert_crd_metadata
        self.metadata = metadata
        self.func_signature: dict[str, dict] = {
            "apply": {
                "help": "Apply an Entity (create or update)",
                "args": [
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
                            "help": (
                                "Custom Resource YAML file"
                                " (can be configured with --file)"
                            ),
                        },
                    },
                ],
            },
            "delete": {
                "help": "Delete an Entity",
                "args": [
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
            },
            "get": {
                "help": "Get an Entity or list all entities in the namespace",
                "args": [
                    {
                        "args": ["name"],
                        "kwargs": {
                            "nargs": "?",
                            "type": str,
                            "help": (
                                "Name of the resource (can be configured with --name)"
                            ),
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
                    {
                        "func_signature": Parameter(
                            "limit", Parameter.POSITIONAL_OR_KEYWORD, default=100
                        ),
                        "args": ["--limit"],
                        "kwargs": {
                            "dest": "limit",
                            "type": int,
                            "default": 100,
                            "help": (
                                "Maximum number of items to return when listing "
                                "(default: 100)"
                            ),
                        },
                    },
                ],
            },
        }

        # TODO: This would be changed to use centralized config metadata stub
        global METADATA_STUB
        METADATA_STUB = [*metadata, ("ttl", "600")]

    def __repr__(self):  # noqa: D105
        return f"CRD(name={self.name}, full_name={self.full_name})"

    def configure_parser(self, action: str, parser: Optional[ArgumentParser]) -> None:
        """Configure argparse parser for action, if parse is set.

        Detailed arguments would be defined by `arguments`.

        Args:
            action: action name to configure parser for
            parser: ArgumentParser to configure
            arguments: list of args and kwargs to add to the parser
        """
        _LOG.info("Configuring argparse (%r) for CRD `%r` action", parser, action)
        if parser is None:
            return
        _LOG.debug(
            "Start to configure parser with args: %r", self.func_signature[action]
        )
        for arg_def in self.func_signature[action]["args"]:
            args = arg_def.get("args", [])
            kwargs = arg_def.get("kwargs", {})
            parser.add_argument(*args, **kwargs)

    def _read_signatures(self, method_name: str) -> Signature:
        """Read function signatures for method name."""
        _LOG.debug("Prepare func signature for `%r` function", method_name)
        res = Signature(
            [Parameter("self", Parameter.POSITIONAL_OR_KEYWORD)]
            + [
                arg["func_signature"]
                for arg in self.func_signature[method_name]["args"]
                if "func_signature" in arg
            ]
        )
        _LOG.debug("Read func signature: %r", res)
        return res

    def _extract_method_info(
        self, channel: Channel, full_name: str, function_name: str
    ) -> tuple[str, type[Message], type[Message]]:
        """Extract method information and their input/output types."""
        assert isinstance(function_name, str), function_name
        assert function_name in ["Get", "Update", "Create", "List", "Delete"]

        methods, method_pool = get_methods_from_service(
            channel, full_name, self.metadata
        )
        method_name = function_name + snake_to_camel(self.name)

        _LOG.info("Get intput/output of method %r", method_name)
        try:
            method = methods[method_name]
        except KeyError as err:
            _LOG.warning(
                "Method %r not found in service %r",
                method_name,
                full_name,
            )
            _LOG.info("Method details: %r", methods)
            raise ValueError(
                f"Method {method_name} not found in service {full_name}"
            ) from err

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

    def generate_delete(
        self, channel: Channel, parser: Optional[ArgumentParser] = None
    ):
        """Generate delete function of this class."""
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
        """Generate get function of this class.

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
        """Generate apply function of this class."""
        self.generate_get(channel)

        _LOG.info("Generate APPLY method for %r / %r", self.name, self.full_name)
        method_info = CrdMethodInfo(
            channel,
            self.full_name,
            *self._extract_method_info(channel, self.full_name, "Update"),
        )

        self.configure_parser("apply", parser)
        func_signature = self._read_signatures("apply")

        bound_func = partial(
            getattr(self, "_apply_func_impl", apply_func_impl), method_info
        )
        bound_func = bind_signature(func_signature)(bound_func)
        self.apply = MethodType(bound_func, self)
        _LOG.debug("Generated APPLY injected well: %r", self.apply)

    def generate_create(self, channel: Channel):
        """Generate create function of this class."""
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

    def generate_list(self, channel: Channel, parser: Optional[ArgumentParser] = None):
        """Generate list function of this class."""
        _LOG.info("Generate LIST method for %r / %r", self.name, self.full_name)

        method_info = CrdMethodInfo(
            channel,
            self.full_name,
            *self._extract_method_info(channel, self.full_name, "List"),
        )

        self.configure_parser("list", parser)
        list_func_signature = Signature(
            [
                Parameter("self", Parameter.POSITIONAL_OR_KEYWORD),
                Parameter("namespace", Parameter.POSITIONAL_OR_KEYWORD),
                Parameter("limit", Parameter.POSITIONAL_OR_KEYWORD, default=100),
            ]
        )

        bound_func = partial(list_func_impl, method_info)
        bound_func = bind_signature(list_func_signature)(bound_func)
        self.list = MethodType(bound_func, self)
        _LOG.debug("Generated LIST injected well: %r", self.list)

    def read_yaml_and_update_crd_request(
        self, input_class: type[Message], yaml_path_string: str, original_crd: Message
    ) -> Message:
        """Read a YAML file and update the original CRD request instance."""
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


def inject_func_signature(crd: CRD, function_name: str, signatures: dict) -> None:
    """Utility function for injecting function signature for plugin command."""
    _LOG.debug(
        "Add or Overwrite function signature action %r: %r",
        function_name,
        crd.func_signature.get(function_name, {}),
    )
    crd.func_signature[function_name] = deepcopy(signatures)
    _LOG.debug(
        "Added function signature for action and argparser: %r",
        crd.func_signature[function_name],
    )
