"""Backend that scores a raw model package.

A raw package holds the trained model itself, not an exported serving bundle:
a PyTorch ``nn.Module`` (plain, Lightning, or a transform fused with a
predictor) or a custom ``Model`` from
:mod:`michelangelo.lib.model_manager.interface.custom_model`.
"""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    import numpy as np

    from michelangelo.lib.inference.backend import BackendContext
    from michelangelo.lib.model_manager.schema import ModelSchema

__all__ = ["DEFAULT_OUTPUT_NAME", "RawModelBackend", "create_raw_backend"]

#: Name given to a single unnamed output when the model has no output schema.
DEFAULT_OUTPUT_NAME = "prediction"


class RawModelBackend:
    """Scores batches with a loaded raw model.

    Args:
        model: A PyTorch ``nn.Module`` taking a ``dict`` of tensors, or a custom
            ``Model`` whose ``predict`` takes and returns numpy dicts.
        schema: The model schema. Provides the input and output names.
        device: Device for a PyTorch model. Ignored for custom models.

    Raises:
        TypeError: If ``model`` is neither a custom ``Model`` nor an ``nn.Module``.
    """

    def __init__(
        self, model: Any, schema: ModelSchema | None = None, device: str = "cpu"
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

    @property
    def input_names(self) -> list[str] | None:
        """Input column names from the schema, or ``None`` without a schema."""
        if self._schema is None or not self._schema.input_schema:
            return None
        return [item.name for item in self._schema.input_schema]

    @property
    def output_names(self) -> list[str] | None:
        """Output column names from the schema, or ``None`` without a schema."""
        if self._schema is None or not self._schema.output_schema:
            return None
        return [item.name for item in self._schema.output_schema]

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
            return self._model.predict(inputs)

        import torch

        # Arrow zero-copy batches are read-only, which torch warns about and
        # cannot safely share, so copy only those.
        tensors = {
            k: torch.as_tensor(
                v if v.flags.writeable else v.copy(), device=self._device
            )
            for k, v in inputs.items()
        }
        with torch.inference_mode():
            output = self._model(tensors)
        return self._name_outputs(output)

    def _name_outputs(self, output: Any) -> dict[str, np.ndarray]:
        """Convert a module's output to a dict of named numpy arrays."""
        if isinstance(output, dict):
            return {k: _to_numpy(v) for k, v in output.items()}

        if isinstance(output, (tuple, list)):
            values = list(output)
            names = self.output_names or list(getattr(output, "_fields", ()))
        else:
            values = [output]
            names = self.output_names or [DEFAULT_OUTPUT_NAME]

        if len(names) != len(values):
            raise ValueError(
                f"The model returned {len(values)} output(s) but the schema "
                f"names {len(names)}: {names}."
            )
        return {name: _to_numpy(value) for name, value in zip(names, values)}


def _to_numpy(value: Any) -> np.ndarray:
    """Convert a tensor to a numpy array on the CPU."""
    return value.detach().cpu().numpy()


def create_raw_backend(context: BackendContext) -> RawModelBackend:
    """Load a raw model package and wrap it in a :class:`RawModelBackend`.

    Args:
        context: The package path, schema and device. When ``context.schema`` is
            ``None``, the schema stored in the package is used.

    Returns:
        The backend.
    """
    from michelangelo.lib.model_manager.serde.model import (
        load_model_schema,
        load_raw_model,
    )

    schema = context.schema or load_model_schema(context.model_path)
    model = load_raw_model(context.model_path)
    return RawModelBackend(model, schema=schema, device=context.device)
