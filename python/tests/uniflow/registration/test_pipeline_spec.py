"""Tests for resolving a Pipeline CR from inline workflow metadata and YAML."""

import importlib
import os
import sys
import types
import unittest

import yaml

from michelangelo.uniflow.core.pipeline_metadata import (
    PipelineMetadata,
    PipelineMetadataError,
)
from michelangelo.uniflow.registration.pipeline_spec import (
    LocalFileSystem,
    WorkflowImportError,
    _module_source,
    apply_defaults,
    build_pipeline_cr,
    check_required,
    ctx_run_targets,
    find_workflow_function,
    load_pipeline_yaml,
    load_sibling_yaml,
    merge,
    parse_pipeline_yaml,
    resolve,
)
from tests.uniflow.registration.pipeline_root import FakeGit, pipeline_root

_IMPORTS = "import michelangelo.uniflow.core as uniflow\n"

_PLAIN_WORKFLOW = (
    _IMPORTS
    + """
@uniflow.workflow()
def demo(marker="m"):
    return marker
"""
)

# A pipeline YAML setting every field the inline metadata models.
_CR_YAML = """\
apiVersion: michelangelo.api/v2
kind: Pipeline
metadata:
  name: {name}
  namespace: ma-examples
  annotations:
    michelangelo/uniflow-image: ghcr.io/michelangelo-ai/examples:main
spec:
  description: {description}
  owner:
    name: jane.doe
  type: PIPELINE_TYPE_TRAIN
  manifest:
    type: PIPELINE_MANIFEST_TYPE_UNIFLOW
    filePath: workflows.{wf}.workflow
  commit:
    gitRef: "master"
    branch: "master"
"""

# The inline equivalent of _CR_YAML (the commit comes from git).
_CR_WORKFLOW = (
    _IMPORTS
    + """
@uniflow.workflow(
    name="{name}",
    namespace="ma-examples",
    owner="jane.doe",
    description="{description}",
    type="TRAIN",
    image="ghcr.io/michelangelo-ai/examples:main",
)
def {wf}(marker="m"):
    return marker
"""
)

# Shaped like python/examples/pipelines/*/pipeline.yaml: no owner or commit,
# which mactl fills in at apply time.
_OSS_YAML = """\
apiVersion: michelangelo.api/v2
kind: Pipeline
metadata:
  namespace: "ma-examples"
  name: "california-housing-xgb"
  annotations:
    michelangelo/uniflow-image: ghcr.io/michelangelo-ai/examples:main
spec:
  type: "PIPELINE_TYPE_TRAIN"
  manifest:
    filePath: pipelines.california_housing_xgb.california_housing_xgb
"""

_OSS_WORKFLOW = (
    _IMPORTS
    + """
@uniflow.workflow(
    name="california-housing-xgb",
    namespace="ma-examples",
    type="TRAIN",
    image="ghcr.io/michelangelo-ai/examples:main",
)
def train_workflow(marker="m"):
    return marker
"""
)

# A pipeline YAML using every user-authored field, and its inline equivalent.
_FULL_YAML = """\
apiVersion: michelangelo.api/v2
kind: Pipeline
metadata:
  name: bert-cola
  namespace: ma-examples
  labels:
    team: ml
  annotations:
    example.com/cost-center: "1234"
    michelangelo/uniflow-image: ghcr.io/michelangelo-ai/examples:main
spec:
  description: Fine-tunes BERT on CoLA.
  owner:
    name: jane.doe
  type: PIPELINE_TYPE_TRAIN
  manifest:
    type: PIPELINE_MANIFEST_TYPE_UNIFLOW
    filePath: workflows.bert.workflow
    triggerMap:
      daily:
        cronSchedule:
          cron: 0 8 * * *
        batchPolicy:
          batchSize: 1
          wait: 60s
        parametersMap:
          cola:
            kwArgs:
              tokenizer_max_length: 128
        maxConcurrency: 1
  commit:
    gitRef: master
    branch: master
  notifications:
  - notificationType: NOTIFICATION_TYPE_EMAIL
    eventTypes:
    - EVENT_TYPE_PIPELINE_RUN_STATE_FAILED
    resourceType: RESOURCE_TYPE_PIPELINE_RUN
    emails:
    - ml-team@example.com
"""

