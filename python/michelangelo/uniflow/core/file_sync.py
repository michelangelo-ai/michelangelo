from abc import ABC, abstractmethod
from pathlib import Path
from typing import Optional
import uuid
import tempfile
import tarfile
import io
import os
import logging
import subprocess
import fsspec


log = logging.getLogger(__name__)


class FileSync(ABC):
    def __init__(self):
        self._file_name = None
        self._remote_file_path = None

    @abstractmethod
    def get_git_sha(self) -> str:
        pass

    @abstractmethod
    def upload_tarball(self, local_path: str, remote_path: str):
        pass

    def get_random_file_name(self) -> str:
        """
        Get a random file name for the tarball.

        Uses "file-sync" as the prefix for all file sync tarballs.

        Returns:
            The random file name in format: file-sync-{uuid}.tar.gz
        """
        return f"file-sync-{uuid.uuid4().hex}.tar.gz"

    def get_file_name(self) -> str:
        """
        Get the file name for the tarball.

        Returns:
            The file name, or None if the file name is not set
        """
        if self._file_name is None:
            self._file_name = self.get_random_file_name()
        return self._file_name

    def get_remote_file_path(self) -> str:
        """
        Get the remote file path for the tarball.

        Returns:
            The remote file path, or None if the remote file path is not set
        """
        base_path = os.environ.get("UF_FILE_SYNC_STORAGE_URL", "s3://default/uniflow")
        if self._remote_file_path is None:
            self._remote_file_path = (
                f"{base_path}/{self.get_file_name()}"
            )
        return self._remote_file_path

    def create_diff_tarball_bytes(self) -> Optional[bytes]:
        """
        Create a tarball of the changed files in the Git repository.

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
                    # Strip 'python/' prefix if present since Dockerfile copies python/ to /app
                    arcname = file_path
                    if arcname.startswith("python/"):
                        arcname = arcname[7:]  # Remove 'python/' prefix
                    tar.add(path, arcname=arcname)
        return bb.getvalue()

    def create_and_upload_tarball(self) -> str:
        """
        Create a tarball of the changed files in the Git repository and upload it to the remote storage.

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
    def __init__(self, docker_image: Optional[str] = None):
        super().__init__()
        self._docker_image = docker_image

    def get_git_sha(self) -> Optional[str]:
        """
        Get the Git SHA from the Docker image.

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
            log.warning("Docker package not available, skipping Git SHA extraction from image")
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
                f"Git SHA not found in Docker image '{docker_image}' labels or environment variables. "
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
        """
        Upload tarball to storage using fsspec.

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
