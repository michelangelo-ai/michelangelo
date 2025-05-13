import os
import shutil
import tempfile
from typing import Optional
from michelangelo.lib.model_manager.utils.terrablob_paths import get_v1_projects_model_zip_path
from michelangelo._internal.gateways.terrablob_gateway import download_from_terrablob
from michelangelo.lib.model_manager._private.utils.terrablob_utils import get_download_multipart_options


def download_legacy_ma_model(
    project_id: str,
    model_id: str,
    dest_model_path: str,
    timeout: Optional[str] = None,
    source_entity: Optional[str] = None,
):
    """
    Download a legacy Michelangelo model.

    Args:
        project_id (str): The project ID in the legacy Michelangelo API
        model_id (str): The model ID in the legacy Michelangelo API
        dest_model_path (str): The path to save the model files.
        timeout (str, optional): The timeout for downloading the model. Defaults to None.
        source_entity (str, optional): The source entity for terrablob command when downloading the model. Defaults to None.

    Returns:
        None
    """
    tb_model_zip_path = get_v1_projects_model_zip_path(project_id, model_id)

    with tempfile.TemporaryDirectory() as temp_dir:
        model_zip_path = os.path.join(temp_dir, "model.zip")
        download_from_terrablob(
            tb_model_zip_path,
            model_zip_path,
            **get_download_multipart_options(),
            timeout=timeout,
            source_entity=source_entity,
        )
        shutil.unpack_archive(model_zip_path, dest_model_path, "zip")
