"""The per-actor batch scorer used by ``Dataset.map_batches``."""

from __future__ import annotations

import logging
import tempfile
import weakref
from concurrent.futures import ThreadPoolExecutor
from typing import TYPE_CHECKING, Any

import numpy as np
import pyarrow as pa

from michelangelo.lib._internal.utils.numpy_utils import (
    assemble_output_table,
    pad_batch,
    pyarrow_to_numpy,
)
from michelangelo.lib.inference.backend import (
    BackendContext,
    build_backend,
    default_device,
    resolve_backend,
)
from michelangelo.lib.inference.raw_backend import RawModelBackend

if TYPE_CHECKING:
    from michelangelo.lib.artifact_manager.storage_backend import StorageBackend
    from michelangelo.lib.model_manager.schema import ModelSchema

__all__ = ["MODEL_NAME_COLUMN", "MODEL_VERSION_COLUMN", "Predictor"]

_logger = logging.getLogger(__name__)

#: Constant column naming the model that produced each row.
MODEL_NAME_COLUMN = "model_name"
#: Constant column holding the model version that produced each row.
MODEL_VERSION_COLUMN = "model_version"


class Predictor:
    """Scores Arrow batches with a model backend.

    Ray Data constructs one instance per worker actor and calls it with each
    batch. Per batch it converts the model's input columns to numpy, pads ragged
    columns, runs the backend, and returns the input table with the prediction
    columns (and the model name and version columns) added.

    Args:
        input_columns: Columns fed to the model. ``None`` uses the backend's
            input names, or every column when it has none.
        model_path: Local directory of the model package. It must be readable
            from the node this actor runs on. Exactly one of ``model_path``,
            ``model_uri`` and ``model`` is required.
        model_uri: URI of the model package in ``storage_backend``. It is
            downloaded by the actor itself, so it works on any node.
        storage_backend: Downloads ``model_uri``. Required with ``model_uri``.
        model: An already loaded model, or a ``ray.ObjectRef`` to one, scored
            with :class:`RawModelBackend`.
        schema: The model schema. Defaults to the one stored in the package.
        model_name: Written to the ``model_name`` column when set.
        model_version: Written to the ``model_version`` column when set.
        columns_to_keep: Restricts the output to these input columns, plus the
            prediction and model columns, which are always kept. ``None`` keeps
            every input column. Names that are not input columns are ignored.
        column_padding_values: Pad value per column for ragged inputs. Unlisted
            columns are padded with the sentinel for their dtype.
        num_cpus: When above ``1``, converts input columns to numpy on a thread
            pool of this size.
        backend: Dotted path of a backend factory. Requires ``model_path``.
        device: Device for the model. Defaults to ``"cuda"`` when available.

    Raises:
        ValueError: If not exactly one model source is given, if
            ``model_uri`` has no ``storage_backend``, if ``backend`` is combined
            with ``model``, or if the backend cannot be built.
        FileNotFoundError: If the model package is missing files.
        TypeError: If ``model`` is not a custom ``Model`` or a PyTorch module.
    """

    def __init__(
        self,
        *,
        input_columns: list[str] | None = None,
        model_path: str | None = None,
        model_uri: str | None = None,
        storage_backend: StorageBackend | None = None,
        model: Any = None,
        schema: ModelSchema | None = None,
        model_name: str | None = None,
        model_version: str | None = None,
        columns_to_keep: list[str] | None = None,
        column_padding_values: dict[str, Any] | None = None,
        num_cpus: int | None = None,
        backend: str | None = None,
        device: str | None = None,
    ) -> None:
        """Load the model and prepare the batch pipeline."""
        sources = [x for x in (model_path, model_uri, model) if x is not None]
        if len(sources) != 1:
            raise ValueError("Pass exactly one of model_path, model_uri and model.")
        if model_uri is not None and storage_backend is None:
            raise ValueError("model_uri needs a storage_backend to download it.")
        if backend is not None and model is not None:
            raise ValueError("A custom backend needs a model package, not model.")

        device = device or default_device()
        if model_uri is not None:
            # Downloaded here, not on the driver, so the package exists on the
            # node that runs this actor. Removed when the actor is collected.
            self._package_dir = tempfile.TemporaryDirectory(prefix="inference_model_")
            storage_backend.download(model_uri, self._package_dir.name)
            model_path = self._package_dir.name
        if model_path is not None:
            from michelangelo.lib.model_manager.serde.model import load_model_schema

            schema = schema or load_model_schema(model_path)
            self._backend = build_backend(
                resolve_backend(model_path, backend),
                BackendContext(model_path=model_path, schema=schema, device=device),
            )
        else:
            import ray

            if isinstance(model, ray.ObjectRef):
                model = ray.get(model)
            self._backend = RawModelBackend(model, schema=schema, device=device)

        self._input_columns = input_columns
        self._model_name = model_name
        self._model_version = model_version
        self._columns_to_keep = columns_to_keep
        self._column_padding_values = column_padding_values
        self._scalar_outputs = _scalar_output_names(schema)
        self._executor = (
            ThreadPoolExecutor(max_workers=num_cpus)
            if num_cpus and num_cpus > 1
            else None
        )
        if self._executor is not None:
            weakref.finalize(self, self._executor.shutdown, wait=False)

    def __call__(self, batch: pa.Table) -> pa.Table:
        """Score one batch.

        Args:
            batch: Rows to score. An empty batch is returned unchanged.

        Returns:
            ``batch`` with prediction and model columns added, in the order
            input columns, predictions, model columns.

        Raises:
            ValueError: If the model reads a column the batch does not have, if a
                prediction or model column has the same name as an input column,
                or a prediction has the same name as a model column.
        """
        if batch.num_rows == 0:
            return batch

        columns = (
            self._input_columns or self._backend.input_names or list(batch.column_names)
        )
        missing = [c for c in columns if c not in batch.column_names]
        if missing:
            raise ValueError(
                f"The dataset has no column(s) {missing}, which the model reads. "
                f"Available columns: {batch.column_names}. Set input_columns or "
                "fix the model's input schema."
            )
        inputs = self._to_numpy(batch, columns)
        inputs = pad_batch(inputs, self._column_padding_values)
        inputs = {k: _encode(v) for k, v in inputs.items()}
        inputs = {k: _add_feature_axis(v) for k, v in inputs.items()}

        predictions = self._backend.predict(inputs)
        predictions = {k: _decode(v) for k, v in predictions.items()}
        predictions = {k: self._squeeze(k, v) for k, v in predictions.items()}

        extra = self._model_columns(batch.num_rows)
        keep = (
            [*self._columns_to_keep, *predictions, *extra]
            if self._columns_to_keep is not None
            else None
        )
        return assemble_output_table(
            batch,
            predictions,
            columns_to_keep=keep,
            extra_columns=extra,
            raise_on_collision=True,
        )

    def _to_numpy(self, batch: pa.Table, columns: list[str]) -> dict[str, np.ndarray]:
        """Convert the model's input columns, in parallel when configured."""
        if self._executor is None:
            return {c: pyarrow_to_numpy(batch[c]) for c in columns}
        arrays = self._executor.map(lambda c: pyarrow_to_numpy(batch[c]), columns)
        return dict(zip(columns, arrays))

    def _squeeze(self, name: str, arr: np.ndarray) -> np.ndarray:
        """Turn an ``(n, 1)`` output into ``(n,)`` when its schema shape is ``[1]``."""
        if name in self._scalar_outputs and arr.ndim == 2 and arr.shape[1] == 1:
            return arr.reshape(arr.shape[0])
        return arr

    def _model_columns(self, num_rows: int) -> dict[str, pa.Array]:
        """Constant columns identifying the model."""
        columns: dict[str, pa.Array] = {}
        if self._model_name:
            columns[MODEL_NAME_COLUMN] = pa.repeat(
                pa.scalar(self._model_name, pa.string()), num_rows
            )
        if self._model_version:
            columns[MODEL_VERSION_COLUMN] = pa.repeat(
                pa.scalar(self._model_version, pa.string()), num_rows
            )
        return columns


