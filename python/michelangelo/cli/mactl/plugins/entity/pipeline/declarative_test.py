"""Unit tests for declarative pipeline packing and explain-plan rendering."""

import datetime
import json
from pathlib import Path
from unittest.mock import Mock, patch

import pytest
import yaml
from google.protobuf.json_format import ParseDict, ParseError

from michelangelo.cli.mactl.crd import yaml_to_dict
from michelangelo.cli.mactl.plugins.entity.pipeline.create import (
    convert_crd_metadata_pipeline,
)
from michelangelo.cli.mactl.plugins.entity.pipeline.declarative import (
    EXPLAIN_PLAN_ANNOTATION,
    DeclarativeManifestError,
    explain_plan_from,
    is_declarative,
    normalize_workflow,
    pack_declarative_manifest,
    render_explain_plan,
)
from michelangelo.gen.api.v2 import declarative_pb2, pipeline_pb2, pipeline_svc_pb2

_MAX = 2**53 - 1


def _parse(content: dict) -> declarative_pb2.DeclarativeWorkflow:
    """Parse a normalized workflow dict the way the request builder does."""
    return ParseDict(content, declarative_pb2.DeclarativeWorkflow())


def test_short_outcome_names_map_to_full_names():
    """Short and lowercase outcome names map in both field spellings."""
    content = {
        "tasks": {
            "a": {"taskFunction": "core/ray"},
            "b": {
                "taskFunction": "core/ray",
                "dependsOn": [{"task": "a", "when": "FAILED"}],
            },
            "c": {
                "task_function": "core/ray",
                "depends_on": [{"task": "a", "when": "completed"}],
            },
            "d": {
                "taskFunction": "core/ray",
                "dependsOn": [{"task": "a", "when": "OUTCOME_SUCCEEDED"}],
            },
        }
    }

    wf = _parse(normalize_workflow(content))

    assert (
        wf.tasks["b"].depends_on[0].when
        == declarative_pb2.TaskDependency.OUTCOME_FAILED
    )
    assert (
        wf.tasks["c"].depends_on[0].when
        == declarative_pb2.TaskDependency.OUTCOME_COMPLETED
    )
    assert (
        wf.tasks["d"].depends_on[0].when
        == declarative_pb2.TaskDependency.OUTCOME_SUCCEEDED
    )


def test_short_parameter_type_maps_to_full_name():
    """A short parameter type name maps to its full enum name."""
    content = {"parameters": {"epochs": {"type": "integer", "required": True}}}

    assert (
        normalize_workflow(content)["parameters"]["epochs"]["type"]
        == "PARAMETER_TYPE_INTEGER"
    )


def test_unknown_enum_name_lists_valid_names():
    """An unknown enum name fails with the path and the valid names."""
    content = {"tasks": {"b": {"dependsOn": [{"task": "a", "when": "ALWAYS"}]}}}

    with pytest.raises(DeclarativeManifestError) as err:
        normalize_workflow(content)

    assert str(err.value) == (
        "content.tasks.b.dependsOn[0].when: unknown value 'ALWAYS'; "
        "use one of UNSPECIFIED, SUCCEEDED, FAILED, COMPLETED"
    )


def test_enums_inside_struct_fields_stay_untouched():
    """A config value that looks like an enum name is user data, not an enum."""
    content = {"tasks": {"a": {"config": {"when": "FAILED", "type": "integer"}}}}

    assert normalize_workflow(content)["tasks"]["a"]["config"] == {
        "when": "FAILED",
        "type": "integer",
    }


def test_task_name_in_depends_on_fails_with_expected_shape():
    """A task name in place of a dependency mapping names the shape to use."""
    content = {"tasks": {"load": {"dependsOn": ["transform"]}}}

    with pytest.raises(DeclarativeManifestError) as err:
        normalize_workflow(content)

    assert str(err.value) == (
        "content.tasks.load.dependsOn[0]: expected a mapping of TaskDependency"
        " fields, got str 'transform'"
    )


def test_both_spellings_of_one_field_fail():
    """A field set as both task_function and taskFunction is an error."""
    content = {"tasks": {"a": {"task_function": "core/x", "taskFunction": "core/y"}}}

    with pytest.raises(DeclarativeManifestError) as err:
        normalize_workflow(content)

    assert str(err.value) == (
        "content.tasks.a: set taskFunction or task_function, not both"
    )


