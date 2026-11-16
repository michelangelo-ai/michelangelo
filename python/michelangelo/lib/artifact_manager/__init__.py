"""Artifact manager library for Michelangelo.

Provides storage backend abstractions for uploading and downloading
model artifacts across different infrastructure backends.

Public API::

    from michelangelo.lib.artifact_manager import (
        StorageBackend,
        LocalStorageBackend,
        FsspecStorageBackend,
        s3_storage_options,
    )

``FsspecStorageBackend`` serves ``s3://`` (``pip install 'michelangelo[s3]'``),
``gs://`` (``pip install 'michelangelo[gcs]'``) and any other fsspec protocol.
``MinioStorageBackend`` is still exported but deprecated; it is now a thin
wrapper around ``FsspecStorageBackend`` and no longer needs the ``minio``
package.

Production configuration
------------------------

- ``secure`` (in ``s3_storage_options``) defaults to ``True`` (TLS). Pass
  ``secure=False`` **only** for local sandbox servers where TLS is not
  configured, never in production.
- ``create_bucket_if_missing`` defaults to ``False``. Enable it only for
  sandbox / dev environments where the bucket may not exist yet. Most
  production IAM policies do not grant ``s3:CreateBucket``.
"""

# flake8: noqa:F401
# These imports do not import s3fs or gcsfs; fsspec loads them on first use.
from michelangelo.lib.artifact_manager.fsspec_backend import (
    FsspecStorageBackend,
    s3_storage_options,
)
from michelangelo.lib.artifact_manager.minio_backend import MinioStorageBackend
from michelangelo.lib.artifact_manager.storage_backend import (
    LocalStorageBackend,
    StorageBackend,
)
