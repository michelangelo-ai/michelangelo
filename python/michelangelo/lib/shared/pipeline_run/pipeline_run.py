"""Pipeline-run provenance for model registration.

When a task executes inside a Michelangelo pipeline, the worker injects the
pipeline run's identity into the task pod's environment. This module reads
that identity back so it can be attached to a registered model as
provenance — which pipeline run produced it.
"""

from __future__ import annotations

import os
from dataclasses import dataclass

__all__ = ["SourcePipelineRun", "get_source_pipeline_run"]

_ENV_PIPELINE_RUN_NAME = "MA_PIPELINE_RUN_NAME"
_ENV_NAMESPACE = "MA_NAMESPACE"


@dataclass(frozen=True)
class SourcePipelineRun:
    """Identifies the pipeline run that produced a model.

    Mirrors the shape of the model registry's ``ResourceIdentifier`` reference
    type: a required ``name`` and an optional ``namespace`` (when omitted,
    readers fall back to the same namespace as the resource that carries the
    reference).

    Attributes:
        name: Name of the pipeline run.
        namespace: Namespace the pipeline run lives in. ``None`` when the
            run's namespace was not available and the referencing resource's
            own namespace should be assumed instead.
    """

    name: str
    namespace: str | None = None


def get_source_pipeline_run() -> SourcePipelineRun | None:
    """Return the identity of the pipeline run executing this process, if any.

    Reads the ``MA_PIPELINE_RUN_NAME`` and ``MA_NAMESPACE`` environment
    variables, which are pod-injected identity values set by the pipeline
    worker for every task it launches.

    Both environment variables are optional: this accessor is also called by
    code that runs outside a pipeline (local development, unit tests), where
    neither variable is set.

    Returns:
        A :class:`SourcePipelineRun` built from the environment, or ``None``
        when ``MA_PIPELINE_RUN_NAME`` is unset or empty — a pipeline-run
        reference with no name does not identify anything, regardless of
        whether a namespace is present.
    """
    run_name = os.environ.get(_ENV_PIPELINE_RUN_NAME)
    if not run_name:
        return None
    namespace = os.environ.get(_ENV_NAMESPACE) or None
    return SourcePipelineRun(name=run_name, namespace=namespace)
