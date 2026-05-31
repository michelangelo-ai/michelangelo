"""Configuration dataclass for APIRegistryClient."""

from __future__ import annotations

from dataclasses import dataclass

from michelangelo.workflow.schema.exceptions import ConfigurationError


@dataclass
class APIRegistryConfig:
    """Configuration for :class:`APIRegistryClient`.

    Attributes:
        endpoint: gRPC server address without the scheme
            (e.g. ``"localhost:50051"`` or ``"api.michelangelo.io:443"``).
        namespace: Kubernetes namespace used for model resources. Injected into
            ``model.metadata.namespace`` when non-empty. Leave empty to use the
            server's default namespace.
        insecure: Use a plaintext gRPC channel (no TLS). Set ``True`` for a
            local sandbox API server, ``False`` for any TLS-protected endpoint.
        timeout_seconds: Per-call deadline in seconds. Applied to every gRPC
            call made by this client.

    Raises:
        ConfigurationError: If ``endpoint`` is empty.

    Example::

        config = APIRegistryConfig(
            endpoint="localhost:50051",
            namespace="sandbox",
            insecure=True,
        )
    """

    endpoint: str
    namespace: str = ""
    insecure: bool = True
    timeout_seconds: int = 30

    def __post_init__(self) -> None:
        """Validate required fields."""
        if not self.endpoint:
            raise ConfigurationError(
                "APIRegistryConfig.endpoint must be non-empty. "
                "Provide the gRPC server address, e.g. 'localhost:50051'."
            )
