"""Artifact manager library for Michelangelo.

Provides storage backend abstractions for uploading and downloading
model artifacts across different infrastructure backends.

Public API::

    from michelangelo.lib.artifact_manager import (
        StorageBackend,
        LocalStorageBackend,
        MinioStorageBackend,
    )

Production configuration
------------------------

- ``secure`` defaults to ``True`` (TLS). Pass ``secure=False`` **only** for
  local sandbox servers where TLS is not configured — never in production.
- ``create_bucket_if_missing`` defaults to ``False``. Enable it only for
  sandbox / dev environments where the bucket may not exist yet. Most
  production IAM policies do not grant ``s3:CreateBucket``.
"""

# flake8: noqa:F401
from michelangelo.lib.artifact_manager.minio_backend import MinioStorageBackend
from michelangelo.lib.artifact_manager.storage_backend import (
    LocalStorageBackend,
    StorageBackend,
)
