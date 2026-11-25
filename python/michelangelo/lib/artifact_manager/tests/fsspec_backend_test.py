"""Tests for FsspecStorageBackend and s3_storage_options."""

from __future__ import annotations

import errno
import os
import sys
import uuid
from unittest.mock import MagicMock

import fsspec
import pytest
from fsspec.asyn import AsyncFileSystem
from fsspec.implementations.memory import MemoryFileSystem

from michelangelo.lib.artifact_manager import fsspec_backend
from michelangelo.lib.artifact_manager.fsspec_backend import (
    FsspecStorageBackend,
    s3_storage_options,
)
from michelangelo.lib.exceptions import ConfigurationError

# ── Fixtures and fakes ──────────────────────────────────────────────────────


@pytest.fixture
def memory_root():
    """A unique memory:// root; MemoryFileSystem's store is process-global."""
    root = f"artifacts-{uuid.uuid4().hex}"
    yield root
    fs = MemoryFileSystem()
    if fs.exists(f"/{root}"):
        fs.rm(f"/{root}", recursive=True)


@pytest.fixture
def local_tree(tmp_path):
    """A local directory with a nested file layout."""
    src = tmp_path / "model"
    (src / "sub" / "deeper").mkdir(parents=True)
    (src / "weights.bin").write_bytes(b"\x00\x01weights")
    (src / "sub" / "config.json").write_text('{"a": 1}')
    (src / "sub" / "deeper" / "vocab.txt").write_text("hello")
    return src


def _tree(path) -> dict[str, bytes]:
    out = {}
    for root, _dirs, files in os.walk(path):
        for name in files:
            full = os.path.join(root, name)
            rel = os.path.relpath(full, path).replace(os.sep, "/")
            with open(full, "rb") as f:
                out[rel] = f.read()
    return out


class _FakeS3FileSystem(fsspec.AbstractFileSystem):
    """Fake ``s3://`` filesystem whose methods can be made to raise."""

    protocol = ("s3", "s3a")
    root_marker = ""
    cachable = False  # a fresh fake per test

    def __init__(self, **kwargs):
        super().__init__(**kwargs)
        self.calls: list[tuple] = []
        self.raise_on: dict[str, BaseException] = {}
        self.exists_result = True
        self.find_result: list[str] = []
        self.info_result: dict = {"type": "file", "size": 1}

    @classmethod
    def _strip_protocol(cls, path):
        for proto in ("s3://", "s3a://"):
            if path.startswith(proto):
                path = path[len(proto) :]
        return path.rstrip("/")

    def _maybe_raise(self, name, *args):
        self.calls.append((name, *args))
        if name in self.raise_on:
            raise self.raise_on[name]

    def mkdir(self, path, create_parents=True, **kwargs):
        self._maybe_raise("mkdir", path, create_parents)

    def exists(self, path, **kwargs):
        self._maybe_raise("exists", path)
        return self.exists_result

    def invalidate_cache(self, path=None):
        self.calls.append(("invalidate_cache", path))

    def info(self, path, **kwargs):
        self._maybe_raise("info", path)
        return self.info_result

    def find(self, path, maxdepth=None, withdirs=False, detail=False, **kwargs):
        self._maybe_raise("find", path)
        return self.find_result

    def put_file(self, lpath, rpath, **kwargs):
        self._maybe_raise("put_file", lpath, rpath)

    def put(self, lpath, rpath, **kwargs):
        self._maybe_raise("put", lpath, rpath, kwargs)

    def get_file(self, rpath, lpath, **kwargs):
        with open(lpath, "wb") as f:
            f.write(b"partial")
        self._maybe_raise("get_file", rpath, lpath)

    def get(self, rpath, lpath, **kwargs):
        self._maybe_raise("get", rpath, lpath, kwargs)
        for path in lpath:
            with open(path, "wb") as f:
                f.write(b"x")


def _s3_backend(fs=None, **kwargs) -> tuple[FsspecStorageBackend, _FakeS3FileSystem]:
    fs = fs or _FakeS3FileSystem()
    return FsspecStorageBackend("s3://bucket", filesystem=fs, **kwargs), fs


# ── Constructor validation and filesystem setup ─────────────────────────────


@pytest.mark.parametrize("base_uri", ["", "bucket", "s3://", "s3:///", "memory://"])
def test_init_rejects_malformed_base_uri(base_uri):
    """Init rejects malformed base URI."""
    with pytest.raises(ConfigurationError, match="base_uri"):
        FsspecStorageBackend(base_uri)


def test_init_rejects_create_bucket_for_gcs_before_importing_gcsfs(monkeypatch):
    """Init rejects create bucket for GCS before importing gcsfs."""
    monkeypatch.setitem(sys.modules, "gcsfs", None)
    with pytest.raises(ConfigurationError, match="infrastructure tooling"):
        FsspecStorageBackend("gs://bucket", create_bucket_if_missing=True)


