load("@plugin", "deployment")

def test_create_deployment():
    """Test creating a new deployment from a template."""
    return deployment.create_or_update_deployment(
        namespace = "ma-dev-test",
        deployment_name = "test-deployment-1",
        model_revision_name = "test-model-revision-1",
        deployment_template = "test-template-deployment",
    )

def test_create_deployment_without_template():
    """Test that creating a new deployment without a template fails."""
    return deployment.create_or_update_deployment(
        namespace = "ma-dev-test",
        deployment_name = "test-deployment-1",
        model_revision_name = "test-model-revision-1",
    )

def test_update_deployment():
    """Test updating an existing deployment to a new model revision."""
    return deployment.create_or_update_deployment(
        namespace = "ma-dev-test",
        deployment_name = "test-deployment-1",
        model_revision_name = "test-model-revision-2",
    )

def test_create_or_update_deployment_hard_failure():
    """Test that a non-not-found GetDeployment error is propagated, not treated as create."""
    return deployment.create_or_update_deployment(
        namespace = "ma-dev-test",
        deployment_name = "test-deployment-1",
        model_revision_name = "test-model-revision-1",
        deployment_template = "test-template-deployment",
    )

def test_wait_for_deployment(expected_model_revision_name):
    """Test waiting for deployment to reach terminal state."""
    return deployment.wait_for_deployment(
        namespace = "ma-dev-test",
        deployment_name = "test-deployment-1",
        expected_model_revision_name = expected_model_revision_name,
        timeout = 60,
        poll = 5,
    )
