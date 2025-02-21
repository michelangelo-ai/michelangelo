from .services.base import Context
from .services.gen import ServicesGen


class APIClient(ServicesGen):
    """
    Python client of Michelangelo 2.0 API.

    :example:

    >>> from michelangelo.api.v2 import APIClient
    >>> APIClient.set_caller('my-client')
    >>> model = APIClient.ModelService.get_model(namespace='my-project', name='my-model')
    """

    _context = Context()
    ServicesGen.init(_context)

    @classmethod
    def set_channel(cls, channel):
        """
        Set grpc channel for the client. Use the default channel if not set.
        """
        cls._context.channel = channel

    @classmethod
    def set_header_provider(cls, provider):
        """
        Set header provider. Use the default header provider if not set.
        """
        cls._context.header_provider = provider

    @classmethod
    def set_caller(cls, caller):
        """
        Set caller name. This will be passed to the server as the "Rpc-Caller" header.
        """
        cls._context.header_provider._caller = caller
