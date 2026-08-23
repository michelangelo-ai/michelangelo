"""Tabular native transform workflow task.

Applies PyTorch transformations to tabular datasets using Ray for distributed
processing. Unlike a Spark-DSL-based transform task, this task uses native
PyTorch layers for transformations like normalization, scaling,
concatenation, and custom numerical transforms — the same
:class:`~michelangelo.lib.native_transform.torch.TransformSpec` DAG runs at
training time (here, via Ray batch inference) and at serving time (embedded
in the resulting model artifact).
"""

from __future__ import annotations

import json
import logging
import time
from typing import TYPE_CHECKING

from michelangelo.lib.native_transform.torch import (
    TorchTransformModule,
    TransformSpec,
    get_transform_module,
)
from michelangelo.lib.native_transform.torch.schema import (
    derive_native_transform_schema,
)
from michelangelo.uniflow.plugins.ray.data_context import set_ray_data_context
from michelangelo.uniflow.plugins.ray.native_transform import (
    transform as native_transform,
)
from michelangelo.uniflow.plugins.ray.parquet_io import parquet_read_config_to_kwargs
from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.tasks.tabular_native_transform._private import (
    incremental_training,
)
from michelangelo.workflow.tasks.tabular_native_transform._private.utils import (
    PREFERRED_DATASET_ORDER,
    convert_to_numpy_sample,
    get_sample_data_from_datasets,
    resolve_data_file_path,
)
from michelangelo.workflow.variables import DatasetVariable, ModelVariable
from michelangelo.workflow.variables._private.utils.serialization import save_object
from michelangelo.workflow.variables.metadata import DatasetMetadata
from michelangelo.workflow.variables.types import NativeTransformResult

if TYPE_CHECKING:
    from michelangelo.lib.artifact_manager.storage_backend import StorageBackend
    from michelangelo.workflow.schema.tabular_native_transform import (
        TabularNativeTransformConfig,
    )

_logger = logging.getLogger(__name__)

__all__ = ["tabular_native_transform"]


def tabular_native_transform(
    config: TabularNativeTransformConfig,
    datasets: dict[str, DatasetVariable],
    *,
    storage_backend: StorageBackend | None = None,
) -> NativeTransformResult:
    """Run the tabular native transform task using Ray and PyTorch.

    Args:
        config: Tabular native transform configuration containing:

            - ``transform_spec``: Transform specification as a dict or a
              YAML file path.
            - ``batch_options``: Ray processing options (batch size,
              concurrency, num_gpus, num_cpus).
            - ``ray_data_context``: Optional Ray Data block size and I/O
              retry pattern tuning.
        datasets: A dict of datasets (e.g. ``{"train": train_dataset,
            "validation": val_dataset, "test": test_dataset}``). The keys
            are dataset names and the values are ``DatasetVariable``
            objects; the transform runs on each dataset. For a training
            pipeline, this typically includes train, validation, and
            optionally test datasets.
        storage_backend: Backend used to download a base run's artifacts.
            Required only when ``config.incremental_training`` specifies
            ``TrainingType.INCREMENTAL`` mode; ignored otherwise.

    Returns:
        A ``NativeTransformResult`` containing the transformed datasets and
        the PyTorch transform module (model), with feature statistics in
        its metadata.

    Raises:
        ConfigurationError: If ``datasets`` is empty or every dataset is
            empty, if ``config.transform_spec`` is not a dict or string
            path, if incremental mode is active without a
            ``storage_backend``, or if an incremental merge/reuse
            constraint is violated (see
            :mod:`~michelangelo.workflow.tasks.tabular_native_transform._private.incremental_training`).
    """
    rc = config.ray_data_context
    set_ray_data_context(
        min_block_size=rc.min_block_size if rc else None,
        max_block_size=rc.max_block_size if rc else None,
        retried_io_errors=rc.retried_io_errors if rc else None,
    )

    inc = config.incremental_training

    # Early return if no transform spec provided. INCREMENTAL mode loads its
    # spec from the base run, so config.transform_spec can be None.
    if config.transform_spec is None and not incremental_training.is_incremental(inc):
        _logger.info("No transform spec provided, returning datasets unchanged")
        return NativeTransformResult(transformed_datasets=datasets)

    # Create Ray Dataset objects from each input dataset, forwarding any
    # user-provided parquet_read_config to ray.data.read_parquet. This does
    # not load the data rows into memory — Ray Datasets execute lazily. See:
    # https://docs.ray.io/en/latest/data/key-concepts.html#data-key-concepts
    for name, dataset_var in datasets.items():
        if dataset_var is not None:
            dataset_var.load_ray_dataset(
                **parquet_read_config_to_kwargs(
                    config.parquet_read_config, dataset_name=name
                )
            )

    _validate_datasets(datasets)

    # Load transform spec and initial feature stats based on mode.
    initial_feature_stats: dict = {}
    if incremental_training.is_incremental(inc):
        if storage_backend is None:
            raise ConfigurationError(
                "storage_backend is required when incremental_training.training_type "
                "is TrainingType.INCREMENTAL."
            )
        base_spec, base_stats = incremental_training.load_incremental_artifacts(
            inc, storage_backend
        )
        if not inc.enforce_full_reuse:
            config_spec = _load_transform_spec(config)
            transform_spec, initial_feature_stats = (
                incremental_training.merge_specs_for_selective_refit(
                    base_spec, config_spec, base_stats
                )
            )
        else:
            transform_spec, initial_feature_stats = base_spec, base_stats
    else:
        transform_spec = _load_transform_spec(config)

    # Sample data from the original datasets BEFORE transformation, for shape
    # derivation.
    sample_data = get_sample_data_from_datasets(datasets)

    # Transform all datasets (train, validation, test). Feature statistics
    # are computed from training data during transformation, or skipped when
    # pre-loaded stats already cover the required levels.
    transformed_datasets, transform_spec, feature_stats = _transform_all_datasets(
        datasets,
        transform_spec,
        config,
        initial_feature_stats=initial_feature_stats,
    )

    _logger.info("Creating transform module (model) from transform spec")
    model_variable = _create_transform_model(transform_spec, feature_stats, sample_data)

    write_kwargs = {}
    if config.write_config:
        if config.write_config.max_rows_per_file is not None:
            write_kwargs["max_rows_per_file"] = config.write_config.max_rows_per_file
        if config.write_config.min_rows_per_file is not None:
            write_kwargs["min_rows_per_file"] = config.write_config.min_rows_per_file
    _save_datasets(transformed_datasets, **write_kwargs)

    if incremental_training.is_incremental(inc) and model_variable is None:
        raise ConfigurationError(
            "INCREMENTAL transform produced no model — the transform spec has "
            "no layers."
        )

    _logger.info("Native transformation completed successfully")
    return NativeTransformResult(
        transformed_datasets=transformed_datasets,
        model=model_variable,
    )


