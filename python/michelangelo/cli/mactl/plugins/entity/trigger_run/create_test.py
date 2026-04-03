"""Unit tests for trigger_run create plugin.

Tests the add_function_signature and generate_create functions.
"""

from inspect import Parameter, Signature
from unittest import TestCase
from unittest.mock import Mock, patch

from grpc import RpcError

from michelangelo.cli.mactl.plugins.entity.trigger_run.create import (
    add_function_signature,
    generate_create,
)

_PATCH_PREFIX = "michelangelo.cli.mactl.plugins.entity.trigger_run.create"

# Reusable Signature matching the create command's expected parameters
_CREATE_SIGNATURE = Signature(
    parameters=[
        Parameter("self", Parameter.POSITIONAL_OR_KEYWORD),
        Parameter("namespace", Parameter.POSITIONAL_OR_KEYWORD),
        Parameter("pipeline", Parameter.POSITIONAL_OR_KEYWORD),
        Parameter("trigger_name", Parameter.POSITIONAL_OR_KEYWORD),
    ]
)


class _MockRpcError(RpcError):
    """Concrete RpcError subclass for testing."""

    def __init__(self, code, details_msg):
        self._code = code
        self._details = details_msg
        super().__init__(details_msg)

    def code(self):
        return self._code

    def details(self):
        return self._details


def _make_crd_mock():
    """Build a CRD mock that returns a real Signature from _read_signatures."""
    mock_crd = Mock()
    mock_crd.full_name = "michelangelo.api.v2.TriggerRunService"
    mock_crd._read_signatures.return_value = _CREATE_SIGNATURE
    mock_crd.configure_parser = Mock()
    return mock_crd


class AddFunctionSignatureTest(TestCase):
    """Tests for add_function_signature."""

    def test_adds_create_entry(self):
        """Test that a 'create' entry is added to func_signature."""
        mock_crd = Mock()
        mock_crd.func_signature = {}

        add_function_signature(mock_crd)

        self.assertIn("create", mock_crd.func_signature)

    def test_help_text(self):
        """Test that help text is set correctly."""
        mock_crd = Mock()
        mock_crd.func_signature = {}

        add_function_signature(mock_crd)

        sig = mock_crd.func_signature["create"]
        self.assertEqual(
            sig["help"],
            "Create a TriggerRun from a pipeline's trigger configuration.",
        )

    def test_has_three_args(self):
        """Test that exactly three args are defined."""
        mock_crd = Mock()
        mock_crd.func_signature = {}

        add_function_signature(mock_crd)

        self.assertEqual(len(mock_crd.func_signature["create"]["args"]), 3)

    def test_namespace_arg(self):
        """Test namespace arg flags and kwargs."""
        mock_crd = Mock()
        mock_crd.func_signature = {}

        add_function_signature(mock_crd)

        arg = mock_crd.func_signature["create"]["args"][0]
        self.assertEqual(arg["args"], ["-n", "--namespace"])
        self.assertTrue(arg["kwargs"]["required"])
        self.assertEqual(arg["kwargs"]["type"], str)

    def test_pipeline_arg(self):
        """Test pipeline arg flags and kwargs."""
        mock_crd = Mock()
        mock_crd.func_signature = {}

        add_function_signature(mock_crd)

        arg = mock_crd.func_signature["create"]["args"][1]
        self.assertEqual(arg["args"], ["-p", "--pipeline"])
        self.assertTrue(arg["kwargs"]["required"])
        self.assertEqual(arg["kwargs"]["type"], str)

    def test_trigger_name_arg(self):
        """Test trigger_name arg flags and kwargs."""
        mock_crd = Mock()
        mock_crd.func_signature = {}

        add_function_signature(mock_crd)

        arg = mock_crd.func_signature["create"]["args"][2]
        self.assertEqual(arg["args"], ["-t", "--trigger-name"])
        self.assertTrue(arg["kwargs"]["required"])
        self.assertEqual(arg["kwargs"]["type"], str)


