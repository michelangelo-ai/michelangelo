"""Tests for git information resolution."""

import subprocess
import tempfile
import unittest
from unittest import mock

from michelangelo.uniflow.registration.git_info import (
    DEFAULT_BRANCH_ENV_VARS,
    GitInfo,
    SubprocessGitInfoProvider,
    run_git,
)


def _runner(head=None, abbrev=None):
    outputs = {
        ("rev-parse", "HEAD"): head,
        ("rev-parse", "--abbrev-ref", "HEAD"): abbrev,
    }

    def run(args, cwd):
        return outputs[tuple(args)]

    return run


class SubprocessGitInfoProviderTest(unittest.TestCase):
    """Tests for SubprocessGitInfoProvider."""

    def test_reads_commit_and_branch(self):
        """A normal checkout yields its commit and branch."""
        provider = SubprocessGitInfoProvider(_runner("abc123", "main"), environ={})
        self.assertEqual(GitInfo("abc123", "main"), provider.get("/repo"))

    def test_detached_head_falls_back_to_ci_variables(self):
        """A detached HEAD takes the branch from the first CI variable set."""
        for var in DEFAULT_BRANCH_ENV_VARS:
            with self.subTest(var=var):
                provider = SubprocessGitInfoProvider(
                    _runner("abc123", "HEAD"), environ={var: "release"}
                )
                self.assertEqual(GitInfo("abc123", "release"), provider.get("/repo"))

    def test_ci_variable_precedence(self):
        """Earlier CI variables win over later ones."""
        provider = SubprocessGitInfoProvider(
            _runner("abc123", "HEAD"),
            environ={"BUILDKITE_BRANCH": "b", "CI_COMMIT_REF_NAME": "a"},
        )
        self.assertEqual("a", provider.get("/repo").branch)

    def test_github_actions(self):
        """GitHub Actions: the PR source branch wins over GITHUB_REF_NAME."""
        push = {"GITHUB_SHA": "sha1", "GITHUB_HEAD_REF": "", "GITHUB_REF_NAME": "main"}
        provider = SubprocessGitInfoProvider(_runner(), environ=push)
        self.assertEqual(GitInfo("sha1", "main"), provider.get("/repo"))
        pull_request = {"GITHUB_HEAD_REF": "feature/x", "GITHUB_REF_NAME": "12/merge"}
        provider = SubprocessGitInfoProvider(_runner(), environ=pull_request)
        self.assertEqual("feature/x", provider.get("/repo").branch)

    def test_custom_ci_variables(self):
        """The CI variables checked are configurable."""
        provider = SubprocessGitInfoProvider(
            _runner(),
            environ={"GIT_COMMIT": "c1", "GIT_BRANCH": "dev", "GITHUB_SHA": "x"},
            branch_env_vars=("GIT_BRANCH",),
            commit_env_vars=("GIT_COMMIT",),
        )
        self.assertEqual(GitInfo("c1", "dev"), provider.get("/repo"))

    def test_detached_head_without_ci_has_no_branch(self):
        """A detached HEAD outside CI yields no branch."""
        provider = SubprocessGitInfoProvider(_runner("abc123", "HEAD"), environ={})
        self.assertEqual(GitInfo("abc123", None), provider.get("/repo"))

    def test_git_unavailable(self):
        """Without git, values come from CI variables or are None."""
        provider = SubprocessGitInfoProvider(_runner(), environ={})
        self.assertEqual(GitInfo(None, None), provider.get("/repo"))
        provider = SubprocessGitInfoProvider(
            _runner(), environ={"CI_COMMIT_SHA": "def456", "CI_COMMIT_BRANCH": "main"}
        )
        self.assertEqual(GitInfo("def456", "main"), provider.get("/repo"))


class RunGitTest(unittest.TestCase):
    """Tests for run_git."""

    def test_returns_none_outside_a_repository(self):
        """A non-repository directory yields None rather than raising."""
        with tempfile.TemporaryDirectory() as d:
            self.assertIsNone(run_git(["rev-parse", "HEAD"], d))

    def test_returns_none_when_git_is_missing(self):
        """A missing git binary yields None rather than raising."""
        with mock.patch.object(subprocess, "run", side_effect=FileNotFoundError):
            self.assertIsNone(run_git(["rev-parse", "HEAD"], "."))

    def test_returns_stripped_stdout(self):
        """Successful output is stripped."""
        completed = subprocess.CompletedProcess([], 0, stdout="abc\n", stderr="")
        with mock.patch.object(subprocess, "run", return_value=completed) as run:
            self.assertEqual("abc", run_git(["rev-parse", "HEAD"], "/repo"))
        self.assertEqual(["git", "rev-parse", "HEAD"], run.call_args.args[0])
        self.assertEqual("/repo", run.call_args.kwargs["cwd"])
