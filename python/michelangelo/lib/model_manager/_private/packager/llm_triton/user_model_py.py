from uber.ai.michelangelo.sdk.model_manager._private.constants import LLMModelType
from uber.ai.michelangelo.sdk.model_manager._private.packager.template_renderer import TritonTemplateRenderer


def generate_user_model_content(
    gen: TritonTemplateRenderer,
    llm_model_type: LLMModelType,
    model_def_script: str,
) -> str:
    """
    Generate the user_model.py content

    Args:
        gen: The TritonTemplateRenderer instance
        llm_model_type: the type of the LLM model, e.g. LLAMA2, T5
        model_def_script: can be one of the following:
            1) the path of the user_model.py in the repo
            2) default/python - the default user_model.py for python models
            3) default/hf - the default user_model.py file for HuggingFace models
            4) default/vllm - the default user_model.py for vLLM models

            If not specified, defaults to default/vllm
            This option is to be deprecated in the future

    Returns:
        The user_model.py file content
    """
    if model_def_script and model_def_script not in {
        "default/hf",
        "default/python",
        "default/vllm",
    }:
        with open(model_def_script) as file:
            content = file.read()
        return content

    if llm_model_type == LLMModelType.T5:
        return gen.render("t5/user_model.py.tmpl")

    if model_def_script == "default/hf":
        return gen.render("hf/user_model.py.tmpl")
    elif model_def_script == "default/python":
        return gen.render("python/user_model.py.tmpl")
    elif model_def_script == "default/vllm":
        return gen.render("vllm/user_model.py.tmpl")
    else:
        return gen.render("vllm/user_model.py.tmpl")
