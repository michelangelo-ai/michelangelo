import tempfile
from typing import Optional
from uber.ai.michelangelo.sdk.model_manager.constants import StorageType
from uber.ai.michelangelo.sdk.model_manager._private.constants import Placeholder
from uber.ai.michelangelo.sdk.model_manager._private.packager.template_renderer import TritonTemplateRenderer
from uber.ai.michelangelo.sdk.model_manager._private.packager.llm_triton import (
    generate_model_package_content,
    generate_raw_model_package_content,
)
from uber.ai.michelangelo.sdk.model_manager._private.packager.common import (
    generate_model_package_folder,
)


class LLMTritonPackager:
    def __init__(self):
        self.gen = TritonTemplateRenderer()

    def create_model_package(
        self,
        model_path: str,
        model_name: Optional[str] = None,
        dest_model_path: Optional[str] = None,
        model_revision: Optional[str] = "0",
        pretrained_model_name: Optional[str] = None,
        model_def_script: Optional[str] = None,
        model_path_source_type: Optional[str] = StorageType.TERRABLOB,
    ) -> str:
        """
        Create a Triton model package for LLM model

        Args:
            model_path: the path of the raw model
            model_name: the name of model in MA Studio
                If not specified, a dummy model name will be created
            dest_model_path: the path to save the model package
                If not specified, a temporary directory will be created
            model_revision: the revision of model in MA Studio
            pretrained_model_name: the model id of a pretrained model hosted
                inside a model repo on huggingface
            model_def_script: the model definition script option to allow
                user to define custom user_model.py file.
                This option is to be deprecated in the future in favor of
                the PythonTritonPackager.
            model_path_source_type: the source type of the model path,
                e.g. 'hdfs', 'terrablob', default is 'terrablob'

        Returns:
            The path of the model package
        """
        if not dest_model_path:
            dest_model_path = tempfile.mkdtemp()

        if not model_name:
            model_name = Placeholder.MODEL_NAME

        content = generate_model_package_content(
            self.gen,
            model_path,
            model_name,
            model_revision,
            pretrained_model_name,
            model_def_script,
            model_path_source_type,
        )

        generate_model_package_folder(content, dest_model_path)

        return dest_model_path

    def create_raw_model_package(
        self,
        model_path: str,
        model_path_source_type: Optional[str] = StorageType.TERRABLOB,
        dest_model_path: Optional[str] = None,
    ) -> str:
        """
        Generate a raw model package for LLM model

        Args:
            model_path: the path of the raw model
            model_path_source_type: the source type of the model path,
                e.g. 'hdfs', 'terrablob', default is 'terrablob'
            dest_model_path: the path to save the model package
                If not specified, a temporary directory will be created

        Returns:
            The path of the raw model package
        """
        if not dest_model_path:
            dest_model_path = tempfile.mkdtemp()

        content = generate_raw_model_package_content(
            model_path,
            model_path_source_type,
            dest_model_path,
        )

        generate_model_package_folder(content, dest_model_path)

        return dest_model_path
