"""Tests for michelangelo.uniflow.core.lib.deployment."""

import unittest
from unittest.mock import call, patch

import grpc

from michelangelo.gen.api.v2.deployment_pb2 import (
    Deployment,
    DeploymentSpec,
    DeploymentStage,
    DeploymentStatus,
)
from michelangelo.uniflow.core.lib.deployment import (
    create_or_update_deployment,
    model_deployment,
    wait_for_deployment,
)


def _not_found_error() -> grpc.RpcError:
    error = grpc.RpcError()
    error.code = lambda: grpc.StatusCode.NOT_FOUND
    return error


def _other_error() -> grpc.RpcError:
    error = grpc.RpcError()
    error.code = lambda: grpc.StatusCode.UNAVAILABLE
    return error


class CreateOrUpdateDeploymentTest(unittest.TestCase):
    """Tests for create_or_update_deployment."""

    @patch("michelangelo.uniflow.core.lib.deployment.APIClient")
    def test_update_path(self, mock_client):
        """An existing deployment is updated with the new desired revision."""
        existing = Deployment(spec=DeploymentSpec())
        mock_client.DeploymentService.get_deployment.return_value = existing

        result = create_or_update_deployment(
            namespace="ns",
            deployment_name="dep",
            model_revision_name="rev-2",
        )

        self.assertEqual(
            result, {"deployment_name": "dep", "model_revision_name": "rev-2"}
        )
        mock_client.DeploymentService.get_deployment.assert_called_once_with(
            namespace="ns", name="dep"
        )
        mock_client.DeploymentService.update_deployment.assert_called_once()
        updated = mock_client.DeploymentService.update_deployment.call_args.kwargs[
            "deployment"
        ]
        self.assertEqual(updated.spec.desired_revision.name, "rev-2")
        # Status must be reset, not carried over from the stale read.
        self.assertEqual(updated.status, DeploymentStatus())

    @patch("michelangelo.uniflow.core.lib.deployment.APIClient")
    def test_create_path_clones_template(self, mock_client):
        """A missing deployment is created by cloning the given template."""
        template = Deployment(
            spec=DeploymentSpec(),
        )
        template.metadata.labels["team"] = "michelangelo"

        mock_client.DeploymentService.get_deployment.side_effect = [
            _not_found_error(),
            template,
        ]

        result = create_or_update_deployment(
            namespace="ns",
            deployment_name="new-dep",
            model_revision_name="rev-1",
            deployment_template="template-dep",
        )

        self.assertEqual(
            result,
            {"deployment_name": "new-dep", "model_revision_name": "rev-1"},
        )
        mock_client.DeploymentService.get_deployment.assert_has_calls(
            [
                call(namespace="ns", name="new-dep"),
                call(namespace="ns", name="template-dep"),
            ]
        )
        mock_client.DeploymentService.create_deployment.assert_called_once()
        created = mock_client.DeploymentService.create_deployment.call_args.kwargs[
            "deployment"
        ]
        self.assertEqual(created.metadata.name, "new-dep")
        self.assertEqual(created.metadata.labels["team"], "michelangelo")
        self.assertEqual(created.spec.desired_revision.name, "rev-1")

    @patch("michelangelo.uniflow.core.lib.deployment.APIClient")
    def test_create_path_without_template_raises(self, mock_client):
        """Creating a missing deployment without a template is rejected."""
        mock_client.DeploymentService.get_deployment.side_effect = _not_found_error()

        with self.assertRaisesRegex(ValueError, "deployment_template required"):
            create_or_update_deployment(
                namespace="ns",
                deployment_name="new-dep",
                model_revision_name="rev-1",
            )

    @patch("michelangelo.uniflow.core.lib.deployment.APIClient")
    def test_hard_failure_is_propagated(self, mock_client):
        """A non-NOT_FOUND error is raised, not treated as "doesn't exist"."""
        mock_client.DeploymentService.get_deployment.side_effect = _other_error()

        with self.assertRaises(grpc.RpcError):
            create_or_update_deployment(
                namespace="ns",
                deployment_name="dep",
                model_revision_name="rev-1",
                deployment_template="template-dep",
            )
        mock_client.DeploymentService.create_deployment.assert_not_called()


