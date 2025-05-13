from unittest import TestCase
from uber.ai.michelangelo.sdk.model_manager._private.utils.api_client import APIClient


class APIClientTest(TestCase):
    def test_api_client(self):
        self.assertEqual(APIClient._context.header_provider._caller, "ml-code")
