from michelangelo.lib.model_manager._private.packager.template_renderer import TritonTemplateRenderer


def generate_config_pbtxt_content(
    gen: TritonTemplateRenderer,
    model_name: str,
    model_revision: str,
) -> str:
    """
    Generate the config.pbtxt file content

    Args:
        gen: The TritonTemplateRenderer instance
        model_name: the name of model in MA Studio
        model_revision: the revision of model in MA Studio

    Returns:
        The config.pbtxt file content
    """
    if model_revision:
        model_name = f"{model_name}-{model_revision}"

    return gen.render(
        "config.pbtxt.tmpl",
        {
            "model_name": f"{model_name}",
            "enable_dynamic_batching": True,
            "preferred_batch_size": 10,
            "max_queue_delay_microseconds": 300,
            "max_batch_size": 256,
            "instance_count": 1,
            "inputs": {
                "prompt": {
                    "data_type": "STRING",
                    "shape": "[ 1 ]",
                },
                "max_tokens": {
                    "data_type": "INT32",
                    "shape": "[ 1 ]",
                },
                "top_p": {
                    "data_type": "FP32",
                    "shape": "[ 1 ]",
                },
                "temperature": {
                    "data_type": "FP32",
                    "shape": "[ 1 ]",
                },
                "n": {
                    "data_type": "INT32",
                    "shape": "[ 1 ]",
                },
            },
            "outputs": {
                "text_json": {
                    "data_type": "STRING",
                    "shape": "[ 1 ]",
                },
                "response": {
                    "data_type": "STRING",
                    "shape": "[ 1 ]",
                },
            },
        },
    )
