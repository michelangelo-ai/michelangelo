"""Tests for git_validation module."""

import os
import subprocess
import tempfile
from unittest.mock import MagicMock, patch

import pytest

from michelangelo.cli.mactl.git_validation import GitInfo, GitValidator


class TestGitInfo:
    """Tests for GitInfo dataclass."""

    def test_git_info_creation(self):
        git_info = GitInfo(
            repo="https://github.com/org/repo.git",
            branch_name="main",
            commit_hash="abc123",
            is_clean=True,
            is_on_main=True,
        )
        assert git_info.repo == "https://github.com/org/repo.git"
        assert git_info.branch_name == "main"
        assert git_info.commit_hash == "abc123"
        assert git_info.is_clean is True
        assert git_info.is_on_main is True


class TestGitValidator:
    """Tests for GitValidator class."""

    def test_init_default_config(self):
        validator = GitValidator()
        assert validator.main_branches == ["main", "master"]
        assert validator.bypass_env == "MA_IGNORE_GIT_CLEAN_CHECK"

    def test_init_custom_config(self):
        config = {
            "main_branches": ["main", "production"],
            "bypass_env": "CUSTOM_BYPASS",
        }
        validator = GitValidator(config)
        assert validator.main_branches == ["main", "production"]
        assert validator.bypass_env == "CUSTOM_BYPASS"

    @patch("subprocess.run")
    def test_detect_workspace_root_success(self, mock_run):
        mock_run.return_value = MagicMock(
            stdout="/path/to/repo\n", stderr="", returncode=0
        )

        validator = GitValidator()
        root = validator._detect_workspace_root()

        assert root == "/path/to/repo"
        mock_run.assert_called_once_with(
            ["git", "rev-parse", "--show-toplevel"],
            capture_output=True,
            text=True,
            check=True,
        )

    @patch("subprocess.run")
    def test_detect_workspace_root_not_git_repo(self, mock_run):
        mock_run.side_effect = subprocess.CalledProcessError(
            128, "git", stderr="fatal: not a git repository"
        )

        validator = GitValidator()
        with pytest.raises(ValueError, match="Not in a git repository"):
            validator._detect_workspace_root()

    @patch("subprocess.run")
    def test_get_branch_name_success(self, mock_run):
        mock_run.return_value = MagicMock(
            stdout="feature/new-model\n", stderr="", returncode=0
        )

        validator = GitValidator()
        branch = validator._get_branch_name("/path/to/repo")

        assert branch == "feature/new-model"
        mock_run.assert_called_once_with(
            ["git", "rev-parse", "--abbrev-ref", "HEAD"],
            capture_output=True,
            text=True,
            cwd="/path/to/repo",
            check=True,
        )

    @patch("subprocess.run")
    def test_get_branch_name_detached_head(self, mock_run):
        mock_run.return_value = MagicMock(stdout="HEAD\n", stderr="", returncode=0)

        validator = GitValidator()
        with pytest.raises(ValueError, match="detached HEAD state"):
            validator._get_branch_name("/path/to/repo")

    @patch("subprocess.run")
    def test_get_commit_hash_success(self, mock_run):
        mock_run.return_value = MagicMock(
            stdout="abc123def456\n", stderr="", returncode=0
        )

        validator = GitValidator()
        commit = validator._get_commit_hash("/path/to/repo")

        assert commit == "abc123def456"
        mock_run.assert_called_once_with(
            ["git", "rev-parse", "HEAD"],
            capture_output=True,
            text=True,
            cwd="/path/to/repo",
            check=True,
        )

    @patch("subprocess.run")
    def test_get_repo_url_success(self, mock_run):
        mock_run.return_value = MagicMock(
            stdout="https://github.com/org/repo.git\n", stderr="", returncode=0
        )

        validator = GitValidator()
        repo_url = validator._get_repo_url("/path/to/repo")

        assert repo_url == "https://github.com/org/repo.git"
        mock_run.assert_called_once_with(
            ["git", "config", "--get", "remote.origin.url"],
            capture_output=True,
            text=True,
            cwd="/path/to/repo",
            check=True,
        )

    @patch("subprocess.run")
    def test_get_repo_url_no_remote(self, mock_run):
        mock_run.side_effect = subprocess.CalledProcessError(
            1, "git", stderr="error: No such remote 'origin'"
        )

        validator = GitValidator()
        with pytest.raises(subprocess.CalledProcessError):
            validator._get_repo_url("/path/to/repo")

    @patch.object(GitValidator, "_get_repo_url")
    @patch.object(GitValidator, "_get_commit_hash")
    @patch.object(GitValidator, "_get_branch_name")
    @patch.object(GitValidator, "_detect_workspace_root")
    def test_get_git_info_auto_detect(
        self, mock_detect_root, mock_get_branch, mock_get_commit, mock_get_repo
    ):
        mock_detect_root.return_value = "/path/to/repo"
        mock_get_branch.return_value = "main"
        mock_get_commit.return_value = "abc123"
        mock_get_repo.return_value = "https://github.com/org/repo.git"

        validator = GitValidator()
        git_info = validator.get_git_info()

        assert git_info.repo == "https://github.com/org/repo.git"
        assert git_info.branch_name == "main"
        assert git_info.commit_hash == "abc123"
        assert git_info.is_clean is False
        assert git_info.is_on_main is False

        mock_detect_root.assert_called_once()
        mock_get_branch.assert_called_once_with("/path/to/repo")
        mock_get_commit.assert_called_once_with("/path/to/repo")
        mock_get_repo.assert_called_once_with("/path/to/repo")

    @patch.object(GitValidator, "_get_repo_url")
    @patch.object(GitValidator, "_get_commit_hash")
    @patch.object(GitValidator, "_get_branch_name")
    def test_get_git_info_explicit_root(
        self, mock_get_branch, mock_get_commit, mock_get_repo
    ):
        mock_get_branch.return_value = "feature/test"
        mock_get_commit.return_value = "def456"
        mock_get_repo.return_value = "https://github.com/org/repo.git"

        validator = GitValidator()
        git_info = validator.get_git_info(workspace_root="/custom/path")

        assert git_info.repo == "https://github.com/org/repo.git"
        assert git_info.branch_name == "feature/test"
        assert git_info.commit_hash == "def456"

        mock_get_branch.assert_called_once_with("/custom/path")
        mock_get_commit.assert_called_once_with("/custom/path")
        mock_get_repo.assert_called_once_with("/custom/path")

    @patch.object(GitValidator, "_get_repo_url")
    @patch.object(GitValidator, "_get_commit_hash")
    @patch.object(GitValidator, "_get_branch_name")
    @patch.object(GitValidator, "_detect_workspace_root")
    def test_get_git_info_external_params(
        self, mock_detect_root, mock_get_branch, mock_get_commit, mock_get_repo
    ):
        mock_detect_root.return_value = "/path/to/repo"
        mock_get_repo.return_value = "https://github.com/org/repo.git"

        validator = GitValidator()
        git_info = validator.get_git_info(
            external_branch="main", external_commit="external123"
        )

        assert git_info.repo == "https://github.com/org/repo.git"
        assert git_info.branch_name == "main"
        assert git_info.commit_hash == "external123"
        assert git_info.is_clean is True
        assert git_info.is_on_main is True

        mock_detect_root.assert_called_once()
        mock_get_repo.assert_called_once()
        mock_get_branch.assert_not_called()
        mock_get_commit.assert_not_called()

    @patch.object(GitValidator, "_get_repo_url")
    @patch.object(GitValidator, "_get_commit_hash")
    @patch.object(GitValidator, "_get_branch_name")
    @patch.object(GitValidator, "_detect_workspace_root")
    def test_get_git_info_external_params_custom_branch(
        self, mock_detect_root, mock_get_branch, mock_get_commit, mock_get_repo
    ):
        mock_detect_root.return_value = "/path/to/repo"
        mock_get_repo.return_value = "https://github.com/org/repo.git"

        config = {"main_branches": ["main", "production"]}
        validator = GitValidator(config)
        git_info = validator.get_git_info(
            external_branch="production", external_commit="prod123"
        )

        assert git_info.branch_name == "production"
        assert git_info.is_on_main is True

        git_info = validator.get_git_info(
            external_branch="feature/test", external_commit="feat123"
        )

        assert git_info.branch_name == "feature/test"
        assert git_info.is_on_main is False

    @patch.object(GitValidator, "_get_repo_url")
    @patch.object(GitValidator, "_get_commit_hash")
    @patch.object(GitValidator, "_get_branch_name")
    def test_get_git_info_external_branch_only_ignored(
        self, mock_get_branch, mock_get_commit, mock_get_repo
    ):
        mock_get_branch.return_value = "main"
        mock_get_commit.return_value = "abc123"
        mock_get_repo.return_value = "https://github.com/org/repo.git"

        validator = GitValidator()
        git_info = validator.get_git_info(
            workspace_root="/path", external_branch="external-only"
        )

        assert git_info.branch_name == "main"
        assert git_info.commit_hash == "abc123"

        mock_get_branch.assert_called_once()
        mock_get_commit.assert_called_once()