@pytest.mark.parametrize(
    ("default", "expected"),
    [
        ("demo", "PARAMETER_TYPE_STRING"),
        (3, "PARAMETER_TYPE_INTEGER"),
        (1.5, "PARAMETER_TYPE_NUMBER"),
        (True, "PARAMETER_TYPE_BOOLEAN"),
        (False, "PARAMETER_TYPE_BOOLEAN"),
        (["a", 1], "PARAMETER_TYPE_LIST"),
        ({"depth": 6}, "PARAMETER_TYPE_DICT"),
    ],
)
def test_parameter_type_inferred_from_default(default, expected):
    """A missing parameter type is inferred from the YAML default."""
    content = {"parameters": {"p": {"default": default}}}

    normalized = normalize_workflow(content)

    assert normalized["parameters"]["p"]["type"] == expected
    assert _parse(normalized).parameters[
        "p"
    ].type == declarative_pb2.ParameterType.Value(expected)


def test_declared_parameter_type_is_kept():
    """A declared type wins over the shape of the default."""
    content = {"parameters": {"p": {"default": 3, "type": "number"}}}

    assert (
        normalize_workflow(content)["parameters"]["p"]["type"]
        == "PARAMETER_TYPE_NUMBER"
    )


@pytest.mark.parametrize(
    "spec",
    [{"required": True}, {"description": "no default"}, {"default": None}],
)
def test_parameter_without_type_or_default_fails(spec):
    """A parameter with no type and no usable default is an error."""
    with pytest.raises(DeclarativeManifestError) as err:
        normalize_workflow({"parameters": {"seed": spec}})

    assert str(err.value) == (
        "content.parameters.seed: set type; "
        "it cannot be inferred without a non-null default"
    )


@pytest.mark.parametrize("value", [_MAX, -_MAX, True, 1.0e300])
def test_values_within_safe_integer_range_pass(value):
    """Integers within ±(2^53-1), bools, and floats pass the range check."""
    content = {"tasks": {"a": {"config": {"seed": value}}}}

    assert normalize_workflow(content)["tasks"]["a"]["config"]["seed"] == value


@pytest.mark.parametrize("value", [_MAX + 1, -(_MAX + 1)])
def test_integer_outside_safe_range_fails(value):
    """An integer outside ±(2^53-1) fails with its path."""
    content = {"tasks": {"a": {"config": {"ids": [1, {"big": value}]}}}}

    with pytest.raises(DeclarativeManifestError) as err:
        normalize_workflow(content)

    assert str(err.value) == (
        f"content.tasks.a.config.ids[1].big: integer {value} is outside ±(2^53-1); "
        "numbers travel as IEEE doubles, so declare it as a string"
    )


def test_integer_outside_safe_range_fails_in_parameter_default():
    """The range check covers parameter defaults too."""
    with pytest.raises(
        DeclarativeManifestError, match=r"content\.parameters\.id\.default"
    ):
        normalize_workflow({"parameters": {"id": {"default": 2**63}}})


def test_non_mapping_parameters_are_left_for_the_parser():
    """A malformed parameters value reaches ParseDict, which reports it."""
    normalized = normalize_workflow({"parameters": ["seed"]})

    with pytest.raises(ParseError, match="parameters"):
        _parse(normalized)


def test_normalize_does_not_mutate_input():
    """The author dict stays unchanged."""
    content = {"parameters": {"p": {"default": 1}}}

    normalize_workflow(content)

    assert content == {"parameters": {"p": {"default": 1}}}


def test_shared_alias_keeps_each_occurrence_separate(tmp_path):
    """Type inference on an anchor does not change a value that aliases it."""
    yaml_dict = yaml.safe_load(
        """
metadata: {name: p, namespace: proj}
spec:
  manifest:
    type: DECLARATIVE
    content:
      parameters:
        count: &count {default: 3}
        payload: {type: dict, default: *count}
"""
    )

    manifest = _convert(yaml_dict, tmp_path).pipeline.spec.manifest

    wf = declarative_pb2.DeclarativeWorkflow()
    assert manifest.content.Unpack(wf)
    assert wf.parameters["count"].type == declarative_pb2.PARAMETER_TYPE_INTEGER
    assert list(wf.parameters["payload"].default.struct_value.fields) == ["default"]


