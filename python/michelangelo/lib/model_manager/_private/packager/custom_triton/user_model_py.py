from typing import Optional
from uber.ai.michelangelo.sdk.model_manager._private.packager.template_renderer import TritonTemplateRenderer


def generate_user_model_content(
    gen: TritonTemplateRenderer,
    process_batch: Optional[bool] = False,
) -> str:
    """Generate the user_model.py content

    Args:
        gen: The TritonTemplateRenderer instance
        process_batch: Indicate whether to automatically process batched inputs in user_model.py

    Returns:
        The user_model.py file content
    """
    return gen.render("custom_python/user_model.py.tmpl", {"process_batch": process_batch})
