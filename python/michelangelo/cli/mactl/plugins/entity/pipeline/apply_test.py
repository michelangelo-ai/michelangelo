"""Unit tests for pipeline apply plugin."""

import json
from contextlib import redirect_stdout
from io import StringIO
from unittest import TestCase
from unittest.mock import MagicMock, Mock, patch

from google.protobuf.json_format import ParseDict, ParseError
from grpc import RpcError, StatusCode

from michelangelo.cli.mactl.plugins.entity.pipeline.apply import (
    pipeline_apply_func_impl,
)
from michelangelo.cli.mactl.plugins.entity.pipeline.declarative import (
    EXPLAIN_PLAN_ANNOTATION,
    DeclarativeManifestError,
)
from michelangelo.gen.api.v2.declarative_pb2 import DeclarativeWorkflow
from michelangelo.gen.api.v2.pipeline_svc_pb2 import (
    CreatePipelineRequest,
    CreatePipelineResponse,
    UpdatePipelineRequest,
    UpdatePipelineResponse,
)


class _FakeRpcError(RpcError):
    def __init__(self, code, details="boom"):
        self._code = code
        self._details = details

    def code(self):
        return self._code

    def details(self):
        return self._details


class PipelineApplyFuncImplTest(TestCase):
    """Tests for pipeline_apply_func_impl."""

    def _make_method_info(self):
        from michelangelo.cli.mactl.crd import CrdMethodInfo

        return CrdMethodInfo(
            channel=Mock(),
            crd_full_name="test.Service",
            method_name="Get",
            input_class=MagicMock(),
            output_class=MagicMock(),
        )

    def _make_bound_args(self, crd, file="f.yaml"):
        return Mock(arguments={"self": crd, "file": file})

    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.apply.crd_method_call")
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.apply.read_yaml_to_crd_request"
    )
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.apply.get_crd_namespace_and_name_from_yaml"
    )
    def test_update_path(self, mock_ns, mock_read_yaml, mock_call):
        """Existing pipeline triggers update path with resourceVersion copy."""
        update_info = self._make_method_info()
        mock_ns.return_value = ("ns", "pipe")
        mock_existing = Mock()
        mock_existing.pipeline.metadata.resourceVersion = "42"
        mock_crd = Mock()
        mock_crd.name = "pipeline"
        mock_crd._get.return_value = mock_existing
        mock_request = Mock()
        mock_read_yaml.return_value = mock_request

        pipeline_apply_func_impl(update_info, self._make_bound_args(mock_crd))

        mock_read_yaml.assert_called_once()
        mock_call.assert_called_once_with(update_info, mock_request)

    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.apply.crd_method_call")
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.apply.read_yaml_to_crd_request"
    )
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.apply.get_crd_namespace_and_name_from_yaml"
    )
    def test_create_path_when_not_found(self, mock_ns, mock_read_yaml, mock_call):
        """NOT_FOUND sends a Create request built by the pipeline converter."""
        update_info = self._make_method_info()
        mock_ns.return_value = ("ns", "pipe")
        mock_crd = Mock()
        mock_crd.name = "pipeline"
        mock_crd.full_name = "michelangelo.api.v2.PipelineService"
        mock_crd._get.side_effect = _FakeRpcError(StatusCode.NOT_FOUND)
        mock_crd._extract_method_info.return_value = (
            "Create",
            CreatePipelineRequest,
            CreatePipelineResponse,
        )
        mock_read_yaml.return_value = CreatePipelineRequest()

        pipeline_apply_func_impl(update_info, self._make_bound_args(mock_crd))

        mock_crd._extract_method_info.assert_called_once_with(
            update_info.channel, "michelangelo.api.v2.PipelineService", "Create"
        )
        mock_read_yaml.assert_called_once_with(
            CreatePipelineRequest,
            "pipeline",
            "f.yaml",
            mock_crd.func_crd_metadata_converter,
        )
        create_info, request = mock_call.call_args.args
        self.assertEqual(create_info.method_name, "Create")
        self.assertEqual(list(request.create_options.dryRun), [])

    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.apply.get_crd_namespace_and_name_from_yaml"
    )
    def test_non_not_found_get_error_exits_with_status_message(self, mock_ns):
        """A non-NOT_FOUND RpcError from get exits with its status, not a traceback."""
        update_info = self._make_method_info()
        mock_ns.return_value = ("ns", "pipe")
        mock_crd = Mock()
        mock_crd._get.side_effect = _FakeRpcError(StatusCode.UNAVAILABLE, "no route")

        with self.assertRaises(SystemExit) as err:
            pipeline_apply_func_impl(update_info, self._make_bound_args(mock_crd))

        self.assertEqual(err.exception.code, "Error: UNAVAILABLE: no route")


_PLAN = {
    "interpreterVersion": "core-v1.4.2",
    "levels": [{"tasks": ["root"]}, {"tasks": ["left", "right"]}],
    "warnings": [],
    "taskCount": 3,
}

_MODULE = "michelangelo.cli.mactl.plugins.entity.pipeline.apply"


def _update_response(annotations):
    """Build an UpdatePipelineResponse that carries the given annotations."""
    response = UpdatePipelineResponse()
    response.pipeline.metadata.name = "pipe"
    response.pipeline.metadata.annotations.update(annotations)
    return response