def test_init_rejects_unknown_protocol():
    """Init rejects unknown protocol."""
    with pytest.raises(ConfigurationError, match="Unknown fsspec protocol"):
        FsspecStorageBackend("nosuchproto-xyz://bucket")


@pytest.mark.parametrize(
    ("protocol", "module", "extra"),
    [("s3", "s3fs", "michelangelo[s3]"), ("gs", "gcsfs", "michelangelo[gcs]")],
)
def test_init_missing_extra_raises_import_error_with_install_hint(
    monkeypatch, protocol, module, extra
):
    """Init missing extra raises import error with install hint."""
    monkeypatch.setitem(sys.modules, module, None)
    # fsspec caches imported classes; drop them so the import is retried.
    # (``fsspec.registry`` the attribute is a mappingproxy, not the module.)
    registry_module = sys.modules["fsspec.registry"]
    monkeypatch.setattr(
        registry_module,
        "_registry",
        {k: v for k, v in registry_module._registry.items() if k != protocol},
    )
    with pytest.raises(ImportError, match=extra.replace("[", r"\[")):
        FsspecStorageBackend(f"{protocol}://bucket")


def test_init_applies_cache_defaults_for_object_stores(monkeypatch):
    """Init applies cache defaults for object stores."""
    spy = MagicMock(return_value=_FakeS3FileSystem())
    monkeypatch.setattr(fsspec_backend.fsspec, "filesystem", spy)

    FsspecStorageBackend("s3://bucket", {"key": "k"})

    spy.assert_called_once_with(
        "s3", key="k", skip_instance_cache=True, use_listings_cache=False
    )


def test_init_keeps_caller_cache_settings(monkeypatch):
    """Init keeps caller cache settings."""
    spy = MagicMock(return_value=_FakeS3FileSystem())
    monkeypatch.setattr(fsspec_backend.fsspec, "filesystem", spy)

    FsspecStorageBackend(
        "s3://bucket", {"skip_instance_cache": False, "use_listings_cache": True}
    )

    spy.assert_called_once_with(
        "s3", skip_instance_cache=False, use_listings_cache=True
    )


def test_init_sets_auto_mkdir_for_file_protocol_only(monkeypatch, tmp_path):
    """Init sets auto mkdir for file protocol only."""
    spy = MagicMock(return_value=MemoryFileSystem())
    monkeypatch.setattr(fsspec_backend.fsspec, "filesystem", spy)

    FsspecStorageBackend(f"file://{tmp_path}")
    FsspecStorageBackend("memory://root")

    assert spy.call_args_list[0].kwargs == {
        "skip_instance_cache": True,
        "auto_mkdir": True,
    }
    assert spy.call_args_list[1].kwargs == {"skip_instance_cache": True}


def test_init_does_not_build_filesystem_when_injected(monkeypatch):
    """Init does not build filesystem when injected."""
    spy = MagicMock()
    monkeypatch.setattr(fsspec_backend.fsspec, "filesystem", spy)

    _s3_backend()

    spy.assert_not_called()


def test_get_storage_location_is_normalised():
    """Get storage location is normalised."""
    backend, _ = _s3_backend()
    assert backend.get_storage_location() == "s3://bucket"
    prefixed = FsspecStorageBackend(
        "s3://bucket/prefix/", filesystem=_FakeS3FileSystem()
    )
    assert prefixed.get_storage_location() == "s3://bucket/prefix"


def test_importing_package_does_not_import_s3fs_or_gcsfs():
    """Importing package does not import s3fs or gcsfs."""
    import subprocess

    code = (
        "import sys, michelangelo.lib.artifact_manager as m; "
        "print('s3fs' in sys.modules, 'gcsfs' in sys.modules)"
    )
    out = subprocess.run(
        [sys.executable, "-c", code], capture_output=True, text=True, check=True
    )
    assert out.stdout.strip() == "False False"


# ── memory:// round trips ───────────────────────────────────────────────────


def test_memory_file_round_trip(memory_root, tmp_path):
    """Memory file round trip."""
    backend = FsspecStorageBackend(f"memory://{memory_root}")
    src = tmp_path / "w.pt"
    src.write_bytes(b"weights")

    uri = backend.upload(str(src), "models/clf/v1/raw")
    dest = tmp_path / "back.pt"
    backend.download(uri, str(dest))

    assert uri == f"memory://{memory_root}/models/clf/v1/raw"
    assert dest.read_bytes() == b"weights"


def test_memory_directory_round_trip_keeps_nested_layout(
    memory_root, local_tree, tmp_path
):
    """Memory directory round trip keeps nested layout."""
    backend = FsspecStorageBackend(f"memory://{memory_root}")

    uri = backend.upload(str(local_tree), "models/clf/v1")
    backend.download(uri, str(tmp_path / "out"))

    assert _tree(tmp_path / "out") == _tree(local_tree)
    keys = sorted(MemoryFileSystem().find(f"/{memory_root}/models/clf/v1"))
    assert keys == [
        f"/{memory_root}/models/clf/v1/sub/config.json",
        f"/{memory_root}/models/clf/v1/sub/deeper/vocab.txt",
        f"/{memory_root}/models/clf/v1/weights.bin",
    ]


