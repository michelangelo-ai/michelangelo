"""Packager for custom Triton models."""

from typing import Optional, Union
from numpy import ndarray
from michelangelo.lib.model_manager.constants import StorageType
from michelangelo.lib.model_manager.schema import ModelSchema
from michelangelo._internal.utils.file_utils import generate_folder
from michelangelo.lib.model_manager._private.packager.template_renderer import (
    TritonTemplateRenderer,
)
from michelangelo.lib.model_manager._private.packager.custom_triton import (
    generate_raw_model_package_content,
    validate_model_class,
)
from michelangelo.lib.model_manager._private.schema.triton import (
    validate_model_schema,
    convert_model_schema,
)
from michelangelo.lib.model_manager._private.utils.data_utils import (
    validate_sample_data,
    validate_sample_data_with_model_schema,
)


class CustomTritonPackager:
    """Packager for custom Triton Python models.

    This class provides utilities to package custom Python models that implement
    the Model interface into formats suitable for deployment with NVIDIA Triton
    Inference Server. It handles the generation of required configuration files,
    dependency management, and model artifact organization.

    The packager supports two main workflows:
    1. Creating Triton model packages for Michelangelo Studio deployment
    2. Creating raw model packages with sample data for testing and validation

    Attributes:
        gen: The template renderer used to generate Triton configuration files.
        custom_batch_processing: Whether batch processing is handled manually by
            the model implementation.
    """

    def __init__(self, custom_batch_processing: Optional[bool] = False):
        """Create a CustomTritonPackager instance.

        Args:
            custom_batch_processing: Whether to handle batching manually in the
                model implementation. Defaults to False.

                If False (default), Triton automatically handles batching and
                the model's predict method receives individual samples with
                shapes matching the model schema exactly.

                If True, the model implementation is responsible for handling
                batches, and the predict method will receive inputs with an
                additional leading batch dimension. For example, if the schema
                specifies shape [n, ..., m], the actual input shape will be
                [batch_size, n, ..., m].
        """
        self.gen = TritonTemplateRenderer()
        self.custom_batch_processing = custom_batch_processing

    def generate_model_package(
        self,
        model_path: str,
        model_class: str,
        model_schema: ModelSchema,
        model_name: Optional[str] = None,
        dest_model_path: Optional[str] = None,
        model_revision: Optional[str] = "0",
        model_path_source_type: Optional[str] = StorageType.LOCAL,
        include_import_prefixes: Optional[list[str]] = None,
    ) -> str:
        """Create a Triton model package for deployment to Michelangelo Studio.

        This method packages a custom Python model into a format suitable for
        deployment on Triton Inference Server through Michelangelo Studio. It
        generates the necessary configuration files, bundles dependencies, and
        organizes model artifacts according to Triton's directory structure.

        Args:
            model_path: The path to the saved model artifacts. This should be
                the directory containing the model files created by the Model's
                save() method.
            model_class: The fully qualified class name of the model
                implementation (e.g., 'mypackage.models.MyModel'). This class
                must implement the Model interface with save, load, and predict
                methods.
            model_schema: The schema defining the model's input and output
                features, including their names, data types, and shapes.
            model_name: The name to use for the model in Michelangelo Studio.
                If not specified, a name will be derived from the model class.
            dest_model_path: The directory path where the model package should
                be saved. If not specified, a temporary directory will be
                created and its path returned.
            model_revision: The revision number for the model in Michelangelo
                Studio. Defaults to "0".
            model_path_source_type: The storage backend type where the model
                artifacts are located. Should be a value from StorageType (e.g.,
                StorageType.LOCAL). Defaults to StorageType.LOCAL.
            include_import_prefixes: A list of module prefixes to include when
                bundling dependencies. Only imported modules whose names start
                with one of these prefixes will be included in the package. For
                example, ['uber', 'data.michelangelo'] will only include modules
                starting with 'uber' or 'data.michelangelo'. If None or empty,
                all imported modules will be included.

        Returns:
            The absolute path to the generated model package directory.
        """

    def create_raw_model_package(
        self,
        model_path: str,
        model_class: str,
        model_schema: ModelSchema,
        sample_data: list[dict[str, ndarray]],
        dest_model_path: Optional[str] = None,
        model_path_source_type: Optional[str] = StorageType.LOCAL,
        requirements: Optional[Union[list[str], str]] = None,
        include_import_prefixes: Optional[list[str]] = None,
    ) -> str:
        """Create a raw model package with sample data for testing.

        This method creates a self-contained model package that includes sample
        data for validation and testing. This is useful for verifying that the
        model package works correctly before deployment, or for creating
        shareable model artifacts for development and testing purposes.

        The raw model package includes:
        - Model artifacts and implementation code
        - Sample input data for testing predictions
        - Dependency specifications

        Args:
            model_path: The path to the saved model artifacts. This should be
                the directory containing the model files created by the Model's
                save() method.
            model_class: The fully qualified class name of the model
                implementation (e.g., 'mypackage.models.MyModel'). This class
                must implement the Model interface with save, load, and predict
                methods.
            model_schema: The schema defining the model's input, palette
                (feature store), and output features, including their names,
                data types, and shapes.
            sample_data: A list of sample inputs for testing the model's
                predict method. Each item should be a dictionary mapping input
                feature names to numpy arrays, matching the format expected by
                the model's predict method.
            dest_model_path: The directory path where the model package should
                be saved. If not specified, a temporary directory will be
                created and its path returned.
            model_path_source_type: The storage backend type where the model
                artifacts are located. Should be a value from StorageType (e.g.,
                StorageType.LOCAL). Defaults to StorageType.LOCAL.
            requirements: The Python package dependencies required by the model.
                This can be either:
                - A list of requirement strings (e.g., ['numpy>=1.20.0',
                  'scikit-learn==1.0.2'])
                - A path to a requirements.txt file
                If not specified, no additional requirements will be included in
                the package (only the model code and its imports will be
                bundled).
            include_import_prefixes: A list of module prefixes to include when
                bundling dependencies. Only imported modules whose names start
                with one of these prefixes will be included in the package. For
                example, ['uber', 'data.michelangelo'] will only include modules
                starting with 'uber' or 'data.michelangelo'. If None or empty,
                all imported modules will be included.

        Returns:
            The absolute path to the generated raw model package directory.
        """
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

        is_sample_data_valid, error = validate_sample_data(sample_data)

        if not is_sample_data_valid:
            raise error

        batch_inference = self.custom_batch_processing

        (
            is_sample_data_with_schema_valid, 
            error,
        ) = validate_sample_data_with_model_schema(
            sample_data, 
            model_schema, 
            batch_inference
        )

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
            batch_inference=batch_inference,
        )

        generate_folder(content, dest_model_path)

        return dest_model_path