import grpc
import time
from typing import Optional, Dict

from michelangelo.uniflow.core.decorator import star_plugin, workflow
from michelangelo.api.v2.client import APIClient
from michelangelo.gen.api.v2.deployment_pb2 import Deployment, DeploymentStatus, DeploymentStage
from michelangelo.gen.api.options_pb2 import ResourceIdentifier

# Deployment stage constants for success and failure detection
_DEPLOYMENT_SUCCESS_STAGES = {
    DeploymentStage.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
}

_DEPLOYMENT_FAILED_STAGES = {
    DeploymentStage.DEPLOYMENT_STAGE_ROLLOUT_FAILED,
    DeploymentStage.DEPLOYMENT_STAGE_ROLLBACK_FAILED,
    DeploymentStage.DEPLOYMENT_STAGE_CLEAN_UP_FAILED,
    DeploymentStage.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE,
    DeploymentStage.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE,
}


@star_plugin(binding="deployment.create_or_update_deployment")
def create_or_update_deployment(
    namespace: str,
    deployment_name: str,
    model_revision_name: str,
    deployment_template: Optional[str] = None
) -> Dict[str, str]:
    """Creates a new deployment or updates an existing one to deploy the specified model revision.

    For existing deployments: updates the deployment to use the new revision.
    For new deployments: clones from the specified template deployment with the desired revision.

    Args:
        namespace: The Michelangelo namespace for all resources
        deployment_name: The name of the deployment to create or update
        model_revision_name: The name of the model revision to deploy
        deployment_template: Template deployment to clone from (required for new deployments)

    Returns:
        dict: Dictionary containing deployment_name and model_revision_name

    Raises:
        ValueError: If deployment_template is required but not provided
        grpc.RpcError: For service call failures (network, auth, permissions, etc.)
    """
    # Check if the deployment already exists to decide between update and create paths.
    try:
        existing_deployment = APIClient.DeploymentService.get_deployment(namespace=namespace, name=deployment_name)
        deployment_exists = True
    except grpc.RpcError as e:
        if e.code() == grpc.StatusCode.NOT_FOUND:
            deployment_exists = False
        else:
            raise

    if deployment_exists:
        # Case 1: Deployment exists - Update path.
        # Update existing deployment to use the new desired model revision.
        existing_deployment.status.CopyFrom(DeploymentStatus())
        existing_deployment.metadata.ClearField("managedFields")
        existing_deployment.spec.desired_revision.CopyFrom(ResourceIdentifier(name=model_revision_name, namespace=namespace))
        APIClient.DeploymentService.update_deployment(deployment=existing_deployment)
    else:
        # Case 2: Deployment does not exist - Create path.
        # Create a new deployment by cloning from the specified template.
        if not deployment_template:
            raise ValueError("deployment_template required")

        # Create ResourceIdentifiers for consistent reference.
        source_deployment = ResourceIdentifier(name=deployment_template, namespace=namespace)
        model_revision = ResourceIdentifier(name=model_revision_name, namespace=namespace)

        # Retrieve the template deployment to clone from.
        template = APIClient.DeploymentService.get_deployment(namespace=namespace, name=source_deployment.name)

        # Clone the template and modify metadata for the new deployment.
        new_deployment = Deployment()
        new_deployment.CopyFrom(template)

        new_deployment.metadata.name = deployment_name
        new_deployment.metadata.resource_version = ""
        new_deployment.metadata.uid = ""
        new_deployment.metadata.creation_timestamp.Clear()
        new_deployment.metadata.deletion_timestamp.Clear()
        new_deployment.metadata.ClearField("managedFields")

        # Reset status fields for the new deployment.
        new_deployment.status.CopyFrom(DeploymentStatus())

        # Set the desired model revision for the new deployment.
        new_deployment.spec.desired_revision.CopyFrom(model_revision)

        APIClient.DeploymentService.create_deployment(deployment=new_deployment)

    return {
        "deployment_name": deployment_name,
        "model_revision_name": model_revision_name,
    }


