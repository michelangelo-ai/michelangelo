"""File sync functions and classes for Uniflow development workflows.

This module provides the file sync mechanism that allows developers to sync
local code changes to remote execution environments without rebuilding Docker
images.

## Architecture

The file sync process operates in two phases:

### Phase 1: Local Development Environment
Uses `FileSync.create_and_upload_tarball()` to:
1. Detect code differences between local workspace and the base commit
2. Create a gzipped tarball containing only the changed files
3. Upload the tarball to remote storage (S3/MinIO)

### Phase 2: Remote Execution Environment
Uses `run()` as the entry point (called by sitecustomize.py) to:
1. Download the tarball from remote storage
2. Extract and apply the changed files to the container filesystem
3. Log the file sync process with [file_sync] prefix
"""

import io
import logging
import os
import shutil
import subprocess
import tarfile
import tempfile
import uuid
from abc import ABC, abstractmethod
from pathlib import Path
from typing import Optional

import fsspec

log = logging.getLogger(__name__)

# Global flag to ensure file_sync_pre_run only executes once per process
_file_sync_executed = False


class FileSync(ABC):
    """Abstract base class for file sync operations.

    FileSync provides a framework for detecting local code changes and uploading
    them to remote storage for consumption by remote execution environments. It
    handles Git integration to identify changed files, tarball creation, and
    upload files logic.

    The class supports two main workflows:
    1. Development workflow: Compare local changes against a Docker image's Git SHA
    2. Fallback workflow: Capture all uncommitted changes when no Git SHA is available

    Subclasses must implement:
        - get_git_sha(): Extract Git SHA from image metadata (labels/env vars)
        - upload_tarball(): Upload tarball to specific storage backend (S3, MinIO, etc.)

    Attributes:
        _file_name: Cached random filename for the tarball
            (format: file-sync-{uuid}.tar.gz)
        _remote_file_path: Cached full remote storage path for the tarball

    Environment Variables:
        UF_FILE_SYNC_STORAGE_URL: Base URL for storing tarballs. Defaults to
            's3://default/uniflow' if not set.
    """

    def __init__(self):
        """Initialize the FileSync instance."""
        self._file_name = None
        self._remote_file_path = None

    @abstractmethod
    def get_git_sha(self) -> str:
        """Get the Git SHA of the base commit in the remote execution environment.

        This method extracts the Git commit hash that represents the code version
        deployed in the remote execution environment. The Git SHA is used to
        determine which files have changed between the deployed code and the local
        development environment.

        The base commit could be obtained from various sources depending on the
        deployment framework:
            - Docker images: labels, environment variables, or build args
            - Kubernetes deployments: ConfigMaps, annotations, or pod labels
            - VM images: metadata files, cloud instance tags
            - Container registries: image tags or manifest annotations
            - CI/CD systems: deployment metadata or artifact manifests

        Implementations should attempt to extract the Git SHA from whatever
        metadata is available in the deployment framework. If no Git SHA is
        available, returning None causes the file sync to fall back to capturing
        all uncommitted local changes.

        Common metadata locations:
            - Image/container labels: git.commit, git.sha, vcs.revision
            - Environment variables: GIT_SHA, GIT_COMMIT, SOURCE_VERSION
            - Manifest files: .git-sha, BUILD_INFO, version.json
            - Cloud metadata: deployment tags, instance attributes

        Returns:
            str: The Git SHA hash of the base commit if found, None otherwise.
                Returning None is acceptable and triggers the fallback workflow
                of capturing all uncommitted local changes.

        Note:
            This method is called during tarball creation in the local development
            environment, not in the remote container. It runs before the code is
            uploaded to remote storage.
        """
        pass

    @abstractmethod
    def upload_tarball(self, local_path: str, remote_path: str):
        """Upload a tarball to remote storage.

        This method handles the actual upload of the tarball containing changed
        files to remote storage where it can be accessed by remote execution
        environments. The upload must be synchronous and should raise an exception
        if the upload fails.

        Implementations should:
            - Support the storage protocol specified in UF_FILE_SYNC_STORAGE_URL
            - Handle authentication/credentials for the storage backend
            - Verify the upload succeeded before returning
            - Raise descriptive exceptions on failure
            - Log progress information using the module logger

        The remote_path format depends on the storage backend:
            - S3/MinIO: s3://bucket-name/path/to/file-sync-{uuid}.tar.gz
            - Local file: file:///absolute/path/to/file-sync-{uuid}.tar.gz
            - Azure: az://container/path/to/file-sync-{uuid}.tar.gz

        Args:
            local_path: Absolute path to the tarball file on local filesystem.
                This file contains the gzipped tar archive of changed files.
            remote_path: Full remote storage URL where the tarball should be
                uploaded. Generated by get_remote_file_path().

        Raises:
            Exception: If upload fails for any reason (auth, network, permissions).
                The exception should include details to help diagnose the failure.

        Note:
            This method is called during workflow submission in the local
            development environment, not in remote containers. The uploaded
            tarball is later downloaded by sitecustomize.py in remote environments.
        """
        pass

    def get_random_file_name(self) -> str:
        """Get a random file name for the tarball.

        Uses "file-sync" as the prefix for all file sync tarballs.

        Returns:
            The random file name in format: file-sync-{uuid}.tar.gz
        """
        return f"file-sync-{uuid.uuid4().hex}.tar.gz"

    def get_file_name(self) -> str:
        """Get the file name for the tarball with caching.

        Generates a random filename on first call and caches it for subsequent calls.
        This ensures the same filename is used throughout the file sync lifecycle.

        Returns:
            str: The cached or newly generated filename in format:
                file-sync-{uuid}.tar.gz
        """
        if self._file_name is None:
            self._file_name = self.get_random_file_name()
        return self._file_name

    def get_remote_file_path(self) -> str:
        """Get the full remote storage path for the tarball with caching.

        Constructs the complete storage URL by combining the base path from
        UF_FILE_SYNC_STORAGE_URL with the generated filename. The path is
        cached after first construction.

        Returns:
            str: The full remote path (e.g., s3://default/uniflow/file-sync-{uuid}.tar.gz)

        Environment Variables:
            UF_FILE_SYNC_STORAGE_URL: Base storage URL. Defaults to 's3://default/uniflow'
        """
        base_path = os.environ.get("UF_FILE_SYNC_STORAGE_URL", "s3://default/uniflow")
        if self._remote_file_path is None:
            self._remote_file_path = f"{base_path}/{self.get_file_name()}"
        return self._remote_file_path

    def create_diff_tarball_bytes(self) -> Optional[bytes]:
        """Create a tarball of changed files detected by Git.

        Uses Git to identify changed files by comparing against the base commit
        (if available) or capturing all uncommitted changes.

        The method handles two scenarios:
        1. Base commit available: Compares current state vs base commit
        2. No base commit: Captures all uncommitted changes
           (staged + unstaged + untracked)

        Returns:
            Optional[bytes]: Gzipped tar archive as bytes, or None if no
                changes detected
        """
        commit_sha = self.get_git_sha()

        # Get the Git root directory
        git_root_result = subprocess.run(
            ["git", "rev-parse", "--show-toplevel"],
            cwd=os.getcwd(),
            capture_output=True,
            text=True,
            check=True,
        )
        git_root = git_root_result.stdout.strip()
        log.info(f"Git root: {git_root}, Current dir: {os.getcwd()}")

        # Get modified files
        if commit_sha and commit_sha.lower() not in ["unknown", "none"]:
            # If we have a valid Git SHA from the Docker image, compare against it
            log.info(f"Comparing against commit SHA: {commit_sha}")
            result = subprocess.run(
                ["git", "diff", "--name-only", commit_sha],
                cwd=git_root,
                capture_output=True,
                text=True,
                check=True,
            )
            changed_files = result.stdout.strip().splitlines()
        else:
            # If no Git SHA, get all uncommitted changes (staged + unstaged)
            log.info("No Git SHA found, getting all uncommitted changes")
            result = subprocess.run(
                ["git", "diff", "--name-only", "HEAD"],
                cwd=git_root,
                capture_output=True,
                text=True,
                check=True,
            )
            changed_files = result.stdout.strip().splitlines()

        # Also get untracked files (new files not in Git)
        untracked_result = subprocess.run(
            ["git", "ls-files", "--others", "--exclude-standard"],
            cwd=git_root,
            capture_output=True,
            text=True,
            check=True,
        )
        untracked_files = untracked_result.stdout.strip().splitlines()

        # Combine both lists
        all_changed_files = list(set(changed_files + untracked_files))

        if not all_changed_files:
            log.info("No changed files found.")
            return None

        log.info(f"Changed files: {all_changed_files}")
        bb = io.BytesIO()
        with tarfile.open(fileobj=bb, mode="w:gz", dereference=True) as tar:
            for file_path in all_changed_files:
                path = Path(git_root) / file_path  # Make path relative to git root
                if path.exists():
                    # Strip 'python/' prefix if present since
                    # Dockerfile copies python/ to /app
                    arcname = file_path
                    if arcname.startswith("python/"):
                        arcname = arcname[7:]  # Remove 'python/' prefix
                    tar.add(path, arcname=arcname)
        return bb.getvalue()

    def create_and_upload_tarball(self) -> str:
        """Create a tarball of changed files and upload to remote storage.

        This is the main entry point that orchestrates the complete file sync upload
        workflow: detecting changes, creating tarball, and uploading to storage.

        Workflow:
        1. Call create_diff_tarball_bytes() to get changed files
        2. Write tarball to temporary file
        3. Upload to remote storage via upload_tarball()
        4. Clean up temporary files

        Returns:
            str: The remote storage URL if upload succeeded, empty string if no changes

        Raises:
            Exception: If tarball creation or upload fails
        """
        with tempfile.TemporaryDirectory() as tmp_dir:
            local_path = os.path.join(tmp_dir, self.get_file_name())
            tarball = self.create_diff_tarball_bytes()
            if tarball is None:
                log.info("No tarball created, skipping upload.")
                return ""
            with open(local_path, "wb") as f:
                f.write(tarball)
            log.info(f"Uploading tarball to {self.get_remote_file_path()}")
            self.upload_tarball(local_path, self.get_remote_file_path())
        return self.get_remote_file_path()


