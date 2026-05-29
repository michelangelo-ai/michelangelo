"""Tests for APIClient backward compatibility and instance-based isolation.

Verifies that:
- The existing class-level singleton API (set_caller, set_channel,
  set_header_provider, class attribute service access) is unchanged.
- The new instance-based constructor produces isolated clients with their
  own service stubs that do not share state with the singleton or each other.
"""

from __future__ import annotations

import os
from unittest import TestCase
from unittest.mock import MagicMock, patch


class TestAPIClientClassLevelSingleton(TestCase):
    """Tests for the class-level singleton API."""

    def test_class_attributes_are_service_instances(self):
        """All service names are wired as class-level attributes at import time."""
        from michelangelo.api.v2 import APIClient

        for svc in [
            "CachedOutputService",
            "ModelService",
            "ModelFamilyService",
            "PipelineService",
            "PipelineRunService",
            "ProjectService",
            "RayClusterService",
            "RayJobService",
            "SparkJobService",
            "TriggerRunService",
        ]:
            self.assertIsNotNone(
                getattr(APIClient, svc, None),
                f"APIClient.{svc} should be wired at import time",
            )

    def test_set_caller_updates_singleton_context(self):
        """set_caller() sets the caller on the singleton header provider."""
        from michelangelo.api.v2 import APIClient

        original = APIClient._context.header_provider._caller
        try:
            APIClient.set_caller("test-backward-compat")
            self.assertEqual(
                APIClient._context.header_provider._caller, "test-backward-compat"
            )
        finally:
            APIClient._context.header_provider._caller = original

    def test_set_caller_after_set_header_provider(self):
        """set_caller() targets the current provider after set_header_provider()."""
        from michelangelo.api.v2 import APIClient

        mock_provider = MagicMock()
        mock_provider.caller = None
        original_provider = APIClient._context._header_provider
        try:
            APIClient.set_header_provider(mock_provider)
            APIClient.set_caller("new-caller")
            self.assertEqual(mock_provider.caller, "new-caller")
        finally:
            APIClient._context._header_provider = original_provider

    def test_set_channel_updates_singleton_context(self):
        """set_channel() replaces the channel on the singleton context."""
        from michelangelo.api.v2 import APIClient

        mock_channel = MagicMock()
        original = APIClient._context._channel
        try:
            APIClient.set_channel(mock_channel)
            self.assertIs(APIClient._context._channel, mock_channel)
        finally:
            APIClient._context._channel = original

    def test_set_header_provider_updates_singleton_context(self):
        """set_header_provider() replaces the provider on the singleton context."""
        from michelangelo.api.v2 import APIClient

        mock_provider = MagicMock()
        mock_provider.caller = "from-mock"
        original = APIClient._context._header_provider
        try:
            APIClient.set_header_provider(mock_provider)
            self.assertIs(APIClient._context._header_provider, mock_provider)
        finally:
            APIClient._context._header_provider = original

    def test_init_classmethod_rewires_singleton_stubs(self):
        """init() re-wires all singleton service stubs to the current context."""
        from michelangelo.api.v2 import APIClient

        APIClient.init()
        after = APIClient.ModelService
        self.assertIsNotNone(after)
        self.assertIs(after._context, APIClient._context)

    def test_validate_env_passes_with_valid_value(self):
        """validate_env() does not raise when MA_API_SERVER is valid."""
        from michelangelo.api.v2 import APIClient

        with patch.dict(os.environ, {"MA_API_SERVER": "localhost:50051"}):
            APIClient.validate_env()  # must not raise

    def test_validate_env_raises_when_unset(self):
        """validate_env() raises ValueError when MA_API_SERVER is absent."""
        from michelangelo.api.v2 import APIClient

        env = {k: v for k, v in os.environ.items() if k != "MA_API_SERVER"}
        with patch.dict(os.environ, env, clear=True), self.assertRaises(ValueError):
            APIClient.validate_env()

    def test_validate_env_raises_on_bad_format(self):
        """validate_env() raises ValueError when MA_API_SERVER lacks host:port."""
        from michelangelo.api.v2 import APIClient

        bad_env = {"MA_API_SERVER": "no-colon-here"}
        with patch.dict(os.environ, bad_env), self.assertRaises(ValueError):
            APIClient.validate_env()

    def test_from_env_returns_instance(self):
        """from_env() returns an APIClient instance, not the class."""
        from michelangelo.api.v2 import APIClient

        mock_channel = MagicMock()
        with (
            patch.dict(os.environ, {"MA_API_SERVER": "localhost:50051"}),
            patch("grpc.insecure_channel", return_value=mock_channel),
        ):
            client = APIClient.from_env("env-caller")
        self.assertIsInstance(client, APIClient)
        self.assertIsNot(client, APIClient)
        self.assertEqual(client._context.header_provider._caller, "env-caller")
        mock_channel.close()


