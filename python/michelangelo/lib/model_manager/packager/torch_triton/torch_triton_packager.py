"""Packager for PyTorch Triton models."""

import tempfile
from typing import Optional, Union

from numpy import ndarray

from michelangelo._internal.utils.file_utils import generate_folder
from michelangelo.lib.model_manager._private.constants.triton_backend_type import (
    TritonBackendType,
)
from michelangelo.lib.model_manager._private.packager.template_renderer import (
    TritonTemplateRenderer,
)
from michelangelo.lib.model_manager._private.packager.torch_triton import (
    generate_model_package_content,
    generate_raw_model_package_content,
    validate_model_class,
    validate_raw_model_package,
)
from michelangelo.lib.model_manager._private.schema.triton import validate_model_schema
from michelangelo.lib.model_manager._private.utils.data_utils import (
    validate_sample_data,
    validate_sample_data_with_model_schema,
)
from michelangelo.lib.model_manager.constants import StorageType
from michelangelo.lib.model_manager.schema import ModelSchema

_SUPPORTED_BACKENDS = {
    TritonBackendType.TORCH,
    TritonBackendType.PYTHON,
    TritonBackendType.ONNX,
}


class TorchTritonPackager:
    """Packager for PyTorch models targeting NVIDIA Triton Inference Server.

    This class packages PyTorch models into formats suitable for deployment on
    Triton Inference Server. It generates the
    required configuration files, bundles dependencies, and organizes model
    artifacts according to Triton's directory structure.

    The packager supports two package types:
        1. Deployable packages: use a torchscript (``.pt``) file for direct
           Triton deployment.
        2. Raw packages: use a state_dict (``.pth`` / ``.pt``) file together
           with a custom Python model class.

    PyTorch packaging uses batch inference, meaning the model receives inputs
    with a batch dimension added to the schema-defined shape. For example, a
    schema shape ``[20]`` becomes input shape ``[batch_size, 20]`` during
    inference, while the schema itself remains ``[20]``.

    Attributes:
        gen: The template renderer used to generate Triton configuration files.
    """

    def __init__(self) -> None:
        """Create a TorchTritonPackager instance."""
        self.gen = TritonTemplateRenderer()

    def create_model_package(
        self,
        model_path: str,
        model_schema: ModelSchema,
        model_name: Optional[str] = None,
        dest_model_path: Optional[str] = None,
        model_revision: Optional[str] = "0",
        model_path_source_type: Optional[str] = StorageType.LOCAL,
        sample_data: Optional[list[dict[str, ndarray]]] = None,
        model_class: Optional[str] = None,
        hyperparameters: Optional[dict] = None,
        enable_dynamic_batching: bool = True,
        backend: Optional[str] = None,
        include_import_prefixes: Optional[list[str]] = None,
    ) -> str:
        """Create a Triton model package for a PyTorch model.

        Args:
            model_path: The path to the model weights or graph. For the
                ``python`` backend this should be a state_dict file
                (``.pth`` / ``.pt``). For the ``onnxruntime`` backend, use a
                ``.onnx`` file or a PyTorch artifact (``.pt`` / ``.pth``) that
                can be validated like the PyTorch backend. Otherwise (the
                default PyTorch backend) use a torchscript or convertible
                ``.pt`` file.
            model_schema: The schema defining the model's input and output
                features, including their names, data types, and shapes.
            model_name: The name to use for the model in the Triton model repository.
                If not specified, a placeholder name is used.
            dest_model_path: The directory path where the model package should
                be saved. If not specified, a temporary directory will be
                created and its path returned.
            model_revision: The revision of the model. Defaults to ``"0"``.
            model_path_source_type: The storage backend type where the model
                artifacts are located. Should be a value from StorageType
                (e.g., StorageType.LOCAL). Defaults to StorageType.LOCAL.
            sample_data: Sample data for the model. A list of input data for
                the predict function. For the ``onnxruntime`` backend this is
                optional when ``model_path`` is already a valid ``.onnx``
                file; otherwise it is required for PyTorch-to-ONNX tracing
                (the first batch is used).
            model_class: The fully qualified class name of the model
                implementation (a ``torch.nn.Module`` subclass), e.g.,
                ``'my_package.my_module.MyModel'``. Required when ``backend``
                is ``'python'``.
            hyperparameters: The hyperparameters for instantiating the model
                class. If not specified, the model class is instantiated
                without parameters.
            enable_dynamic_batching: If True (default), enables dynamic
                batching with max_batch_size=256. If False, sets
                max_batch_size=0 and disables dynamic batching. Set to False
                for models with ``list[str]`` inputs, as required by Triton's
                PyTorch backend.
            backend: The Triton backend type (``'pytorch'``, ``'python'``, or
                ``'onnxruntime'``). Use ``'python'`` to package ``nn.Module``
                models that cannot be converted to TorchScript. Use
                ``'onnxruntime'`` to package ONNX models (``model.onnx``) or to
                export PyTorch artifacts to ONNX. If None, the default PyTorch
                backend is used.
            include_import_prefixes: A list of module prefixes to include when
                bundling dependencies. Only imported modules whose names start
                with one of these prefixes will be included in the package. If
                None (default) or empty, all imported modules are included.

        Example::

            packager = TorchTritonPackager()
            package_path = packager.create_model_package(
                model_path="model.pt",
                model_schema=ModelSchema(
                    input_schema=[
                        ModelSchemaItem(name="x", data_type=DataType.FLOAT, shape=[10]),
                    ],
                    output_schema=[
                        ModelSchemaItem(name="y", data_type=DataType.FLOAT, shape=[1]),
                    ],
                ),
            )

        Returns:
            The absolute path to the generated model package directory.

        Raises:
            ValueError: If ``model_path`` or ``model_schema`` is missing, if
                ``backend`` is not supported, or if ``model_class`` is missing
                for the Python backend.
        """
        if not model_path:
            raise ValueError(
                "model_path is required: provide a path to a .pt, .pth, or .onnx file"
            )

        if not model_schema:
            raise ValueError("model_schema is required")

        if backend is not None and backend not in _SUPPORTED_BACKENDS:
            raise ValueError(
                f"Unsupported backend: '{backend}'. Supported backends are: "
                f"{sorted(_SUPPORTED_BACKENDS)}"
            )

        is_schema_valid, error = validate_model_schema(model_schema)

        if not is_schema_valid:
            raise error

        if not model_name:
            model_name = "$MODEL_NAME"

        if not dest_model_path:
            dest_model_path = tempfile.mkdtemp()

        if backend == TritonBackendType.PYTHON:
            if not model_class:
                raise ValueError("model_class is required for Python backend")

            is_model_class_valid, error = validate_model_class(model_class)
            if not is_model_class_valid:
                raise error

        content = generate_model_package_content(
            self.gen,
            model_path,
            model_name,
            model_revision,
            model_schema,
            model_path_source_type=model_path_source_type,
            root_path=dest_model_path,
            enable_dynamic_batching=enable_dynamic_batching,
            model_class=model_class,
            hyperparameters=hyperparameters,
            backend=backend,
            include_import_prefixes=include_import_prefixes,
            sample_data=sample_data,
        )

        generate_folder(content, dest_model_path)

        return dest_model_path

    def create_raw_model_package(
        self,
        model_path: str,
        model_class: str,
        model_schema: ModelSchema,
        sample_data: Optional[list[dict[str, ndarray]]] = None,
        dest_model_path: Optional[str] = None,
        model_path_source_type: Optional[str] = StorageType.LOCAL,
        requirements: Optional[Union[list[str], str]] = None,
        include_import_prefixes: Optional[list[str]] = None,
        hyperparameters: Optional[dict] = None,
        transform_spec: Optional[dict] = None,
        transform_feature_stats: Optional[dict] = None,
    ) -> str:
        """Create a raw model package for a PyTorch model.

        Args:
            model_path: The path to the PyTorch model file (``.pth`` / ``.pt``
                state_dict or full model).
            model_class: The fully qualified class name of the model
                implementation (a ``torch.nn.Module`` subclass) that contains
                the forward function and any custom logic.
            model_schema: The schema defining the model's input, palette
                (feature store), and output features.
            sample_data: An optional list of sample inputs for testing the model's
                forward function. Each item should be a dictionary mapping
                input feature names to numpy arrays. If not provided, model
                validation with sample data is skipped.
            dest_model_path: The directory path where the model package should
                be saved. If not specified, a temporary directory will be
                created and its path returned.
            model_path_source_type: The storage backend type where the model
                artifacts are located. Should be a value from StorageType
                (e.g., StorageType.LOCAL). Defaults to StorageType.LOCAL.
            requirements: The Python package dependencies required by the
                model. This can be either:
                    - A list of requirement strings (e.g.,
                      ``['numpy>=1.20.0']``)
                    - A path to a requirements.txt file
                If not specified, requirements are not included in the package.
            include_import_prefixes: A list of module prefixes to include when
                bundling dependencies. Only imported modules whose names start
                with one of these prefixes will be included in the package. If
                None (default) or empty, all imported modules are included.
            hyperparameters: The hyperparameters for instantiating the model
                class. If not specified, the model class is instantiated
                without parameters. These parameters are stored in
                ``metadata/hyperparameters.yaml``.
            transform_spec: An optional transform specification stored with the
                model package.
            transform_feature_stats: Optional feature statistics used by the
                transform, stored with the model package.

        Example::

            import numpy as np

            packager = TorchTritonPackager()
            package_path = packager.create_raw_model_package(
                model_path="model.pt",
                model_class="my_package.models.MyModel",
                model_schema=ModelSchema(
                    input_schema=[
                        ModelSchemaItem(name="x", data_type=DataType.FLOAT, shape=[10]),
                    ],
                    output_schema=[
                        ModelSchemaItem(name="y", data_type=DataType.FLOAT, shape=[1]),
                    ],
                ),
                sample_data=[{"x": np.random.randn(10).astype(np.float32)}],
            )

        Returns:
            The absolute path to the generated raw model package directory.

        Raises:
            ValueError: If ``model_path``, ``model_class``, or ``model_schema``
                is missing.
        """
        if not model_path:
            raise ValueError(
                "model_path is required: provide a path to a .pt or .pth model file"
            )

        if not model_class:
            raise ValueError("model_class is required")

        is_model_class_valid, error = validate_model_class(model_class)

        if not is_model_class_valid:
            raise error

        if not model_schema:
            raise ValueError("model_schema is required")

        is_schema_valid, error = validate_model_schema(model_schema)

        if not is_schema_valid:
            raise error

        # Sample data validation is optional - only validate if provided
        if sample_data:
            is_sample_data_valid, error = validate_sample_data(sample_data)

            if not is_sample_data_valid:
                raise error

            (
                is_sample_data_with_schema_valid,
                error,
            ) = validate_sample_data_with_model_schema(sample_data, model_schema)

            if not is_sample_data_with_schema_valid:
                raise error

        if not dest_model_path:
            dest_model_path = tempfile.mkdtemp()

        content = generate_raw_model_package_content(
            model_path,
            model_class,
            model_schema,
            sample_data,
            model_path_source_type=model_path_source_type,
            requirements=requirements,
            root_path=dest_model_path,
            include_import_prefixes=include_import_prefixes,
            hyperparameters=hyperparameters,
            transform_spec=transform_spec,
            transform_feature_stats=transform_feature_stats,
        )

        generate_folder(content, dest_model_path)

        validate_raw_model_package(dest_model_path, sample_data, model_schema)

        return dest_model_path