def test_memory_reupload_to_existing_prefix_does_not_nest(
    memory_root, local_tree, tmp_path
):
    """Memory reupload to existing prefix does not nest."""
    backend = FsspecStorageBackend(f"memory://{memory_root}")
    backend.upload(str(local_tree), "models/clf/v1")
    (local_tree / "weights.bin").write_bytes(b"new")

    backend.upload(str(local_tree), "models/clf/v1")

    keys = MemoryFileSystem().find(f"/{memory_root}/models/clf/v1")
    assert len(keys) == 3
    assert not any("/model/" in k for k in keys)
    backend.download(
        f"memory://{memory_root}/models/clf/v1/weights.bin", str(tmp_path / "w")
    )
    assert (tmp_path / "w").read_bytes() == b"new"


def test_memory_empty_directory_uploads_nothing(memory_root, tmp_path):
    """Memory empty directory uploads nothing."""
    backend = FsspecStorageBackend(f"memory://{memory_root}")
    empty = tmp_path / "empty"
    empty.mkdir()

    uri = backend.upload(str(empty), "models/empty")

    assert uri == f"memory://{memory_root}/models/empty"
    assert not MemoryFileSystem().exists(f"/{memory_root}/models/empty")


def test_memory_download_missing_path_raises_file_not_found(memory_root, tmp_path):
    """Memory download missing path raises file not found."""
    backend = FsspecStorageBackend(f"memory://{memory_root}")
    with pytest.raises(FileNotFoundError, match="No object or directory found"):
        backend.download(f"memory://{memory_root}/nope", str(tmp_path / "x"))


def test_memory_create_bucket_if_missing_creates_base(memory_root):
    """Memory create bucket if missing creates base."""
    FsspecStorageBackend(f"memory://{memory_root}/a", create_bucket_if_missing=True)
    assert MemoryFileSystem().isdir(f"/{memory_root}/a")


# ── file:// round trips ─────────────────────────────────────────────────────


def test_file_protocol_round_trip_creates_parent_dirs(local_tree, tmp_path):
    """File protocol round trip creates parent dirs."""
    base = tmp_path / "store"
    backend = FsspecStorageBackend(f"file://{base}", create_bucket_if_missing=True)

    file_uri = backend.upload(str(local_tree / "weights.bin"), "a/b/c/weights.bin")
    dir_uri = backend.upload(str(local_tree), "a/dir")
    backend.download(file_uri, str(tmp_path / "w.bin"))
    backend.download(dir_uri, str(tmp_path / "dir_out"))

    assert (base / "a" / "b" / "c" / "weights.bin").is_file()
    assert (tmp_path / "w.bin").read_bytes() == b"\x00\x01weights"
    assert _tree(tmp_path / "dir_out") == _tree(local_tree)


def test_file_protocol_overwrite_file(tmp_path):
    """File protocol overwrite file."""
    backend = FsspecStorageBackend(f"file://{tmp_path / 'store'}")
    src = tmp_path / "f.txt"
    src.write_text("one")
    backend.upload(str(src), "k/f.txt")
    src.write_text("two")

    uri = backend.upload(str(src), "k/f.txt")
    backend.download(uri, str(tmp_path / "back.txt"))

    assert (tmp_path / "back.txt").read_text() == "two"


# ── Argument validation ─────────────────────────────────────────────────────


def test_upload_rejects_empty_destination_key(tmp_path):
    """Upload rejects empty destination key."""
    backend, fs = _s3_backend()
    with pytest.raises(ValueError, match="destination_key"):
        backend.upload(str(tmp_path), "")
    assert fs.calls == []


def test_upload_missing_local_path_raises_file_not_found(tmp_path):
    """Upload missing local path raises file not found."""
    backend, fs = _s3_backend()
    with pytest.raises(FileNotFoundError, match="does not exist"):
        backend.upload(str(tmp_path / "missing"), "k")
    assert fs.calls == []


@pytest.mark.parametrize(
    ("uri", "match"),
    [
        ("gs://bucket/key", "protocol"),
        ("/local/path", "protocol"),
        ("s3:///key", "no bucket"),
        ("s3://bucket", "no object key"),
        ("s3://bucket/", "no object key"),
    ],
)
def test_download_rejects_invalid_uri(uri, match, tmp_path):
    """Download rejects invalid URI."""
    backend, fs = _s3_backend()
    with pytest.raises(ValueError, match=match):
        backend.download(uri, str(tmp_path / "x"))
    assert fs.calls == []


def test_download_rejects_uri_without_path_for_non_bucket_protocol(
    memory_root, tmp_path
):
    """Download rejects URI without path for non bucket protocol."""
    backend = FsspecStorageBackend(f"memory://{memory_root}")
    with pytest.raises(ValueError, match="no path"):
        backend.download("memory://", str(tmp_path / "x"))


