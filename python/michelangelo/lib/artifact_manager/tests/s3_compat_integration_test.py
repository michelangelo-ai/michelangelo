"""Live S3-compatibility tests for the artifact storage backends.

Skipped unless ``MA_TEST_S3_ENDPOINT`` is set. Run against any S3-compatible
server, for example a local SeaweedFS or MinIO::

    export MA_TEST_S3_ENDPOINT=localhost:9091
    export MA_TEST_S3_ACCESS_KEY=... MA_TEST_S3_SECRET_KEY=...
    pytest -m integration michelangelo/lib/artifact_manager/tests/

Optional: ``MA_TEST_S3_SECURE=1`` for HTTPS, ``MA_TEST_S3_LARGE=1`` to add
the 110 MiB multipart upload.
"""

from __future__ import annotations

import multiprocessing
import os
import sys
import threading
import uuid
import warnings
from typing import Callable

import pytest

from michelangelo.lib.artifact_manager.fsspec_backend import (
    FsspecStorageBackend,
    s3_storage_options,
)
from michelangelo.lib.artifact_manager.minio_backend import MinioStorageBackend

ENDPOINT = os.environ.get("MA_TEST_S3_ENDPOINT", "")
ACCESS_KEY = os.environ.get("MA_TEST_S3_ACCESS_KEY", "")
SECRET_KEY = os.environ.get("MA_TEST_S3_SECRET_KEY", "")
SECURE = os.environ.get("MA_TEST_S3_SECURE", "") == "1"

pytestmark = [
    pytest.mark.integration,
    pytest.mark.skipif(not ENDPOINT, reason="MA_TEST_S3_ENDPOINT is not set"),
]

BackendFactory = Callable[..., FsspecStorageBackend]


def _fsspec_factory(bucket: str, create: bool = True, secret: str | None = None):
    return FsspecStorageBackend(
        f"s3://{bucket}",
        s3_storage_options(
            ENDPOINT,
            access_key=ACCESS_KEY,
            secret_key=secret or SECRET_KEY,
            secure=SECURE,
        ),
        create_bucket_if_missing=create,
    )


def _minio_shim_factory(bucket: str, create: bool = True, secret: str | None = None):
    with warnings.catch_warnings():
        warnings.simplefilter("ignore", DeprecationWarning)
        return MinioStorageBackend(
            endpoint=ENDPOINT,
            bucket=bucket,
            access_key=ACCESS_KEY,
            secret_key=secret or SECRET_KEY,
            secure=SECURE,
            create_bucket_if_missing=create,
        )


@pytest.fixture(params=[_fsspec_factory, _minio_shim_factory], ids=["fsspec", "shim"])
def factory(request) -> BackendFactory:
    """Factory."""
    return request.param


def _cleanup_fs():
    return FsspecStorageBackend(
        "s3://cleanup",
        s3_storage_options(
            ENDPOINT, access_key=ACCESS_KEY, secret_key=SECRET_KEY, secure=SECURE
        ),
    )._fs


@pytest.fixture
def bucket():
    """Unique bucket name; the bucket and its ``-race`` twin are removed after."""
    name = f"ma-compat-{uuid.uuid4().hex[:12]}"
    yield name
    fs = _cleanup_fs()
    for b in (name, name + "-race"):
        try:
            if fs.exists(b):
                fs.rm(b, recursive=True)
        except Exception as exc:  # best effort: never fail a test on cleanup
            warnings.warn(f"could not remove test bucket {b!r}: {exc}", stacklevel=2)


@pytest.fixture
def model_dir(tmp_path):
    """Model dir."""
    src = tmp_path / "model"
    (src / "sub").mkdir(parents=True)
    (src / "weights.bin").write_bytes(os.urandom(4096))
    (src / "sub" / "config.json").write_text('{"layers": 2}')
    return src


def _tree(path) -> dict[str, bytes]:
    out = {}
    for root, _dirs, files in os.walk(path):
        for name in files:
            full = os.path.join(root, name)
            with open(full, "rb") as f:
                out[os.path.relpath(full, path).replace(os.sep, "/")] = f.read()
    return out


