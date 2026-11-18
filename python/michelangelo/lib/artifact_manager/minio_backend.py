"""Deprecated: use :mod:`michelangelo.lib.artifact_manager.fsspec_backend`.

:class:`MinioStorageBackend` is kept so existing callers keep working. It is
now a thin subclass of
:class:`~michelangelo.lib.artifact_manager.fsspec_backend.FsspecStorageBackend`
on ``s3fs``, and no longer uses the ``minio`` package. It will be removed in a
future release.

Migrate with::

    from michelangelo.lib.artifact_manager.fsspec_backend import (
        FsspecStorageBackend,
        s3_storage_options,
    )

    # Before
    backend = MinioStorageBackend(
        endpoint="localhost:9091",
        bucket="my-bucket",
        access_key=key,
        secret_key=secret,
        secure=False,
        create_bucket_if_missing=True,
    )

    # After
    backend = FsspecStorageBackend(
        "s3://my-bucket",
        s3_storage_options(
            "localhost:9091", access_key=key, secret_key=secret, secure=False
        ),
        create_bucket_if_missing=True,
    )
"""

from __future__ import annotations

import logging
import threading
import warnings

from michelangelo.lib.artifact_manager.fsspec_backend import (
    FsspecStorageBackend,
    s3_storage_options,
)
from michelangelo.lib.exceptions import ConfigurationError

_logger = logging.getLogger(__name__)

__all__ = ["MinioStorageBackend"]

_DEPRECATION_MESSAGE = (
    "MinioStorageBackend is deprecated and will be removed in a future release. "
    "Use FsspecStorageBackend('s3://<bucket>', s3_storage_options(...)) from "
    "michelangelo.lib.artifact_manager.fsspec_backend instead."
)
_logged_once = False
_log_lock = threading.Lock()


def _log_deprecation_once() -> None:
    # DeprecationWarning is hidden by default outside __main__ and test
    # runners, so also log it once per process.
    global _logged_once
    with _log_lock:
        if _logged_once:
            return
        _logged_once = True
    _logger.warning(_DEPRECATION_MESSAGE)


class MinioStorageBackend(FsspecStorageBackend):
    """Deprecated S3-compatible backend; use :class:`FsspecStorageBackend`.

    Same constructor as before. The arguments are mapped with
    :func:`~michelangelo.lib.artifact_manager.fsspec_backend.s3_storage_options`
    onto ``FsspecStorageBackend("s3://<bucket>", ...)``. Needs ``s3fs``
    (``pip install 'michelangelo[s3]'``); the ``minio`` package is not used.

    The shim always uses path-style addressing (the ``s3_storage_options``
    default). The old ``minio`` client switched AWS and Aliyun endpoints to
    virtual-host addressing automatically; for those, migrate to
    ``FsspecStorageBackend`` with ``s3_storage_options(..., path_style=False)``.

    Args:
        endpoint: Server address without the scheme, e.g. ``"localhost:9091"``.
        bucket: Target bucket name.
        access_key: Access key ID. If empty, the default AWS credential chain
            is used, and anonymous access if that finds nothing.
        secret_key: Secret access key matching ``access_key``.
        secure: Use TLS. Defaults to ``True``. Set ``False`` only for a local
            sandbox server without TLS.
        region: Bucket region, or ``None``.
        create_bucket_if_missing: Create the bucket if absent. Defaults to
            ``False``.

    Raises:
        ConfigurationError: If ``endpoint`` or ``bucket`` is empty.
        ValueError: If ``endpoint`` contains a scheme or a path.
        ImportError: If ``s3fs`` is not installed.
        OSError: If ``create_bucket_if_missing`` is set and the bucket cannot
            be created or accessed.
    """

    def __init__(
        self,
        endpoint: str,
        bucket: str,
        access_key: str,
        secret_key: str,
        secure: bool = True,
        region: str | None = None,
        create_bucket_if_missing: bool = False,
    ) -> None:
        """Map the legacy arguments onto :class:`FsspecStorageBackend`."""
        if not endpoint:
            raise ConfigurationError(
                "MinioStorageBackend endpoint must be non-empty. "
                "Provide the server address, e.g. 'localhost:9091'."
            )
        if not bucket:
            raise ConfigurationError(
                "MinioStorageBackend bucket must be non-empty. "
                "Provide the target bucket name."
            )
        storage_options = s3_storage_options(
            endpoint,
            access_key=access_key,
            secret_key=secret_key,
            secure=secure,
            region=region,
        )
        warnings.warn(_DEPRECATION_MESSAGE, DeprecationWarning, stacklevel=2)
        _log_deprecation_once()
        super().__init__(
            f"s3://{bucket}",
            storage_options,
            create_bucket_if_missing=create_bucket_if_missing,
        )
