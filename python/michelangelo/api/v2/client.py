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

from __future__ import annotations

import os

import grpc

from .services.base import (
    _CHANNEL_OPTIONS,
    _MA_API_SERVER_ENV,
    Context,
    DefaultHeaderProvider,
)
from .services.gen import ServicesGen

__all__ = ["APIClient"]


class APIClient(ServicesGen):
    """Michelangelo 2.0 API client.

    Can be used as a **class-level singleton** (existing usage, no breaking
    changes) or as a **per-instance client** for process isolation.

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
        model = client_a.ModelService.get_model(namespace="proj", name="clf")

        client_a.close()
        client_b.close()

    Use as a context manager to close the channel automatically::

        with APIClient(endpoint="localhost:50051", caller="my-trainer") as client:
            model = client.ModelService.get_model(namespace="proj", name="clf")

    To list all available services at runtime::

        [s for s in dir(APIClient) if s.endswith("Service")]

    Args:
        endpoint: gRPC server address as ``"host:port"``.  Required — use the
            class directly (``APIClient.ModelService``) for singleton access.
        caller: Caller name forwarded to the server as the ``rpc-caller``
            header.  Recommended for observability.
        channel: Pre-built ``grpc.Channel`` to use instead of creating one
            from ``endpoint``.  Mutually exclusive with ``endpoint``.
        credentials: ``grpc.ChannelCredentials`` for TLS.  When ``None``
            (default) a plaintext channel is created.  Pass
            ``grpc.ssl_channel_credentials()`` for standard TLS or supply
            custom credentials for mutual TLS / token-based auth.

            .. warning::
                The default (``credentials=None``) creates a **plaintext
                channel**.  All headers — including ``rpc-caller`` — travel
                unencrypted.  Always pass ``credentials`` for any non-localhost
                deployment.

        interceptors: List of ``grpc.ClientInterceptor`` instances applied to
            the channel.  Use for distributed tracing, custom auth, or
            logging.  Ignored when ``channel`` is provided (the injected
            channel already has its interceptors applied).
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
        credentials: grpc.ChannelCredentials | None = None,
        interceptors: list | None = None,
        header_provider=None,
    ) -> None:
        """Create a per-instance client with its own channel and service stubs.

        Args:
            endpoint: Server address as ``"host:port"``.  An insecure or TLS
                channel is created from this value.  Mutually exclusive with
                ``channel``.  Required — omitting both ``endpoint`` and
                ``channel`` raises ``ValueError``.
            caller: Caller name for the ``rpc-caller`` header.
            channel: Pre-built ``grpc.Channel``.  The caller is responsible for
                closing it.  Mutually exclusive with ``endpoint``.
            credentials: ``grpc.ChannelCredentials`` for TLS connections.
                When ``None`` a plaintext channel is opened (default).
                Pass ``grpc.ssl_channel_credentials()`` for TLS.
            interceptors: ``grpc.ClientInterceptor`` list wrapping the created
                channel.  Ignored when ``channel`` is injected.
            header_provider: Replaces ``DefaultHeaderProvider`` for this
                instance.

        Raises:
            ValueError: If both ``endpoint`` and ``channel`` are provided, or
                if neither is provided (use the class directly for singleton
                access).
        """
        if endpoint is not None and channel is not None:
            raise ValueError("Provide either 'endpoint' or 'channel', not both.")
        if endpoint is None and channel is None:
            raise ValueError(
                "Provide 'endpoint' or 'channel' to create a per-instance client. "
                "For singleton access use the class directly: APIClient.ModelService"
            )

        ctx = Context()

        if channel is not None:
            ctx.channel = channel
            self._channel_owned = False
        else:
            ch = (
                grpc.secure_channel(endpoint, credentials, options=_CHANNEL_OPTIONS)
                if credentials is not None
                else grpc.insecure_channel(endpoint, options=_CHANNEL_OPTIONS)
            )
            if interceptors:
                ch = grpc.intercept_channel(ch, *interceptors)
            ctx.channel = ch
            self._channel_owned = True

        if header_provider is not None:
            ctx.header_provider = header_provider

        if caller is not None:
            # Check the *class* for the caller descriptor to avoid triggering
            # DefaultHeaderProvider.caller's getter (which raises ValueError
            # when _caller is None before the setter has been called).
            if getattr(type(ctx.header_provider), "caller", None) is not None:
                ctx.header_provider.caller = caller
            else:
                raise TypeError(
                    f"Header provider {type(ctx.header_provider).__name__!r} has no "
                    "'caller' attribute. Set the caller directly on the provider."
                )

        self._context = ctx

        # Wire per-instance service stubs as instance attributes.
        # Instance attributes shadow the class-level ones from ServicesGen,
        # so self.ModelService returns the per-instance stub while
        # APIClient.ModelService still returns the singleton stub.
        ServicesGen.init_instance(self, ctx)

    def close(self) -> None:
        """Close the per-instance gRPC channel.

        Only closes channels created by this instance (i.e. when ``endpoint``
        was passed).  No-op when a pre-built ``channel`` was injected.
        """
        if self._channel_owned and self._context._channel is not None:
            self._context._channel.close()

    def __enter__(self) -> APIClient:
        """Return self to support use as a context manager."""
        return self

    def __exit__(self, *exc: object) -> None:
        """Close the channel on context-manager exit."""
        self.close()

    def __repr__(self) -> str:
        """Return a human-readable description useful in REPLs and logs."""
        ch = self._context._channel
        caller = (
            self._context._header_provider._caller
            if self._context._header_provider
            else None
        )
        return f"APIClient(channel={ch!r}, caller={caller!r})"

    # ------------------------------------------------------------------
    # Class-level singleton helpers (backward compat)
    # ------------------------------------------------------------------

    @classmethod
    def set_channel(cls, channel) -> None:
        """Set a custom gRPC channel for the class-level singleton.

        After calling this, call :meth:`init` to re-wire the singleton
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
        Always targets the *current* header provider, so it is safe to call
        after :meth:`set_header_provider`.

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

        Call this after :meth:`set_channel` to ensure the singleton service
        stubs use the new channel.  Idempotent — safe to call multiple times.

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
            ValueError: If ``MA_API_SERVER`` is not set or not ``host:port``.
        """
        server = os.getenv(_MA_API_SERVER_ENV)
        if not server:
            raise ValueError(
                f"Environment variable '{_MA_API_SERVER_ENV}' is not set. "
                "Set it to the Michelangelo API server address in 'host:port' format, "
                "e.g. 'localhost:50051'."
            )
        if ":" not in server:
            raise ValueError(
                f"Invalid value for '{_MA_API_SERVER_ENV}': {server!r}. "
                "Expected 'host:port' format, e.g. 'localhost:50051'."
            )

    @classmethod
    def from_env(cls, caller: str) -> APIClient:
        """Create a per-instance client from the ``MA_API_SERVER`` environment variable.

        Validates that ``MA_API_SERVER`` is set and correctly formatted, then
        creates and returns an isolated :class:`APIClient` instance using that
        endpoint.  Each call produces an independent client with its own channel.

        Args:
            caller: Caller name for the ``rpc-caller`` header.

        Returns:
            A new :class:`APIClient` instance connected to the server at
            ``MA_API_SERVER``.

        Raises:
            ValueError: If ``MA_API_SERVER`` is not set or malformed.

        Example::

            import os
            os.environ["MA_API_SERVER"] = "localhost:50051"

            from michelangelo.api.v2 import APIClient

            with APIClient.from_env("my-pipeline") as client:
                model = client.ModelService.get_model(
                    namespace="my-project", name="my-model"
                )
        """
        cls.validate_env()
        endpoint = os.environ[_MA_API_SERVER_ENV]
        return cls(endpoint=endpoint, caller=caller)
