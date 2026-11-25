"""Unit tests for pipeline apply plugin."""

import json
from contextlib import redirect_stderr, redirect_stdout
from io import StringIO
from pathlib import Path
from tempfile import TemporaryDirectory
from unittest import TestCase
from unittest.mock import MagicMock, Mock, patch

from google.protobuf import descriptor_pb2, descriptor_pool, message_factory
from google.protobuf.json_format import ParseDict, ParseError
from grpc import RpcError, StatusCode

from michelangelo.cli.mactl.crd import yaml_to_dict
from michelangelo.cli.mactl.plugins.entity.pipeline.apply import (
    pipeline_apply_func_impl,
)
from michelangelo.cli.mactl.plugins.entity.pipeline.create import (
    convert_crd_metadata_pipeline,
)
from michelangelo.cli.mactl.plugins.entity.pipeline.declarative import (
    EXPLAIN_PLAN_ANNOTATION,
    DeclarativeManifestError,
)
from michelangelo.gen.api.v2.declarative_pb2 import DeclarativeWorkflow
from michelangelo.gen.api.v2.pipeline_pb2 import PipelineManifest
from michelangelo.gen.api.v2.pipeline_svc_pb2 import (
    CreatePipelineRequest,
    CreatePipelineResponse,
    UpdatePipelineRequest,
    UpdatePipelineResponse,
)

_MODULE = "michelangelo.cli.mactl.plugins.entity.pipeline.apply"


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

    def setUp(self):
        """Stub the YAML read of the early manifest check; f.yaml is not a file."""
        patcher = patch(f"{_MODULE}.yaml_to_dict", return_value={})
        patcher.start()
        self.addCleanup(patcher.stop)

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
        self.assertEqual(mock_request.pipeline.metadata.resourceVersion, "42")
        mock_request.update_options.dryRun.append.assert_not_called()

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

    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.apply.crd_method_call")
    def test_bad_declarative_file_fails_before_any_call(self, mock_call):
        """A manifest error exits before the get, so the apiserver sees no call."""
        cases = [
            (
                "uniflowTar: s3://x",
                "{tasks: {a: {taskFunction: mock/python}}}",
                "Error: spec.manifest.uniflowTar is server-owned for declarative"
                " pipelines; remove it, because the apiserver sets it from the"
                " interpreter pin",
            ),
            (
                "description: unknown content field",
                "{tasks: {a: {taskFuncton: mock/python}}}",
                'Error: Failed to parse tasks field: Message type "michelangelo.api'
                '.v2.DeclarativeTask" has no field named "taskFuncton"',
            ),
        ]
        for manifest_line, content, message in cases:
            with self.subTest(message=message):
                update_info = self._make_method_info()
                mock_crd = Mock(
                    func_crd_metadata_converter=convert_crd_metadata_pipeline
                )
                mock_crd.name = "pipeline"
                mock_crd._get.return_value = UpdatePipelineResponse()
                with TemporaryDirectory() as tmp:
                    path = Path(tmp) / "p.yaml"
                    path.write_text(
                        "metadata: {name: pipe, namespace: ns}\n"
                        "spec:\n"
                        "  manifest:\n"
                        "    type: DECLARATIVE\n"
                        f"    {manifest_line}\n"
                        f"    content: {content}\n"
                    )

                    with (
                        patch(f"{_MODULE}.yaml_to_dict", side_effect=yaml_to_dict),
                        self.assertRaises(SystemExit) as err,
                    ):
                        pipeline_apply_func_impl(
                            update_info, self._make_bound_args(mock_crd, str(path))
                        )

                self.assertTrue(
                    err.exception.code.startswith(message), err.exception.code
                )
                self.assertEqual(update_info.channel.mock_calls, [])
                self.assertEqual(mock_crd.mock_calls, [])
                mock_call.assert_not_called()

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


def _update_response(annotations):
    """Build an UpdatePipelineResponse that carries the given annotations."""
    response = UpdatePipelineResponse()
    response.pipeline.metadata.name = "pipe"
    response.pipeline.metadata.annotations.update(annotations)
    return response


def _reflected_class(message_class):
    """Return message_class built in a separate pool, as server reflection does."""
    pool = descriptor_pool.DescriptorPool()
    added = set()

    def add(file_descriptor):
        if file_descriptor.name in added:
            return
        for dependency in file_descriptor.dependencies:
            add(dependency)
        file_proto = descriptor_pb2.FileDescriptorProto()
        file_descriptor.CopyToProto(file_proto)
        pool.Add(file_proto)
        added.add(file_descriptor.name)

    add(message_class.DESCRIPTOR.file)
    return message_factory.GetMessageClass(
        pool.FindMessageTypeByName(message_class.DESCRIPTOR.full_name)
    )


