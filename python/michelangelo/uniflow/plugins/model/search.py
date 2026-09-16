"""Python mirror of the model search plugin used by local workflows."""

from __future__ import annotations

from typing import Any

from michelangelo.api.v2 import APIClient
from michelangelo.uniflow.core import star_plugin


def _pipeline_run_query(pipeline_run_name: str) -> dict[str, str]:
    return {"field_selector": f"spec.source_pipeline_run.name={pipeline_run_name}"}


@star_plugin("model.get_models_by_pipeline_run")
def get_models_by_pipeline_run(
    namespace: str, pipeline_run_name: str
) -> list[dict[str, Any]]:
    """Return models produced by one pipeline run.

    The API query uses the Model resource's indexed pipeline-run provenance
    fields. Results are checked again client-side in case the server ignores
    extended list options, then sorted to keep workflow output deterministic.
    """
    if not namespace or not pipeline_run_name:
        raise ValueError('both "namespace" and "pipeline_run_name" are required')

    model_list = APIClient.ModelService.list_model(
        namespace=namespace,
        list_options=_pipeline_run_query(pipeline_run_name),
    )

    models = []
    for model in model_list.items:
        source = model.spec.source_pipeline_run
        model_namespace = model.metadata.namespace or namespace
        source_namespace = source.namespace or model_namespace
        if (
            model_namespace != namespace
            or source_namespace != namespace
            or source.name != pipeline_run_name
        ):
            continue
        models.append(
            {
                "name": model.metadata.name,
                "namespace": model_namespace,
                "revision_id": model.spec.revision_id,
            }
        )

    models.sort(key=lambda model: (model["name"], model["revision_id"]))
    if not models:
        raise RuntimeError(
            f"no models found for pipeline run {namespace}/{pipeline_run_name}"
        )
    return models