def test_download_accepts_other_bucket_on_same_protocol(tmp_path):
    """Download accepts other bucket on same protocol."""
    backend, fs = _s3_backend()
    backend.download("s3://other-bucket/models/x", str(tmp_path / "x"))
    assert ("info", "other-bucket/models/x") in fs.calls


# ── Transfer details on bucket filesystems ──────────────────────────────────


def test_upload_file_uses_put_file_and_returns_uri(tmp_path):
    """Upload file uses put file and returns URI."""
    backend, fs = _s3_backend()
    src = tmp_path / "f"
    src.write_text("x")

    uri = backend.upload(str(src), "models/f")

    assert uri == "s3://bucket/models/f"
    assert fs.calls == [("put_file", str(src), "bucket/models/f")]


def test_upload_directory_uses_list_form_put(local_tree):
    """Upload directory uses list form put."""
    backend, fs = _s3_backend()

    backend.upload(str(local_tree), "models/clf")

    (name, lpaths, rpaths, kwargs) = fs.calls[0]
    assert name == "put"
    assert kwargs == {}  # sync filesystem: no batch_size
    assert sorted(rpaths) == [
        "bucket/models/clf/sub/config.json",
        "bucket/models/clf/sub/deeper/vocab.txt",
        "bucket/models/clf/weights.bin",
    ]
    assert len(lpaths) == len(rpaths)


def test_async_filesystem_gets_small_batch_size(local_tree, tmp_path):
    """Async filesystem gets small batch size."""
    fs = MagicMock(spec=AsyncFileSystem)
    fs._strip_protocol.side_effect = _FakeS3FileSystem._strip_protocol
    fs.info.return_value = {"type": "directory"}
    fs.find.return_value = ["bucket/models/clf/a.bin"]
    backend = FsspecStorageBackend("s3://bucket", filesystem=fs)

    backend.upload(str(local_tree), "models/clf")
    backend.download("s3://bucket/models/clf", str(tmp_path / "out"))

    assert fs.put.call_args.kwargs == {"batch_size": 8}
    assert fs.get.call_args.kwargs == {"batch_size": 8}


def test_download_directory_skips_dir_markers_and_foreign_keys(tmp_path):
    """Download directory skips dir markers and foreign keys."""
    backend, fs = _s3_backend()
    fs.raise_on["info"] = FileNotFoundError("nope")
    fs.find_result = [
        "bucket/models/clf/a.bin",
        "bucket/models/clf/marker/",
        "bucket/models/clf/sub/b.bin",
        "bucket/models/clfother/c.bin",
    ]

    backend.download("s3://bucket/models/clf", str(tmp_path / "out"))

    get_call = next(c for c in fs.calls if c[0] == "get")
    assert get_call[1] == ["bucket/models/clf/a.bin", "bucket/models/clf/sub/b.bin"]
    assert _tree(tmp_path / "out") == {"a.bin": b"x", "sub/b.bin": b"x"}


def test_download_directory_empty_listing_raises_file_not_found(tmp_path):
    """Download directory empty listing raises file not found."""
    backend, fs = _s3_backend()
    fs.info_result = {"type": "directory"}
    fs.find_result = ["bucket/models/clf/marker/"]

    with pytest.raises(FileNotFoundError, match="No object or directory found"):
        backend.download("s3://bucket/models/clf", str(tmp_path / "out"))


def test_failed_file_download_leaves_no_partial_file(tmp_path):
    """Failed file download leaves no partial file."""
    backend, fs = _s3_backend()
    fs.raise_on["get_file"] = ConnectionResetError("reset")
    dest = tmp_path / "dest.bin"

    with pytest.raises(ConnectionResetError, match="download failed"):
        backend.download("s3://bucket/models/f", str(dest))

    assert not dest.exists()


# ── Bucket creation ─────────────────────────────────────────────────────────


def test_create_bucket_calls_mkdir_on_bucket_root():
    """Create bucket calls mkdir on bucket root."""
    fs = _FakeS3FileSystem()
    FsspecStorageBackend(
        "s3://bucket/prefix", filesystem=fs, create_bucket_if_missing=True
    )
    assert fs.calls == [("mkdir", "bucket", False)]


def test_create_bucket_existing_and_accessible_is_ok():
    """Create bucket existing and accessible is ok."""
    fs = _FakeS3FileSystem()
    fs.raise_on["mkdir"] = FileExistsError("exists")
    fs.exists_result = True

    _s3_backend(fs, create_bucket_if_missing=True)

    assert [c[0] for c in fs.calls] == ["mkdir", "invalidate_cache", "exists"]


def test_create_bucket_owned_by_someone_else_raises_permission_error():
    """Create bucket owned by someone else raises permission error."""
    fs = _FakeS3FileSystem()
    fs.raise_on["mkdir"] = FileExistsError("BucketAlreadyExists")
    fs.exists_result = False

    with pytest.raises(PermissionError, match="not accessible"):
        _s3_backend(fs, create_bucket_if_missing=True)


