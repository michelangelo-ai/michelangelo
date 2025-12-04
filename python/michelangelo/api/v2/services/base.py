"""Base classes and utilities for Michelangelo API service clients.

This module provides the foundational classes for gRPC-based communication with
Michelangelo services, including:
- HeaderProvider: Abstract base class for customizing gRPC headers
- DefaultHeaderProvider: Default implementation for standard header management
- Context: Manages gRPC channels and header providers
- BaseService: Base class for generated service clients

The module handles connection management, retry policies, and header injection
for all API service calls.
"""

import atexit
import json
import os
from abc import ABC, abstractmethod
from typing import Optional

import grpc
from google.protobuf import any_pb2, json_format
from google.protobuf.struct_pb2 import Value

from michelangelo.gen.api.list_pb2 import Criterion, CriterionOperation

_TIMEOUT_SECONDS = 60
_DEFAULT_SERVICE_CONFIG = {
    "methodConfig": [
        {
            "timeout": f"{_TIMEOUT_SECONDS}s",
            "retryPolicy": {
                "maxAttempts": 3,
                "initialBackoff": "0.1s",
                "maxBackoff": "10s",
                "backoffMultiplier": 2,
                "retryableStatusCodes": ["INTERNAL", "UNAVAILABLE", "UNKNOWN"],
            },
        }
    ]
}
_MAX_MESSAGE_LENGTH = 1 * 1024 * 1024 * 1024  # 1GB
_HEADER_RPC_ENCODING = "rpc-encoding"
_HEADER_RPC_SERVICE = "rpc-service"
_HEADER_RPC_CALLER = "rpc-caller"
# environment variable name of Michelangelo API server address in host:port format
_MA_API_SERVER_ENV = "MA_API_SERVER"
# environment variable name of the gRPC service name of Michelangelo API server
_API_SERVICE_ENV_VAR = "MA_API_SERVER_NAME"
_DEFAULT_MA_API_SERVER_NAME = "ma-apiserver"
_channel = None


class HeaderProvider(ABC):
    """HeaderProvider appends or updates gRPC request headers before each gRPC call.

    A custom HeaderProvider can be used to add additional headers for authentication,
    tracing, etc.
    """

    @abstractmethod
    def get_headers(self, request_headers: Optional[dict[str, str]] = None):
        """Returns updated headers for gRPC requests.

        Args:
            request_headers: The original headers (e.g., specified when calling
                the service method).

        Returns:
            Dictionary of updated headers.
        """
        pass


class DefaultHeaderProvider(HeaderProvider):
    """Default implementation of HeaderProvider for Michelangelo API.

    Manages standard gRPC headers including encoding, service name, and caller
    identification. Headers are automatically added to all gRPC requests.

    Attributes:
        _caller: Name of the calling application or service.
    """

    def __init__(self):
        """Initialize DefaultHeaderProvider with no caller set."""
        self._caller = None

    @property
    def caller(self):
        """Get the caller name.

        Returns:
            The configured caller name.

        Raises:
            ValueError: If caller is not set.
        """
        if self._caller:
            return self._caller
        raise ValueError("caller is not set")

    @caller.setter
    def caller(self, caller):
        """Set the caller name.

        Args:
            caller: Name of the calling application or service.
        """
        self._caller = caller

    @property
    def service(self):
        """Get the service name for gRPC routing.

        Returns:
            Service name from MA_API_SERVER_NAME environment variable,
            or default 'ma-apiserver' if not set.
        """
        if os.environ.get(_API_SERVICE_ENV_VAR):
            return os.environ.get(_API_SERVICE_ENV_VAR)
        else:
            return _DEFAULT_MA_API_SERVER_NAME

    def get_headers(self, request_headers=None):
        """Build complete headers for gRPC requests.

        Adds standard headers for encoding, service name, and caller identification.
        Existing headers in request_headers are preserved.

        Args:
            request_headers: Optional dictionary of existing headers.

        Returns:
            Complete headers dictionary with all required fields.
        """
        headers = request_headers or {}
        headers[_HEADER_RPC_ENCODING] = "proto"

        if _HEADER_RPC_SERVICE not in headers:
            headers[_HEADER_RPC_SERVICE] = self.service

        if _HEADER_RPC_CALLER not in headers:
            headers[_HEADER_RPC_CALLER] = self.caller

        return headers