_FULL_WORKFLOW = (
    _IMPORTS
    + """
@uniflow.workflow(
    name="bert-cola",
    namespace="ma-examples",
    owner="jane.doe",
    description="Fine-tunes BERT on CoLA.",
    type="TRAIN",
    image="ghcr.io/michelangelo-ai/examples:main",
    labels={"team": "ml"},
    annotations={"example.com/cost-center": "1234"},
    triggers={
        "daily": {
            "cronSchedule": {"cron": "0 8 * * *"},
            "batchPolicy": {"batchSize": 1, "wait": "60s"},
            "parametersMap": {"cola": {"kwArgs": {"tokenizer_max_length": 128}}},
            "maxConcurrency": 1,
        }
    },
    notifications=[
        {
            "notificationType": "NOTIFICATION_TYPE_EMAIL",
            "eventTypes": ["EVENT_TYPE_PIPELINE_RUN_STATE_FAILED"],
            "resourceType": "RESOURCE_TYPE_PIPELINE_RUN",
            "emails": ["ml-team@example.com"],
        }
    ],
)
def bert(marker="m"):
    return marker
"""
)

_OSS_MODULE = "pipelines.california_housing_xgb.california_housing_xgb"
_OSS_SOURCE = "pipelines/california_housing_xgb/california_housing_xgb.py"


def _decorated(**kwargs):
    args = ", ".join(f"{k}={v!r}" for k, v in kwargs.items())
    return (
        _IMPORTS
        + f"""
@uniflow.workflow({args})
def demo(marker="m"):
    return marker
"""
    )


def _yaml(**overrides):
    doc = yaml.safe_load(
        _CR_YAML.format(name="from-yaml", description="From YAML.", wf="wf")
    )
    doc["metadata"].setdefault("labels", {})["team"] = "ml"
    for path, value in overrides.items():
        node = doc
        *parents, leaf = path.split(".")
        for key in parents:
            node = node[key]
        node[leaf] = value
    return yaml.safe_dump(doc, sort_keys=False)


class FindWorkflowFunctionTest(unittest.TestCase):
    """Tests for find_workflow_function."""

    def test_single_workflow(self):
        """The only workflow defined in the module is chosen."""
        source = _PLAIN_WORKFLOW + "\nfrom os.path import join\n"
        with pipeline_root({"pkg/wf.py": source}):
            module = importlib.import_module("pkg.wf")
            self.assertIs(module.demo, find_workflow_function(module))

    def test_ignores_workflows_imported_from_elsewhere(self):
        """Workflows imported from another module are not candidates."""
        files = {
            "pkg/other.py": _PLAIN_WORKFLOW.replace("def demo", "def other"),
            "pkg/wf.py": _PLAIN_WORKFLOW + "\nfrom pkg.other import other\n",
        }
        with pipeline_root(files):
            module = importlib.import_module("pkg.wf")
            self.assertIs(module.demo, find_workflow_function(module))

    def test_explicit_function(self):
        """An explicit function name is honoured and must be a workflow."""
        source = _PLAIN_WORKFLOW + "\ndef helper():\n    pass\n"
        with pipeline_root({"pkg/wf.py": source}):
            module = importlib.import_module("pkg.wf")
            self.assertIs(module.demo, find_workflow_function(module, "demo"))
            with self.assertRaisesRegex(PipelineMetadataError, "not found"):
                find_workflow_function(module, "missing")
            with self.assertRaisesRegex(PipelineMetadataError, "not a @uniflow"):
                find_workflow_function(module, "helper")

    def test_ctx_run_disambiguates(self):
        """With several workflows, the ctx.run target is chosen."""
        source = (
            _PLAIN_WORKFLOW
            + _PLAIN_WORKFLOW.replace("def demo", "def second")
            + "\nif __name__ == '__main__':\n"
            + "    ctx = uniflow.create_context()\n"
            + "    ctx.run(second)\n"
        )
        with pipeline_root({"pkg/wf.py": source}):
            module = importlib.import_module("pkg.wf")
            self.assertIs(module.second, find_workflow_function(module))

    def test_ambiguous_and_missing(self):
        """No or several undisambiguated workflows are errors."""
        files = {
            "pkg/two.py": _PLAIN_WORKFLOW
            + _PLAIN_WORKFLOW.replace("def demo", "def second"),
            "pkg/none.py": "x = 1\n",
        }
        with pipeline_root(files):
            with self.assertRaisesRegex(
                PipelineMetadataError, "multiple workflow functions .*demo, second"
            ):
                find_workflow_function(importlib.import_module("pkg.two"))
            with self.assertRaisesRegex(PipelineMetadataError, "no @uniflow"):
                find_workflow_function(importlib.import_module("pkg.none"))

    def test_ctx_run_targets(self):
        """Only ctx.run(<name>, ...) calls are reported, in order."""
        source = "ctx.run(a, x=1)\nother.run(b)\nctx.run('c')\nctx.run(d)\n"
        self.assertEqual(["a", "d"], ctx_run_targets(source))
        self.assertEqual([], ctx_run_targets("def ("))
        self.assertEqual([], ctx_run_targets(None))


