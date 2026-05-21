"""Artifact manager library for Michelangelo.

Provides storage backend abstractions for uploading and downloading
model artifacts across different infrastructure backends.

Public API::

    from michelangelo.lib.artifact_manager import (
        StorageBackend,
        LocalStorageBackend,
    )
"""

# flake8: noqa:F401
from michelangelo.lib.artifact_manager.storage_backend import (
    LocalStorageBackend,
    StorageBackend,
)
