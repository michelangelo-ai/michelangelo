"""Ray batch inference using ``ray.data.Dataset.map_batches``."""

from __future__ import annotations

import logging
import os
import shutil
import tempfile
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass
from typing import TYPE_CHECKING

import numpy as np
import pyarrow as pa
import ray

from michelangelo.lib._internal.utils.numpy_utils import assemble_output_table
from michelangelo.lib._internal.utils.numpy_utils.pad import pad_batch as _pad_batch
from michelangelo.lib._internal.utils.numpy_utils.pyarrow_conversion import (
    pyarrow_to_numpy,
)
from michelangelo.lib._internal.utils.numpy_utils.type import (
    infer_dtype as _infer_dtype,
)
from michelangelo.lib.inference._private.lib.gpu_errors import (
    _should_restart_actor_for_gpu_error,
)
from michelangelo.lib.inference.lib.model_inference import ModelInference
from michelangelo.lib.inference.ray.utils.data import merge_ray_datasets

if TYPE_CHECKING:
    from michelangelo.lib.artifact_manager.storage_backend import StorageBackend

_logger = logging.getLogger(__name__)


# Default Ray actor fault tolerance for inference map_batches workers. Non-zero so a
# single
# dead actor (transient GPU/NVLink fault, OOM-killed process) restarts and its in-
# flight block
# is resubmitted, instead of failing a multi-day job at the first fault. Users can
# override or
# disable via BatchOptions.max_restarts / max_task_retries = 0.
DEFAULT_ACTOR_MAX_RESTARTS = 3
DEFAULT_ACTOR_MAX_TASK_RETRIES = 3


