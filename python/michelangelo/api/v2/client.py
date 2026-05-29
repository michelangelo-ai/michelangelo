"""Michelangelo API v2 client implementation.

``APIClient`` can be used in two modes:

**Class-level singleton** (existing behaviour, backward compatible)::

    import os
    os.environ["MA_API_SERVER"] = "localhost:50051"

    from michelangelo.api.v2 import APIClient

    APIClient.set_caller("my-pipeline")
    model = APIClient.ModelService.get_model(namespace="my-project", name="my-model")

**Per-instance** (isolated channel and caller per client)::

    from michelangelo.api.v2 import APIClient

    client = APIClient(endpoint="localhost:50051", caller="my-pipeline")
    model = client.ModelService.get_model(namespace="my-project", name="my-model")
    client.close()

    # Or use as a context manager for automatic channel cleanup:
    with APIClient(endpoint="localhost:50051", caller="my-pipeline") as client:
        model = client.ModelService.get_model(namespace="my-project", name="my-model")

The per-instance mode allows multiple independent clients in the same process,
each with their own endpoint, caller name, and gRPC channel — eliminating the
race conditions and shared-state surprises of the singleton pattern.
"""

import json
import os

import grpc

from .services.base import (
    _DEFAULT_SERVICE_CONFIG,
    _MA_API_SERVER_ENV,
    _MAX_MESSAGE_LENGTH,
    Context,
    DefaultHeaderProvider,
)
from .services.gen import ServicesGen

__all__ = ["APIClient"]

_CHANNEL_OPTIONS = [
    ("grpc.service_config", json.dumps(_DEFAULT_SERVICE_CONFIG)),
    ("grpc.max_send_message_length", _MAX_MESSAGE_LENGTH),
    ("grpc.max_receive_message_length", _MAX_MESSAGE_LENGTH),
]


