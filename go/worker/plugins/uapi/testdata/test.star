load("@plugin", "uapi")

def test_model_search():
    return uapi.model_search(
        namespace = "default",
        deployment_name = "test-model-deployment",
    )
