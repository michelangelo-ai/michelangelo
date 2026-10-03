"""Unit tests for trigger_run rerun plugin.

Tests the add_function_signature and generate_rerun functions.
"""

from inspect import Parameter, Signature
from unittest import TestCase
from unittest.mock import Mock, patch

from grpc import RpcError

from michelangelo.cli.mactl.plugins.entity.trigger_run.rerun import (
    add_function_signature,
    generate_rerun,
)

_PATCH_PREFIX = "michelangelo.cli.mactl.plugins.entity.trigger_run.rerun"

_RERUN_SIGNATURE = Signature(
    parameters=[
        Parameter("self", Parameter.POSITIONAL_OR_KEYWORD),
        Parameter("namespace", Parameter.POSITIONAL_OR_KEYWORD),
        Parameter("name", Parameter.POSITIONAL_OR_KEYWORD),
        Parameter("pipeline", Parameter.POSITIONAL_OR_KEYWORD),
        Parameter("pipeline_run", Parameter.POSITIONAL_OR_KEYWORD, default=None),
        Parameter("resume_from", Parameter.POSITIONAL_OR_KEYWORD, default=None),
        Parameter("resume_up_to", Parameter.POSITIONAL_OR_KEYWORD, default=None),
        Parameter("revision", Parameter.POSITIONAL_OR_KEYWORD, default=None),
    ]
)


class _MockRpcError(RpcError):
    """Concrete RpcError subclass for testing."""

    def __init__(self, details_msg):
        self._details = details_msg
        super().__init__(details_msg)

    def details(self):
        return self._details


def _make_crd_mock():
    """Build a CRD mock that returns a real Signature from _read_signatures."""
    mock_crd = Mock()
    mock_crd.full_name = "michelangelo.api.v2.TriggerRunService"
    mock_crd._read_signatures.return_value = _RERUN_SIGNATURE
    mock_crd.configure_parser = Mock()
    return mock_crd


class AddFunctionSignatureTest(TestCase):
    """Tests for add_function_signature."""

    def test_adds_rerun_entry(self):
        """Test that a 'rerun' entry is added to func_signature."""
        mock_crd = Mock()
        mock_crd.func_signature = {}

        add_function_signature(mock_crd)

        self.assertIn("rerun", mock_crd.func_signature)

    def test_has_eight_args(self):
        """Test that exactly eight args are defined for rerun."""
        mock_crd = Mock()
        mock_crd.func_signature = {}

        add_function_signature(mock_crd)

        self.assertEqual(len(mock_crd.func_signature["rerun"]["args"]), 8)

    def test_pipeline_run_arg_is_repeatable_and_required(self):
        """Test --pipeline-run is a required, repeatable arg."""
        mock_crd = Mock()
        mock_crd.func_signature = {}

        add_function_signature(mock_crd)

        arg = mock_crd.func_signature["rerun"]["args"][3]
        self.assertEqual(arg["args"], ["--pipeline-run"])
        self.assertEqual(arg["kwargs"]["action"], "append")
        self.assertTrue(arg["kwargs"]["required"])


class GenerateRerunSetupTest(TestCase):
    """Tests for generate_rerun setup phase (before the bound function runs)."""

    def test_calls_extract_method_info(self):
        """Test that _extract_method_info is called with Create."""
        mock_crd = _make_crd_mock()
        mock_crd._extract_method_info.return_value = ("Create", Mock(), Mock())
        mock_channel = Mock()

        generate_rerun(mock_crd, mock_channel, Mock())

        mock_crd._extract_method_info.assert_called_once_with(
            mock_channel, mock_crd.full_name, "Create"
        )

    def test_binds_rerun_method(self):
        """Test that a callable rerun method is bound to the CRD."""
        mock_crd = _make_crd_mock()
        mock_crd._extract_method_info.return_value = ("Create", Mock(), Mock())
        mock_channel = Mock()

        generate_rerun(mock_crd, mock_channel, Mock())

        self.assertTrue(hasattr(mock_crd, "rerun"))
        self.assertTrue(callable(mock_crd.rerun))


