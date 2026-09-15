"""Tests for local model lookup by source pipeline run."""

from types import SimpleNamespace
from unittest.mock import MagicMock, patch

import pytest
from google.protobuf.wrappers_pb2 import StringValue

from michelangelo.api.v2.services.gen.model import ModelService
from michelangelo.gen.api.options_pb2 import ResourceIdentifier
from michelangelo.gen.api.v2.model_pb2 import Model, ModelList, ModelSpec
from michelangelo.gen.k8s.io.apimachinery.pkg.apis.meta.v1.generated_pb2 import (
    ObjectMeta,
)
from michelangelo.uniflow.plugins.model.search import get_models_by_pipeline_run

_MODULE = "michelangelo.uniflow.plugins.model.search"


def _model(name: str, revision_id: int, run_name: str = "child-run") -> Model:
    return Model(
        metadata=ObjectMeta(name=name, namespace="default"),
        spec=ModelSpec(
            revision_id=revision_id,
            source_pipeline_run=ResourceIdentifier(
                name=run_name,
                namespace="default",
            ),
        ),
    )


@patch(f"{_MODULE}.APIClient.ModelService.list_model")
def test_get_models_by_pipeline_run_queries_provenance_and_sorts(mock_list_model):
    """The plugin queries both indexed fields and returns stable identities."""
    mock_list_model.return_value = ModelList(
        items=[_model("model-z", 2), _model("model-a", 3)]
    )

    result = get_models_by_pipeline_run("default", "child-run")

    assert result == [
        {"name": "model-a", "namespace": "default", "revision_id": 3},
        {"name": "model-z", "namespace": "default", "revision_id": 2},
    ]
    query = mock_list_model.call_args.kwargs["list_options_ext"]["operation"]
    assert [criterion.field_name for criterion in query.criterion] == [
        "model.spec.source_pipeline_run.namespace",
        "model.spec.source_pipeline_run.name",
    ]
    values = []
    for criterion in query.criterion:
        value = StringValue()
        assert criterion.match_value.Unpack(value)
        values.append(value.value)
    assert values == ["default", "child-run"]


@patch(f"{_MODULE}.APIClient.ModelService.list_model")
def test_get_models_by_pipeline_run_rechecks_server_results(mock_list_model):
    """An ignored server-side filter cannot return unrelated models."""
    mock_list_model.return_value = ModelList(items=[_model("wrong", 1, "other-run")])

    with pytest.raises(RuntimeError, match=r"no models found.*default/child-run"):
        get_models_by_pipeline_run("default", "child-run")


def test_pipeline_run_query_survives_generated_client_encoding():
    """The generated client preserves the operation only for the dict path."""
    context = MagicMock()
    context.header_provider.get_headers.return_value = {}
    service = ModelService(context)
    service._service_stub = MagicMock()
    service._service_stub.ListModel.return_value = SimpleNamespace(
        model_list=ModelList(items=[_model("trained-model", 4)])
    )

    with patch(f"{_MODULE}.APIClient.ModelService", service):
        result = get_models_by_pipeline_run("default", "child-run")

    assert result[0]["name"] == "trained-model"
    request = service._service_stub.ListModel.call_args.args[0]
    assert [
        criterion.field_name
        for criterion in request.list_options_ext.operation.criterion
    ] == [
        "model.spec.source_pipeline_run.namespace",
        "model.spec.source_pipeline_run.name",
    ]


@pytest.mark.parametrize("namespace,pipeline_run_name", [("", "run"), ("ns", "")])
def test_get_models_by_pipeline_run_requires_both_identifiers(
    namespace, pipeline_run_name
):
    """Both parts of the source PipelineRun identity are required."""
    with pytest.raises(ValueError, match=r"namespace.*pipeline_run_name"):
        get_models_by_pipeline_run(namespace, pipeline_run_name)
