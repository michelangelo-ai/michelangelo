"""Backend that scores a raw model.

A raw model is the trained model itself, not an exported serving bundle: a
PyTorch ``nn.Module`` (plain, Lightning, or a transform fused with a
predictor) or a custom ``Model`` from
:mod:`michelangelo.lib.model_manager.interface.custom_model`. The task scores
custom models' raw packages and model objects passed in memory with it.
"""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

from michelangelo.lib.inference._invoke import (
    DEFAULT_OUTPUT_NAME,
    call_module,
    name_outputs,
    predict_custom,
    read_batch_inference,
    schema_names,
    to_torch_inputs,
)

if TYPE_CHECKING:
    import numpy as np

    from michelangelo.lib.inference.backend import BackendContext
    from michelangelo.lib.model_manager.schema import ModelSchema

__all__ = ["DEFAULT_OUTPUT_NAME", "RawModelBackend", "create_raw_backend"]


class RawModelBackend:
    """Scores batches with a loaded raw model.

    Args:
        model: A PyTorch ``nn.Module``, or a custom ``Model`` whose ``predict``
            takes and returns numpy dicts.
        schema: The model schema. Provides the input and output names.
        device: Device for a PyTorch model. Ignored for custom models.
        batch_inference: For a custom ``Model``, whether ``predict`` takes the
            whole batch. By default it is called once per record, which is how a
            custom model is packaged unless it opts into batching. Ignored for
            PyTorch models.

    Raises:
        TypeError: If ``model`` is neither a custom ``Model`` nor an ``nn.Module``.
    """

    def __init__(
        self,
        model: Any,
        schema: ModelSchema | None = None,
        device: str = "cpu",
        batch_inference: bool = False,
    ) -> None:
        """Wrap ``model``, moving a PyTorch module to ``device``."""
        from michelangelo.lib.model_manager.interface.custom_model import Model

        self._custom = isinstance(model, Model)
        if not self._custom:
            import torch

            if not isinstance(model, torch.nn.Module):
                raise TypeError(
                    "RawModelBackend needs a custom Model or a torch.nn.Module, "
                    f"got {type(model).__name__}."
                )
            model = model.to(device).eval()
        self._model = model
        self._schema = schema
        self._device = device
        self._batch_inference = batch_inference

    @property
    def input_names(self) -> list[str] | None:
        """Input column names from the schema, or ``None`` without a schema."""
        return schema_names(self._schema.input_schema if self._schema else None)

    @property
    def output_names(self) -> list[str] | None:
        """Output column names from the schema, or ``None`` without a schema."""
        return schema_names(self._schema.output_schema if self._schema else None)

    def predict(self, inputs: dict[str, np.ndarray]) -> dict[str, np.ndarray]:
        """Score one batch.

        Args:
            inputs: Column name to array, each with the batch on axis 0.

        Returns:
            Output column name to array. A model that returns a single tensor
            gets its name from the schema, or ``"prediction"`` without one.

        Raises:
            ValueError: If the model returns a different number of outputs than
                the schema names.
        """
        if self._custom:
            return predict_custom(self._model, inputs, self._batch_inference)

        import torch

        tensors = to_torch_inputs(inputs, self._device)
        with torch.inference_mode():
            output = call_module(self._model, tensors)
        return name_outputs(output, self.output_names)


def create_raw_backend(context: BackendContext) -> RawModelBackend:
    """Load a raw model package and wrap it in a :class:`RawModelBackend`.

    Args:
        context: The package path, schema and device. When ``context.schema`` is
            ``None``, the schema stored in the package is used.

    Returns:
        The backend. A custom model is scored one record at a time unless the
        package's ``metadata/type.yaml`` sets ``batch_inference``.
    """
    from michelangelo.lib.model_manager.serde.model import (
        load_model_schema,
        load_raw_model,
    )

    schema = context.schema or load_model_schema(context.model_path)
    model = load_raw_model(context.model_path)
    return RawModelBackend(
        model,
        schema=schema,
        device=context.device,
        batch_inference=read_batch_inference(context.model_path),
    )
