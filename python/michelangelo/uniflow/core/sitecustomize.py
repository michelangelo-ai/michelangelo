"""
sitecustomize.py - Automatic Uniflow initialization for remote environments.

This module is automatically imported by Python during startup when it's in the
Python path. It runs the uniflow pre-run script to download and apply local
changes in remote containers.

This leverages Python's built-in site initialization mechanism for clean,
automatic execution without explicit container startup script modifications.
"""

import os
import tempfile
import tarfile
import shutil
import traceback
import logging
from pathlib import Path
from abc import ABC, abstractmethod


def _get_logger(context="uniflow_task_pre_run"):
    """Get a configured logger for the pre-run script."""
    logging.basicConfig(level=logging.INFO, format=f"[{context}] %(message)s")
    return logging.getLogger(f"uniflow.{context}")


class StorageDownloader(ABC):
    """Abstract interface for downloading files from remote storage."""

    @abstractmethod
    def download(
        self, remote_path: str, local_path: Path, logger: logging.Logger
    ) -> bool:
        """
        Download a file from remote storage to local path.

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
            import fsspec
        except ImportError:
            logger.error("fsspec not installed. Install with: pip install fsspec s3fs")
            return False

        try:
            logger.info(f"Downloading from: {remote_path}")
            with fsspec.open(remote_path, "rb") as remote_file:
                with open(local_path, "wb") as local_file:
                    local_file.write(remote_file.read())

            logger.info(f"Successfully downloaded to: {local_path}")
            return True
        except Exception as e:
            logger.error(f"fsspec download failed: {e}")
            return False


def download_and_extract_dev_files(*, downloader: StorageDownloader, logger=None):
    """
    Download and extract development files from remote storage with following steps:
    1. Check for DEV_RUN_REMOTE_FILE_PATH environment variable
    2. Download tarball using appropriate downloader (tb-cli or fsspec)
    3. Extract and replace files in current working directory
    4. Clean up temporary files

    Args:
        logger: Optional logger instance (will create one if not provided)
        downloader: Optional StorageDownloader instance (auto-detected if not provided)

    Returns:
        bool: True if files were processed, False if skipped or failed
    """
    if logger is None:
        logger = _get_logger()

    # Check for the required environment variable
    remote_file_path = os.environ.get("DEV_RUN_REMOTE_FILE_PATH")
    if not remote_file_path:
        logger.info("DEV_RUN_REMOTE_FILE_PATH not set, skipping file sync")
        return False
    logger.info(f"Downloading development files from: {remote_file_path}")

    try:
        with tempfile.TemporaryDirectory() as tmp_dir:
            tarball_path = Path(tmp_dir) / "dev_run.tar.gz"

            # Download tarball using the configured downloader
            if not downloader.download(remote_file_path, tarball_path, logger):
                return False

            # Extract tarball
            logger.info("Extracting files...")
            try:
                with tarfile.open(tarball_path, "r:gz") as tar:
                    tar.extractall(tmp_dir)
            except tarfile.TarError as e:
                logger.error(f"Extraction failed: {e}")
                return False

            # Remove the tarball to avoid copying it
            tarball_path.unlink()

            # Copy extracted files to current directory
            repo_root = Path.cwd()
            logger.info(f"Applying changes to: {repo_root}")

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
                    logger.info(f"  ✓ Applied: {rel_path}")

            logger.info(f"Applied {file_count} file(s) successfully")
            return True

    except Exception as e:
        logger.error(f"Unexpected error: {e}")
        return False


def auto_run_if_enabled():
    """
    Automatically run the pre-run script if environment conditions are met.

    This is the entry point used by sitecustomize.py for automatic execution.
    It includes additional safety checks and logging for the container environment.
    """
    logger = _get_logger("sitecustomize")

    if os.environ.get("DEV_RUN_REMOTE_FILE_PATH"):
        logger.info("Development file sync starting...")
        success = download_and_extract_dev_files(
            downloader=FsspecDownloader(), logger=logger
        )
        if success:
            logger.info("Development file sync completed")
        else:
            logger.warning("Development file sync failed (check logs above)")
    else:
        logger.info("No development files to sync (DEV_RUN_REMOTE_FILE_PATH not set)")


# Run the pre-run functionality automatically when this module is imported
# (but not when executed directly as a script)
if __name__ != "__main__":
    import sys

    print(f"[sitecustomize] Python executable: {sys.executable}")
    print(f"[sitecustomize] Working directory: {os.getcwd()}")
    print(
        f"[sitecustomize] DEV_RUN_REMOTE_FILE_PATH: {os.environ.get('DEV_RUN_REMOTE_FILE_PATH', 'NOT SET')}"
    )

    try:
        auto_run_if_enabled()
    except Exception as e:
        # Never let sitecustomize errors break Python startup
        import traceback

        print(f"[sitecustomize] Error: {e}")
        print(f"[sitecustomize] Traceback: {traceback.format_exc()}")
        # Continue despite errors to avoid breaking containers