def test_create_bucket_connection_error_is_wrapped_as_oserror():
    """Create bucket connection error is wrapped as OSError."""

    class EndpointConnectionError(Exception):
        pass

    fs = _FakeS3FileSystem()
    fs.raise_on["mkdir"] = EndpointConnectionError("could not connect")

    with pytest.raises(OSError, match="create bucket failed") as info:
        _s3_backend(fs, create_bucket_if_missing=True)

    assert isinstance(info.value.__cause__, EndpointConnectionError)


# ── Error contract ──────────────────────────────────────────────────────────


@pytest.mark.parametrize(
    "raised",
    [
        PermissionError("Access Denied"),
        FileNotFoundError("NoSuchBucket"),
        TimeoutError("RequestTimeout"),
        OSError(errno.EIO, "internal error"),
    ],
)
def test_oserror_subclasses_keep_their_class(raised, tmp_path):
    """OSError subclasses keep their class."""
    backend, fs = _s3_backend()
    fs.raise_on["put_file"] = raised
    src = tmp_path / "f"
    src.write_text("x")

    with pytest.raises(type(raised), match="upload failed for s3://bucket/k") as info:
        backend.upload(str(src), "k")

    assert type(info.value) is type(raised)
    assert info.value.__cause__ is raised
    assert info.value.errno == raised.errno


@pytest.mark.parametrize(
    "raised",
    [
        ValueError("invalid response"),  # gcsfs
        RuntimeError("FlexibleChecksumError"),  # botocore-style
        type("HttpError", (Exception,), {})("409 conflict"),  # gcsfs
    ],
)
def test_non_oserror_exceptions_are_wrapped(raised, tmp_path):
    """Non OSError exceptions are wrapped."""
    backend, fs = _s3_backend()
    fs.raise_on["put_file"] = raised
    src = tmp_path / "f"
    src.write_text("x")

    with pytest.raises(OSError, match=type(raised).__name__) as info:
        backend.upload(str(src), "k")

    assert info.value.__cause__ is raised


def test_region_errors_get_a_hint(tmp_path):
    """Region errors get a hint."""
    backend, fs = _s3_backend()
    fs.raise_on["info"] = OSError(errno.EINVAL, "AuthorizationHeaderMalformed")

    with pytest.raises(OSError, match="check the bucket region") as info:
        backend.download("s3://bucket/k", str(tmp_path / "x"))

    assert info.value.errno == errno.EINVAL


def test_oserror_subclass_with_unusual_constructor_falls_back_to_oserror(tmp_path):
    """OSError subclass with unusual constructor falls back to OSError."""

    class OddError(OSError):
        def __init__(self, a, b, c):
            super().__init__(f"{a}{b}{c}")

    backend, fs = _s3_backend()
    fs.raise_on["put_file"] = OddError(1, 2, 3)
    src = tmp_path / "f"
    src.write_text("x")

    with pytest.raises(OSError, match="upload failed") as info:
        backend.upload(str(src), "k")

    assert type(info.value) is OSError


# ── s3_storage_options ──────────────────────────────────────────────────────


@pytest.fixture
def no_ambient_aws(monkeypatch):
    """Isolate from the developer's AWS environment."""
    for var in (
        "AWS_REQUEST_CHECKSUM_CALCULATION",
        "AWS_RESPONSE_CHECKSUM_VALIDATION",
    ):
        monkeypatch.delenv(var, raising=False)
    monkeypatch.setattr(fsspec_backend, "_default_credentials_available", lambda: False)
    monkeypatch.setattr(
        fsspec_backend, "_botocore_supports_checksum_config", lambda: True
    )


def test_s3_options_sandbox_endpoint(no_ambient_aws):
    """S3 options sandbox endpoint."""
    opts = s3_storage_options(
        "localhost:9091", access_key="ak", secret_key="sk", secure=False
    )
    assert opts == {
        "key": "ak",
        "secret": "sk",
        "client_kwargs": {"endpoint_url": "http://localhost:9091"},
        "config_kwargs": {
            "s3": {"addressing_style": "path"},
            "request_checksum_calculation": "when_required",
            "response_checksum_validation": "when_required",
        },
    }


def test_s3_options_secure_uses_https(no_ambient_aws):
    """S3 options secure uses https."""
    opts = s3_storage_options("objectstore.example.com", access_key="a", secret_key="b")
    assert opts["client_kwargs"]["endpoint_url"] == "https://objectstore.example.com"


@pytest.mark.parametrize(
    "endpoint", ["", "http://localhost:9091", "localhost:9091/path", "s3://x"]
)
def test_s3_options_rejects_bad_endpoint(endpoint, no_ambient_aws):
    """S3 options rejects bad endpoint."""
    with pytest.raises(ValueError, match="host\\[:port\\]"):
        s3_storage_options(endpoint)