class Predictor:
    """Scores Arrow batches with a model. Ray Data builds one per worker actor."""

    def __init__(
        self,
        input_columns: list[str] | None = None,
        model_name: str | None = None,
        model_path: str | None = None,
        model_type: str | None = None,
        storage_backend: StorageBackend | None = None,
        column_padding_values: dict[str, any] | None = None,
        columns_to_keep: list[str] | None = None,
        num_cpus: int | None = None,
    ):
        """Load the model.

        Args:
            input_columns: The columns to use for inference. If not provided (or empty),
                inferred from the model schema when available; otherwise
                all batch columns are used.
            model_name: The name of the model.
            model_path: The path to the packaged model. A storage URI that
                ``storage_backend`` downloads, or a local directory when no
                ``storage_backend`` is given.
            model_type: The raw model type of the model.
            storage_backend: Downloads ``model_path`` onto this worker. When None,
                ``model_path`` is used as a local directory.
            column_padding_values: The padding value for each column.
            columns_to_keep: Restricts output to exactly these columns, plus
                predictions and metadata columns (model_name), which are always
                kept automatically. None keeps all input columns. Label/target
                columns are NOT auto-kept -- list them explicitly if a downstream
                task needs them, or they will be silently dropped.
            num_cpus: The actor's CPU reservation (``batch_options.num_cpus``).
                When > 1, also sizes a thread pool used to parallelize the
                per-column PyArrow->NumPy conversion of the input batch, since
                that work is memory-bandwidth-bound rather than CPU-count-bound
                and benefits from concurrency up to the actor's own CPU budget.
                Defaults to None (serial conversion, unchanged behavior).

        Raises:
            ValueError: If ``model_path`` is not provided.
        """
        local_model_path = None

        if model_path:
            if storage_backend is not None:
                local_model_path = _download_model_package(storage_backend, model_path)
            else:
                local_model_path = model_path

        self.inference = ModelInference(
            local_model_path,
            model_type,
            model_name,
        )
        self.model_name = model_name
        self.column_padding_values = column_padding_values
        self.columns_to_keep = columns_to_keep
        self.inference.load_model()
        self.input_columns = self.get_input_columns(input_columns)
        self.scalar_output_columns = self.get_scalar_output_columns(self.inference)
        self._conversion_executor = (
            ThreadPoolExecutor(max_workers=num_cpus)
            if num_cpus and num_cpus > 1
            else None
        )

    def _convert_input_batch(
        self, batch: pa.Table, input_columns: list[str]
    ) -> dict[str, np.ndarray]:
        """Convert each input column from PyArrow to NumPy.

        Uses a thread pool sized to ``num_cpus`` (see __init__) when that's > 1;
        otherwise identical to the original serial dict comprehension.
        """
        if self._conversion_executor is None:
            return {col: pyarrow_to_numpy(batch[col]) for col in input_columns}
        results = self._conversion_executor.map(
            lambda c: pyarrow_to_numpy(batch[c]), input_columns
        )
        return dict(zip(input_columns, results))

    def __call__(self, batch: pa.Table) -> pa.Table:
        """Score one batch.

        Args:
            batch: The rows to score.

        Returns:
            The batch with the prediction and metadata columns added.
        """
        # Only convert model input columns to numpy — passthrough columns stay
        # as PyArrow
        input_columns = (
            self.input_columns if self.input_columns else list(batch.column_names)
        )
        input_batch = self._convert_input_batch(batch, input_columns)

        try:
            if self.inference.batch_inference:
                predictions = self.inference_on_batch(input_batch)
            else:
                predictions = self.inference_on_records(input_batch)
        except Exception as e:
            if _should_restart_actor_for_gpu_error(e):
                # The CUDA context is poisoned: raising here would
                # fail the whole Ray Data job
                # (and an in-process retry could never succeed
                # anyway). Kill the worker process
                # instead, so Ray restarts the actor on a fresh CUDA
                # context (max_restarts) and
                # resubmits this block (max_task_retries). With
                # restarts disabled the job fails
                # either way, so this is never worse than raising.
                _logger.critical(
                    "Unrecoverable GPU error in %s inference; exiting worker process "
                    "so Ray can restart the actor and retry the block on a fresh CUDA "
                    "context: %s",
                    self.model_name,
                    e,
                    exc_info=True,
                )
                logging.shutdown()
                os._exit(1)
            raise

        extra_columns = self._metadata_columns(len(batch))
        columns_to_keep = (
            list(self.columns_to_keep) + list(predictions) + list(extra_columns)
            if self.columns_to_keep is not None
            else None
        )

        return assemble_output_table(
            batch,
            predictions,
            columns_to_keep=columns_to_keep,
            extra_columns=extra_columns,
            raise_on_collision=True,
        )

    def _metadata_columns(self, batch_size: int) -> dict[str, pa.Array]:
        """Constant model-id columns appended after predictions."""
        extra: dict[str, pa.Array] = {}
        if self.model_name:
            extra["model_name"] = pa.array(np.full(batch_size, self.model_name))
        return extra

    def inference_on_records(
        self, batch: dict[str, np.ndarray]
    ) -> dict[str, np.ndarray]:
        """Run inference by converting the batch into records."""
        input_batch = self.pad_batch(batch)
        input_batch = self.encode_strings(input_batch)
        records = self.batch_to_records(input_batch)
        predictions = [self.inference.execute(r) for r in records]
        res = {
            key: np.stack([item[key] for item in predictions]) for key in predictions[0]
        }
        res = self.decode_strings(res)
        res = self.reshape_result(res)
        return res

    def inference_on_batch(self, batch: dict[str, np.ndarray]) -> dict[str, np.ndarray]:
        """Run inference on the batch directly."""
        input_batch = self.pad_batch(batch)
        input_batch = self.encode_strings(input_batch)
        input_batch = self.reshape_batch(input_batch)
        res = self.inference.execute(input_batch)
        res = self.decode_strings(res)
        res = self.reshape_result(res)
        return res

    def encode_strings(self, batch: dict[str, np.ndarray]) -> dict[str, np.ndarray]:
        """Convert unicode strings to bytes in the batch."""
        return {col: self._encode_column(b) for col, b in batch.items()}

    @staticmethod
    def _encode_column(b: np.ndarray) -> np.ndarray:
        # Pure unicode arrays: fully vectorized C-level encode (np.vectorize is a
        # disguised per-element Python loop with real overhead - avoid it here).
        if b.dtype.kind == "U":
            return np.char.encode(b, "utf-8")
        # Object arrays may mix strs with other types (None, already-bytes, ...);
        # a plain comprehension is still much lighter than np.vectorize's broadcasting
        # machinery for this elementwise case. Let np.array infer the dtype (as
        # np.vectorize did) rather than forcing dtype=object, so a homogeneous
        # column of encoded bytes collapses to a fixed-width 'S' array - matching
        # what downstream inference backends expect for byte tensors.
        if b.dtype.kind == "O":
            # np.array([]) defaults to float64, silently discarding the object
            # dtype for an empty column - short-circuit rather than let dtype
            # inference run on zero elements.
            if len(b) == 0:
                return b
            return np.array([x.encode("utf-8") if isinstance(x, str) else x for x in b])
        return b

    def decode_strings(self, batch: dict[str, np.ndarray]) -> dict[str, np.ndarray]:
        """Convert bytes to unicode strings in the batch."""
        return {col: self._decode_column(b) for col, b in batch.items()}

    @staticmethod
    def _decode_column(b: np.ndarray) -> np.ndarray:
        if b.dtype.kind == "S":
            return np.char.decode(b, "utf-8")
        if b.dtype.kind == "O":
            if len(b) == 0:
                return b
            return np.array(
                [x.decode("utf-8") if isinstance(x, bytes) else x for x in b]
            )
        return b

    def reshape_batch(self, batch: dict[str, np.ndarray]) -> dict[str, np.ndarray]:
        """Convert batches whose shapes are (n,) to (n, 1), rest are unchanged."""
        return {
            col: b.reshape(b.shape[0], 1) if len(b.shape) == 1 else b
            for col, b in batch.items()
        }

    def reshape_result(self, result: dict[str, np.ndarray]) -> dict[str, np.ndarray]:
        """Convert results whose shapes are (n, 1) to (n,), rest are unchanged."""
        return {
            col: b.reshape(b.shape[0])
            if len(b.shape) == 2
            and b.shape[1] == 1
            and col in self.scalar_output_columns
            else b
            for col, b in result.items()
        }

    def batch_to_records(
        self, batch: dict[str, np.ndarray]
    ) -> list[dict[str, np.ndarray]]:
        """Split the batch into a list of records.

        Args:
            batch: a dict whose keys are the columns and items are the
                values in the batch

        Returns:
            A list of records in which each record is a dict whose keys are
            the columns and the items are values in the record
        """
        records = []
        for col, v in batch.items():
            # Split the batch into records along the first axis
            splits = np.split(v, v.shape[0], 0) if v.shape[0] > 0 else v
            if len(records) == 0:
                records.extend([{col: r} for r in splits])
            else:
                for record, sp in zip(records, splits):
                    record[col] = sp
        return records

    def infer_dtype(self, arr: np.ndarray) -> type:
        """Delegate to shared util. See pad.infer_dtype for full docstring."""
        return _infer_dtype(arr)

    def pad_batch(self, batch: dict[str, np.ndarray]) -> dict[str, np.ndarray]:
        """Delegate to shared util. See pad.pad_batch for full docstring."""
        return _pad_batch(batch, self.column_padding_values)

    def get_input_columns(
        self,
        input_columns: list[str] | None,
    ) -> list[str] | None:
        """Resolve input_columns from explicit value or model schema.

        When input_columns is not provided (None or empty) and the model has a schema
        with input_schema, returns the input feature names from the schema.
        Otherwise returns the given input_columns as-is.
        """
        if (
            not input_columns
            and self.inference
            and self.inference.model_schema
            and self.inference.model_schema.input_schema
        ):
            resolved = [item.name for item in self.inference.model_schema.input_schema]
            _logger.info("Inferred input_columns from model schema: %s", resolved)
            return resolved
        return input_columns

    def get_scalar_output_columns(self, inference: ModelInference) -> set[str]:
        """Get the scalar output column set.

        An output column is considered scalar if its schema shape is explicitly
        specified as [1] by the user

        Args:
            inference: The inference object

        Returns:
            The scalar output column set
        """
        model_schema = inference.model_schema
        output_schema = model_schema.output_schema if model_schema else None
        scalar_output_columns = (
            {
                item.name
                for item in output_schema
                if item.shape and len(item.shape) == 1 and item.shape[0] == 1
            }
            if output_schema
            else set()
        )
        return scalar_output_columns


