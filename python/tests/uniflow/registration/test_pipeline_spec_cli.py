"""Tests for ``python -m michelangelo.uniflow.registration.pipeline_spec``."""

import io
import json
import os
import subprocess
import sys
import unittest
from pathlib import Path

import yaml

from michelangelo.uniflow.core.pipeline_metadata import (
    PipelineMetadata,
    PipelineMetadataError,
)
from michelangelo.uniflow.registration.pipeline_spec import (
    ResolvedPipeline,
    WorkflowImportError,
    main,
)

_PYTHON_ROOT = Path(__file__).resolve().parents[3]

_RESOLVED = ResolvedPipeline(
    metadata=PipelineMetadata(name="demo", namespace="ns", owner="o"),
    module="workflows.wf.workflow",
    function="demo",
    source_path="workflows/wf/workflow.py",
    source="decorator",
    pipeline_cr={"apiVersion": "michelangelo.api/v2", "kind": "Pipeline"},
    warnings=["something to know"],
)


def _run(argv, resolver=None):
    stdout, stderr = io.StringIO(), io.StringIO()
    kwargs = {"resolver": resolver} if resolver else {}
    code = main(argv, stdout=stdout, stderr=stderr, **kwargs)
    return code, stdout.getvalue(), stderr.getvalue()


def _raising(error):
    def resolver(**kwargs):
        raise error

    return resolver


class CliTest(unittest.TestCase):
    """Tests for cli.main."""

    def test_json_output_and_warnings(self):
        """JSON goes to stdout, warnings to stderr."""
        calls = []

        def resolver(**kwargs):
            calls.append(kwargs)
            return _RESOLVED

        code, out, err = _run(
            ["--module", "workflows.wf.workflow", "--root", "/r"], resolver
        )
        self.assertEqual(0, code)
        self.assertEqual(1, json.loads(out)["schema_version"])
        self.assertEqual("warning: something to know\n", err)
        self.assertEqual(
            [
                {
                    "module": "workflows.wf.workflow",
                    "config_file": None,
                    "function": None,
                    "root": "/r",
                    "sibling_yaml": "pipeline.yaml",
                    "required": ("name", "namespace"),
                }
            ],
            calls,
        )

    def test_sibling_yaml_and_require_flags(self):
        """--sibling-yaml and --require reach the resolver; '' disables the lookup."""
        calls = []

        def resolver(**kwargs):
            calls.append(kwargs)
            return _RESOLVED

        argv = ["--module", "m", "--require", "owner", "--require", "name"]
        self.assertEqual(0, _run([*argv, "--sibling-yaml", "custom.yaml"], resolver)[0])
        self.assertEqual(0, _run(["--module", "m", "--sibling-yaml", ""], resolver)[0])
        self.assertEqual("custom.yaml", calls[0]["sibling_yaml"])
        self.assertEqual(("name", "namespace", "owner"), calls[0]["required"])
        self.assertIsNone(calls[1]["sibling_yaml"])

    def test_yaml_output_is_the_bare_cr(self):
        """--format yaml prints only the Pipeline CR."""
        code, out, _ = _run(
            ["--config-file", "x.yaml", "--format", "yaml"], lambda **_: _RESOLVED
        )
        self.assertEqual(0, code)
        self.assertEqual(_RESOLVED.pipeline_cr, yaml.safe_load(out))

    def test_user_prints_do_not_pollute_stdout(self):
        """Output printed while importing the workflow is moved to stderr."""

        def resolver(**kwargs):
            print("hello from workflow.py")
            return _RESOLVED

        code, out, err = _run(["--module", "m"], resolver)
        self.assertEqual(0, code)
        json.loads(out)
        self.assertIn("hello from workflow.py", err)

    def test_invalid_metadata_and_import_errors_exit_3(self):
        """Actionable user errors exit 3 with the message on stderr."""
        for error in (
            PipelineMetadataError("bad type"),
            WorkflowImportError("no module"),
        ):
            with self.subTest(error=type(error).__name__):
                code, out, err = _run(["--module", "m"], _raising(error))
                self.assertEqual(3, code)
                self.assertEqual("", out)
                self.assertEqual(f"error: {error}\n", err)

    def test_unexpected_errors_exit_1(self):
        """Unexpected failures exit 1 with a traceback."""
        code, out, err = _run(["--module", "m"], _raising(RuntimeError("boom")))
        self.assertEqual(1, code)
        self.assertEqual("", out)
        self.assertIn("RuntimeError: boom", err)

    def test_usage_errors_exit_2(self):
        """Missing or conflicting sources are usage errors."""
        for argv in (
            [],
            ["--module", "m", "--config-file", "c"],
            ["--module", "m", "--format", "xml"],
            ["--module", "m", "--require", "nmae"],
        ):
            with self.subTest(argv=argv):
                self.assertEqual(2, _run(argv)[0])

    def test_module_entry_point(self):
        """``python -m`` runs the CLI."""
        env = dict(os.environ, PYTHONPATH=str(_PYTHON_ROOT))
        result = subprocess.run(
            [
                sys.executable,
                "-m",
                "michelangelo.uniflow.registration.pipeline_spec",
                "--help",
            ],
            capture_output=True,
            text=True,
            env=env,
            check=False,
        )
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertIn("--module", result.stdout)
