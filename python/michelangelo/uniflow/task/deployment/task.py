import logging
from michelangelo.gen.api.v2.deployment_pb2 import Deployment
from michelangelo.uniflow.core import star_plugin

log = logging.getLogger(__name__)


@star_plugin("deployment.deploy")
def deploy(namespace, name, model_name):
    """
    Decorator to deploy model based on model name.

    Args:
        namespace (str): Deployment namespace.
        name (str): Deployment name.
        model_name (str): URL of the model to be deployed.

    """
    pass


@star_plugin("deployment.create_deployment")
def create_deployment(deployment: Deployment):
    """
    Decorator to create a deployment task directly from the workflow definition.

    Args:
        deployment (Deployment): Deployment CR.

    """
    pass