class DefaultFileSync(FileSync):
    """Default implementation of FileSync using Docker and fsspec.

    This implementation integrates with Docker to extract Git SHA information from
    container image metadata and uses fsspec for flexible storage backend support
    (S3, MinIO, local filesystem, etc.).

    The class attempts to find Git SHA in the following order:
    1. Docker image labels: git.commit, git.sha, vcs.revision,
       org.opencontainers.image.revision
    2. Docker image environment variables: GIT_SHA
    3. If not found, falls back to comparing all uncommitted local changes

    This approach enables accurate change detection in development workflows where
    the remote environment runs code from a specific Git commit baked into the
    Docker image.

    Args:
        docker_image: Optional Docker image name to extract Git SHA from.
            If None or if extraction fails, all uncommitted changes are captured.

    Note:
        Requires the 'docker' Python package to be installed. If not available,
        Git SHA extraction is skipped and the fallback workflow is used.
    """

    def __init__(self, docker_image: Optional[str] = None):
        """Initialize DefaultFileSync.

        Args:
            docker_image: Optional Docker image name to extract Git SHA from.
        """
        super().__init__()
        self._docker_image = docker_image

    def get_git_sha(self) -> Optional[str]:
        """Get the Git SHA from Docker image metadata.

        Inspects the Docker image to extract Git commit information from labels
        or environment variables. This implementation handles missing Docker package
        and images gracefully by returning None to trigger fallback behavior.

        The method searches in order:
        1. Image labels (git.commit, git.sha, vcs.revision,
           org.opencontainers.image.revision)
        2. Environment variables (GIT_SHA)

        Returns:
            Optional[str]: Git SHA if found, None if not found or extraction fails
        """
        # Check if docker package is available
        try:
            import docker
        except ImportError:
            log.warning(
                "Docker package not available, skipping Git SHA extraction from image"
            )
            return None

        docker_image = self._docker_image
        try:
            client = docker.from_env()
            image = client.images.get(docker_image)
            # Check labels
            labels = image.labels or {}
            for key in [
                "git.commit",
                "git.sha",
                "vcs.revision",
                "org.opencontainers.image.revision",
            ]:
                if key in labels:
                    log.info(f"Found Git SHA in label '{key}': {labels[key]}")
                    return labels[key]
            # Check environment variables
            config = image.attrs.get("Config", {})
            env_vars = config.get("Env", [])
            for env in env_vars:
                if env.startswith("GIT_SHA="):
                    git_sha = env.split("=", 1)[1]
                    log.info(f"Found Git SHA in environment variable: {git_sha}")
                    return git_sha

            log.info(
                f"Git SHA not found in Docker image '{docker_image}' "
                "labels or environment variables. "
                "Will create tarball with all uncommitted changes."
            )
            return None

        except Exception as e:
            log.info(
                f"Failed to inspect Docker image '{docker_image}': {e}. "
                "Will create tarball with all uncommitted changes."
            )
            return None

    def upload_tarball(self, local_path: str, remote_path: str):
        """Upload tarball to storage using fsspec.

        Args:
            local_path: Local path to the tarball file
            remote_path: Remote storage path where the tarball should be uploaded

        Raises:
            Exception: If upload fails (auth, network, permissions, etc.)
        """
        try:
            log.info(f"Uploading tarball from {local_path} to {remote_path}")

            with open(local_path, "rb") as local_file:
                tarball_bytes = local_file.read()

            with fsspec.open(remote_path, "wb") as remote_file:
                remote_file.write(tarball_bytes)

            log.info(f"Successfully uploaded tarball to: {remote_path}")

        except Exception as e:
            log.error(f"Failed to upload tarball: {e}")
            raise