class PipelineApplyRenderTest(TestCase):
    """Tests for response rendering and the error boundary of pipeline apply."""

    def setUp(self):
        """Stub the YAML read of the early manifest check; f.yaml is not a file."""
        patcher = patch(f"{_MODULE}.yaml_to_dict", return_value={})
        patcher.start()
        self.addCleanup(patcher.stop)

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
            "1      left, right\n"
            "\n"
            "Dry run: nothing was saved.\n",
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
        """A non-declarative response prints the text form as before."""
        response = _update_response({"team": "ml"})
        response.pipeline.spec.manifest.type = (
            PipelineManifest.PIPELINE_MANIFEST_TYPE_UNIFLOW
        )

        out = self._run(response, dry_run=False)

        self.assertEqual(out, f"{response}\n")
        self.assertEqual(list(self.sent.update_options.dryRun), [])

    def test_declarative_response_prints_the_workflow(self):
        """A declarative response shows the stored workflow, not escaped bytes."""
        response = _update_response({})
        response.pipeline.spec.manifest.type = (
            PipelineManifest.PIPELINE_MANIFEST_TYPE_DECLARATIVE
        )
        response.pipeline.spec.manifest.content.Pack(
            DeclarativeWorkflow(schema_version="v1alpha1")
        )

        out = self._run(response, dry_run=False)

        self.assertIn(
            "[type.googleapis.com/michelangelo.api.v2.DeclarativeWorkflow] {\n"
            '          schema_version: "v1alpha1"\n',
            out,
        )
        self.assertNotIn("value:", out)

    def test_reflected_declarative_response_prints_the_workflow(self):
        """The CLI response class comes from a reflection pool, not pipeline_pb2."""
        response = _reflected_class(UpdatePipelineResponse)()
        self.assertNotIsInstance(response, UpdatePipelineResponse)
        response.pipeline.spec.manifest.type = (
            PipelineManifest.PIPELINE_MANIFEST_TYPE_DECLARATIVE
        )
        response.pipeline.spec.manifest.content.Pack(
            DeclarativeWorkflow(schema_version="v1alpha1")
        )

        out = self._run(response, dry_run=False)
        json_out = self._run(response, output="json", dry_run=False)

        self.assertIn(
            "[type.googleapis.com/michelangelo.api.v2.DeclarativeWorkflow] {", out
        )
        self.assertNotIn("value:", out)
        self.assertEqual(
            json.loads(json_out)["pipeline"]["spec"]["manifest"]["content"],
            {
                "@type": "type.googleapis.com/michelangelo.api.v2.DeclarativeWorkflow",
                "schema_version": "v1alpha1",
            },
        )

    def test_json_output_applies_to_every_response(self):
        """-o json prints a response without a plan as JSON."""
        response = _update_response({"team": "ml"})

        out = self._run(response, output="json", dry_run=False)

        self.assertEqual(
            json.loads(out)["pipeline"]["metadata"],
            {"name": "pipe", "annotations": {"team": "ml"}},
        )

    def test_json_output_falls_back_to_text_for_unknown_any(self):
        """An Any type that JSON cannot resolve prints as text with a warning."""
        response = _update_response({})
        response.pipeline.spec.manifest.content.type_url = (
            "type.googleapis.com/unknown.Type"
        )
        stderr = StringIO()

        with redirect_stderr(stderr):
            out = self._run(response, output="json", dry_run=False)

        self.assertEqual(out, f"{response}\n")
        self.assertEqual(
            stderr.getvalue(),
            "Warning: cannot print the response as JSON (Can not find message"
            " descriptor by type_url: type.googleapis.com/unknown.Type);"
            " printing it as text\n",
        )

    def test_write_ignores_the_plan_annotation(self):
        """Without --dry-run, mactl does not read the plan annotation."""
        response = _update_response({EXPLAIN_PLAN_ANNOTATION: "{not json"})

        out = self._run(response, dry_run=False)

        self.assertEqual(out, f"{response}\n")

    def test_unreadable_plan_on_dry_run_names_the_annotation(self):
        """A dry run with a malformed plan exits with the annotation name."""
        response = _update_response({EXPLAIN_PLAN_ANNOTATION: "{not json"})

        with self.assertRaises(SystemExit) as err:
            self._run(response)

        self.assertTrue(
            err.exception.code.startswith(
                f"Error: cannot read the {EXPLAIN_PLAN_ANNOTATION} annotation:"
                " Failed to load JSON"
            ),
            err.exception.code,
        )

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
