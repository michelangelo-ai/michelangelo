"""File synchronization utilities for Uniflow development workflows."""

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
    """Abstract base class for file synchronization operations."""

    def __init__(self):
        """Initialize the FileSync instance."""
        self._file_name = None
        self._remote_file_path = None

    @abstractmethod
    def get_git_sha(self) -> str:
        """Get the Git SHA of the current commit.

        Returns:
            str: The Git SHA hash.
        """
        pass

    @abstractmethod
    def upload_tarball(self, local_path: str, remote_path: str):
        """Upload a tarball to remote storage.

        Args:
            local_path: Local path to the tarball file.
            remote_path: Remote destination path.
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
        """Get the file name for the tarball.

        Returns:
            The file name, or None if the file name is not set
        """
        if self._file_name is None:
            self._file_name = self.get_random_file_name()
        return self._file_name

    def get_remote_file_path(self) -> str:
        """Get the remote file path for the tarball.

        Returns:
            The remote file path, or None if the remote file path is not set
        """
        base_path = os.environ.get("UF_FILE_SYNC_STORAGE_URL", "s3://default/uniflow")
        if self._remote_file_path is None:
            self._remote_file_path = f"{base_path}/{self.get_file_name()}"
        return self._remote_file_path

    def create_diff_tarball_bytes(self) -> Optional[bytes]:
        """Create a tarball of the changed files in the Git repository.

        Returns:
            The tarball as bytes, or None if no changed files are found
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

        Returns:
            The remote file path, or None if the remote file path is not set
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
    """Default implementation of FileSync using Docker and fsspec."""

    def __init__(self, docker_image: Optional[str] = None):
        """Initialize DefaultFileSync.

        Args:
            docker_image: Optional Docker image name to extract Git SHA from.
        """
        super().__init__()
        self._docker_image = docker_image

    def get_git_sha(self) -> Optional[str]:
        """Get the Git SHA from the Docker image.

        If the Git SHA is not found in the Docker image labels or environment variables,
        returns None instead of raising an error. This allows file sync to work even
        when the Docker image doesn't have Git metadata embedded.

        Args:
            docker_image: Docker image to get the Git SHA from

        Returns:
            The Git SHA if found, None otherwise
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
            Exception: If upload fails
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
    """Abstract interface for downloading files from remote storage."""

    @abstractmethod
    def download(
        self, remote_path: str, local_path: Path, logger: logging.Logger
    ) -> bool:
        """Download a file from remote storage to local path.

        Args:
            remote_path: The remote storage path (e.g., s3://bucket/key)
            local_path: The local filesystem path to save the file
            logger: Logger instance for reporting progress/errors

        Returns:
            bool: True if download succeeded, False otherwise
        """
        pass


class FsspecDownloader(StorageDownloader):
    """Downloader using fsspec for OSS S3-compatible storage."""

    def download(
        self, remote_path: str, local_path: Path, logger: logging.Logger
    ) -> bool:
        """Download using fsspec (works with S3, MinIO, etc)."""
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


def download_and_extract_dev_files(*, downloader: StorageDownloader):
    """Download and extract development files from remote storage.

    Steps:
    1. Check for UF_FILE_SYNC_TARBALL_URL environment variable
    2. Download tarball using appropriate downloader (tb-cli or fsspec)
    3. Extract and replace files in current working directory
    4. Clean up temporary files

    Args:
        downloader: StorageDownloader instance for downloading files

    Returns:
        bool: True if files were processed, False if skipped or failed
    """
    # Check for the required environment variable
    remote_file_path = os.environ.get("UF_FILE_SYNC_TARBALL_URL")
    if not remote_file_path:
        log.info("UF_FILE_SYNC_TARBALL_URL not set, skipping file sync")
        return False
    log.info(f"Downloading development files from: {remote_file_path}")

    try:
        with tempfile.TemporaryDirectory() as tmp_dir:
            tarball_path = Path(tmp_dir) / "dev_run.tar.gz"

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


def file_sync_pre_run(downloader: StorageDownloader):
    """Automatically run the pre_run script if environment conditions are met.

    This is the entry point used by sitecustomize.py for automatic execution.
    It includes additional safety checks and logging for the container environment.

    Args:
        downloader: StorageDownloader instance for downloading files (required).
    """
    global _file_sync_executed
    # Only run once per Python process
    if _file_sync_executed:
        return
    _file_sync_executed = True

    if os.environ.get("UF_FILE_SYNC_TARBALL_URL"):
        try:
            log.info("Development file sync starting...")
            success = download_and_extract_dev_files(downloader=downloader)
            if success:
                log.info("Development file sync completed")
            else:
                log.warning("Development file sync failed (check logs above)")
        except Exception as e:
            log.error(f"Error during file sync: {e}")
            # Continue despite errors to avoid breaking containers
    else:
        log.info("No development files to sync (UF_FILE_SYNC_TARBALL_URL not set)")