class ParseAndMergeTest(unittest.TestCase):
    """Tests for parse_pipeline_yaml and merge."""

    def _pipeline_yaml(self, **overrides):
        return parse_pipeline_yaml("pipeline.yaml", yaml.safe_load(_yaml(**overrides)))

    def test_parse_extracts_identity_fields(self):
        """Every identity field and the manifest location are extracted."""
        parsed = self._pipeline_yaml(**{"spec.manifest.uniflowFunction": "demo"})
        self.assertEqual(
            PipelineMetadata(
                name="from-yaml",
                namespace="ma-examples",
                owner="jane.doe",
                description="From YAML.",
                type="PIPELINE_TYPE_TRAIN",
                image="ghcr.io/michelangelo-ai/examples:main",
                git_ref="master",
                branch="master",
                labels={"team": "ml"},
            ),
            parsed.metadata,
        )
        self.assertEqual("workflows.wf.workflow", parsed.module)
        self.assertEqual("demo", parsed.function)

    def test_parse_rejects_non_mapping(self):
        """A YAML that is not a mapping is rejected."""
        with self.assertRaisesRegex(PipelineMetadataError, "expected a Pipeline CR"):
            parse_pipeline_yaml("x.yaml", ["not", "a", "mapping"])

    def test_merge_single_source(self):
        """A single source is returned as is, without warnings."""
        inline = PipelineMetadata(name="a")
        self.assertEqual((inline, []), merge(inline, None))
        parsed = self._pipeline_yaml()
        self.assertEqual((parsed.metadata, []), merge(None, parsed))

    def test_merge_inline_wins_with_warnings(self):
        """Inline values win; conflicts and what is left to migrate are reported."""
        merged, warnings = merge(
            PipelineMetadata(owner="john.doe", type="TRAIN"), self._pipeline_yaml()
        )
        self.assertEqual("john.doe", merged.owner)
        self.assertEqual("from-yaml", merged.name)  # YAML fills the gaps
        self.assertEqual(
            [
                '@uniflow.workflow(owner="john.doe") overrides '
                'spec.owner.name="jane.doe" from pipeline.yaml',
                "pipeline.yaml still supplies name, namespace, labels (team), "
                "image, description; move them into @uniflow.workflow(...)",
            ],
            warnings,
        )

    def test_merge_structured_fields(self):
        """Labels, annotations and triggers merge per key; notifications replace."""
        parsed = parse_pipeline_yaml("pipeline.yaml", yaml.safe_load(_FULL_YAML))
        self.assertEqual(
            {"example.com/cost-center": "1234"}, parsed.metadata.annotations
        )
        inline = PipelineMetadata(
            labels={"team": "search", "tier": "gold"},
            triggers={"daily": {"cronSchedule": {"cron": "0 9 * * *"}}},
            notifications=[{"emails": ["oncall@example.com"]}],
        )
        merged, warnings = merge(inline, parsed)
        self.assertEqual({"team": "search", "tier": "gold"}, merged.labels)
        self.assertEqual(
            {"cronSchedule": {"cron": "0 9 * * *"}}, merged.triggers["daily"]
        )
        self.assertEqual([{"emails": ["oncall@example.com"]}], merged.notifications)
        self.assertEqual(
            [
                '@uniflow.workflow(labels["team"]="search") overrides '
                'metadata.labels.team="ml" from pipeline.yaml',
                '@uniflow.workflow(triggers["daily"]) overrides '
                "spec.manifest.triggerMap.daily from pipeline.yaml",
                "@uniflow.workflow(notifications) overrides spec.notifications "
                "from pipeline.yaml",
            ],
            [w for w in warnings if "overrides" in w],
        )
        self.assertIn("annotations (example.com/cost-center)", warnings[-1])

    def test_merge_reports_unsupported_yaml_keys(self):
        """Keys no inline field can set keep the YAML alive; registration's don't."""
        raw = yaml.safe_load(_OSS_YAML)
        raw["spec"]["manifest"]["uniflowTar"] = "s3://bucket/x.tar.gz"
        raw["spec"]["manifest"]["paramsMap"] = {"p": {"kwArgs": {"a": 1}}}
        inline = PipelineMetadata(
            name="california-housing-xgb",
            namespace="ma-examples",
            type="TRAIN",
            image="ghcr.io/michelangelo-ai/examples:main",
        )
        self.assertEqual(
            [
                "pipeline.yaml also sets spec.manifest.paramsMap.p.kwArgs.a, which "
                "@uniflow.workflow(...) cannot express; keep pipeline.yaml for them"
            ],
            merge(inline, parse_pipeline_yaml("pipeline.yaml", raw))[1],
        )

    def test_merge_equal_values_do_not_warn(self):
        """Equal values (after type normalization) are not conflicts."""
        _, warnings = merge(
            PipelineMetadata(name="from-yaml", type="train"), self._pipeline_yaml()
        )
        self.assertEqual([], [w for w in warnings if "overrides" in w])

    def test_merge_fully_migrated_yaml_is_redundant(self):
        """A YAML the decorator fully covers is reported as deletable."""
        parsed = parse_pipeline_yaml("pipeline.yaml", yaml.safe_load(_OSS_YAML))
        inline = PipelineMetadata(
            name="california-housing-xgb",
            namespace="ma-examples",
            type="TRAIN",
            image="ghcr.io/michelangelo-ai/examples:main",
        )
        self.assertEqual(
            ["pipeline.yaml is redundant with @uniflow.workflow(...); delete it"],
            merge(inline, parsed)[1],
        )


