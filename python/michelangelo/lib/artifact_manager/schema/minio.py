"""Configuration dataclass for MinioStorageBackend."""

from __future__ import annotations

from dataclasses import dataclass

from michelangelo.lib.exceptions import ConfigurationError


@dataclass
class MinioStorageConfig:
    """Configuration for :class:`MinioStorageBackend`.

    Attributes:
        endpoint: MinIO or S3-compatible server address without the scheme
            (e.g. ``"localhost:9000"`` or ``"s3.amazonaws.com"``).
        bucket: Target bucket name.
        access_key: Access key ID (MinIO root user or AWS IAM key ID).
        secret_key: Secret access key matching ``access_key``.
        secure: Use TLS for the connection. Defaults to ``True`` (matches the
            MinIO SDK default). Set ``False`` only for a local sandbox server
            where TLS is not configured.
        region: AWS region string (e.g. ``"us-east-1"``). Required when
            connecting to AWS S3. Leave ``None`` for plain MinIO installations.
        create_bucket_if_missing: When ``True``, :class:`MinioStorageBackend`
            calls ``make_bucket`` on initialisation if the configured bucket
            does not exist. Defaults to ``False`` — useful when the IAM policy
            does not grant ``s3:CreateBucket`` (most production environments).

    Raises:
        ConfigurationError: If ``endpoint`` or ``bucket`` is empty.

    Example::

        # Production (TLS enabled, bucket pre-created by infra)
        config = MinioStorageConfig(
            endpoint="minio.prod.example.com:443",
            bucket="michelangelo-models",
            access_key=os.environ["MINIO_ACCESS_KEY"],
            secret_key=os.environ["MINIO_SECRET_KEY"],
        )

        # Local sandbox (plaintext, auto-create bucket)
        config = MinioStorageConfig(
            endpoint="localhost:9000",
            bucket="michelangelo-models",
            access_key="minioadmin",
            secret_key="minioadmin",
            secure=False,            # local dev only — do not use in production
            create_bucket_if_missing=True,
        )
    """

    endpoint: str
    bucket: str
    access_key: str
    secret_key: str
    secure: bool = True
    region: str | None = None
    create_bucket_if_missing: bool = False

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
