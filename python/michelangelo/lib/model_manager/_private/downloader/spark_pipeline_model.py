from typing import Optional
from michelangelo._internal.gateways.terrablob_gateway import path_exists
from michelangelo.lib.model_manager._private.downloader.v2_projects_model import download_v2_projects_model
from michelangelo.lib.model_manager._private.downloader.legacy_ma_model import download_legacy_ma_model
from michelangelo.lib.model_manager.utils.terrablob_paths import (
    get_v2_projects_model_jar_path,
    get_v2_projects_model_base_path,
)
from michelangelo.lib.model_manager._private.utils.model_utils import get_latest_uploaded_model_revision


def download_spark_pipeline_model(
    project_name: str,
    model_name: str,
    model_revision: str,
    dest_model_path: str,
    timeout: Optional[str] = None,
    source_entity: Optional[str] = None,
):
    """
    Download a spark pipeline model from Michelangelo.

    Args:
        project_name (str): The name of the project.
        model_name (str): The name of the model.
        model_revision (str): The revision of the model.
        dest_model_path (str): The path to save the model files.
        timeout (str, optional): The timeout for downloading the model. Defaults to None.
        source_entity (str, optional): The source entity for terrablob command when downloading the model. Defaults to None.

    Returns:
        None
    """
    if model_revision is None or model_revision == "":
        model_revision = get_latest_model_revision(project_name, model_name, timeout=timeout, source_entity=source_entity)

    v2_projects_model_jar_path = get_v2_projects_model_jar_path(project_name, model_name, model_revision)
    if model_revision is not None and path_exists(v2_projects_model_jar_path, timeout=timeout, source_entity=source_entity):
        download_v2_projects_model(
            project_name,
            model_name,
            model_revision,
            dest_model_path,
            timeout=timeout,
            source_entity=source_entity,
        )


def get_latest_model_revision(project_name: str, model_name: str, timeout: str, source_entity: str) -> str:
    """
    Get the latest model revision of a model in a project.

    Args:
        project_name (str): The name of the project.
        model_name (str): The name of the model.
        timeout (str): The timeout for downloading the model.
        source_entity (str): The source entity for terrablob command when downloading the model.

    Returns:
        str: The latest model revision. If the model is not found, return None.
    """

    def get_model_path(revision_id: int) -> str:
        return get_v2_projects_model_jar_path(project_name, model_name, str(revision_id))

    base_model_path = get_v2_projects_model_base_path(project_name, model_name)

    return get_latest_uploaded_model_revision(
        project_name,
        model_name,
        get_model_path,
        base_model_path,
        timeout=timeout,
        source_entity=source_entity,
    )
