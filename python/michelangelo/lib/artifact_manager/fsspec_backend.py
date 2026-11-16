"""fsspec-based storage backend for Michelangelo artifact storage.

Implements :class:`~michelangelo.lib.artifact_manager.storage_backend.StorageBackend`
on top of `fsspec <https://filesystem-spec.readthedocs.io/>`_, so one class
serves every fsspec protocol:

- ``s3://`` through ``s3fs`` (AWS S3, SeaweedFS, MinIO, Ceph RGW and other
  S3-compatible servers). Install with ``pip install 'michelangelo[s3]'``.
- ``gs://`` through ``gcsfs`` (Google Cloud Storage). Install with
  ``pip install 'michelangelo[gcs]'``.
- ``memory://`` and ``file://``, which need no extra package (useful in tests).
- Any other protocol registered with fsspec (for example ``abfs://`` from
  ``adlfs``), or a pre-built filesystem passed as ``filesystem=``.

Typical usage::

    import os

    from michelangelo.lib.artifact_manager.fsspec_backend import (
        FsspecStorageBackend,
        s3_storage_options,
    )

    # S3-compatible server (for example the sandbox object store)
    backend = FsspecStorageBackend(
        "s3://michelangelo-models",
        s3_storage_options(
            "localhost:9091",
            access_key=os.environ["AWS_ACCESS_KEY_ID"],
            secret_key=os.environ["AWS_SECRET_ACCESS_KEY"],
            secure=False,  # local dev only, never in production
        ),
        create_bucket_if_missing=True,  # sandbox only
    )

    # Google Cloud Storage (credentials from the environment)
    backend = FsspecStorageBackend("gs://my-bucket/models")

    uri = backend.upload("/tmp/my-model", "models/clf/v1/raw")
    backend.download(uri, "/tmp/retrieved")
"""

from __future__ import annotations

import errno
import logging
import os
import tempfile
import weakref
from contextlib import contextmanager
from typing import TYPE_CHECKING, Any, NoReturn

import fsspec
import fsspec.asyn
from fsspec.asyn import AsyncFileSystem

from michelangelo.lib.artifact_manager.storage_backend import StorageBackend
from michelangelo.lib.exceptions import ConfigurationError

if TYPE_CHECKING:
    from collections.abc import Iterator, Mapping

_logger = logging.getLogger(__name__)

__all__ = ["FsspecStorageBackend", "s3_storage_options"]

# Protocols whose first path segment is a bucket.
_BUCKET_PROTOCOLS = frozenset({"s3", "s3a", "gs", "gcs"})
_GCS_PROTOCOLS = frozenset({"gs", "gcs"})
# Protocols whose filesystem keeps a listings cache we want off by default.
_LISTINGS_CACHE_PROTOCOLS = _BUCKET_PROTOCOLS
_INSTALL_HINTS = {
    "s3": "pip install 'michelangelo[s3]'",
    "s3a": "pip install 'michelangelo[s3]'",
    "gs": "pip install 'michelangelo[gcs]'",
    "gcs": "pip install 'michelangelo[gcs]'",
}
# Concurrent transfers per directory upload/download. s3fs reads a whole file
# into memory for single-request uploads (below 2 x chunksize, 100 MiB by
# default), so this bounds the memory a directory of large files can take.
_BATCH_SIZE = 8
# Hosts that need virtual-host addressing (mirrors minio's BucketLookupAuto).
_VIRTUAL_HOST_SUFFIXES = (".amazonaws.com", ".aliyuncs.com")
_VIRTUAL_HOSTS = frozenset({"storage.googleapis.com"})
_REGION_ERRNOS = frozenset(
    code for code in (getattr(errno, "EREMCHG", None), errno.EINVAL) if code
)


