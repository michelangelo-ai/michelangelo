import os
import shutil
import tempfile
from typing import Optional
from michelangelo.lib.model_manager.utils.terrablob_paths import (
    get_deployable_model_base_path,
    get_deployable_model_tar_path,
)
from uber.ai.michelangelo.shared.gateways.terrablob_gateway import download_from_terrablob
from michelangelo.lib.model_manager.utils.model import retrieve_model_assets
from michelangelo.lib.model_manager._private.utils.terrablob_utils import (
    get_terrablob_auth_mode,
    get_download_multipart_options,
)
from michelangelo.lib.model_manager._private.utils.model_utils import get_latest_uploaded_model_revision


def download_generic_deployable_model(
    project_name: str,
    model_name: str,
    model_revision: str,
    package_type: str,
    dest_model_path: str,
    timeout: Optional[str] = None,
    source_entity: Optional[str] = None,
):
    """
    Download a generic deployable model.

    Args:
        project_name (str): The name of the project.
        model_name (str): The name of the model.
        model_revision (str): The revision of the model.
        package_type (str): The package type of the model, e.g (triton | raw).
        dest_model_path (str): The path to save the model files.
        timeout (str, optional): The timeout for downloading the model. Defaults to None.
        source_entity (str, optional): The source entity for terrablob command when downloading the model. Defaults to None.

    Returns:
        None
    """
    if model_revision is None or model_revision == "":
        model_revision = get_latest_model_revision(project_name, model_name, package_type, timeout=timeout, source_entity=source_entity)

    if model_revision is None:
        raise ValueError(f"No model revision found for the model {model_name} in project {project_name}. Most likely the model does not exist.")

    tb_model_tar_path = get_deployable_model_tar_path(
        project_name,
        model_name,
        model_revision,
        package_type,
    )

    with tempfile.TemporaryDirectory() as temp_dir:
        tar_local_path = os.path.join(temp_dir, "model.tar")
        download_from_terrablob(
            tb_model_tar_path,
            tar_local_path,
            **get_download_multipart_options(),
            timeout=timeout,
            source_entity=source_entity,
            auth_mode=get_terrablob_auth_mode(),
        )
        shutil.unpack_archive(tar_local_path, dest_model_path, "tar")

    retrieve_model_assets(dest_model_path, timeout=timeout, source_entity=source_entity)


def get_latest_model_revision(project_name: str, model_name: str, package_type: str, timeout: str, source_entity: str) -> str:
    """
    Get the latest model revision for the model

    Args:
        project_name (str): the name of the project
        model_name (str): the name of the model
        package_type (str): the package type of the model, e.g (triton | raw)
        timeout (str): the timeout for downloading the model
        source_entity (str): the source entity for terrablob command when downloading the model

    Returns:
        The latest model revision. If the model is not found, return None
    """

    def get_model_path(revision_id: int) -> str:
        return get_deployable_model_tar_path(project_name, model_name, str(revision_id), package_type)

    base_path = f"{get_deployable_model_base_path(project_name, model_name)}/revisions"

    return get_latest_uploaded_model_revision(
        project_name,
        model_name,
        get_model_path,
        base_path,
        timeout=timeout,
        source_entity=source_entity,
    )
