"""Michelangelo API v2 client implementation.

This module provides the main APIClient class for interacting with
Michelangelo services. The client is built on top of generated service
stubs and provides a high-level interface for all API operations.
"""

from .services.base import Context
from .services.gen import ServicesGen


class APIClient(ServicesGen):
    """Python client of Michelangelo 2.0 API.

    Example:
        >>> from michelangelo.api.v2 import APIClient
        >>> APIClient.set_caller('my-client')
        >>> model = APIClient.ModelService.get_model(
        ...     namespace='my-project', name='my-model'
        ... )
    """

    _context = Context()
    ServicesGen.init(_context)

    @classmethod
    def set_channel(cls, channel):
        """Set gRPC channel for the client.

        Args:
            channel: Custom gRPC channel to use. If not set, a default channel is used.
        """
        cls._context.channel = channel

    @classmethod
    def set_header_provider(cls, provider):
        """Set custom header provider for gRPC requests.

        Args:
            provider: HeaderProvider instance for managing request headers.
                If not set, DefaultHeaderProvider is used.
        """
        cls._context.header_provider = provider

    @classmethod
    def set_caller(cls, caller):
        """Set caller name for API requests.

        Args:
            caller: Name of the calling application or service.
                Passed to server as "Rpc-Caller" header.
        """
        cls._context.header_provider._caller = caller
