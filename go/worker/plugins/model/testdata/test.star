load("@plugin", "model")

def test_model_search():
    return model.model_search(
        namespace = "default",
        deployment_name = "test-model-deployment",
    )

def main():
    return test_model_search()