def _download_model_package(storage_backend: StorageBackend, model_path: str) -> str:
    """Download a model package onto this worker and return its local directory.

    A package stored as a single archive, such as a deployable package pushed
    as ``model.tar``, arrives as one file and is unpacked.

    Args:
        storage_backend: Downloads ``model_path``.
        model_path: The package's URI: a directory, or a single archive file.

    Returns:
        The local package directory.
    """
    work_dir = tempfile.mkdtemp()
    package_dir = os.path.join(work_dir, "package")
    storage_backend.download(model_path, package_dir)
    if not os.path.isfile(package_dir):
        return package_dir

    archive = os.path.join(work_dir, "package.tar")
    os.rename(package_dir, archive)
    shutil.unpack_archive(archive, package_dir, "tar")
    os.remove(archive)
    return package_dir


@dataclass
class ModelIdentifier:
    """Identifies the model to run."""

    name: str | None = None
    revision_id: int | None = None
    # The path to the model in remote storage
    model_path: str | None = None
    # The type of the raw model
    model_type: str | None = None


@dataclass
class BatchOptions:
    """Options for batching and scheduling the inference workers."""

    # The desired number of rows in each batch
    batch_size: int | None = None
    # The number of GPUs to reserve for each parallel map worker.
    # For example, specify num_gpus=1 to request 1 GPU for each parallel map worker.
    num_gpus: float | None = None
    # The number of workers to use concurrently
    concurrency: int | None = None
    # The number of CPUs to reserve for each parallel map worker.
    # For example, specify num_cpus=1 to request 1 CPU for each parallel map worker.
    # Warning: Specifying both num_cpus and num_gpus for map tasks is
    # experimental, and may result in scheduling or stability issues.
    num_cpus: int | None = None
    # Ray actor max concurrency for class-based map_batches workers.
    max_concurrency: int | None = None
    # Maximum queued/in-flight tasks per Ray actor in the ActorPoolMapOperator.
    max_tasks_in_flight_per_actor: int | None = None
    # Times Ray restarts a dead inference actor before failing the job.
    # None applies DEFAULT_ACTOR_MAX_RESTARTS; 0 disables restarts.
    max_restarts: int | None = None
    # Times the in-flight block is resubmitted after its actor dies. Only effective with
    # max_restarts != 0. None applies DEFAULT_ACTOR_MAX_TASK_RETRIES; 0 disables.
    max_task_retries: int | None = None


