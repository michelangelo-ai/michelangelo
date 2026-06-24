"""Generate the config.pbtxt file content for torch-based Triton backends."""

from __future__ import annotations

from typing import TYPE_CHECKING

from michelangelo.lib.model_manager._private.constants.triton_backend_type import (
    TritonBackendType,
)
from michelangelo.lib.model_manager._private.schema.triton import convert_model_schema

if TYPE_CHECKING:
    from michelangelo.lib.model_manager._private.packager.template_renderer import (
        TritonTemplateRenderer,
    )
    from michelangelo.lib.model_manager.schema import ModelSchema


def generate_config_pbtxt_content(
    gen: TritonTemplateRenderer,
    model_name: str,
    model_revision: str | None,
    model_schema: ModelSchema,
    backend: str = TritonBackendType.TORCH,
    enable_dynamic_batching: bool = True,
) -> str:
    """Generate the config.pbtxt file content for a torch-based backend.

    Supports the pytorch, python, and onnxruntime backends.

    Note: The Triton ``pytorch`` (libtorch) backend matches TorchScript forward
    arguments positionally, not by name. Input names in the generated
    config.pbtxt come from the model schema and are informational for the
    ``python`` and ``onnxruntime`` backends. For the ``pytorch`` backend,
    ensure your TorchScript forward signature argument order matches the schema
    input order — Triton binds inputs positionally for this backend.

    Args:
        gen: The TritonTemplateRenderer instance.
        model_name: The name of the model.
        model_revision: An optional revision string appended to the model name
            as ``<name>-<revision>`` in the config.
        model_schema: The model schema, converted to the Triton I/O layout
            internally.
        backend: The Triton backend to use for the model.
        enable_dynamic_batching: If True, enables dynamic batching with
            max_batch_size=256. If False, sets max_batch_size=0 and disables
            dynamic batching (required for models with List[str] inputs).

    Returns:
        The config.pbtxt file content.
    """
    input_schema, output_schema = convert_model_schema(model_schema)

    if model_revision:
        model_name = f"{model_name}-{model_revision}"

    template_vars = {
        "model_name": model_name,
        "backend": backend,
        "max_batch_size": 256 if enable_dynamic_batching else 0,
        "enable_dynamic_batching": enable_dynamic_batching,
        "instance_count": 1,
        "inputs": input_schema,
        "outputs": output_schema,
    }

    if enable_dynamic_batching:
        template_vars["max_queue_delay_microseconds"] = 300

    return gen.render("config.pbtxt.tmpl", template_vars)
