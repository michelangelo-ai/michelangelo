"""Ray ``DataContext`` tuning shared across Ray-based workflow tasks.

Configures Ray Data block sizing, I/O retry patterns, and streaming-executor
memory/scheduling knobs. This is a general-purpose Ray Data helper — not
specific to any single workflow task — so it lives alongside the other Ray
plugin utilities (:mod:`~michelangelo.uniflow.plugins.ray.io`,
:mod:`~michelangelo.uniflow.plugins.ray.native_transform`) rather than being
duplicated into each task that reads large Parquet datasets via Ray Data.
"""

from __future__ import annotations

import logging

import ray

_logger = logging.getLogger(__name__)

__all__ = ["RETRIED_IO_ERRORS", "set_ray_data_context"]


RETRIED_IO_ERRORS = [
    "OSError: Could not open Parquet input source",
    "Failed to get file info for",
    "Error opening input stream",
]
"""Transient I/O error message substrings that Ray Data should retry.

These occur against distributed/object-store filesystem backends (e.g. S3,
GCS, or an on-prem distributed filesystem) when the backend is saturated by
large file counts, or during credential refresh / connection drops.
"""


def set_ray_data_context(
    min_block_size: int | None = None,
    max_block_size: int | None = None,
    retried_io_errors: list[str] | None = None,
    object_store_memory_limit: int | None = None,
    wait_for_min_actors_s: int | None = None,
) -> None:
    """Configure Ray ``DataContext`` with block sizes and retried I/O error patterns.

    Shared utility for Ray-based workflow steps (native transform, trainer,
    inference, etc.) that use Ray Data for Parquet I/O over a distributed or
    object-store filesystem.

    Args:
        min_block_size: Target min block size in bytes. If ``None``, Ray's
            default is used.
        max_block_size: Target max block size in bytes. If ``None``, Ray's
            default is used.
        retried_io_errors: Additional I/O error patterns to retry on. If
            ``None``, uses the default patterns in :data:`RETRIED_IO_ERRORS`.
            Pass an empty list to skip adding extra patterns beyond Ray's own
            defaults.
        object_store_memory_limit: Upper bound in bytes on the object store
            memory the streaming executor may use for buffered (pending)
            blocks across all operators. When buffered output reaches this
            size, upstream read tasks are backpressured and stop launching
            until a downstream operator (e.g. a GPU predictor) drains
            blocks. This bounds the worst-case memory peak: during actor
            warmup the predictor consumes nothing, so without a bound the
            readers fill the entire object store and keep decompressing,
            then the actor model loads land on top and OOM-kill the node.
            ``None`` leaves Ray's default (unbounded, i.e. capped only by
            the physical object store).
        wait_for_min_actors_s: Seconds the executor blocks for an actor-pool
            operator's actors to finish provisioning (model load) before it
            begins scheduling upstream read tasks. This decouples actor
            warmup from reader ramp-up: the model loads (large transient
            memory) happen first, then readers start, so the two memory
            peaks never overlap and OOM the node. ``None`` keeps Ray's
            default (no wait; actors provision asynchronously while reads
            run, which lets readers race the warmup).
    """
    ctx = ray.data.DataContext.get_current()
    # Only override block sizes when explicitly provided; otherwise keep Ray's
    # auto-tuned defaults.
    if min_block_size is not None:
        ctx.target_min_block_size = min_block_size
    if max_block_size is not None:
        ctx.target_max_block_size = max_block_size

    extra_errors = (
        retried_io_errors if retried_io_errors is not None else RETRIED_IO_ERRORS
    )
    if extra_errors:
        ctx.retried_io_errors = [
            *ctx.retried_io_errors,
            *extra_errors,
        ]

    # Cap the executor's buffered-block budget so readers backpressure instead of
    # racing an actor pool's warmup and OOM-killing the node. See the arg
    # docstring for the failure mode.
    # ExecutionResources.object_store_memory is a read-only property on this Ray
    # version, so we replace resource_limits with a new instance (the setter
    # fills the unset cpu/gpu/memory fields with inf, leaving them unbounded)
    # rather than assigning the property in place.
    if object_store_memory_limit is not None:
        limits = ctx.execution_options.resource_limits
        ctx.execution_options.resource_limits = type(limits)(
            object_store_memory=object_store_memory_limit
        )

    # Make actor-pool operators block on model load before reads are scheduled, so
    # reader heap and actor model-load memory don't peak at the same time.
    if wait_for_min_actors_s is not None:
        ctx.wait_for_min_actors_s = wait_for_min_actors_s

    _logger.info(
        "Ray data context set: target_min_block_size=%s MB, "
        "target_max_block_size=%s MB, object_store_memory_limit=%s, "
        "wait_for_min_actors_s=%s, retried_io_errors=%s",
        f"{ctx.target_min_block_size / (1024 * 1024):.0f}"
        if ctx.target_min_block_size
        else "ray-default",
        f"{ctx.target_max_block_size / (1024 * 1024):.0f}"
        if ctx.target_max_block_size
        else "ray-default",
        object_store_memory_limit,
        wait_for_min_actors_s,
        ctx.retried_io_errors,
    )
