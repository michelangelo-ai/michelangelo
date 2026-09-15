"""Python mirror of the model search plugin used by local workflows."""

from __future__ import annotations

from typing import Any

from google.protobuf.any_pb2 import Any as AnyMessage
from google.protobuf.wrappers_pb2 import StringValue

from michelangelo.api.v2 import APIClient
from michelangelo.gen.api.list_pb2 import (
    CRITERION_OPERATOR_EQUAL,
    Criterion,
    CriterionOperation,
)
from michelangelo.uniflow.core import star_plugin


def _packed_string(value: str) -> AnyMessage:
    packed = AnyMessage()
    packed.Pack(StringValue(value=value))
    return packed


def _pipeline_run_query(
    namespace: str, pipeline_run_name: str
) -> dict[str, CriterionOperation]:
    # The generated ModelService client handles Criterion.match_value specially
    # only when the outer ListOptionsExt is supplied as a dictionary.
    return {
        "operation": CriterionOperation(
            criterion=[
                Criterion(
                    field_name="model.spec.source_pipeline_run.namespace",
                    match_value=_packed_string(namespace),
                    operator=CRITERION_OPERATOR_EQUAL,
                ),
                Criterion(
                    field_name="model.spec.source_pipeline_run.name",
                    match_value=_packed_string(pipeline_run_name),
                    operator=CRITERION_OPERATOR_EQUAL,
                ),
            ]
        )
    }


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
        list_options_ext=_pipeline_run_query(namespace, pipeline_run_name),
    )

    models = []
    for model in model_list.items:
        source = model.spec.source_pipeline_run
        if source.namespace != namespace or source.name != pipeline_run_name:
            continue
        models.append(
            {
                "name": model.metadata.name,
                "namespace": model.metadata.namespace or namespace,
                "revision_id": model.spec.revision_id,
            }
        )

    models.sort(key=lambda model: (model["name"], model["revision_id"]))
    if not models:
        raise RuntimeError(
            f"no models found for pipeline run {namespace}/{pipeline_run_name}"
        )
    return models