def _validate_datasets(datasets: dict[str, DatasetVariable]) -> None:
    """Validate that at least one dataset is non-empty.

    Raises:
        ConfigurationError: If ``datasets`` is empty, or every dataset in it
            is empty.
    """
    if not datasets:
        raise ConfigurationError("No datasets provided for native transformation")

    for dataset_var in datasets.values():
        if dataset_var.value is not None and len(dataset_var.value.take(1)) > 0:
            return

    raise ConfigurationError(
        "All datasets are empty. At least one non-empty dataset is required for "
        "native transformation."
    )


def _load_transform_spec(config: TabularNativeTransformConfig) -> TransformSpec:
    """Load a transform specification from config.

    ``config.transform_spec`` accepts either:

    - ``dict``: an inlined spec.
    - ``str``: a file path to a YAML spec, resolved via
      :func:`~michelangelo.workflow.tasks.tabular_native_transform._private.utils.resolve_data_file_path`.

    Raises:
        ConfigurationError: If ``config.transform_spec`` is neither a dict
            nor a string.
    """
    if isinstance(config.transform_spec, dict):
        _logger.info("Loading transform spec from inlined dict")
        return TransformSpec(raw_transform_specs=config.transform_spec)

    if isinstance(config.transform_spec, str):
        resolved_path = resolve_data_file_path(config.transform_spec)
        _logger.info("Loading transform spec from file: %s", resolved_path)
        return TransformSpec(transform_spec_yaml_path=resolved_path)

    raise ConfigurationError("transform_spec must be a dict or a file path string")


