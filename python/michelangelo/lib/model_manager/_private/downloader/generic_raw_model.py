from typing import Optional
from uber.ai.michelangelo.shared.gateways.terrablob_gateway import download_from_terrablob
from michelangelo.lib.model_manager.utils.terrablob_paths import (
    get_raw_model_main_path,
    get_raw_model_base_path,
)
from michelangelo.lib.model_manager._private.utils.model_utils import get_latest_uploaded_model_revision
from michelangelo.lib.model_manager._private.utils.terrablob_utils import (
    get_terrablob_auth_mode,
    get_download_multipart_options,
)


def download_generic_raw_model(
    project_name: str,
    model_name: str,
    model_revision: str,
    dest_model_path: str,
    timeout: Optional[str] = None,
    source_entity: Optional[str] = None,
):
    """
    Download a generic raw model

    Args:
        project_name (str): The name of the project.
        model_name (str): The name of the model.
        model_revision (str): The revision of the model.
        dest_model_path (str): The path to save the model files.
        timeout (str, optional): The timeout for downloading the model. Defaults to None.
        source_entity (str, optional): The source entity for terrablob command when downloading the model. Defaults to None.
    """
    if model_revision is None or model_revision == "":
        model_revision = get_latest_model_revision(project_name, model_name, timeout=timeout, source_entity=source_entity)

    if model_revision is None:
        raise ValueError(f"No model revision found for the raw model {model_name} in project {project_name}. Most likely the model does not exist.")

    src_model_path = get_raw_model_main_path(project_name, model_name, model_revision)

    download_from_terrablob(
        src_model_path,
        dest_model_path,
        **get_download_multipart_options(),
        timeout=timeout,
        source_entity=source_entity,
        auth_mode=get_terrablob_auth_mode(),
    )


def get_latest_model_revision(project_name: str, model_name: str, timeout: str, source_entity: str) -> str:
    """
    Get the latest model revision for the raw model

    Args:
        project_name (str): the name of the project
        model_name (str): the name of the model
        timeout (str): the timeout for downloading the model
        source_entity (str): the source entity for terrablob command when downloading the model

    Returns:
        The latest model revision. If the model is not found, return None
    """

    def get_model_path(revision_id: int) -> str:
        return get_raw_model_main_path(project_name, model_name, str(revision_id))

    base_path = f"{get_raw_model_base_path(project_name, model_name)}/revisions"

    return get_latest_uploaded_model_revision(
        project_name,
        model_name,
        get_model_path,
        base_path,
        timeout=timeout,
        source_entity=source_entity,
    )
