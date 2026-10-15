"""Run batch inference over Ray datasets."""

from __future__ import annotations

import logging
from dataclasses import dataclass
from typing import TYPE_CHECKING, Any

from michelangelo.lib.inference.predictor import Predictor

if TYPE_CHECKING:
    import ray.data

    from michelangelo.lib.artifact_manager.storage_backend import StorageBackend
    from michelangelo.lib.model_manager.schema import ModelSchema
    from michelangelo.workflow.schema.inference import (
        BatchOptions,
        OfflineInferenceConfig,
    )

__all__ = ["ModelSource", "batch_model_inference"]

_logger = logging.getLogger(__name__)


@dataclass
class ModelSource:
    """Where to get a model and how to label its output rows.

    Exactly one of ``model_path``, ``uri`` and ``model`` is required.

    Attributes:
        model_path: Local directory of the model package. Every worker reads it,
            so on a multi-node cluster it must be on storage shared by all
            nodes. Prefer ``uri`` there.
        uri: URI of the model package in ``storage_backend``. Each worker
            downloads it, so it works on any node.
        storage_backend: Downloads ``uri``. Required with ``uri``.
        model: An already loaded model, scored with the raw backend. It is
            placed in the Ray object store once and shared by every worker.
        schema: The model schema. Defaults to the one stored in the package.
        name: Written to the ``model_name`` column when set.
        version: Written to the ``model_version`` column when set.
    """

    model_path: str | None = None
    uri: str | None = None
    storage_backend: StorageBackend | None = None
    model: Any = None
    schema: ModelSchema | None = None
    name: str | None = None
    version: str | None = None


def batch_model_inference(
    datasets: dict[str, ray.data.Dataset | None],
    models: list[tuple[OfflineInferenceConfig, ModelSource]],
) -> dict[str, ray.data.Dataset | None]:
    """Score Ray datasets with one or more models.

    Args:
        datasets: Dataset name to dataset. ``None`` and empty datasets are
            returned as they are.
        models: One ``(config, source)`` pair per model. With several models,
            each dataset's result is the union of every model's rows. Set
            ``ModelSource.name`` to tell them apart by the ``model_name`` column.

    Returns:
        A dict with the same keys as ``datasets``. Scoring is lazy: nothing runs
        until the result is consumed or written. Whether a dataset is empty is
        checked eagerly, which reads the first row of its upstream plan. An empty
        ``models`` list returns ``datasets`` unchanged.

    Raises:
        ValueError: If models name different output columns, which could not be
            combined into one dataset.
    """
    if not models:
        _logger.warning("No models configured; returning the datasets unchanged.")
        return datasets

    _check_outputs_match(models)

    # One object-store copy per distinct in-memory model, shared by every dataset.
    refs: dict[int, Any] = {}
    if any(source.model is not None for _, source in models):
        import ray

        for _, source in models:
            if source.model is not None and id(source.model) not in refs:
                refs[id(source.model)] = ray.put(source.model)

    results: dict[str, ray.data.Dataset | None] = {}
    for name, dataset in datasets.items():
        if dataset is None or not dataset.limit(1).take(1):
            _logger.warning("Dataset %s is empty; skipping inference.", name)
            results[name] = dataset
            continue
        scored = [_score(dataset, config, source, refs) for config, source in models]
        results[name] = scored[0].union(*scored[1:]) if len(scored) > 1 else scored[0]
    return results


def _check_outputs_match(models: list[tuple[OfflineInferenceConfig, ModelSource]]):
    """Reject models whose known output column names differ."""
    known = {
        frozenset(item.name for item in source.schema.output_schema)
        for _, source in models
        if source.schema is not None and source.schema.output_schema
    }
    if len(known) > 1:
        raise ValueError(
            "All models must produce the same output columns to be combined, "
            f"but the schemas name {[sorted(k) for k in known]}."
        )


def _score(
    dataset: ray.data.Dataset,
    config: OfflineInferenceConfig,
    source: ModelSource,
    refs: dict[int, Any],
) -> ray.data.Dataset:
    """Add one model's predictions to a dataset."""
    options = config.batch_options
    model = refs.get(id(source.model))
    return dataset.map_batches(
        Predictor,
        fn_constructor_kwargs={
            "input_columns": config.input_columns,
            "model_path": source.model_path,
            "model_uri": source.uri,
            "storage_backend": source.storage_backend,
            "model": model,
            "schema": source.schema,
            "model_name": source.name,
            "model_version": source.version,
            "columns_to_keep": config.columns_to_keep,
            "column_padding_values": config.padding_options.column_padding_values,
            "num_cpus": options.num_cpus,
            "backend": config.backend,
        },
        batch_size=options.batch_size,
        num_gpus=options.num_gpus,
        num_cpus=options.num_cpus,
        compute=_compute_strategy(options),
        batch_format="pyarrow",
        zero_copy_batch=True,
        **_actor_options(options),
    )


def _compute_strategy(options: BatchOptions) -> ray.data.ActorPoolStrategy:
    """Build the actor pool.

    ``compute`` is always set, and ``concurrency`` is never used, because newer
    Ray versions deprecate ``concurrency`` and ignore it when ``compute`` is set,
    and older ones need an explicit strategy for a callable class. One strategy
    behaves the same on every supported version.
    """
    import ray.data

    kwargs: dict[str, int] = {}
    if options.concurrency is not None:
        kwargs["size"] = options.concurrency
    if options.max_tasks_in_flight_per_actor is not None:
        kwargs["max_tasks_in_flight_per_actor"] = options.max_tasks_in_flight_per_actor
    return ray.data.ActorPoolStrategy(**kwargs)


def _actor_options(options: BatchOptions) -> dict[str, int]:
    """Ray actor options, passed only when set.

    A ``None`` value would override Ray's own default instead of keeping it, so
    unset options are left out entirely. Ray restarts a dead actor without limit
    and retries its in-flight block when ``max_restarts`` and ``max_task_retries``
    are omitted.
    """
    candidates = {
        "max_concurrency": options.max_concurrency,
        "max_restarts": options.max_restarts,
        "max_task_retries": options.max_task_retries,
    }
    return {k: v for k, v in candidates.items() if v is not None}
