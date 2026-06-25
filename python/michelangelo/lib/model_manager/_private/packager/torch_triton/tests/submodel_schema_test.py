"""Tests for submodel schema capture."""

from unittest import TestCase

import torch

from michelangelo.lib.model_manager._private.packager.torch_triton.submodel_schema import (  # noqa: E501
    _output_facts,
    _return_names,
    _tensor_facts,
    capture_submodel_schemas,
    get_forward_param_names,
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


class TensorFactsTest(TestCase):
    """Tests for _tensor_facts."""

    def test_named_tensor(self):
        """A tensor with param_name includes the name in the fact."""
        t = torch.zeros(8, 4)
        facts = _tensor_facts(t, param_name="x")
        self.assertEqual(len(facts), 1)
        self.assertEqual(facts[0]["name"], "x")

    def test_unnamed_tensor(self):
        """A tensor without param_name has no name key."""
        t = torch.zeros(8, 4)
        facts = _tensor_facts(t, param_name=None)
        self.assertNotIn("name", facts[0])

    def test_dict_input(self):
        """A dict of tensors is unpacked into facts keyed by dict key."""
        d = {"a": torch.zeros(4), "b": torch.zeros(2)}
        facts = _tensor_facts(d)
        names = {f["name"] for f in facts}
        self.assertIn("a", names)
        self.assertIn("b", names)

    def test_list_input(self):
        """A list of tensors is recursively unpacked."""
        lst = [torch.zeros(4), torch.zeros(2)]
        facts = _tensor_facts(lst)
        self.assertEqual(len(facts), 2)

    def test_namedtuple_input(self):
        """A namedtuple is unpacked via _asdict."""
        from collections import namedtuple

        NT = namedtuple("NT", ["x", "y"])
        facts = _tensor_facts(NT(x=torch.zeros(4), y=torch.zeros(2)))
        names = {f["name"] for f in facts}
        self.assertIn("x", names)

    def test_non_tensor_returns_empty(self):
        """A non-tensor scalar returns an empty list."""
        facts = _tensor_facts(42)
        self.assertEqual(facts, [])


class OutputFactsTest(TestCase):
    """Tests for _output_facts."""

    def test_single_tensor_with_return_name(self):
        """A single tensor gets the name from return_names."""
        t = torch.zeros(8, 2)
        facts = _output_facts(t, return_names=["y"])
        self.assertEqual(facts[0]["name"], "y")

    def test_single_tensor_fallback_name(self):
        """A single tensor with no return_names gets output_0."""
        t = torch.zeros(8, 2)
        facts = _output_facts(t, return_names=None)
        self.assertEqual(facts[0]["name"], "output_0")

    def test_list_output_with_return_names(self):
        """A list output assigns return_names to each element."""
        output = [torch.zeros(8, 2), torch.zeros(8, 4)]
        facts = _output_facts(output, return_names=["a", "b"])
        names = [f["name"] for f in facts]
        self.assertIn("a", names)
        self.assertIn("b", names)

    def test_dict_output(self):
        """A dict output is handled via _tensor_facts."""
        output = {"z": torch.zeros(8, 2)}
        facts = _output_facts(output, return_names=None)
        self.assertEqual(facts[0]["name"], "z")

    def test_namedtuple_output(self):
        """A namedtuple output is handled via _asdict path."""
        from collections import namedtuple

        NT = namedtuple("NT", ["z"])
        output = NT(z=torch.zeros(8, 2))
        facts = _output_facts(output, return_names=None)
        self.assertEqual(facts[0]["name"], "z")


class ReturnNamesTest(TestCase):
    """Tests for _return_names."""

    def test_single_name_return(self):
        """A function returning a single variable has one name."""

        class M(torch.nn.Module):
            """Single return model."""

            def forward(self, x):
                """Forward."""
                result = x + 1
                return result

        names = _return_names(M())
        self.assertEqual(names, ["result"])

    def test_tuple_return(self):
        """A function returning a tuple of variables has multiple names."""

        class M(torch.nn.Module):
            """Tuple return model."""

            def forward(self, x):
                """Forward."""
                a = x
                b = x + 1
                return a, b

        names = _return_names(M())
        self.assertIn("a", names)
        self.assertIn("b", names)

    def test_no_return_statement(self):
        """A forward with no explicit return yields None."""

        class M(torch.nn.Module):
            """No return model."""

            def forward(self, x):
                """Forward."""
                pass

        names = _return_names(M())
        self.assertIsNone(names)


class GetForwardParamNamesTest(TestCase):
    """Tests for get_forward_param_names."""

    def test_named_params_extracted(self):
        """Named parameters of forward (excluding self) are returned."""
        model = torch.nn.Linear(4, 2)
        names = get_forward_param_names(model)
        self.assertIn("input", names)
        self.assertNotIn("self", names)

    def test_error_returns_empty_list(self):
        """If inspect raises, an empty list is returned."""

        class BadModule(torch.nn.Module):
            """Module with broken forward."""

            forward = property(lambda self: None)  # type: ignore[assignment]

        names = get_forward_param_names(BadModule())
        self.assertEqual(names, [])
