"""Artifact manager library for Michelangelo.

Provides storage backend abstractions for uploading and downloading
model artifacts across different infrastructure backends.

Public API::

    from michelangelo.lib.artifact_manager import (
        StorageBackend,
        LocalStorageBackend,
        MinioStorageBackend,
        MinioStorageConfig,
    )
"""

# flake8: noqa:F401
from michelangelo.lib.artifact_manager.minio_backend import MinioStorageBackend
from michelangelo.lib.artifact_manager.schema.minio import MinioStorageConfig
from michelangelo.lib.artifact_manager.storage_backend import (
    LocalStorageBackend,
    StorageBackend,
)
