"""Shared storage-backend resolution for the ``train`` and ``push_step`` tasks.

Both tasks in this workflow select a storage backend the same way:
``AWS_ENDPOINT_URL`` set -> MinIO/S3-compatible remote storage; unset -> a
local temp directory (development and CI). ``train`` and ``push_step`` run
in separate Ray/Spark worker processes and can't share a live config object
at runtime, so this module only shares the *resolution logic*, not a shared
instance -- each task calls ``resolve_storage_backend()`` independently.
"""

from __future__ import annotations

import logging
import os
import tempfile
from typing import TYPE_CHECKING
from urllib.parse import urlparse

if TYPE_CHECKING:
    from michelangelo.lib.artifact_manager.storage_backend import StorageBackend

log = logging.getLogger(__name__)

__all__ = ["resolve_storage_backend"]


def resolve_storage_backend(tmp_prefix: str) -> tuple[StorageBackend, bool]:
    """Select MinIO (remote) or a local temp directory.

    Args:
        tmp_prefix: Prefix for the local temp directory, used only when
            ``AWS_ENDPOINT_URL`` is unset. Lets each caller keep its own
            recognizable temp dir (e.g. ``"..._train_"`` vs ``"..._push_"``)
            when inspecting local runs.

    Returns:
        A ``(storage_backend, is_remote)`` tuple. ``is_remote`` is True when
        a MinIO/S3-compatible backend was selected, so callers that need
        remote-vs-local sink types (e.g. ``S3Sink`` vs ``LocalFileSink``)
        don't have to re-check ``AWS_ENDPOINT_URL`` themselves.
    """
    s3_endpoint = os.environ.get("AWS_ENDPOINT_URL", "")
    if s3_endpoint:
        parsed = urlparse(s3_endpoint)
        endpoint = parsed.netloc
        if not endpoint:
            raise ValueError(
                f"AWS_ENDPOINT_URL={s3_endpoint!r} is missing a scheme. "
                "Use a full URL like http://minio:9091"
            )
        bucket = (
            os.environ.get("AWS_S3_BUCKET")
            or (
                os.environ.get("MA_FILE_SYSTEM")
                or os.environ.get("UF_STORAGE_URL", "s3://default")
            )
            .removeprefix("s3://")
            .split("/")[0]
        )
        if not bucket:
            raise OSError(
                "Could not determine storage bucket. "
                "Set AWS_S3_BUCKET or MA_FILE_SYSTEM."
            )
        from michelangelo.lib.artifact_manager.minio_backend import MinioStorageBackend

        storage_backend = MinioStorageBackend(
            endpoint=endpoint,
            bucket=bucket,
            access_key=os.environ.get("AWS_ACCESS_KEY_ID", ""),
            secret_key=os.environ.get("AWS_SECRET_ACCESS_KEY", ""),
            secure=parsed.scheme == "https",
            create_bucket_if_missing=True,
        )
        log.info(
            "using MinioStorageBackend (remote) -> %s",
            storage_backend.get_storage_location(),
        )
        return storage_backend, True

    from michelangelo.lib.artifact_manager.storage_backend import LocalStorageBackend

    local_dir = tempfile.mkdtemp(prefix=tmp_prefix)
    storage_backend = LocalStorageBackend(local_dir)
    log.info("using LocalStorageBackend (local/CI) -> %s", local_dir)
    return storage_backend, False
