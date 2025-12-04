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

    For existing deployments: updates the deployment to use the new revision and waits for the change to be applied.
    For new deployments: clones from the specified template deployment with the desired revision.

    Args:
        namespace: The Michelangelo namespace for all resources
        deployment_name: The name of the deployment to create or update
        model_revision_name: The name of the model revision to deploy
        deployment_template: Template deployment to clone from (required for new deployments)

    Returns:
        dict: Dictionary containing the deployment_revision_name

    Raises:
        ValueError: If deployment_template is required but not provided
        RuntimeError: If no revisions are found or if the revision didn't update
        grpc.RpcError: For service call failures (network, auth, permissions, etc.)
    """
    old_revision = None

    # Configure query options for deployment revisions:
    # 1. Filter by deployment name and kind="Deployment"
    # 2. Order by most recent update timestamp
    # 3. Limit to 1 result to get the latest revision
    list_options_ext = {
        "operation": {
            "criterion": [
                {"field_name": "revision.spec.base_resource.name", "match_value": deployment_name, "operator": "CRITERION_OPERATOR_EQUAL"},
                {"field_name": "revision.spec.base_type.kind", "match_value": "Deployment", "operator": "CRITERION_OPERATOR_EQUAL"},
            ],
            "logical_operator": "LOGICAL_OPERATOR_AND",
        },
        "order_by": [{"field": "metadata.update_timestamp", "dir": "SORT_ORDER_DESC"}],
        "pagination": {"offset": 0, "limit": 1},
    }

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
        # Capture the current revision name before updating so we can verify the change later.
        list_response = APIClient.RevisionService.list_revision(namespace=namespace, list_options_ext=list_options_ext)
        if not list_response.items:
            raise RuntimeError("deployment exists, but no deployment revisions found")

        old_revision = list_response.items[0].metadata.name

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

        # NOTE: OSS does not have DeploymentExtService.clone_deployment()
        # Instead, we manually clone the template deployment by fetching it first.
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

    # Wait for the new deployment revision to be created and indexed with retry logic.
    for attempt in range(5):
        list_response = APIClient.RevisionService.list_revision(namespace=namespace, list_options_ext=list_options_ext)

        # Check if we found any revisions.
        if not list_response.items:
            if attempt < 4:
                time.sleep(5)
                continue
            raise RuntimeError("no deployment revisions found")

        latest_revision_name = list_response.items[0].metadata.name

        # For updates, verify that the revision has actually changed from the old one.
        if old_revision is not None and latest_revision_name == old_revision:
            if attempt < 4:
                time.sleep(5)
                continue
            raise RuntimeError("latest revision is the same as the old revision")

        # Revision has been successfully created or updated.
        return {"deployment_revision_name": latest_revision_name}


@star_plugin(binding="deployment.wait_for_deployment")
def wait_for_deployment(
    namespace: str,
    deployment_revision_name: str,
    timeout: int = 31536000,
    poll: int = 600
) -> Dict[str, str]:
    """Waits for deployment revision to reach terminal state.

    Args:
        namespace: The Michelangelo namespace containing the revision
        deployment_revision_name: The name of the deployment revision to wait for
        timeout: Maximum time to wait in seconds (default: 1 year)
        poll: Polling interval in seconds (default: 600 seconds)

    Returns:
        dict: Dictionary containing the deployment stage {"stage": <stage_name>}

    Raises:
        RuntimeError: If deployment fails or timeout occurs
        grpc.RpcError: For service call failures
    """
    start_time = time.time()
    attempt = 0

    print(f"Waiting for deployment revision: {deployment_revision_name} | Timeout: {timeout}s | Poll: {poll}s")

    while True:
        attempt += 1
        elapsed_time = time.time() - start_time

        # Get the revision
        revision_response = APIClient.RevisionService.get_revision(namespace=namespace, name=deployment_revision_name)

        # Unmarshal the deployment from the revision content
        deployment = Deployment()
        revision_response.spec.content.Unpack(deployment)

        # Check deployment stage
        stage = deployment.status.stage
        stage_name = DeploymentStage.Name(stage)

        # Check if deployment reached a successful terminal state
        if stage in _DEPLOYMENT_SUCCESS_STAGES:
            print(f"Deployment completed successfully | Stage: {stage_name} | Elapsed: {elapsed_time:.1f}s")
            return {"stage": stage_name}

        # Check if deployment reached a failed terminal state
        if stage in _DEPLOYMENT_FAILED_STAGES:
            print(f"Deployment failed | Stage: {stage_name} | Elapsed: {elapsed_time:.1f}s")
            raise RuntimeError(f"deployment failed with stage: {stage_name}")

        # Check if timeout has been exceeded
        if time.time() - start_time >= timeout:
            print(f"Deployment timeout | Stage: {stage_name} | Elapsed: {elapsed_time:.1f}s")
            raise RuntimeError(f"timeout waiting for deployment revision {deployment_revision_name}")

        # Log current deployment status and next retry
        print(f"Attempt #{attempt} | Stage: {stage_name} | Elapsed: {elapsed_time:.1f}s | Sleeping {poll}s")

        # Sleep before next poll
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
        dict: Results containing deployment_revision_name and final_stage
    """
    # Create or update the deployment
    create_result = create_or_update_deployment(
        namespace=namespace,
        deployment_name=deployment_name,
        model_revision_name=model_revision_name,
        deployment_template=deployment_template,
    )
    actual_deployment_revision_name = create_result["deployment_revision_name"]

    # Wait for deployment completion (unless async deployment is enabled)
    if not async_deployment:
        wait_result = wait_for_deployment(
            namespace=namespace,
            deployment_revision_name=actual_deployment_revision_name,
            timeout=timeout,
        )
        final_stage = wait_result["stage"]
    else:
        final_stage = "ASYNC_DEPLOYMENT_STARTED"

    return {
        "deployment_revision_name": actual_deployment_revision_name,
        "final_stage": final_stage,
    }
