import tempfile
from typing import Optional, Union
from numpy import ndarray
from michelangelo.lib.model_manager.schema import ModelSchema
from michelangelo.lib.model_manager.constants import StorageType
from michelangelo.lib.model_manager._private.packager.template_renderer import TritonTemplateRenderer


class CustomTritonPackager:
    def __init__(self, custom_batch_processing: Optional[bool] = False):
        """
        Create a CustomTritonPackager instance
        A packager for custom Triton Python models

        Args:
            custom_batch_processing (Optional):
                If to handle batch manually in the Triton model package.
                Default is False. If set to True, the user is responsible for handling batch in the model class,
                and the model input/output will have an additional batch dimension on top of the existing model schema.
                For example, the schema shape [n, ..., m], the input dimension will be [batch_size, n, ..., m].
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
        model_path_source_type: Optional[str] = StorageType.HDFS,
        include_import_prefixes: Optional[list[str]] = None,
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
                model package.
                and if the list is empty, save all imports

        Returns:
            The path of the model package
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
                model package.
                and if the list is empty, save all imports

        Returns:
            The path of the raw model package
        """