class PipelineApplyRenderTest(TestCase):
    """Tests for response rendering and the error boundary of pipeline apply."""

    def _run(self, response, output=None, dry_run=True, create=False, error=None):
        """Run pipeline apply against stubbed calls and return stdout."""
        crd = Mock()
        crd.name = "pipeline"
        crd.full_name = "michelangelo.api.v2.PipelineService"
        crd.get.side_effect = AssertionError("get prints a table row")
        if create:
            crd._get.side_effect = _FakeRpcError(StatusCode.NOT_FOUND)
            crd._extract_method_info.return_value = (
                "Create",
                CreatePipelineRequest,
                CreatePipelineResponse,
            )
            request = CreatePipelineRequest()
        else:
            crd._get.return_value = UpdatePipelineRequest()
            request = UpdatePipelineRequest()
        arguments = {"self": crd, "file": "f.yaml", "dry_run": dry_run}
        if output is not None:
            arguments["output"] = output
        update_info = Mock(channel=Mock())
        buf = StringIO()
        with (
            patch(
                f"{_MODULE}.get_crd_namespace_and_name_from_yaml",
                return_value=("ns", "pipe"),
            ),
            patch(f"{_MODULE}.read_yaml_to_crd_request", return_value=request),
            patch(
                f"{_MODULE}.crd_method_call", return_value=response, side_effect=error
            ) as call,
            redirect_stdout(buf),
        ):
            pipeline_apply_func_impl(update_info, Mock(arguments=arguments))
        self.sent = call.call_args.args[1]
        return buf.getvalue()

    def test_dry_run_update_prints_plan_table(self):
        """A dry-run update with a plan prints the plan table."""
        response = _update_response({EXPLAIN_PLAN_ANNOTATION: json.dumps(_PLAN)})

        out = self._run(response)

        self.assertEqual(
            out,
            "Interpreter: core-v1.4.2\n"
            "Tasks: 3\n"
            "\n"
            "LEVEL  TASKS\n"
            "0      root\n"
            "1      left, right\n",
        )
        self.assertEqual(list(self.sent.update_options.dryRun), ["All"])

    def test_dry_run_create_json_prints_only_json(self):
        """A dry-run create with -o json prints only the plan as JSON."""
        response = CreatePipelineResponse()
        response.pipeline.metadata.annotations[EXPLAIN_PLAN_ANNOTATION] = json.dumps(
            _PLAN
        )

        out = self._run(response, output="json", create=True)

        self.assertEqual(
            json.loads(out),
            {
                "interpreter_version": "core-v1.4.2",
                "levels": [{"tasks": ["root"]}, {"tasks": ["left", "right"]}],
                "warnings": [],
                "task_count": 3,
            },
        )
        self.assertEqual(list(self.sent.create_options.dryRun), ["All"])

    def test_response_without_plan_prints_as_before(self):
        """A response without the annotation prints the message as before."""
        response = _update_response({"team": "ml"})

        out = self._run(response, output="json", dry_run=False)

        self.assertEqual(out, f"{response}\n")
        self.assertEqual(list(self.sent.update_options.dryRun), [])

    def test_rejection_exits_with_status_message(self):
        """A server rejection exits with the gRPC status message."""
        rejection = _FakeRpcError(
            StatusCode.INVALID_ARGUMENT, "interpreter_pin is server-owned"
        )

        with self.assertRaises(SystemExit) as err:
            self._run(None, error=rejection)

        self.assertEqual(
            err.exception.code,
            "Error: INVALID_ARGUMENT: interpreter_pin is server-owned",
        )

    def test_manifest_error_exits_with_message(self):
        """A local manifest error exits with its message."""
        with (
            patch(
                f"{_MODULE}.read_yaml_to_crd_request",
                side_effect=DeclarativeManifestError("content.x: bad"),
            ),
            patch(
                f"{_MODULE}.get_crd_namespace_and_name_from_yaml",
                return_value=("ns", "pipe"),
            ),
        ):
            crd = Mock()
            with self.assertRaises(SystemExit) as err:
                pipeline_apply_func_impl(
                    Mock(), Mock(arguments={"self": crd, "file": "f.yaml"})
                )

        self.assertEqual(err.exception.code, "Error: content.x: bad")

    def test_parse_error_exits_on_one_line(self):
        """A real unknown-field ParseError prints on one line."""
        try:
            ParseDict({"tasks": {"a": {"taskFuncton": "x"}}}, DeclarativeWorkflow())
        except ParseError as parse_error:
            error = parse_error
        self.assertIn("\n", str(error))

        with (
            patch(f"{_MODULE}.read_yaml_to_crd_request", side_effect=error),
            patch(
                f"{_MODULE}.get_crd_namespace_and_name_from_yaml",
                return_value=("ns", "pipe"),
            ),
            self.assertRaises(SystemExit) as err,
        ):
            pipeline_apply_func_impl(
                Mock(), Mock(arguments={"self": Mock(), "file": "f.yaml"})
            )

        message = err.exception.code
        self.assertNotIn("\n", message)
        self.assertIn('has no field named "taskFuncton"', message)
        self.assertIn('"DeclarativeWorkflow.tasks[a]". Available Fields', message)

    def test_rejection_details_print_on_one_line(self):
        """Line breaks become spaces and other control characters are escaped."""
        rejection = _FakeRpcError(StatusCode.INVALID_ARGUMENT, "bad\r\n\tvalue\x1b[2J")

        with self.assertRaises(SystemExit) as err:
            self._run(None, error=rejection)

        self.assertEqual(
            err.exception.code, "Error: INVALID_ARGUMENT: bad value\\x1b[2J"
        )
