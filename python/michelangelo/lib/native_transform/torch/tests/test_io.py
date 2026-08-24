"""Tests for :mod:`michelangelo.lib.native_transform.torch.io`."""

from __future__ import annotations

import json

import pytest

torch = pytest.importorskip("torch")
pytest.importorskip("pydantic")
pytest.importorskip("yaml")
pytest.importorskip("fsspec")

from michelangelo.lib.native_transform.torch.io import TransformSpecIO  # noqa: E402
from michelangelo.lib.native_transform.torch.transform_spec import (  # noqa: E402
    TransformSpec,
)

RAW_SPECS = {
    "transform_specs": [
        {
            "transform_name": "Concatenate",
            "input_cols": ["col1"],
            "output_cols": ["col1_concatenated"],
        },
        {
            "transform_name": "StandardScaler",
            "input_cols": ["col2", "col3"],
            "output_cols": ["col2_scaled_col3_scaled"],
        },
    ]
}


class TestTransformSpecIO:
    """Round-trip and edge-case coverage for TransformSpecIO."""

    def test_write_then_read_round_trip(self, tmp_path) -> None:
        """A spec written to disk reads back with equivalent DAG state."""
        spec = TransformSpec(raw_transform_specs=RAW_SPECS)
        url = str(tmp_path / "spec.json")
        io = TransformSpecIO()

        metadata = io.write(url, spec)
        restored = io.read(url, metadata)

        assert isinstance(restored, TransformSpec)
        assert restored.to_dict() == spec.to_dict()
        assert restored.transform_levels == spec.transform_levels

    def test_write_returns_no_metadata(self, tmp_path) -> None:
        """write() returns None; the JSON is fully self-describing."""
        spec = TransformSpec(raw_transform_specs=RAW_SPECS)
        io = TransformSpecIO()

        assert io.write(str(tmp_path / "spec.json"), spec) is None

    def test_write_produces_valid_json_matching_to_json(self, tmp_path) -> None:
        """The file written is exactly TransformSpec.to_json()'s output."""
        spec = TransformSpec(raw_transform_specs=RAW_SPECS)
        url = str(tmp_path / "spec.json")
        TransformSpecIO().write(url, spec)

        with open(url) as f:
            on_disk = json.load(f)
        assert on_disk == json.loads(spec.to_json())

    def test_read_missing_file_raises(self, tmp_path) -> None:
        """Reading a nonexistent path surfaces the underlying fsspec error."""
        io = TransformSpecIO()
        with pytest.raises(FileNotFoundError):
            io.read(str(tmp_path / "does_not_exist.json"), None)

    def test_read_malformed_json_raises(self, tmp_path) -> None:
        """Malformed JSON at the target path raises during deserialization."""
        url = tmp_path / "bad.json"
        url.write_text("{not valid json")
        io = TransformSpecIO()
        with pytest.raises(json.JSONDecodeError):
            io.read(str(url), None)

    def test_read_result_is_independent_of_written_instance(self, tmp_path) -> None:
        """Mutating the original spec after write() doesn't affect the restored copy."""
        spec = TransformSpec(raw_transform_specs=RAW_SPECS)
        url = str(tmp_path / "spec.json")
        io = TransformSpecIO()
        io.write(url, spec)
        restored = io.read(url, None)

        spec.update_standard_scaler_specs({"col2_mean": 1.0, "col2_std": 2.0})

        assert restored.to_dict() != spec.to_dict()
