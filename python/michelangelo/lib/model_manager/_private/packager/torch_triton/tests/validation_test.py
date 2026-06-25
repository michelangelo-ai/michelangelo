"""Tests for torch_triton validation helpers."""

import os
import tempfile
from unittest import TestCase

import pytest
import torch

from michelangelo.lib.model_manager._private.packager.torch_triton.raw_model_package import (  # noqa: E501
    convert_to_state_dict,
)
from michelangelo.lib.model_manager._private.packager.torch_triton.tests.fixtures.simple_model import (  # noqa: E501
    SimpleModel,
    save_scripted_model,
    save_state_dict,
)
from michelangelo.lib.model_manager._private.packager.torch_triton.validation import (
    _collect_outputs,
    _has_batch_dimension,
    validate_deployable_onnx_file,
    validate_model_class,
    validate_state_dict_file,
    validate_torchscript_file,
)
from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem

_MODEL_CLASS = (
    "michelangelo.lib.model_manager._private.packager.torch_triton."
    "tests.fixtures.simple_model.SimpleModel"
)
_NOT_A_MODULE_CLASS = (
    "michelangelo.lib.model_manager._private.packager.torch_triton."
    "tests.fixtures.simple_model.NotAModule"
)


class ValidateStateDictFileTest(TestCase):
    """Test cases for ``validate_state_dict_file``."""

    def test_valid_state_dict(self):
        """A genuine state_dict file validates successfully."""
        with tempfile.TemporaryDirectory() as temp_dir:
            path = os.path.join(temp_dir, "model.pt")
            save_state_dict(path)

            is_valid, error = validate_state_dict_file(path)

            self.assertTrue(is_valid)
            self.assertIsNone(error)

    def test_torchscript_is_not_a_state_dict(self):
        """A TorchScript file is rejected as a state_dict."""
        with tempfile.TemporaryDirectory() as temp_dir:
            path = os.path.join(temp_dir, "model.pt")
            save_scripted_model(path)

            is_valid, error = validate_state_dict_file(path)

            self.assertFalse(is_valid)
            self.assertIsNotNone(error)

    def test_empty_file_is_invalid(self):
        """An empty file is rejected."""
        with tempfile.TemporaryDirectory() as temp_dir:
            path = os.path.join(temp_dir, "model.pt")
            open(path, "w").close()

            is_valid, error = validate_state_dict_file(path)

            self.assertFalse(is_valid)
            self.assertIsInstance(error, ValueError)

    def test_directory_is_invalid(self):
        """A directory path is rejected."""
        with tempfile.TemporaryDirectory() as temp_dir:
            path = os.path.join(temp_dir, "model.pt")
            os.makedirs(path)

            is_valid, error = validate_state_dict_file(path)

            self.assertFalse(is_valid)
            self.assertIsInstance(error, ValueError)

    def test_missing_file_is_invalid(self):
        """A nonexistent path is rejected with FileNotFoundError."""
        with tempfile.TemporaryDirectory() as temp_dir:
            path = os.path.join(temp_dir, "missing.pt")

            is_valid, error = validate_state_dict_file(path)

            self.assertFalse(is_valid)
            self.assertIsInstance(error, FileNotFoundError)


class ValidateTorchScriptFileTest(TestCase):
    """Test cases for ``validate_torchscript_file``."""

    def test_valid_torchscript(self):
        """A scripted model file validates successfully."""
        with tempfile.TemporaryDirectory() as temp_dir:
            path = os.path.join(temp_dir, "model.pt")
            save_scripted_model(path)

            is_valid, error = validate_torchscript_file(path)

            self.assertTrue(is_valid)
            self.assertIsNone(error)

    def test_state_dict_is_not_torchscript(self):
        """A state_dict file is rejected as TorchScript."""
        with tempfile.TemporaryDirectory() as temp_dir:
            path = os.path.join(temp_dir, "model.pt")
            save_state_dict(path)

            is_valid, error = validate_torchscript_file(path)

            self.assertFalse(is_valid)
            self.assertIsNotNone(error)

    def test_missing_file_is_invalid(self):
        """A nonexistent path is rejected."""
        with tempfile.TemporaryDirectory() as temp_dir:
            path = os.path.join(temp_dir, "missing.pt")

            is_valid, error = validate_torchscript_file(path)

            self.assertFalse(is_valid)
            self.assertIsNotNone(error)


