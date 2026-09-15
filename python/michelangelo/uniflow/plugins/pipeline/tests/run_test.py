"""Tests for local pipeline plugin input serialization."""

from google.protobuf.json_format import MessageToDict

from michelangelo.uniflow.plugins.pipeline.run import _build_input_struct


def test_build_uniflow_input_struct_with_installed_protobuf_runtime():
    """Generated Values are copied into protobuf maps without type coercion."""
    input_struct = _build_input_struct(
        environ={"MA_NAMESPACE": "default"},
        args=[{"sample_size": 3}, "raw-argument"],
        kwargs={"name": "cola", "tokenizer_max_length": 128},
    )

    assert input_struct is not None
    assert MessageToDict(input_struct, preserving_proto_field_name=True) == {
        "environ": {"MA_NAMESPACE": "default"},
        "args": [{"sample_size": 3.0}, {"value": "raw-argument"}],
        "kwargs": [["name", "cola"], ["tokenizer_max_length", 128.0]],
    }
