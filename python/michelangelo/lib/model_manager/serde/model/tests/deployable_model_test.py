"""Tests for the deployable model loader.

Packages are built with the real packagers, so the tests check that the loader
reads exactly what the packagers write.
"""

from __future__ import annotations

import os
from unittest import TestCase
from unittest.mock import patch

import numpy as np
import torch

from michelangelo.lib.model_manager._private.packager.torch_triton.tests.fixtures.simple_model import (  # noqa: E501
    SimpleModel,
)
from michelangelo.lib.model_manager.interface.custom_model import Model
from michelangelo.lib.model_manager.packager.custom_triton.custom_triton_packager import (  # noqa: E501
    CustomTritonPackager,
)
from michelangelo.lib.model_manager.packager.torch_triton.torch_triton_packager import (
    TorchTritonPackager,
)
from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem
from michelangelo.lib.model_manager.serde.model import load_deployable_model

_FIXTURES = (
    "michelangelo.lib.model_manager._private.packager.torch_triton.tests.fixtures"
)
_SCHEMA = ModelSchema(
    input_schema=[ModelSchemaItem("x", DataType.FLOAT, [4])],
    output_schema=[ModelSchemaItem("y", DataType.FLOAT, [2])],
)
_CUSTOM_CLASS = (
    "michelangelo.lib.model_manager._private.packager.custom_triton.tests.fixtures."
    "predict.Predict"
)


def _torch_package(root: str, backend: str | None) -> tuple[str, torch.nn.Module]:
    """Package a seeded SimpleModel with the torch packager."""
    torch.manual_seed(0)
    model = SimpleModel().eval()
    source = os.path.join(root, "model.pt")
    torch.save(model.state_dict(), source)
    path = TorchTritonPackager().create_model_package(
        source,
        _SCHEMA,
        dest_model_path=os.path.join(root, "package"),
        model_class=f"{_FIXTURES}.simple_model.SimpleModel",
        backend=backend,
        sample_data=[{"x": torch.ones(1, 4)}],
        include_import_prefixes=[_FIXTURES],
    )
    return path, model


class LoadDeployableModelTest(TestCase):
    """Loading each kind of deployable package."""

    def setUp(self) -> None:
        """Create a scratch directory."""
        import tempfile

        self._tmp = tempfile.TemporaryDirectory()
        self.root = self._tmp.name
        self.batch = torch.randn(3, 4)

    def tearDown(self) -> None:
        """Remove the scratch directory."""
        self._tmp.cleanup()

    def _expected(self, model: torch.nn.Module) -> np.ndarray:
        with torch.no_grad():
            return model(self.batch).numpy()

    def test_pytorch_backend_loads_torchscript_on_cpu_in_eval_mode(self):
        """A pytorch package gives the packaged TorchScript module."""
        path, source = _torch_package(self.root, backend=None)

        loaded = load_deployable_model(path)

        self.assertIsInstance(loaded, torch.jit.ScriptModule)
        self.assertFalse(loaded.training)
        self.assertEqual({p.device.type for p in loaded.parameters()}, {"cpu"})
        np.testing.assert_allclose(
            loaded(self.batch).detach().numpy(), self._expected(source), rtol=1e-5
        )

    def test_onnxruntime_backend_creates_a_session(self):
        """An onnxruntime package gives a session that reproduces the model."""
        path, source = _torch_package(self.root, backend="onnxruntime")

        session = load_deployable_model(path)

        name = session.get_inputs()[0].name
        (out,) = session.run(None, {name: self.batch.numpy()})
        np.testing.assert_allclose(out, self._expected(source), rtol=1e-5)

    def test_python_backend_with_a_torch_class_loads_the_module(self):
        """A python package of an nn.Module gives the module, on the CPU."""
        path, source = _torch_package(self.root, backend="python")

        loaded = load_deployable_model(path)

        self.assertIsInstance(loaded, SimpleModel)
        self.assertFalse(loaded.training)
        with torch.no_grad():
            np.testing.assert_allclose(
                loaded(self.batch).numpy(), self._expected(source), rtol=1e-5
            )

    def test_python_backend_with_a_custom_class_loads_the_custom_model(self):
        """A custom package gives a custom Model, decided by its class."""
        os.makedirs(os.path.join(self.root, "src"))
        path = CustomTritonPackager().create_model_package(
            os.path.join(self.root, "src"),
            model_class=_CUSTOM_CLASS,
            model_schema=ModelSchema(
                input_schema=[ModelSchemaItem("input", DataType.INT, [1])],
                output_schema=[ModelSchemaItem("response", DataType.INT, [1])],
            ),
            dest_model_path=os.path.join(self.root, "package"),
            include_import_prefixes=["michelangelo"],
        )

        loaded = load_deployable_model(path)

        self.assertIsInstance(loaded, Model)
        self.assertEqual(type(loaded).__name__, "Predict")

    def test_missing_path_is_reported(self):
        """A path that does not exist is a FileNotFoundError."""
        with self.assertRaisesRegex(FileNotFoundError, "Model path not found"):
            load_deployable_model(os.path.join(self.root, "nope"))

    def test_missing_config_pbtxt_is_not_a_deployable_package(self):
        """A directory without config.pbtxt is a ValueError."""
        with self.assertRaisesRegex(ValueError, "Missing config.pbtxt"):
            load_deployable_model(self.root)

    def test_unknown_backend_is_rejected(self):
        """A backend that is not a TritonBackendType is a ValueError."""
        self._write_config('backend: "tensorrt"\n')
        with self.assertRaisesRegex(ValueError, "Invalid backend type 'tensorrt'"):
            load_deployable_model(self.root)

    def test_supported_backend_without_a_loader_is_not_implemented(self):
        """A supported backend that has no loader is NotImplementedError."""
        self._write_config('backend: "pytorch"\n')
        with (
            patch(
                "michelangelo.lib.model_manager.serde.model.deployable_model."
                "get_deployable_backend_type",
                return_value="tensorrt",
            ),
            self.assertRaisesRegex(NotImplementedError, "tensorrt"),
        ):
            load_deployable_model(self.root)

    def test_missing_model_files_are_reported(self):
        """Each backend names the file it expected under 0/."""
        for backend, message in (
            ("pytorch", "0/model.pt"),
            ("onnxruntime", "0/model.onnx"),
        ):
            self._write_config(f'backend: "{backend}"\n')
            with self.assertRaisesRegex(FileNotFoundError, message):
                load_deployable_model(self.root)

    def test_unreadable_torchscript_is_a_runtime_error(self):
        """A model.pt that is not TorchScript is wrapped in a RuntimeError."""
        self._write_config('backend: "pytorch"\n')
        os.makedirs(os.path.join(self.root, "0"))
        with open(os.path.join(self.root, "0", "model.pt"), "w") as f:
            f.write("not a torchscript archive")
        with self.assertRaisesRegex(RuntimeError, "Failed to load TorchScript"):
            load_deployable_model(self.root)

    def test_python_package_needs_a_model_class(self):
        """A missing or empty model_class.txt is a ValueError."""
        self._write_config('backend: "python"\n')
        os.makedirs(os.path.join(self.root, "0"))
        with self.assertRaisesRegex(ValueError, "Missing model_class.txt"):
            load_deployable_model(self.root)
        with open(os.path.join(self.root, "0", "model_class.txt"), "w") as f:
            f.write("  \n")
        with self.assertRaisesRegex(ValueError, "model_class.txt is empty"):
            load_deployable_model(self.root)

    def _write_config(self, content: str) -> None:
        with open(os.path.join(self.root, "config.pbtxt"), "w") as f:
            f.write(content)


