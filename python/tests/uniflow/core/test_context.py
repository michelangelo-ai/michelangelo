"""Tests for workflow execution context."""

import argparse
import os
import unittest
from unittest.mock import Mock, patch

from michelangelo.uniflow.core.context import (
    Context,
    _local_run,
    _remote_run_argument_parser,
    create_context,
)


class TestContext(unittest.TestCase):
    """Test cases for Context class."""

    def test_is_local_run_true(self):
        """Test is_local_run() returns True for local-run target."""
        ctx = Context(_args=[], _target="local-run")
        self.assertTrue(ctx.is_local_run())

    def test_is_local_run_false(self):
        """Test is_local_run() returns False for remote-run target."""
        ctx = Context(_args=[], _target="remote-run")
        self.assertFalse(ctx.is_local_run())

    @patch("michelangelo.uniflow.core.context._local_run")
    def test_run_local_mode(self, mock_local_run):
        """Test run() executes in local mode."""
        ctx = Context(_args=[], _target="local-run", environ={"TEST_VAR": "value"})
        mock_fn = Mock()

        ctx.run(mock_fn, "arg1", kwarg1="value1")

        mock_local_run.assert_called_once_with(mock_fn, "arg1", kwarg1="value1")

    @patch("michelangelo.uniflow.core.context._local_run")
    def test_run_local_mode_with_environ_args(self, mock_local_run):
        """Test run() in local mode with environment arguments."""
        ctx = Context(_args=["--environ", "KEY=VALUE"], _target="local-run")
        mock_fn = Mock()

        with patch.dict(os.environ, {}, clear=False):
            ctx.run(mock_fn)

        mock_local_run.assert_called_once()


class TestCreateContext(unittest.TestCase):
    """Test cases for create_context() function."""

    @patch("sys.argv", ["script.py"])
    def test_create_context_default_local_run(self):
        """Test create_context() defaults to local-run when no args provided."""
        ctx = create_context()
        self.assertEqual(ctx._target, "local-run")
        self.assertTrue(ctx.is_local_run())

    @patch("sys.argv", ["script.py", "local-run"])
    def test_create_context_explicit_local_run(self):
        """Test create_context() with explicit local-run."""
        ctx = create_context()
        self.assertEqual(ctx._target, "local-run")
        self.assertTrue(ctx.is_local_run())

    @patch("sys.argv", ["script.py", "remote-run", "--storage-url", "s3://bucket"])
    def test_create_context_remote_run(self):
        """Test create_context() with remote-run target."""
        ctx = create_context()
        self.assertEqual(ctx._target, "remote-run")
        self.assertFalse(ctx.is_local_run())

    @patch("sys.argv", ["script.py", "--some-flag"])
    def test_create_context_with_flag_defaults_to_local_run(self):
        """Test create_context() defaults to local-run when args start with dash."""
        ctx = create_context()
        self.assertEqual(ctx._target, "local-run")
        self.assertIn("--some-flag", ctx._args)

    @patch("sys.argv", ["script.py", "invalid-target"])
    def test_create_context_invalid_target_raises_assertion_error(self):
        """Test create_context() raises AssertionError for invalid target."""
        with self.assertRaises(AssertionError) as cm:
            create_context()
        self.assertIn("Unsupported target: invalid-target", str(cm.exception))


class TestLocalRun(unittest.TestCase):
    """Test cases for _local_run() function."""

    @patch("michelangelo.uniflow.core.context.build")
    def test_local_run_executes_function(self, mock_build):
        """Test _local_run() validates and executes the function."""
        mock_fn = Mock(return_value="result")

        # Save original values
        orig_local_run = os.environ.get("UF_LOCAL_RUN")
        orig_storage_url = os.environ.get("UF_STORAGE_URL")

        try:
            _local_run(mock_fn, "arg1", kwarg1="value1")

            mock_build.assert_called_once_with(mock_fn)
            mock_fn.assert_called_once_with("arg1", kwarg1="value1")
            self.assertEqual(os.environ.get("UF_LOCAL_RUN"), "1")
            self.assertTrue(os.environ.get("UF_STORAGE_URL"))
        finally:
            # Restore original values
            if orig_local_run is None:
                os.environ.pop("UF_LOCAL_RUN", None)
            else:
                os.environ["UF_LOCAL_RUN"] = orig_local_run
            if orig_storage_url is None:
                os.environ.pop("UF_STORAGE_URL", None)
            else:
                os.environ["UF_STORAGE_URL"] = orig_storage_url

    @patch("michelangelo.uniflow.core.context.build")
    def test_local_run_sets_environment_variables(self, mock_build):
        """Test _local_run() sets required environment variables."""
        mock_fn = Mock()

        # Save original values
        orig_local_run = os.environ.get("UF_LOCAL_RUN")
        orig_storage_url = os.environ.get("UF_STORAGE_URL")

        try:
            _local_run(mock_fn)

            self.assertEqual(os.environ.get("UF_LOCAL_RUN"), "1")
            storage_url = os.environ.get("UF_STORAGE_URL")
            self.assertTrue(storage_url)
            self.assertTrue(storage_url.endswith("uf_storage"))
        finally:
            # Restore original values
            if orig_local_run is None:
                os.environ.pop("UF_LOCAL_RUN", None)
            else:
                os.environ["UF_LOCAL_RUN"] = orig_local_run
            if orig_storage_url is None:
                os.environ.pop("UF_STORAGE_URL", None)
            else:
                os.environ["UF_STORAGE_URL"] = orig_storage_url


class TestRemoteRunArgumentParser(unittest.TestCase):
    """Test cases for _remote_run_argument_parser() function."""

    def test_argument_parser_without_environ(self):
        """Test _remote_run_argument_parser() without environ option."""
        parser = _remote_run_argument_parser(environ=False)

        self.assertIsInstance(parser, argparse.ArgumentParser)

        # Verify required arguments are present
        args = parser.parse_args(
            ["--storage-url", "s3://bucket", "--image", "my-image:latest"]
        )

        self.assertEqual(args.storage_url, "s3://bucket")
        self.assertEqual(args.image, "my-image:latest")
        self.assertEqual(args.workflow, "cadence")  # Default value
        self.assertFalse(args.yes)
        self.assertFalse(args.file_sync)

    def test_argument_parser_with_environ(self):
        """Test _remote_run_argument_parser() with environ option."""
        parser = _remote_run_argument_parser(environ=True)

        args = parser.parse_args(
            [
                "--storage-url",
                "s3://bucket",
                "--image",
                "my-image:latest",
                "--environ",
                "KEY=VALUE",
            ]
        )

        self.assertEqual(args.storage_url, "s3://bucket")
        self.assertEqual(args.image, "my-image:latest")
        self.assertEqual(args.environ, {"KEY": "VALUE"})

    def test_argument_parser_with_optional_flags(self):
        """Test _remote_run_argument_parser() with optional flags."""
        parser = _remote_run_argument_parser(environ=False)

        args = parser.parse_args(
            [
                "--storage-url",
                "s3://bucket",
                "--image",
                "my-image:latest",
                "--workflow",
                "temporal",
                "--yes",
                "--file-sync",
                "--cron",
                "0 0 * * *",
            ]
        )

        self.assertEqual(args.workflow, "temporal")
        self.assertTrue(args.yes)
        self.assertTrue(args.file_sync)
        self.assertEqual(args.cron, "0 0 * * *")


if __name__ == "__main__":
    unittest.main()
