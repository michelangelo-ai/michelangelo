"""Tests for the Uniflow retrain example."""

import importlib
import importlib.resources
import sys
from pathlib import Path
from types import ModuleType
from unittest.mock import Mock, patch

import yaml

import michelangelo.uniflow.core as uniflow
from examples.retrain_example.retrain import retrain_workflow
from michelangelo.uniflow.core.build import build
from michelangelo.uniflow.plugins.ray import RayTask
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

    assert "__pipeline__='pipeline'" in source
    assert "__model__='model'" in source
    assert "__deployment__='deployment'" in source
    assert "__pipeline__.run_pipeline(" in source
    assert "__model__.get_models_by_pipeline_run(" in source
    assert "__deployment__.create_or_update_deployment(" in source
    assert "__deployment__.wait_for_deployment(" in source


@uniflow.task(config=RayTask())
def _load_data(path, name, tokenizer_max_length):
    return "train", "validation", "test"


@uniflow.task(config=RayTask())
def _train(train_data, validation_data, test_data):
    return "result", "model"


@uniflow.task(config=RayTask(worker_instances=0))
def _assembler(model_variable, lr, eps, tokenizer_max_length):
    return "assembled"


@uniflow.task(config=RayTask(worker_instances=0))
def _push_step(assembled):
    return ["pushed"]


def _module(name, attribute, value):
    module = ModuleType(name)
    setattr(module, attribute, value)
    return module


def test_training_adaptation_uses_head_only_ray_tasks():
    """The e2e training image is pulled onto one sandbox node only."""
    modules = {
        "examples.bert_cola.assembler": _module(
            "examples.bert_cola.assembler", "assembler", _assembler
        ),
        "examples.bert_cola.data": _module(
            "examples.bert_cola.data", "load_data", _load_data
        ),
        "examples.bert_cola.push": _module(
            "examples.bert_cola.push", "push_step", _push_step
        ),
        "examples.bert_cola.train": _module(
            "examples.bert_cola.train", "train", _train
        ),
    }
    with patch.dict(sys.modules, modules):
        sys.modules.pop("examples.retrain_example.training", None)
        training = importlib.import_module("examples.retrain_example.training")
        package = build(training.train_workflow)
    source = package.files[package.main_file].decode("utf-8")

    assert "alias='retrain_load_data'" in source
    assert "alias='retrain_train'" in source
    assert "examples.retrain_example.training.assembler" in source
    assert source.count("worker_instances=0") == 4

    load_data = Mock()
    load_data.with_overrides.return_value.return_value = (
        "train",
        "validation",
        "test",
    )
    train = Mock()
    train.with_overrides.return_value.return_value = ("result", "model")
    assembler = Mock(return_value="assembled")
    push_step = Mock(return_value=["pushed"])
    with (
        patch.object(training, "_load_data", load_data),
        patch.object(training, "_train", train),
        patch.object(training, "assembler", assembler),
        patch.object(training, "push_step", push_step),
    ):
        training.train_workflow()

    assert load_data.with_overrides.call_args.kwargs["config"].worker_instances == 0
    assert train.with_overrides.call_args.kwargs["config"].worker_instances == 0
    assembler.assert_called_once_with(
        "model", lr=2e-5, eps=1e-8, tokenizer_max_length=128
    )
    push_step.assert_called_once_with("assembled")


def test_training_adaptation_uses_stdlib_resources_as_backport_fallback():
    """Python 3.9 task images can import Matplotlib without the backport."""
    from examples.retrain_example import training

    existing = sys.modules.pop("importlib_resources", None)
    try:
        with patch.object(
            training.importlib,
            "import_module",
            side_effect=ModuleNotFoundError("missing backport"),
        ):
            training._ensure_importlib_resources()
        assert sys.modules["importlib_resources"] is importlib.resources
    finally:
        sys.modules.pop("importlib_resources", None)
        if existing is not None:
            sys.modules["importlib_resources"] = existing


def test_retrain_registration_discovers_workflow():
    """Registration selects retrain rather than an imported plugin function."""
    config_path = str(_EXAMPLE_DIR / "pipeline.yaml")
    workflow = discover_workflow_from_config(config_path)

    with ConfigBuilder.from_config_file(config_path) as config_builder:
        configured_workflow = config_builder.workflow_function_obj

    assert workflow is retrain_workflow
    assert configured_workflow is retrain_workflow


@patch(f"{_MODULE}.wait_for_deployment")
@patch(f"{_MODULE}.create_or_update_deployment")
@patch(f"{_MODULE}.get_models_by_pipeline_run")
@patch(
    f"{_MODULE}.run_pipeline",
    return_value={
        "metadata": {"name": "run-20260914-120000-a1b2c3d4", "namespace": "default"},
        "status": {"state": "PIPELINE_RUN_STATE_SUCCEEDED"},
    },
)
def test_retrain_runs_child_then_deploys_its_model(
    mock_run_pipeline,
    mock_get_models,
    mock_create_deployment,
    mock_wait_for_deployment,
):
    """The local workflow bridges the exact child run name to deployment."""
    mock_get_models.return_value = [
        {"name": "bert-cola-model", "namespace": "default", "revision_id": 12}
    ]
    mock_create_deployment.return_value = {
        "deployment_name": "retrain-example",
        "model_revision_name": "bert-cola-model",
    }
    mock_wait_for_deployment.return_value = {
        "stage": "DEPLOYMENT_STAGE_ROLLOUT_COMPLETE",
        "current_revision": "bert-cola-model",
        "desired_revision": "bert-cola-model",
    }
    result = retrain_workflow(
        namespace="default",
        retrainer_pipeline="bert-cola-test",
        deployment_name="retrain-example",
        deployment_template="deployment-example",
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
    mock_get_models.assert_called_once_with(
        namespace="default",
        pipeline_run_name="run-20260914-120000-a1b2c3d4",
    )
    mock_create_deployment.assert_called_once_with(
        namespace="default",
        deployment_name="retrain-example",
        model_revision_name="bert-cola-model",
        deployment_template="deployment-example",
    )
    mock_wait_for_deployment.assert_called_once_with(
        namespace="default",
        deployment_name="retrain-example",
        expected_model_revision_name="bert-cola-model",
        timeout=1800,
        poll=5,
    )
    assert result["pipeline_run"]["status"]["state"] == ("PIPELINE_RUN_STATE_SUCCEEDED")
    assert result["model_name"] == "bert-cola-model"
    assert result["deployment_name"] == "retrain-example"
    assert result["deployment_stage"] == "DEPLOYMENT_STAGE_ROLLOUT_COMPLETE"


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
        "examples.retrain_example.training"
    )
