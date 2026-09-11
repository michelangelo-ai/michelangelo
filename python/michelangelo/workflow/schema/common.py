"""Config-layer incremental-training schema types shared across workflow tasks.

These types are parsed directly from pipeline configuration and are
intentionally distinct from any runtime-layer incremental-training state a
trainer task might construct for its own model training — the two live at
different layers (parsed configuration vs. constructed runtime state) and
are not meant to be unified. Kept here (rather than under a single task's
schema module) because more than one workflow task's config parsing is
expected to reuse them, matching the shared-then-task-scoped split already
used for e.g. ``ray_data_io.py``.
"""

from __future__ import annotations

from dataclasses import dataclass
from enum import Enum

__all__ = [
    "IncrementalTrainingConfig",
    "TrainingTypeConfig",
]


class TrainingTypeConfig(str, Enum):
    """Incremental-training mode, as parsed from pipeline configuration.

    A config-layer value — distinct from (and not interchangeable with) any
    runtime-layer incremental-training enum a trainer task might construct
    from it. Trainer tasks that build their own runtime training state are
    expected to derive it from their own config, not from this type.

    Attributes:
        BASE: A base run whose fitted transform can later be reused/refit by
            an ``INCREMENTAL`` run.
        INCREMENTAL: Reuse (and optionally selectively refit) a base run's
            fitted transform spec and feature statistics.

    Example:
        >>> TrainingTypeConfig.INCREMENTAL.value
        'INCREMENTAL'
    """

    BASE = "BASE"
    INCREMENTAL = "INCREMENTAL"


@dataclass
class IncrementalTrainingConfig:
    """Incremental-training configuration, as parsed from pipeline configuration.

    A config-layer type read directly from pipeline configuration for the
    consuming task. It is intentionally distinct from any runtime-layer
    incremental-training spec a trainer task might build for its own model
    training — the two live at different layers (parsed configuration vs.
    constructed runtime state) and are not meant to be unified. Scoped to
    only the fields shared by config-parsing consumers today: a more general
    equivalent configuration (as consumed by a model-initializer-style task
    elsewhere) would also carry fields like ``load_optimizer_weights`` and
    ``fused_model_submodule`` that are meaningless here — this config
    intentionally omits them rather than porting unused surface area.

    Attributes:
        training_type: Whether this run is a base run, an incremental run,
            or neither (unset, ``None``).
        baseline_model_uri: URI of the base run's raw model package, as
            returned by ``StorageBackend.upload()`` — passed directly to
            ``StorageBackend.download()`` to retrieve a base run's saved
            state from its metadata directory. Required when
            ``training_type == TrainingTypeConfig.INCREMENTAL``.
        enforce_full_reuse: When ``True`` and ``training_type ==
            TrainingTypeConfig.INCREMENTAL``, every layer in the (optional)
            inlined transform spec must use
            :attr:`~michelangelo.lib.native_transform.torch.transform_layer_spec.TransformerMode.REUSE`
            (or the default ``INVALID``, which behaves as ``REUSE``) — no
            refitting allowed. This is the only supported setting for now.

    Example:
        >>> IncrementalTrainingConfig(
        ...     training_type=TrainingTypeConfig.INCREMENTAL,
        ...     baseline_model_uri="s3://bucket/models/base-run/",
        ... ).enforce_full_reuse
        True
    """

    training_type: TrainingTypeConfig | None = None
    baseline_model_uri: str | None = None
    enforce_full_reuse: bool = True