def test_file_round_trip(factory, bucket, tmp_path):
    """File round trip."""
    backend = factory(bucket)
    src = tmp_path / "w.pt"
    src.write_bytes(os.urandom(1024))

    uri = backend.upload(str(src), "models/clf/v1/raw")
    backend.download(uri, str(tmp_path / "back.pt"))

    assert uri == f"s3://{bucket}/models/clf/v1/raw"
    assert (tmp_path / "back.pt").read_bytes() == src.read_bytes()


def test_directory_round_trip_and_overwrite_without_nesting(
    factory, bucket, model_dir, tmp_path
):
    """Directory round trip and overwrite without nesting."""
    backend = factory(bucket)
    uri = backend.upload(str(model_dir), "models/clf/v1")
    (model_dir / "weights.bin").write_bytes(b"updated")
    backend.upload(str(model_dir), "models/clf/v1")

    backend.download(uri, str(tmp_path / "out"))

    assert _tree(tmp_path / "out") == _tree(model_dir)
    assert sorted(backend._fs.find(f"{bucket}/models/clf/v1/")) == [
        f"{bucket}/models/clf/v1/sub/config.json",
        f"{bucket}/models/clf/v1/weights.bin",
    ]


def test_empty_directory_and_missing_path(factory, bucket, tmp_path):
    """Empty directory and missing path."""
    backend = factory(bucket)
    (tmp_path / "empty").mkdir()

    uri = backend.upload(str(tmp_path / "empty"), "models/empty")

    with pytest.raises(FileNotFoundError, match="No object or directory found"):
        backend.download(uri, str(tmp_path / "out"))


def test_bucket_creation_is_idempotent_and_race_safe(factory, bucket):
    """Bucket creation is idempotent and race safe."""
    factory(bucket)
    factory(bucket)  # second construction sees the existing bucket

    race_bucket = bucket + "-race"
    errors: list[BaseException] = []

    def create():
        try:
            factory(race_bucket)
        except BaseException as exc:
            errors.append(exc)

    threads = [threading.Thread(target=create) for _ in range(6)]
    for t in threads:
        t.start()
    for t in threads:
        t.join()
    assert errors == []


def test_wrong_secret_raises_permission_error(factory, bucket, tmp_path):
    """Wrong secret raises permission error."""
    factory(bucket)
    backend = factory(bucket, create=False, secret="definitely-wrong")
    src = tmp_path / "f"
    src.write_text("x")

    with pytest.raises(PermissionError):
        backend.upload(str(src), "k")


def _child_upload(backend, path, key, queue):
    try:
        queue.put(backend.upload(path, key))
    except BaseException as exc:
        queue.put(repr(exc))


def test_backend_built_in_parent_uploads_from_child_process(
    factory, bucket, tmp_path, monkeypatch
):
    """Backend built in parent uploads from child process."""
    if sys.platform == "darwin":
        # macOS system proxy lookup crashes in forked children; unrelated to
        # the backend (Linux, where pipelines run, is not affected).
        monkeypatch.setenv("no_proxy", "*")
    backend = factory(bucket)
    src = tmp_path / "f"
    src.write_text("from child")
    backend.upload(str(src), "parent")  # start the I/O loop in the parent
    ctx = multiprocessing.get_context("fork")
    queue = ctx.Queue()

    proc = ctx.Process(target=_child_upload, args=(backend, str(src), "child", queue))
    proc.start()
    proc.join(60)
    if proc.is_alive():
        proc.kill()

    assert proc.exitcode == 0
    assert queue.get(timeout=5) == f"s3://{bucket}/child"
    backend.download(f"s3://{bucket}/child", str(tmp_path / "back"))
    assert (tmp_path / "back").read_text() == "from child"


@pytest.mark.skipif(
    os.environ.get("MA_TEST_S3_LARGE") != "1", reason="MA_TEST_S3_LARGE is not set"
)
def test_large_file_multipart_round_trip(factory, bucket, tmp_path):
    """Large file multipart round trip."""
    backend = factory(bucket)
    src = tmp_path / "big.bin"
    with open(src, "wb") as f:
        block = os.urandom(1 << 20)
        for _ in range(110):
            f.write(block)

    uri = backend.upload(str(src), "models/big.bin")
    backend.download(uri, str(tmp_path / "big.back"))

    assert (tmp_path / "big.back").stat().st_size == src.stat().st_size
    assert "-" in backend._fs.info(f"{bucket}/models/big.bin")["ETag"]  # multipart
