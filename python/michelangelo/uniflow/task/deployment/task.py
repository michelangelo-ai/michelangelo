import logging

from michelangelo.uniflow.core import star_plugin

log = logging.getLogger(__name__)

@star_plugin("deployment.deploy")
def deploy(namespace, name, model_name):
    """
    Decorator to create a deployment task directly from the workflow definition.

    Args:
        namespace (str): Deployment namespace.
        name (str): Deployment name.
        model_name (str): URL of the model to be deployed.

    """
    pass