class FsspecStorageBackend(StorageBackend):
    """StorageBackend backed by any fsspec filesystem (S3, GCS, local, ...).

    Artifacts are stored under ``base_uri``. Callers work with local paths on
    both sides.

    **Directory vs. file:** a directory is uploaded as one object per file,
    each at ``destination_key/<relative path>``, keeping nested
    subdirectories. A plain file is stored at ``destination_key``.
    ``download()`` checks the exact key first: if an object exists there it
    is a file; otherwise every object under ``key + "/"`` is downloaded and
    the directory structure is rebuilt.

    **Bucket creation:** by default the backend does not create anything.
    Set ``create_bucket_if_missing=True`` for sandbox or dev environments.
    Most production IAM policies do not grant ``s3:CreateBucket``. It is not
    supported for ``gs://``: create GCS buckets with infrastructure tooling.

    **Errors:** every failure from the filesystem layer is raised as an
    :class:`OSError` (or a subclass such as :class:`FileNotFoundError` or
    :class:`PermissionError`), with the original exception chained.
    :class:`ValueError` is raised only for invalid arguments, before any
    filesystem call.

    The filesystem implementation (``s3fs``, ``gcsfs``, ...) is imported when
    the backend is constructed, not when this module is imported.

    Args:
        base_uri: Root URI for artifacts, ``protocol://path``. For example
            ``"s3://bucket"``, ``"s3://bucket/prefix"``, ``"gs://bucket"``,
            ``"memory://artifacts"`` or ``"file:///abs/dir"``.
        storage_options: Keyword arguments for the filesystem constructor,
            passed to ``fsspec.filesystem(protocol, **storage_options)``. For
            S3-compatible servers, build them with :func:`s3_storage_options`.
            Ignored when ``filesystem`` is given.
        create_bucket_if_missing: Create the bucket (``s3://``) or base
            directory (``memory://``, ``file://``) if it does not exist.
            Defaults to ``False``.
        filesystem: A pre-built fsspec filesystem to use instead of creating
            one from ``storage_options``. Its protocol should match
            ``base_uri``. Unlike a filesystem the backend creates, it is not
            rebuilt in forked child processes.

    Raises:
        ConfigurationError: If ``base_uri`` is malformed, the protocol is
            unknown, or ``create_bucket_if_missing`` is set for ``gs://``.
        ImportError: If the filesystem package for the protocol is not
            installed. The message includes the ``pip install`` command.
        OSError: If ``create_bucket_if_missing`` is set and the bucket cannot
            be created or accessed.
    """

    def __init__(
        self,
        base_uri: str,
        storage_options: Mapping[str, Any] | None = None,
        *,
        create_bucket_if_missing: bool = False,
        filesystem: fsspec.AbstractFileSystem | None = None,
    ) -> None:
        """Validate ``base_uri``, set up the filesystem and optional bucket."""
        protocol, path = _split_uri(base_uri or "")
        if not protocol or not path.strip("/"):
            raise ConfigurationError(
                f"FsspecStorageBackend base_uri {base_uri!r} is invalid. "
                "Use 'protocol://path', for example 's3://my-bucket' or "
                "'gs://my-bucket/prefix'."
            )
        if create_bucket_if_missing and protocol in _GCS_PROTOCOLS:
            raise ConfigurationError(
                "create_bucket_if_missing is not supported for GCS; "
                "create GCS buckets with infrastructure tooling."
            )
        self._protocol = protocol
        # Keep a leading "/" for absolute local paths (file:///abs/dir).
        self._base_path = path.rstrip("/")
        self._base_uri = f"{protocol}://{self._base_path}"
        # Rebuilt after a fork when the backend created it (see _fs).
        self._storage_options = dict(storage_options or {})
        self._owns_filesystem = filesystem is None
        self._filesystem = (
            filesystem
            if filesystem is not None
            else _make_filesystem(protocol, self._storage_options)
        )
        self._pid = os.getpid()
        if create_bucket_if_missing:
            self._ensure_bucket()

    @property
    def _fs(self) -> fsspec.AbstractFileSystem:
        """The filesystem, rebuilt in a forked child process.

        Async filesystems (s3fs, gcsfs) refuse to run in a child process
        forked after they were used. A backend that built its own filesystem
        builds a fresh one in the child; an injected ``filesystem`` is used
        as given.
        """
        if self._owns_filesystem and self._pid != os.getpid():
            _reset_fsspec_loop_after_fork()
            _abandon_inherited_filesystem(self._filesystem)
            self._filesystem = _make_filesystem(self._protocol, self._storage_options)
            self._pid = os.getpid()
        return self._filesystem

    # ── StorageBackend API ──────────────────────────────────────────────────

    def upload(self, local_path: str, destination_key: str) -> str:
        """Upload a local file or directory under ``base_uri``.

        Args:
            local_path: Path to the local file or directory to upload.
            destination_key: Object key (file) or key prefix (directory)
                relative to ``base_uri``, for example
                ``"models/my-clf/a1b2c3d4e5f6a7b8/raw"``.

        Returns:
            ``f"{base_uri}/{destination_key}"``.

        Raises:
            ValueError: If ``destination_key`` is empty.
            FileNotFoundError: If ``local_path`` does not exist.
            OSError: If the upload fails.
        """
        if not destination_key:
            raise ValueError(
                "destination_key must be non-empty. "
                "Provide a key such as 'models/classifier/v1'."
            )
        if not os.path.exists(local_path):
            raise FileNotFoundError(
                errno.ENOENT, f"Local path {local_path!r} does not exist."
            )
        uri = f"{self._base_uri}/{destination_key}"
        remote = self._to_fs_path(uri)
        if os.path.isdir(local_path):
            local_files, remote_files = [], []

            def _raise(exc: OSError) -> None:
                # os.walk skips unreadable directories silently by default.
                raise exc

            for root, _dirs, files in os.walk(local_path, onerror=_raise):
                for filename in files:
                    file_path = os.path.join(root, filename)
                    rel_path = os.path.relpath(file_path, local_path)
                    local_files.append(file_path)
                    remote_files.append(f"{remote}/{rel_path.replace(os.sep, '/')}")
            _logger.debug(
                "Uploading %d file(s) from '%s' to %s.",
                len(local_files),
                local_path,
                uri,
            )
            if local_files:
                with self._errors("upload", uri):
                    self._fs.put(local_files, remote_files, **self._batch_kwargs())
        else:
            _logger.debug("Uploading file '%s' to %s.", local_path, uri)
            with self._errors("upload", uri):
                self._fs.put_file(local_path, remote)
        return uri

    def download(self, uri: str, local_path: str) -> None:
        """Download a file or directory artifact to ``local_path``.

        Args:
            uri: A URI returned by :meth:`upload`. It must use this backend's
                protocol; for bucket protocols it may name any bucket the
                credentials can read.
            local_path: Destination path. For file artifacts the parent
                directory must exist. For directory artifacts the destination
                is created if absent.

        Raises:
            ValueError: If ``uri`` uses another protocol or has no key.
            FileNotFoundError: If nothing exists at ``uri``.
            OSError: If the download fails.
        """
        self._validate_uri(uri)
        remote = self._to_fs_path(uri)
        try:
            with self._errors("download", uri):
                info = self._fs.info(remote)
        except FileNotFoundError:
            info = {"type": "directory"}  # let the listing decide
        if info.get("type") == "file":
            self._download_file(uri, remote, local_path)
        else:
            self._download_directory(uri, remote, local_path)

    def get_storage_location(self) -> str:
        """Return ``base_uri`` (normalised, no trailing slash)."""
        return self._base_uri

    # ── Private helpers ─────────────────────────────────────────────────────

    def _download_file(self, uri: str, remote: str, local_path: str) -> None:
        """Download to a temp file first, so a failure leaves no partial file."""
        _logger.debug("Downloading %s to '%s'.", uri, local_path)
        # Stage next to the destination so os.replace is atomic (same
        # filesystem) and a failure never truncates an existing file.
        dest_dir = os.path.dirname(os.path.abspath(local_path))
        tmp_fd, tmp_path = tempfile.mkstemp(dir=dest_dir)
        os.close(tmp_fd)
        try:
            with self._errors("download", uri):
                self._fs.get_file(remote, tmp_path)
            os.replace(tmp_path, local_path)
        finally:
            if os.path.exists(tmp_path):
                os.unlink(tmp_path)

    def _download_directory(self, uri: str, remote: str, local_path: str) -> None:
        """Download every object under ``remote + "/"`` into ``local_path``."""
        prefix = remote.rstrip("/") + "/"
        with self._errors("download", uri):
            found = self._fs.find(prefix, withdirs=False)
        # Skip zero-byte "dir/" markers some tools create.
        keys = sorted(k for k in found if k.startswith(prefix) and not k.endswith("/"))
        if not keys:
            raise FileNotFoundError(
                errno.ENOENT, f"No object or directory found at {uri}."
            )
        root = os.path.realpath(local_path)
        local_files = []
        for key in keys:
            parts = key[len(prefix) :].split("/")
            dest = os.path.join(local_path, *parts)
            # An object key may contain ".." (S3 allows it): never write
            # outside the download directory.
            if (
                ".." in parts
                or os.path.commonpath([root, os.path.realpath(dest)]) != root
            ):
                raise ValueError(
                    f"Refusing to download {key!r} from {uri}: it would be "
                    f"written outside {local_path!r}."
                )
            local_files.append(dest)
        for dest in local_files:
            os.makedirs(os.path.dirname(dest), exist_ok=True)
        _logger.debug("Downloading %d object(s) from %s.", len(keys), uri)
        with self._errors("download", uri):
            self._fs.get(keys, local_files, **self._batch_kwargs())

    def _ensure_bucket(self) -> None:
        """Create the bucket (bucket protocols) or base directory (others).

        For buckets, a ``FileExistsError`` from the create call means the
        bucket already exists, another worker created it concurrently, or
        another account owns the name. The last case is reported as
        :class:`PermissionError` instead of being ignored.
        """
        if self._protocol not in _BUCKET_PROTOCOLS:
            with self._errors("create base directory", self._base_uri):
                self._fs.makedirs(self._base_path, exist_ok=True)
            return
        bucket = self._base_path.split("/", 1)[0]
        bucket_uri = f"{self._protocol}://{bucket}"
        try:
            with self._errors("create bucket", bucket_uri):
                self._fs.mkdir(bucket, create_parents=False)
            _logger.info("Created bucket %s.", bucket_uri)
        except FileExistsError:
            self._fs.invalidate_cache(bucket)
            with self._errors("check bucket", bucket_uri):
                accessible = self._fs.exists(bucket)
            if not accessible:
                raise PermissionError(
                    errno.EACCES,
                    f"Bucket {bucket_uri} exists but is not accessible with "
                    "these credentials (it may be owned by another account).",
                ) from None
            _logger.debug("Bucket %s already exists.", bucket_uri)

    def _validate_uri(self, uri: str) -> None:
        protocol, path = _split_uri(uri)
        if protocol != self._protocol:
            raise ValueError(
                f"URI {uri!r} does not use this backend's protocol. "
                f"Expected a URI in the form '{self._protocol}://...'."
            )
        if self._protocol in _BUCKET_PROTOCOLS:
            bucket, _sep, key = path.partition("/")
            if not bucket:
                raise ValueError(f"URI contains no bucket: {uri!r}")
            if not key.strip("/"):
                raise ValueError(f"URI contains no object key: {uri!r}")
        elif not path.strip("/"):
            raise ValueError(f"URI contains no path: {uri!r}")

    def _to_fs_path(self, uri: str) -> str:
        return self._fs._strip_protocol(uri)

    def _batch_kwargs(self) -> dict[str, Any]:
        # Only async filesystems (s3fs, gcsfs) take batch_size.
        if isinstance(self._fs, AsyncFileSystem):
            return {"batch_size": _BATCH_SIZE}
        return {}

    @contextmanager
    def _errors(self, op: str, uri: str) -> Iterator[None]:
        """Normalise filesystem exceptions to OSError (see class docstring)."""
        try:
            yield
        except OSError as exc:
            _reraise_oserror(exc, f"{op} failed for {uri}")
        except Exception as exc:  # botocore, gcsfs, aiohttp, ...
            raise OSError(
                f"{op} failed for {uri}: {type(exc).__name__}: {exc}"
            ) from exc


