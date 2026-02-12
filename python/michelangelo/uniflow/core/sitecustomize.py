"""sitecustomize.py - Automatic Uniflow initialization for remote environments.

This module is automatically imported by Python during startup when it's in the
Python path. It runs the uniflow pre-run script to download and apply local
changes in remote containers.

This leverages Python's built-in site initialization mechanism for clean,
automatic execution without explicit container startup script modifications.
"""

import logging
import os
import shutil
import sys
import tarfile
import tempfile
import traceback
from abc import ABC, abstractmethod
from pathlib import Path

import fsspec

# Global flag to ensure file_sync_pre_run only executes once per process
_file_sync_executed = False

# Initialize module-level logger
logger = logging.getLogger("michelangelo.uniflow.sitecustomize")
logger.propagate = False


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
            with fsspec.open(remote_path, "rb") as remote_file:
                with open(local_path, "wb") as local_file:
                    local_file.write(remote_file.read())

            logger.info(f"Successfully downloaded to: {local_path}")
            return True
        except Exception as e:
            logger.error(f"fsspec download failed: {e}")
            return False


def download_and_extract_dev_files(*, downloader: StorageDownloader, logger=None):
    """Download and extract development files from remote storage with following steps:
    1. Check for UF_FILE_SYNC_TARBALL_URL environment variable
    2. Download tarball using appropriate downloader (tb-cli or fsspec)
    3. Extract and replace files in current working directory
    4. Clean up temporary files

    Args:
        downloader: StorageDownloader instance for downloading files
        logger: Optional logger instance (uses module logger if not provided)

    Returns:
        bool: True if files were processed, False if skipped or failed
    """
    # Use module-level logger if none provided
    if logger is None:
        logger = globals()['logger']
    
    # Check for the required environment variable
    remote_file_path = os.environ.get("UF_FILE_SYNC_TARBALL_URL")
    if not remote_file_path:
        logger.info("UF_FILE_SYNC_TARBALL_URL not set, skipping file sync")
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


def file_sync_pre_run(downloader: StorageDownloader, logger=None):
    """Automatically run the pre_run script if environment conditions are met.

    This is the entry point used by sitecustomize.py for automatic execution.
    It includes additional safety checks and logging for the container environment.

    Args:
        downloader: StorageDownloader instance for downloading files (required).
        logger: Optional logger instance. If not provided, uses module-level logger.
    """
    global _file_sync_executed
    # Only run once per Python process
    if _file_sync_executed:
        return
    _file_sync_executed = True

    # Use module-level logger if none provided
    if logger is None:
        logger = globals()['logger']

    # Check if debug mode is enabled via environment variable
    debug_mode = os.environ.get("UF_FILE_SYNC_DEBUG", "").lower() in ("1", "true", "yes")
    
    # Configure logger based on debug mode
    if debug_mode:
        logger.setLevel(logging.INFO)
        handler = logging.StreamHandler(sys.stderr)
        handler.setFormatter(logging.Formatter("[sitecustomize] %(message)s"))
        logger.addHandler(handler)
        
        # Print debug info
        logger.info(f"Python executable: {sys.executable}")
        logger.info(f"Working directory: {os.getcwd()}")
        logger.info(f"UF_FILE_SYNC_TARBALL_URL: {os.environ.get('UF_FILE_SYNC_TARBALL_URL', 'NOT SET')}")
    else:
        # Disable logger completely if not in debug mode
        logger.disabled = True

    try:
        if os.environ.get("UF_FILE_SYNC_TARBALL_URL"):
            logger.info("Development file sync starting...")
            success = download_and_extract_dev_files(
                downloader=downloader, logger=logger
            )
            if success:
                logger.info("Development file sync completed")
            else:
                logger.warning("Development file sync failed (check logs above)")
        else:
            logger.info("No development files to sync (UF_FILE_SYNC_TARBALL_URL not set)")
    except Exception as e:
        logger.error(f"Error: {e}")
        logger.error(f"Traceback: {traceback.format_exc()}")
        # Continue despite errors to avoid breaking containers

# Run the file sync pre-run functionality automatically when this module is imported
if __name__ != "__main__":
    file_sync_pre_run(downloader=FsspecDownloader())