def _scalar_output_names(schema: ModelSchema | None) -> set[str]:
    """Names of outputs whose schema shape is exactly ``[1]``."""
    if schema is None:
        return set()
    return {
        item.name
        for item in schema.output_schema
        if item.shape and len(item.shape) == 1 and item.shape[0] == 1
    }


def _convert_strings(arr: np.ndarray, source: type, fn: Any) -> np.ndarray:
    """Apply ``fn`` to the ``source``-typed elements of an object array.

    The array is returned untouched when it holds no ``source`` elements, so
    object arrays of vectors pass through. Otherwise the result stays an object
    array of the same shape: letting numpy infer a fixed-width string dtype would
    strip trailing NUL bytes from binary values, widen every element to the
    longest one, and turn non-string elements into text.
    """
    flat = arr.ravel()
    if not any(isinstance(x, source) for x in flat):
        return arr
    out = np.empty(len(flat), dtype=object)
    for i, x in enumerate(flat):
        out[i] = fn(x) if isinstance(x, source) else x
    return out.reshape(arr.shape)


def _encode(arr: np.ndarray) -> np.ndarray:
    """Encode string columns as UTF-8 bytes, which backends expect."""
    if arr.dtype.kind == "U":
        return np.char.encode(arr, "utf-8")
    if arr.dtype.kind == "O":
        return _convert_strings(arr, str, lambda x: x.encode("utf-8"))
    return arr


def _decode(arr: np.ndarray) -> np.ndarray:
    """Decode UTF-8 byte columns back to strings."""
    if arr.dtype.kind == "S":
        return np.char.decode(arr, "utf-8")
    if arr.dtype.kind == "O":
        return _convert_strings(arr, bytes, lambda x: x.decode("utf-8"))
    return arr


def _add_feature_axis(arr: np.ndarray) -> np.ndarray:
    """Turn a scalar column of shape ``(n,)`` into ``(n, 1)``."""
    return arr.reshape(arr.shape[0], 1) if arr.ndim == 1 else arr
