"""Tests for pipeline metadata declared as ``@uniflow.workflow(...)`` arguments."""

import importlib
import inspect
import unittest

import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.core.build import build
from michelangelo.uniflow.core.decorator import is_workflow
from michelangelo.uniflow.core.pipeline_metadata import (
    METADATA_ATTR,
    PipelineMetadataError,
    get_pipeline_metadata,
)
from tests.uniflow.registration.pipeline_root import pipeline_root


@uniflow.workflow(
    name="demo-pipeline", namespace="team", owner="jane.doe", type="train"
)
def decorated_workflow(x=1):
    """Return its input."""
    return x


@uniflow.workflow()
def bare_workflow(x=1):
    """Return its input."""
    return x


_WORKFLOW_BODY = '''
def demo(marker="m"):
    """Demo workflow."""
    return {"marker": marker}
'''


class WorkflowPipelineMetadataTest(unittest.TestCase):
    """Tests for the metadata arguments of uniflow.workflow."""

    def test_attaches_normalized_metadata_to_wrapper_and_original(self):
        """Metadata is reachable from the exported function and its original."""
        meta = get_pipeline_metadata(decorated_workflow)
        self.assertEqual("demo-pipeline", meta.name)
        self.assertEqual("PIPELINE_TYPE_TRAIN", meta.type)
        self.assertIs(meta, getattr(decorated_workflow, METADATA_ATTR))
        self.assertIs(meta, getattr(inspect.unwrap(decorated_workflow), METADATA_ATTR))
        self.assertIsInstance(meta, uniflow.PipelineMetadata)

    def test_structured_fields(self):
        """Labels, annotations, triggers and notifications are attached as given."""
        trigger = {"cronSchedule": {"cron": "0 8 * * *"}, "maxConcurrency": 1}

        @uniflow.workflow(
            labels={"team": "ml"},
            annotations={"example.com/note": "x"},
            triggers={"daily": trigger},
            notifications=[{"emails": ["a@example.com"]}],
        )
        def wf():
            pass

        meta = get_pipeline_metadata(wf)
        self.assertEqual({"team": "ml"}, meta.labels)
        self.assertEqual({"daily": trigger}, meta.triggers)
        self.assertIsNot(trigger, meta.triggers["daily"])
        self.assertEqual([{"emails": ["a@example.com"]}], meta.notifications)
        with self.assertRaisesRegex(PipelineMetadataError, r'triggers\["daily"\]: use'):
            uniflow.workflow(triggers={"daily": {"max_concurrency": 1}})(lambda: None)

    def test_bare_workflow_is_unchanged(self):
        """Without arguments nothing is attached and workflow behaviour is as before."""
        self.assertFalse(hasattr(bare_workflow, METADATA_ATTR))
        self.assertFalse(hasattr(inspect.unwrap(bare_workflow), METADATA_ATTR))
        self.assertIsNone(get_pipeline_metadata(bare_workflow))
        self.assertTrue(is_workflow(inspect.unwrap(bare_workflow)))
        self.assertFalse(is_workflow(bare_workflow))
        self.assertEqual(2, bare_workflow(x=2))
        self.assertEqual(2, decorated_workflow(x=2))

    def test_invalid_value_names_the_function(self):
        """Validation errors name the workflow function and its location."""

        def bad_workflow():
            pass

        with self.assertRaisesRegex(
            PipelineMetadataError,
            r"bad_workflow \(.*test_workflow_pipeline_metadata.py:\d+\): "
            r'type "TRAINING" is not a valid pipeline type; did you mean "TRAIN"\?',
        ):
            uniflow.workflow(type="TRAINING")(bad_workflow)
        self.assertFalse(is_workflow(bad_workflow))

    def test_unknown_or_positional_arguments_are_type_errors(self):
        """Typos and positional arguments fail as ordinary TypeErrors."""
        with self.assertRaises(TypeError):
            uniflow.workflow(nmae="x")  # type: ignore[call-arg]
        with self.assertRaises(TypeError):
            uniflow.workflow("x")  # type: ignore[misc]

    def test_starlark_output_is_unaffected(self):
        """The transpiled Starlark is byte-identical with and without metadata."""
        plain = "import michelangelo.uniflow.core as uniflow\n\n@uniflow.workflow()"
        with_meta = (
            "import michelangelo.uniflow.core as uniflow\n\n"
            "@uniflow.workflow(\n"
            '    name="demo",\n'
            '    namespace="team",\n'
            '    owner="jane.doe",\n'
            '    labels={"team": "ml"},\n'
            '    triggers={"daily": {"cronSchedule": {"cron": "0 8 * * *"}}},\n'
            ")"
        )
        with pipeline_root(
            {
                "plainpkg/wf.py": plain + _WORKFLOW_BODY,
                "metapkg/wf.py": with_meta + _WORKFLOW_BODY,
            }
        ):
            plain_pkg = build(importlib.import_module("plainpkg.wf").demo)
            meta_pkg = build(importlib.import_module("metapkg.wf").demo)

        self.assertEqual(
            plain_pkg.files[plain_pkg.main_file], meta_pkg.files[meta_pkg.main_file]
        )
        self.assertEqual(plain_pkg.main_function, meta_pkg.main_function)
        self.assertEqual(len(plain_pkg.files), len(meta_pkg.files))
        self.assertNotIn(b"jane.doe", meta_pkg.files[meta_pkg.main_file])
