"""gRPC Tools for dynamic service introspection and method retrieval
using gRPC reflection.
"""

from logging import getLogger
from typing import Union

from google.protobuf import message_factory
from google.protobuf.descriptor_pb2 import (
    FileDescriptorProto,
    MethodDescriptorProto,
)
from google.protobuf.descriptor_pool import DescriptorPool
from google.protobuf.message import Message
from grpc import Channel
from grpc_reflection.v1alpha import reflection_pb2, reflection_pb2_grpc

_LOG = getLogger(__name__)


def list_services(channel, metadata: list) -> list[str]:
    """List the services available on a gRPC server using reflection.
    """
    stub = reflection_pb2_grpc.ServerReflectionStub(channel)

    request = reflection_pb2.ServerReflectionRequest(list_services="")
    responses = stub.ServerReflectionInfo(
        iter([request]),
        metadata=metadata,
    )
    _LOG.info("Got response from ServerReflection: %r", responses)

    for response in responses:
        services = response.list_services_response.service
        return [s.name for s in services]
    raise ValueError("No services found")


def get_methods_from_service(
    channel: Channel,
    service: str,
    metadata: list,
) -> tuple[dict[str, MethodDescriptorProto], DescriptorPool]:
    """Get methods from a service descriptor.
    """
    methods = list(get_service_descriptors(channel, service, metadata))
    _LOG.info("Succeed to retireve %d methods", len(methods))
    _LOG.debug("Detailed Method information: %r", methods)
    if not methods:
        _LOG.error("No methods found for service: %s", service)
        raise ValueError(f"No methods found for service: {service}")

    _LOG.info("Method list for service: %r", [m.name for m in methods])
    # Assume only one method matching
    pool = retrieve_full_descriptor_pool(channel, methods[0].name, metadata)
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


def get_service_descriptors(
    channel, service_name, metadata: list
) -> list[FileDescriptorProto]:
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
    responses = stub.ServerReflectionInfo(iter([request]), metadata=metadata)

    for response in responses:
        file_desc_proto = response.file_descriptor_response.file_descriptor_proto
        for fd_bytes in file_desc_proto:
            fd = FileDescriptorProto()
            fd.ParseFromString(fd_bytes)
            yield fd


def retrieve_full_descriptor_pool(
    channel: Channel, filename: str, metadata: list
) -> DescriptorPool:
    """Retrieve the full descriptor pool for a given filename.
    This is useful for introspection and understanding the service's API.
    """
    _LOG.info("Start retrieve full descriptor pool for %r / %r", channel, filename)
    fds = list(get_all_file_descriptors_by_filename(channel, filename, metadata))
    # print([type(f) for f in fds])
    pool = DescriptorPool()
    for fd in fds:
        pool.Add(fd)

    _LOG.info("Retrieved %d file descriptors", len(fds))
    return pool


def get_all_file_descriptors_by_filename(
    channel: Channel,
    filename: str,
    metadata: list,
    deps: int = 0,
    visited: Union[None, set[str]] = None,
):
    """Get all file descriptors recursively by filename using ServerReflection.
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
    responses = stub.ServerReflectionInfo(iter([request]), metadata=metadata)
    for response in responses:
        for fd_bytes in response.file_descriptor_response.file_descriptor_proto:
            fd = FileDescriptorProto()
            fd.ParseFromString(fd_bytes)
            _LOG.debug("Deps: %r", fd.dependency)
            for dependency in fd.dependency:
                yield from get_all_file_descriptors_by_filename(
                    channel, dependency, metadata, deps + 1, visited
                )
            yield fd


def get_message_class_by_name(pool: DescriptorPool, message_name: str) -> type[Message]:
    """message_name example: "michelangelo.api.v2beta1.Pipeline"
    """
    descriptor = pool.FindMessageTypeByName(message_name)
    # factory = message_factory.MessageFactory(pool)
    # MessageClass = factory.GetPrototype(descriptor)
    MessageClass = message_factory.GetMessageClass(descriptor)
    return MessageClass