class TestGitValidatorIntegration:
    """Integration tests using a real git repository."""

    @pytest.fixture
    def temp_git_repo(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            subprocess.run(["git", "init"], cwd=tmpdir, check=True, capture_output=True)
            subprocess.run(
                ["git", "config", "user.name", "Test User"],
                cwd=tmpdir,
                check=True,
                capture_output=True,
            )
            subprocess.run(
                ["git", "config", "user.email", "test@example.com"],
                cwd=tmpdir,
                check=True,
                capture_output=True,
            )
            subprocess.run(
                ["git", "remote", "add", "origin", "https://github.com/test/repo.git"],
                cwd=tmpdir,
                check=True,
                capture_output=True,
            )

            test_file = os.path.join(tmpdir, "test.txt")
            with open(test_file, "w") as f:
                f.write("test content")
            subprocess.run(
                ["git", "add", "test.txt"], cwd=tmpdir, check=True, capture_output=True
            )
            subprocess.run(
                ["git", "commit", "-m", "Initial commit"],
                cwd=tmpdir,
                check=True,
                capture_output=True,
            )

            yield tmpdir

    def test_integration_get_git_info(self, temp_git_repo):
        validator = GitValidator()
        git_info = validator.get_git_info(workspace_root=temp_git_repo)

        assert git_info.repo == "https://github.com/test/repo.git"
        assert git_info.branch_name in ["main", "master"]
        assert len(git_info.commit_hash) == 40
        assert git_info.is_clean is False
        assert git_info.is_on_main is False

    def test_integration_feature_branch(self, temp_git_repo):
        subprocess.run(
            ["git", "checkout", "-b", "feature/test"],
            cwd=temp_git_repo,
            check=True,
            capture_output=True,
        )

        validator = GitValidator()
        git_info = validator.get_git_info(workspace_root=temp_git_repo)

        assert git_info.branch_name == "feature/test"
        assert len(git_info.commit_hash) == 40
