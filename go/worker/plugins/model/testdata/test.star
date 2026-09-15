load("@plugin", "model")

def test_model_search():
    return model.model_search(
        namespace = "default",
        deployment_name = "test-model-deployment",
    )

def test_get_models_by_pipeline_run():
    return model.get_models_by_pipeline_run(
        namespace = "default",
        pipeline_run_name = "child-run",
    )