class Context:
    """Manages gRPC channel and header provider for API clients.

    The Context class provides centralized management of gRPC connections
    and header injection for all service calls. It handles lazy initialization
    of both the gRPC channel and header provider.

    Attributes:
        _channel: The gRPC channel for API communication.
        _header_provider: Provider for generating request headers.
    """

    def __init__(self):
        """Initialize Context with no channel or header provider set."""
        self._channel = None
        self._header_provider = None

    @property
    def channel(self):
        """Get the gRPC channel for API communication.

        Lazily initializes a default channel if none is set.

        Returns:
            The configured gRPC channel.
        """
        if not self._channel:
            self._channel = self._get_default_channel()
        return self._channel

    @channel.setter
    def channel(self, channel):
        self._channel = channel

    @property
    def header_provider(self):
        """Get the header provider for gRPC requests.

        Lazily initializes a DefaultHeaderProvider if none is set.

        Returns:
            The configured HeaderProvider instance.
        """
        if not self._header_provider:
            self._header_provider = DefaultHeaderProvider()
        return self._header_provider

    @header_provider.setter
    def header_provider(self, provider):
        self._header_provider = provider

    @staticmethod
    def _get_default_channel():
        global _channel
        if _channel is None:
            server_address = os.getenv(_MA_API_SERVER_ENV)

            if not server_address:
                raise ValueError(
                    f"Environment variable '{_MA_API_SERVER_ENV}' is not set."
                )

            if ":" not in server_address:
                raise ValueError(
                    f"Invalid server address format in '{_MA_API_SERVER_ENV}'. "
                    f"Expected format: 'IP:PORT'"
                )

            channel = grpc.insecure_channel(
                server_address,
                options=[
                    ("grpc.service_config", json.dumps(_DEFAULT_SERVICE_CONFIG)),
                    ("grpc.max_send_message_length", _MAX_MESSAGE_LENGTH),
                    ("grpc.max_receive_message_length", _MAX_MESSAGE_LENGTH),
                ],
            )
            atexit.register(channel.close)
            _channel = channel
        return _channel


class BaseService:
    """Base class for all generated Michelangelo service clients.

    Provides common functionality for gRPC service stubs including context management,
    header injection, and protobuf message processing. All generated service classes
    inherit from this base.

    Attributes:
        _context: The Context instance managing channels and headers.
        _service_stub: The gRPC service stub instance.
        _stub_clz: The gRPC stub class to instantiate.
    """

    def __init__(self, context, stub_clz):
        """Initialize BaseService with context and stub class.

        Args:
            context: Context instance for managing gRPC connections.
            stub_clz: gRPC stub class to instantiate for this service.
        """
        self._context = context
        self._service_stub = None
        self._stub_clz = stub_clz

    @staticmethod
    def _process_message_or_dict(message_or_dict, clz):
        """Process input that can be either a protobuf message or dictionary.

        Args:
            message_or_dict: Either a protobuf message instance, dictionary, or None.
            clz: Protobuf message class to instantiate if input is a dictionary.

        Returns:
            A protobuf message instance. If input is None, returns empty
            instance. If input is a dict, returns populated instance.
            Otherwise returns input as-is.
        """
        opts = clz()
        if message_or_dict is None:
            return opts
        elif isinstance(message_or_dict, dict):
            json_format.ParseDict(_keys_to_camel(message_or_dict), opts)
            return opts
        else:
            return message_or_dict

    def _get_metadata(self, headers):
        """Build gRPC metadata from headers.

        Args:
            headers: Dictionary of header key-value pairs.

        Returns:
            Tuple of sorted (key, value) pairs suitable for gRPC metadata.
        """
        provider = self._context.header_provider
        headers = provider.get_headers(headers)

        metadata = []
        for k, v in headers.items():
            metadata.append((k, v))
        metadata = sorted(metadata, key=lambda x: x[0])
        return tuple(metadata)

    def _process_criterion_operation(self, operation):
        """Process criterion operation from dictionary or protobuf format.

        Args:
            operation: Either a CriterionOperation protobuf message or a dictionary
                containing criterion specifications.

        Returns:
            CriterionOperation protobuf message. If input is already a protobuf
            message, returns it unchanged. If input is a dict, converts to protobuf.
        """
        if isinstance(operation, dict):
            criterion = operation.get("criterion", [])
            criterion_list = []
            for i in range(len(criterion)):
                if isinstance(criterion[i]["match_value"], dict):
                    any_value = json_format.ParseDict(
                        criterion[i]["match_value"], Value()
                    )
                    value = any_pb2.Any()
                    value.Pack(any_value)
                else:
                    value = any_pb2.Any(value=criterion[i]["match_value"].encode())
                c = Criterion(
                    field_name=criterion[i]["field_name"],
                    match_value=value,
                    operator=criterion[i]["operator"],
                )
                criterion_list.append(c)

            operation = CriterionOperation(criterion=criterion_list)

        return operation

    @property
    def _stub(self):
        """Get the gRPC service stub.

        Lazily initializes the stub on first access.

        Returns:
            The initialized gRPC service stub instance.
        """
        if not self._service_stub:
            self._service_stub = self._stub_clz(self._context.channel)
        return self._service_stub


def _keys_to_camel(d):
    """Convert dictionary keys from snake_case to camelCase.

    Recursively processes nested dictionaries to ensure all keys follow
    camelCase convention required by protobuf JSON parsing.

    Args:
        d: Dictionary with snake_case keys.

    Returns:
        Dictionary with camelCase keys and same values. Nested dictionaries
        are also converted.
    """
    res = {}

    def to_camel_case(snake_case):
        splits = snake_case.split("_")
        joined = "".join([s.title() for s in splits[1:]])
        return splits[0] + joined

    for key in d:
        if isinstance(d[key], dict):
            res[to_camel_case(key)] = _keys_to_camel(d[key])
        else:
            res[to_camel_case(key)] = d[key]
    return res