class DefaultsAndRequiredTest(unittest.TestCase):
    """Tests for apply_defaults and check_required."""

    def test_defaults_fill_only_unset_fields(self):
        """Type defaults to TRAIN and commit to git, without overriding."""
        git = FakeGit("abc", "main")
        self.assertEqual(
            PipelineMetadata(
                name="x", type="PIPELINE_TYPE_TRAIN", git_ref="abc", branch="main"
            ),
            apply_defaults(PipelineMetadata(name="x"), git, "/root"),
        )
        explicit = PipelineMetadata(type="PIPELINE_TYPE_EVAL", git_ref="r", branch="b")
        self.assertEqual(explicit, apply_defaults(explicit, git, "/root"))
        self.assertEqual(1, git.calls)  # git is not consulted when not needed

    def test_required_fields(self):
        """Name and namespace are required by default; callers may add more."""
        check_required(PipelineMetadata(name="a", namespace="b"), "wf")
        with self.assertRaisesRegex(
            PipelineMetadataError,
            r"wf: missing required pipeline metadata: namespace; "
            r"add namespace=\.\.\. to @uniflow.workflow\(\.\.\.\)",
        ):
            check_required(PipelineMetadata(name="a"), "wf")
        with self.assertRaisesRegex(
            PipelineMetadataError,
            r"missing required pipeline metadata: namespace, owner; "
            r"add namespace=\.\.\., owner=\.\.\.",
        ):
            check_required(
                PipelineMetadata(name="a"), "wf", ("name", "namespace", "owner")
            )