def s3_storage_options(
    endpoint: str | None = None,
    *,
    access_key: str | None = None,
    secret_key: str | None = None,
    secure: bool = True,
    region: str | None = None,
) -> dict[str, Any]:
    """Build ``s3fs`` storage options for an S3-compatible endpoint.

    Pure function: it does not import ``s3fs``. It may import ``botocore``
    (installed with ``s3fs``) to look up default credentials and supported
    config options; without it those steps are skipped.

    Args:
        endpoint: Server address as ``host[:port]``, with no scheme or path
            (for example ``"localhost:9091"`` or ``"s3.eu-west-1.amazonaws.com"``).
            ``None`` uses the AWS default endpoint.
        access_key: Access key ID. If either key is empty or ``None``, the
            botocore default credential chain (env vars, config files, IAM
            role) is used, and anonymous access if that finds nothing.
        secret_key: Secret access key matching ``access_key``.
        secure: Use HTTPS. Defaults to ``True``. Set ``False`` only for a
            local sandbox server without TLS.
        region: Bucket region, for example ``"eu-west-1"``. ``None`` and
            ``"us-east-1"`` use the botocore default.

    Returns:
        Options for ``FsspecStorageBackend(storage_options=...)`` or
        ``s3fs.S3FileSystem(**options)``. Flexible checksums are set to
        ``when_required`` unless ``AWS_REQUEST_CHECKSUM_CALCULATION`` or
        ``AWS_RESPONSE_CHECKSUM_VALIDATION`` is set.

    Raises:
        ValueError: If ``endpoint`` contains a scheme or a path.
    """
    client_kwargs: dict[str, Any] = {}
    config_kwargs: dict[str, Any] = {}
    options: dict[str, Any] = {}

    if endpoint is not None:
        if not endpoint or "://" in endpoint or "/" in endpoint:
            raise ValueError(
                f"endpoint {endpoint!r} must be 'host[:port]' with no scheme or "
                "path, for example 'localhost:9091'. Use secure= for the scheme."
            )
        client_kwargs["endpoint_url"] = f"{'https' if secure else 'http'}://{endpoint}"
        host = endpoint.rsplit(":", 1)[0].lower()
        virtual = host in _VIRTUAL_HOSTS or host.endswith(_VIRTUAL_HOST_SUFFIXES)
        config_kwargs["s3"] = {"addressing_style": "virtual" if virtual else "path"}

    if region and region != "us-east-1":
        # s3fs sends LocationConstraint=region_name on bucket creation, which
        # AWS rejects for us-east-1 (the botocore default region).
        client_kwargs["region_name"] = region

    if access_key and secret_key:
        options["key"] = access_key
        options["secret"] = secret_key
    elif not _default_credentials_available():
        options["anon"] = True

    if (
        "AWS_REQUEST_CHECKSUM_CALCULATION" not in os.environ
        and "AWS_RESPONSE_CHECKSUM_VALIDATION" not in os.environ
        and _botocore_supports_checksum_config()
    ):
        config_kwargs["request_checksum_calculation"] = "when_required"
        config_kwargs["response_checksum_validation"] = "when_required"

    if client_kwargs:
        options["client_kwargs"] = client_kwargs
    if config_kwargs:
        options["config_kwargs"] = config_kwargs
    return options


