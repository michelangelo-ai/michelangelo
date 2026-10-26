"""Wire contract tests for the DECLARATIVE pipeline manifest type."""

import pytest
from google.protobuf import any_pb2, json_format, struct_pb2

from michelangelo.gen.api.v2 import declarative_pb2, pipeline_pb2

TYPE_URL = "type.googleapis.com/michelangelo.api.v2.DeclarativeWorkflow"


def test_declarative_manifest_type_value():
    """DECLARATIVE is value 4 and the removed value 2 stays unused."""
    assert pipeline_pb2.PipelineManifest.PIPELINE_MANIFEST_TYPE_DECLARATIVE == 4
    assert 2 not in pipeline_pb2.PipelineManifest.Type.values()


def test_declarative_workflow_any_round_trip():
    """A DeclarativeWorkflow packs into manifest.content and unpacks unchanged."""
    wf = declarative_pb2.DeclarativeWorkflow(schema_version="v1alpha1")
    wf.tasks["root"].task_function = "core/ray"
    packed = any_pb2.Any()
    packed.Pack(wf)
    assert packed.type_url == TYPE_URL

    manifest = pipeline_pb2.PipelineManifest(
        type=pipeline_pb2.PipelineManifest.PIPELINE_MANIFEST_TYPE_DECLARATIVE,
        content=packed,
        interpreter_pin=pipeline_pb2.InterpreterPin(version="core-v1.4.2"),
    )
    back = pipeline_pb2.PipelineManifest.FromString(manifest.SerializeToString())
    out = declarative_pb2.DeclarativeWorkflow()
    assert back.content.Unpack(out)
    assert out.schema_version == "v1alpha1"
    assert back.interpreter_pin.version == "core-v1.4.2"


def _spec(type_, default):
    """Build a ParameterSpec with the given type and a Value default."""
    spec = declarative_pb2.ParameterSpec(type=type_)
    spec.default.CopyFrom(_value(default))
    return spec


def _value(default):
    """Convert a Python value to a google.protobuf.Value."""
    value = struct_pb2.Value()
    json_format.ParseDict(default, value)
    return value


@pytest.mark.parametrize(
    ("type_", "default"),
    [
        (declarative_pb2.PARAMETER_TYPE_STRING, "demo"),
        (declarative_pb2.PARAMETER_TYPE_INTEGER, 2000),
        (declarative_pb2.PARAMETER_TYPE_NUMBER, 0.001),
        (declarative_pb2.PARAMETER_TYPE_BOOLEAN, True),
        (declarative_pb2.PARAMETER_TYPE_LIST, ["a", 7]),
        (declarative_pb2.PARAMETER_TYPE_DICT, {"depth": 6}),
        (declarative_pb2.PARAMETER_TYPE_STRING, None),
    ],
)
def test_parameter_spec_default_round_trip(type_, default):
    """A default of every declared type survives Any and protojson round trips."""
    wf = declarative_pb2.DeclarativeWorkflow(schema_version="v1alpha1")
    wf.parameters["p"].CopyFrom(_spec(type_, default))

    packed = any_pb2.Any()
    packed.Pack(wf)
    out = declarative_pb2.DeclarativeWorkflow()
    assert packed.Unpack(out)
    assert out.parameters["p"].type == type_
    assert json_format.MessageToDict(out.parameters["p"].default) == default

    as_json = json_format.MessageToJson(wf)
    from_json = json_format.Parse(as_json, declarative_pb2.DeclarativeWorkflow())
    assert from_json.parameters["p"].type == type_
    assert json_format.MessageToDict(from_json.parameters["p"].default) == default


def test_parameter_spec_unset_versus_null():
    """An absent default and an explicit JSON null stay distinct on the wire."""
    unset = declarative_pb2.ParameterSpec(
        type=declarative_pb2.PARAMETER_TYPE_STRING, required=True
    )
    null = _spec(declarative_pb2.PARAMETER_TYPE_STRING, None)
    assert not unset.HasField("default")
    assert null.HasField("default")
    for spec in (unset, null):
        back = declarative_pb2.ParameterSpec.FromString(spec.SerializeToString())
        assert back.HasField("default") == spec.HasField("default")
    assert null.default.WhichOneof("kind") == "null_value"


@pytest.mark.parametrize(
    ("body", "ok"),
    [
        ('{"tasks":{"t":{"dependsOn":[{"task":"a","when":"OUTCOME_FAILED"}]}}}', True),
        ('{"tasks":{"t":{"dependsOn":[{"task":"a","when":"FAILED"}]}}}', False),
        ('{"parameters":{"p":{"type":"PARAMETER_TYPE_INTEGER"}}}', True),
        ('{"parameters":{"p":{"type":"integer"}}}', False),
    ],
)
def test_enum_wire_form_is_full_name(body, ok):
    """Full enum names decode; protojson rejects short names."""
    if ok:
        json_format.Parse(body, declarative_pb2.DeclarativeWorkflow())
        return
    with pytest.raises(json_format.ParseError):
        json_format.Parse(body, declarative_pb2.DeclarativeWorkflow())
