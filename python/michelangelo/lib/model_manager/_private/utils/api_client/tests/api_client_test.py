from unittest import TestCase
from michelangelo.lib.model_manager._private.utils.api_client import APIClient


class APIClientTest(TestCase):
    def test_api_client(self):
        self.assertIsNotNone(APIClient._context.header_provider._caller)