@star_plugin(binding="deployment.wait_for_deployment")
def wait_for_deployment(
    namespace: str,
    deployment_name: str,
    expected_model_revision_name: str,
    timeout: int = 31536000,
    poll: int = 600
) -> Dict[str, str]:
    """Waits for deployment to reach terminal state by polling the live Deployment resource.

    The expected_model_revision_name parameter ensures each workflow tracks its intended model,
    preventing race conditions when multiple workflows update the same deployment concurrently.

    Args:
        namespace: The Michelangelo namespace containing the deployment
        deployment_name: The name of the deployment to wait for
        expected_model_revision_name: The model revision this workflow expects to be deployed.
            If the deployment's desired revision changes to a different model, this will fail
            immediately to prevent tracking the wrong deployment.
        timeout: Maximum time to wait in seconds (default: 31536000 = 1 year)
        poll: Polling interval in seconds (default: 600 = 10 minutes)

    Returns:
        dict: Dictionary containing:
            - stage: The terminal deployment stage name
            - current_revision: The currently deployed model revision (empty string if not set)
            - desired_revision: The desired model revision

    Raises:
        RuntimeError: If deployment fails, timeout occurs, or deployment was updated by another workflow
        grpc.RpcError: For service call failures
    """
    start_time = time.time()
    attempt = 0

    print(f"Waiting for deployment: {deployment_name} | Expected model: {expected_model_revision_name} | Timeout: {timeout}s | Poll: {poll}s")

    while True:
        attempt += 1
        elapsed_time = time.time() - start_time

        # Get the live deployment resource
        deployment = APIClient.DeploymentService.get_deployment(namespace=namespace, name=deployment_name)

        stage = deployment.status.stage
        desired_rev = deployment.spec.desired_revision.name
        current_rev = deployment.status.current_revision.name if deployment.status.current_revision else None
        stage_name = DeploymentStage.Name(stage)

        # Check if deployment was updated by another workflow - fail immediately if expected revision doesn't match
        if desired_rev != expected_model_revision_name:
            print(f"Deployment was updated by another workflow | Expected: {expected_model_revision_name}, Current: {desired_rev} | Elapsed: {elapsed_time:.1f}s")
            raise RuntimeError(f"deployment was updated by another workflow: expected model revision {expected_model_revision_name}, but deployment now targets {desired_rev}")

        # Check if deployment reached terminal state (success or failure)
        if stage in _DEPLOYMENT_SUCCESS_STAGES:
            print(f"Deployment completed successfully | Stage: {stage_name} | Elapsed: {elapsed_time:.1f}s")
            return {
                "stage": stage_name,
                "current_revision": current_rev or "",
                "desired_revision": desired_rev,
            }

        if stage in _DEPLOYMENT_FAILED_STAGES:
            print(f"Deployment failed | Stage: {stage_name} | Elapsed: {elapsed_time:.1f}s")
            raise RuntimeError(f"deployment failed with stage: {stage_name}")

        # Check if timeout has been exceeded
        if time.time() - start_time >= timeout:
            print(f"Deployment timeout | Stage: {stage_name} | Elapsed: {elapsed_time:.1f}s")
            raise RuntimeError(f"timeout waiting for deployment {deployment_name}")

        # Non-terminal state - log and retry
        print(f"Attempt #{attempt} | Stage: {stage_name} | Current revision: {current_rev}, Desired revision: {desired_rev} | Elapsed: {elapsed_time:.1f}s | Sleeping {poll}s")
        time.sleep(poll)


@workflow()
def model_deployment(
    namespace: str,
    deployment_name: str,
    model_revision_name: str,
    deployment_template: Optional[str] = None,
    async_deployment: bool = False,
    timeout: int = 31536000,
) -> Dict[str, str]:
    """
    Model deployment workflow that creates/updates a deployment and waits for completion.

    This workflow orchestrates the complete model deployment process by:
    1. Creating or updating a deployment with the specified model revision
    2. Waiting for the deployment to reach a terminal state

    Args:
        namespace: Michelangelo namespace
        deployment_name: Name of the deployment
        model_revision_name: Model revision to deploy
        deployment_template: Template to use for new deployments (optional)
        async_deployment: Skip waiting if True (default: False)
        timeout: Maximum time to wait in seconds (default: 31536000 = 1 year)

    Returns:
        dict: Results containing deployment_name, model_revision_name, and final_stage
    """
    # Create or update the deployment
    create_result = create_or_update_deployment(
        namespace=namespace,
        deployment_name=deployment_name,
        model_revision_name=model_revision_name,
        deployment_template=deployment_template,
    )

    # Wait for deployment completion (unless async deployment is enabled)
    if not async_deployment:
        wait_result = wait_for_deployment(
            namespace=namespace,
            deployment_name=create_result["deployment_name"],
            expected_model_revision_name=create_result["model_revision_name"],
            timeout=timeout,
        )
        final_stage = wait_result["stage"]
    else:
        final_stage = "ASYNC_DEPLOYMENT_STARTED"

    return {
        "deployment_name": create_result["deployment_name"],
        "model_revision_name": create_result["model_revision_name"],
        "final_stage": final_stage,
    }
