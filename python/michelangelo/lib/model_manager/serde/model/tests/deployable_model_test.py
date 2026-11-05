"""Tests for loading deployable packages and their schemas."""

from __future__ import annotations

from unittest.mock import patch

import pytest

from michelangelo.lib.model_manager._private.packager.template_renderer import (
    TritonTemplateRenderer,
)
from michelangelo.lib.model_manager._private.packager.torch_triton.config_pbtxt import (
    generate_config_pbtxt_content,
)
from michelangelo.lib.model_manager._private.serde.model.deployable_model_schema import (  # noqa: E501
    load_deployable_model_schema,
)
from michelangelo.lib.model_manager._private.serde.model.triton_backend_type import (
    get_deployable_backend_type,
)
from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem
from michelangelo.lib.model_manager.serde.model import (
    load_deployable_model,
    load_model_schema,
)

_SCHEMA = ModelSchema(
    input_schema=[
        ModelSchemaItem("ids", DataType.LONG, [3]),
        ModelSchemaItem("text", DataType.STRING, [1], optional=True),
        ModelSchemaItem("grid", DataType.DOUBLE, [2, 4]),
        ModelSchemaItem("unshaped", DataType.BOOLEAN, None),
    ],
    output_schema=[ModelSchemaItem("score", DataType.FLOAT, [1])],
)


def _write_config(path, schema=_SCHEMA, backend="pytorch"):
    """Write the config.pbtxt the torch packager would write."""
    path.mkdir(parents=True, exist_ok=True)
    (path / "config.pbtxt").write_text(
        generate_config_pbtxt_content(
            TritonTemplateRenderer(), "m", "1", schema, backend=backend
        )
    )
    return str(path)


def test_schema_round_trips_through_config_pbtxt(tmp_path):
    """Names, types, shapes and optional survive; no shape comes back as [-1]."""
    schema = load_deployable_model_schema(_write_config(tmp_path))

    assert [
        (i.name, i.data_type, i.shape, i.optional) for i in schema.input_schema
    ] == [
        ("ids", DataType.LONG, [3], None),
        ("text", DataType.STRING, [1], True),
        ("grid", DataType.DOUBLE, [2, 4], None),
        ("unshaped", DataType.BOOLEAN, [-1], None),
    ]
    assert [(i.name, i.data_type, i.shape) for i in schema.output_schema] == [
        ("score", DataType.FLOAT, [1])
    ]


def test_schema_tolerates_missing_fields_and_sections(tmp_path):
    """Unknown types, missing dims and a missing output section do not fail."""
    (tmp_path / "config.pbtxt").write_text(
        'backend: "python"\n'
        "input: [\n"
        '  { name: "a", data_type: TYPE_BF16 },\n'
        "  { data_type: TYPE_FP32, dims: [ 1 ] }\n"
        "]\n"
    )

    schema = load_deployable_model_schema(str(tmp_path))

    assert [(i.name, i.data_type, i.shape) for i in schema.input_schema] == [
        ("a", DataType.UNKNOWN, None)
    ]
    assert schema.output_schema == []


def test_schema_with_an_unterminated_section(tmp_path):
    """A truncated file still yields the items it contains."""
    (tmp_path / "config.pbtxt").write_text(
        'input: [ { name: "a", data_type: TYPE_INT64, dims: [ 2 ] }'
    )

    schema = load_deployable_model_schema(str(tmp_path))

    assert [(i.name, i.shape) for i in schema.input_schema] == [("a", [2])]


def test_schema_needs_config_pbtxt(tmp_path):
    """A directory without config.pbtxt is reported."""
    with pytest.raises(FileNotFoundError, match="config.pbtxt"):
        load_deployable_model_schema(str(tmp_path))


def test_load_model_schema_falls_back_to_config_pbtxt(tmp_path):
    """A torch deployable has no schema.yaml, so config.pbtxt is read."""
    schema = load_model_schema(_write_config(tmp_path))

    assert [i.name for i in schema.output_schema] == ["score"]


def test_load_model_schema_without_either_file(tmp_path):
    """A directory with neither file has no schema."""
    assert load_model_schema(str(tmp_path)) is None


@pytest.mark.parametrize(
    ("content", "error", "match"),
    [
        (None, FileNotFoundError, "config.pbtxt not found"),
        ("max_batch_size: 8\n", ValueError, "names no backend"),
    ],
    ids=["no-config", "no-backend"],
)
def test_backend_type_errors(tmp_path, content, error, match):
    """A missing file or backend line is reported."""
    if content is not None:
        (tmp_path / "config.pbtxt").write_text(content)
    with pytest.raises(error, match=match):
        get_deployable_backend_type(str(tmp_path))


def test_load_needs_a_package_directory(tmp_path):
    """A path that does not exist is reported before anything else."""
    with pytest.raises(FileNotFoundError, match="Model package not found"):
        load_deployable_model(str(tmp_path / "nope"))


def test_load_rejects_an_unsupported_backend(tmp_path):
    """A backend without a loader names the supported ones."""
    _write_config(tmp_path, backend="tensorrt")
    with pytest.raises(NotImplementedError, match="'tensorrt'.*pytorch"):
        load_deployable_model(str(tmp_path))


@pytest.mark.parametrize("backend", ["pytorch", "onnxruntime"])
def test_load_reports_a_missing_model_file(tmp_path, backend):
    """The expected file under 0/ is named in the error."""
    _write_config(tmp_path, backend=backend)
    with pytest.raises(FileNotFoundError, match="Deployable model file not found"):
        load_deployable_model(str(tmp_path))


@pytest.mark.parametrize(
    ("class_file", "match"),
    [(None, "Missing model_class.txt"), ("  \n", "model_class.txt is empty")],
    ids=["missing", "empty"],
)
def test_load_python_package_needs_a_model_class(tmp_path, class_file, match):
    """A python package without a usable model_class.txt is reported."""
    _write_config(tmp_path, backend="python")
    (tmp_path / "0").mkdir()
    if class_file is not None:
        (tmp_path / "0" / "model_class.txt").write_text(class_file)
    with pytest.raises(ValueError, match=match):
        load_deployable_model(str(tmp_path))


@pytest.mark.parametrize(
    ("device", "available", "expected"),
    [
        ("cpu", ["CUDAExecutionProvider", "CPUExecutionProvider"], ["CPU"]),
        ("cuda", ["CPUExecutionProvider"], ["CPU"]),
        ("cuda", ["CUDAExecutionProvider", "CPUExecutionProvider"], ["CUDA", "CPU"]),
    ],
    ids=["cpu-requested", "no-cuda-provider", "cuda"],
)
def test_onnx_requests_cuda_only_when_asked_and_available(
    tmp_path, device, available, expected
):
    """A GPU build on a CPU worker never asks for the CUDA provider."""
    _write_config(tmp_path, backend="onnxruntime")
    (tmp_path / "0").mkdir()
    (tmp_path / "0" / "model.onnx").write_bytes(b"")
    with (
        patch("onnxruntime.get_available_providers", return_value=available),
        patch("onnxruntime.InferenceSession") as session,
    ):
        load_deployable_model(str(tmp_path), device=device)

    assert session.call_args.kwargs["providers"] == [
        f"{name}ExecutionProvider" for name in expected
    ]
