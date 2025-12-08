load("@plugin", "deployment")

def test_create_or_update_deployment():
    return deployment.create_or_update_deployment(
        namespace = "ma-dev-test",
        deployment_name = "test-deployment-1",
        model_revision_name = "test-model-revision-1",
        deployment_template = "test-template-deployment",
    )

def test_wait_for_deployment():
    return deployment.wait_for_deployment(
        namespace = "ma-dev-test",
        deployment_revision_name = "test-deployment-1",
        timeout = 10,
        poll = 1,
    )

def main():
    return test_create_or_update_deployment()

