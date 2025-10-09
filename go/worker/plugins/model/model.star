"""
model.star allows you to implement plugins related to managing models in starlark code.

Functions:
    model_search: searches for a model based on the specified criteria.

        Arguments:
            namespace: str: Namespace, also known as Michelangelo Project ID, where the model is located.
            deployment_name: str: The deployment name of the model. Use this criterion to find a deployed model by the deployment name. For example, this way you can find the latest model currently in production.

        Returns:
            model_details: dict: Model details
                namespace: str: Namespace
                model_name: str: Name of the model
                model_revision_id: int: RevisionID of the model
"""

load("@plugin", "model")

def main():
    return test_model_search()

def test_model_search():
    """
    This is an example of how to use the modelsearch plugin.
    """
    # Note: Ensure that both a model and its corresponding deployment Custom Resource (CR) exist for the example below to work.
    namespace = "default"
    deployment_name = "test-model"

    data = model.model_search(namespace, deployment_name)
    return data