# ── Module helpers ──────────────────────────────────────────────────────────


def _split_uri(uri: str) -> tuple[str, str]:
    protocol, sep, path = uri.partition("://")
    if not sep:
        return "", uri
    return protocol, path


def _make_filesystem(
    protocol: str, storage_options: Mapping[str, Any] | None
) -> fsspec.AbstractFileSystem:
    options = dict(storage_options or {})
    # A fresh instance per backend: no state shared across backends or forks.
    options.setdefault("skip_instance_cache", True)
    if protocol in _LISTINGS_CACHE_PROTOCOLS:
        options.setdefault("use_listings_cache", False)
    if protocol == "file":
        options.setdefault("auto_mkdir", True)
    try:
        fs = fsspec.filesystem(protocol, **options)
        _track_owned_filesystem(fs)
        return fs
    except ImportError as exc:
        hint = _INSTALL_HINTS.get(
            protocol, f"install the fsspec implementation for {protocol!r}"
        )
        raise ImportError(
            f"FsspecStorageBackend needs an extra package for {protocol}:// "
            f"URIs. Install it with: {hint}"
        ) from exc
    except ValueError as exc:
        raise ConfigurationError(
            f"Unknown fsspec protocol {protocol!r}: {exc}"
        ) from exc


# Filesystems this module created, by id (weakly: they are not kept alive).
# Keyed by id because fsspec filesystems compare equal by their arguments, so
# a set would merge distinct instances.
_owned_filesystems: dict[int, weakref.ref] = {}