@pytest.fixture
def fresh_path_style_hint(monkeypatch):
    """Reset the once-per-process path-style hint."""
    monkeypatch.setattr(fsspec_backend, "_path_style_hint_logged", False)


def _hint_records(caplog):
    return [r for r in caplog.records if "path_style=False" in r.getMessage()]


@pytest.mark.parametrize(
    "endpoint",
    [
        "localhost:9091",
        "objectstore:9091",
        "s3.amazonaws.com",
        "storage.googleapis.com",
        "oss-cn-hangzhou.aliyuncs.com",
    ],
)
def test_s3_options_defaults_to_path_style_for_any_host(
    endpoint, no_ambient_aws, fresh_path_style_hint
):
    """The addressing style no longer depends on the endpoint host."""
    opts = s3_storage_options(endpoint, access_key="a", secret_key="b")
    assert opts["config_kwargs"]["s3"] == {"addressing_style": "path"}


@pytest.mark.parametrize(
    "endpoint", ["localhost:9091", "s3.eu-west-1.amazonaws.com:443"]
)
def test_s3_options_path_style_false_uses_virtual_host(
    endpoint, no_ambient_aws, fresh_path_style_hint, caplog
):
    """path_style=False selects virtual-host addressing and logs no hint."""
    caplog.set_level("WARNING", logger=fsspec_backend.__name__)
    opts = s3_storage_options(
        endpoint, access_key="a", secret_key="b", path_style=False
    )
    assert opts["config_kwargs"]["s3"] == {"addressing_style": "virtual"}
    assert _hint_records(caplog) == []


def test_s3_options_path_style_on_aws_host_logs_hint_once(
    no_ambient_aws, fresh_path_style_hint, caplog
):
    """Path-style on an AWS or Aliyun host keeps path-style and hints once."""
    caplog.set_level("WARNING", logger=fsspec_backend.__name__)
    first = s3_storage_options(
        "s3.eu-west-1.amazonaws.com:443",
        access_key="a",
        secret_key="b",
        path_style=True,
    )
    second = s3_storage_options(
        "oss-cn-hangzhou.aliyuncs.com", access_key="a", secret_key="b"
    )

    assert first["config_kwargs"]["s3"] == {"addressing_style": "path"}
    assert second["config_kwargs"]["s3"] == {"addressing_style": "path"}
    records = _hint_records(caplog)
    assert len(records) == 1
    assert records[0].levelname == "WARNING"
    assert "s3.eu-west-1.amazonaws.com:443" in records[0].getMessage()


@pytest.mark.parametrize(
    "endpoint", ["localhost:9091", "evilamazonaws.com", "storage.googleapis.com"]
)
def test_s3_options_path_style_on_other_hosts_logs_no_hint(
    endpoint, no_ambient_aws, fresh_path_style_hint, caplog
):
    """Hosts outside amazonaws.com / aliyuncs.com get no hint."""
    caplog.set_level("WARNING", logger=fsspec_backend.__name__)
    s3_storage_options(endpoint, access_key="a", secret_key="b")
    assert _hint_records(caplog) == []


@pytest.mark.parametrize("path_style", [True, False])
def test_s3_options_no_endpoint_ignores_path_style(path_style, no_ambient_aws):
    """Without an endpoint, botocore picks the addressing style."""
    opts = s3_storage_options(access_key="a", secret_key="b", path_style=path_style)
    assert "s3" not in opts["config_kwargs"]


def test_s3_options_no_endpoint_uses_aws_defaults(no_ambient_aws):
    """S3 options no endpoint uses AWS defaults."""
    opts = s3_storage_options(access_key="a", secret_key="b")
    assert "client_kwargs" not in opts
    assert "s3" not in opts["config_kwargs"]


@pytest.mark.parametrize(
    ("access_key", "secret_key"), [("", ""), (None, None), ("ak", ""), ("", "sk")]
)
def test_s3_options_empty_keys_fall_back_to_anonymous(
    access_key, secret_key, no_ambient_aws
):
    """S3 options empty keys fall back to anonymous."""
    opts = s3_storage_options(
        "localhost:9091", access_key=access_key, secret_key=secret_key
    )
    assert opts["anon"] is True
    assert "key" not in opts
    assert "secret" not in opts


def test_s3_options_empty_keys_use_default_chain_when_available(
    no_ambient_aws, monkeypatch
):
    """S3 options empty keys use default chain when available."""
    monkeypatch.setattr(fsspec_backend, "_default_credentials_available", lambda: True)
    opts = s3_storage_options("localhost:9091", access_key="", secret_key="")
    assert "anon" not in opts
    assert "key" not in opts


@pytest.mark.parametrize(
    ("region", "expected"),
    [(None, None), ("us-east-1", None), ("eu-west-1", "eu-west-1")],
)
def test_s3_options_region(region, expected, no_ambient_aws):
    """S3 options region."""
    opts = s3_storage_options(
        "localhost:9091", access_key="a", secret_key="b", region=region
    )
    assert opts["client_kwargs"].get("region_name") == expected


