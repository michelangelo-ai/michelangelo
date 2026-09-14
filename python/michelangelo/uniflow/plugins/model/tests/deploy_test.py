"""Tests for local model deployment orchestration."""

from types import SimpleNamespace
from unittest import TestCase
from unittest.mock import MagicMock, patch

import grpc

from michelangelo.gen.api.options_pb2 import ResourceIdentifier
from michelangelo.gen.api.v2.deployment_pb2 import (
    DEPLOYMENT_STAGE_ROLLBACK_COMPLETE,
    DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
    DEPLOYMENT_STATE_HEALTHY,
    Deployment,
)
from michelangelo.gen.api.v2.model_pb2 import Model
from michelangelo.uniflow.plugins.model.deploy import deploy_model

_MODULE = "michelangelo.uniflow.plugins.model.deploy"


class _RpcError(grpc.RpcError):
    def __init__(self, code, details="test error"):
        super().__init__()
        self._code = code
        self._details = details

    def code(self):
        return self._code

    def details(self):
        return self._details


def _model(name="trained-model", run_name="child-run"):
    model = Model()
    model.metadata.name = name
    model.metadata.namespace = "default"
    model.spec.source_pipeline_run.CopyFrom(
        ResourceIdentifier(namespace="default", name=run_name)
    )
    return model


def _complete_deployment(model_name="trained-model"):
    deployment = Deployment()
    deployment.metadata.name = "retrain-deployment"
    deployment.metadata.namespace = "default"
    deployment.spec.desired_revision.CopyFrom(
        ResourceIdentifier(namespace="default", name=model_name)
    )
    deployment.spec.inference_server.CopyFrom(
        ResourceIdentifier(namespace="default", name="inference-server")
    )
    deployment.status.current_revision.CopyFrom(
        ResourceIdentifier(namespace="default", name=model_name)
    )
    deployment.status.stage = DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
    deployment.status.state = DEPLOYMENT_STATE_HEALTHY
    return deployment


