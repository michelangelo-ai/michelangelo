"""APISinkConfig — config for the gRPC EvaluationReportService sink."""

from __future__ import annotations

from dataclasses import dataclass

from michelangelo.workflow.schema.exceptions import ConfigurationError

__all__ = ["APISinkConfig"]


@dataclass
class APISinkConfig:
    """Configuration for ``APISink``.

    Connects to any server that implements the ``EvaluationReportService``
    gRPC interface (local sandbox, community deployments, provider API servers).

    Attributes:
        endpoint: gRPC server address, e.g. ``"localhost:50051"`` for a local
            sandbox or ``"api.michelangelo.io:443"`` for a remote server.
        namespace: Value injected into ``report.metadata.namespace`` before the
            create call. Leave empty to preserve whatever the caller set on the
            proto.
        insecure: When ``True`` (default) an insecure plaintext channel is used
            — suitable for local sandbox and development. Set to ``False`` for
            TLS-encrypted connections to remote servers.
        timeout_seconds: Per-call deadline in seconds.

    Raises:
        ConfigurationError: If ``endpoint`` is empty.

    Example:
        >>> # Local sandbox
        >>> cfg = APISinkConfig(endpoint="localhost:50051")
        >>> # Remote TLS server
        >>> cfg = APISinkConfig(
        ...     endpoint="api.michelangelo.io:443",
        ...     namespace="ml-prod",
        ...     insecure=False,
        ... )
    """

    endpoint: str
    namespace: str = ""
    insecure: bool = True
    timeout_seconds: int = 30

    def __post_init__(self) -> None:
        """Validate endpoint is non-empty."""
        if not self.endpoint:
            raise ConfigurationError(
                "APISinkConfig.endpoint must be a non-empty string, "
                "e.g. 'localhost:50051'."
            )