@dataclass
class PaddingOptions:
    """Options for padding Rag tensors in input data."""

    # The padding value for each column.
    # If not provided, the padding value will be 0 for all columns.
    # If provided, the padding value will be used for the corresponding column.
    # The key is the column name and the value is the padding value.
    # The padding value can be a scalar of any type, but it must match the type of
    # the column.
    column_padding_values: dict[str, any] | None = None


@dataclass
class InferenceConfig:
    """Describes one model to run over a dataset."""

    model: ModelIdentifier
    # The name of the input column to use for inference
    input_columns: list[str] | None = None
    batch_options: BatchOptions | None = None
    padding_options: PaddingOptions | None = None
    columns_to_keep: list[str] | None = None


@dataclass
class OutputColumnMetadata:
    """Names a column added by inference and the model that produced it."""

    column_name: str
    model_name: str


def batch_model_inference(
    config: list[InferenceConfig],
    datasets: dict[str, ray.data.Dataset],
    storage_backend: StorageBackend | None = None,
) -> tuple[dict[str, ray.data.Dataset], list[OutputColumnMetadata]]:
    """Run offline batch inference on packaged models.

    Args:
        config: A list of InferenceConfig objects, each containing model
            information and input columns.
        datasets: A dictionary mapping dataset names to ray.data.Dataset objects.
        storage_backend: Downloads each model's ``model_path`` onto the workers.
            When None,
            ``model_path`` is used as a local directory.

    Returns:
        A tuple containing:
        - A dictionary mapping dataset names to ray.data.Dataset objects with
            inference results.
        - A list of OutputColumnMetadata objects describing the output columns
            added by the inference.
    """
    if not config:
        _logger.warning(
            "No model inference config provided. "
            "Skipping model inference and returning the original datasets."
        )
        return datasets, []

    results = {}
    output_column_metadata = []

    for dataset_name, dataset in datasets.items():
        if not dataset or not dataset.limit(1).take(1):
            _logger.warning(
                f"Dataset {dataset_name} is empty. "
                "Skipping model inference for this dataset."
            )
            results[dataset_name] = dataset
            continue

        inference_results = []
        output_metadata = []

        for inference_config in config:
            model_name = inference_config.model.name
            _logger.info(
                f"Running model inference for {model_name} on dataset {dataset_name}."
            )

            inference_result = _run_predictor_inference(
                inference_config, dataset, storage_backend
            )
            inference_results.append(inference_result)

            # Determine output columns by running a 1-row sample through the Predictor.
            # We must NOT call inference_result.schema() directly — that triggers Ray to
            # execute the full map_batches(Predictor) pipeline on the entire dataset,
            # which fills the object store and causes a deadlock when limit() cannot
            # short-circuit the ActorPoolMapOperator.
            sample_for_schema = _run_predictor_inference(
                inference_config, dataset.limit(1), storage_backend
            )
            input_set = set(dataset.schema().names)
            output_set = set(sample_for_schema.schema().names)
            output_columns = output_set - input_set - {"model_name"}
            output_metadata.extend(
                [
                    OutputColumnMetadata(column_name=col, model_name=model_name)
                    for col in output_columns
                ]
            )

        # We assume all the output columns for every dataset is the same
        # because each dataset are used by the same set of models
        if len(output_column_metadata) == 0:
            output_column_metadata = output_metadata

        results[dataset_name] = merge_ray_datasets(inference_results) or dataset

    _logger.info("Model inference completed for all datasets.")
    return results, output_column_metadata