class ValidateDeployableOnnxFileTest(TestCase):
    """Test cases for ``validate_deployable_onnx_file``."""

    def test_valid_onnx(self):
        """A genuine ONNX export validates successfully."""
        with tempfile.TemporaryDirectory() as temp_dir:
            path = os.path.join(temp_dir, "model.onnx")
            model = torch.nn.Linear(4, 2)
            model.eval()
            torch.onnx.export(
                model,
                (torch.randn(2, 4),),
                path,
                input_names=["x"],
                output_names=["y"],
                opset_version=14,
            )

            is_valid, error = validate_deployable_onnx_file(path)

            self.assertTrue(is_valid, error)
            self.assertIsNone(error)

    def test_non_onnx_file_is_invalid(self):
        """A file that is not an ONNX model is rejected."""
        with tempfile.TemporaryDirectory() as temp_dir:
            path = os.path.join(temp_dir, "model.onnx")
            with open(path, "wb") as f:
                f.write(b"not an onnx model")

            is_valid, error = validate_deployable_onnx_file(path)

            self.assertFalse(is_valid)
            self.assertIsNotNone(error)

    def test_empty_file_is_invalid(self):
        """An empty file is rejected."""
        with tempfile.TemporaryDirectory() as temp_dir:
            path = os.path.join(temp_dir, "model.onnx")
            open(path, "w").close()

            is_valid, error = validate_deployable_onnx_file(path)

            self.assertFalse(is_valid)
            self.assertIsInstance(error, ValueError)

    def test_missing_file_is_invalid(self):
        """A nonexistent path is rejected with FileNotFoundError."""
        with tempfile.TemporaryDirectory() as temp_dir:
            path = os.path.join(temp_dir, "missing.onnx")

            is_valid, error = validate_deployable_onnx_file(path)

            self.assertFalse(is_valid)
            self.assertIsInstance(error, FileNotFoundError)


class ValidateModelClassTest(TestCase):
    """Test cases for ``validate_model_class``."""

    def test_valid_nn_module_subclass(self):
        """A torch.nn.Module subclass import path validates successfully."""
        is_valid, error = validate_model_class(_MODEL_CLASS)

        self.assertTrue(is_valid)
        self.assertIsNone(error)

    def test_non_module_class_is_invalid(self):
        """A class that is not a torch.nn.Module is rejected with TypeError."""
        is_valid, error = validate_model_class(_NOT_A_MODULE_CLASS)

        self.assertFalse(is_valid)
        self.assertIsInstance(error, TypeError)

    def test_unimportable_class_is_invalid(self):
        """An unresolvable import path is rejected."""
        is_valid, error = validate_model_class("does.not.exist.Model")

        self.assertFalse(is_valid)
        self.assertIsNotNone(error)


class HasBatchDimensionTest(TestCase):
    """Test cases for ``_has_batch_dimension``."""

    def test_batched_tensor_detected(self):
        """A tensor with a batch dimension larger than one is detected."""
        tensor = torch.zeros(8, 4)

        self.assertTrue(_has_batch_dimension(tensor, expected_shape=[4]))

    def test_unbatched_tensor_detected(self):
        """A tensor matching the per-sample shape is not treated as batched."""
        tensor = torch.zeros(4)

        self.assertFalse(_has_batch_dimension(tensor, expected_shape=[4]))

    def test_empty_expected_shape_is_not_batched(self):
        """With no expected shape, the tensor is never treated as batched."""
        tensor = torch.zeros(8, 4)

        self.assertFalse(_has_batch_dimension(tensor, expected_shape=[]))

    def test_scalar_tensor_is_not_batched(self):
        """A 0-dim tensor is never treated as batched."""
        tensor = torch.tensor(1.0)

        self.assertFalse(_has_batch_dimension(tensor, expected_shape=[4]))


# ---------------------------------------------------------------------------
# T2: _collect_outputs raises TypeError for unsupported output type
# ---------------------------------------------------------------------------

_SCHEMA = ModelSchema(
    input_schema=[ModelSchemaItem(name="x", data_type=DataType.FLOAT, shape=[4])],
    output_schema=[ModelSchemaItem(name="y", data_type=DataType.FLOAT, shape=[2])],
)


