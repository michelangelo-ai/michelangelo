import tempfile
from typing import Optional
from michelangelo.lib.model_manager._private.constants import Placeholder
from michelangelo.lib.model_manager._private.packager.template_renderer import TritonTemplateRenderer
from michelangelo.lib.model_manager._private.packager.common import generate_model_package_folder
from michelangelo.lib.model_manager._private.packager.llm_triton_fireworks import generate_model_package_content


class LLMTritonFireworksPackager:
    def __init__(self):
        self.gen = TritonTemplateRenderer()

    def create_model_package(
        self,
        model_path: str,
        model_name: Optional[str] = None,
        dest_model_path: Optional[str] = None,
        model_revision: Optional[str] = "0",
    ) -> str:
        """
        Create a Triton model package for LLM Online TensorRT model
        It is an ensemble model with preprocessing, postprocessing and TensorRT LLM
        The conversion to TensorRT LLM is done online inside the preprocessor

        Args:
            model_path: The model path in terrablob
            model_name: The model name
                If not specified, a dummy model name will be created
            dest_model_path: The destination model path
                If not specified, a temporary directory will be created
            model_revision: The model revision

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
        )

        generate_model_package_folder(content, dest_model_path)

        return dest_model_path
