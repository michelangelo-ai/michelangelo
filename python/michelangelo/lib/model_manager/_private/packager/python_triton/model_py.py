from michelangelo.lib.model_manager._private.packager.template_renderer import TritonTemplateRenderer


def generate_model_py_content(
    gen: TritonTemplateRenderer,
) -> str:
    """
    Generate the model.py file content

    Args:
        gen: The TritonTemplateRenderer instance

    Returns:
        The model.py file content
    """
    return gen.render("model.py.tmpl")
