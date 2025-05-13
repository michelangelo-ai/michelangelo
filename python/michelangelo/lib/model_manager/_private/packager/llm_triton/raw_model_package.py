import os
import tempfile
from typing import Optional
from michelangelo.lib.model_manager._private.packager.common import download_model
from michelangelo.lib.model_manager._private.packager.llm_triton.type_yaml import generate_type_yaml


def generate_raw_model_package_content(
    model_path: str,
    model_path_source_type: Optional[str] = None,
    root_path: Optional[str] = None,
) -> dict:
    """
    Generate the raw model package content

    Args:
        model_path: the model dir path in Terrablob
        model_path_source_type: the source type of the model path,
            e.g. 'hdfs', 'terrablob'
        pretrained_model_name: the model id of a pretrained model
            hosted inside a model repo on huggingface
        root_path: the root path for temp files to be stored,
            if not specified, use a temp dir

    Returns:
        The raw model package content
    """
    if not root_path:
        root_path = tempfile.mkdtemp()

    target_model_path = os.path.join(root_path, "model")

    download_model(model_path, target_model_path, model_path_source_type)

    content = {
        "metadata": {
            "type.yaml": generate_type_yaml(),
        },
        "model": f"dir://{target_model_path}",
    }

    return content
