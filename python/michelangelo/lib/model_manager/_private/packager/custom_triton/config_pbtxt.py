"""Config pbtxt generation module."""

from michelangelo.lib.model_manager._private.constants.triton_backend_type import (
    TritonBackendType,
)
from michelangelo.lib.model_manager._private.packager.template_renderer import (
    TritonTemplateRenderer,
)


def generate_config_pbtxt_content(
    gen: TritonTemplateRenderer,
    model_name: str,
    model_revision: str,
    input_schema: dict[str, dict[str, str]],
    output_schema: dict[str, dict[str, str]],
) -> str:
    """Generate the config.pbtxt file content.

    Args:
        gen: The TritonTemplateRenderer instance
        model_name: the name of model in MA Studio
        model_revision: the revision of model in MA Studio
        input_schema: the input schema of the model
        output_schema: the output schema of the model

    Returns:
        The config.pbtxt file content
    """
    if model_revision:
        model_name = f"{model_name}-{model_revision}"

    return gen.render(
        "config.pbtxt.tmpl",
        {
            "model_name": f"{model_name}",
            "backend": TritonBackendType.PYTHON,
            "max_batch_size": 256,
            "enable_dynamic_batching": True,
            "preferred_batch_size": 10,
            "max_queue_delay_microseconds": 300,
            "instance_count": 1,
            "inputs": input_schema,
            "outputs": output_schema,
        },
    )