@pytest.mark.parametrize(
    "env_var", ["AWS_REQUEST_CHECKSUM_CALCULATION", "AWS_RESPONSE_CHECKSUM_VALIDATION"]
)
def test_s3_options_checksum_env_var_wins(env_var, no_ambient_aws, monkeypatch):
    """S3 options checksum env var wins."""
    monkeypatch.setenv(env_var, "when_supported")
    opts = s3_storage_options("localhost:9091", access_key="a", secret_key="b")
    assert "request_checksum_calculation" not in opts["config_kwargs"]
    assert "response_checksum_validation" not in opts["config_kwargs"]


def test_s3_options_checksum_omitted_for_old_botocore(no_ambient_aws, monkeypatch):
    """S3 options checksum omitted for old botocore."""
    monkeypatch.setattr(
        fsspec_backend, "_botocore_supports_checksum_config", lambda: False
    )
    opts = s3_storage_options(access_key="a", secret_key="b")
    assert "config_kwargs" not in opts


def test_default_credentials_available_reads_env(monkeypatch):
    """Default credentials available reads env."""
    pytest.importorskip("botocore")
    monkeypatch.setenv("AWS_ACCESS_KEY_ID", "env-key")
    monkeypatch.setenv("AWS_SECRET_ACCESS_KEY", "env-secret")
    assert fsspec_backend._default_credentials_available() is True


def test_default_credentials_without_botocore_defers_to_s3fs(monkeypatch):
    """Default credentials without botocore defers to s3fs."""
    monkeypatch.setitem(sys.modules, "botocore.session", None)
    assert fsspec_backend._default_credentials_available() is True


def test_default_credentials_lookup_error_means_none(monkeypatch):
    """Default credentials lookup error means none."""
    botocore_session = pytest.importorskip("botocore.session")

    def boom(*args, **kwargs):
        raise RuntimeError("bad config file")

    monkeypatch.setattr(botocore_session, "Session", boom)
    assert fsspec_backend._default_credentials_available() is False


def test_botocore_checksum_feature_detection(monkeypatch):
    """Botocore checksum feature detection."""
    pytest.importorskip("botocore")
    from botocore.config import Config

    expected = "request_checksum_calculation" in Config.OPTION_DEFAULTS
    assert fsspec_backend._botocore_supports_checksum_config() is expected
    monkeypatch.setitem(sys.modules, "botocore.config", None)
    assert fsspec_backend._botocore_supports_checksum_config() is False


def test_s3fs_accepts_generated_options_without_network(monkeypatch):
    """S3fs accepts generated options without network."""
    s3fs = pytest.importorskip("s3fs")
    monkeypatch.delenv("AWS_REQUEST_CHECKSUM_CALCULATION", raising=False)
    monkeypatch.delenv("AWS_RESPONSE_CHECKSUM_VALIDATION", raising=False)
    opts = s3_storage_options(
        "localhost:9091",
        access_key="a",
        secret_key="b",
        secure=False,
        region="eu-west-1",
    )

    fs = s3fs.S3FileSystem(skip_instance_cache=True, **opts)

    assert fs.client_kwargs["endpoint_url"] == "http://localhost:9091"
    assert fs.config_kwargs["s3"] == {"addressing_style": "path"}


def test_s3_backend_builds_real_s3fs_lazily(monkeypatch):
    """S3 backend builds real s3fs lazily."""
    s3fs = pytest.importorskip("s3fs")
    backend = FsspecStorageBackend(
        "s3://bucket",
        s3_storage_options("localhost:9091", access_key="a", secret_key="b"),
    )
    assert isinstance(backend._fs, s3fs.S3FileSystem)
    assert backend._fs._strip_protocol("s3://bucket/k") == "bucket/k"


# ── Fork safety ─────────────────────────────────────────────────────────────


def test_owned_filesystem_is_rebuilt_after_fork(monkeypatch):
    """Owned filesystem is rebuilt after fork."""
    built = [_FakeS3FileSystem(), _FakeS3FileSystem()]
    factory = MagicMock(side_effect=built)
    monkeypatch.setattr(fsspec_backend.fsspec, "filesystem", factory)
    backend = FsspecStorageBackend("s3://bucket", {"key": "k"})
    assert backend._fs is built[0]

    monkeypatch.setattr(fsspec_backend.os, "getpid", lambda: -1)

    assert backend._fs is built[1]
    assert backend._fs is built[1]  # rebuilt once per process
    assert factory.call_args_list[1] == factory.call_args_list[0]


def test_injected_filesystem_is_not_rebuilt_after_fork(monkeypatch):
    """Injected filesystem is not rebuilt after fork."""
    backend, fs = _s3_backend()
    monkeypatch.setattr(fsspec_backend.os, "getpid", lambda: -1)
    assert backend._fs is fs


