import yaml
from michelangelo.lib.model_manager._private.packager.template_renderer import TritonTemplateRenderer
from michelangelo.lib.model_manager._private.packager.common import generate_download_yaml_content


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
        "config.pbtxt": gen.render(
            "fireworks/config.pbtxt.tmpl",
            {
                model_name: f"{model_name}-{model_revision}",
            },
        ),
        "0": {
            "model.json": gen.render("fireworks/model.json.tmpl"),
            "download.yaml": download_yaml,
        },
    }

    return content