class OnnxProvidersTest(TestCase):
    """The CUDA provider is requested only when a GPU is present."""

    def _providers(self, available: list[str], cuda: bool) -> list[str]:
        import tempfile

        with tempfile.TemporaryDirectory() as root:
            os.makedirs(os.path.join(root, "0"))
            open(os.path.join(root, "0", "model.onnx"), "wb").close()
            with open(os.path.join(root, "config.pbtxt"), "w") as f:
                f.write('backend: "onnxruntime"\n')
            with (
                patch("onnxruntime.get_available_providers", return_value=available),
                patch("onnxruntime.InferenceSession") as session,
                patch("torch.cuda.is_available", return_value=cuda),
            ):
                load_deployable_model(root)
        return session.call_args.kwargs["providers"]

    def test_gpu_build_on_a_cpu_machine_uses_only_cpu(self):
        """A compiled-in CUDA provider is not requested without a GPU."""
        self.assertEqual(
            self._providers(["CUDAExecutionProvider", "CPUExecutionProvider"], False),
            ["CPUExecutionProvider"],
        )

    def test_gpu_present_requests_cuda_first(self):
        """With a GPU and the provider, CUDA is tried before the CPU."""
        self.assertEqual(
            self._providers(["CUDAExecutionProvider", "CPUExecutionProvider"], True),
            ["CUDAExecutionProvider", "CPUExecutionProvider"],
        )

    def test_cpu_build_with_a_gpu_uses_only_cpu(self):
        """Without the CUDA provider, a GPU does not matter."""
        self.assertEqual(
            self._providers(["CPUExecutionProvider"], True), ["CPUExecutionProvider"]
        )

    def test_no_torch_means_no_cuda(self):
        """Without PyTorch installed, the session runs on the CPU."""
        import builtins

        real_import = builtins.__import__

        def no_torch(name, *args, **kwargs):
            if name == "torch":
                raise ImportError(name)
            return real_import(name, *args, **kwargs)

        from michelangelo.lib.model_manager._private.serde.model import (
            onnx_deployable_model,
        )

        with patch("builtins.__import__", side_effect=no_torch):
            self.assertFalse(onnx_deployable_model._cuda_available())