class StorageDownloader(ABC):
    """Abstract interface for downloading files from remote storage.

    This interface abstracts the storage backend used for downloading file sync
    tarballs in remote execution environments. Implementations can support different
    storage systems (S3, MinIO, GCS, Azure Blob, etc.) while providing a consistent
    API for the file sync mechanism.

    The downloader is used by sitecustomize.py during Python initialization to
    retrieve code changes uploaded by the local development environment.
    """

    @abstractmethod
    def download(
        self, remote_path: str, local_path: Path, logger: logging.Logger
    ) -> bool:
        """Download a file from remote storage to local path.

        This method is called during Python initialization in remote execution
        environments to download tarballs containing code changes. It must handle
        all aspects of the download including authentication, error handling, and
        progress logging.

        The method should be robust and fail gracefully:
            - Handle authentication/credentials appropriately for their backend
            - Provide informative error messages via the logger parameter
            - Return False on failure rather than raising exceptions
            - Support the storage URL format used by FileSync.upload_tarball()

        Args:
            remote_path: The remote storage URL (e.g., s3://bucket/key,
                file:///path, az://container/blob). Format matches what was
                passed to FileSync.upload_tarball().
            local_path: The local filesystem Path where the downloaded file
                should be saved. Parent directories are guaranteed to exist.
            logger: Logger instance for reporting progress and errors. Use
                logger.info() for progress, logger.error() for failures.

        Returns:
            bool: True if download succeeded and file was saved to local_path,
                False if download failed for any reason.

        Note:
            This method runs in remote containers during Python startup
            (via sitecustomize.py), so it must be efficient and not block for
            extended periods. Failed downloads should not prevent container
            startup - just return False and let execution continue with the
            Docker image's baked-in code.
        """
        pass


