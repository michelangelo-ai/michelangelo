"""Current git commit and branch, used to default ``spec.commit``."""

import dataclasses
import os
import subprocess
from collections.abc import Mapping, Sequence
from typing import Callable, Optional, Protocol

# CI variables checked in order when HEAD is detached (typical in CI) or git is
# unavailable: GitHub Actions, GitLab CI, then Buildkite.
DEFAULT_BRANCH_ENV_VARS = (
    "GITHUB_HEAD_REF",  # pull request source branch; empty on push
    "GITHUB_REF_NAME",
    "CI_COMMIT_REF_NAME",
    "CI_COMMIT_BRANCH",
    "BUILDKITE_BRANCH",
)
DEFAULT_COMMIT_ENV_VARS = ("GITHUB_SHA", "CI_COMMIT_SHA", "BUILDKITE_COMMIT")

GitRunner = Callable[[Sequence[str], str], Optional[str]]


@dataclasses.dataclass(frozen=True)
class GitInfo:
    """Git commit and branch of the pipeline source.

    Attributes:
        git_ref: Commit hash, or ``None`` when unknown.
        branch: Branch name, or ``None`` when unknown.
    """

    git_ref: Optional[str] = None
    branch: Optional[str] = None


class GitInfoProvider(Protocol):
    """Resolves git information for a directory."""

    def get(self, root: str) -> GitInfo:
        """Return git information for the repository containing ``root``."""


def run_git(args: Sequence[str], cwd: str) -> Optional[str]:
    """Run ``git`` and return its stripped stdout, or ``None`` on any failure."""
    try:
        result = subprocess.run(
            ["git", *args],
            cwd=cwd,
            capture_output=True,
            text=True,
            check=True,
            timeout=10,
        )
    except (OSError, subprocess.SubprocessError):
        return None
    return result.stdout.strip() or None


class SubprocessGitInfoProvider:
    """Reads git information by shelling out to ``git``, with CI fallbacks."""

    def __init__(
        self,
        runner: GitRunner = run_git,
        environ: Optional[Mapping[str, str]] = None,
        branch_env_vars: Sequence[str] = DEFAULT_BRANCH_ENV_VARS,
        commit_env_vars: Sequence[str] = DEFAULT_COMMIT_ENV_VARS,
    ):
        """Initialize the provider.

        Args:
            runner: Runs a git command; injectable for tests.
            environ: Environment used for CI fallbacks. Defaults to os.environ.
            branch_env_vars: CI variables holding the branch, checked in order.
            commit_env_vars: CI variables holding the commit, checked in order.
        """
        self._runner = runner
        self._environ = os.environ if environ is None else environ
        self._branch_env_vars = tuple(branch_env_vars)
        self._commit_env_vars = tuple(commit_env_vars)

    def get(self, root: str) -> GitInfo:
        """Return git information for the repository containing ``root``."""
        git_ref = self._runner(["rev-parse", "HEAD"], root)
        branch = self._runner(["rev-parse", "--abbrev-ref", "HEAD"], root)
        if branch == "HEAD":  # detached HEAD
            branch = None
        return GitInfo(
            git_ref=git_ref or self._first_env(self._commit_env_vars),
            branch=branch or self._first_env(self._branch_env_vars),
        )

    def _first_env(self, names: Sequence[str]) -> Optional[str]:
        for name in names:
            value = self._environ.get(name, "").strip()
            if value:
                return value
        return None