class TestDeployModel(TestCase):
    """Exercise model resolution, deployment upsert, and rollout polling."""

    def setUp(self):
        """Create isolated API service mocks."""
        self.model_service = MagicMock()
        self.model_service.list_model.return_value = SimpleNamespace(items=[_model()])
        self.deployment_service = MagicMock()

    def _api_patch(self):
        return patch.multiple(
            f"{_MODULE}.APIClient",
            ModelService=self.model_service,
            DeploymentService=self.deployment_service,
        )

    def test_create_and_wait_for_exact_model(self):
        """A missing deployment is created and polled to exact-model health."""
        complete = _complete_deployment()
        self.deployment_service.get_deployment.side_effect = [
            _RpcError(grpc.StatusCode.NOT_FOUND),
            complete,
        ]
        self.deployment_service.create_deployment.side_effect = (
            lambda deployment, create_options: deployment
        )

        with self._api_patch(), patch(f"{_MODULE}.time.sleep"):
            result = deploy_model(
                namespace="default",
                deployment_name="retrain-deployment",
                pipeline_run_name="child-run",
                inference_server_name="inference-server",
                actor="integration-test",
                timeout_seconds=60,
                poll_seconds=1,
            )

        created = self.deployment_service.create_deployment.call_args.kwargs[
            "deployment"
        ]
        self.assertEqual(created.spec.desired_revision.name, "trained-model")
        self.assertEqual(created.spec.inference_server.name, "inference-server")
        self.assertEqual(created.spec.owner.name, "integration-test")
        self.assertIsNotNone(created.spec.strategy.rolling)
        self.assertEqual(result["model"]["name"], "trained-model")
        self.assertEqual(result["status"]["stage"], "DEPLOYMENT_STAGE_ROLLOUT_COMPLETE")

        query = self.model_service.list_model.call_args.kwargs["list_options_ext"]
        self.assertEqual(len(query.operation.criterion), 2)

    def test_model_name_disambiguates_multiple_outputs(self):
        """An explicit model name selects one of multiple run outputs."""
        self.model_service.list_model.return_value = SimpleNamespace(
            items=[_model("first"), _model("second")]
        )
        self.deployment_service.get_deployment.side_effect = [
            _RpcError(grpc.StatusCode.NOT_FOUND),
            _complete_deployment("second"),
        ]
        self.deployment_service.create_deployment.side_effect = (
            lambda deployment, create_options: deployment
        )

        with self._api_patch(), patch(f"{_MODULE}.time.sleep"):
            result = deploy_model(
                namespace="default",
                deployment_name="retrain-deployment",
                pipeline_run_name="child-run",
                inference_server_name="inference-server",
                model_name="second",
                timeout_seconds=60,
                poll_seconds=1,
            )

        self.assertEqual(result["model"]["name"], "second")

    def test_multiple_outputs_require_model_name(self):
        """Ambiguous pipeline output fails instead of choosing by list order."""
        self.model_service.list_model.return_value = SimpleNamespace(
            items=[_model("first"), _model("second")]
        )
        with (
            self._api_patch(),
            self.assertRaisesRegex(
                RuntimeError, "produced multiple models.*model_name"
            ),
        ):
            deploy_model(
                namespace="default",
                deployment_name="retrain-deployment",
                pipeline_run_name="child-run",
                inference_server_name="inference-server",
                timeout_seconds=60,
            )

    def test_server_side_query_is_refiltered_by_exact_provenance(self):
        """Unexpected server results cannot bypass exact provenance matching."""
        wrong_run = _model("wrong", "other-run")
        wrong_namespace = _model("wrong-namespace")
        wrong_namespace.spec.source_pipeline_run.namespace = "other"
        self.model_service.list_model.return_value = SimpleNamespace(
            items=[wrong_run, wrong_namespace]
        )
        with (
            self._api_patch(),
            self.assertRaisesRegex(RuntimeError, "no model produced"),
        ):
            deploy_model(
                namespace="default",
                deployment_name="retrain-deployment",
                pipeline_run_name="child-run",
                inference_server_name="inference-server",
                timeout_seconds=60,
            )

    def test_existing_deployment_is_not_silently_retargeted(self):
        """An existing deployment cannot be moved to a different server."""
        existing = _complete_deployment("old-model")
        existing.spec.inference_server.name = "another-server"
        self.deployment_service.get_deployment.return_value = existing
        with (
            self._api_patch(),
            self.assertRaisesRegex(RuntimeError, "refusing to retarget"),
        ):
            deploy_model(
                namespace="default",
                deployment_name="retrain-deployment",
                pipeline_run_name="child-run",
                inference_server_name="inference-server",
                timeout_seconds=60,
            )

    def test_rollback_complete_is_a_failed_deploy(self):
        """A completed rollback means this requested rollout failed."""
        failed = _complete_deployment()
        failed.status.stage = DEPLOYMENT_STAGE_ROLLBACK_COMPLETE
        failed.status.message = "new model failed health checks"
        self.deployment_service.get_deployment.side_effect = [failed, failed]
        with (
            self._api_patch(),
            self.assertRaisesRegex(RuntimeError, "ROLLBACK_COMPLETE.*health checks"),
        ):
            deploy_model(
                namespace="default",
                deployment_name="retrain-deployment",
                pipeline_run_name="child-run",
                inference_server_name="inference-server",
                timeout_seconds=60,
                poll_seconds=1,
            )

    def test_rejects_invalid_poll_interval(self):
        """Polling must make forward progress."""
        with self.assertRaisesRegex(ValueError, "poll_seconds must be positive"):
            deploy_model(
                namespace="default",
                deployment_name="retrain-deployment",
                pipeline_run_name="child-run",
                inference_server_name="inference-server",
                poll_seconds=0,
            )