def test_collect_outputs_unsupported_type_raises_type_error():
    """Unsupported output type raises TypeError."""
    with pytest.raises(TypeError, match="Unsupported model output type"):
        _collect_outputs("not a tensor", _SCHEMA)


# ---------------------------------------------------------------------------
# T3: convert_to_state_dict converts a full nn.Module to state_dict in place
# ---------------------------------------------------------------------------


def test_convert_to_state_dict_from_nn_module():
    """A full nn.Module is converted to state_dict in place."""
    model = SimpleModel()
    expected = model.state_dict()

    with tempfile.TemporaryDirectory() as tmp:
        path = os.path.join(tmp, "model.pt")
        torch.save(model, path)

        convert_to_state_dict(path)

        loaded = torch.load(path, map_location="cpu", weights_only=True)
        assert isinstance(loaded, dict)
        assert set(loaded.keys()) == set(expected.keys())


# ---------------------------------------------------------------------------
# T1: _build_python_backend produces expected file structure
# ---------------------------------------------------------------------------


def test_build_python_backend_file_structure():
    """Python backend generates expected file structure."""
    from michelangelo.lib.model_manager._private.packager.template_renderer import (
        TritonTemplateRenderer,
    )
    from michelangelo.lib.model_manager._private.packager.torch_triton.model_package import (  # noqa: E501
        generate_model_package_content,
    )

    model_class_str = (
        "michelangelo.lib.model_manager._private.packager.torch_triton."
        "tests.fixtures.simple_model.SimpleModel"
    )

    with tempfile.TemporaryDirectory() as tmp:
        model_path = os.path.join(tmp, "model.pt")
        save_state_dict(model_path)

        root_path = os.path.join(tmp, "pkg")
        os.makedirs(root_path)

        gen = TritonTemplateRenderer()
        content = generate_model_package_content(
            gen=gen,
            model_path=model_path,
            model_name="test_model",
            model_revision="1",
            model_schema=_SCHEMA,
            backend="python",
            model_class=model_class_str,
            root_path=root_path,
            include_import_prefixes=["michelangelo"],
        )

        version_dir = os.path.join(root_path, "0")
        assert os.path.isfile(os.path.join(version_dir, "model", "model.pt"))
        assert os.path.isfile(os.path.join(version_dir, "model_class.txt"))
        assert "config.pbtxt" in content
        assert "user_model.py" in content["0"]


# ---------------------------------------------------------------------------
# Additional validation coverage
# ---------------------------------------------------------------------------

import numpy as np  # noqa: E402

from michelangelo.lib.model_manager._private.packager.torch_triton.validation import (  # noqa: E402
    _build_batch_inputs,
    _invoke_model,
    _load_model_class,
    _remove_batch_size_dimension,
    _validate_package_structure,
    validate_deployable_model_file,
    validate_pytorch_model_file,
    validate_raw_model_file,
    validate_raw_model_package,
)

_MODEL_CLASS_STR = (
    "michelangelo.lib.model_manager._private.packager.torch_triton."
    "tests.fixtures.simple_model.SimpleModel"
)


def _make_raw_package(tmp_dir: str) -> str:
    """Build a minimal valid raw model package under tmp_dir."""
    for d in ["metadata", "model", "defs", "dependencies"]:
        os.makedirs(os.path.join(tmp_dir, d))
    save_state_dict(os.path.join(tmp_dir, "model", "model.pt"))
    defs_path = os.path.join(tmp_dir, "defs", "model_class.txt")
    with open(defs_path, "w") as f:
        f.write(_MODEL_CLASS_STR)
    with open(os.path.join(tmp_dir, "metadata", "type.yaml"), "w") as f:
        f.write("type: torch\n")
    return tmp_dir