class GenerateCreateSetupTest(TestCase):
    """Tests for generate_create setup phase (before the bound function runs)."""

    def test_calls_extract_method_info(self):
        """Test that _extract_method_info is called with Create."""
        mock_crd = _make_crd_mock()
        mock_crd._extract_method_info.return_value = ("Create", Mock(), Mock())
        mock_channel = Mock()

        generate_create(mock_crd, mock_channel, Mock())

        mock_crd._extract_method_info.assert_called_once_with(
            mock_channel, mock_crd.full_name, "Create"
        )

    def test_calls_configure_parser(self):
        """Test that configure_parser is called with 'create'."""
        mock_crd = _make_crd_mock()
        mock_crd._extract_method_info.return_value = ("Create", Mock(), Mock())
        mock_channel = Mock()
        mock_parser = Mock()

        generate_create(mock_crd, mock_channel, mock_parser)

        mock_crd.configure_parser.assert_called_once_with("create", mock_parser)

    def test_calls_read_signatures(self):
        """Test that _read_signatures is called with 'create'."""
        mock_crd = _make_crd_mock()
        mock_crd._extract_method_info.return_value = ("Create", Mock(), Mock())
        mock_channel = Mock()

        generate_create(mock_crd, mock_channel, Mock())

        mock_crd._read_signatures.assert_called_once_with("create")

    def test_binds_create_method(self):
        """Test that a callable create method is bound to the CRD."""
        mock_crd = _make_crd_mock()
        mock_crd._extract_method_info.return_value = ("Create", Mock(), Mock())
        mock_channel = Mock()

        generate_create(mock_crd, mock_channel, Mock())

        self.assertTrue(hasattr(mock_crd, "create"))
        self.assertTrue(callable(mock_crd.create))