class BuildPipelineCrTest(unittest.TestCase):
    """Tests for build_pipeline_cr."""

    def test_canonical_key_order(self):
        """Keys follow the conventional pipeline YAML order."""
        cr = build_pipeline_cr(
            PipelineMetadata(
                name="n",
                namespace="ma-examples",
                owner="jane.doe",
                description="d",
                type="PIPELINE_TYPE_TRAIN",
                image="ghcr.io/michelangelo-ai/examples:main",
                git_ref="r",
                branch="b",
            ),
            "workflows.wf.workflow",
        )
        expected = yaml.safe_load(_CR_YAML.format(name="n", description="d", wf="wf"))
        expected["spec"]["commit"] = {"gitRef": "r", "branch": "b"}
        self.assertEqual(
            yaml.safe_dump(expected, sort_keys=False),
            yaml.safe_dump(cr, sort_keys=False),
        )

    def test_preserves_other_yaml_keys(self):
        """Keys the metadata doesn't model are kept from the base YAML."""
        base = yaml.safe_load(_yaml(**{"spec.manifest.path": "old.module"}))
        del base["spec"]["manifest"]["filePath"]
        cr = build_pipeline_cr(PipelineMetadata(owner="john.doe"), "new.module", base)
        self.assertEqual({"team": "ml"}, cr["metadata"]["labels"])
        self.assertEqual("john.doe", cr["spec"]["owner"]["name"])
        self.assertEqual(
            {"type": "PIPELINE_MANIFEST_TYPE_UNIFLOW", "filePath": "new.module"},
            cr["spec"]["manifest"],
        )