def _run_predictor_inference(
    config: InferenceConfig,
    dataset: ray.data.Dataset,
    storage_backend: StorageBackend | None = None,
) -> ray.data.Dataset:
    """Run inference using the Predictor on a packaged model."""
    model_name = config.model.name
    model_path = config.model.model_path
    model_type = config.model.model_type
    input_columns = config.input_columns
    batch_options = config.batch_options
    batch_size = batch_options.batch_size if batch_options else None
    num_gpus = batch_options.num_gpus if batch_options else None
    concurrency = batch_options.concurrency if batch_options else None
    num_cpus = batch_options.num_cpus if batch_options else None
    max_concurrency = batch_options.max_concurrency if batch_options else None
    max_tasks_in_flight_per_actor = (
        batch_options.max_tasks_in_flight_per_actor if batch_options else None
    )
    max_restarts = (
        batch_options.max_restarts
        if batch_options and batch_options.max_restarts is not None
        else DEFAULT_ACTOR_MAX_RESTARTS
    )
    max_task_retries = (
        batch_options.max_task_retries
        if batch_options and batch_options.max_task_retries is not None
        else DEFAULT_ACTOR_MAX_TASK_RETRIES
    )
    padding_options = config.padding_options
    column_padding_values = (
        padding_options.column_padding_values if padding_options else None
    )

    _logger.info(f"Running Predictor inference for {model_name}.")

    if max_tasks_in_flight_per_actor is None:
        compute = None
    elif concurrency is not None:
        compute = ray.data.ActorPoolStrategy(
            size=concurrency,
            max_tasks_in_flight_per_actor=max_tasks_in_flight_per_actor,
        )
    else:
        compute = ray.data.ActorPoolStrategy(
            max_tasks_in_flight_per_actor=max_tasks_in_flight_per_actor,
        )

    # `max_concurrency` is forwarded into Ray's `**ray_remote_args`, where
    # `None` overrides the actor default of 1 (unlike the named `map_batches`
    # params, which handle `None` gracefully) and crashes ActorPoolMapOperator.
    # Only pass it through when explicitly configured.
    extra_ray_remote_args = {}
    if max_concurrency is not None:
        extra_ray_remote_args["max_concurrency"] = max_concurrency

    # Actor fault tolerance: restart dead actors and resubmit their in-flight
    # blocks, so one
    # transient GPU fault (see _private/lib/gpu_errors.py, used in
    # Predictor.__call__) or OOM-killed worker
    # doesn't fail a multi-day job. Defaults are applied above; 0 disables explicitly.
    extra_ray_remote_args["max_restarts"] = max_restarts
    extra_ray_remote_args["max_task_retries"] = max_task_retries

    return dataset.map_batches(
        Predictor,
        fn_constructor_kwargs={
            "input_columns": input_columns,
            "model_name": model_name,
            "model_path": model_path,
            "model_type": model_type,
            "storage_backend": storage_backend,
            "column_padding_values": column_padding_values,
            "columns_to_keep": config.columns_to_keep,
            "num_cpus": num_cpus,
        },
        batch_size=batch_size,
        num_gpus=num_gpus,
        num_cpus=num_cpus,
        concurrency=concurrency,
        compute=compute,
        batch_format="pyarrow",
        zero_copy_batch=True,
        **extra_ray_remote_args,
    )
