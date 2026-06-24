"""Tests for submodel schema capture."""

from unittest import TestCase

import torch

from michelangelo.lib.model_manager._private.packager.torch_triton.submodel_schema import (  # noqa: E501
    capture_submodel_schemas,
    write_submodel_schemas,
)
from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem


class _TwoLayer(torch.nn.Module):
    def __init__(self):
        super().__init__()
        self.fc1 = torch.nn.Linear(4, 3)
        self.fc2 = torch.nn.Linear(3, 2)

    def forward(self, x):
        return self.fc2(self.fc1(x))


class CaptureSubmodelSchemasTest(TestCase):
    """Tests for capture_submodel_schemas."""

    def test_captures_each_submodel(self):
        """Captures each submodel."""
        model = _TwoLayer().eval()
        x = torch.zeros(8, 4)
        with torch.no_grad():
            output, schemas = capture_submodel_schemas(model, lambda: model(x))

        self.assertEqual(list(output.shape), [8, 2])
        self.assertIn("fc1", schemas)
        self.assertIn("fc2", schemas)

        fc1 = schemas["fc1"]
        # Batch dim is stripped: per-sample input shape is [4].
        self.assertEqual(fc1.input_schema[0].shape, [4])
        self.assertEqual(fc1.input_schema[0].data_type, DataType.FLOAT)
        self.assertEqual(fc1.output_schema[0].shape, [3])

    def test_hook_failure_does_not_propagate(self):
        """Hook failure does not propagate."""
        model = _TwoLayer().eval()

        def boom():
            raise RuntimeError("forward exploded")

        with self.assertRaisesRegex(RuntimeError, "forward exploded"):
            capture_submodel_schemas(model, boom)


class WriteSubmodelSchemasTest(TestCase):
    """Tests for write_submodel_schemas."""

    def test_empty_schemas_writes_nothing(self, tmp_path=None):
        """Empty schemas writes nothing."""
        import tempfile

        with tempfile.TemporaryDirectory() as tmp:
            write_submodel_schemas(tmp, {}, "submodel_schemas.yaml")
            import os

            self.assertFalse(
                os.path.exists(os.path.join(tmp, "metadata", "submodel_schemas.yaml"))
            )

    def test_writes_yaml(self):
        """Writes yaml."""
        import os
        import tempfile

        import yaml

        schemas = {
            "fc1": ModelSchema(
                input_schema=[
                    ModelSchemaItem(name="x", data_type=DataType.FLOAT, shape=[4])
                ],
                output_schema=[
                    ModelSchemaItem(name="y", data_type=DataType.FLOAT, shape=[3])
                ],
            )
        }
        with tempfile.TemporaryDirectory() as tmp:
            os.makedirs(os.path.join(tmp, "metadata"))
            write_submodel_schemas(tmp, schemas, "submodel_schemas.yaml")
            path = os.path.join(tmp, "metadata", "submodel_schemas.yaml")
            self.assertTrue(os.path.exists(path))
            with open(path) as f:
                data = yaml.safe_load(f)
            self.assertEqual(data["fc1"]["input_schema"][0]["name"], "x")
            self.assertEqual(data["fc1"]["input_schema"][0]["data_type"], "float")