class ResolveTest(unittest.TestCase):
    """End-to-end tests for resolve."""

    def test_parity_with_equivalent_yaml(self):
        """The decorator renders the same CR as the YAML it replaces."""
        params = {"name": "demo-pipeline", "description": "Trains.", "wf": "demo"}
        files = {"workflows/demo/workflow.py": _CR_WORKFLOW.format(**params)}
        with pipeline_root(files) as root:
            resolved = resolve(
                module="workflows.demo.workflow", root=root, git=FakeGit()
            )
        self.assertEqual(
            yaml.safe_dump(yaml.safe_load(_CR_YAML.format(**params)), sort_keys=False),
            yaml.safe_dump(resolved.pipeline_cr, sort_keys=False),
        )
        self.assertEqual("decorator", resolved.source)
        self.assertEqual("demo", resolved.function)
        self.assertEqual("workflows/demo/workflow.py", resolved.source_path)
        self.assertEqual([], resolved.warnings)

    def test_parity_with_full_yaml(self):
        """Every user-authored YAML field renders identically from the decorator."""
        files = {"workflows/bert/workflow.py": _FULL_WORKFLOW}
        with pipeline_root(files) as root:
            resolved = resolve(
                module="workflows.bert.workflow", root=root, git=FakeGit()
            )
        self.assertEqual(
            yaml.safe_dump(yaml.safe_load(_FULL_YAML), sort_keys=False),
            yaml.safe_dump(resolved.pipeline_cr, sort_keys=False),
        )
        data = resolved.to_dict()
        self.assertEqual({"team": "ml"}, data["labels"])
        self.assertEqual(1, data["triggers"]["daily"]["maxConcurrency"])
        self.assertEqual([], resolved.warnings)

    def test_full_yaml_round_trips_through_merge(self):
        """Next to an equivalent decorator, the full YAML is reported redundant."""
        files = {
            "workflows/bert/workflow.py": _FULL_WORKFLOW,
            "workflows/bert/pipeline.yaml": _FULL_YAML,
        }
        with pipeline_root(files) as root:
            resolved = resolve(
                module="workflows.bert.workflow", root=root, git=FakeGit()
            )
        self.assertEqual("decorator+yaml", resolved.source)
        self.assertEqual(yaml.safe_load(_FULL_YAML), resolved.pipeline_cr)
        self.assertEqual(
            [
                "workflows/bert/pipeline.yaml is redundant with "
                "@uniflow.workflow(...); delete it"
            ],
            [w.replace(root + os.sep, "") for w in resolved.warnings],
        )

    def test_parity_with_oss_example_yaml(self):
        """An OSS-style pipeline.yaml and its decorator equivalent agree."""
        yaml_files = {_OSS_SOURCE: _PLAIN_WORKFLOW, "pipeline.yaml": _OSS_YAML}
        with pipeline_root(yaml_files) as root:
            from_yaml = resolve(
                config_file=os.path.join(root, "pipeline.yaml"),
                root=root,
                git=FakeGit(),
            )
        with pipeline_root({_OSS_SOURCE: _OSS_WORKFLOW}) as root:
            inline = resolve(module=_OSS_MODULE, root=root, git=FakeGit())

        self.assertEqual("yaml", from_yaml.source)
        self.assertEqual(yaml.safe_load(_OSS_YAML), from_yaml.pipeline_cr)
        self.assertIsNone(from_yaml.metadata.owner)  # owner is not required

        expected = yaml.safe_load(_OSS_YAML)
        expected["spec"]["manifest"]["type"] = "PIPELINE_MANIFEST_TYPE_UNIFLOW"
        expected["spec"]["commit"] = {"gitRef": "master", "branch": "master"}
        self.assertEqual(expected, inline.pipeline_cr)
        self.assertEqual("train_workflow", inline.function)
        self.assertIsNone(inline.metadata.owner)

    def test_extra_required_fields(self):
        """Callers can require more than name and namespace."""
        files = {_OSS_SOURCE: _PLAIN_WORKFLOW, "pipeline.yaml": _OSS_YAML}
        with (
            pipeline_root(files) as root,
            self.assertRaisesRegex(
                PipelineMetadataError,
                r"demo \(.*\): missing required pipeline metadata: owner;",
            ),
        ):
            resolve(
                config_file=os.path.join(root, "pipeline.yaml"),
                root=root,
                required=("name", "namespace", "owner"),
                git=FakeGit(),
            )

    def test_to_dict_contract(self):
        """The JSON contract carries schema_version 1 and the flattened fields."""
        source = _decorated(name="demo", namespace="ns", owner="o")
        with pipeline_root({"pkg/wf.py": source}) as root:
            result = resolve(module="pkg.wf", root=root, git=FakeGit("abc", "main"))
        data = result.to_dict()
        self.assertEqual(1, data["schema_version"])
        self.assertEqual({"git_ref": "abc", "branch": "main"}, data["commit"])
        self.assertEqual("PIPELINE_TYPE_TRAIN", data["type"])
        self.assertEqual(
            {
                "name",
                "namespace",
                "description",
                "owner",
                "type",
                "image",
                "commit",
                "labels",
                "annotations",
                "triggers",
                "notifications",
                "module",
                "function",
                "source_path",
                "source",
                "pipeline_cr",
                "warnings",
                "schema_version",
            },
            set(data),
        )

    def test_yaml_only_config_mode_renders_yaml_exactly(self):
        """A pipeline YAML without inline metadata is rendered verbatim."""
        files = {
            "workflows/wf/workflow.py": _PLAIN_WORKFLOW,
            "workflows/wf/pipeline.yaml": _yaml(),
        }
        with pipeline_root(files) as root:
            git = FakeGit()
            config = os.path.join(root, "workflows/wf/pipeline.yaml")
            resolved = resolve(config_file=config, root=root, git=git)
        self.assertEqual(yaml.safe_load(_yaml()), resolved.pipeline_cr)
        self.assertEqual("yaml", resolved.source)
        self.assertEqual("from-yaml", resolved.metadata.name)
        self.assertEqual(0, git.calls)

    def test_yaml_only_module_mode_uses_sibling_yaml(self):
        """Module mode without inline metadata falls back to the sibling YAML."""
        files = {
            "workflows/wf/workflow.py": _PLAIN_WORKFLOW,
            "workflows/wf/pipeline.yaml": _yaml(),
        }
        with pipeline_root(files) as root:
            resolved = resolve(module="workflows.wf.workflow", root=root, git=FakeGit())
        self.assertEqual("yaml", resolved.source)
        self.assertEqual(yaml.safe_load(_yaml()), resolved.pipeline_cr)

    def test_sibling_yaml_name(self):
        """The sibling YAML name is configurable, and the lookup can be disabled."""
        files = {
            "a/workflow.py": _PLAIN_WORKFLOW,
            "a/custom-pipeline.yaml": _yaml(),
            "b/workflow.py": _decorated(name="demo", namespace="ns"),
            "b/pipeline.yaml": _yaml(),
        }
        with pipeline_root(files) as root:
            with self.assertRaisesRegex(PipelineMetadataError, "no pipeline metadata"):
                resolve(module="a.workflow", root=root, git=FakeGit())
            custom = resolve(
                module="a.workflow",
                root=root,
                sibling_yaml="custom-pipeline.yaml",
                git=FakeGit(),
            )
            disabled = resolve(
                module="b.workflow", root=root, sibling_yaml=None, git=FakeGit()
            )
        self.assertEqual("yaml", custom.source)
        self.assertEqual("from-yaml", custom.metadata.name)
        self.assertEqual("decorator", disabled.source)
        self.assertEqual([], disabled.warnings)

    def test_decorator_and_sibling_yaml(self):
        """The decorator wins over a sibling YAML, which fills gaps and extra keys."""
        files = {
            "workflows/wf/workflow.py": _decorated(owner="john.doe"),
            "workflows/wf/pipeline.yaml": _yaml(),
        }
        with pipeline_root(files) as root:
            resolved = resolve(
                module="workflows.wf.workflow", root=root, git=FakeGit("x", "y")
            )
        self.assertEqual("decorator+yaml", resolved.source)
        self.assertEqual("john.doe", resolved.pipeline_cr["spec"]["owner"]["name"])
        self.assertEqual("from-yaml", resolved.pipeline_cr["metadata"]["name"])
        self.assertEqual({"team": "ml"}, resolved.pipeline_cr["metadata"]["labels"])
        # git_ref/branch come from the YAML (it sets them), not from git.
        self.assertEqual(
            {"gitRef": "master", "branch": "master"},
            resolved.pipeline_cr["spec"]["commit"],
        )
        self.assertEqual(2, len(resolved.warnings))
        self.assertIn('owner="john.doe"', resolved.warnings[0])

    def test_config_mode_merges_decorator(self):
        """Config mode also merges metadata declared on the workflow."""
        files = {
            "workflows/wf/workflow.py": _decorated(owner="john.doe"),
            "cfg/pipeline.yaml": _yaml(),
        }
        with pipeline_root(files) as root:
            resolved = resolve(
                config_file=os.path.join(root, "cfg/pipeline.yaml"),
                root=root,
                git=FakeGit(),
            )
        self.assertEqual("decorator+yaml", resolved.source)
        self.assertEqual("john.doe", resolved.metadata.owner)

    def test_missing_metadata_errors(self):
        """No metadata at all, or missing required fields, fail actionably."""
        files = {"a/wf.py": _PLAIN_WORKFLOW, "b/wf.py": _decorated(name="only-name")}
        with pipeline_root(files) as root:
            with self.assertRaisesRegex(
                PipelineMetadataError, r"no pipeline metadata; add name=\.\.\."
            ):
                resolve(module="a.wf", root=root, git=FakeGit())
            with self.assertRaisesRegex(
                PipelineMetadataError,
                r"demo \(.*wf.py:\d+\): missing required .*: namespace;",
            ):
                resolve(module="b.wf", root=root, git=FakeGit())

    def test_import_failures(self):
        """Import errors and invalid decorator values surface distinctly."""
        files = {
            "a/wf.py": "import does_not_exist\n",
            "b/wf.py": _decorated(type="TRAINING"),
        }
        with pipeline_root(files) as root:
            with self.assertRaisesRegex(WorkflowImportError, "ModuleNotFoundError"):
                resolve(module="a.wf", root=root, git=FakeGit())
            with self.assertRaisesRegex(PipelineMetadataError, "did you mean"):
                resolve(module="b.wf", root=root, git=FakeGit())

    def test_config_file_requires_file_path(self):
        """A pipeline YAML must point at the workflow module."""
        files = {"pipeline.yaml": "apiVersion: v\nkind: Pipeline\nspec: {}\n"}
        with pipeline_root(files) as root:
            config = os.path.join(root, "pipeline.yaml")
            with self.assertRaisesRegex(PipelineMetadataError, "filePath is required"):
                resolve(config_file=config, root=root)

    def test_exactly_one_source(self):
        """Exactly one of module or config_file is required."""
        with self.assertRaises(ValueError):
            resolve()
        with self.assertRaises(ValueError):
            resolve(module="a", config_file="b")


