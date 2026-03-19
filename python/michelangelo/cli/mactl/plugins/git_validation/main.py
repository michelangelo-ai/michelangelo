"""Git validation for mactl resources.

This module provides git workspace detection and information gathering
to ensure resources are created/updated from a clean git state with
proper branch permissions.
"""

import os
import subprocess
from dataclasses import dataclass
from typing import Optional


@dataclass
class GitInfo:
    """Git information for a workspace.

    Attributes:
        repo: Git remote URL (e.g., "https://github.com/org/repo.git")
        branch_name: Current branch name (e.g., "main", "feature/new-model")
        commit_hash: Current commit SHA
        is_clean: Whether workspace is clean (no uncommitted changes, all pushed)
        is_on_main: Whether current branch is main/master
    """

    repo: str
    branch_name: str
    commit_hash: str
    is_clean: bool
    is_on_main: bool


class GitValidator:
    """Git validation for mactl resources.

    This class provides methods to detect git workspace information
    and validate git state before creating/updating resources.
    """

    def __init__(self, config: Optional[dict] = None):
        """Initialize GitValidator.

        Args:
            config: Optional configuration dict with keys:
                - main_branches: List of main branch names
                  (default: ['main', 'master'])
                - bypass_env: Environment variable to bypass checks
                  (default: 'MACTL_IGNORE_GIT_CLEAN_CHECK')
        """
        self.config = config or {}
        self.main_branches = self.config.get("main_branches", ["main", "master"])
        self.bypass_env = self.config.get("bypass_env", "MACTL_IGNORE_GIT_CLEAN_CHECK")

    def get_git_info(
        self,
        workspace_root: Optional[str] = None,
        external_branch: Optional[str] = None,
        external_commit: Optional[str] = None,
    ) -> GitInfo:
        """Get git information from workspace.

        Args:
            workspace_root: Optional workspace root path. If not provided, auto-detect.
            external_branch: Optional external branch name (for CI/CD). Skips detection.
            external_commit: Optional external commit hash (for CI/CD). Skips detection.

        Returns:
            GitInfo object with workspace information.

        Raises:
            ValueError: If not in a git repository or in detached HEAD state.
            subprocess.CalledProcessError: If git commands fail.
        """
        root = workspace_root or self._detect_workspace_root()

        if external_branch and external_commit:
            return GitInfo(
                repo=self._get_repo_url(root),
                branch_name=external_branch,
                commit_hash=external_commit,
                is_clean=True,
                is_on_main=self._is_on_main(external_branch),
            )

        branch = self._get_branch_name(root)
        return GitInfo(
            repo=self._get_repo_url(root),
            branch_name=branch,
            commit_hash=self._get_commit_hash(root),
            is_clean=self._is_clean(root),
            is_on_main=self._is_on_main(branch),
        )

    def _detect_workspace_root(self) -> str:
        """Detect git workspace root.

        Priority order:
        1. WORKSPACE_ROOT environment variable
        2. git rev-parse --show-toplevel
        3. BUILDKITE_BUILD_CHECKOUT_PATH (Buildkite standard variable)
        4. Current working directory (fallback)

        Returns:
            Absolute path to workspace root.

        Raises:
            ValueError: If not in a git repository and not in CI/CD mode.
        """
        workspace_root = os.environ.get("WORKSPACE_ROOT")
        if workspace_root:
            return workspace_root

        try:
            result = subprocess.run(
                ["git", "rev-parse", "--show-toplevel"],
                capture_output=True,
                text=True,
                check=True,
            )
            return result.stdout.strip()
        except subprocess.CalledProcessError as e:
            if self._is_buildkite():
                # Use BUILDKITE_BUILD_CHECKOUT_PATH (standard Buildkite variable)
                # to handle cases where the job may have changed directories
                return os.environ.get("BUILDKITE_BUILD_CHECKOUT_PATH", os.getcwd())
            raise ValueError(
                "Not in a git repository. Please run this command from "
                f"within a git repository.\nGit error: {e.stderr.strip()}"
            ) from e

    def _get_branch_name(self, root: str) -> str:
        """Get current branch name.

        Args:
            root: Workspace root path.

        Returns:
            Branch name (e.g., "main", "feature/new-model").

        Raises:
            ValueError: If in detached HEAD state.
            subprocess.CalledProcessError: If git command fails.
        """
        if self._is_buildkite():
            # Use BUILDKITE_BRANCH (standard Buildkite variable) if available
            branch = os.environ.get("BUILDKITE_BRANCH", "")
            if branch:
                return branch

        result = subprocess.run(
            ["git", "rev-parse", "--abbrev-ref", "HEAD"],
            capture_output=True,
            text=True,
            cwd=root,
            check=True,
        )
        branch = result.stdout.strip()

        if branch == "HEAD":
            raise ValueError(
                "Git ref is not a valid branch (detached HEAD state). "
                "Please checkout a branch before running this command."
            )

        return branch

    def _get_commit_hash(self, root: str) -> str:
        """Get current commit hash.

        Args:
            root: Workspace root path.

        Returns:
            Commit SHA (e.g., "abc123def456...").

        Raises:
            subprocess.CalledProcessError: If git command fails.
        """
        result = subprocess.run(
            ["git", "rev-parse", "HEAD"],
            capture_output=True,
            text=True,
            cwd=root,
            check=True,
        )
        return result.stdout.strip()

    def _get_repo_url(self, root: str) -> str:
        """Get git remote URL.

        Args:
            root: Workspace root path.

        Returns:
            Remote URL (e.g., "https://github.com/org/repo.git").

        Raises:
            ValueError: If no git remote 'origin' is configured.
            subprocess.CalledProcessError: If git command fails.
        """
        try:
            result = subprocess.run(
                ["git", "config", "--get", "remote.origin.url"],
                capture_output=True,
                text=True,
                cwd=root,
                check=True,
            )
            return result.stdout.strip()
        except subprocess.CalledProcessError as e:
            raise ValueError(
                "No git remote 'origin' configured. Please add a remote origin."
            ) from e

    def _is_buildkite(self) -> bool:
        """Check if running in Buildkite CI/CD environment.

        Returns:
            True if BUILDKITE environment variable is set to 'true'.
        """
        return os.environ.get("BUILDKITE", "").lower() == "true"

    def _is_clean(self, root: str) -> bool:
        """Check if git workspace is clean.

        A workspace is clean if:
        1. No uncommitted changes (git status --porcelain is empty)
        2. All commits are pushed (git push -n shows 'Everything up-to-date')

        Args:
            root: Workspace root path.

        Returns:
            True if workspace is clean, False otherwise.
        """
        if os.environ.get(self.bypass_env, "").lower() == "true":
            return True

        try:
            result = subprocess.run(
                ["git", "status", "--porcelain"],
                capture_output=True,
                text=True,
                cwd=root,
                check=True,
            )
            if result.stdout.strip():
                return False
        except subprocess.CalledProcessError:
            return False

        result = subprocess.run(
            ["git", "push", "-n"],
            capture_output=True,
            text=True,
            cwd=root,
            check=False,
        )
        output = result.stdout + result.stderr
        return "Everything up-to-date" in output

    def _is_on_main(self, branch_name: str) -> bool:
        """Check if branch is a main branch.

        Args:
            branch_name: Branch name to check.

        Returns:
            True if branch is main/master or running in Buildkite.
        """
        if self._is_buildkite():
            # Skip main branch check in CI
            return True
        return branch_name in self.main_branches