def test_self_alias_fails_at_the_depth_cap(tmp_path):
    """A self-alias fails with a manifest error, not a RecursionError."""
    yaml_dict = yaml.safe_load(
        """
metadata: {name: p, namespace: proj}
spec:
  manifest:
    type: DECLARATIVE
    content:
      parameters:
        p: &p {default: [*p]}
"""
    )

    with pytest.raises(DeclarativeManifestError, match="nested deeper than 64 levels"):
        _convert(yaml_dict, tmp_path)


def test_alias_expansion_fails_at_the_node_budget():
    """Nested aliases that expand past the node budget fail fast."""
    lines = ["l0: &l0 [1, 1, 1, 1, 1, 1, 1, 1, 1, 1]"]
    for level in range(1, 10):
        refs = ", ".join([f"*l{level - 1}"] * 10)
        lines.append(f"l{level}: &l{level} [{refs}]")
    content = {"tasks": {"a": {"config": yaml.safe_load("\n".join(lines))}}}

    with pytest.raises(DeclarativeManifestError) as err:
        normalize_workflow(content)

    assert str(err.value) == (
        "spec.manifest.content has more than 100000 values"
        " after YAML aliases are expanded"
    )


@pytest.mark.parametrize(
    ("config", "message"),
    [
        (
            "class_weights: {0: 1.0}",
            "content.tasks.a.config.class_weights: key 0 is a YAML int; quote it",
        ),
        ("on: 1", "content.tasks.a.config: key True is a YAML bool; quote it"),
        ("x: .nan", "content.tasks.a.config.x: nan is not a finite number"),
        ("x: [-.inf]", "content.tasks.a.config.x[0]: -inf is not a finite number"),
        ("x: 1.0e+400", "content.tasks.a.config.x: inf is not a finite number"),
    ],
)
def test_scalars_that_json_cannot_hold_fail(config, message):
    """Non-string keys and non-finite floats fail with their path."""
    content = {"tasks": {"a": {"config": yaml.safe_load(config)}}}

    with pytest.raises(DeclarativeManifestError) as err:
        normalize_workflow(content)

    assert str(err.value) == message


def test_non_string_parameter_name_fails():
    """A parameter name must be a string."""
    with pytest.raises(DeclarativeManifestError) as err:
        normalize_workflow({"parameters": {1: {"default": 1}}})

    assert str(err.value) == "content.parameters: key 1 is a YAML int; quote it"


_DECLARATIVE_YAML = {
    "apiVersion": "michelangelo.api/v2",
    "kind": "Pipeline",
    "metadata": {"name": "diamond", "namespace": "proj"},
    "spec": {
        "type": "PIPELINE_TYPE_TRAIN",
        "manifest": {
            "type": "PIPELINE_MANIFEST_TYPE_DECLARATIVE",
            "triggerMap": {
                "nightly": {"cronSchedule": {"cron": "0 0 * * *"}, "maxConcurrency": 1}
            },
            "content": {
                "schemaVersion": "v1alpha1",
                "parameters": {"epochs": {"default": 3}},
                "tasks": {
                    "root": {
                        "taskFunction": "core/ray",
                        "config": {"n": "{{ params.epochs }}"},
                    },
                    "cleanup": {
                        "taskFunction": "core/ray",
                        "dependsOn": [{"task": "root", "when": "FAILED"}],
                    },
                },
            },
        },
    },
}


def _convert(
    yaml_dict: dict, repo_root: Path
) -> pipeline_svc_pb2.UpdatePipelineRequest:
    """Run the pipeline converter with git stubbed and parse the request."""
    repo = Mock()
    repo.git.rev_parse.return_value = str(repo_root)
    repo.active_branch.name = "main"
    repo.head.commit.hexsha = "abc123"
    module = "michelangelo.cli.mactl.plugins.entity.pipeline.create"
    with (
        patch(f"{module}.Repo", return_value=repo),
        patch(f"{module}.get_user_name", return_value="alice"),
        patch(f"{module}.handle_workflow_inputs_retrieval") as registration,
    ):
        converted = convert_crd_metadata_pipeline(
            yaml_dict, pipeline_svc_pb2.UpdatePipelineRequest, repo_root / "p.yaml"
        )
    registration.assert_not_called()
    request = pipeline_svc_pb2.UpdatePipelineRequest()
    ParseDict({"pipeline": converted}, request)
    return request