class GenerateRerunFunctionTest(TestCase):
    """Tests for the bound rerun function's runtime behavior."""

    def _setup_and_bind(self, response=None, error=None):
        mock_crd = _make_crd_mock()
        mock_channel = Mock()
        input_class = Mock()
        output_class = Mock()
        mock_crd._extract_method_info.return_value = (
            "CreateTriggerRun",
            input_class,
            output_class,
        )
        mock_stub = Mock(side_effect=error, return_value=response)
        mock_channel.unary_unary.return_value = mock_stub

        generate_rerun(mock_crd, mock_channel, Mock())
        return mock_crd

    @patch(f"{_PATCH_PREFIX}.ParseDict")
    @patch(f"{_PATCH_PREFIX}.get_user_name")
    def test_successful_rerun(self, mock_get_user, mock_parse_dict):
        """Test a successful batch rerun trigger run creation end-to-end."""
        mock_get_user.return_value = "test-user"
        trigger_run_resp = Mock()
        trigger_run_resp.trigger_run.metadata.name = "rerun-1"

        mock_crd = self._setup_and_bind(response=trigger_run_resp)

        result = mock_crd.rerun(
            namespace="test-ns",
            name="rerun-1",
            pipeline="my-pipeline",
            pipeline_run=["run-1", "run-2"],
            resume_from=["feature_gen"],
            resume_up_to=None,
            revision=None,
        )

        self.assertIs(result, trigger_run_resp)

        trigger_run_dict = mock_parse_dict.call_args_list[0][0][0]
        tr = trigger_run_dict["triggerRun"]
        self.assertEqual(tr["metadata"]["name"], "rerun-1")
        self.assertEqual(tr["metadata"]["namespace"], "test-ns")
        self.assertEqual(tr["spec"]["actor"]["name"], "test-user")
        self.assertEqual(
            tr["spec"]["pipeline"], {"name": "my-pipeline", "namespace": "test-ns"}
        )
        batch_rerun = tr["spec"]["trigger"]["batchRerun"]
        self.assertEqual(
            batch_rerun["pipelineRuns"],
            [
                {"name": "run-1", "namespace": "test-ns"},
                {"name": "run-2", "namespace": "test-ns"},
            ],
        )
        self.assertEqual(batch_rerun["resumeFrom"], ["feature_gen"])
        self.assertEqual(batch_rerun["resumeUpTo"], [])
        self.assertNotIn("revision", tr["spec"])

    @patch(f"{_PATCH_PREFIX}.ParseDict")
    @patch(f"{_PATCH_PREFIX}.get_user_name")
    def test_rerun_with_revision(self, mock_get_user, mock_parse_dict):
        """Test that an explicit --revision is included in the request."""
        mock_get_user.return_value = "test-user"
        mock_crd = self._setup_and_bind(response=Mock())

        mock_crd.rerun(
            namespace="ns",
            name="rerun-2",
            pipeline="pipe",
            pipeline_run=["run-1"],
            resume_from=None,
            resume_up_to=["train_model"],
            revision="rev-abc",
        )

        trigger_run_dict = mock_parse_dict.call_args_list[0][0][0]
        tr = trigger_run_dict["triggerRun"]
        self.assertEqual(tr["spec"]["revision"], {"name": "rev-abc", "namespace": "ns"})
        self.assertEqual(
            tr["spec"]["trigger"]["batchRerun"]["resumeUpTo"], ["train_model"]
        )

    @patch(f"{_PATCH_PREFIX}.ParseDict")
    @patch(f"{_PATCH_PREFIX}.get_user_name")
    def test_grpc_error_raises_runtime_error(self, mock_get_user, mock_parse_dict):
        """Test RuntimeError is raised when the gRPC create call fails."""
        mock_get_user.return_value = "test-user"
        error = _MockRpcError("internal error")
        mock_crd = self._setup_and_bind(error=error)

        with self.assertRaises(RuntimeError) as ctx:
            mock_crd.rerun(
                namespace="ns",
                name="rerun-3",
                pipeline="pipe",
                pipeline_run=["run-1"],
                resume_from=None,
                resume_up_to=None,
                revision=None,
            )

        self.assertIn("Failed to create batch rerun TriggerRun", str(ctx.exception))
