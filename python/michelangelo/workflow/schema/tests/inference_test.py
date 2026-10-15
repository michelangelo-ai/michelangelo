"""Tests for michelangelo.workflow.schema.inference dataclasses."""

from __future__ import annotations

import dataclasses
import json

import pytest

from michelangelo.uniflow.core.codec import DataclassCodec
from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.schema.inference import (
    BatchOptions,
    ModelReference,
    OfflineInferenceConfig,
    PaddingOptions,
    TabularInferenceConfig,
)
from michelangelo.workflow.schema.ray_data_io import (
    ParquetReadConfig,
    RayDataContextConfig,
    WriteConfig,
)

# Field names are a wire format shared with other implementations of this
# schema, so a rename must show up as a test failure.
_WIRE_FIELDS = {
    TabularInferenceConfig: {
        "local_models",
        "parquet_read_config",
        "write_config",
        "ray_data_context",
    },
    OfflineInferenceConfig: {
        "model",
        "input_columns",
        "batch_options",
        "padding_options",
        "columns_to_keep",
        "backend",
    },
    BatchOptions: {
        "batch_size",
        "num_gpus",
        "concurrency",
        "num_cpus",
        "max_concurrency",
        "max_tasks_in_flight_per_actor",
        "max_restarts",
        "max_task_retries",
    },
    PaddingOptions: {"column_padding_values"},
    ModelReference: {"name", "revision_id"},
}

_FULL_CONFIG = TabularInferenceConfig(
    local_models=[
        OfflineInferenceConfig(
            model=ModelReference(name="housing", revision_id=2),
            input_columns=["a", "b"],
            batch_options=BatchOptions(
                batch_size=1024,
                num_gpus=0.5,
                concurrency=2,
                num_cpus=4,
                max_concurrency=2,
                max_tasks_in_flight_per_actor=3,
                max_restarts=1,
                max_task_retries=0,
            ),
            padding_options=PaddingOptions(column_padding_values={"a": -1, "b": 0.0}),
            columns_to_keep=["label"],
            backend="pkg.module.factory",
        )
    ],
    parquet_read_config=ParquetReadConfig(num_cpus=1),
    write_config=WriteConfig(max_rows_per_file=100),
    ray_data_context=RayDataContextConfig(min_block_size=1),
)


def _roundtrip(value):
    codec = DataclassCodec()
    return codec.decode(codec.encode(value))


@pytest.mark.parametrize("cls", list(_WIRE_FIELDS))
def test_field_names_match_wire_format(cls):
    """Field names are exactly the documented set."""
    assert {f.name for f in dataclasses.fields(cls)} == _WIRE_FIELDS[cls]


@pytest.mark.parametrize(
    "value",
    [
        ModelReference(name="m"),
        ModelReference(name="m", revision_id=0),
        BatchOptions(),
        BatchOptions(batch_size=8, num_gpus=0.125, max_restarts=-1),
        PaddingOptions(),
        PaddingOptions(column_padding_values={"x": -1}),
        OfflineInferenceConfig(),
        TabularInferenceConfig(),
        TabularInferenceConfig(local_models=[]),
    ],
)
def test_codec_roundtrip_leaf_values(value):
    """Each dataclass, including defaults and empty values, round-trips."""
    assert _roundtrip(value) == value


def test_codec_roundtrip_nested_config():
    """A fully populated config round-trips through the dataclass dict form."""
    encoded = DataclassCodec().encode(_FULL_CONFIG)
    assert json.dumps(encoded, default=str)
    decoded = _roundtrip(_FULL_CONFIG)
    assert decoded.local_models[0].model == ModelReference(
        name="housing", revision_id=2
    )
    assert decoded.local_models[0].batch_options.max_task_retries == 0


def test_defaults_are_unset():
    """A bare config sets nothing, so the task decides every default."""
    cfg = OfflineInferenceConfig()
    assert cfg.model is None
    assert cfg.batch_options == BatchOptions()
    assert cfg.padding_options.column_padding_values is None
    assert TabularInferenceConfig().local_models is None


def test_unknown_key_fails_loudly():
    """Options this package does not support raise TypeError on decode."""
    for key in ("onnx_to_trt", "trt_precision", "inference_engine", "namespace"):
        with pytest.raises(TypeError):
            OfflineInferenceConfig(**{key: 1})
    with pytest.raises(TypeError):
        ModelReference(name="m", namespace="ns")
    with pytest.raises(TypeError):
        TabularInferenceConfig(sampling=None)


def test_unknown_key_fails_through_codec():
    """The codec path raises the same TypeError as direct construction."""
    encoded = DataclassCodec().encode(OfflineInferenceConfig())
    encoded["trt_precision"] = "fp16"
    with pytest.raises(TypeError):
        DataclassCodec().decode(encoded)


@pytest.mark.parametrize(
    "field_name",
    [
        "batch_size",
        "num_gpus",
        "concurrency",
        "num_cpus",
        "max_concurrency",
        "max_tasks_in_flight_per_actor",
    ],
)
@pytest.mark.parametrize("bad", [0, -1])
def test_batch_options_reject_non_positive(field_name, bad):
    """Sizes and worker counts must be positive when set."""
    with pytest.raises(ConfigurationError, match=field_name):
        BatchOptions(**{field_name: bad})


@pytest.mark.parametrize("field_name", ["max_restarts", "max_task_retries"])
def test_retry_fields_allow_zero_and_minus_one_only_down_to_minus_one(field_name):
    """Zero disables and -1 is unlimited; anything lower is rejected."""
    assert getattr(BatchOptions(**{field_name: 0}), field_name) == 0
    assert getattr(BatchOptions(**{field_name: -1}), field_name) == -1
    with pytest.raises(ConfigurationError, match=field_name):
        BatchOptions(**{field_name: -2})


def test_model_reference_validation():
    """An empty name or a negative revision is rejected."""
    with pytest.raises(ConfigurationError, match="name"):
        ModelReference(name="")
    with pytest.raises(ConfigurationError, match="revision_id"):
        ModelReference(name="m", revision_id=-1)


def test_padding_options_reject_non_string_keys():
    """Column names must be strings."""
    with pytest.raises(ConfigurationError, match="keys"):
        PaddingOptions(column_padding_values={1: 0})


def test_no_unsupported_surface_in_module():
    """Out-of-scope options never appear in the schema module source."""
    import inspect

    from michelangelo.workflow.schema import inference

    source = inspect.getsource(inference)
    for banned in ("onnx_to_trt", "trt_precision", "inference_engine", "namespace:"):
        assert banned not in source.replace("``namespace``", "")