def test_declarative_yaml_packs_content_as_any(tmp_path):
    """The converter packs Any<DeclarativeWorkflow> and sets no server-owned field."""
    manifest = _convert(_DECLARATIVE_YAML, tmp_path).pipeline.spec.manifest

    assert (
        manifest.type
        == pipeline_pb2.PipelineManifest.PIPELINE_MANIFEST_TYPE_DECLARATIVE
    )
    assert manifest.content.type_url == (
        "type.googleapis.com/michelangelo.api.v2.DeclarativeWorkflow"
    )
    assert not manifest.HasField("interpreter_pin")
    assert manifest.uniflow_tar == ""
    assert manifest.uniflow_function == ""
    assert manifest.file_path == "p.yaml"
    assert list(manifest.trigger_map) == ["nightly"]

    wf = declarative_pb2.DeclarativeWorkflow()
    assert manifest.content.Unpack(wf)
    assert wf.schema_version == "v1alpha1"
    assert wf.parameters["epochs"].type == declarative_pb2.PARAMETER_TYPE_INTEGER
    assert wf.parameters["epochs"].default.number_value == 3
    assert wf.tasks["cleanup"].depends_on[0].when == (
        declarative_pb2.TaskDependency.OUTCOME_FAILED
    )
    assert wf.tasks["root"].config["n"] == "{{ params.epochs }}"


@pytest.mark.parametrize("field", ["uniflowTar", "uniflow_tar"])
def test_uniflow_tar_fails(tmp_path, field):
    """A uniflowTar in the YAML fails before any request."""
    yaml_dict = json.loads(json.dumps(_DECLARATIVE_YAML))
    yaml_dict["spec"]["manifest"][field] = "x"

    with pytest.raises(DeclarativeManifestError) as err:
        _convert(yaml_dict, tmp_path)

    assert str(err.value) == (
        f"spec.manifest.{field} is server-owned for declarative pipelines;"
        " remove it, because the apiserver sets it from the interpreter pin"
    )


@pytest.mark.parametrize("field", ["interpreterPin", "interpreter_pin"])
def test_pin_version_is_sent(tmp_path, field):
    """interpreterPin.version goes to the apiserver, which writes the pin."""
    yaml_dict = json.loads(json.dumps(_DECLARATIVE_YAML))
    yaml_dict["spec"]["manifest"][field] = {"version": "mock-v2"}

    manifest = _convert(yaml_dict, tmp_path).pipeline.spec.manifest

    assert manifest.interpreter_pin == pipeline_pb2.InterpreterPin(version="mock-v2")
    assert manifest.content.type_url.endswith("DeclarativeWorkflow")


@pytest.mark.parametrize(
    ("pin", "message"),
    [
        (
            {"version": "mock-v2", "artifact": "s3://x"},
            "spec.manifest.interpreterPin.artifact is server-owned; to change"
            " the pin, set only interpreterPin.version",
        ),
        (
            {"digest": "sha256:0"},
            "spec.manifest.interpreterPin.digest is server-owned; to change"
            " the pin, set only interpreterPin.version",
        ),
        (
            "mock-v2",
            "spec.manifest.interpreterPin must be a mapping with one key,"
            " version; for example interpreterPin: {version: <release>}",
        ),
        (
            {"version": ""},
            "spec.manifest.interpreterPin.version must be a release version"
            " string, got ''",
        ),
        (
            {"version": 2},
            "spec.manifest.interpreterPin.version must be a release version"
            " string, got 2",
        ),
    ],
)
def test_pin_other_than_version_fails(tmp_path, pin, message):
    """Only interpreterPin.version passes; the apiserver owns the rest."""
    yaml_dict = json.loads(json.dumps(_DECLARATIVE_YAML))
    yaml_dict["spec"]["manifest"]["interpreterPin"] = pin

    with pytest.raises(DeclarativeManifestError) as err:
        _convert(yaml_dict, tmp_path)

    assert str(err.value) == message