def _track_owned_filesystem(fs: fsspec.AbstractFileSystem) -> None:
    key = id(fs)
    _owned_filesystems[key] = weakref.ref(
        fs, lambda _ref, key=key: _owned_filesystems.pop(key, None)
    )


def _abandon_owned_filesystems() -> None:
    """Run in a forked child: abandon every filesystem inherited from the parent.

    Doing this at fork time, not lazily when a backend is next used, covers
    filesystems the child never touches but whose finalizers would still run
    at child shutdown.
    """
    for ref in list(_owned_filesystems.values()):
        fs = ref()
        if fs is not None:
            _abandon_inherited_filesystem(fs)
    _owned_filesystems.clear()


if hasattr(os, "register_at_fork"):  # not available on Windows
    os.register_at_fork(after_in_child=_abandon_owned_filesystems)


# Filesystems inherited across a fork. Kept referenced so they are never
# garbage-collected (and their session never closed) in the child.
_inherited_filesystems: list[fsspec.AbstractFileSystem] = []


def _abandon_inherited_filesystem(fs: fsspec.AbstractFileSystem) -> None:
    """Make sure the child never closes the parent's connections.

    s3fs registers ``weakref.finalize(fs, close_session, ...)``. Run in the
    child, that closes sockets the child shares with the parent (the epoll
    instance is shared through fork), and the parent's next request hangs
    forever. Detach those finalizers and keep the object alive, so neither
    garbage collection nor interpreter exit in the child closes anything.
    """
    _inherited_filesystems.append(fs)
    registry = getattr(weakref.finalize, "_registry", None)  # CPython internal
    if registry is None:
        return
    for finalizer in list(registry):
        if finalizer.peek() is not None and finalizer.peek()[0] is fs:
            finalizer.detach()