@pytest.mark.parametrize(("alive", "reset"), [(False, True), (True, False)])
def test_fsspec_loop_reset_only_when_io_thread_is_dead(monkeypatch, alive, reset):
    """Fsspec loop reset only when I/O thread is dead."""
    thread = MagicMock()
    thread.is_alive.return_value = alive
    reset_lock = MagicMock()
    monkeypatch.setattr(fsspec_backend.fsspec.asyn, "iothread", [thread])
    monkeypatch.setattr(fsspec_backend.fsspec.asyn, "reset_lock", reset_lock)

    fsspec_backend._reset_fsspec_loop_after_fork()

    assert reset_lock.called is reset


def test_fsspec_loop_reset_skipped_when_loop_never_started(monkeypatch):
    """Fsspec loop reset skipped when loop never started."""
    reset_lock = MagicMock()
    monkeypatch.setattr(fsspec_backend.fsspec.asyn, "iothread", [None])
    monkeypatch.setattr(fsspec_backend.fsspec.asyn, "reset_lock", reset_lock)

    fsspec_backend._reset_fsspec_loop_after_fork()

    reset_lock.assert_not_called()


def test_upload_directory_fails_when_a_subdirectory_is_unreadable(tmp_path):
    """Upload directory fails when a subdirectory is unreadable."""
    src = tmp_path / "model"
    (src / "locked").mkdir(parents=True)
    (src / "weights.bin").write_bytes(b"w")
    (src / "locked" / "a.bin").write_bytes(b"a")
    (src / "locked").chmod(0o000)
    backend, fs = _s3_backend()
    try:
        if os.access(src / "locked", os.R_OK):
            pytest.skip("directory permissions are not enforced (running as root)")
        with pytest.raises(PermissionError):
            backend.upload(str(src), "models/clf")
    finally:
        (src / "locked").chmod(0o755)

    assert not any(call[0] == "put" for call in fs.calls)


@pytest.mark.parametrize("name", ["../victim", "sub/../../victim", ".."])
def test_download_directory_rejects_keys_escaping_the_destination(tmp_path, name):
    """Download directory rejects keys escaping the destination."""
    backend, fs = _s3_backend()
    fs.info_result = {"type": "directory"}
    fs.find_result = [f"bucket/models/clf/{name}"]
    out = tmp_path / "out"

    with pytest.raises(ValueError, match="outside"):
        backend.download("s3://bucket/models/clf", str(out))

    assert not (tmp_path / "victim").exists()
    assert not out.exists()


def test_failed_file_download_keeps_existing_destination(tmp_path):
    """Failed file download keeps existing destination."""
    backend, fs = _s3_backend()
    fs.raise_on["get_file"] = ConnectionResetError("reset")
    dest = tmp_path / "dest.bin"
    dest.write_bytes(b"previous")

    with pytest.raises(ConnectionResetError, match="download failed"):
        backend.download("s3://bucket/models/f", str(dest))

    assert dest.read_bytes() == b"previous"
    assert [p.name for p in tmp_path.iterdir()] == ["dest.bin"]


def test_abandoning_an_inherited_filesystem_detaches_its_finalizers():
    """Abandoning an inherited filesystem detaches its finalizers."""
    import gc
    import weakref

    class _Fs:
        pass

    fs = _Fs()
    closed = []
    weakref.finalize(fs, closed.append, "closed")

    fsspec_backend._abandon_inherited_filesystem(fs)
    # Drop our keep-alive reference too: only the detach can prevent the call.
    fsspec_backend._inherited_filesystems.clear()
    del fs
    gc.collect()
    assert closed == []


def test_owned_filesystems_are_all_abandoned_at_fork_even_if_unused():
    """Owned filesystems are all abandoned at fork even if unused in the child."""
    import gc
    import weakref

    class _Fs:
        pass

    closed = []
    fss = [_Fs(), _Fs()]
    for i, fs in enumerate(fss):
        fsspec_backend._track_owned_filesystem(fs)
        weakref.finalize(fs, closed.append, i)

    fsspec_backend._abandon_owned_filesystems()  # what the after-fork hook runs
    fsspec_backend._inherited_filesystems.clear()
    del fss, fs
    gc.collect()

    assert closed == []
    assert fsspec_backend._owned_filesystems == {}


def test_after_fork_hook_is_registered():
    """The after-fork hook is registered where the platform supports it."""
    if not hasattr(os, "register_at_fork"):
        pytest.skip("os.register_at_fork is unavailable")

    class _Fs:
        pass

    fs = _Fs()  # alive in the parent across the fork
    fsspec_backend._track_owned_filesystem(fs)
    assert fsspec_backend._owned_filesystems
    pid = os.fork()
    if pid == 0:
        # In the child: the hook must have emptied the registry of anything
        # tracked before the fork.
        os._exit(0 if fsspec_backend._owned_filesystems == {} else 1)
    _, status = os.waitpid(pid, 0)
    assert os.waitstatus_to_exitcode(status) == 0
    assert fsspec_backend._owned_filesystems  # the parent keeps its own
    fsspec_backend._owned_filesystems.pop(id(fs), None)