class FsspecDownloader(StorageDownloader):
    """Downloader using fsspec for S3-compatible storage.

    This implementation uses the fsspec library to download files from S3-compatible
    storage systems including AWS S3, MinIO, and other object stores that implement
    the S3 API.

    Fsspec automatically handles:
        - AWS credentials from environment variables
          (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY)
        - S3 endpoint configuration (AWS_ENDPOINT_URL for MinIO/custom endpoints)
        - Multiple storage protocols (s3://, file://, etc.)
        - Retry logic and error handling

    This is the recommended downloader for the Uniflow sandbox environment which
    uses MinIO for S3-compatible object storage.

    Usage:
        downloader = FsspecDownloader()
        success = downloader.download(
            "s3://default/uniflow/file-sync-abc123.tar.gz",
            Path("/tmp/file_sync.tar.gz"),
            logger
        )
    """

    def download(
        self, remote_path: str, local_path: Path, logger: logging.Logger
    ) -> bool:
        """Download tarball from remote storage using fsspec.

        Opens remote file in binary read mode and writes to local filesystem.
        Automatically handles credentials from environment variables based on
        the storage protocol (AWS_* vars for S3/MinIO).

        Args:
            remote_path: Remote storage URL (e.g., s3://bucket/key)
            local_path: Local Path to save the downloaded file
            logger: Logger for progress and error reporting

        Returns:
            bool: True if download succeeded, False on any error
        """
        try:
            logger.info(f"Downloading from: {remote_path}")
            with (
                fsspec.open(remote_path, "rb") as remote_file,
                open(local_path, "wb") as local_file,
            ):
                local_file.write(remote_file.read())

            logger.info(f"Successfully downloaded to: {local_path}")
            return True
        except Exception as e:
            logger.error(f"fsspec download failed: {e}")
            return False


