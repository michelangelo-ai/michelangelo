import yaml
from michelangelo.lib.model_manager._private.packager.template_renderer import TritonTemplateRenderer
from michelangelo.lib.model_manager._private.packager.llm_triton.config_pbtxt import generate_config_pbtxt_content
from michelangelo.lib.model_manager._private.packager.llm_triton.model_py import generate_model_py_content
from michelangelo.lib.model_manager._private.packager.llm_triton.user_model_py import generate_user_model_content
from michelangelo.lib.model_manager._private.packager.llm_triton.llm_model_type import infer_llm_model_type
from michelangelo.lib.model_manager._private.packager.common import generate_download_yaml_content


def generate_model_package_content(
    gen: TritonTemplateRenderer,
    model_path: str,
    model_name: str,
    model_revision: str,
    pretrained_model_name: str,
    model_def_script: str,
    model_path_source_type: str,
) -> dict:
    """
    Generate the model package content

    Args:
        gen: The TritonTemplateRenderer instance
        model_path: the model dir path in Terrablob
        model_name: the name of model in MA Studio
        model_revision: the revision of the model
        pretrained_model_name: the model id of a pretrained model
            hosted inside a model repo on huggingface
        model_def_script: the model definition script in triton
        model_path_source_type: the source type of the model path,
            e.g. 'hdfs', 'terrablob'

    Returns:
        The model package content
    """
    model_py = generate_model_py_content(gen)
    llm_model_type = infer_llm_model_type(model_path, pretrained_model_name, model_path_source_type)
    user_model_py = generate_user_model_content(gen, llm_model_type, model_def_script)
    config_pbtxt = generate_config_pbtxt_content(gen, model_name, model_revision)

    content = {
        "config.pbtxt": config_pbtxt,
        "0": {
            "model.py": model_py,
            "user_model.py": user_model_py,
        },
    }

    if model_path:
        download_yaml = generate_download_yaml_content(model_path, model_path_source_type)
        content["0"]["download.yaml"] = yaml.dump(download_yaml, sort_keys=False)

    return content
