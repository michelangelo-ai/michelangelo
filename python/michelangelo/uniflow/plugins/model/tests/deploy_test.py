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

    def test_existing_deployment_updates_model_and_actor(self):
        """An existing deployment advances to the new model and actor."""
        existing = _complete_deployment("old-model")
        existing.spec.owner.name = "old-actor"
        complete = _complete_deployment()
        self.deployment_service.get_deployment.side_effect = [existing, complete]
        self.deployment_service.update_deployment.side_effect = (
            lambda deployment, update_options: deployment
        )

        with self._api_patch(), patch(f"{_MODULE}.time.sleep"):
            result = deploy_model(
                namespace="default",
                deployment_name="retrain-deployment",
                pipeline_run_name="child-run",
                inference_server_name="inference-server",
                actor="new-actor",
                timeout_seconds=60,
                poll_seconds=1,
            )

        updated = self.deployment_service.update_deployment.call_args.kwargs[
            "deployment"
        ]
        self.assertEqual(updated.spec.desired_revision.name, "trained-model")
        self.assertEqual(updated.spec.owner.name, "new-actor")
        self.assertEqual(result["model"]["name"], "trained-model")

    def test_get_deployment_unexpected_error_is_propagated(self):
        """Only a not-found read enters the create path."""
        self.deployment_service.get_deployment.side_effect = _RpcError(
            grpc.StatusCode.PERMISSION_DENIED
        )

        with self._api_patch(), self.assertRaises(grpc.RpcError):
            deploy_model(
                namespace="default",
                deployment_name="retrain-deployment",
                pipeline_run_name="child-run",
                inference_server_name="inference-server",
                timeout_seconds=60,
            )

    def test_create_conflict_retries_as_update(self):
        """A concurrent creator is resolved by reading and updating its object."""
        existing = _complete_deployment("old-model")
        complete = _complete_deployment()
        self.deployment_service.get_deployment.side_effect = [
            _RpcError(grpc.StatusCode.NOT_FOUND),
            existing,
            complete,
        ]
        self.deployment_service.create_deployment.side_effect = _RpcError(
            grpc.StatusCode.ALREADY_EXISTS
        )
        self.deployment_service.update_deployment.side_effect = (
            lambda deployment, update_options: deployment
        )

        with self._api_patch(), patch(f"{_MODULE}.time.sleep"):
            result = deploy_model(
                namespace="default",
                deployment_name="retrain-deployment",
                pipeline_run_name="child-run",
                inference_server_name="inference-server",
                timeout_seconds=60,
                poll_seconds=1,
            )

        self.assertEqual(self.deployment_service.get_deployment.call_count, 3)
        self.assertEqual(self.deployment_service.update_deployment.call_count, 1)
        self.assertEqual(result["status"]["state"], "DEPLOYMENT_STATE_HEALTHY")

    def test_create_unexpected_error_is_propagated(self):
        """Create failures other than a concurrent create are not hidden."""
        self.deployment_service.get_deployment.side_effect = _RpcError(
            grpc.StatusCode.NOT_FOUND
        )
        self.deployment_service.create_deployment.side_effect = _RpcError(
            grpc.StatusCode.INTERNAL
        )

        with self._api_patch(), self.assertRaises(grpc.RpcError):
            deploy_model(
                namespace="default",
                deployment_name="retrain-deployment",
                pipeline_run_name="child-run",
                inference_server_name="inference-server",
                timeout_seconds=60,
            )

    def test_update_conflict_retries(self):
        """A resource-version conflict is retried from a fresh deployment read."""
        first = _complete_deployment("old-model")
        second = _complete_deployment("old-model")
        complete = _complete_deployment()
        self.deployment_service.get_deployment.side_effect = [first, second, complete]
        self.deployment_service.update_deployment.side_effect = [
            _RpcError(grpc.StatusCode.FAILED_PRECONDITION),
            second,
        ]

        with self._api_patch(), patch(f"{_MODULE}.time.sleep"):
            result = deploy_model(
                namespace="default",
                deployment_name="retrain-deployment",
                pipeline_run_name="child-run",
                inference_server_name="inference-server",
                timeout_seconds=60,
                poll_seconds=1,
            )

        self.assertEqual(self.deployment_service.update_deployment.call_count, 2)
        self.assertEqual(result["model"]["name"], "trained-model")

    def test_update_unexpected_error_is_propagated(self):
        """Non-conflict update failures are not retried or hidden."""
        self.deployment_service.get_deployment.return_value = _complete_deployment(
            "old-model"
        )
        self.deployment_service.update_deployment.side_effect = _RpcError(
            grpc.StatusCode.INTERNAL
        )

        with self._api_patch(), self.assertRaises(grpc.RpcError):
            deploy_model(
                namespace="default",
                deployment_name="retrain-deployment",
                pipeline_run_name="child-run",
                inference_server_name="inference-server",
                timeout_seconds=60,
            )

    def test_poll_retries_transient_error_and_pending_state(self):
        """Transient reads and an incomplete rollout are polled again."""
        existing = _complete_deployment()
        pending = _complete_deployment()
        pending.status.stage = 0
        complete = _complete_deployment()
        self.deployment_service.get_deployment.side_effect = [
            existing,
            _RpcError(grpc.StatusCode.UNAVAILABLE),
            pending,
            complete,
        ]

        with self._api_patch(), patch(f"{_MODULE}.time.sleep") as sleep:
            result = deploy_model(
                namespace="default",
                deployment_name="retrain-deployment",
                pipeline_run_name="child-run",
                inference_server_name="inference-server",
                timeout_seconds=60,
                poll_seconds=1,
            )

        self.assertEqual(sleep.call_count, 2)
        self.assertEqual(result["status"]["stage"], "DEPLOYMENT_STAGE_ROLLOUT_COMPLETE")

    def test_poll_unexpected_error_reports_details(self):
        """A non-transient poll failure includes the server details."""
        self.deployment_service.get_deployment.side_effect = [
            _complete_deployment(),
            _RpcError(grpc.StatusCode.PERMISSION_DENIED, "access denied"),
        ]

        with (
            self._api_patch(),
            self.assertRaisesRegex(RuntimeError, "access denied"),
        ):
            deploy_model(
                namespace="default",
                deployment_name="retrain-deployment",
                pipeline_run_name="child-run",
                inference_server_name="inference-server",
                timeout_seconds=60,
            )

    def test_poll_timeout(self):
        """An incomplete rollout fails after the caller's deadline."""
        pending = _complete_deployment()
        pending.status.stage = 0
        self.deployment_service.get_deployment.side_effect = [
            _complete_deployment(),
            pending,
        ]

        with (
            self._api_patch(),
            patch(f"{_MODULE}.time.monotonic", side_effect=[0, 0, 1]),
            patch(f"{_MODULE}.time.sleep"),
            self.assertRaises(TimeoutError),
        ):
            deploy_model(
                namespace="default",
                deployment_name="retrain-deployment",
                pipeline_run_name="child-run",
                inference_server_name="inference-server",
                timeout_seconds=1,
                poll_seconds=1,
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

    def test_rejects_invalid_arguments(self):
        """Required names and optional controls are validated before API calls."""
        cases = [
            ({"namespace": ""}, "namespace must be a non-empty string"),
            ({"model_name": ""}, "model_name must be non-empty"),
            ({"timeout_seconds": -1}, "timeout_seconds must be non-negative"),
        ]
        defaults = {
            "namespace": "default",
            "deployment_name": "retrain-deployment",
            "pipeline_run_name": "child-run",
            "inference_server_name": "inference-server",
            "timeout_seconds": 60,
        }

        for overrides, message in cases:
            with (
                self.subTest(overrides=overrides),
                self.assertRaisesRegex(ValueError, message),
            ):
                deploy_model(**(defaults | overrides))

    def test_zero_timeout_uses_default(self):
        """The public zero timeout preserves the effectively-unbounded default."""
        with (
            patch(f"{_MODULE}._resolve_model", return_value=_model()),
            patch(f"{_MODULE}._create_or_update_deployment"),
            patch(f"{_MODULE}._poll_deployment", return_value={}) as poll,
        ):
            deploy_model(
                namespace="default",
                deployment_name="retrain-deployment",
                pipeline_run_name="child-run",
                inference_server_name="inference-server",
            )

        self.assertGreater(poll.call_args.args[3], 365 * 24 * 60 * 60)

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
