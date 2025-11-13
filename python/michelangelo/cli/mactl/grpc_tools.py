"""
"""
from grpc_reflection.v1alpha import reflection_pb2, reflection_pb2_grpc
from logging import getLogger


_LOG = getLogger(__name__)


def list_services(channel, metadata: list) -> list[str]:
    """
    List the services available on a gRPC server using reflection.
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
