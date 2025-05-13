import yaml
from uber.ai.michelangelo.sdk.model_manager._private.packager.template_renderer import TritonTemplateRenderer
from uber.ai.michelangelo.sdk.model_manager._private.packager.common import generate_download_yaml_content


def generate_model_package_content(
    gen: TritonTemplateRenderer,
    model_path: str,
    model_name: str,
    model_revision: str,
) -> dict:
    """
    Generate the model package content

    Args:
        gen: The template renderer
        model_path: The model path in terrablob
        model_name: The model name
        model_revision: The model revision

    Returns:
        The model package content
    """
    download_yaml = yaml.dump(
        generate_download_yaml_content(model_path),
        sort_keys=False,
    )

    content = {
        "ensemble": {
            "config.pbtxt": gen.render(
                "online-trt/ensemble/config.pbtxt.tmpl",
                {
                    model_name: f"{model_name}-{model_revision}",
                },
            ),
            "1": {},
        },
        "postprocessing": {
            "1": {
                "model.py": gen.render("online-trt/postprocessing/model.py.tmpl"),
            },
            "config.pbtxt": gen.render("online-trt/postprocessing/config.pbtxt.tmpl"),
        },
        "preprocessing": {
            "1": {
                "model.py": gen.render("online-trt/preprocessing/model.py.tmpl"),
            },
            "config.pbtxt": gen.render("online-trt/preprocessing/config.pbtxt.tmpl"),
        },
        "tensorrt_llm": {
            "config.pbtxt": gen.render("online-trt/tensorrt_llm/config.pbtxt.tmpl"),
            "1": {},
        },
        "download.yaml": download_yaml,
    }

    return content
