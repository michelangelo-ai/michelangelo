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

log = logging.getLogger(__name__)


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
        date_str = datetime.now().strftime("%Y-%m-%d-%H-%M-%S")
        random_suffix = uuid.uuid4().hex[:8]
        prefix = self._pipeline if self._pipeline else "uniflow"
        return f"{prefix}-{date_str}-{random_suffix}.tar.gz"

    def get_file_name(self) -> str:
        if self._file_name is None:
            self._file_name = self.get_random_file_name()
        return self._file_name

    def get_remote_file_path(self) -> str:
        base_path = os.environ.get("UF_BASE_PROJECTS_PATH")
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
        commit_sha = self.get_git_sha()
        result = subprocess.run(
            ["git", "diff", "--name-only", commit_sha],
            cwd=os.getcwd(),
            capture_output=True,
            text=True,
            check=True,
        )
        changed_files = result.stdout.strip().splitlines()
        if not changed_files:
            log.info("No changed files found.")
            return None

        log.info(f"Changed files: {changed_files}")
        bb = io.BytesIO()
        with tarfile.open(fileobj=bb, mode="w:gz", dereference=True) as tar:
            for file_path in changed_files:
                path = Path(file_path)
                if path.exists():
                    tar.add(path, arcname=path)
        return bb.getvalue()

    def create_and_upload_tarball(self) -> str:
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