class GenerateCreateFunctionTest(TestCase):
    """Tests for the bound create function's runtime behavior."""

    def _setup_and_bind(
        self,
        pipeline_response=None,
        pipeline_error=None,
        trigger_run_response=None,
        trigger_run_error=None,
    ):
        """Set up mocks, call generate_create, and return the CRD.

        This helper builds all the mocks needed to invoke the bound
        create function.  It patches get_methods_from_service and
        get_message_class_by_name for the PipelineService lookup.
        """
        mock_crd = _make_crd_mock()
        mock_channel = Mock()

        pipeline_input_class = Mock()
        pipeline_output_class = Mock()
        trigger_run_input_class = Mock()
        trigger_run_output_class = Mock()

        mock_crd._extract_method_info.return_value = (
            "CreateTriggerRun",
            trigger_run_input_class,
            trigger_run_output_class,
        )

        # Mock PipelineService method lookup
        mock_pipeline_method = Mock()
        mock_pipeline_method.input_type = ".GetPipelineRequest"
        mock_pipeline_method.output_type = ".GetPipelineResponse"

        self._mock_get_methods = patch(
            f"{_PATCH_PREFIX}.get_methods_from_service",
            return_value=(
                {"GetPipeline": mock_pipeline_method},
                Mock(),
            ),
        )
        self._mock_get_methods.start()

        def get_class_side_effect(pool, name):
            if "Request" in name:
                return pipeline_input_class
            return pipeline_output_class

        self._mock_get_class = patch(
            f"{_PATCH_PREFIX}.get_message_class_by_name",
            side_effect=get_class_side_effect,
        )
        self._mock_get_class.start()

        # Channel stubs
        mock_pipeline_stub = Mock(
            side_effect=pipeline_error,
            return_value=pipeline_response,
        )
        mock_trigger_run_stub = Mock(
            side_effect=trigger_run_error,
            return_value=trigger_run_response,
        )

        def unary_unary_side_effect(method_fullname, **kwargs):
            if "PipelineService" in method_fullname:
                return mock_pipeline_stub
            return mock_trigger_run_stub

        mock_channel.unary_unary.side_effect = unary_unary_side_effect

        generate_create(mock_crd, mock_channel, Mock())

        return mock_crd

    def tearDown(self):
        """Stop patches started by _setup_and_bind."""
        for p in ("_mock_get_methods", "_mock_get_class"):
            patcher = getattr(self, p, None)
            if patcher:
                patcher.stop()

    @patch(f"{_PATCH_PREFIX}.ParseDict")
    @patch(f"{_PATCH_PREFIX}.MessageToDict")
    @patch(f"{_PATCH_PREFIX}.uuid")
    @patch(f"{_PATCH_PREFIX}.get_user_name")
    def test_successful_create(
        self, mock_get_user, mock_uuid, mock_msg_to_dict, mock_parse_dict
    ):
        """Test a successful trigger run creation end-to-end."""
        mock_get_user.return_value = "test-user"
        mock_uuid.uuid4.return_value = Mock(hex="a1b2c3d400000000")
        mock_msg_to_dict.return_value = {
            "trigger_map": {
                "daily-01": {
                    "cronSchedule": {"cron": "0 8 * * *"},
                    "maxConcurrency": 1,
                }
            }
        }

        mock_pipeline = Mock()
        mock_pipeline.metadata.name = "my-pipeline"
        mock_pipeline.spec.manifest.trigger_map = Mock()
        pipeline_resp = Mock()
        pipeline_resp.pipeline = mock_pipeline

        trigger_run_resp = Mock()
        trigger_run_resp.trigger_run.metadata.name = "daily-01-a1b2c3d4"

        mock_crd = self._setup_and_bind(
            pipeline_response=pipeline_resp,
            trigger_run_response=trigger_run_resp,
        )

        result = mock_crd.create(
            namespace="test-ns", pipeline="my-pipeline", trigger_name="daily-01"
        )

        self.assertIs(result, trigger_run_resp)

        # Verify ParseDict was called to build the TriggerRun request
        create_call_args = mock_parse_dict.call_args_list
        # Second call is the TriggerRun create (first is GetPipelineRequest)
        trigger_run_dict = create_call_args[1][0][0]
        tr = trigger_run_dict["triggerRun"]
        self.assertEqual(tr["metadata"]["name"], "daily-01-a1b2c3d4")
        self.assertEqual(tr["metadata"]["namespace"], "test-ns")
        self.assertEqual(tr["spec"]["sourceTriggerName"], "daily-01")
        self.assertEqual(tr["spec"]["actor"]["name"], "test-user")
        self.assertEqual(
            tr["spec"]["pipeline"],
            {"name": "my-pipeline", "namespace": "test-ns"},
        )

    @patch(f"{_PATCH_PREFIX}.ParseDict")
    @patch(f"{_PATCH_PREFIX}.MessageToDict")
    @patch(f"{_PATCH_PREFIX}.uuid")
    @patch(f"{_PATCH_PREFIX}.get_user_name")
    def test_trigger_run_name_format(
        self, mock_get_user, mock_uuid, mock_msg_to_dict, mock_parse_dict
    ):
        """Test that trigger run name follows {trigger_name}_{8-hex} format."""
        mock_get_user.return_value = "user"
        mock_uuid.uuid4.return_value = Mock(hex="e70c6f9200000000")
        mock_msg_to_dict.return_value = {
            "trigger_map": {
                "hourly": {
                    "intervalSchedule": {"interval": "1h"},
                    "maxConcurrency": 2,
                }
            }
        }

        mock_pipeline = Mock()
        mock_pipeline.spec.manifest.trigger_map = Mock()
        pipeline_resp = Mock()
        pipeline_resp.pipeline = mock_pipeline

        trigger_run_resp = Mock()
        trigger_run_resp.trigger_run.metadata.name = "hourly-e70c6f92"

        mock_crd = self._setup_and_bind(
            pipeline_response=pipeline_resp,
            trigger_run_response=trigger_run_resp,
        )

        mock_crd.create(namespace="ns", pipeline="pipe", trigger_name="hourly")

        create_call_dict = mock_parse_dict.call_args_list[1][0][0]
        self.assertEqual(
            create_call_dict["triggerRun"]["metadata"]["name"],
            "hourly-e70c6f92",
        )

    @patch(f"{_PATCH_PREFIX}.ParseDict")
    def test_pipeline_not_found_raises_value_error(self, mock_parse_dict):
        """Test ValueError is raised when the pipeline is not found."""
        from grpc import StatusCode

        error = _MockRpcError(StatusCode.NOT_FOUND, "not found")

        mock_crd = self._setup_and_bind(pipeline_error=error)

        with self.assertRaises(ValueError) as ctx:
            mock_crd.create(
                namespace="ns",
                pipeline="missing-pipeline",
                trigger_name="t",
            )

        self.assertIn("not found", str(ctx.exception))
        self.assertIn("missing-pipeline", str(ctx.exception))

    @patch(f"{_PATCH_PREFIX}.ParseDict")
    def test_pipeline_grpc_error_raises_runtime_error(self, mock_parse_dict):
        """Test RuntimeError is raised for non-NOT_FOUND gRPC errors."""
        from grpc import StatusCode

        error = _MockRpcError(StatusCode.INTERNAL, "server error")

        mock_crd = self._setup_and_bind(pipeline_error=error)

        with self.assertRaises(RuntimeError) as ctx:
            mock_crd.create(namespace="ns", pipeline="pipe", trigger_name="t")

        self.assertIn("Failed to get pipeline", str(ctx.exception))

    @patch(f"{_PATCH_PREFIX}.ParseDict")
    def test_pipeline_without_manifest_raises_value_error(self, mock_parse_dict):
        """Test ValueError when pipeline has no manifest attribute."""
        mock_pipeline = Mock(spec=["metadata"])
        mock_pipeline.metadata.name = "pipe"
        # spec has no 'manifest'
        mock_pipeline.spec = Mock(spec=[])
        pipeline_resp = Mock()
        pipeline_resp.pipeline = mock_pipeline

        mock_crd = self._setup_and_bind(pipeline_response=pipeline_resp)

        with self.assertRaises(ValueError) as ctx:
            mock_crd.create(namespace="ns", pipeline="pipe", trigger_name="t")

        self.assertIn("does not have any triggers", str(ctx.exception))

    @patch(f"{_PATCH_PREFIX}.ParseDict")
    @patch(f"{_PATCH_PREFIX}.MessageToDict")
    def test_trigger_name_not_in_map_raises_value_error(
        self, mock_msg_to_dict, mock_parse_dict
    ):
        """Test ValueError when trigger name is not in the trigger map."""
        mock_msg_to_dict.return_value = {
            "trigger_map": {
                "existing-trigger": {"cronSchedule": {"cron": "0 0 * * *"}}
            }
        }

        mock_pipeline = Mock()
        mock_pipeline.spec.manifest.trigger_map = Mock()
        pipeline_resp = Mock()
        pipeline_resp.pipeline = mock_pipeline

        mock_crd = self._setup_and_bind(pipeline_response=pipeline_resp)

        with self.assertRaises(ValueError) as ctx:
            mock_crd.create(
                namespace="ns",
                pipeline="pipe",
                trigger_name="nonexistent",
            )

        self.assertIn("nonexistent", str(ctx.exception))
        self.assertIn("Available triggers", str(ctx.exception))
        self.assertIn("existing-trigger", str(ctx.exception))

    @patch(f"{_PATCH_PREFIX}.ParseDict")
    @patch(f"{_PATCH_PREFIX}.MessageToDict")
    @patch(f"{_PATCH_PREFIX}.uuid")
    @patch(f"{_PATCH_PREFIX}.get_user_name")
    def test_trigger_run_grpc_error_raises_runtime_error(
        self, mock_get_user, mock_uuid, mock_msg_to_dict, mock_parse_dict
    ):
        """Test RuntimeError when the TriggerRun gRPC create call fails."""
        from grpc import StatusCode

        mock_get_user.return_value = "user"
        mock_uuid.uuid4.return_value = Mock(hex="0000000000000000")
        mock_msg_to_dict.return_value = {
            "trigger_map": {
                "t1": {
                    "cronSchedule": {"cron": "* * * * *"},
                    "maxConcurrency": 1,
                }
            }
        }

        mock_pipeline = Mock()
        mock_pipeline.spec.manifest.trigger_map = Mock()
        pipeline_resp = Mock()
        pipeline_resp.pipeline = mock_pipeline

        error = _MockRpcError(StatusCode.INTERNAL, "internal error")

        mock_crd = self._setup_and_bind(
            pipeline_response=pipeline_resp,
            trigger_run_error=error,
        )

        with self.assertRaises(RuntimeError) as ctx:
            mock_crd.create(namespace="ns", pipeline="pipe", trigger_name="t1")

        self.assertIn("Failed to create TriggerRun", str(ctx.exception))
