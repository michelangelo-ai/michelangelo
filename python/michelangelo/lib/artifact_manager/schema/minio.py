"""Configuration dataclass for MinioStorageBackend."""

from __future__ import annotations

from dataclasses import dataclass

from michelangelo.workflow.schema.exceptions import ConfigurationError


@dataclass
class MinioStorageConfig:
    """Configuration for :class:`MinioStorageBackend`.

    Attributes:
        endpoint: MinIO or S3-compatible server address without the scheme
            (e.g. ``"localhost:9000"`` or ``"s3.amazonaws.com"``).
        bucket: Target bucket name. Created automatically on first use if it
            does not exist.
        access_key: Access key ID (MinIO root user or AWS IAM key ID).
        secret_key: Secret access key matching ``access_key``.
        secure: Use TLS for the connection. Set ``False`` for a local sandbox
            MinIO server (plaintext), ``True`` for production or any
            TLS-protected endpoint.
        region: AWS region string (e.g. ``"us-east-1"``). Required when
            connecting to AWS S3. Leave ``None`` for plain MinIO installations.

    Raises:
        ConfigurationError: If ``endpoint`` or ``bucket`` is empty.

    Example::

        config = MinioStorageConfig(
            endpoint="localhost:9000",
            bucket="michelangelo-models",
            access_key="minioadmin",
            secret_key="minioadmin",
        )
    """

    endpoint: str
    bucket: str
    access_key: str
    secret_key: str
    secure: bool = False
    region: str | None = None

    def __post_init__(self) -> None:
        """Validate required fields."""
        if not self.endpoint:
            raise ConfigurationError(
                "MinioStorageConfig.endpoint must be non-empty. "
                "Provide the server address, e.g. 'localhost:9000'."
            )
        if not self.bucket:
            raise ConfigurationError(
                "MinioStorageConfig.bucket must be non-empty. "
                "Provide the target bucket name."
            )
