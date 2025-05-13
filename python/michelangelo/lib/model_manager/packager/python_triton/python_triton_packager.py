import tempfile
from typing import Optional, Union
from numpy import ndarray
from michelangelo.lib.model_manager.constants import StorageType
from michelangelo.lib.model_manager.schema import ModelSchema
from michelangelo.lib.model_manager._private.constants import Placeholder
from michelangelo.lib.model_manager._private.packager.template_renderer import TritonTemplateRenderer
from michelangelo.lib.model_manager._private.packager.python_triton import (
    generate_model_package_content,
    generate_raw_model_package_content,
    validate_model_class,
    validate_raw_model_package,
)
from michelangelo.lib.model_manager._private.packager.common import generate_model_package_folder
from michelangelo.lib.model_manager._private.schema.triton import validate_model_schema, convert_model_schema
from michelangelo.lib.model_manager._private.utils.data_utils import (
    validate_sample_data,
    validate_sample_data_with_model_schema,
)


class PythonTritonPackager:
    def __init__(self, custom_batch_processing: Optional[bool] = False):
        """
        Create a PythonTritonPackager instance

        Args:
            custom_batch_processing (Optional):
                If to handle batch manually in the Triton model package.
                Default is False. If set to True, the user is responsible for handling batch in the model class,
                and the model input/output will have an additional batch dimension on top of the existing model schema.
                For example, the schema shape [1] will be converted to [-1, 1].
        """
        self.gen = TritonTemplateRenderer()
        self.custom_batch_processing = custom_batch_processing

    def create_model_package(
        self,
        model_path: str,
        model_class: str,
        model_schema: ModelSchema,
        model_name: Optional[str] = None,
        dest_model_path: Optional[str] = None,
        model_revision: Optional[str] = "0",
        model_path_source_type: Optional[str] = StorageType.HDFS,
        include_import_prefixes: "Optional[list[str]]" = None,
    ) -> str:
        """
        Create a Triton model package for custom Python model

        Args:
            model_path: the path of the raw model
            model_class: the model class of the model
                that contains the custom predict function
            model_schema: the schema of the model
            model_name: the name of model in MA Studio
            dest_model_path: the path to save the model package
                If not specified, a temporary directory will be created
            model_revision: the revision of model in MA Studio
            model_path_source_type: the source type of the model path,
                e.g. 'hdfs', 'terrablob', default is 'hdfs'
            include_import_prefixes (Optional): only save the imported
                modules with the given prefixes in the model package,
                e.g. ['uber', 'data.michelangelo'] only imports starting
                with 'uber' or 'data.michelangelo' will be saved in the
                model package. Default is ['uber'],
                and if the list is empty, save all imports

        Returns:
            The path of the model package
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

        input_schema, output_schema = convert_model_schema(model_schema)

        if not model_name:
            model_name = Placeholder.MODEL_NAME

        if not dest_model_path:
            dest_model_path = tempfile.mkdtemp()

        if include_import_prefixes is None:
            include_import_prefixes = ["uber"]

        content = generate_model_package_content(
            self.gen,
            model_path,
            model_name,
            model_revision,
            model_class,
            input_schema,
            output_schema,
            model_path_source_type=model_path_source_type,
            root_path=dest_model_path,
            include_import_prefixes=include_import_prefixes,
            custom_batch_processing=self.custom_batch_processing,
        )

        generate_model_package_folder(content, dest_model_path)

        return dest_model_path

    def create_raw_model_package(
        self,
        model_path: str,
        model_class: str,
        model_schema: ModelSchema,
        sample_data: list[dict[str, ndarray]],
        dest_model_path: Optional[str] = None,
        model_path_source_type: Optional[str] = StorageType.HDFS,
        requirements: Optional[Union[list[str], str]] = None,
        include_import_prefixes: Optional[list[str]] = None,
    ) -> str:
        """
        Create a raw model package for custom Python model

        Args:
            model_path: the path of the raw model
            model_class: the model class of the model
                that contains the custom predict function
            model_schema: the schema of the model, which specifies the input/palette/output features
            sample_data: the sample data of the model. A list of input data for the predict function.
            dest_model_path: the path to save the model package
                If not specified, a temporary directory will be created
            model_path_source_type: the source type of the model path,
                e.g. 'hdfs', 'terrablob', default is 'hdfs'
            requirements: the requirements of the model, which can be one of the following:
                - a list of requirements
                - a path to the requirements.txt file
                If not specified, the requirements will not be included in the model package
            include_import_prefixes (Optional): only save the imported
                modules with the given prefixes in the model package,
                e.g. ['uber', 'data.michelangelo'] only imports starting
                with 'uber' or 'data.michelangelo' will be saved in the
                model package. Default is ['uber'],
                and if the list is empty, save all imports

        Returns:
            The path of the raw model package
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

        is_sample_data_with_schema_valid, error = validate_sample_data_with_model_schema(sample_data, model_schema, batch_inference)

        if not is_sample_data_with_schema_valid:
            raise error

        if not dest_model_path:
            dest_model_path = tempfile.mkdtemp()

        if include_import_prefixes is None:
            include_import_prefixes = ["uber"]

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

        generate_model_package_folder(content, dest_model_path)

        validate_raw_model_package(dest_model_path, sample_data, model_schema, batch_inference)

        return dest_model_path
