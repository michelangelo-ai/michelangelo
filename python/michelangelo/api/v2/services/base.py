import atexit
import json
import os
from abc import ABC, abstractmethod
from typing import Dict

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
    """HeaderProvider appends or updates gRPC request headers before each gRPC call

    A custom HeaderProvider can be used to add additional headers for authentication,
    tracing, etc.
    """

    @abstractmethod
    def get_headers(self, request_headers: Dict[str, str] = None):
        """Returns updated headers in a Dict[str, str]

        :param request_headers: the original headers (e.g. specified when calling the
            service method)
        """
        pass


class DefaultHeaderProvider(HeaderProvider):
    def __init__(self):
        self._caller = None

    @property
    def caller(self):
        if self._caller:
            return self._caller
        raise ValueError("caller is not set")

    @caller.setter
    def caller(self, caller):
        self._caller = caller

    @property
    def service(self):
        if os.environ.get(_API_SERVICE_ENV_VAR):
            return os.environ.get(_API_SERVICE_ENV_VAR)
        else:
            return _DEFAULT_MA_API_SERVER_NAME

    def get_headers(self, request_headers=None):
        headers = request_headers or {}
        headers[_HEADER_RPC_ENCODING] = "proto"

        if _HEADER_RPC_SERVICE not in headers:
            headers[_HEADER_RPC_SERVICE] = self.service

        if _HEADER_RPC_CALLER not in headers:
            headers[_HEADER_RPC_CALLER] = self.caller

        return headers


class Context:
    def __init__(self):
        self._channel = None
        self._header_provider = None

    @property
    def channel(self):
        if not self._channel:
            self._channel = self._get_default_channel()
        return self._channel

    @channel.setter
    def channel(self, channel):
        self._channel = channel

    @property
    def header_provider(self):
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
                    f"Invalid server address format in '{_MA_API_SERVER_ENV}'. Expected format: 'IP:PORT'"
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
    def __init__(self, context, stub_clz):
        self._context = context
        self._service_stub = None
        self._stub_clz = stub_clz

    @staticmethod
    def _process_message_or_dict(message_or_dict, clz):
        opts = clz()
        if message_or_dict is None:
            return opts
        elif isinstance(message_or_dict, dict):
            json_format.ParseDict(_keys_to_camel(message_or_dict), opts)
            return opts
        else:
            return message_or_dict

    def _get_metadata(self, headers):
        provider = self._context.header_provider
        headers = provider.get_headers(headers)

        metadata = []
        for k, v in headers.items():
            metadata.append((k, v))
        metadata = sorted(metadata, key=lambda x: x[0])
        return tuple(metadata)

    def _process_criterion_operation(self, operation):
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
        """Create stub lazily"""
        if not self._service_stub:
            self._service_stub = self._stub_clz(self._context.channel)
        return self._service_stub


def _keys_to_camel(d):
    res = {}

    def to_camel_case(snake_case):
        splits = snake_case.split("_")
        joined = "".join([s.title() for s in splits[1:]])
        return splits[0] + joined

    for key in d.keys():
        if isinstance(d[key], dict):
            res[to_camel_case(key)] = _keys_to_camel(d[key])
        else:
            res[to_camel_case(key)] = d[key]
    return res
