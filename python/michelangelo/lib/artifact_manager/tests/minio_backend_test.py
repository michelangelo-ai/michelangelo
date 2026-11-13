"""Tests for the deprecated MinioStorageBackend shim."""

from __future__ import annotations

import logging
import pathlib
import re
import subprocess
import sys
import warnings
from unittest.mock import MagicMock

import fsspec
import pytest

from michelangelo.lib.artifact_manager import fsspec_backend, minio_backend
from michelangelo.lib.artifact_manager.fsspec_backend import FsspecStorageBackend
from michelangelo.lib.artifact_manager.minio_backend import MinioStorageBackend
from michelangelo.lib.exceptions import ConfigurationError


class _SpyFileSystem(fsspec.AbstractFileSystem):
    protocol = ("s3", "s3a")
    root_marker = ""
    cachable = False

    def __init__(self, **kwargs):
        super().__init__(**kwargs)
        self.mkdirs: list[str] = []

    def mkdir(self, path, create_parents=True, **kwargs):
        self.mkdirs.append(path)


@pytest.fixture
def spy(monkeypatch):
    """Replace fsspec.filesystem with a spy and pin the ambient AWS state."""
    fs = _SpyFileSystem()
    factory = MagicMock(return_value=fs)
    monkeypatch.setattr(fsspec_backend.fsspec, "filesystem", factory)
    monkeypatch.setattr(fsspec_backend, "_default_credentials_available", lambda: False)
    monkeypatch.setattr(
        fsspec_backend, "_botocore_supports_checksum_config", lambda: False
    )
    monkeypatch.setattr(minio_backend, "_logged_once", False)
    factory.fs = fs
    return factory


@pytest.mark.parametrize(
    ("kwargs", "match"),
    [
        ({"endpoint": "", "bucket": "b"}, "endpoint must be non-empty"),
        ({"endpoint": "localhost:9091", "bucket": ""}, "bucket must be non-empty"),
    ],
)
def test_shim_rejects_empty_endpoint_or_bucket(kwargs, match, spy):
    """Shim rejects empty endpoint or bucket."""
    with pytest.raises(ConfigurationError, match=match):
        MinioStorageBackend(access_key="a", secret_key="s", **kwargs)
    spy.assert_not_called()


def test_shim_rejects_endpoint_with_scheme_without_warning(spy):
    """Shim rejects endpoint with scheme without warning."""
    with warnings.catch_warnings():
        warnings.simplefilter("error", DeprecationWarning)
        with pytest.raises(ValueError, match="no scheme"):
            MinioStorageBackend("http://localhost:9091", "b", "a", "s")


def test_shim_forwards_all_seven_positional_arguments(spy):
    """Shim forwards all seven positional arguments."""
    with pytest.warns(DeprecationWarning):
        backend = MinioStorageBackend(
            "localhost:9091", "my-bucket", "ak", "sk", False, "eu-west-1", True
        )

    spy.assert_called_once_with(
        "s3",
        key="ak",
        secret="sk",
        client_kwargs={
            "endpoint_url": "http://localhost:9091",
            "region_name": "eu-west-1",
        },
        config_kwargs={"s3": {"addressing_style": "path"}},
        skip_instance_cache=True,
        use_listings_cache=False,
    )
    assert spy.fs.mkdirs == ["my-bucket"]
    assert backend.get_storage_location() == "s3://my-bucket"


def test_shim_keyword_arguments_as_used_by_examples(spy):
    """Matches the call in michelangelo-examples' _backend.py files."""
    with pytest.warns(DeprecationWarning):
        backend = MinioStorageBackend(
            endpoint="objectstore:9091",
            bucket="default",
            access_key="",
            secret_key="",
            secure=False,
            create_bucket_if_missing=True,
        )

    options = spy.call_args.kwargs
    assert options["anon"] is True  # empty keys and no default credentials
    assert "key" not in options
    assert options["client_kwargs"] == {"endpoint_url": "http://objectstore:9091"}
    assert spy.fs.mkdirs == ["default"]
    assert backend.get_storage_location() == "s3://default"


def test_shim_defaults_secure_true_and_no_bucket_creation(spy):
    """Shim defaults secure true and no bucket creation."""
    with pytest.warns(DeprecationWarning):
        MinioStorageBackend("s3.amazonaws.com", "b", "ak", "sk")

    options = spy.call_args.kwargs
    assert options["client_kwargs"] == {"endpoint_url": "https://s3.amazonaws.com"}
    assert options["config_kwargs"] == {"s3": {"addressing_style": "virtual"}}
    assert spy.fs.mkdirs == []


def test_shim_is_an_fsspec_backend(spy):
    """Shim is an fsspec backend."""
    with pytest.warns(DeprecationWarning):
        backend = MinioStorageBackend("localhost:9091", "b", "a", "s")
    assert isinstance(backend, FsspecStorageBackend)


def test_shim_warning_points_at_the_caller(spy):
    """Shim warning points at the caller."""
    with warnings.catch_warnings(record=True) as caught:
        warnings.simplefilter("always")
        MinioStorageBackend("localhost:9091", "b", "a", "s")

    deprecations = [w for w in caught if w.category is DeprecationWarning]
    assert len(deprecations) == 1
    assert "FsspecStorageBackend" in str(deprecations[0].message)
    assert deprecations[0].filename == __file__


def test_shim_logs_deprecation_once_per_process(spy, caplog):
    """Shim logs deprecation once per process."""
    capture = caplog.at_level(logging.WARNING, logger=minio_backend.__name__)
    with capture, pytest.warns(DeprecationWarning):
        MinioStorageBackend("localhost:9091", "b", "a", "s")
        MinioStorageBackend("localhost:9091", "b", "a", "s")

    records = [r for r in caplog.records if "deprecated" in r.getMessage()]
    assert len(records) == 1


def test_importing_shim_does_not_warn():
    """Importing shim does not warn."""
    code = "import michelangelo.lib.artifact_manager.minio_backend"
    subprocess.run(
        [sys.executable, "-W", "error::DeprecationWarning", "-c", code], check=True
    )


def test_both_import_paths_resolve_to_the_same_class():
    """Both import paths resolve to the same class."""
    import michelangelo.lib.artifact_manager as package

    assert package.MinioStorageBackend is minio_backend.MinioStorageBackend


def test_no_module_imports_the_minio_sdk():
    """No module imports the minio sdk."""
    package_root = pathlib.Path(minio_backend.__file__).resolve().parents[2]
    pattern = re.compile(r"^\s*(import minio\b|from minio\b)", re.MULTILINE)
    offenders = [
        str(path.relative_to(package_root))
        for path in package_root.rglob("*.py")
        if pattern.search(path.read_text(encoding="utf-8", errors="ignore"))
    ]
    assert offenders == []