def test_both_pin_spellings_fail(tmp_path):
    """Both pin spellings in one manifest is an error."""
    yaml_dict = json.loads(json.dumps(_DECLARATIVE_YAML))
    yaml_dict["spec"]["manifest"]["interpreterPin"] = {"version": "mock-v1"}
    yaml_dict["spec"]["manifest"]["interpreter_pin"] = {"version": "mock-v2"}

    with pytest.raises(DeclarativeManifestError) as err:
        _convert(yaml_dict, tmp_path)

    assert str(err.value) == (
        "spec.manifest: set interpreterPin or interpreter_pin, not both"
    )


@pytest.mark.parametrize(
    "manifest_type",
    ["PIPELINE_MANIFEST_TYPE_DECLARATIVE", "DECLARATIVE", "declarative"],
)
def test_manifest_type_names_are_declarative(tmp_path, manifest_type):
    """The full, short, and lowercase type names all pack the content."""
    yaml_dict = json.loads(json.dumps(_DECLARATIVE_YAML))
    yaml_dict["spec"]["manifest"]["type"] = manifest_type

    manifest = _convert(yaml_dict, tmp_path).pipeline.spec.manifest

    assert (
        manifest.type
        == pipeline_pb2.PipelineManifest.PIPELINE_MANIFEST_TYPE_DECLARATIVE
    )
    assert manifest.content.type_url.endswith("DeclarativeWorkflow")


@pytest.mark.parametrize(
    ("manifest_type", "message"),
    [
        (
            None,
            "spec.manifest.type is missing; set it to DECLARATIVE to apply"
            " spec.manifest.content",
        ),
        (
            "DECLARATVE",
            "spec.manifest.type 'DECLARATVE' is not a manifest type; set it to"
            " DECLARATIVE to apply spec.manifest.content",
        ),
    ],
)
def test_content_without_known_type_fails(tmp_path, manifest_type, message):
    """Content with a missing or unknown type fails, so it is never dropped."""
    yaml_dict = json.loads(json.dumps(_DECLARATIVE_YAML))
    if manifest_type is None:
        del yaml_dict["spec"]["manifest"]["type"]
    else:
        yaml_dict["spec"]["manifest"]["type"] = manifest_type

    with pytest.raises(DeclarativeManifestError) as err:
        _convert(yaml_dict, tmp_path)

    assert str(err.value) == message


@pytest.mark.parametrize("manifest_type", ["PIPELINE_MANIFEST_TYPE_UNIFLOW", "uniflow"])
def test_uniflow_with_content_takes_the_uniflow_path(manifest_type):
    """A UNIFLOW YAML from `ma pipeline get -o yaml` carries content and passes."""
    yaml_dict = {
        "spec": {"manifest": {"type": manifest_type, "content": {"@type": "x"}}}
    }

    assert not is_declarative(yaml_dict)


def test_unknown_workflow_field_fails_at_parse(tmp_path):
    """A mistyped field name fails in ParseDict, which the apply boundary reports."""
    yaml_dict = json.loads(json.dumps(_DECLARATIVE_YAML))
    yaml_dict["spec"]["manifest"]["content"]["tasks"]["root"]["taskFuncton"] = "x"

    with pytest.raises(ParseError, match="taskFuncton"):
        _convert(yaml_dict, tmp_path)


_EXAMPLES = Path(__file__).parents[6] / "examples" / "declarative" / "pipelines"
_EXAMPLE_FILES = sorted(_EXAMPLES.rglob("*.yaml"))


def test_example_dir_is_found():
    """The example pipelines exist where the next test reads them."""
    assert _EXAMPLE_FILES, f"no examples under {_EXAMPLES}"


@pytest.mark.parametrize(
    "path", _EXAMPLE_FILES, ids=lambda p: str(p.relative_to(_EXAMPLES))
)
def test_example_packs_through_the_converter(tmp_path, path):
    """Each example packs; an invalid one fails only on the apiserver."""
    manifest = _convert(yaml_to_dict(str(path)), tmp_path).pipeline.spec.manifest

    wf = declarative_pb2.DeclarativeWorkflow()
    assert manifest.content.Unpack(wf)
    assert wf.tasks
    if path.parent.name == "invalid":
        assert path.read_text().startswith("# Expected: INVALID_ARGUMENT")


