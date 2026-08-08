"""Config pbtxt generation module."""

from typing import Optional

from michelangelo.lib.model_manager._private.packager.template_renderer import (
    TritonTemplateRenderer,
)
from michelangelo.lib.model_manager.constants.triton_backend_type import (
    TritonBackendType,
)


def generate_config_pbtxt_content(
    gen: TritonTemplateRenderer,
    model_name: str,
    model_revision: str,
    input_schema: dict[str, dict[str, str]],
    output_schema: dict[str, dict[str, str]],
    triton_parameters: Optional[dict[str, str]] = None,
) -> str:
    """Generate the config.pbtxt file content.

    Args:
        gen: The TritonTemplateRenderer instance
        model_name: the name of model in MA Studio
        model_revision: the revision of model in MA Studio
        input_schema: the input schema of the model
        output_schema: the output schema of the model
        triton_parameters: Optional custom Triton model config parameters to
            include in config.pbtxt. Each key-value pair becomes a
            ``parameters`` block in the generated config, e.g.
            ``{"MY_CUSTOM_PARAM": "16"}``.

    Returns:
        The config.pbtxt file content
    """
    if (
        model_name is not None
        and model_name != ""
        and model_revision is not None
        and model_revision != ""
    ):
        model_name = f"{model_name}-{model_revision}"

    return gen.render(
        "config.pbtxt.tmpl",
        {
            "model_name": model_name,
            "backend": TritonBackendType.PYTHON,
            "max_batch_size": 256,
            "enable_dynamic_batching": True,
            "preferred_batch_size": 10,
            "max_queue_delay_microseconds": 300,
            "instance_count": 1,
            "inputs": input_schema,
            "outputs": output_schema,
            "parameters": triton_parameters or {},
        },
    )