class TestAPIClientInstanceIsolation(TestCase):
    """Per-instance clients must be isolated from the singleton and from each other."""

    def _make_client(self, caller="test-caller", **kwargs):
        """Build an APIClient instance with a mocked insecure channel."""
        from michelangelo.api.v2 import APIClient

        mock_channel = MagicMock()
        with patch("grpc.insecure_channel", return_value=mock_channel):
            client = APIClient(endpoint="localhost:50051", caller=caller, **kwargs)
        return client, mock_channel

    def test_instance_has_own_service_stubs(self):
        """Instance service stubs are different objects from the singleton stubs."""
        from michelangelo.api.v2 import APIClient

        client, _ = self._make_client()
        self.assertIsNot(client.ModelService, APIClient.ModelService)
        self.assertIsNot(client.PipelineService, APIClient.PipelineService)

    def test_instance_stubs_use_instance_context(self):
        """Instance service stubs reference the per-instance Context."""
        from michelangelo.api.v2 import APIClient

        client, _ = self._make_client()
        self.assertIs(client.ModelService._context, client._context)
        self.assertIsNot(client.ModelService._context, APIClient._context)

    def test_two_instances_do_not_share_context(self):
        """Two instances have fully independent contexts."""
        from michelangelo.api.v2 import APIClient

        with patch("grpc.insecure_channel", return_value=MagicMock()):
            client_a = APIClient(endpoint="server-a:50051", caller="a")
            client_b = APIClient(endpoint="server-b:50051", caller="b")

        self.assertIsNot(client_a._context, client_b._context)
        self.assertIsNot(client_a.ModelService, client_b.ModelService)
        self.assertEqual(client_a._context.header_provider._caller, "a")
        self.assertEqual(client_b._context.header_provider._caller, "b")

    def test_singleton_unaffected_by_instance_caller(self):
        """Setting a caller on an instance does not mutate the singleton."""
        from michelangelo.api.v2 import APIClient

        singleton_caller_before = APIClient._context.header_provider._caller
        self._make_client(caller="instance-only")
        self.assertEqual(
            APIClient._context.header_provider._caller, singleton_caller_before
        )

    def test_close_closes_owned_channel(self):
        """close() closes the channel when it was created by the instance."""
        client, mock_channel = self._make_client()
        client.close()
        mock_channel.close.assert_called_once()

    def test_close_is_noop_for_injected_channel(self):
        """close() does NOT close a channel that was injected by the caller."""
        from michelangelo.api.v2 import APIClient

        mock_channel = MagicMock()
        client = APIClient(channel=mock_channel, caller="injected")
        client.close()
        mock_channel.close.assert_not_called()

    def test_context_manager_closes_channel(self):
        """The context manager closes the owned channel on exit."""
        from michelangelo.api.v2 import APIClient

        mock_channel = MagicMock()
        with (
            patch("grpc.insecure_channel", return_value=mock_channel),
            APIClient(endpoint="localhost:50051", caller="ctx") as client,
        ):
            self.assertIsNotNone(client.ModelService)
        mock_channel.close.assert_called_once()

    def test_endpoint_and_channel_both_set_raises(self):
        """Providing both endpoint and channel raises ValueError."""
        from michelangelo.api.v2 import APIClient

        with self.assertRaises(ValueError):
            APIClient(endpoint="localhost:50051", channel=MagicMock())

    def test_no_endpoint_no_channel_raises(self):
        """Omitting both endpoint and channel raises ValueError with helpful message."""
        from michelangelo.api.v2 import APIClient

        with self.assertRaises(ValueError) as ctx:
            APIClient()
        self.assertIn("APIClient.ModelService", str(ctx.exception))

    def test_tls_channel_created_with_credentials(self):
        """Passing credentials= creates a secure_channel, not insecure_channel."""
        import grpc as _grpc

        from michelangelo.api.v2 import APIClient

        mock_channel = MagicMock()
        creds = _grpc.ssl_channel_credentials()
        with (
            patch("grpc.secure_channel", return_value=mock_channel) as mock_secure,
            patch("grpc.insecure_channel") as mock_insecure,
        ):
            client = APIClient(endpoint="secure.example.com:443", credentials=creds)
        mock_secure.assert_called_once()
        mock_insecure.assert_not_called()
        self.assertTrue(client._channel_owned)

    def test_interceptors_wrap_channel(self):
        """Providing interceptors= wraps the channel via grpc.intercept_channel."""
        from michelangelo.api.v2 import APIClient

        mock_channel = MagicMock()
        mock_intercepted = MagicMock()
        interceptor = MagicMock()

        with (
            patch("grpc.insecure_channel", return_value=mock_channel),
            patch("grpc.intercept_channel", return_value=mock_intercepted) as mock_ic,
        ):
            client = APIClient(endpoint="localhost:50051", interceptors=[interceptor])

        mock_ic.assert_called_once_with(mock_channel, interceptor)
        self.assertIs(client._context._channel, mock_intercepted)

    def test_repr_contains_caller(self):
        """__repr__ includes the caller name for debuggability."""
        client, _ = self._make_client(caller="debug-caller")
        r = repr(client)
        self.assertIn("debug-caller", r)