def _transform_all_datasets(
    datasets: dict[str, DatasetVariable],
    transform_spec: TransformSpec,
    config: TabularNativeTransformConfig,
    initial_feature_stats: dict | None = None,
) -> tuple[dict[str, DatasetVariable], TransformSpec, dict]:
    """Transform all datasets in order, reusing the updated spec and feature stats.

    Feature statistics are computed from the training data and reused for
    validation/test. Processes datasets in
    :data:`~michelangelo.workflow.tasks.tabular_native_transform._private.utils.PREFERRED_DATASET_ORDER`
    order to ensure stats from training are used first.

    Args:
        datasets: Dictionary of dataset variables.
        transform_spec: Transform specification.
        config: Tabular native transform configuration.
        initial_feature_stats: Pre-loaded feature stats (e.g. from a base
            run in ``TrainingType.INCREMENTAL`` mode). When provided, the
            stats are used as-is and fitting is skipped for levels whose
            stats are already present.

    Returns:
        A ``(transformed_datasets, updated_transform_spec,
        computed_feature_stats)`` tuple.
    """
    transformed_datasets: dict[str, DatasetVariable] = {}
    feature_stats = dict(initial_feature_stats) if initial_feature_stats else {}

    dataset_order = [name for name in PREFERRED_DATASET_ORDER if name in datasets]
    dataset_order.extend(name for name in datasets if name not in dataset_order)

    for dataset_name in dataset_order:
        dataset_var = datasets[dataset_name]

        # Pass through empty datasets unchanged (validation/test may be optional).
        if dataset_var.value is None or len(dataset_var.value.take(1)) == 0:
            _logger.warning("Passing through empty dataset unchanged: %s", dataset_name)
            transformed_datasets[dataset_name] = dataset_var
            continue

        _logger.info("Transforming dataset: %s", dataset_name)
        t = time.time()

        transformed_data, transform_spec, feature_stats = native_transform(
            df=dataset_var.value,
            transform_spec=transform_spec,
            feature_stats=feature_stats,
            batch_size=config.batch_options.batch_size,
            map_batch_concurrency=config.batch_options.concurrency,
            num_gpus=config.batch_options.num_gpus,
        )
        _logger.info(
            "Transform %s data completed in %.2f seconds", dataset_name, time.time() - t
        )

        if dataset_name == "train":
            _logger.info(
                "Feature stats computed from training data: %s",
                json.dumps(feature_stats),
            )

        transformed_dataset = DatasetVariable.create(transformed_data)

        # Derive output columns from the transform spec. Use columns_to_keep
        # if provided; otherwise use transform outputs at all levels.
        input_columns = set(dataset_var.value.schema().names)
        if transform_spec.columns_to_keep is not None:
            output_columns = set(transform_spec.columns_to_keep)
        else:
            output_columns = set()
            for level in range(transform_spec.get_max_transform_level() + 1):
                output_columns.update(transform_spec.get_transform_output_cols(level))
        derived_features = sorted(output_columns - input_columns)
        _logger.info("Derived features for %s: %s", dataset_name, derived_features)
        transformed_dataset.metadata = DatasetMetadata(
            derived_features=derived_features
        )

        transformed_datasets[dataset_name] = transformed_dataset

    return transformed_datasets, transform_spec, feature_stats


def _create_transform_model(
    transform_spec: TransformSpec,
    feature_stats: dict,
    sample_data: dict | None,
) -> ModelVariable | None:
    """Create a ``ModelVariable`` wrapping a ``TorchTransformModule`` from the spec.

    The transform module can be used for inference and deployment.

    Args:
        transform_spec: The transform specification.
        feature_stats: Feature statistics computed during transformation.
        sample_data: Sample row from the original dataset (for shape
            derivation). Stored on ``metadata.sample_data`` as numpy sample
            rows filtered to the derived ``input_schema`` column names.

    Returns:
        A ``ModelVariable`` containing the transform module with metadata,
        or ``None`` if the transform spec has no layers.
    """
    transform_module: TorchTransformModule | None = get_transform_module(
        transform_spec=transform_spec,
        start_level=0,
        end_level=transform_spec.get_max_transform_level(),
    )

    if transform_module is None:
        _logger.warning("No transform module created (no transform layers)")
        return None

    model_variable = ModelVariable.create(transform_module)
    model_variable.metadata.model_class = (
        f"{transform_module.__class__.__module__}.{transform_module.__class__.__name__}"
    )
    # Store in both fields: hyperparameters follows the convention for model
    # init params, transform_spec is the dedicated field used by the
    # assembler and pusher for incremental training.
    model_variable.metadata.hyperparameters = transform_spec.to_dict()
    model_variable.metadata.transform_spec = transform_spec.to_dict()
    model_variable.metadata.feature_stats = feature_stats

    # Populate schema from transform_spec with derived shapes for downstream
    # tasks (trainer, assembler).
    derived_schema = derive_native_transform_schema(
        transform_spec=transform_spec,
        transform_module=transform_module,
        sample_data=sample_data,
    )
    model_variable.metadata._schema = save_object(derived_schema)
    model_variable.metadata._sample_data = save_object(
        convert_to_numpy_sample(sample_data, input_schema=derived_schema.input_schema)
    )

    model_variable.save()

    return model_variable


def _save_datasets(
    transformed_datasets: dict[str, DatasetVariable], **write_kwargs
) -> None:
    """Persist each transformed dataset through its ``DatasetVariable``.

    Transformed datasets already carry native PyArrow schemas (the native
    transform emits ``pa.Table`` blocks with multi-dim columns encoded as
    nested ``list<list<...>>``), so each dataset is written straight through
    to its sink.

    Args:
        transformed_datasets: Mapping of dataset name to the transformed
            ``DatasetVariable`` to persist.
        **write_kwargs: Forwarded to ``DatasetVariable.save_ray_dataset()``
            and ultimately to ``ray.data.Dataset.write_parquet()``.
            Supported keys include ``max_rows_per_file`` and
            ``min_rows_per_file``.
    """
    for dataset_name, dataset_var in transformed_datasets.items():
        _logger.info("Saving transformed dataset: %s", dataset_name)
        t = time.time()

        if write_kwargs:
            _logger.info("Writing %s with write_kwargs=%s", dataset_name, write_kwargs)
        dataset_var.save_ray_dataset(**write_kwargs)
        _logger.info("Saved %s dataset in %.2f seconds", dataset_name, time.time() - t)
