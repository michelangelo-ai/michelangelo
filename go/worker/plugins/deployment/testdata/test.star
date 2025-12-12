load("@plugin", "deployment")

def test_create_deployment():
    """Test creating a new deployment from a template."""
    return deployment.create_or_update_deployment(
        namespace = "ma-dev-test",
        deployment_name = "test-deployment-1",
        model_revision_name = "test-model-revision-1",
        deployment_template = "test-template-deployment",
    )

def test_update_deployment():
    """Test updating an existing deployment to a new model revision."""
    return deployment.create_or_update_deployment(
        namespace = "ma-dev-test",
        deployment_name = "test-deployment-1",
        model_revision_name = "test-model-revision-2",
    )

def test_wait_for_deployment(expected_model_revision_name):
    """Test waiting for deployment to reach terminal state."""
    return deployment.wait_for_deployment(
        namespace = "ma-dev-test",
        deployment_name = "test-deployment-1",
        expected_model_revision_name = expected_model_revision_name,
        timeout = 60,  # 60 seconds for testing
        poll = 5,      # Poll every 5 seconds
    )

def main():
    """
    Main test flow:
    1. Create deployment with revision-1
    2. Wait for deployment to reach terminal state
    3. Update deployment to revision-2
    4. Wait for deployment again
    """
    # Test CREATE path
    create_result = test_create_deployment()
    print("Create result:", create_result)
    
    # Test WAIT - uncomment when testing with active deployment controller
    wait_result = test_wait_for_deployment(create_result["model_revision_name"])
    print("Wait result:", wait_result)
    
    # Test UPDATE path
    update_result = test_update_deployment()
    print("Update result:", update_result)
    
    wait_result2 = test_wait_for_deployment(update_result["model_revision_name"])
    print("Wait result 2:", wait_result2)

    return {
        "create_result": create_result,
        "wait_result": wait_result,
        "update_result": update_result,
        "wait_result2": wait_result2,
    }

