load("@plugin", "deployment")

def test_create_or_update_deployment():
    return deployment.create_or_update_deployment(
        namespace = "test-namespace",
        deployment_name = "test-deployment",
        model_revision_name = "test-revision",
        deployment_template = "test-template",
    )

def test_wait_for_deployment():
    return deployment.wait_for_deployment(
        namespace = "test-namespace",
        deployment_revision_name = "test-deployment-1",
        timeout = 10,
        poll = 1,
    )

def main():
    return test_create_or_update_deployment()