class WaitForDeploymentTest(unittest.TestCase):
    """Tests for wait_for_deployment."""

    @patch("michelangelo.uniflow.core.lib.deployment.time.sleep", return_value=None)
    @patch("michelangelo.uniflow.core.lib.deployment.APIClient")
    def test_success(self, mock_client, mock_sleep):
        """A terminal success stage returns immediately without polling."""
        deployment = Deployment(spec=DeploymentSpec())
        deployment.spec.desired_revision.name = "rev-1"
        deployment.status.stage = DeploymentStage.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
        deployment.status.current_revision.name = "rev-1"
        mock_client.DeploymentService.get_deployment.return_value = deployment

        result = wait_for_deployment(
            namespace="ns",
            deployment_name="dep",
            expected_model_revision_name="rev-1",
        )

        self.assertEqual(
            result,
            {
                "stage": "DEPLOYMENT_STAGE_ROLLOUT_COMPLETE",
                "current_revision": "rev-1",
                "desired_revision": "rev-1",
            },
        )
        mock_sleep.assert_not_called()

    @patch("michelangelo.uniflow.core.lib.deployment.time.sleep", return_value=None)
    @patch("michelangelo.uniflow.core.lib.deployment.APIClient")
    def test_failed_stage_raises(self, mock_client, mock_sleep):
        """A terminal failure stage raises instead of returning success."""
        deployment = Deployment(spec=DeploymentSpec())
        deployment.spec.desired_revision.name = "rev-1"
        deployment.status.stage = DeploymentStage.DEPLOYMENT_STAGE_ROLLOUT_FAILED
        mock_client.DeploymentService.get_deployment.return_value = deployment

        with self.assertRaisesRegex(RuntimeError, "deployment failed with stage"):
            wait_for_deployment(
                namespace="ns",
                deployment_name="dep",
                expected_model_revision_name="rev-1",
            )

    @patch("michelangelo.uniflow.core.lib.deployment.time.sleep", return_value=None)
    @patch("michelangelo.uniflow.core.lib.deployment.APIClient")
    def test_revision_mismatch_raises_immediately(self, mock_client, mock_sleep):
        """A desired-revision change from another workflow aborts the wait."""
        deployment = Deployment(spec=DeploymentSpec())
        deployment.spec.desired_revision.name = "rev-2"
        deployment.status.stage = DeploymentStage.DEPLOYMENT_STAGE_PLACEMENT
        mock_client.DeploymentService.get_deployment.return_value = deployment

        with self.assertRaisesRegex(RuntimeError, "updated by another workflow"):
            wait_for_deployment(
                namespace="ns",
                deployment_name="dep",
                expected_model_revision_name="rev-1",
            )
        mock_sleep.assert_not_called()

    @patch("michelangelo.uniflow.core.lib.deployment.time.sleep", return_value=None)
    @patch("michelangelo.uniflow.core.lib.deployment.APIClient")
    def test_non_terminal_then_success_polls(self, mock_client, mock_sleep):
        """A non-terminal stage is retried until a terminal stage is seen."""
        pending = Deployment(spec=DeploymentSpec())
        pending.spec.desired_revision.name = "rev-1"
        pending.status.stage = DeploymentStage.DEPLOYMENT_STAGE_PLACEMENT

        done = Deployment(spec=DeploymentSpec())
        done.spec.desired_revision.name = "rev-1"
        done.status.stage = DeploymentStage.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
        done.status.current_revision.name = "rev-1"

        mock_client.DeploymentService.get_deployment.side_effect = [pending, done]

        result = wait_for_deployment(
            namespace="ns",
            deployment_name="dep",
            expected_model_revision_name="rev-1",
            poll=5,
        )

        self.assertEqual(result["stage"], "DEPLOYMENT_STAGE_ROLLOUT_COMPLETE")
        mock_sleep.assert_called_once_with(5)

    @patch(
        "michelangelo.uniflow.core.lib.deployment.time.time",
        side_effect=[0, 0, 100],
    )
    @patch("michelangelo.uniflow.core.lib.deployment.time.sleep", return_value=None)
    @patch("michelangelo.uniflow.core.lib.deployment.APIClient")
    def test_timeout_raises(self, mock_client, mock_sleep, mock_time):
        """Waiting past the configured timeout raises rather than looping forever."""
        pending = Deployment(spec=DeploymentSpec())
        pending.spec.desired_revision.name = "rev-1"
        pending.status.stage = DeploymentStage.DEPLOYMENT_STAGE_PLACEMENT
        mock_client.DeploymentService.get_deployment.return_value = pending

        with self.assertRaisesRegex(RuntimeError, "timeout waiting for deployment"):
            wait_for_deployment(
                namespace="ns",
                deployment_name="dep",
                expected_model_revision_name="rev-1",
                timeout=10,
            )


class ModelDeploymentTest(unittest.TestCase):
    """Tests for the model_deployment workflow."""

    @patch("michelangelo.uniflow.core.lib.deployment.wait_for_deployment")
    @patch("michelangelo.uniflow.core.lib.deployment.create_or_update_deployment")
    def test_orchestrates_create_then_wait(self, mock_create, mock_wait):
        """The workflow creates/updates the deployment, then waits on it."""
        mock_create.return_value = {
            "deployment_name": "dep",
            "model_revision_name": "rev-1",
        }
        mock_wait.return_value = {
            "stage": "DEPLOYMENT_STAGE_ROLLOUT_COMPLETE",
            "current_revision": "rev-1",
            "desired_revision": "rev-1",
        }

        result = model_deployment(
            namespace="ns",
            deployment_name="dep",
            model_revision_name="rev-1",
            deployment_template="template-dep",
        )

        mock_create.assert_called_once_with(
            namespace="ns",
            deployment_name="dep",
            model_revision_name="rev-1",
            deployment_template="template-dep",
        )
        mock_wait.assert_called_once_with(
            namespace="ns",
            deployment_name="dep",
            expected_model_revision_name="rev-1",
            timeout=31536000,
        )
        self.assertEqual(
            result,
            {
                "deployment_name": "dep",
                "model_revision_name": "rev-1",
                "final_stage": "DEPLOYMENT_STAGE_ROLLOUT_COMPLETE",
            },
        )

    @patch("michelangelo.uniflow.core.lib.deployment.wait_for_deployment")
    @patch("michelangelo.uniflow.core.lib.deployment.create_or_update_deployment")
    def test_async_deployment_skips_wait(self, mock_create, mock_wait):
        """async_deployment=True returns immediately without waiting."""
        mock_create.return_value = {
            "deployment_name": "dep",
            "model_revision_name": "rev-1",
        }

        result = model_deployment(
            namespace="ns",
            deployment_name="dep",
            model_revision_name="rev-1",
            async_deployment=True,
        )

        mock_wait.assert_not_called()
        self.assertEqual(result["final_stage"], "ASYNC_DEPLOYMENT_STARTED")


if __name__ == "__main__":
    unittest.main()
