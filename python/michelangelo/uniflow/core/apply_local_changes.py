from abc import ABC, abstractmethod
from pathlib import Path
from datetime import datetime
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
_DEFAULT_BASE_PROJECTS_PATH = "s3://default/uniflow"


class UniflowDevRunFileBuilder(ABC):
    def __init__(self, project: Optional[str] = None, pipeline: Optional[str] = None):
        self._project = project
        self._pipeline = pipeline
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

        Returns:
            The random file name, or None if the random file name is not set
        """
        date_str = datetime.now().strftime("%Y-%m-%d-%H-%M-%S")
        random_suffix = uuid.uuid4().hex[:8]
        prefix = self._pipeline if self._pipeline else "uniflow"
        return f"{prefix}-{date_str}-{random_suffix}.tar.gz"

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
        base_path = (
            os.environ.get("UF_BASE_PROJECTS_PATH") or _DEFAULT_BASE_PROJECTS_PATH
        )
        if self._remote_file_path is None:
            if self._project:
                self._remote_file_path = (
                    f"{base_path}/{self._project}/{self.get_file_name()}"
                )
            else:
                tmp_path = os.path.join(base_path, "../tmp", uuid.uuid4().hex[:8])
                self._remote_file_path = f"{tmp_path}/{self.get_file_name()}"
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
        result = subprocess.run(
            ["git", "diff", "--name-only", commit_sha],
            cwd=git_root,  # Use git root instead of current dir
            capture_output=True,
            text=True,
            check=True,
        )
        changed_files = result.stdout.strip().splitlines()

        # Also get untracked files (new files not in Git)
        untracked_result = subprocess.run(
            ["git", "ls-files", "--others", "--exclude-standard"],
            cwd=git_root,  # Use git root instead of current dir
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


class UniflowDevRunFileBuilderOSS(UniflowDevRunFileBuilder):
    def __init__(self, project: str, pipeline: str, docker_image: str):
        super().__init__(project, pipeline)
        self._docker_image = docker_image

    def get_git_sha(self) -> str:
        """
        Get the Git SHA from the Docker image.

        Args:
            docker_image: Docker image to get the Git SHA from

        Returns:
            The Git SHA

        Raises:
            ValueError: If the Git SHA is not found in the image labels or environment variables
            Exception: If the Docker image is not found
        """
        # Lazy import docker to avoid dependency issues in containers
        import docker

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
                    return labels[key]
            # Check environment variables
            config = image.attrs.get("Config", {})
            env_vars = config.get("Env", [])
            for env in env_vars:
                if env.startswith("GIT_SHA="):
                    return env.split("=", 1)[1]
            raise ValueError(
                "Git SHA not found in image labels or environment variables."
            )

        except Exception as e:
            raise ValueError(f"Failed to get Git SHA from image: {e}") from e

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
