"""Generate the user_model.py file content for the torch python backend."""

from __future__ import annotations

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from michelangelo.lib.model_manager._private.packager.template_renderer import (
        TritonTemplateRenderer,
    )


def generate_torch_python_user_model_content(
    gen: TritonTemplateRenderer,
    output_names: list[str],
) -> str:
    """Generate the user_model.py content for the torch python backend.

    Args:
        gen: The TritonTemplateRenderer instance.
        output_names: Output field names from the model schema, used to map the
            model's forward outputs back to named Triton tensors.

    Returns:
        The user_model.py file content.
    """
    return gen.render("torch_python/user_model.py.tmpl", {"output_names": output_names})
