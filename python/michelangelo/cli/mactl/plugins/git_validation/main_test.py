"""Tests for git_validation module."""

import os
import subprocess
import tempfile
from unittest.mock import MagicMock, patch

import pytest

from michelangelo.cli.mactl.plugins.git_validation.main import GitInfo, GitValidator


class TestGitInfo:
    """Tests for GitInfo dataclass."""

    def test_git_info_creation(self):
        """Test GitInfo dataclass creation."""
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
        """Test GitValidator initialization with default config."""
        validator = GitValidator()
        assert validator.main_branches == ["main", "master"]
        assert validator.bypass_env == "MACTL_IGNORE_GIT_CLEAN_CHECK"

    def test_init_custom_config(self):
        """Test GitValidator initialization with custom config."""
        config = {
            "main_branches": ["main", "production"],
            "bypass_env": "CUSTOM_BYPASS",
        }
        validator = GitValidator(config)
        assert validator.main_branches == ["main", "production"]
        assert validator.bypass_env == "CUSTOM_BYPASS"

    @patch("subprocess.run")
    def test_detect_workspace_root_success(self, mock_run):
        """Test successful workspace root detection."""
        mock_run.return_value = MagicMock(
            stdout="/path/to/repo\n", stderr="", returncode=0
        )

        with patch.dict(os.environ, {}, clear=False):
            os.environ.pop("WORKSPACE_ROOT", None)
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
        """Test workspace root detection fails when not in git repo."""
        mock_run.side_effect = subprocess.CalledProcessError(
            128, "git", stderr="fatal: not a git repository"
        )

        with patch.dict(os.environ, {}, clear=False):
            os.environ.pop("WORKSPACE_ROOT", None)
            os.environ.pop("BUILDKITE", None)
            validator = GitValidator()
            with pytest.raises(ValueError, match="Not in a git repository"):
                validator._detect_workspace_root()

    @patch("subprocess.run")
    def test_get_branch_name_success(self, mock_run):
        """Test successful branch name retrieval."""
        mock_run.return_value = MagicMock(
            stdout="feature/new-model\n", stderr="", returncode=0
        )

        with patch.dict(os.environ, {}, clear=False):
            os.environ.pop("BUILDKITE", None)
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
        """Test branch name detection fails in detached HEAD state."""
        mock_run.return_value = MagicMock(stdout="HEAD\n", stderr="", returncode=0)

        with patch.dict(os.environ, {}, clear=False):
            os.environ.pop("BUILDKITE", None)
            validator = GitValidator()
            with pytest.raises(ValueError, match="detached HEAD state"):
                validator._get_branch_name("/path/to/repo")

    @patch("subprocess.run")
    def test_get_commit_hash_success(self, mock_run):
        """Test successful commit hash retrieval."""
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
        """Test successful repo URL retrieval."""
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
        """Test repo URL retrieval fails when no remote configured."""
        mock_run.side_effect = subprocess.CalledProcessError(
            1, "git", stderr="error: No such remote 'origin'"
        )

        validator = GitValidator()
        with pytest.raises(ValueError, match="No git remote 'origin' configured"):
            validator._get_repo_url("/path/to/repo")

    def test_is_buildkite_true(self):
        """Test Buildkite detection returns True when BUILDKITE=true."""
        with patch.dict(os.environ, {"BUILDKITE": "true"}):
            validator = GitValidator()
            assert validator._is_buildkite() is True

    def test_is_buildkite_false(self):
        """Test Buildkite detection returns False when BUILDKITE not set."""
        with patch.dict(os.environ, {}, clear=False):
            os.environ.pop("BUILDKITE", None)
            validator = GitValidator()
            assert validator._is_buildkite() is False

    def test_is_buildkite_case_insensitive(self):
        """Test Buildkite detection is case insensitive."""
        with patch.dict(os.environ, {"BUILDKITE": "True"}):
            validator = GitValidator()
            assert validator._is_buildkite() is True

    @patch("subprocess.run")
    def test_is_clean_no_changes(self, mock_run):
        """Test workspace is clean when no uncommitted or unpushed changes."""
        mock_run.side_effect = [
            MagicMock(stdout="", stderr="", returncode=0),
            MagicMock(stdout="Everything up-to-date", stderr="", returncode=0),
        ]

        with patch.dict(os.environ, {}, clear=False):
            os.environ.pop("MACTL_IGNORE_GIT_CLEAN_CHECK", None)
            validator = GitValidator()
            assert validator._is_clean("/path/to/repo") is True

        assert mock_run.call_count == 2
        mock_run.assert_any_call(
            ["git", "status", "--porcelain"],
            capture_output=True,
            text=True,
            cwd="/path/to/repo",
            check=True,
        )
        mock_run.assert_any_call(
            ["git", "push", "-n"],
            capture_output=True,
            text=True,
            cwd="/path/to/repo",
            check=False,
        )

    @patch("subprocess.run")
    def test_is_clean_uncommitted_changes(self, mock_run):
        """Test workspace is not clean when uncommitted changes exist."""
        mock_run.return_value = MagicMock(
            stdout=" M file.txt\n", stderr="", returncode=0
        )

        with patch.dict(os.environ, {}, clear=False):
            os.environ.pop("MACTL_IGNORE_GIT_CLEAN_CHECK", None)
            validator = GitValidator()
            assert validator._is_clean("/path/to/repo") is False

        mock_run.assert_called_once_with(
            ["git", "status", "--porcelain"],
            capture_output=True,
            text=True,
            cwd="/path/to/repo",
            check=True,
        )

    @patch("subprocess.run")
    def test_is_clean_unpushed_commits(self, mock_run):
        """Test workspace is not clean when unpushed commits exist."""
        mock_run.side_effect = [
            MagicMock(stdout="", stderr="", returncode=0),
            MagicMock(
                stdout="",
                stderr="To github.com:org/repo.git\n   abc123..def456  main -> main",
                returncode=0,
            ),
        ]

        with patch.dict(os.environ, {}, clear=False):
            os.environ.pop("MACTL_IGNORE_GIT_CLEAN_CHECK", None)
            validator = GitValidator()
            assert validator._is_clean("/path/to/repo") is False

    @patch("subprocess.run")
    def test_is_clean_bypass_env(self, mock_run):
        """Test clean check bypassed when MACTL_IGNORE_GIT_CLEAN_CHECK=true."""
        with patch.dict(os.environ, {"MACTL_IGNORE_GIT_CLEAN_CHECK": "true"}):
            validator = GitValidator()
            assert validator._is_clean("/path/to/repo") is True
            mock_run.assert_not_called()

    def test_is_on_main_in_buildkite(self):
        """Test is_on_main returns True in Buildkite regardless of branch."""
        with patch.dict(os.environ, {"BUILDKITE": "true"}):
            validator = GitValidator()
            assert validator._is_on_main("feature/test") is True

    def test_is_on_main_main_branch(self):
        """Test is_on_main returns True for main and master branches."""
        validator = GitValidator()
        assert validator._is_on_main("main") is True
        assert validator._is_on_main("master") is True

    def test_is_on_main_feature_branch(self):
        """Test is_on_main returns False for feature branches."""
        with patch.dict(os.environ, {}, clear=False):
            os.environ.pop("BUILDKITE", None)
            validator = GitValidator()
            assert validator._is_on_main("feature/test") is False
            assert validator._is_on_main("develop") is False

    def test_is_on_main_custom_branches(self):
        """Test is_on_main respects custom main branch configuration."""
        with patch.dict(os.environ, {}, clear=False):
            os.environ.pop("BUILDKITE", None)
            validator = GitValidator(
                {"main_branches": ["main", "production", "release"]}
            )
            assert validator._is_on_main("production") is True
            assert validator._is_on_main("release") is True
            assert validator._is_on_main("master") is False

    @patch.object(GitValidator, "_get_repo_url")
    @patch.object(GitValidator, "_get_commit_hash")
    @patch.object(GitValidator, "_get_branch_name")
    @patch.object(GitValidator, "_detect_workspace_root")
    def test_get_git_info_external_params(
        self, mock_detect_root, mock_get_branch, mock_get_commit, mock_get_repo
    ):
        """Test get_git_info with external branch and commit params."""
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
    def test_get_git_info_external_main_branch_is_on_main(
        self, mock_detect_root, mock_get_branch, mock_get_commit, mock_get_repo
    ):
        """Test get_git_info with external main branch sets is_on_main=True."""
        mock_detect_root.return_value = "/path/to/repo"
        mock_get_repo.return_value = "https://github.com/org/repo.git"

        with patch.dict(os.environ, {}, clear=False):
            os.environ.pop("BUILDKITE", None)
            config = {"main_branches": ["main", "production"]}
            validator = GitValidator(config)
            git_info = validator.get_git_info(
                external_branch="production", external_commit="prod123"
            )

        assert git_info.branch_name == "production"
        assert git_info.is_on_main is True

    @patch.object(GitValidator, "_get_repo_url")
    @patch.object(GitValidator, "_get_commit_hash")
    @patch.object(GitValidator, "_get_branch_name")
    @patch.object(GitValidator, "_detect_workspace_root")
    def test_get_git_info_external_feature_branch_not_on_main(
        self, mock_detect_root, mock_get_branch, mock_get_commit, mock_get_repo
    ):
        """Test get_git_info with external feature branch sets is_on_main=False."""
        mock_detect_root.return_value = "/path/to/repo"
        mock_get_repo.return_value = "https://github.com/org/repo.git"

        with patch.dict(os.environ, {}, clear=False):
            os.environ.pop("BUILDKITE", None)
            config = {"main_branches": ["main", "production"]}
            validator = GitValidator(config)
            git_info = validator.get_git_info(
                external_branch="feature/test", external_commit="feat123"
            )

        assert git_info.branch_name == "feature/test"
        assert git_info.is_on_main is False

    def test_detect_workspace_root_uses_env_var(self):
        """Test workspace root detection uses WORKSPACE_ROOT env var."""
        with (
            patch.dict(os.environ, {"WORKSPACE_ROOT": "/env/path"}),
            patch("subprocess.run") as mock_run,
        ):
            validator = GitValidator()
            root = validator._detect_workspace_root()
            assert root == "/env/path"
            mock_run.assert_not_called()

    @patch("subprocess.run")
    def test_is_clean_bypass_env_case_insensitive(self, mock_run):
        """Test bypass env var is case insensitive."""
        with patch.dict(os.environ, {"MACTL_IGNORE_GIT_CLEAN_CHECK": "TRUE"}):
            validator = GitValidator()
            assert validator._is_clean("/path/to/repo") is True
            mock_run.assert_not_called()

    @patch("subprocess.run")
    def test_get_branch_name_buildkite_no_branch_env(self, mock_run):
        """Test branch name git fallback in Buildkite without BUILDKITE_BRANCH."""
        mock_run.return_value = MagicMock(stdout="main\n", stderr="", returncode=0)
        env = {"BUILDKITE": "true"}
        with patch.dict(os.environ, env, clear=False):
            os.environ.pop("BUILDKITE_BRANCH", None)
            validator = GitValidator()
            branch = validator._get_branch_name("/path")
        assert branch == "main"
        mock_run.assert_called_once()