class ValidatePytorchModelFileTest(TestCase):
    """Tests for validate_pytorch_model_file."""

    def test_full_model_is_valid(self):
        """A pickled full nn.Module passes validation."""
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "model.pt")
            torch.save(SimpleModel(), path)

            is_valid, error = validate_pytorch_model_file(path)

            self.assertTrue(is_valid)
            self.assertIsNone(error)

    def test_state_dict_is_valid(self):
        """A state_dict file is valid for validate_pytorch_model_file."""
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "model.pt")
            save_state_dict(path)

            is_valid, error = validate_pytorch_model_file(path)

            self.assertTrue(is_valid)
            self.assertIsNone(error)

    def test_require_state_dict_fails_for_plain_module(self):
        """A full module without state_dict method fails when require_state_dict=True."""  # noqa: E501
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "model.pt")
            # Save a module — it has state_dict, so use a tensor instead
            torch.save(torch.tensor([1.0, 2.0]), path)

            is_valid, error = validate_pytorch_model_file(
                path, require_state_dict=False
            )
            # Tensor is not an nn.Module or state_dict — should fail
            self.assertFalse(is_valid)


class ValidateDeployableModelFileTest(TestCase):
    """Tests for validate_deployable_model_file."""

    def test_torchscript_is_valid(self):
        """A TorchScript model is accepted."""
        from michelangelo.lib.model_manager._private.packager.torch_triton.tests.fixtures.simple_model import (  # noqa: E501
            save_scripted_model,
        )

        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "model.pt")
            save_scripted_model(path)

            is_valid, error = validate_deployable_model_file(path)

            self.assertTrue(is_valid)
            self.assertIsNone(error)

    def test_full_pytorch_model_is_valid(self):
        """A full nn.Module is accepted for deployment."""
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "model.pt")
            torch.save(SimpleModel(), path)

            is_valid, error = validate_deployable_model_file(path)

            self.assertTrue(is_valid)
            self.assertIsNone(error)


class ValidateRawModelFileTest(TestCase):
    """Tests for validate_raw_model_file."""

    def test_full_model_with_state_dict_method_is_valid(self):
        """A full nn.Module (which has state_dict) passes raw validation."""
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "model.pt")
            torch.save(SimpleModel(), path)

            is_valid, error = validate_raw_model_file(path)

            self.assertTrue(is_valid)
            self.assertIsNone(error)


class ValidatePackageStructureTest(TestCase):
    """Tests for _validate_package_structure."""

    def test_missing_package_raises_file_not_found(self):
        """A nonexistent package path raises FileNotFoundError."""
        with self.assertRaises(FileNotFoundError):
            _validate_package_structure("/nonexistent/package")

    def test_missing_required_dir_raises_file_not_found(self):
        """A package missing a required subdirectory raises FileNotFoundError."""
        with tempfile.TemporaryDirectory() as tmp:
            # Only create metadata, model, defs — missing dependencies
            for d in ["metadata", "model", "defs"]:
                os.makedirs(os.path.join(tmp, d))

            with self.assertRaises(FileNotFoundError):
                _validate_package_structure(tmp)

    def test_missing_pt_file_raises_file_not_found(self):
        """A package with no .pt file in model dir raises FileNotFoundError."""
        with tempfile.TemporaryDirectory() as tmp:
            for d in ["metadata", "model", "defs", "dependencies"]:
                os.makedirs(os.path.join(tmp, d))

            with self.assertRaises(FileNotFoundError):
                _validate_package_structure(tmp)

    def test_valid_package_returns_model_path(self):
        """A valid package returns the path to the .pt file."""
        with tempfile.TemporaryDirectory() as tmp:
            _make_raw_package(tmp)

            result = _validate_package_structure(tmp)

            self.assertTrue(result.endswith("model.pt"))
            self.assertTrue(os.path.isfile(result))


class LoadModelClassTest(TestCase):
    """Tests for _load_model_class."""

    def test_loads_model_class(self):
        """_load_model_class returns the class from model_class.txt."""
        with tempfile.TemporaryDirectory() as tmp:
            _make_raw_package(tmp)

            cls = _load_model_class(tmp)

            self.assertTrue(issubclass(cls, torch.nn.Module))


class InvokeModelTest(TestCase):
    """Tests for _invoke_model."""

    def test_kwargs_calling_convention(self):
        """A model whose forward takes named params is called with **kwargs."""
        model = SimpleModel()
        model.eval()
        batch = {"x": torch.randn(1, 4)}

        with torch.no_grad():
            result = _invoke_model(model, batch)

        self.assertIsInstance(result, torch.Tensor)
        self.assertEqual(result.shape, (1, 2))

    def test_single_dict_calling_convention(self):
        """A model whose forward takes a single mapping is called with positional."""

        class DictInputModel(torch.nn.Module):
            """Model that takes a dict as forward input."""

            def forward(self, inputs):
                """Forward pass."""
                return inputs["x"]

        model = DictInputModel()
        batch = {"x": torch.randn(1, 4)}

        result = _invoke_model(model, batch)

        self.assertTrue(torch.allclose(result, batch["x"]))


