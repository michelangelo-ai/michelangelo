from google.protobuf import any_pb2

from michelangelo.gen.api.v2 import declarative_pb2, pipeline_pb2

TYPE_URL = "type.googleapis.com/michelangelo.api.v2.DeclarativeWorkflow"


def test_declarative_manifest_type_value():
    assert pipeline_pb2.PipelineManifest.PIPELINE_MANIFEST_TYPE_DECLARATIVE == 4
    assert 2 not in pipeline_pb2.PipelineManifest.Type.values()


def test_declarative_workflow_any_round_trip():
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
