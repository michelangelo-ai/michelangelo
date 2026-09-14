"""Tests for the Uniflow retrain example."""

from pathlib import Path
from unittest.mock import patch

import yaml

from examples.retrain_example.retrain import retrain_workflow
from michelangelo.uniflow.core.build import build
from michelangelo.uniflow.registration.config_builder import ConfigBuilder
from michelangelo.uniflow.registration.subprocess import (
    discover_workflow_from_config,
)

_MODULE = "examples.retrain_example.retrain"
_EXAMPLE_DIR = Path(__file__).parents[2] / "examples" / "retrain_example"


def test_retrain_compiles_to_remote_plugins():
    """The remote package binds both plugin calls instead of Python bodies."""
    package = build(retrain_workflow)
    source = package.files[package.main_file].decode("utf-8")

    assert "load('@plugin', __pipeline__='pipeline', __model__='model')" in source
    assert "__pipeline__.run_pipeline(" in source
    assert "__model__.deploy_model(" in source


def test_retrain_registration_discovers_workflow():
    """Registration selects retrain rather than an imported plugin function."""
    config_path = str(_EXAMPLE_DIR / "pipeline.yaml")
    workflow = discover_workflow_from_config(config_path)

    with ConfigBuilder.from_config_file(config_path) as config_builder:
        configured_workflow = config_builder.workflow_function_obj

    assert workflow is retrain_workflow
    assert configured_workflow is retrain_workflow


@patch(
    f"{_MODULE}.deploy_model",
    return_value={
        "metadata": {"name": "retrain-example", "namespace": "default"},
        "model": {"name": "bert-cola-model-a1b2c3d4", "namespace": "default"},
        "status": {
            "state": "DEPLOYMENT_STATE_HEALTHY",
            "stage": "DEPLOYMENT_STAGE_ROLLOUT_COMPLETE",
        },
    },
)
@patch(
    f"{_MODULE}.run_pipeline",
    return_value={
        "metadata": {"name": "run-20260914-120000-a1b2c3d4", "namespace": "default"},
        "status": {"state": "PIPELINE_RUN_STATE_SUCCEEDED"},
    },
)
def test_retrain_runs_child_then_deploys_its_model(
    mock_run_pipeline, mock_deploy_model
):
    """The local workflow bridges the exact child run name to deployment."""
    result = retrain_workflow(
        namespace="default",
        retrainer_pipeline="bert-cola-test",
        deployment_name="retrain-example",
        inference_server_name="inference-server-example",
        path="nyu-mll/glue",
        name="cola",
        tokenizer_max_length=128,
        timeout_seconds=1800,
        poll_seconds=5,
    )

    mock_run_pipeline.assert_called_once_with(
        namespace="default",
        pipeline_name="bert-cola-test",
        kwargs={
            "path": "nyu-mll/glue",
            "name": "cola",
            "tokenizer_max_length": 128,
        },
        timeout_seconds=1800,
        poll_seconds=5,
    )
    mock_deploy_model.assert_called_once_with(
        namespace="default",
        deployment_name="retrain-example",
        pipeline_run_name="run-20260914-120000-a1b2c3d4",
        inference_server_name="inference-server-example",
        timeout_seconds=1800,
        poll_seconds=5,
    )
    assert result["pipeline_run"]["status"]["state"] == ("PIPELINE_RUN_STATE_SUCCEEDED")
    assert result["deployment"]["status"] == {
        "state": "DEPLOYMENT_STATE_HEALTHY",
        "stage": "DEPLOYMENT_STAGE_ROLLOUT_COMPLETE",
    }


def test_pipeline_resources_use_existing_training_code_in_one_namespace():
    """Retrain, training, and inference demo resources can coexist in default."""
    project = yaml.safe_load((_EXAMPLE_DIR / "project.yaml").read_text())
    retrain_pipeline = yaml.safe_load((_EXAMPLE_DIR / "pipeline.yaml").read_text())
    training_pipeline = yaml.safe_load(
        (_EXAMPLE_DIR / "training_pipeline.yaml").read_text()
    )

    assert project["metadata"] == {
        "namespace": "default",
        "name": "default",
        "annotations": {"michelangelo/worker_queue": "default"},
    }
    assert retrain_pipeline["metadata"] == {
        "namespace": "default",
        "name": "retrain-example",
        "annotations": {
            "michelangelo/uniflow-image": ("ghcr.io/michelangelo-ai/examples:main")
        },
    }
    assert retrain_pipeline["spec"]["type"] == "PIPELINE_TYPE_RETRAIN"
    assert training_pipeline["metadata"]["namespace"] == "default"
    assert training_pipeline["metadata"]["name"] == "bert-cola-test"
    assert training_pipeline["spec"]["manifest"]["filePath"] == (
        "examples.bert_cola.bert_cola"
    )