def _download_and_extract_dev_files(*, downloader: StorageDownloader):
    """Internal: Download and extract development files from remote storage.

    This is a private helper function that implements the remote-side of file sync.
    It downloads the tarball of changed files and applies them to the container
    filesystem. Should not be called directly - use run() instead.

    Workflow:
    1. Check UF_FILE_SYNC_TARBALL_URL environment variable
    2. Download tarball to temporary directory using provided downloader
    3. Extract tarball contents
    4. Copy files to current working directory (typically /app)
    5. Clean up temporary files

    The function is resilient to failures - if download or extraction fails, it
    returns False and logs errors but doesn't raise exceptions.

    Args:
        downloader: StorageDownloader instance configured for the storage backend

    Returns:
        bool: True if files were successfully downloaded and applied,
              False if skipped (no URL) or failed

    Environment Variables:
        UF_FILE_SYNC_TARBALL_URL: Remote storage URL for the tarball. If not set,
            file sync is skipped.
    """
    # Check for the required environment variable
    remote_file_path = os.environ.get("UF_FILE_SYNC_TARBALL_URL")
    if not remote_file_path:
        log.info("UF_FILE_SYNC_TARBALL_URL not set, skipping file sync")
        return False
    log.info(f"Downloading development files from: {remote_file_path}")

    try:
        with tempfile.TemporaryDirectory() as tmp_dir:
            tarball_path = Path(tmp_dir) / "file_sync.tar.gz"

            # Download tarball using the configured downloader
            if not downloader.download(remote_file_path, tarball_path, log):
                return False

            # Extract tarball
            log.info("Extracting files...")
            try:
                with tarfile.open(tarball_path, "r:gz") as tar:
                    tar.extractall(tmp_dir)
            except tarfile.TarError as e:
                log.error(f"Extraction failed: {e}")
                return False

            # Remove the tarball to avoid copying it
            tarball_path.unlink()

            # Copy extracted files to current directory
            repo_root = Path.cwd()
            log.info(f"Applying changes to: {repo_root}")

            file_count = 0
            for file_path in Path(tmp_dir).rglob("*"):
                if file_path.is_file():
                    rel_path = file_path.relative_to(tmp_dir)
                    target_file = repo_root / rel_path

                    # Create parent directories if needed
                    target_file.parent.mkdir(parents=True, exist_ok=True)

                    # Copy file with metadata preservation
                    shutil.copy2(file_path, target_file)
                    file_count += 1
                    log.info(f"  ✓ Applied: {rel_path}")

            log.info(f"Applied {file_count} file(s) successfully")
            return True

    except Exception as e:
        log.error(f"Unexpected error: {e}")
        return False