class EdgeCaseTest(unittest.TestCase):
    """Tests for error paths and less common inputs."""

    def test_parse_ignores_malformed_structured_fields(self):
        """Mapping and list fields of the wrong shape are left out, not crashed on."""
        raw = yaml.safe_load(_OSS_YAML)
        raw["metadata"]["labels"] = ["team"]
        raw["spec"]["notifications"] = {"emails": ["a@example.com"]}
        parsed = parse_pipeline_yaml("pipeline.yaml", raw)
        self.assertIsNone(parsed.metadata.labels)
        self.assertIsNone(parsed.metadata.notifications)
        self.assertEqual("california-housing-xgb", parsed.metadata.name)

    def test_unreadable_yaml(self):
        """A missing or malformed YAML is reported as invalid metadata."""
        with pipeline_root({"bad.yaml": "metadata: [unclosed\n"}) as root:
            for name in ("missing.yaml", "bad.yaml"):
                with (
                    self.subTest(name=name),
                    self.assertRaisesRegex(PipelineMetadataError, "cannot read"),
                ):
                    load_pipeline_yaml(os.path.join(root, name), LocalFileSystem())

    def test_no_sibling_yaml_without_a_module_file(self):
        """Modules without a file (e.g. built-ins) have no sibling YAML."""
        self.assertIsNone(load_sibling_yaml(None, LocalFileSystem()))

    def test_defaults_fill_only_the_missing_commit_field(self):
        """A set git_ref or branch is kept; only the other comes from git."""
        git = FakeGit("abc", "main")
        only_ref = apply_defaults(PipelineMetadata(git_ref="r"), git, "/root")
        self.assertEqual(("r", "main"), (only_ref.git_ref, only_ref.branch))
        only_branch = apply_defaults(PipelineMetadata(branch="b"), git, "/root")
        self.assertEqual(("abc", "b"), (only_branch.git_ref, only_branch.branch))

    def test_resolve_puts_the_root_on_sys_path(self):
        """The pipeline root is added to sys.path when it isn't there yet."""
        source = _decorated(name="demo", namespace="ns")
        with pipeline_root({"pkg/wf.py": source}) as root:
            sys.path.remove(root)
            resolved = resolve(module="pkg.wf", root=root, git=FakeGit())
            self.assertIn(root, sys.path)
        self.assertEqual("demo", resolved.metadata.name)

    def test_module_without_source(self):
        """A module whose source can't be read yields no ctx.run targets."""
        self.assertIsNone(_module_source(types.ModuleType("no_source")))

    def test_merge_with_an_invalid_yaml_type_warns(self):
        """An invalid YAML type can't be equal to the inline one, so it conflicts."""
        parsed = parse_pipeline_yaml(
            "pipeline.yaml", yaml.safe_load(_yaml(**{"spec.type": "BOGUS"}))
        )
        merged, warnings = merge(PipelineMetadata(type="TRAIN"), parsed)
        self.assertEqual("TRAIN", merged.type)
        self.assertIn('@uniflow.workflow(type="TRAIN") overrides', warnings[0])
