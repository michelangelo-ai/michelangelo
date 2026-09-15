load("@plugin", "model")

def test_model_search():
    return model.model_search(
        namespace = "default",
        deployment_name = "test-model-deployment",
    )

def test_deploy_model():
    return model.deploy_model(
        namespace = "default",
        deployment_name = "retrain-deployment",
        pipeline_run_name = "child-run",
        inference_server_name = "inference-server",
        actor = "integration-test",
        timeout_seconds = 60,
        poll_seconds = 1,
    )