class RemoveBatchSizeDimensionTest(TestCase):
    """Tests for _remove_batch_size_dimension."""

    def test_removes_batch_dim_from_tensor(self):
        """Batch dimension is squeezed and converted to numpy."""
        output = torch.randn(1, 2)
        schema_item = ModelSchemaItem(name="y", data_type=DataType.FLOAT, shape=[2])

        result = _remove_batch_size_dimension(output, schema_item)

        self.assertEqual(result.shape, (2,))

    def test_scalar_reshaped_to_1d(self):
        """A scalar output is reshaped to 1-D when schema has dimensions."""
        output = torch.tensor([[1.5]])  # shape [1, 1] -> squeeze(0) -> [1]
        schema_item = ModelSchemaItem(name="y", data_type=DataType.FLOAT, shape=[1])

        result = _remove_batch_size_dimension(output, schema_item)

        self.assertEqual(result.ndim, 1)


class BuildBatchInputsTest(TestCase):
    """Tests for _build_batch_inputs."""

    def test_numpy_input_converted_to_batched_tensor(self):
        """A numpy array input is converted to a batched tensor."""
        data = {"x": np.ones((4,), dtype=np.float32)}
        result = _build_batch_inputs(data, _SCHEMA)

        self.assertIn("x", result)
        self.assertEqual(result["x"].shape[0], 1)

    def test_torch_tensor_input_accepted(self):
        """A torch.Tensor input is accepted directly."""
        data = {"x": torch.ones(1, 4)}
        result = _build_batch_inputs(data, _SCHEMA)

        self.assertIn("x", result)

    def test_unsupported_type_raises_type_error(self):
        """An unsupported input type raises TypeError."""
        data = {"x": "not an array"}

        with self.assertRaisesRegex(TypeError, "Unsupported input type"):
            _build_batch_inputs(data, _SCHEMA)


class CollectOutputsTest(TestCase):
    """Tests for _collect_outputs."""

    def test_single_tensor_output(self):
        """A single tensor output maps to the schema's output name."""
        output = torch.randn(1, 2)
        result = _collect_outputs(output, _SCHEMA)

        self.assertIn("y", result)

    def test_dict_output(self):
        """A dict output is passed through with batch dim removed."""
        output = {"y": torch.randn(1, 2)}
        result = _collect_outputs(output, _SCHEMA)

        self.assertIn("y", result)

    def test_list_output(self):
        """A list output maps elements to schema output names."""
        output = [torch.randn(1, 2)]
        result = _collect_outputs(output, _SCHEMA)

        self.assertIn("y", result)

    def test_named_tuple_output(self):
        """A named tuple output is handled via _asdict."""
        from collections import namedtuple

        Output = namedtuple("Output", ["y"])
        output = Output(y=torch.randn(1, 2))
        result = _collect_outputs(output, _SCHEMA)

        self.assertIn("y", result)

    def test_unsupported_type_raises_type_error(self):
        """An unsupported output type raises TypeError."""
        with self.assertRaisesRegex(TypeError, "Unsupported model output type"):
            _collect_outputs("not a tensor", _SCHEMA)


class ValidateRawModelPackageTest(TestCase):
    """Tests for validate_raw_model_package."""

    def test_valid_package_passes_without_sample_data(self):
        """A valid package without sample data passes validation."""
        with tempfile.TemporaryDirectory() as tmp:
            _make_raw_package(tmp)

            # Should not raise
            validate_raw_model_package(tmp)

    def test_valid_package_with_sample_data(self):
        """A valid package with sample data passes full validation."""
        with tempfile.TemporaryDirectory() as tmp:
            _make_raw_package(tmp)
            sample_data = [{"x": np.random.randn(4).astype(np.float32)}]

            # Should not raise
            validate_raw_model_package(
                tmp, sample_data=sample_data, model_schema=_SCHEMA
            )