@pytest.mark.parametrize(
    ("manifest", "message"),
    [
        (
            {"content": ["tasks"]},
            "spec.manifest.content must be a mapping of DeclarativeWorkflow fields",
        ),
        (
            {"content": {"@type": "type.googleapis.com/michelangelo.api.v2.Pipeline"}},
            "spec.manifest.content.@type must be type.googleapis.com/"
            "michelangelo.api.v2.DeclarativeWorkflow, got "
            "type.googleapis.com/michelangelo.api.v2.Pipeline",
        ),
        (
            {
                "content": {
                    "parameters": {"day": {"default": datetime.date(2024, 1, 1)}}
                }
            },
            "content.parameters.day: set type; it cannot be inferred from a default"
            " of YAML type date",
        ),
    ],
)
def test_pack_rejects_malformed_content(manifest, message):
    """Malformed content fails locally with the path and the fix."""
    with pytest.raises(DeclarativeManifestError) as err:
        pack_declarative_manifest(manifest)

    assert str(err.value) == message


_PLAN = {
    "interpreterVersion": "core-v1.4.2",
    "levels": [{"tasks": ["root"]}, {"tasks": ["left", "right"]}, {"tasks": ["join"]}],
    "warnings": ["task right has no retry policy"],
    "taskCount": 4,
}


def _pipeline(annotations: dict) -> pipeline_pb2.Pipeline:
    """Build a Pipeline that carries the given annotations."""
    pipeline = pipeline_pb2.Pipeline()
    pipeline.metadata.annotations.update(annotations)
    return pipeline


def test_explain_plan_absent_returns_none():
    """A pipeline without the annotation has no plan."""
    assert explain_plan_from(_pipeline({"other": "x"})) is None


def test_explain_plan_ignores_unknown_fields():
    """A newer server may add plan fields; mactl still parses the plan."""
    plan = explain_plan_from(
        _pipeline({EXPLAIN_PLAN_ANNOTATION: json.dumps({**_PLAN, "newField": 1})})
    )

    assert plan.interpreter_version == "core-v1.4.2"
    assert plan.task_count == 4


def test_render_explain_plan_table():
    """The table view lists levels in order and then the warnings."""
    plan = explain_plan_from(_pipeline({EXPLAIN_PLAN_ANNOTATION: json.dumps(_PLAN)}))

    assert render_explain_plan(plan, "table") == (
        "Interpreter: core-v1.4.2\n"
        "Tasks: 4\n"
        "\n"
        "LEVEL  TASKS\n"
        "0      root\n"
        "1      left, right\n"
        "2      join\n"
        "\n"
        "WARNINGS\n"
        "task right has no retry policy\n"
        "\n"
        "Dry run: nothing was saved."
    )


def test_render_explain_plan_table_without_warnings():
    """The table view omits the warnings section when there are none."""
    plan = explain_plan_from(
        _pipeline({EXPLAIN_PLAN_ANNOTATION: json.dumps({**_PLAN, "warnings": []})})
    )

    assert render_explain_plan(plan, "table").endswith(
        "2      join\n\nDry run: nothing was saved."
    )


def test_render_explain_plan_json():
    """The JSON view prints every field with proto field names."""
    plan = explain_plan_from(
        _pipeline({EXPLAIN_PLAN_ANNOTATION: json.dumps({**_PLAN, "warnings": []})})
    )

    assert json.loads(render_explain_plan(plan, "json")) == {
        "interpreter_version": "core-v1.4.2",
        "levels": [
            {"tasks": ["root"]},
            {"tasks": ["left", "right"]},
            {"tasks": ["join"]},
        ],
        "warnings": [],
        "task_count": 4,
    }


def test_render_explain_plan_table_escapes_control_characters():
    """Server text cannot move the cursor or set the terminal title."""
    plan = declarative_pb2.ExplainPlan(
        interpreter_version="v1\x1b[2J",
        task_count=1,
        levels=[declarative_pb2.ExecutionLevel(tasks=["a\x1b]0;t\x07"])],
        warnings=["line one\nline two\x9b"],
    )

    assert render_explain_plan(plan, "table") == (
        "Interpreter: v1\\x1b[2J\n"
        "Tasks: 1\n"
        "\n"
        "LEVEL  TASKS\n"
        "0      a\\x1b]0;t\\x07\n"
        "\n"
        "WARNINGS\n"
        "line one line two\\x9b\n"
        "\n"
        "Dry run: nothing was saved."
    )