def _file_sync_pre_run(downloader: StorageDownloader):
    """Internal: Execute file sync workflow without logging setup.

    This is a private helper function that contains the core file sync logic.
    It should not be called directly - use run() instead.

    The function:
    1. Checks if file sync already executed (uses global flag)
    2. Downloads tarball if UF_FILE_SYNC_TARBALL_URL is set
    3. Extracts and applies changed files
    4. Handles errors gracefully to avoid breaking containers

    Args:
        downloader: StorageDownloader instance for downloading files

    Note:
        Uses module-level logger which must be configured before calling.
        Safe to call multiple times - only executes once per process.
    """
    global _file_sync_executed
    # Only run once per Python process
    if _file_sync_executed:
        return
    _file_sync_executed = True

    if os.environ.get("UF_FILE_SYNC_TARBALL_URL"):
        try:
            log.info("Development file sync starting...")
            success = _download_and_extract_dev_files(downloader=downloader)
            if success:
                log.info("Development file sync completed")
            else:
                log.warning("Development file sync failed (check logs above)")
        except Exception as e:
            log.error(f"Error during file sync: {e}")
            # Continue despite errors to avoid breaking containers
    else:
        log.info("No development files to sync (UF_FILE_SYNC_TARBALL_URL not set)")


def run(downloader: StorageDownloader):  # pragma: no cover
    """Run file sync with automatic logging setup.

    This is the main public entry point for file sync. It sets up structured
    logging (if debug mode is enabled), then executes the file sync workflow
    to download and apply local code changes to the remote environment.

    The function is designed to be called from sitecustomize.py during Python
    startup initialization. It's safe to call multiple times - the underlying
    file sync logic only executes once per process.

    Workflow:
    1. Configure logging if UF_FILE_SYNC_DEBUG is enabled
    2. Log startup diagnostics (Python executable, working directory, URLs)
    3. Execute file sync via internal _file_sync_pre_run()
    4. Handle exceptions gracefully with detailed error logging

    Environment Variables:
        UF_FILE_SYNC_DEBUG: Enable debug logging (1, true, yes). When enabled,
            all file sync operations are logged with [file_sync] prefix.
        UF_FILE_SYNC_TARBALL_URL: Remote storage URL for the tarball containing
            changed files. If not set, file sync is skipped.

    Args:
        downloader: StorageDownloader instance for downloading files from remote
            storage. Use FsspecDownloader() for S3/MinIO-compatible storage.

    Example:
        # Typical usage in sitecustomize.py
        from michelangelo.uniflow.core import file_sync
        file_sync.run(downloader=file_sync.FsspecDownloader())

    Note:
        This function never raises exceptions - errors are logged and execution
        continues to avoid breaking container startup.
    """
    import sys
    import traceback

    # Check if debug mode is enabled via environment variable
    debug_mode = os.environ.get("UF_FILE_SYNC_DEBUG", "").lower() in (
        "1",
        "true",
        "yes",
    )

    configured_log = None
    if debug_mode:
        # Create shared handler with [file_sync] prefix
        handler = logging.StreamHandler(sys.stderr)
        handler.setFormatter(logging.Formatter("[file_sync] %(message)s"))

        # Configure loggers for sitecustomize and file_sync modules
        for logger_name in ["sitecustomize", "michelangelo.uniflow.core.file_sync"]:
            logger = logging.getLogger(logger_name)
            logger.setLevel(logging.INFO)
            logger.addHandler(handler)
            logger.propagate = False
            if logger_name == "michelangelo.uniflow.core.file_sync":
                configured_log = logger

        # Log startup diagnostics
        if configured_log:
            configured_log.info(f"Python executable: {sys.executable}")
            configured_log.info(f"Working directory: {os.getcwd()}")
            configured_log.info(
                "UF_FILE_SYNC_TARBALL_URL: "
                f"{os.environ.get('UF_FILE_SYNC_TARBALL_URL', 'NOT SET')}"
            )

    try:
        _file_sync_pre_run(downloader=downloader)
    except Exception as e:
        if debug_mode and configured_log:
            configured_log.error(f"Error: {e}")
            configured_log.error(f"Traceback: {traceback.format_exc()}")
        # Continue despite errors to avoid breaking containers
