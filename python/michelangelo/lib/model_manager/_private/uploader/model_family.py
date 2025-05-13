import grpc
import logging
from uber.gen.code_uber_internal.uberai.michelangelo.api.v2beta1.model_family_proto import ModelFamily

_logger = logging.getLogger(__name__)


def create_model_family(project_name: str, model_family_name: str) -> ModelFamily:
    """
    Create a model family in Michelangelo Unified API.
    The model family needs to be unique across all the namespaces.
    If the model family already exists, it will not be created again.
    If the model family exists in another namespace, it will raise an error.

    Args:
        project_name (str): Name of the project.
        model_family_name (str): Name of the model family.

    Returns:
        Model family object if the model family is created successfully, otherwise None.
    """
    from michelangelo.lib.model_manager._private.utils.api_client import APIClient

    model_family = ModelFamily()
    model_family.metadata.namespace = project_name
    model_family.metadata.name = model_family_name
    model_family.spec.name = model_family_name

    try:
        APIClient.ModelFamilyService.create_model_family(model_family)
    except grpc.RpcError as e:
        if e.code() == grpc.StatusCode.ALREADY_EXISTS:
            try:
                APIClient.ModelFamilyService.get_model_family(
                    namespace=project_name,
                    name=model_family_name,
                )
            except grpc.RpcError as ge:
                if ge.code() == grpc.StatusCode.NOT_FOUND:
                    raise RuntimeError(
                        f"Failed to create model family {model_family_name}. "
                        f"The model family {model_family_name} already exists in another namespace. "
                        "Please use a different model family name."
                    ) from e
                else:
                    raise RuntimeError(
                        f"Failed to create model family {model_family_name}. "
                        f"The model family {model_family_name} already exists. "
                        "But we cannot fetch it for unknown reason. "
                        "Please try again."
                    ) from ge
            else:
                _logger.info(f"Model family {model_family_name} already exists.")
        else:
            raise
    else:
        return model_family