class TestGitValidatorIntegration:
    """Integration tests using a real git repository."""

    @pytest.fixture
    def temp_git_repo(self):
        """Create temporary git repository for integration tests."""
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
        """Test get_git_info integration with real git repository."""
        validator = GitValidator()
        git_info = validator.get_git_info(workspace_root=temp_git_repo)

        assert git_info.repo == "https://github.com/test/repo.git"
        assert git_info.branch_name in ["main", "master"]
        assert len(git_info.commit_hash) == 40
        assert git_info.is_clean is False
        assert git_info.is_on_main is True

    def test_integration_feature_branch(self, temp_git_repo):
        """Test get_git_info on feature branch."""
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
        assert git_info.is_on_main is False

    def test_integration_buildkite_detection(self, temp_git_repo):
        """Test Buildkite environment detection in integration test."""
        env = {"BUILDKITE": "true", "BUILDKITE_BRANCH": "ci-branch"}
        with patch.dict(os.environ, env):
            validator = GitValidator()
            git_info = validator.get_git_info(workspace_root=temp_git_repo)

            assert git_info.branch_name == "ci-branch"
            assert git_info.is_on_main is True

    def test_integration_not_clean_when_commits_unpushed(self, temp_git_repo):
        """Test workspace is not clean when commits are unpushed."""
        validator = GitValidator()
        git_info = validator.get_git_info(workspace_root=temp_git_repo)

        assert git_info.is_clean is False

    def test_integration_bypass_clean_check(self, temp_git_repo):
        """Test bypass clean check in integration test."""
        with patch.dict(os.environ, {"MACTL_IGNORE_GIT_CLEAN_CHECK": "true"}):
            validator = GitValidator()
            git_info = validator.get_git_info(workspace_root=temp_git_repo)

            assert git_info.is_clean is True

    def test_integration_workspace_root_env_var(self, temp_git_repo):
        """Test WORKSPACE_ROOT env var in integration test."""
        with patch.dict(os.environ, {"WORKSPACE_ROOT": temp_git_repo}):
            validator = GitValidator()
            git_info = validator.get_git_info()

            assert git_info.repo == "https://github.com/test/repo.git"
            assert git_info.branch_name in ["main", "master"]

    def test_integration_external_ci_params(self, temp_git_repo):
        """Test external CI parameters in integration test."""
        with patch.dict(os.environ, {}, clear=False):
            os.environ.pop("BUILDKITE", None)
            validator = GitValidator()
            git_info = validator.get_git_info(
                workspace_root=temp_git_repo,
                external_branch="ci-branch",
                external_commit="abc123def456789",
            )

        assert git_info.branch_name == "ci-branch"
        assert git_info.commit_hash == "abc123def456789"
        assert git_info.is_clean is True
        assert git_info.is_on_main is False

    def test_integration_custom_main_branches(self, temp_git_repo):
        """Test custom main branch configuration in integration test."""
        subprocess.run(
            ["git", "checkout", "-b", "production"],
            cwd=temp_git_repo,
            check=True,
            capture_output=True,
        )

        test_file = os.path.join(temp_git_repo, "test2.txt")
        with open(test_file, "w") as f:
            f.write("test content 2")
        subprocess.run(
            ["git", "add", "test2.txt"],
            cwd=temp_git_repo,
            check=True,
            capture_output=True,
        )
        subprocess.run(
            ["git", "commit", "-m", "Second commit"],
            cwd=temp_git_repo,
            check=True,
            capture_output=True,
        )

        validator = GitValidator({"main_branches": ["main", "production"]})
        git_info = validator.get_git_info(workspace_root=temp_git_repo)

        assert git_info.branch_name == "production"
        assert git_info.is_on_main is True