def _reset_fsspec_loop_after_fork() -> None:
    # fsspec < 2025.10 does not reset its global I/O loop in a forked child,
    # so a new async filesystem would wait forever on the parent's dead loop
    # thread. Reset only when that thread is dead (newer fsspec already did).
    thread = fsspec.asyn.iothread[0]
    if thread is not None and not thread.is_alive():
        fsspec.asyn.reset_lock()


def _reraise_oserror(exc: OSError, context: str) -> NoReturn:
    message = f"{context}: {exc.strerror or exc}"
    if exc.errno in _REGION_ERRNOS:
        message += (
            " (check the bucket region: region= in s3_storage_options or "
            "client_kwargs['region_name'])"
        )
    try:
        new = (
            type(exc)(exc.errno, message)
            if exc.errno is not None
            else type(exc)(message)
        )
    except Exception:  # subclass with an unusual constructor
        new = OSError(exc.errno, message) if exc.errno is not None else OSError(message)
    raise new from exc


def _default_credentials_available() -> bool:
    try:
        import botocore.session
    except ImportError:
        return True  # let s3fs decide; it cannot run without botocore anyway
    try:
        return botocore.session.Session().get_credentials() is not None
    except Exception:  # malformed config files etc.
        return False


def _botocore_supports_checksum_config() -> bool:
    try:
        from botocore.config import Config
    except ImportError:
        return False
    return "request_checksum_calculation" in getattr(Config, "OPTION_DEFAULTS", {})