class APIClient(ServicesGen):
    """Michelangelo 2.0 API client.

    Can be used as a **class-level singleton** (existing usage, no breaking
    changes) or as an **instance** for per-process isolation.

    Singleton usage
    ---------------
    The class wires all service stubs at import time using the ``MA_API_SERVER``
    environment variable and a shared channel.  All class methods (``set_caller``,
    ``set_channel``, ``set_header_provider``) mutate shared state and affect
    every singleton user in the process::

        import os
        os.environ["MA_API_SERVER"] = "localhost:50051"

        from michelangelo.api.v2 import APIClient

        APIClient.set_caller("my-pipeline")
        model = APIClient.ModelService.get_model(
            namespace="my-project", name="my-model"
        )

    Instance usage
    --------------
    Construct ``APIClient`` with an explicit ``endpoint`` to get an isolated
    client with its own channel, caller name, and service stubs.  Two instances
    with different endpoints never share state::

        client_a = APIClient(endpoint="server-a:443", caller="pipeline-a")
        client_b = APIClient(endpoint="server-b:443", caller="pipeline-b")

        # Completely independent — different channels, different callers:
        report = client_a.EvaluationReportService.create_evaluation_report(r)

        client_a.close()
        client_b.close()

    Use as a context manager to close the channel automatically::

        with APIClient(endpoint="localhost:50051", caller="my-trainer") as client:
            client.ModelService.get_model(namespace="proj", name="clf")

    Available services
    ------------------
    ``CachedOutputService``, ``ModelService``, ``ModelFamilyService``,
    ``PipelineService``, ``PipelineRunService``, ``ProjectService``,
    ``RayClusterService``, ``RayJobService``, ``SparkJobService``,
    ``TriggerRunService``

    Args:
        endpoint: gRPC server address as ``"host:port"``.  When provided an
            isolated per-instance channel is created.  When omitted the
            class-level singleton channel (from ``MA_API_SERVER``) is used.
        caller: Caller name forwarded to the server as the ``rpc-caller``
            header.  Optional; sets the header on this instance only.
        channel: Pre-built ``grpc.Channel`` to use instead of creating one
            from ``endpoint``.  Mutually exclusive with ``endpoint``.
        insecure: When ``True`` (default) create a plaintext channel from
            ``endpoint``.  Set ``False`` for TLS.
        header_provider: Custom ``HeaderProvider`` for this instance.
    """

    # ------------------------------------------------------------------
    # Class-level singleton (backward compat)
    # ------------------------------------------------------------------
    _context = Context()
    ServicesGen.init(_context)

    def __init__(
        self,
        endpoint: str | None = None,
        *,
        caller: str | None = None,
        channel=None,
        insecure: bool = True,
        header_provider=None,
    ) -> None:
        """Create a per-instance client with its own channel and service stubs.

        Args:
            endpoint: Server address as ``"host:port"``.  An insecure or TLS
                channel is created from this value (controlled by ``insecure``).
                Mutually exclusive with ``channel``.
            caller: Caller name for the ``rpc-caller`` header.
            channel: Pre-built ``grpc.Channel``.  The caller is responsible for
                closing it.  Mutually exclusive with ``endpoint``.
            insecure: Create a plaintext channel when ``True`` (default).
                Ignored when ``channel`` is provided.
            header_provider: Replaces the default ``DefaultHeaderProvider`` for
                this instance.

        Raises:
            ValueError: If both ``endpoint`` and ``channel`` are provided.
        """
        if endpoint is not None and channel is not None:
            raise ValueError("Provide either 'endpoint' or 'channel', not both.")

        ctx = Context()

        if channel is not None:
            ctx.channel = channel
            self._channel_owned = False
        elif endpoint is not None:
            factory = (
                grpc.insecure_channel
                if insecure
                else (
                    lambda addr, **kw: grpc.secure_channel(
                        addr, grpc.ssl_channel_credentials(), **kw
                    )
                )
            )
            ctx.channel = factory(endpoint, options=_CHANNEL_OPTIONS)
            self._channel_owned = True
        else:
            # No endpoint or channel — share the singleton channel lazily.
            self._channel_owned = False

        if header_provider is not None:
            ctx.header_provider = header_provider

        if caller is not None:
            ctx.header_provider.caller = caller

        self._context = ctx

        # Wire per-instance service stubs as instance attributes.
        # Instance attributes shadow the class-level ones from ServicesGen,
        # so self.ModelService returns the per-instance stub while
        # APIClient.ModelService still returns the singleton stub.
        ServicesGen.init_instance(self, ctx)

    def close(self) -> None:
        """Close the per-instance gRPC channel.

        Only closes channels that were created by this instance (i.e. when
        ``endpoint`` was passed to the constructor).  No-op when a pre-built
        ``channel`` was injected or when using the no-arg constructor.
        """
        if self._channel_owned and self._context._channel is not None:
            self._context._channel.close()

    def __enter__(self) -> "APIClient":
        """Return self to support use as a context manager."""
        return self

    def __exit__(self, *exc: object) -> None:
        """Close the channel on context-manager exit."""
        self.close()

    # ------------------------------------------------------------------
    # Class-level singleton helpers (backward compat)
    # ------------------------------------------------------------------

    @classmethod
    def set_channel(cls, channel) -> None:
        """Set a custom gRPC channel for the class-level singleton.

        After calling this, call ``APIClient.init()`` to re-wire the singleton
        service stubs to the new channel.

        Args:
            channel: A ``grpc.Channel`` instance.
        """
        cls._context.channel = channel

    @classmethod
    def set_header_provider(cls, provider) -> None:
        """Replace the header provider for the class-level singleton.

        Args:
            provider: A ``HeaderProvider`` instance.
        """
        cls._context.header_provider = provider

    @classmethod
    def set_caller(cls, caller: str) -> None:
        """Set the caller name for the class-level singleton.

        The caller is forwarded to the server as the ``rpc-caller`` header.
        This method always targets the *current* header provider, so it is
        safe to call after ``set_header_provider()``.

        Args:
            caller: Stable, human-readable identifier for the calling service.
        """
        provider = cls._context.header_provider
        if isinstance(provider, DefaultHeaderProvider) or hasattr(provider, "caller"):
            provider.caller = caller
        else:
            raise TypeError(
                f"Header provider {type(provider).__name__!r} has no 'caller' "
                "attribute. Configure the caller directly on the provider."
            )

    @classmethod
    def init(cls) -> None:
        """Re-wire singleton service stubs to the current class-level context.

        Call this after ``set_channel()`` to ensure the singleton service stubs
        use the new channel.  Idempotent — safe to call multiple times.

        Example::

            import grpc
            from michelangelo.api.v2 import APIClient

            channel = grpc.insecure_channel("other-host:50051")
            APIClient.set_channel(channel)
            APIClient.init()
        """
        ServicesGen.init(cls._context)

    @classmethod
    def validate_env(cls) -> None:
        """Raise ``ValueError`` if ``MA_API_SERVER`` is missing or malformed.

        Use at application startup to surface misconfiguration before the
        first RPC rather than receiving an error mid-request.

        Raises:
            ValueError: If ``MA_API_SERVER`` is not set or is not ``host:port``.
        """
        server = os.getenv(_MA_API_SERVER_ENV)
        if not server:
            raise ValueError(
                f"Environment variable '{_MA_API_SERVER_ENV}' is not set. "
                "Set it to the Michelangelo API server address in 'host:port' format."
            )
        if ":" not in server:
            raise ValueError(
                f"Invalid value for '{_MA_API_SERVER_ENV}': {server!r}. "
                "Expected 'host:port' format, e.g. 'localhost:50051'."
            )

    @classmethod
    def from_env(cls, caller: str) -> type:
        """Validate the environment, set the caller, and return the class.

        Convenience entry point for singleton usage — validates ``MA_API_SERVER``
        is set before the first RPC, then sets the caller name.

        Args:
            caller: Caller name for the ``rpc-caller`` header.

        Returns:
            ``APIClient`` (the class) so callers can chain::

                APIClient.from_env("my-pipeline").ModelService.get_model(...)

        Raises:
            ValueError: If ``MA_API_SERVER`` is not set or malformed.

        Example::

            import os
            os.environ["MA_API_SERVER"] = "localhost:50051"

            from michelangelo.api.v2 import APIClient

            APIClient.from_env("my-pipeline")
            model = APIClient.ModelService.get_model(
                namespace="my-project", name="my-model"
            )
        """
        cls.validate_env()
        cls.set_caller(caller)
        return cls
